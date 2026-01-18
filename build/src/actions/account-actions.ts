'use server'
import { db } from "@/src/db";
import { accounts } from "@/src/schema";
import { eq, asc, and } from "drizzle-orm";
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
export async function getAccounts() { 
    const user = await getUser();
    const rawAccounts = await db.select().from(accounts).where(eq(accounts.userId, user.id)).orderBy(asc(accounts.position));
    return rawAccounts.map(a => ({ ...a, name: decrypt(a.name) }));
}
export async function createAccount(formData: FormData) {
  const user = await getUser();
  const name = formData.get("name") as string; 
  if (!name) return;
  const balance = parseFloat(formData.get("balance") as string) || 0;
  const all = await db.select().from(accounts).where(eq(accounts.userId, user.id));
  const maxPos = all.reduce((max, c) => Math.max(max, c.position || 0), 0);
  await db.insert(accounts).values({ 
      userId: user.id, 
      name: encrypt(name), 
      balance: roundCurrency(balance), 
      color: formData.get("color") as string || "#3b82f6", 
      isYieldActive: formData.get("isYieldActive") === "on", 
      yieldType: formData.get("yieldType") as string || "FIXED", 
      yieldMin: parseFloat(formData.get("yieldMin") as string) || 0, 
      yieldMax: parseFloat(formData.get("yieldMax") as string) || 0, 
      yieldFrequency: formData.get("yieldFrequency") as string || "YEARLY", 
      payoutFrequency: formData.get("payoutFrequency") as string || "MONTHLY", 
      reinvestmentRate: formData.get("reinvestmentRate") ? parseInt(formData.get("reinvestmentRate") as string) : 100, 
      targetAccountId: formData.get("targetAccountId") ? parseInt(formData.get("targetAccountId") as string) : null, 
      position: maxPos + 1 
  });
  revalidatePath("/"); revalidatePath("/accounts");
}
export async function updateAccountFull(formData: FormData) {
  const user = await getUser();
  const id = parseInt(formData.get("id") as string);
  if (!id) return; 
  await db.update(accounts).set({ 
      name: encrypt(formData.get("name") as string),
      balance: roundCurrency(parseFloat(formData.get("balance") as string) || 0), 
      color: formData.get("color") as string || "#3b82f6", 
      isYieldActive: formData.get("isYieldActive") === "on", 
      yieldType: formData.get("yieldType") as string || "FIXED", 
      yieldMin: parseFloat(formData.get("yieldMin") as string) || 0, 
      yieldMax: parseFloat(formData.get("yieldMax") as string) || 0, 
      yieldFrequency: formData.get("yieldFrequency") as string || "YEARLY", 
      payoutFrequency: formData.get("payoutFrequency") as string || "YEARLY", 
      reinvestmentRate: formData.get("reinvestmentRate") ? parseInt(formData.get("reinvestmentRate") as string) : 100, 
      targetAccountId: formData.get("targetAccountId") ? parseInt(formData.get("targetAccountId") as string) : null, 
      updatedAt: new Date() 
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