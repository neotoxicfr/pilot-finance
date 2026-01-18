'use server'
import { db } from "@/src/db";
import { recurringOperations, accounts, transactions } from "@/src/schema";
import { eq, desc, asc, and, sql } from "drizzle-orm";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { getSession } from "@/src/lib/auth";
import { encrypt, decrypt } from "@/src/lib/crypto";
const roundCurrency = (amount: number) => Math.round((amount + Number.EPSILON) * 100) / 100;
async function getUser() {
    const session = await getSession();
    if (!session || !session.user) redirect('/login');
    return session.user;
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
export async function createRecurring(formData: FormData) {
    const user = await getUser();
    const accountId = parseInt(formData.get("accountId") as string);
    if(!accountId) return;
    const amountRaw = parseFloat(formData.get("amount") as string);
    const type = formData.get("type") as string; 
    let amount = 0;
    if (type === 'transfer') amount = Math.abs(amountRaw);
    else if (type === 'expense') amount = -Math.abs(amountRaw);
    else amount = Math.abs(amountRaw);
    await db.insert(recurringOperations).values({ 
        userId: user.id, 
        accountId, 
        toAccountId: type === 'transfer' ? (formData.get("toAccountId") ? parseInt(formData.get("toAccountId") as string) : null) : null, 
        amount: roundCurrency(amount), 
        description: encrypt(formData.get("description") as string), 
        dayOfMonth: parseInt(formData.get("dayOfMonth") as string), 
        isActive: true 
    });
    revalidatePath("/accounts");
}
export async function updateRecurring(formData: FormData) {
    const user = await getUser();
    const id = parseInt(formData.get("id") as string);
    if(!id) return;
    const accountId = parseInt(formData.get("accountId") as string);
    const amountRaw = parseFloat(formData.get("amount") as string);
    const type = formData.get("type") as string;
    let amount = 0;
    if (type === 'transfer') amount = Math.abs(amountRaw);
    else if (type === 'expense') amount = -Math.abs(amountRaw);
    else amount = Math.abs(amountRaw);
    await db.update(recurringOperations).set({ 
        accountId, 
        toAccountId: type === 'transfer' ? (formData.get("toAccountId") ? parseInt(formData.get("toAccountId") as string) : null) : null, 
        amount: roundCurrency(amount), 
        description: encrypt(formData.get("description") as string), 
        dayOfMonth: parseInt(formData.get("dayOfMonth") as string), 
        isActive: true 
    }).where(and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id)));
    revalidatePath("/accounts");
}
export async function deleteRecurring(id: number) { 
    const user = await getUser();
    await db.delete(recurringOperations).where(and(eq(recurringOperations.id, id), eq(recurringOperations.userId, user.id))); 
    revalidatePath("/accounts"); 
}
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
      if (!op.accountId) continue;
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
          if (account) await db.update(accounts).set({ balance: roundCurrency(account.balance + op.amount) }).where(eq(accounts.id, op.accountId));
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
        if (!lastRun || (lastRun.getMonth() !== currentMonth || lastRun.getFullYear() !== currentYear)) shouldRun = true; 
    } else if (acc.payoutFrequency === 'YEARLY') { 
        if ((!lastRun || lastRun.getFullYear() !== currentYear) && currentMonth === 11 && today.getDate() === 31) shouldRun = true;
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