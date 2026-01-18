'use server'
import { db } from "@/src/db";
import { users } from "@/src/schema";
import { eq, desc, and, gt } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { getSession } from "@/src/lib/auth";
import { decrypt, computeBlindIndex, hashToken } from "@/src/lib/crypto";
import { sendEmail } from "@/src/lib/mail";
import { getHtmlTemplate } from "@/src/lib/email-templates";
async function getUser() {
    const session = await getSession();
    if (!session || !session.user) redirect('/login');
    return session.user;
}
async function checkAdmin() {
    const user = await getUser();
    const [dbUser] = await db.select().from(users).where(eq(users.id, user.id));
    if (!dbUser || dbUser.role !== 'ADMIN') throw new Error("Accès refusé");
    return dbUser;
}
function isValidEmail(email: string): boolean {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}
function validatePasswordStrength(password: string) {
    if (password.length < 8) return { valid: false, error: "8 caractères min." };
    if (!/[A-Z]/.test(password)) return { valid: false, error: "1 majuscule." };
    if (!/[a-z]/.test(password)) return { valid: false, error: "1 minuscule." };
    if (!/[0-9]/.test(password)) return { valid: false, error: "1 chiffre." };
    if (!/[!@#$%^&*(),.?":{}|<>]/.test(password)) return { valid: false, error: "1 spécial." };
    return { valid: true, error: null };
}
export async function forgotPasswordAction(formData: FormData) {
    const { randomBytes } = await import('crypto');
    const email = formData.get('email') as string;
    if (!email || !isValidEmail(email)) return { error: "Email invalide" };
    const emailIndex = computeBlindIndex(email);
    const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (!user) return { success: true }; 
    const token = randomBytes(32).toString('hex');
    const hashedToken = hashToken(token);
    const expiry = new Date(Date.now() + 3600000);
    await db.update(users).set({ resetToken: hashedToken, resetTokenExpiry: expiry }).where(eq(users.id, user.id));
    const resetUrl = `https://${process.env.HOST}/reset-password?token=${token}`;
    await sendEmail({
        to: email,
        subject: "Réinitialisation mot de passe",
        html: getHtmlTemplate("Mot de passe oublié ?", "Cliquez ci-dessous pour changer votre mot de passe.", "Changer mot de passe", resetUrl)
    });
    return { success: true };
}
export async function resetPasswordAction(formData: FormData) {
    const { hash } = await import('bcryptjs');
    const token = formData.get('token') as string;
    const password = formData.get('password') as string;
    if (!token || !password) return { error: "Données manquantes" };
    const pwdCheck = validatePasswordStrength(password);
    if (!pwdCheck.valid) return { error: pwdCheck.error };
    const hashedToken = hashToken(token);
    const [user] = await db.select().from(users).where(and(eq(users.resetToken, hashedToken), gt(users.resetTokenExpiry, new Date())));
    if (!user) return { error: "Lien invalide ou expiré." };
    const hashedPassword = await hash(password, 10);
    const newVersion = (user.sessionVersion || 1) + 1;
    await db.update(users).set({ 
        password: hashedPassword, 
        resetToken: null, 
        resetTokenExpiry: null,
        sessionVersion: newVersion 
    }).where(eq(users.id, user.id));
    redirect('/login?reset=success');
}
export async function updatePasswordAction(formData: FormData) {
    const { hash, compare } = await import('bcryptjs');
    const userSession = await getUser();
    const currentPassword = formData.get("currentPassword") as string;
    const newPassword = formData.get("newPassword") as string;
    const confirmPassword = formData.get("confirmPassword") as string;
    if (!currentPassword || !newPassword || !confirmPassword) return { error: "Champs requis" };
    if (newPassword !== confirmPassword) return { error: "Mots de passe différents" };
    const pwdCheck = validatePasswordStrength(newPassword);
    if (!pwdCheck.valid) return { error: pwdCheck.error };
    const [dbUser] = await db.select().from(users).where(eq(users.id, userSession.id));
    if (!dbUser) return { error: "Utilisateur introuvable" };
    const match = await compare(currentPassword, dbUser.password);
    if (!match) return { error: "Mot de passe actuel incorrect" };
    const hashedPassword = await hash(newPassword, 10);
    const newVersion = (dbUser.sessionVersion || 1) + 1;
    await db.update(users).set({ password: hashedPassword, sessionVersion: newVersion }).where(eq(users.id, userSession.id));
    return { success: true };
}
export async function getAllUsers() {
    await checkAdmin();
    const rawUsers = await db.select().from(users).orderBy(desc(users.createdAt));
    return rawUsers.map(u => ({ ...u, email: decrypt(u.emailEncrypted) }));
}
export async function deleteUser(userId: number) {
    const admin = await checkAdmin();
    if (admin.id === userId) return { error: "Impossible de se supprimer soi-même." };
    await db.delete(users).where(eq(users.id, userId));
    revalidatePath("/admin");
}
export async function getRegistrationStatus() {
    const allUsers = await db.select().from(users);
    if (allUsers.length === 0) return true;
    return process.env.ALLOW_REGISTER === 'true';
}
export async function getMailStatus() {
  return process.env.ENABLE_MAIL === 'true';
}