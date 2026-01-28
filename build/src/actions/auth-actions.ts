'use server'

import { db } from "@/src/db";
import { users, authenticators, accounts, transactions, recurringOperations } from "@/src/schema";
import { eq, and, isNull } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { cookies } from "next/headers";
import { login, getSession, logout as logoutLib } from "@/src/lib/auth";
import { encrypt, decrypt, computeBlindIndex, hashToken } from "@/src/lib/crypto";
import { sendEmail } from "@/src/lib/mail";
import { getHtmlTemplate } from "@/src/lib/email-templates";
import { authSchema, registerSchema } from "@/src/lib/validations";
import logger from "@/src/lib/logger";

const RP_NAME = 'Pilot Finance';
const getHost = () => process.env.HOST || null;
const getRpID = () => getHost() || 'localhost';
const getOrigin = () => `https://${getRpID()}`;
const MAX_LOGIN_ATTEMPTS = 5;
const LOCK_TIME_MS = 15 * 60 * 1000;

export async function authenticate(formData: FormData, isRegister: boolean) {
  const { hash, compare } = await import('bcryptjs');
  const { randomBytes } = await import('crypto');

  const rawData = Object.fromEntries(formData.entries());
  
  // Correction TypeScript: Définition explicite du type
  const validation = isRegister ? registerSchema.safeParse(rawData) : authSchema.safeParse(rawData);

  if (!validation.success) {
      // Correction de l'accès à l'erreur pour TypeScript
      return { error: validation.error.issues[0].message };
  }

  const { email, password, twoFactorCode } = validation.data;
  const emailIndex = computeBlindIndex(email);

  if (isRegister) {
    const allUsers = await db.select().from(users);
    const isFirstUser = allUsers.length === 0;

    if (!isFirstUser && process.env.ALLOW_REGISTER !== 'true') {
        return { error: "Inscriptions fermées." };
    }

    const existing = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (existing.length > 0) return { error: "Email déjà utilisé." };

    const hashedPassword = await hash(password, 10);

    try {
        const mailEnabled = process.env.ENABLE_MAIL === 'true';
        let verificationToken = null;
        let hashedToken = null;

        if (mailEnabled) {
            verificationToken = randomBytes(32).toString('hex');
            hashedToken = hashToken(verificationToken);
        }

        const userId = await db.transaction(async (tx) => {
            const newUser = await tx.insert(users).values({
                emailEncrypted: encrypt(email),
                emailBlindIndex: emailIndex,
                password: hashedPassword,
                role: isFirstUser ? 'ADMIN' : 'USER',
                email_verified: !mailEnabled,
                verification_token: hashedToken,
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

        if (mailEnabled && verificationToken) {
            const verifyUrl = `https://${process.env.HOST}/verify-email?token=${verificationToken}`;
            await sendEmail({
                to: email,
                subject: "Validez votre compte",
                html: getHtmlTemplate("Bienvenue", "Cliquez pour valider votre compte.", "Valider", verifyUrl)
            });
        }

        if (mailEnabled) return { success: true, message: "Vérifiez vos emails." };
        await login({ id: userId, email: email, role: isFirstUser ? 'ADMIN' : 'USER', sessionVersion: 1 });

    } catch (e) {
        logger.error({ err: e }, "Erreur critique lors de l'inscription");
        return { error: "Erreur création compte." };
    }
  } else {
    const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (!user) return { error: "Identifiants incorrects" };

    if (user.lockUntil && user.lockUntil > new Date()) {
        const waitMin = Math.ceil((user.lockUntil.getTime() - Date.now()) / 60000);
        logger.warn({ userId: user.id }, "Tentative de connexion sur compte verrouillé");
        return { error: `Compte verrouillé. Réessayez dans ${waitMin} min.` };
    }

    const match = await compare(password, user.password);
    if (!match) {
        const newFailCount = (user.failedLoginAttempts || 0) + 1;
        let updateData: any = { failedLoginAttempts: newFailCount };
        
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
        if (!twoFactorCode) return { requires2FA: true };
        const decryptedSecret = decrypt(user.mfaSecret!);
        const { authenticator } = await import('@otplib/preset-default');
        if (!authenticator.verify({ token: twoFactorCode, secret: decryptedSecret })) {
            logger.warn({ userId: user.id }, "Echec validation MFA");
            return { error: "Code A2F invalide", requires2FA: true };
        }
    }

    const decryptedEmail = decrypt(user.emailEncrypted);
    await login({ id: user.id, email: decryptedEmail, role: user.role, sessionVersion: user.sessionVersion });
  }
  
  redirect('/');
}

export async function logoutAction() {
    await logoutLib();
    redirect('/login');
}

export async function verifyEmailAction(token: string) {
    const hashedToken = hashToken(token);
    const [user] = await db.select().from(users).where(eq(users.verification_token, hashedToken));
    if (!user) return { error: "Token invalide." };

    await db.update(users).set({ email_verified: true, verification_token: null }).where(eq(users.id, user.id));
    return { success: true };
}

export async function isPasskeyConfigured() { return !!process.env.HOST; }

export async function registerPasskeyStart() {
    if (!getHost()) return { error: "Variable HOST manquante." };
    const { generateRegistrationOptions } = await import('@simplewebauthn/server');
    const { isoUint8Array } = await import('@simplewebauthn/server/helpers');

    const session = await getSession();
    if (!session?.user) redirect('/login');
    const user = session.user;

    const [dbUser] = await db.select().from(users).where(eq(users.id, user.id));
    const userAuthenticators = await db.select().from(authenticators).where(eq(authenticators.userId, user.id));

    const options = await generateRegistrationOptions({
        rpName: RP_NAME, rpID: getRpID(),
        userID: isoUint8Array.fromUTF8String(user.id.toString()),
        userName: decrypt(dbUser.emailEncrypted),
        attestationType: 'none',
        excludeCredentials: userAuthenticators.map(auth => ({
          id: auth.credentialID, transports: auth.transports ? JSON.parse(auth.transports) : undefined,
        })),
        authenticatorSelection: { residentKey: 'preferred', userVerification: 'preferred' },
    });

    (await cookies()).set('passkey_challenge', options.challenge, { httpOnly: true, secure: true, maxAge: 300 });
    return options;
}

export async function registerPasskeyFinish(response: any) {
    const { verifyRegistrationResponse } = await import('@simplewebauthn/server');
    const { isoBase64URL } = await import('@simplewebauthn/server/helpers');
    
    const session = await getSession();
    const user = session?.user;
    if(!user) return { error: "Non connecté" };

    const challenge = (await cookies()).get('passkey_challenge')?.value;
    if (!challenge) return { error: "Session expirée." };

    try {
        const verification = await verifyRegistrationResponse({
            response, expectedChallenge: challenge, expectedOrigin: getOrigin(), expectedRPID: getRpID(),
        });

        if (verification.verified && verification.registrationInfo) {
            const { credential } = verification.registrationInfo;
            await db.insert(authenticators).values({
                credentialID: credential.id,
                credentialPublicKey: isoBase64URL.fromBuffer(credential.publicKey), 
                counter: 0, credentialDeviceType: 'singleDevice', credentialBackedUp: false,
                transports: JSON.stringify(response.response.transports || []),
                userId: user.id, name: `Passkey (${new Date().toLocaleDateString('fr-FR')})`
            });
            (await cookies()).delete('passkey_challenge');
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
    if (!getHost()) return { error: "Passkeys non activés." };
    const { generateAuthenticationOptions } = await import('@simplewebauthn/server');
    const options = await generateAuthenticationOptions({ rpID: getRpID(), userVerification: 'preferred' });
    (await cookies()).set('passkey_auth_challenge', options.challenge, { httpOnly: true, secure: true, maxAge: 300 });
    return options;
}

export async function loginPasskeyFinish(response: any) {
    const { verifyAuthenticationResponse } = await import('@simplewebauthn/server');
    const { isoBase64URL } = await import('@simplewebauthn/server/helpers');
    const challenge = (await cookies()).get('passkey_auth_challenge')?.value;

    if (!challenge) return { error: "Session expirée." };

    try {
        const [auth] = await db.select().from(authenticators).where(eq(authenticators.credentialID, response.id));
        if (!auth || !auth.userId) return { error: "Passkey inconnu." };

        const [user] = await db.select().from(users).where(eq(users.id, auth.userId));
        if (!user) return { error: "Utilisateur inconnu." };

        if (user.lockUntil && user.lockUntil > new Date()) {
             return { error: "Compte verrouillé." };
        }

        const verification = await verifyAuthenticationResponse({
            response, expectedChallenge: challenge, expectedOrigin: getOrigin(), expectedRPID: getRpID(),
            credential: {
                id: auth.credentialID, publicKey: isoBase64URL.toBuffer(auth.credentialPublicKey),
                counter: auth.counter, transports: auth.transports ? JSON.parse(auth.transports) : undefined,
            }
        });

        if (verification.verified) {
            await db.update(authenticators).set({ counter: verification.authenticationInfo.newCounter }).where(eq(authenticators.id, auth.id));
            if ((user.failedLoginAttempts || 0) > 0 || user.lockUntil) {
                await db.update(users).set({ failedLoginAttempts: 0, lockUntil: null }).where(eq(users.id, user.id));
            }
            (await cookies()).delete('passkey_auth_challenge');
            await login({ id: user.id, email: decrypt(user.emailEncrypted), role: user.role, sessionVersion: user.sessionVersion });
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
    if(!session?.user) return; 

    const [dbUser] = await db.select().from(users).where(eq(users.id, session.user.id));
    const secret = authenticator.generateSecret();
    const otpauth = authenticator.keyuri(decrypt(dbUser.emailEncrypted), RP_NAME, secret);
    const imageUrl = await qrcode.toDataURL(otpauth);
    return { secret, imageUrl };
}

export async function enableMfaAction(secret: string, token: string) {
    const { authenticator } = await import('@otplib/preset-default');
    const session = await getSession();
    if(!session?.user) return { error: "Non connecté" };

    if (!authenticator.verify({ token, secret })) return { error: "Code invalide." };

    await db.update(users).set({ mfaEnabled: true, mfaSecret: encrypt(secret), sessionVersion: (session.user.sessionVersion || 1) + 1 }).where(eq(users.id, session.user.id));
    revalidatePath('/settings');
    return { success: true };
}

export async function disableMfaAction() {
    const session = await getSession();
    if(!session?.user) return;
    await db.update(users).set({ mfaEnabled: false, mfaSecret: null, sessionVersion: (session.user.sessionVersion || 1) + 1 }).where(eq(users.id, session.user.id));
    revalidatePath('/settings');
}

export async function getMfaStatus() {
    const session = await getSession();
    if(!session?.user) return false;
    const [dbUser] = await db.select().from(users).where(eq(users.id, session.user.id));
    return dbUser?.mfaEnabled || false;
}

export async function getUserPasskeys() {
    const session = await getSession();
    if(!session?.user) return [];
    return await db.select().from(authenticators).where(eq(authenticators.userId, session.user.id));
}

export async function deletePasskey(id: number) {
    const session = await getSession();
    if(!session?.user) return;
    await db.delete(authenticators).where(and(eq(authenticators.id, id), eq(authenticators.userId, session.user.id)));
    revalidatePath('/settings');
}

export async function renamePasskey(id: number, newName: string) {
    const session = await getSession();
    if(!session?.user || !newName) return;
    await db.update(authenticators).set({ name: newName }).where(and(eq(authenticators.id, id), eq(authenticators.userId, session.user.id)));
    revalidatePath('/settings');
}