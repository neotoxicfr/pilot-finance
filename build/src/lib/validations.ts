import { z } from 'zod';

export const authSchema = z.object({
  email: z.string().email("Email invalide"),
  password: z.string().min(8, "Mot de passe trop court"),
  twoFactorCode: z.string().optional(),
});

export const registerSchema = authSchema.extend({
  confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
  message: "Les mots de passe ne correspondent pas",
  path: ["confirmPassword"],
});

export const accountSchema = z.object({
  name: z.string().min(1, "Nom requis"),
  balance: z.coerce.number(),
  color: z.string().regex(/^#[0-9A-F]{6}$/i, "Couleur invalide"),
  isYieldActive: z.coerce.boolean().optional(),
  yieldType: z.string().optional(),
  yieldMin: z.coerce.number().optional(),
  yieldMax: z.coerce.number().optional(),
  yieldFrequency: z.string().optional(),
  payoutFrequency: z.string().optional(),
  reinvestmentRate: z.coerce.number().optional(),
  targetAccountId: z.coerce.number().optional().nullable(),
});

export const transactionSchema = z.object({
  accountId: z.coerce.number().positive(),
  amount: z.coerce.number(),
  description: z.string().min(1, "Description requise"),
  date: z.string().refine((val) => !isNaN(Date.parse(val)), "Date invalide"),
});

export const recurringSchema = z.object({
  accountId: z.coerce.number().positive(),
  toAccountId: z.coerce.number().positive().optional().nullable(),
  amount: z.coerce.number(),
  description: z.string().min(1, "Description requise"),
  dayOfMonth: z.coerce.number().min(1).max(31),
  type: z.enum(['income', 'expense', 'transfer']).optional(),
});