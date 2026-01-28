import { z } from 'zod';

export const emailSchema = z
  .string()
  .min(1, "Email requis")
  .email("Email invalide")
  .max(255, "Email trop long");

export const passwordSchema = z
  .string()
  .min(8, "8 caractères minimum")
  .regex(/[A-Z]/, "1 majuscule requise")
  .regex(/[a-z]/, "1 minuscule requise")
  .regex(/[0-9]/, "1 chiffre requis")
  .regex(/[!@#$%^&*(),.?":{}|<>]/, "1 caractère spécial requis");

export const authSchema = z.object({
  email: emailSchema,
  password: z.string().min(1, "Mot de passe requis"),
  twoFactorCode: z.string().optional(),
});

export const registerSchema = z.object({
  email: emailSchema,
  password: passwordSchema,
  confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
  message: "Les mots de passe ne correspondent pas",
  path: ["confirmPassword"],
});

export const changePasswordSchema = z.object({
  currentPassword: z.string().min(1, "Mot de passe actuel requis"),
  newPassword: passwordSchema,
  confirmPassword: z.string(),
}).refine((data) => data.newPassword === data.confirmPassword, {
  message: "Les mots de passe ne correspondent pas",
  path: ["confirmPassword"],
});

export const resetPasswordSchema = z.object({
  token: z.string().min(1, "Token requis"),
  password: passwordSchema,
});

export const forgotPasswordSchema = z.object({
  email: emailSchema,
});

export const accountSchema = z.object({
  name: z.string().min(1, "Nom requis").max(100, "Nom trop long"),
  balance: z.coerce.number(),
  color: z.string().regex(/^#[0-9A-Fa-f]{6}$/, "Couleur invalide"),
  isYieldActive: z.coerce.boolean().optional(),
  yieldType: z.enum(["FIXED", "RANGE"]).optional(),
  yieldMin: z.coerce.number().min(0).max(100).optional(),
  yieldMax: z.coerce.number().min(0).max(100).optional(),
  yieldFrequency: z.enum(["DAILY", "MONTHLY", "QUARTERLY", "YEARLY"]).optional(),
  payoutFrequency: z.enum(["DAILY", "MONTHLY", "QUARTERLY", "YEARLY"]).optional(),
  reinvestmentRate: z.coerce.number().min(0).max(100).optional(),
  targetAccountId: z.coerce.number().positive().optional().nullable(),
});

export const transactionSchema = z.object({
  accountId: z.coerce.number().positive("Compte requis"),
  amount: z.coerce.number().refine((val) => val !== 0, "Montant ne peut pas être 0"),
  description: z.string().min(1, "Description requise").max(255, "Description trop longue"),
  date: z.string().refine((val) => !isNaN(Date.parse(val)), "Date invalide"),
});

export const recurringSchema = z.object({
  accountId: z.coerce.number().positive("Compte source requis"),
  toAccountId: z.coerce.number().positive().optional().nullable(),
  amount: z.coerce.number().refine((val) => val !== 0, "Montant ne peut pas être 0"),
  description: z.string().min(1, "Description requise").max(255, "Description trop longue"),
  dayOfMonth: z.coerce.number().min(1, "Jour minimum: 1").max(31, "Jour maximum: 31"),
  type: z.enum(["income", "expense", "transfer"]).optional(),
});

export function validatePassword(password: string): { valid: boolean; error: string | null } {
  const result = passwordSchema.safeParse(password);
  if (result.success) {
    return { valid: true, error: null };
  }
  return { valid: false, error: result.error.issues[0].message };
}

export function validateEmail(email: string): boolean {
  return emailSchema.safeParse(email).success;
}

export function sanitizeString(input: string): string {
  return input.trim().slice(0, 1000);
}
