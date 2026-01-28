'use server'

import { db } from "@/src/db";
import { accounts, transactions, recurringOperations } from "@/src/schema";
import { eq, desc, and, gte, inArray, asc } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { getSession } from "@/src/lib/auth";
import { encrypt, decrypt } from "@/src/lib/crypto";
import { accountSchema, transactionSchema, recurringSchema } from "@/src/lib/validations";

async function getUser() {
  const session = await getSession();
  if (!session || !session.user) {
    throw new Error("Non connecté");
  }
  return session.user;
}

export async function getAccounts() {
  const user = await getUser();
  const rawAccounts = await db.select()
    .from(accounts)
    .where(eq(accounts.userId, user.id))
    .orderBy(asc(accounts.position));

  return rawAccounts.map((a: any) => ({ ...a, name: decrypt(a.name) }));
}

export async function createAccount(formData: FormData) {
  const user = await getUser();
  const rawData = Object.fromEntries(formData.entries());
  const validation = accountSchema.safeParse(rawData);

  if (!validation.success) {
    throw new Error(validation.error.issues[0].message);
  }

  const all = await db.select().from(accounts).where(eq(accounts.userId, user.id));
  const maxPos = all.reduce((max, c) => Math.max(max, c.position || 0), 0);

  await db.insert(accounts).values({
    userId: user.id,
    name: encrypt(validation.data.name),
    balance: validation.data.balance,
    color: validation.data.color,
    isYieldActive: validation.data.isYieldActive,
    yieldType: validation.data.yieldType || "FIXED",
    yieldMin: validation.data.yieldMin || 0,
    yieldMax: validation.data.yieldMax || 0,
    yieldFrequency: validation.data.yieldFrequency || "YEARLY",
    payoutFrequency: validation.data.payoutFrequency || "MONTHLY",
    reinvestmentRate: validation.data.reinvestmentRate ?? 100,
    targetAccountId: validation.data.targetAccountId,
    position: maxPos + 1
  });

  revalidatePath("/");
  revalidatePath("/accounts");
}

export async function updateAccountFull(formData: FormData) {
  const user = await getUser();
  const id = parseInt(formData.get("id") as string);
  if (!id) return;

  const rawData = Object.fromEntries(formData.entries());
  const validation = accountSchema.safeParse(rawData);

  if (!validation.success) {
    throw new Error(validation.error.issues[0].message);
  }

  await db.update(accounts).set({
    name: encrypt(validation.data.name),
    balance: validation.data.balance,
    color: validation.data.color,
    isYieldActive: validation.data.isYieldActive,
    yieldType: validation.data.yieldType || "FIXED",
    yieldMin: validation.data.yieldMin || 0,
    yieldMax: validation.data.yieldMax || 0,
    yieldFrequency: validation.data.yieldFrequency || "YEARLY",
    payoutFrequency: validation.data.payoutFrequency || "MONTHLY",
    reinvestmentRate: validation.data.reinvestmentRate ?? 100,
    targetAccountId: validation.data.targetAccountId,
    updatedAt: new Date()
  }).where(and(eq(accounts.id, id), eq(accounts.userId, user.id)));

  revalidatePath("/");
  revalidatePath("/accounts");
}

export async function updateBalanceDirect(formData: FormData) {
  const user = await getUser();
  const id = parseInt(formData.get("id") as string);
  const balance = parseFloat(formData.get("balance") as string);

  if (!id || isNaN(balance)) return;

  await db.update(accounts)
    .set({ balance })
    .where(and(eq(accounts.id, id), eq(accounts.userId, user.id)));

  revalidatePath("/");
  revalidatePath("/accounts");
}

export async function deleteAccount(id: number) {
  const user = await getUser();
  await db.delete(accounts).where(and(eq(accounts.id, id), eq(accounts.userId, user.id)));
  revalidatePath("/accounts");
}

export async function swapAccounts(id1: number, id2: number) {
  const user = await getUser();
  const allAccounts = await db.select()
    .from(accounts)
    .where(eq(accounts.userId, user.id))
    .orderBy(asc(accounts.position));

  const acc1 = allAccounts.find(a => a.id === id1);
  const acc2 = allAccounts.find(a => a.id === id2);

  if (acc1 && acc2) {
    await db.transaction(async (tx) => {
      await tx.update(accounts).set({ position: acc2.position }).where(eq(accounts.id, id1));
      await tx.update(accounts).set({ position: acc1.position }).where(eq(accounts.id, id2));
    });
  }

  revalidatePath("/accounts");
}

export async function getTransactions(accountId?: number, page: number = 1, limit: number = 50) {
  const user = await getUser();
  const offset = (page - 1) * limit;

  let conditions = eq(transactions.userId, user.id);
  if (accountId) {
    conditions = and(conditions, eq(transactions.accountId, accountId))!;
  }

  const rawTx = await db.select()
    .from(transactions)
    .where(conditions)
    .orderBy(desc(transactions.date))
    .limit(limit)
    .offset(offset);

  return rawTx.map((t: any) => ({ ...t, description: decrypt(t.description) }));
}

export async function addTransaction(formData: FormData) {
  const user = await getUser();
  const rawData = Object.fromEntries(formData.entries());
  const validation = transactionSchema.safeParse(rawData);

  if (!validation.success) {
    throw new Error(validation.error.issues[0].message);
  }

  await db.transaction(async (tx) => {
    await tx.insert(transactions).values({
      userId: user.id,
      accountId: validation.data.accountId,
      amount: validation.data.amount,
      description: encrypt(validation.data.description),
      date: new Date(validation.data.date)
    });

    const [acc] = await tx.select().from(accounts).where(eq(accounts.id, validation.data.accountId));
    if (acc) {
      await tx.update(accounts)
        .set({ balance: acc.balance + validation.data.amount })
        .where(eq(accounts.id, validation.data.accountId));
    }
  });

  revalidatePath("/");
  revalidatePath("/accounts");
}

export async function checkRecurringOperations() {
  const user = await getUser();
  const ops = await db.select()
    .from(recurringOperations)
    .where(and(eq(recurringOperations.userId, user.id), eq(recurringOperations.isActive, true)));

  if (ops.length === 0) return;

  const today = new Date();
  const currentMonthStart = new Date(today.getFullYear(), today.getMonth(), 1);

  const accountIds = new Set<number>();
  ops.forEach(op => {
    accountIds.add(op.accountId);
    if (op.toAccountId) accountIds.add(op.toAccountId);
  });

  const userAccounts = await db.select()
    .from(accounts)
    .where(and(
      eq(accounts.userId, user.id),
      inArray(accounts.id, Array.from(accountIds))
    ));

  const accountMap = new Map(userAccounts.map((a: any) => [a.id, a]));

  const existingTx = await db.select()
    .from(transactions)
    .where(and(
      eq(transactions.userId, user.id),
      gte(transactions.date, currentMonthStart),
      inArray(transactions.accountId, Array.from(accountIds))
    ));

  const existingDescriptions = new Set(existingTx.map((t: any) => decrypt(t.description)));

  const balanceUpdates = new Map<number, number>();

  await db.transaction(async (tx) => {
    for (const op of ops) {
      if (today.getDate() >= op.dayOfMonth) {
        const opDesc = decrypt(op.description);

        if (!existingDescriptions.has(opDesc)) {
          const sourceAccount = accountMap.get(op.accountId);
          if (!sourceAccount) continue;

          await tx.insert(transactions).values({
            userId: user.id,
            accountId: op.accountId,
            amount: op.amount,
            description: encrypt(opDesc),
            date: today
          });

          const currentSourceBalance = balanceUpdates.get(op.accountId) ?? sourceAccount.balance;
          balanceUpdates.set(op.accountId, currentSourceBalance + op.amount);

          if (op.toAccountId) {
            const destAccount = accountMap.get(op.toAccountId);
            if (destAccount) {
              await tx.insert(transactions).values({
                userId: user.id,
                accountId: op.toAccountId,
                amount: Math.abs(op.amount),
                description: encrypt(opDesc),
                date: today
              });

              const currentDestBalance = balanceUpdates.get(op.toAccountId) ?? destAccount.balance;
              balanceUpdates.set(op.toAccountId, currentDestBalance + Math.abs(op.amount));
            }
          }

          await tx.update(recurringOperations)
            .set({ lastRunDate: today })
            .where(eq(recurringOperations.id, op.id));
        }
      }
    }

    for (const [accountId, newBalance] of balanceUpdates) {
      await tx.update(accounts)
        .set({ balance: newBalance })
        .where(eq(accounts.id, accountId));
    }
  });
}

export async function getRecurringOperations() {
  const user = await getUser();
  const ops = await db.select()
    .from(recurringOperations)
    .where(eq(recurringOperations.userId, user.id));

  const accs = await getAccounts();
  const accMap = new Map(accs.map((a: any) => [a.id, a.name]));

  return ops.map((o: any) => ({
    ...o,
    description: decrypt(o.description),
    accountName: accMap.get(o.accountId) || 'Inconnu',
    toAccountName: o.toAccountId ? accMap.get(o.toAccountId) : null
  }));
}

export async function createRecurring(formData: FormData) {
  const user = await getUser();
  const rawData = Object.fromEntries(formData.entries());
  const validation = recurringSchema.safeParse(rawData);

  if (!validation.success) {
    throw new Error(validation.error.issues[0].message);
  }

  let finalAmount = validation.data.amount;
  if (validation.data.type === 'expense' || validation.data.type === 'transfer') {
    finalAmount = -Math.abs(finalAmount);
  } else {
    finalAmount = Math.abs(finalAmount);
  }

  await db.insert(recurringOperations).values({
    userId: user.id,
    accountId: validation.data.accountId,
    toAccountId: validation.data.toAccountId || null,
    amount: finalAmount,
    description: encrypt(validation.data.description),
    dayOfMonth: validation.data.dayOfMonth,
    isActive: true
  });

  revalidatePath("/accounts");
}

export async function updateRecurring(formData: FormData) {
  const user = await getUser();
  const id = parseInt(formData.get("id") as string);
  if (!id) return;

  const rawData = Object.fromEntries(formData.entries());
  const validation = recurringSchema.safeParse(rawData);

  if (!validation.success) {
    throw new Error(validation.error.issues[0].message);
  }

  let finalAmount = validation.data.amount;
  if (validation.data.type === 'expense' || validation.data.type === 'transfer') {
    finalAmount = -Math.abs(finalAmount);
  } else {
    finalAmount = Math.abs(finalAmount);
  }

  await db.update(recurringOperations).set({
    accountId: validation.data.accountId,
    toAccountId: validation.data.toAccountId || null,
    amount: finalAmount,
    description: encrypt(validation.data.description),
    dayOfMonth: validation.data.dayOfMonth
  }).where(and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id)));

  revalidatePath("/accounts");
}

export async function deleteRecurring(id: number) {
  const user = await getUser();
  await db.delete(recurringOperations).where(
    and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id))
  );
  revalidatePath("/accounts");
}

export async function getDashboardData(years: number = 5) {
  const accs = await getAccounts();
  const projection = [];
  let totalInterests = 0;

  for (let i = 0; i <= years; i++) {
    const yearData: Record<string, number | string> = {
      name: `Année ${i}`,
      totalMin: 0,
      totalMax: 0,
      totalAvg: 0
    };

    for (const acc of accs) {
      if (!acc.isYieldActive) {
        yearData[acc.name] = acc.balance;
        (yearData.totalMin as number) += acc.balance;
        (yearData.totalMax as number) += acc.balance;
        (yearData.totalAvg as number) += acc.balance;
      } else {
        const rateMin = acc.yieldType === 'RANGE' ? acc.yieldMin : acc.yieldMin;
        const rateMax = acc.yieldType === 'RANGE' ? acc.yieldMax : acc.yieldMin;

        const compoundMin = acc.balance * Math.pow(1 + (rateMin || 0) / 100, i);
        const compoundMax = acc.balance * Math.pow(1 + (rateMax || 0) / 100, i);
        const compoundAvg = (compoundMin + compoundMax) / 2;

        yearData[acc.name] = Math.round(compoundAvg);
        (yearData.totalMin as number) += compoundMin;
        (yearData.totalMax as number) += compoundMax;
        (yearData.totalAvg as number) += compoundAvg;

        if (i === years) {
          totalInterests += (compoundAvg - acc.balance);
        }
      }
    }

    yearData.totalMin = Math.round(yearData.totalMin as number);
    yearData.totalMax = Math.round(yearData.totalMax as number);
    yearData.totalAvg = Math.round(yearData.totalAvg as number);
    projection.push(yearData);
  }

  return {
    accounts: accs,
    projection,
    totalInterests: Math.round(totalInterests)
  };
}
