'use server'

import { db } from "@/src/db";
import { users } from "@/src/schema";
import { eq, desc, and, gt } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { headers } from "next/headers";
import { getSession } from "@/src/lib/auth";
import { decrypt, computeBlindIndex, hashToken } from "@/src/lib/crypto";
import { sendEmail } from "@/src/lib/mail";
import { getHtmlTemplate } from "@/src/lib/email-templates";
import {
  forgotPasswordSchema,
  resetPasswordSchema,
  changePasswordSchema
} from "@/src/lib/validations";
import { checkRateLimit, RATE_LIMIT_CONFIGS } from "@/src/lib/rate-limit";
import logger from "@/src/lib/logger";

async function getUser() {
  const session = await getSession();
  if (!session || !session.user) {
    redirect('/login');
  }
  return session.user;
}

async function checkAdmin() {
  const user = await getUser();
  const [dbUser] = await db.select().from(users).where(eq(users.id, user.id));
  if (!dbUser || dbUser.role !== 'ADMIN') {
    throw new Error("Accès refusé");
  }
  return dbUser;
}

async function getClientIP(): Promise<string> {
  const headersList = await headers();
  return headersList.get('x-forwarded-for')?.split(',')[0]?.trim() 
    || headersList.get('x-real-ip') 
    || 'unknown';
}

export async function forgotPasswordAction(formData: FormData) {
  const { randomBytes } = await import('crypto');

  const clientIP = await getClientIP();
  const rateCheck = checkRateLimit(clientIP, 'forgotPassword', RATE_LIMIT_CONFIGS.forgotPassword);

  if (!rateCheck.allowed) {
    const waitMin = Math.ceil(rateCheck.retryAfterMs / 60000);
    return { error: `Trop de demandes. Réessayez dans ${waitMin} min.` };
  }

  const rawData = Object.fromEntries(formData.entries());
  const validation = forgotPasswordSchema.safeParse(rawData);

  if (!validation.success) {
    return { error: validation.error.issues[0].message };
  }

  const { email } = validation.data;
  const emailIndex = computeBlindIndex(email);
  const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));

  if (!user) {
    return { success: true };
  }

  const token = randomBytes(32).toString('hex');
  const hashedToken = hashToken(token);
  const expiry = new Date(Date.now() + 3600000);

  await db.update(users)
    .set({ resetToken: hashedToken, resetTokenExpiry: expiry })
    .where(eq(users.id, user.id));

  const resetUrl = `https://${process.env.HOST}/reset-password?token=${token}`;
  await sendEmail({
    to: email,
    subject: "Réinitialisation mot de passe",
    html: getHtmlTemplate(
      "Mot de passe oublié ?",
      "Cliquez ci-dessous pour changer votre mot de passe. Ce lien expire dans 1 heure.",
      "Changer mot de passe",
      resetUrl
    )
  });

  logger.info({ userId: user.id }, 'Demande de réinitialisation de mot de passe');
  return { success: true };
}

export async function resetPasswordAction(formData: FormData) {
  const { hash } = await import('bcryptjs');

  const rawData = Object.fromEntries(formData.entries());
  const validation = resetPasswordSchema.safeParse(rawData);

  if (!validation.success) {
    return { error: validation.error.issues[0].message };
  }

  const { token, password } = validation.data;
  const hashedToken = hashToken(token);

  const [user] = await db.select().from(users).where(
    and(
      eq(users.resetToken, hashedToken),
      gt(users.resetTokenExpiry, new Date())
    )
  );

  if (!user) {
    return { error: "Lien invalide ou expiré." };
  }

  const hashedPassword = await hash(password, 10);
  const newVersion = (user.sessionVersion || 1) + 1;

  await db.update(users).set({
    password: hashedPassword,
    resetToken: null,
    resetTokenExpiry: null,
    sessionVersion: newVersion
  }).where(eq(users.id, user.id));

  logger.info({ userId: user.id }, 'Mot de passe réinitialisé');
  redirect('/login?reset=success');
}

export async function updatePasswordAction(formData: FormData) {
  const { hash, compare } = await import('bcryptjs');

  const userSession = await getUser();

  const rawData = Object.fromEntries(formData.entries());
  const validation = changePasswordSchema.safeParse(rawData);

  if (!validation.success) {
    return { error: validation.error.issues[0].message };
  }

  const { currentPassword, newPassword } = validation.data;

  const [dbUser] = await db.select().from(users).where(eq(users.id, userSession.id));
  if (!dbUser) {
    return { error: "Utilisateur introuvable" };
  }

  const match = await compare(currentPassword, dbUser.password);
  if (!match) {
    return { error: "Mot de passe actuel incorrect" };
  }

  const hashedPassword = await hash(newPassword, 10);
  const newVersion = (dbUser.sessionVersion || 1) + 1;

  await db.update(users)
    .set({ password: hashedPassword, sessionVersion: newVersion })
    .where(eq(users.id, userSession.id));

  logger.info({ userId: userSession.id }, 'Mot de passe mis à jour');
  return { success: true };
}

export async function getAllUsers() {
  await checkAdmin();

  const rawUsers = await db.select().from(users).orderBy(desc(users.createdAt));
  return rawUsers.map((u: any) => ({
    ...u,
    email: decrypt(u.emailEncrypted)
  }));
}

export async function deleteUser(userId: number) {
  const admin = await checkAdmin();

  if (admin.id === userId) {
    return { error: "Impossible de se supprimer soi-même." };
  }

  await db.delete(users).where(eq(users.id, userId));
  logger.info({ deletedUserId: userId, adminId: admin.id }, 'Utilisateur supprimé');
  revalidatePath("/admin");
}

export async function getRegistrationStatus(): Promise<boolean> {
  const allUsers = await db.select().from(users);
  if (allUsers.length === 0) {
    return true;
  }
  return process.env.ALLOW_REGISTER === 'true';
}

export async function getMailStatus(): Promise<boolean> {
  return process.env.ENABLE_MAIL === 'true';
}
