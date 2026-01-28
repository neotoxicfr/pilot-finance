'use server'

import { db } from "@/src/db";
import { users, authenticators, accounts, transactions, recurringOperations } from "@/src/schema";
import { eq, and, isNull } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { cookies, headers } from "next/headers";
import { login, getSession, logout as logoutLib } from "@/src/lib/auth";
import { encrypt, decrypt, computeBlindIndex, hashToken } from "@/src/lib/crypto";
import { sendEmail } from "@/src/lib/mail";
import { getHtmlTemplate } from "@/src/lib/email-templates";
import { authSchema, registerSchema } from "@/src/lib/validations";
import { checkRateLimit, RATE_LIMIT_CONFIGS } from "@/src/lib/rate-limit";
import logger from "@/src/lib/logger";

const RP_NAME = 'Pilot Finance';
const MAX_LOGIN_ATTEMPTS = 5;
const LOCK_TIME_MS = 15 * 60 * 1000;

function getHost(): string | null {
  return process.env.HOST || null;
}

function getRpID(): string {
  return getHost() || 'localhost';
}

function getOrigin(): string {
  return `https://${getRpID()}`;
}

async function getClientIP(): Promise<string> {
  const headersList = await headers();
  return headersList.get('x-forwarded-for')?.split(',')[0]?.trim() 
    || headersList.get('x-real-ip') 
    || 'unknown';
}

export async function authenticate(formData: FormData, isRegister: boolean) {
  const { hash, compare } = await import('bcryptjs');
  const { randomBytes } = await import('crypto');

  const clientIP = await getClientIP();
  const rawData = Object.fromEntries(formData.entries());
  const validation = isRegister ? registerSchema.safeParse(rawData) : authSchema.safeParse(rawData);

  if (!validation.success) {
    return { error: validation.error.issues[0].message };
  }

  const { email, password } = validation.data;
  const twoFactorCode = isRegister ? undefined : (validation.data as { twoFactorCode?: string }).twoFactorCode;
  const emailIndex = computeBlindIndex(email);

  if (isRegister) {
    const rateCheck = checkRateLimit(clientIP, 'register', RATE_LIMIT_CONFIGS.register);
    if (!rateCheck.allowed) {
      const waitMin = Math.ceil(rateCheck.retryAfterMs / 60000);
      return { error: `Trop de tentatives. Réessayez dans ${waitMin} min.` };
    }

    const allUsers = await db.select().from(users);
    const isFirstUser = allUsers.length === 0;

    if (!isFirstUser && process.env.ALLOW_REGISTER !== 'true') {
      return { error: "Inscriptions fermées." };
    }

    const existing = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (existing.length > 0) {
      return { error: "Email déjà utilisé." };
    }

    const hashedPassword = await hash(password, 10);

    try {
      const mailEnabled = process.env.ENABLE_MAIL === 'true';
      let verificationToken: string | null = null;
      let hashedVerificationToken: string | null = null;

      if (mailEnabled) {
        verificationToken = randomBytes(32).toString('hex');
        hashedVerificationToken = hashToken(verificationToken);
      }

      const userId = await db.transaction(async (tx: typeof db) => {
        const newUser = await tx.insert(users).values({
          emailEncrypted: encrypt(email),
          emailBlindIndex: emailIndex,
          password: hashedPassword,
          role: isFirstUser ? 'ADMIN' : 'USER',
          email_verified: !mailEnabled,
          verification_token: hashedVerificationToken,
          sessionVersion: 1
        }).returning();

        const newUserId = newUser[0].id;

        if (isFirstUser) {
          await tx.update(accounts).set({ userId: newUserId }).where(isNull(accounts.userId));
          await tx.update(transactions).set({ userId: newUserId }).where(isNull(transactions.userId));
          await tx.update(recurringOperations).set({ userId: newUserId }).where(isNull(recurringOperations.userId));
        }

        return newUserId;
      });

      logger.info({ userId, isFirstUser }, 'Nouvel utilisateur créé');

      if (mailEnabled && verificationToken) {
        const verifyUrl = `https://${process.env.HOST}/verify-email?token=${verificationToken}`;
        await sendEmail({
          to: email,
          subject: "Validez votre compte",
          html: getHtmlTemplate("Bienvenue", "Cliquez pour valider votre compte.", "Valider", verifyUrl)
        });
        return { success: true, message: "Vérifiez vos emails." };
      }

      await login({ id: userId, email, role: isFirstUser ? 'ADMIN' : 'USER', sessionVersion: 1 });
    } catch (e) {
      logger.error({ err: e }, "Erreur critique lors de l'inscription");
      return { error: "Erreur création compte." };
    }
  } else {
    const rateCheck = checkRateLimit(clientIP, 'login', RATE_LIMIT_CONFIGS.login);
    if (!rateCheck.allowed) {
      const waitMin = Math.ceil(rateCheck.retryAfterMs / 60000);
      return { error: `Trop de tentatives. Réessayez dans ${waitMin} min.` };
    }

    const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (!user) {
      return { error: "Identifiants incorrects" };
    }

    if (user.lockUntil && user.lockUntil > new Date()) {
      const waitMin = Math.ceil((user.lockUntil.getTime() - Date.now()) / 60000);
      logger.warn({ userId: user.id }, "Tentative de connexion sur compte verrouillé");
      return { error: `Compte verrouillé. Réessayez dans ${waitMin} min.` };
    }

    const match = await compare(password, user.password);
    if (!match) {
      const newFailCount = (user.failedLoginAttempts || 0) + 1;
      const updateData: { failedLoginAttempts: number; lockUntil?: Date } = { 
        failedLoginAttempts: newFailCount 
      };

      if (newFailCount >= MAX_LOGIN_ATTEMPTS) {
        updateData.lockUntil = new Date(Date.now() + LOCK_TIME_MS);
        updateData.failedLoginAttempts = 0;
        logger.warn({ userId: user.id }, "Compte verrouillé après trop d'échecs");
      }

      await db.update(users).set(updateData).where(eq(users.id, user.id));
      return { error: "Identifiants incorrects" };
    }

    if ((user.failedLoginAttempts || 0) > 0 || user.lockUntil) {
      await db.update(users).set({ failedLoginAttempts: 0, lockUntil: null }).where(eq(users.id, user.id));
    }

    if (process.env.ENABLE_MAIL === 'true' && !user.email_verified) {
      return { error: "Email non validé." };
    }

    if (user.mfaEnabled) {
      if (!twoFactorCode) {
        return { requires2FA: true };
      }
      const decryptedSecret = decrypt(user.mfaSecret!);
      const { authenticator } = await import('@otplib/preset-default');
      if (!authenticator.verify({ token: twoFactorCode, secret: decryptedSecret })) {
        logger.warn({ userId: user.id }, "Echec validation MFA");
        return { error: "Code A2F invalide", requires2FA: true };
      }
    }

    const decryptedEmail = decrypt(user.emailEncrypted);
    await login({ id: user.id, email: decryptedEmail, role: user.role ?? 'USER', sessionVersion: user.sessionVersion });
  }

  redirect('/');
}

export async function logoutAction() {
  await logoutLib();
  redirect('/login');
}

export async function verifyEmailAction(token: string) {
  const clientIP = await getClientIP();
  const rateCheck = checkRateLimit(clientIP, 'verifyEmail', RATE_LIMIT_CONFIGS.verifyEmail);

  if (!rateCheck.allowed) {
    return { error: "Trop de tentatives. Réessayez plus tard." };
  }

  const hashedToken = hashToken(token);
  const [user] = await db.select().from(users).where(eq(users.verification_token, hashedToken));

  if (!user) {
    return { error: "Token invalide." };
  }

  await db.update(users).set({ 
    email_verified: true, 
    verification_token: null 
  }).where(eq(users.id, user.id));

  logger.info({ userId: user.id }, 'Email vérifié');
  return { success: true };
}

export async function isPasskeyConfigured(): Promise<boolean> {
  return !!process.env.HOST;
}

export async function registerPasskeyStart() {
  if (!getHost()) {
    return { error: "Variable HOST manquante." };
  }

  const { generateRegistrationOptions } = await import('@simplewebauthn/server');
  const { isoUint8Array } = await import('@simplewebauthn/server/helpers');

  const session = await getSession();
  if (!session?.user) {
    redirect('/login');
  }

  const [dbUser] = await db.select().from(users).where(eq(users.id, session.user.id));
  const userAuthenticators = await db.select().from(authenticators).where(eq(authenticators.userId, session.user.id));

  const options = await generateRegistrationOptions({
    rpName: RP_NAME,
    rpID: getRpID(),
    userID: isoUint8Array.fromUTF8String(session.user.id.toString()),
    userName: decrypt(dbUser.emailEncrypted),
    attestationType: 'none',
    excludeCredentials: userAuthenticators.map((auth: any) => ({
      id: auth.credentialID,
      transports: auth.transports ? JSON.parse(auth.transports) : undefined,
    })),
    authenticatorSelection: {
      residentKey: 'preferred',
      userVerification: 'preferred'
    },
  });

  (await cookies()).set('passkey_challenge', options.challenge, {
    httpOnly: true,
    secure: true,
    maxAge: 300
  });

  return options;
}

export async function registerPasskeyFinish(response: unknown) {
  const { verifyRegistrationResponse } = await import('@simplewebauthn/server');
  const { isoBase64URL } = await import('@simplewebauthn/server/helpers');

  const session = await getSession();
  if (!session?.user) {
    return { error: "Non connecté" };
  }

  const challenge = (await cookies()).get('passkey_challenge')?.value;
  if (!challenge) {
    return { error: "Session expirée." };
  }

  try {
    const verification = await verifyRegistrationResponse({
      response: response as Parameters<typeof verifyRegistrationResponse>[0]['response'],
      expectedChallenge: challenge,
      expectedOrigin: getOrigin(),
      expectedRPID: getRpID(),
    });

    if (verification.verified && verification.registrationInfo) {
      const { credential } = verification.registrationInfo;
      await db.insert(authenticators).values({
        credentialID: credential.id,
        credentialPublicKey: isoBase64URL.fromBuffer(credential.publicKey),
        counter: 0,
        credentialDeviceType: 'singleDevice',
        credentialBackedUp: false,
        transports: JSON.stringify((response as { response?: { transports?: string[] } })?.response?.transports || []),
        userId: session.user.id,
        name: `Passkey (${new Date().toLocaleDateString('fr-FR')})`
      });

      (await cookies()).delete('passkey_challenge');
      logger.info({ userId: session.user.id }, 'Passkey enregistré');
      revalidatePath('/settings');
      return { success: true };
    }
  } catch (error) {
    logger.error({ err: error }, "Erreur enregistrement Passkey");
    return { error: "Erreur enregistrement." };
  }

  return { error: "Vérification échouée." };
}

export async function loginPasskeyStart() {
  if (!getHost()) {
    return { error: "Passkeys non activés." };
  }

  const { generateAuthenticationOptions } = await import('@simplewebauthn/server');

  const options = await generateAuthenticationOptions({
    rpID: getRpID(),
    userVerification: 'preferred'
  });

  (await cookies()).set('passkey_auth_challenge', options.challenge, {
    httpOnly: true,
    secure: true,
    maxAge: 300
  });

  return options;
}

export async function loginPasskeyFinish(response: { id: string; [key: string]: unknown }) {
  const { verifyAuthenticationResponse } = await import('@simplewebauthn/server');
  const { isoBase64URL } = await import('@simplewebauthn/server/helpers');

  const challenge = (await cookies()).get('passkey_auth_challenge')?.value;
  if (!challenge) {
    return { error: "Session expirée." };
  }

  try {
    const [auth] = await db.select().from(authenticators).where(eq(authenticators.credentialID, response.id));
    if (!auth || !auth.userId) {
      return { error: "Passkey inconnu." };
    }

    const [user] = await db.select().from(users).where(eq(users.id, auth.userId));
    if (!user) {
      return { error: "Utilisateur inconnu." };
    }

    if (user.lockUntil && user.lockUntil > new Date()) {
      return { error: "Compte verrouillé." };
    }

    const verification = await verifyAuthenticationResponse({
      response: response as unknown as Parameters<typeof verifyAuthenticationResponse>[0]['response'],
      expectedChallenge: challenge,
      expectedOrigin: getOrigin(),
      expectedRPID: getRpID(),
      credential: {
        id: auth.credentialID,
        publicKey: isoBase64URL.toBuffer(auth.credentialPublicKey),
        counter: auth.counter,
        transports: auth.transports ? JSON.parse(auth.transports) : undefined,
      }
    });

    if (verification.verified) {
      await db.update(authenticators)
        .set({ counter: verification.authenticationInfo.newCounter })
        .where(eq(authenticators.id, auth.id));

      if ((user.failedLoginAttempts || 0) > 0 || user.lockUntil) {
        await db.update(users)
          .set({ failedLoginAttempts: 0, lockUntil: null })
          .where(eq(users.id, user.id));
      }

      (await cookies()).delete('passkey_auth_challenge');
      await login({
        id: user.id,
        email: decrypt(user.emailEncrypted),
        role: user.role ?? 'USER',
        sessionVersion: user.sessionVersion
      });
      return { success: true };
    }
  } catch (error) {
    logger.error({ err: error }, "Erreur connexion Passkey");
    return { error: "Erreur connexion." };
  }

  return { error: "Connexion refusée." };
}

export async function generateMfaSecretAction() {
  const { authenticator } = await import('@otplib/preset-default');
  const qrcode = (await import('qrcode')).default;

  const session = await getSession();
  if (!session?.user) {
    return null;
  }

  const [dbUser] = await db.select().from(users).where(eq(users.id, session.user.id));
  const secret = authenticator.generateSecret();
  const otpauth = authenticator.keyuri(decrypt(dbUser.emailEncrypted), RP_NAME, secret);
  const imageUrl = await qrcode.toDataURL(otpauth);

  return { secret, imageUrl };
}

export async function enableMfaAction(secret: string, token: string) {
  const { authenticator } = await import('@otplib/preset-default');

  const session = await getSession();
  if (!session?.user) {
    return { error: "Non connecté" };
  }

  if (!authenticator.verify({ token, secret })) {
    return { error: "Code invalide." };
  }

  await db.update(users).set({
    mfaEnabled: true,
    mfaSecret: encrypt(secret),
    sessionVersion: (session.user.sessionVersion || 1) + 1
  }).where(eq(users.id, session.user.id));

  logger.info({ userId: session.user.id }, 'MFA activé');
  revalidatePath('/settings');
  return { success: true };
}

export async function disableMfaAction() {
  const session = await getSession();
  if (!session?.user) {
    return;
  }

  await db.update(users).set({
    mfaEnabled: false,
    mfaSecret: null,
    sessionVersion: (session.user.sessionVersion || 1) + 1
  }).where(eq(users.id, session.user.id));

  logger.info({ userId: session.user.id }, 'MFA désactivé');
  revalidatePath('/settings');
}

export async function getMfaStatus(): Promise<boolean> {
  const session = await getSession();
  if (!session?.user) {
    return false;
  }

  const [dbUser] = await db.select().from(users).where(eq(users.id, session.user.id));
  return dbUser?.mfaEnabled || false;
}

export async function getUserPasskeys() {
  const session = await getSession();
  if (!session?.user) {
    return [];
  }

  return await db.select().from(authenticators).where(eq(authenticators.userId, session.user.id));
}

export async function deletePasskey(id: number) {
  const session = await getSession();
  if (!session?.user) {
    return;
  }

  await db.delete(authenticators).where(
    and(eq(authenticators.id, id), eq(authenticators.userId, session.user.id))
  );

  revalidatePath('/settings');
}

export async function renamePasskey(id: number, newName: string) {
  const session = await getSession();
  if (!session?.user || !newName) {
    return;
  }

  const safeName = newName.trim().slice(0, 50);

  await db.update(authenticators)
    .set({ name: safeName })
    .where(and(eq(authenticators.id, id), eq(authenticators.userId, session.user.id)));

  revalidatePath('/settings');
}
