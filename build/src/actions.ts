'use server'

import { db } from "@/src/db";
import { accounts, transactions, recurringOperations, users, authenticators } from "@/src/schema";
import { eq, sql, desc, asc, and, isNull, gt } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { headers, cookies } from "next/headers";
import { login, getSession, logout as logoutLib } from "@/src/lib/auth";
import { encrypt, decrypt, computeBlindIndex } from "@/src/lib/crypto";
import { sendEmail } from "@/src/lib/mail";
import { getHtmlTemplate } from "@/src/lib/email-templates";

// NOTE: Tous les imports Node.js et Server-Side (crypto, otplib, simplewebauthn)
// sont désormais dynamiques pour éviter les erreurs "Module not found" côté client.

const roundCurrency = (amount: number) => Math.round((amount + Number.EPSILON) * 100) / 100;

// --- CONFIGURATION ---

const RP_NAME = 'Pilot Finance';

const getHost = () => process.env.HOST || null;

const getRpID = () => {
    const host = getHost();
    return host || 'localhost'; 
};

const getOrigin = () => {
    return `https://${getRpID()}`;
};

export async function isPasskeyConfigured() {
    return !!process.env.HOST;
}

// --- UTILITAIRES DE SÉCURITÉ ---

function isValidEmail(email: string): boolean {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
}

function validatePasswordStrength(password: string) {
    if (password.length < 8) return { valid: false, error: "Le mot de passe doit faire au moins 8 caractères." };
    if (!/[A-Z]/.test(password)) return { valid: false, error: "Le mot de passe doit contenir une majuscule." };
    if (!/[a-z]/.test(password)) return { valid: false, error: "Le mot de passe doit contenir une minuscule." };
    if (!/[0-9]/.test(password)) return { valid: false, error: "Le mot de passe doit contenir un chiffre." };
    if (!/[!@#$%^&*(),.?":{}|<>]/.test(password)) return { valid: false, error: "Le mot de passe doit contenir un caractère spécial." };
    return { valid: true, error: null };
}

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

// --- AUTHENTIFICATION PRINCIPALE ---

export async function authenticate(formData: FormData, isRegister: boolean) {
  // Imports dynamiques
  const { hash, compare } = await import('bcryptjs');
  const { randomBytes } = await import('crypto');

  const email = formData.get('email') as string;
  const password = formData.get('password') as string;
  const twoFactorCode = formData.get('twoFactorCode') as string;

  if (!email || !password) return { error: "Champs requis" };
  if (!isValidEmail(email)) return { error: "Format d'adresse email invalide." };

  const emailIndex = computeBlindIndex(email);

  if (isRegister) {
    const confirmPassword = formData.get('confirmPassword') as string;
    if (password !== confirmPassword) return { error: "Les mots de passe ne correspondent pas." };

    const pwdCheck = validatePasswordStrength(password);
    if (!pwdCheck.valid) return { error: pwdCheck.error };

    const allUsers = await db.select().from(users);
    const isFirstUser = allUsers.length === 0;

    if (!isFirstUser) {
        const allow = process.env.ALLOW_REGISTER === 'true';
        if (!allow) return { error: "Les inscriptions sont fermées." };
    }

    const existing = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (existing.length > 0) return { error: "Cet email est déjà utilisé." };

    const hashedPassword = await hash(password, 10);

    try {
        const newUser = await db.insert(users).values({
            emailEncrypted: encrypt(email),
            emailBlindIndex: emailIndex,
            password: hashedPassword,
            role: isFirstUser ? 'ADMIN' : 'USER'
        }).returning();

        const userId = newUser[0].id;

        const mailEnabled = process.env.ENABLE_MAIL === 'true';
        if (mailEnabled) {
            const verificationToken = randomBytes(32).toString('hex');
            await db.update(users)
                .set({ email_verified: false, verification_token: verificationToken })
                .where(eq(users.id, userId));
            
            const verifyUrl = `https://${process.env.HOST}/verify-email?token=${verificationToken}`;
            
            await sendEmail({
                to: email,
                subject: "Validez votre compte Pilot Finance",
                html: getHtmlTemplate(
                    "Confirmez votre adresse email",
                    "Merci de vous être inscrit sur Pilot Finance. Pour activer votre compte et sécuriser votre accès, veuillez cliquer sur le bouton ci-dessous.",
                    "Valider mon compte",
                    verifyUrl
                )
            });
        }

        if (isFirstUser) {
            await db.update(accounts).set({ userId }).where(isNull(accounts.userId));
            await db.update(transactions).set({ userId }).where(isNull(transactions.userId));
            await db.update(recurringOperations).set({ userId }).where(isNull(recurringOperations.userId));
        }

        if (mailEnabled) {
            return { success: true, message: "Compte créé. Veuillez valider votre e-mail." };
        }

        await login({ id: userId, email: email, role: newUser[0].role });

    } catch (e) {
        console.error(e);
        return { error: "Erreur technique lors de la création." };
    }
  } else {
    // LOGIN
    const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));
    if (!user) return { error: "Identifiants incorrects" };

    const match = await compare(password, user.password);
    if (!match) return { error: "Identifiants incorrects" };

    if (process.env.ENABLE_MAIL === 'true' && !user.email_verified) {
        return { error: "Veuillez valider votre adresse email avant de vous connecter." };
    }

    // Vérification 2FA (TOTP)
    if (user.mfaEnabled) {
        if (!twoFactorCode) {
            return { requires2FA: true };
        }
        const decryptedSecret = decrypt(user.mfaSecret!);
        
        // Import dynamique otplib
        const { authenticator } = await import('@otplib/preset-default');
        
        const isValid = authenticator.verify({ token: twoFactorCode, secret: decryptedSecret });
        
        if (!isValid) {
            return { error: "Code A2F invalide", requires2FA: true };
        }
    }

    const decryptedEmail = decrypt(user.emailEncrypted);
    await login({ id: user.id, email: decryptedEmail, role: user.role });
  }
  
  redirect('/');
}

export async function logoutAction() {
    await logoutLib();
    redirect('/login');
}

// --- EMAIL & RECUPERATION ---

export async function verifyEmailAction(token: string) {
    const [user] = await db.select().from(users).where(eq(users.verification_token, token));

    if (!user) {
        return { error: "Lien de validation invalide ou expiré." };
    }

    await db.update(users)
        .set({ email_verified: true, verification_token: null })
        .where(eq(users.id, user.id));

    return { success: true };
}

// --- PASSKEYS (GESTION COMPLÈTE) ---

export async function getUserPasskeys() {
    const user = await getUser();
    return await db.select().from(authenticators).where(eq(authenticators.userId, user.id));
}

export async function deletePasskey(id: number) {
    const user = await getUser();
    await db.delete(authenticators).where(and(eq(authenticators.id, id), eq(authenticators.userId, user.id)));
    revalidatePath('/settings');
}

export async function renamePasskey(id: number, newName: string) {
    const user = await getUser();
    if (!newName) return;
    await db.update(authenticators).set({ name: newName }).where(and(eq(authenticators.id, id), eq(authenticators.userId, user.id)));
    revalidatePath('/settings');
}

export async function registerPasskeyStart() {
    if (!getHost()) return { error: "Passkeys non activés sur ce serveur (Variable HOST manquante)." };

    // Imports dynamiques SimpleWebAuthn
    const { generateRegistrationOptions } = await import('@simplewebauthn/server');
    const { isoUint8Array } = await import('@simplewebauthn/server/helpers');

    const user = await getUser();
    const [dbUser] = await db.select().from(users).where(eq(users.id, user.id));
    const userEmail = decrypt(dbUser.emailEncrypted);
    const userAuthenticators = await db.select().from(authenticators).where(eq(authenticators.userId, user.id));

    const options = await generateRegistrationOptions({
        rpName: RP_NAME,
        rpID: getRpID(),
        userID: isoUint8Array.fromUTF8String(user.id.toString()),
        userName: userEmail,
        attestationType: 'none',
        excludeCredentials: userAuthenticators.map(auth => ({
          id: auth.credentialID,
          transports: auth.transports ? JSON.parse(auth.transports) : undefined,
        })),
        authenticatorSelection: {
            residentKey: 'preferred',
            userVerification: 'preferred',
        },
    });

    (await cookies()).set('passkey_challenge', options.challenge, { httpOnly: true, secure: true, maxAge: 60 * 5 });

    return options;
}

export async function registerPasskeyFinish(response: any) {
    // Imports dynamiques SimpleWebAuthn
    const { verifyRegistrationResponse } = await import('@simplewebauthn/server');
    const { isoBase64URL } = await import('@simplewebauthn/server/helpers');

    const user = await getUser();
    const challenge = (await cookies()).get('passkey_challenge')?.value;

    if (!challenge) return { error: "Session expirée, réessayez." };

    try {
        const verification = await verifyRegistrationResponse({
            response,
            expectedChallenge: challenge,
            expectedOrigin: getOrigin(),
            expectedRPID: getRpID(),
        });

        if (verification.verified && verification.registrationInfo) {
            const { credential } = verification.registrationInfo;
            const defaultName = `Clé Passkey (${new Date().toLocaleDateString('fr-FR')})`;

            await db.insert(authenticators).values({
                credentialID: credential.id,
                credentialPublicKey: isoBase64URL.fromBuffer(credential.publicKey), 
                counter: 0,
                credentialDeviceType: 'singleDevice',
                credentialBackedUp: false,
                transports: JSON.stringify(response.response.transports || []),
                userId: user.id,
                name: defaultName
            });

            (await cookies()).delete('passkey_challenge');
            revalidatePath('/settings');
            return { success: true };
        }
    } catch (error) {
        console.error(error);
        return { error: "Échec de l'enregistrement." };
    }
    return { error: "Vérification échouée." };
}

export async function loginPasskeyStart() {
    if (!getHost()) return { error: "Passkeys non activés." };

    // Import dynamique SimpleWebAuthn
    const { generateAuthenticationOptions } = await import('@simplewebauthn/server');

    const options = await generateAuthenticationOptions({
        rpID: getRpID(),
        userVerification: 'preferred',
    });

    (await cookies()).set('passkey_auth_challenge', options.challenge, { httpOnly: true, secure: true, maxAge: 60 * 5 });
    return options;
}

export async function loginPasskeyFinish(response: any) {
    // Imports dynamiques SimpleWebAuthn
    const { verifyAuthenticationResponse } = await import('@simplewebauthn/server');
    const { isoBase64URL } = await import('@simplewebauthn/server/helpers');

    const challenge = (await cookies()).get('passkey_auth_challenge')?.value;

    if (!challenge) return { error: "Session expirée." };

    try {
        const credentialID = response.id;
        const [auth] = await db.select().from(authenticators).where(eq(authenticators.credentialID, credentialID));

        if (!auth || !auth.userId) return { error: "Passkey inconnu." };

        const [user] = await db.select().from(users).where(eq(users.id, auth.userId));
        if (!user) return { error: "Utilisateur introuvable." };

        const verification = await verifyAuthenticationResponse({
            response,
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

            (await cookies()).delete('passkey_auth_challenge');

            const decryptedEmail = decrypt(user.emailEncrypted);
            await login({ id: user.id, email: decryptedEmail, role: user.role });
            return { success: true };
        }
    } catch (error) {
        console.error(error);
        return { error: "Échec de la connexion." };
    }
    return { error: "Connexion refusée." };
}

// --- DOUBLE AUTHENTIFICATION (TOTP) ---

export async function generateMfaSecretAction() {
    // Import dynamique pour crypto/qrcode
    const { authenticator } = await import('@otplib/preset-default');
    const qrcode = (await import('qrcode')).default;
    
    const user = await getUser();
    const [dbUser] = await db.select().from(users).where(eq(users.id, user.id));
    const userEmail = decrypt(dbUser.emailEncrypted);
    const secret = authenticator.generateSecret();
    const otpauth = authenticator.keyuri(userEmail, RP_NAME, secret);
    const imageUrl = await qrcode.toDataURL(otpauth);
    return { secret, imageUrl };
}

export async function enableMfaAction(secret: string, token: string) {
    const { authenticator } = await import('@otplib/preset-default');
    
    const userSession = await getUser();
    const isValid = authenticator.verify({ token, secret });
    
    if (!isValid) return { error: "Code invalide. Réessayez." };

    await db.update(users).set({ mfaEnabled: true, mfaSecret: encrypt(secret) }).where(eq(users.id, userSession.id));
    revalidatePath('/settings');
    return { success: true };
}

export async function disableMfaAction() {
    const userSession = await getUser();
    await db.update(users).set({ mfaEnabled: false, mfaSecret: null }).where(eq(users.id, userSession.id));
    revalidatePath('/settings');
}

export async function getMfaStatus() {
    const userSession = await getUser();
    const [dbUser] = await db.select().from(users).where(eq(users.id, userSession.id));
    return dbUser?.mfaEnabled || false;
}

// --- GESTION DES MOTS DE PASSE ---

export async function forgotPasswordAction(formData: FormData) {
    // Import dynamique crypto
    const { randomBytes } = await import('crypto');

    const email = formData.get('email') as string;
    if (!email || !isValidEmail(email)) return { error: "Email invalide" };

    const emailIndex = computeBlindIndex(email);
    const [user] = await db.select().from(users).where(eq(users.emailBlindIndex, emailIndex));

    if (!user) return { success: true }; 

    const token = randomBytes(32).toString('hex');
    const expiry = new Date(Date.now() + 3600000);

    await db.update(users).set({ 
        resetToken: token, 
        resetTokenExpiry: expiry 
    }).where(eq(users.id, user.id));

    const resetUrl = `https://${process.env.HOST}/reset-password?token=${token}`;
    
    await sendEmail({
        to: email,
        subject: "Réinitialisation de votre mot de passe",
        html: getHtmlTemplate(
            "Mot de passe oublié ?",
            "Vous avez demandé la réinitialisation de votre mot de passe Pilot Finance. Si vous n'êtes pas à l'origine de cette demande, ignorez cet e-mail.",
            "Réinitialiser le mot de passe",
            resetUrl
        )
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

    const [user] = await db.select().from(users).where(and(
        eq(users.resetToken, token),
        gt(users.resetTokenExpiry, new Date())
    ));

    if (!user) return { error: "Lien invalide ou expiré." };

    const hashedPassword = await hash(password, 10);

    await db.update(users).set({
        password: hashedPassword,
        resetToken: null,
        resetTokenExpiry: null
    }).where(eq(users.id, user.id));

    redirect('/login?reset=success');
}

export async function updatePasswordAction(formData: FormData) {
    const { hash, compare } = await import('bcryptjs');

    const userSession = await getUser();
    const currentPassword = formData.get("currentPassword") as string;
    const newPassword = formData.get("newPassword") as string;
    const confirmPassword = formData.get("confirmPassword") as string;

    if (!currentPassword || !newPassword || !confirmPassword) return { error: "Tous les champs sont requis" };
    if (newPassword !== confirmPassword) {
        return { error: "Les nouveaux mots de passe ne correspondent pas" };
    }

    const pwdCheck = validatePasswordStrength(newPassword);
    if (!pwdCheck.valid) return { error: pwdCheck.error };

    const [dbUser] = await db.select().from(users).where(eq(users.id, userSession.id));
    if (!dbUser) return { error: "Utilisateur introuvable" };

    const match = await compare(currentPassword, dbUser.password);
    if (!match) return { error: "Le mot de passe actuel est incorrect" };

    const hashedPassword = await hash(newPassword, 10);

    await db.update(users).set({ password: hashedPassword }).where(eq(users.id, userSession.id));

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

// --- DASHBOARD & CALCULS ---
// (Le reste reste inchangé)

export async function getDashboardData(projectionYears: number = 5) {
  const user = await getUser();
  await checkRecurringOperations(user.id);
  await processRealYields(user.id);

  const rawAccounts = await db.select().from(accounts)
    .where(eq(accounts.userId, user.id))
    .orderBy(asc(accounts.position));
    
  const allAccounts = rawAccounts.map(a => ({ ...a, name: decrypt(a.name) }));

  const rawRecurrings = await db.select().from(recurringOperations)
    .where(and(eq(recurringOperations.isActive, true), eq(recurringOperations.userId, user.id)))
    .orderBy(asc(recurringOperations.dayOfMonth));
    
  const recurrings = rawRecurrings.map(r => ({ ...r, description: decrypt(r.description) }));

  const { dataPoints, totalInterests } = simulateProjection(allAccounts, recurrings, projectionYears);

  return { accounts: allAccounts, projection: dataPoints, totalInterests, user };
}

function simulateProjection(initialAccounts: any[], recurrings: any[], years: number) {
  let accountsAvg = initialAccounts.map(a => ({ ...a }));
  let accountsMin = initialAccounts.map(a => ({ ...a }));
  let accountsMax = initialAccounts.map(a => ({ ...a }));
  
  const dataPoints = [];
  const now = new Date();
  let totalInterests = 0;
  const showMonthly = years < 3;
  const totalMonths = years * 12;

  let startName = showMonthly ? now.toLocaleDateString('fr-FR', { month: 'short', year: '2-digit' }) : now.getFullYear().toString();
  const startTotal = Math.round(initialAccounts.reduce((sum, a) => sum + a.balance, 0));
  const startPoint: any = { name: startName, totalMin: startTotal, totalMax: startTotal, totalAvg: startTotal };
  initialAccounts.forEach(acc => { startPoint[acc.name] = Math.round(acc.balance); });
  dataPoints.push(startPoint);

  for (let m = 1; m <= totalMonths; m++) {
    const isYearEnd = m % 12 === 0;

    recurrings.forEach(op => {
      [accountsAvg, accountsMin, accountsMax].forEach(universe => {
         const source = universe.find(a => a.id === op.accountId);
         if (op.toAccountId) {
             const target = universe.find(a => a.id === op.toAccountId);
             if (source && target) {
                 source.balance = roundCurrency(source.balance - Math.abs(op.amount));
                 target.balance = roundCurrency(target.balance + Math.abs(op.amount));
             }
         } else {
             if (source) source.balance = roundCurrency(source.balance + op.amount);
         }
      });
    });

    const applyYield = (universe: any[], mode: 'MIN' | 'MAX' | 'AVG') => {
       universe.forEach(acc => {
          if (!acc.isYieldActive) return;
          const isPayoutDue = (acc.payoutFrequency === 'MONTHLY') || (acc.payoutFrequency === 'YEARLY' && isYearEnd);

          if (isPayoutDue) {
             const yieldMin = acc.yieldMin ?? 0;
             const yieldMax = acc.yieldMax ?? 0;
             const reinvestmentRate = acc.reinvestmentRate ?? 0;

             let rate = 0;
             if (mode === 'MIN') rate = acc.yieldType === 'RANGE' ? yieldMin : yieldMin;
             if (mode === 'MAX') rate = acc.yieldType === 'RANGE' ? yieldMax : yieldMin;
             if (mode === 'AVG') rate = acc.yieldType === 'RANGE' ? (yieldMin + yieldMax) / 2 : yieldMin;

             if (acc.yieldFrequency === 'YEARLY' && acc.payoutFrequency === 'MONTHLY') rate = rate / 12;
             if (acc.yieldFrequency === 'MONTHLY' && acc.payoutFrequency === 'YEARLY') rate = rate * 12;

             const gain = roundCurrency(acc.balance * (rate / 100));
             if(mode === 'AVG') totalInterests += gain;
             
             const reinvest = roundCurrency(gain * (reinvestmentRate / 100));
             const payout = roundCurrency(gain - reinvest);

             acc.balance = roundCurrency(acc.balance + reinvest);

             if (payout > 0) {
               const targetId = acc.targetAccountId || acc.id;
               const targetAcc = universe.find(a => a.id === targetId);
               if (targetAcc) targetAcc.balance = roundCurrency(targetAcc.balance + payout);
             }
          }
       });
    };

    applyYield(accountsAvg, 'AVG');
    applyYield(accountsMin, 'MIN');
    applyYield(accountsMax, 'MAX');

    if (showMonthly || isYearEnd) {
        const futureDate = new Date(now.getFullYear(), now.getMonth() + m, 1);
        let name = showMonthly ? futureDate.toLocaleDateString('fr-FR', { month: 'short', year: '2-digit' }) : futureDate.getFullYear().toString();
        
        const point: any = { 
            name: name, 
            totalMin: Math.round(accountsMin.reduce((sum, a) => sum + a.balance, 0)), 
            totalMax: Math.round(accountsMax.reduce((sum, a) => sum + a.balance, 0)), 
            totalAvg: Math.round(accountsAvg.reduce((sum, a) => sum + a.balance, 0)) 
        };
        
        accountsAvg.forEach(acc => { point[acc.name] = Math.round(acc.balance); });
        dataPoints.push(point);
    }
  }
  return { dataPoints, totalInterests };
}

async function checkRecurringOperations(userId: number) {
  const today = new Date();
  const currentDay = today.getDate();
  const currentMonth = today.getMonth();
  const currentYear = today.getFullYear();

  const rawOps = await db.select().from(recurringOperations).where(and(eq(recurringOperations.isActive, true), eq(recurringOperations.userId, userId)));

  for (const opRaw of rawOps) {
    const op = { ...opRaw, description: decrypt(opRaw.description) };
    const lastRun = op.lastRunDate ? new Date(op.lastRunDate) : null;
    const isDue = currentDay >= op.dayOfMonth;
    const alreadyRunThisMonth = lastRun && lastRun.getMonth() === currentMonth && lastRun.getFullYear() === currentYear;

    if (isDue && !alreadyRunThisMonth) {
      if (op.toAccountId) {
          const [rawSource] = await db.select().from(accounts).where(and(eq(accounts.id, op.accountId), eq(accounts.userId, userId)));
          const [rawTarget] = await db.select().from(accounts).where(and(eq(accounts.id, op.toAccountId), eq(accounts.userId, userId)));
          
          if (rawSource && rawTarget) {
              const source = { ...rawSource, name: decrypt(rawSource.name) };
              const target = { ...rawTarget, name: decrypt(rawTarget.name) };

              const amount = Math.abs(op.amount);
              await db.update(accounts).set({ balance: roundCurrency(source.balance - amount) }).where(eq(accounts.id, source.id));
              await db.insert(transactions).values({ userId, accountId: source.id, amount: -amount, description: encrypt(`[Auto] Virement vers ${target.name} : ${op.description}`), category: 'Virement Sortant', date: new Date() });
              
              await db.update(accounts).set({ balance: roundCurrency(target.balance + amount) }).where(eq(accounts.id, target.id));
              await db.insert(transactions).values({ userId, accountId: target.id, amount: amount, description: encrypt(`[Auto] Virement de ${source.name} : ${op.description}`), category: 'Virement Entrant', date: new Date() });
          }
      } else {
          await db.insert(transactions).values({ userId, accountId: op.accountId, amount: op.amount, description: encrypt(`[Auto] ${op.description}`), category: 'Récurrent', date: new Date() });
          const [account] = await db.select().from(accounts).where(and(eq(accounts.id, op.accountId), eq(accounts.userId, userId)));
          if (account) {
              await db.update(accounts).set({ balance: roundCurrency(account.balance + op.amount) }).where(eq(accounts.id, op.accountId));
          }
      }
      await db.update(recurringOperations).set({ lastRunDate: new Date() }).where(eq(recurringOperations.id, op.id));
    }
  }
}

async function processRealYields(userId: number) {
  const today = new Date();
  const currentMonth = today.getMonth();
  const currentYear = today.getFullYear(); 
  
  const rawActiveAccounts = await db.select().from(accounts).where(and(eq(accounts.isYieldActive, true), eq(accounts.userId, userId)));
  
  for (const accRaw of rawActiveAccounts) { 
    const acc = { ...accRaw, name: decrypt(accRaw.name) };
    const lastRun = acc.lastYieldDate ? new Date(acc.lastYieldDate) : null; 
    
    let shouldRun = false;

    if (acc.payoutFrequency === 'MONTHLY') { 
        const processedThisMonth = lastRun && lastRun.getMonth() === currentMonth && lastRun.getFullYear() === currentYear;
        if (!processedThisMonth) shouldRun = true; 
    } else if (acc.payoutFrequency === 'YEARLY') { 
        const processedThisYear = lastRun && lastRun.getFullYear() === currentYear;
        if (!processedThisYear && currentMonth === 11 && today.getDate() === 31) shouldRun = true;
    } 
    
    if (shouldRun) {
        const yieldMin = acc.yieldMin ?? 0;
        const yieldMax = acc.yieldMax ?? 0;
        const reinvestmentRate = acc.reinvestmentRate ?? 0;

        let rate = acc.yieldType === 'RANGE' ? (yieldMin + yieldMax) / 2 : yieldMin;
        
        if (acc.yieldFrequency === 'YEARLY' && acc.payoutFrequency === 'MONTHLY') rate = rate / 12;
        if (acc.yieldFrequency === 'MONTHLY' && acc.payoutFrequency === 'YEARLY') rate = rate * 12;

        const gain = roundCurrency(acc.balance * (rate / 100)); 
        const reinvestAmount = roundCurrency(gain * (reinvestmentRate / 100));
        const payoutAmount = roundCurrency(gain - reinvestAmount); 
        
        await db.update(accounts).set({ balance: roundCurrency(acc.balance + gain), lastYieldDate: new Date() }).where(eq(accounts.id, acc.id));
        await db.insert(transactions).values({ userId, accountId: acc.id, amount: gain, description: encrypt("Intérêts / Dividendes"), category: "Rendement", date: new Date() });
        
        if (payoutAmount > 0) { 
            const targetId = acc.targetAccountId || acc.id;
            const targetIdSafe = targetId || acc.id; 
            
            if (targetIdSafe !== acc.id) { 
                await db.update(accounts).set({ balance: sql`ROUND(balance - ${payoutAmount}, 2)` }).where(eq(accounts.id, acc.id));
                await db.insert(transactions).values({ userId, accountId: acc.id, amount: -payoutAmount, description: encrypt(`Virement intérêts vers cpt ${targetIdSafe}`), category: "Virement Sortant", date: new Date() });
                
                await db.update(accounts).set({ balance: sql`ROUND(balance + ${payoutAmount}, 2)` }).where(eq(accounts.id, targetIdSafe)); 
                await db.insert(transactions).values({ userId, accountId: targetIdSafe, amount: payoutAmount, description: encrypt(`Reçu intérêts de ${acc.name}`), category: "Rendement", date: new Date() });
            } 
        } 
    } 
  }
}

// --- CRUD COMPTES ---

export async function createAccount(formData: FormData) {
  const user = await getUser();
  const name = formData.get("name") as string; 
  const balance = parseFloat(formData.get("balance") as string) || 0;
  const color = formData.get("color") as string || "#3b82f6";
  const isYieldActive = formData.get("isYieldActive") === "on";
  const yieldType = formData.get("yieldType") as string || "FIXED";
  const yieldMin = parseFloat(formData.get("yieldMin") as string) || 0;
  const yieldMax = parseFloat(formData.get("yieldMax") as string) || 0;
  const yieldFrequency = formData.get("yieldFrequency") as string || "YEARLY";
  const payoutFrequency = formData.get("payoutFrequency") as string || "MONTHLY"; 
  const reinvestRaw = formData.get("reinvestmentRate");
  const reinvestmentRate = (reinvestRaw !== null && reinvestRaw !== '') ? parseInt(reinvestRaw as string) : 100;
  const targetAccountId = formData.get("targetAccountId") ? parseInt(formData.get("targetAccountId") as string) : null;
  
  const all = await db.select().from(accounts).where(eq(accounts.userId, user.id));
  const maxPos = all.reduce((max, c) => Math.max(max, c.position || 0), 0);
  
  if (!name) return;

  await db.insert(accounts).values({ 
      userId: user.id, 
      name: encrypt(name), 
      balance: roundCurrency(balance), color, isYieldActive, yieldType, yieldMin, yieldMax, yieldFrequency, payoutFrequency, reinvestmentRate, targetAccountId, position: maxPos + 1 
  });
  revalidatePath("/"); revalidatePath("/accounts");
}

export async function updateAccountFull(formData: FormData) {
  const user = await getUser();
  const id = parseInt(formData.get("id") as string);
  if (!id) return; 
  
  const name = formData.get("name") as string;
  const balance = parseFloat(formData.get("balance") as string) || 0;
  const color = formData.get("color") as string || "#3b82f6";
  const isYieldActive = formData.get("isYieldActive") === "on";
  const yieldType = formData.get("yieldType") as string || "FIXED";
  const yieldMin = parseFloat(formData.get("yieldMin") as string) || 0;
  const yieldMax = parseFloat(formData.get("yieldMax") as string) || 0;
  const yieldFrequency = formData.get("yieldFrequency") as string || "YEARLY";
  const payoutFrequency = formData.get("payoutFrequency") as string || "YEARLY"; 
  const reinvestRaw = formData.get("reinvestmentRate");
  const reinvestmentRate = (reinvestRaw !== null && reinvestRaw !== '') ? parseInt(reinvestRaw as string) : 100;
  const targetAccountId = formData.get("targetAccountId") ? parseInt(formData.get("targetAccountId") as string) : null;

  await db.update(accounts).set({ 
      name: encrypt(name),
      balance: roundCurrency(balance), color, isYieldActive, yieldType, yieldMin, yieldMax, yieldFrequency, payoutFrequency, reinvestmentRate, targetAccountId, updatedAt: new Date() 
  }).where(and(eq(accounts.id, id), eq(accounts.userId, user.id)));
  
  revalidatePath("/"); revalidatePath("/accounts");
}

export async function updateBalanceDirect(formData: FormData) { 
    const user = await getUser();
    const id = parseInt(formData.get("id") as string);
    const newBalance = parseFloat(formData.get("balance") as string); 
    
    if (!id || isNaN(newBalance)) return;
    
    await db.update(accounts).set({ balance: roundCurrency(newBalance) }).where(and(eq(accounts.id, id), eq(accounts.userId, user.id))); 
    revalidatePath("/"); revalidatePath("/accounts");
}

export async function deleteAccount(id: number) { 
    const user = await getUser();
    await db.delete(accounts).where(and(eq(accounts.id, id), eq(accounts.userId, user.id)));
    revalidatePath("/accounts"); 
}

export async function swapAccounts(id1: number, id2: number) {
    const user = await getUser();
    const allAccounts = await db.select().from(accounts).where(eq(accounts.userId, user.id)).orderBy(asc(accounts.position), asc(accounts.id));
    const reordered = allAccounts.map((acc, index) => ({ ...acc, tempPosition: index }));
    
    const idx1 = reordered.findIndex(a => a.id === id1);
    const idx2 = reordered.findIndex(a => a.id === id2);
    
    if (idx1 !== -1 && idx2 !== -1) {
        const temp = reordered[idx1].tempPosition;
        reordered[idx1].tempPosition = reordered[idx2].tempPosition; 
        reordered[idx2].tempPosition = temp;
        
        for (const acc of reordered) {
             await db.update(accounts).set({ position: acc.tempPosition }).where(and(eq(accounts.id, acc.id), eq(accounts.userId, user.id)));
        }
    }
    revalidatePath("/accounts"); revalidatePath("/");
}

// --- CRUD OPERATIONS ---

export async function createRecurring(formData: FormData) {
    const user = await getUser();
    const accountId = parseInt(formData.get("accountId") as string);
    const toAccountId = formData.get("toAccountId") ? parseInt(formData.get("toAccountId") as string) : null;
    const amountRaw = parseFloat(formData.get("amount") as string);
    const type = formData.get("type") as string; 
    const description = formData.get("description") as string;
    const dayOfMonth = parseInt(formData.get("dayOfMonth") as string);
    
    if(!accountId) return;
    
    let amount = 0;
    if (type === 'transfer') amount = Math.abs(amountRaw);
    else if (type === 'expense') amount = -Math.abs(amountRaw);
    else amount = Math.abs(amountRaw);

    await db.insert(recurringOperations).values({ userId: user.id, accountId, toAccountId: type === 'transfer' ? toAccountId : null, amount: roundCurrency(amount), description: encrypt(description), dayOfMonth, isActive: true });
    revalidatePath("/accounts");
}

export async function updateRecurring(formData: FormData) {
    const user = await getUser();
    const id = parseInt(formData.get("id") as string);
    const accountId = parseInt(formData.get("accountId") as string);
    const toAccountId = formData.get("toAccountId") ? parseInt(formData.get("toAccountId") as string) : null;
    const amountRaw = parseFloat(formData.get("amount") as string);
    const type = formData.get("type") as string;
    const description = formData.get("description") as string; 
    const dayOfMonth = parseInt(formData.get("dayOfMonth") as string);
    
    if(!id || !accountId) return;
    
    let amount = 0;
    if (type === 'transfer') amount = Math.abs(amountRaw);
    else if (type === 'expense') amount = -Math.abs(amountRaw);
    else amount = Math.abs(amountRaw);
    
    await db.update(recurringOperations).set({ accountId, toAccountId: type === 'transfer' ? toAccountId : null, amount: roundCurrency(amount), description: encrypt(description), dayOfMonth, isActive: true }).where(and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id)));
    revalidatePath("/accounts");
}

export async function deleteRecurring(id: number) { 
    const user = await getUser();
    await db.delete(recurringOperations).where(and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id))); 
    revalidatePath("/accounts"); 
}

export async function getAccounts() { 
    const user = await getUser();
    const rawAccounts = await db.select().from(accounts).where(eq(accounts.userId, user.id)).orderBy(asc(accounts.position));
    return rawAccounts.map(a => ({ ...a, name: decrypt(a.name) }));
}

export async function getRecurringOperations() {
    const user = await getUser();
    const ops = await db.select().from(recurringOperations).where(and(eq(recurringOperations.isActive, true), eq(recurringOperations.userId, user.id))).orderBy(desc(recurringOperations.id));
    const rawAccounts = await db.select().from(accounts).where(eq(accounts.userId, user.id));
    
    return ops.map(op => {
        const source = rawAccounts.find(a => a.id === op.accountId);
        const target = op.toAccountId ? rawAccounts.find(a => a.id === op.toAccountId) : null;
        return { 
            ...op, 
            description: decrypt(op.description), 
            accountName: source ? decrypt(source.name) : 'Compte supprimé', 
            toAccountName: target ? decrypt(target.name) : null 
        };
    });
}

export async function getMailStatus() {
  return process.env.ENABLE_MAIL === 'true';
}