'use client'

import { useState, useEffect } from 'react';
import { getAccounts, createAccount, updateAccountFull, deleteAccount, updateBalanceDirect, createRecurring, updateRecurring, deleteRecurring, swapAccounts, getRecurringOperations } from '@/src/actions';
import { Wallet, Plus, Trash2, Save, RefreshCw, ArrowRight, Pencil, X, ChevronDown, ChevronUp, TrendingUp, Target, Palette, Percent } from "lucide-react";

// --- CONFIGURATION ---
const COLOR_PRESETS = [
  "#ef4444", "#f97316", "#f59e0b", "#eab308", "#84cc16", "#22c55e",
  "#10b981", "#14b8a6", "#06b6d4", "#0ea5e9", "#3b82f6", "#6366f1",
  "#8b5cf6", "#a855f7", "#d946ef", "#ec4899", "#f43f5e", "#64748b"
];

const formatMoney = (amount: number) => {
  const val = Number(amount);
  if (isNaN(val)) return "0 €";
  const decimals = Number.isInteger(val) ? 0 : 2;
  return new Intl.NumberFormat('fr-FR', {
    style: 'currency',
    currency: 'EUR',
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals
  }).format(val);
};

const formatInputBalance = (amount: number) => {
  const val = Number(amount);
  if (isNaN(val)) return "";
  if (Number.isInteger(val)) return val.toString();
  return val.toFixed(2);
};

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<any[]>([]);
  const [recurrings, setRecurrings] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const [isCreatingAccount, setIsCreatingAccount] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [accFormData, setAccFormData] = useState({
    name: '', balance: '', color: '#3b82f6',
    isYieldActive: false, yieldType: 'FIXED', yieldMin: '', yieldMax: '',
    yieldFrequency: 'YEARLY', payoutFrequency: 'MONTHLY',
    reinvestmentRate: 100, targetAccountId: ''
  });

  const [isCreatingRecurring, setIsCreatingRecurring] = useState(false);
  const [editingRecurringId, setEditingRecurringId] = useState<number | null>(null);
  const [recurringType, setRecurringType] = useState('expense');
  const [recurringSourceId, setRecurringSourceId] = useState('');
  const [recurringToId, setRecurringToId] = useState('');
  const [recurringForm, setRecurringForm] = useState({ description: '', amount: '', dayOfMonth: '' });

  const [sortConfig, setSortConfig] = useState<{ key: string, direction: 'asc' | 'desc' }>({ key: 'dayOfMonth', direction: 'asc' });

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    try {
      const [accData, opsData] = await Promise.all([getAccounts(), getRecurringOperations()]);
      setAccounts(accData || []);
      setRecurrings(opsData || []);
    } catch (e) {
      console.error("Erreur chargement données", e);
    } finally {
      setLoading(false);
    }
  }

  const currentDay = new Date().getDate();
  let monthlyYieldPayout = 0;
  const virtualYieldsOps: any[] = [];

  accounts.forEach((acc: any) => {
    if (acc.isYieldActive) {
      const rate = acc.yieldType === 'RANGE' ? (acc.yieldMin + acc.yieldMax) / 2 : acc.yieldMin;
      const annualGain = acc.balance * (rate / 100);
      const monthlyGain = annualGain / 12;
      const payout = monthlyGain * (1 - (acc.reinvestmentRate / 100));
      monthlyYieldPayout += payout;

      if (payout > 1) {
        virtualYieldsOps.push({
          id: `yield-${acc.id}`,
          description: `Rendement ${acc.name}`,
          amount: payout,
          dayOfMonth: 1,
          isVirtual: true,
          accountName: acc.name,
          isPayout: true
        });
      }
    }
  });

  const workIncome = recurrings.filter((r: any) => r.amount > 0 && !r.toAccountId).reduce((sum: number, r: any) => sum + r.amount, 0);
  const fixedExpenses = recurrings.filter((r: any) => r.amount < 0 && !r.toAccountId).reduce((sum: number, r: any) => sum + Math.abs(r.amount), 0);
  const transfersToSavings = recurrings.filter((r: any) => r.toAccountId && accounts.find((a: any) => a.id === r.toAccountId)?.isYieldActive).reduce((sum: number, r: any) => sum + Math.abs(r.amount), 0);

  const totalIncomeMonth = workIncome + monthlyYieldPayout;
  const totalExpenseMonth = fixedExpenses;
  const totalNetMonth = totalIncomeMonth - totalExpenseMonth;

  const handleEditClick = (account: any) => {
    setIsCreatingAccount(false);
    setEditingId(account.id);
    setAccFormData({
      name: account.name,
      balance: formatInputBalance(account.balance),
      color: account.color,
      isYieldActive: account.isYieldActive,
      yieldType: account.yieldType || 'FIXED',
      yieldMin: account.yieldMin || '',
      yieldMax: account.yieldMax || '',
      yieldFrequency: account.yieldFrequency || 'YEARLY',
      payoutFrequency: account.payoutFrequency || 'MONTHLY',
      reinvestmentRate: account.reinvestmentRate ?? 100,
      targetAccountId: account.targetAccountId || ''
    });
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleCancelEdit = () => {
    setEditingId(null);
    setIsCreatingAccount(false);
    setAccFormData({ name: '', balance: '', color: '#3b82f6', isYieldActive: false, yieldType: 'FIXED', yieldMin: '', yieldMax: '', yieldFrequency: 'YEARLY', payoutFrequency: 'MONTHLY', reinvestmentRate: 100, targetAccountId: '' });
  };

  const moveAccount = async (idx: number, direction: 'up' | 'down') => {
    const targetIdx = direction === 'up' ? idx - 1 : idx + 1;
    if (targetIdx >= 0 && targetIdx < accounts.length) {
      await swapAccounts(accounts[idx].id, accounts[targetIdx].id);
      loadData();
    }
  };

  const handleCreateOrUpdateAccount = async (formDataPayload: FormData) => {
    if (editingId) await updateAccountFull(formDataPayload);
    else await createAccount(formDataPayload);
    handleCancelEdit();
    loadData();
  };

  const handleEditRecurring = (op: any) => {
    setIsCreatingRecurring(false);
    setEditingRecurringId(op.id);
    setRecurringForm({ description: op.description, amount: Math.abs(op.amount).toString(), dayOfMonth: op.dayOfMonth });
    setRecurringSourceId(op.accountId.toString());
    if (op.toAccountId) {
      setRecurringType('transfer');
      setRecurringToId(op.toAccountId.toString());
    } else {
      setRecurringType(op.amount > 0 ? 'income' : 'expense');
      setRecurringToId('');
    }
    const el = document.getElementById('recurring-form');
    if (el) el.scrollIntoView({ behavior: 'smooth' });
  };

  const handleCancelRecurring = () => {
    setEditingRecurringId(null);
    setIsCreatingRecurring(false);
    setRecurringForm({ description: '', amount: '', dayOfMonth: '' });
    setRecurringType('expense');
    setRecurringSourceId('');
    setRecurringToId('');
  };

  const handleCreateOrUpdateRecurring = async (formDataPayload: FormData) => {
    if (editingRecurringId) await updateRecurring(formDataPayload);
    else await createRecurring(formDataPayload);
    handleCancelRecurring();
    loadData();
  };

  const handleSort = (key: string) => {
    setSortConfig({ key, direction: sortConfig.key === key && sortConfig.direction === 'asc' ? 'desc' : 'asc' });
  };

  const sortedRecurrings = [...recurrings].sort((a: any, b: any) => {
    let aVal = a[sortConfig.key];
    let bVal = b[sortConfig.key];
    if (sortConfig.key === 'amount' || sortConfig.key === 'dayOfMonth') {
      aVal = Number(aVal || 0);
      bVal = Number(bVal || 0);
    }
    if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
    if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
    return 0;
  });

  if (loading) return <div className="p-10 text-center text-muted-foreground">Chargement...</div>;

  return (
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8 text-foreground">
      <style jsx global>{` input::-webkit-outer-spin-button, input::-webkit-inner-spin-button { -webkit-appearance: none; margin: 0; } input[type=number] { -moz-appearance: textfield; } `}</style>

      {/* BANDEAU SYNTHÈSE */}
      <div className="dashboard-card bg-background border rounded-2xl overflow-hidden grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-border">
        {/* BLOC MENSUEL */}
        <div className="p-5 min-w-0">
          <div className="flex items-center gap-2 mb-4">
            <span className="bg-blue-500/10 text-blue-500 px-3 py-1 rounded-xl text-xs font-bold uppercase tracking-wider">Mensuel</span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 sm:gap-6">
            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Entrées</div>
                <div className="text-lg sm:text-2xl font-mono font-bold text-emerald-500 truncate">+{formatMoney(totalIncomeMonth)}</div>
                {monthlyYieldPayout > 0 && <div className="hidden sm:block text-[11px] text-emerald-500/70 mt-1 truncate">Dont loyers: {formatMoney(monthlyYieldPayout)}</div>}
              </div>
            </div>

            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2 border-t border-border sm:border-0 pt-2 sm:pt-0">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Sorties</div>
                <div className="text-lg sm:text-2xl font-mono font-bold text-foreground truncate">-{formatMoney(totalExpenseMonth)}</div>
                {transfersToSavings > 0 && <div className="hidden sm:block text-[11px] text-blue-400/70 mt-1 truncate">Dont épargne: {formatMoney(transfersToSavings)}</div>}
              </div>
            </div>

            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2 border-t border-border sm:border-0 pt-2 sm:pt-0">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Reste</div>
                <div className={`text-lg sm:text-2xl font-mono font-bold truncate ${totalNetMonth >= 0 ? 'text-blue-500' : 'text-red-500'}`}>
                  {totalNetMonth > 0 ? '+' : ''}{formatMoney(totalNetMonth)}
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* BLOC ANNUEL */}
        <div className="p-5 bg-accent/20 min-w-0">
          <div className="flex items-center gap-2 mb-4">
            <span className="bg-purple-500/10 text-purple-500 px-3 py-1 rounded-xl text-xs font-bold uppercase tracking-wider">Annuel (Proj.)</span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 sm:gap-6 opacity-90">
            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Entrées</div>
                <div className="text-base sm:text-xl font-mono font-bold text-emerald-500 truncate">+{formatMoney(totalIncomeMonth * 12)}</div>
                {monthlyYieldPayout > 0 && <div className="hidden sm:block text-[11px] text-emerald-500/70 mt-1 truncate">Dont loyers: {formatMoney(monthlyYieldPayout * 12)}</div>}
              </div>
            </div>

            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2 border-t border-border sm:border-0 pt-2 sm:pt-0">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Sorties</div>
                <div className="text-base sm:text-xl font-mono font-bold text-foreground truncate">-{formatMoney(totalExpenseMonth * 12)}</div>
                {transfersToSavings > 0 && <div className="hidden sm:block text-[11px] text-blue-400/70 mt-1 truncate">Dont épargne: {formatMoney(transfersToSavings * 12)}</div>}
              </div>
            </div>

            <div className="min-w-0 flex sm:block items-baseline justify-between gap-2 border-t border-border sm:border-0 pt-2 sm:pt-0">
              <div className="min-w-0">
                <div className="text-[10px] sm:text-xs text-muted-foreground uppercase font-bold mb-1 truncate">Reste</div>
                <div className={`text-base sm:text-xl font-mono font-bold truncate ${totalNetMonth >= 0 ? 'text-blue-500' : 'text-red-500'}`}>
                  {totalNetMonth > 0 ? '+' : ''}{formatMoney(totalNetMonth * 12)}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        
        {/* COLONNE GAUCHE : COMPTES */}
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <h2 className="text-lg font-semibold flex items-center gap-2 text-foreground"><Wallet size={18} /> Mes Comptes</h2>
            {!isCreatingAccount && !editingId && (
              <button onClick={() => setIsCreatingAccount(true)} className="text-xs bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 rounded-xl flex items-center gap-1 transition-all font-bold">
                <Plus size={16} /> Ajouter
              </button>
            )}
          </div>

          {(isCreatingAccount || editingId) && (
            <div className="dashboard-card bg-background border border-blue-500/50 rounded-2xl p-5 mb-4 relative animate-in slide-in-from-left-2">
              <button onClick={handleCancelEdit} className="absolute top-4 right-4 text-muted-foreground hover:text-foreground"><X size={20} /></button>
              <h3 className="text-base font-bold text-blue-500 mb-5">{editingId ? 'Modifier le compte' : 'Nouveau Compte'}</h3>
              <form action={handleCreateOrUpdateAccount} className="space-y-5">
                {editingId && <input type="hidden" name="id" value={editingId} />}
                <div className="grid grid-cols-2 gap-4">
                  <input type="text" name="name" placeholder="Nom" required value={accFormData.name} onChange={e => setAccFormData({ ...accFormData, name: e.target.value })} className="bg-accent border border-border rounded-xl p-3 text-sm w-full outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all text-foreground" />
                  <input type="text" inputMode="decimal" name="balance" placeholder="Solde" value={accFormData.balance} onChange={e => setAccFormData({ ...accFormData, balance: e.target.value })} className="bg-accent border border-border rounded-xl p-3 text-sm w-full font-mono outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all text-foreground" />
                </div>
                <div>
                  <label className="text-[10px] uppercase font-bold text-muted-foreground mb-2 block tracking-wider">Couleur</label>
                  <div className="flex flex-wrap gap-2 mb-3">
                    {COLOR_PRESETS.map(c => (
                      <button type="button" key={c} onClick={() => setAccFormData({ ...accFormData, color: c })} className={`w-8 h-8 rounded-full transition-transform ${accFormData.color === c ? 'ring-2 ring-foreground scale-110' : 'opacity-60 hover:opacity-100'}`} style={{ backgroundColor: c }}></button>
                    ))}
                  </div>
                  <div className="flex items-center gap-2 bg-accent border border-border rounded-xl p-2.5 w-full max-w-[200px] relative">
                    <Palette size={16} className="text-muted-foreground" />
                    <input type="text" name="color" placeholder="#RRGGBB" value={accFormData.color} onChange={e => setAccFormData({ ...accFormData, color: e.target.value })} className="bg-transparent text-sm w-full outline-none font-mono text-foreground uppercase" />
                    <div className="w-6 h-6 rounded-full border border-border relative overflow-hidden cursor-pointer">
                      <div style={{ backgroundColor: accFormData.color }} className="absolute inset-0"></div>
                      <input type="color" value={accFormData.color} onChange={e => setAccFormData({ ...accFormData, color: e.target.value })} className="absolute inset-0 opacity-0 cursor-pointer" />
                    </div>
                  </div>
                </div>
                
                <div className="border-t border-border pt-4">
                  <label className="flex items-center gap-3 cursor-pointer text-sm group select-none">
                    <div className={`w-10 h-6 rounded-full p-1 transition-colors ${accFormData.isYieldActive ? 'bg-blue-600' : 'bg-accent'}`}>
                      <div className={`w-4 h-4 bg-white rounded-full transform transition-transform ${accFormData.isYieldActive ? 'translate-x-4' : ''}`}></div>
                    </div>
                    <input type="checkbox" name="isYieldActive" className="hidden" checked={accFormData.isYieldActive} onChange={e => setAccFormData({ ...accFormData, isYieldActive: e.target.checked })} />
                    <span className="text-muted-foreground group-hover:text-foreground transition-colors">Activer le rendement</span>
                  </label>
                  {accFormData.isYieldActive && (
                    <div className="mt-4 space-y-4 bg-accent/30 p-4 rounded-xl border border-border text-sm animate-in fade-in slide-in-from-top-2">
                      <div className="flex bg-accent rounded-xl p-1 border border-border">
                        <button type="button" onClick={() => setAccFormData({ ...accFormData, yieldType: 'FIXED' })} className={`flex-1 py-1.5 rounded-lg text-xs font-bold transition-all ${accFormData.yieldType === 'FIXED' ? 'bg-background text-foreground border border-border' : 'text-muted-foreground hover:text-foreground'}`}>Taux Fixe</button>
                        <button type="button" onClick={() => setAccFormData({ ...accFormData, yieldType: 'RANGE' })} className={`flex-1 py-1.5 rounded-lg text-xs font-bold transition-all ${accFormData.yieldType === 'RANGE' ? 'bg-background text-foreground border border-border' : 'text-muted-foreground hover:text-foreground'}`}>Fourchette</button>
                        <input type="hidden" name="yieldType" value={accFormData.yieldType} />
                      </div>
                      <div className="flex gap-3 items-center">
                        <div className="relative w-full">
                          <input type="number" step="0.1" name="yieldMin" placeholder={accFormData.yieldType === 'RANGE' ? "Min" : "Taux"} value={accFormData.yieldMin} onChange={e => setAccFormData({ ...accFormData, yieldMin: e.target.value })} className="bg-background border border-border rounded-xl p-2.5 pl-9 w-full outline-none focus:border-blue-500 text-foreground" />
                          <Percent size={14} className="absolute left-3 top-3 text-muted-foreground" />
                        </div>
                        {accFormData.yieldType === 'RANGE' && (
                          <>
                            <span className="text-muted-foreground">-</span>
                            <div className="relative w-full">
                              <input type="number" step="0.1" name="yieldMax" placeholder="Max" value={accFormData.yieldMax} onChange={e => setAccFormData({ ...accFormData, yieldMax: e.target.value })} className="bg-background border border-border rounded-xl p-2.5 pl-9 w-full outline-none focus:border-blue-500 text-foreground" />
                              <Percent size={14} className="absolute left-3 top-3 text-muted-foreground" />
                            </div>
                          </>
                        )}
                      </div>
                      <div className="grid grid-cols-2 gap-4">
                        <div>
                          <label className="text-[10px] uppercase font-bold text-muted-foreground mb-1.5 block">Calculé</label>
                          <select name="yieldFrequency" value={accFormData.yieldFrequency} onChange={e => setAccFormData({ ...accFormData, yieldFrequency: e.target.value })} className="w-full bg-background border border-border text-foreground rounded-xl p-2.5 text-xs outline-none focus:border-blue-500">
                            <option value="YEARLY">Par An</option>
                            <option value="MONTHLY">Par Mois</option>
                          </select>
                        </div>
                        <div>
                          <label className="text-[10px] uppercase font-bold text-muted-foreground mb-1.5 block">Versé</label>
                          <select name="payoutFrequency" value={accFormData.payoutFrequency} onChange={e => setAccFormData({ ...accFormData, payoutFrequency: e.target.value })} className="w-full bg-background border border-border text-foreground rounded-xl p-2.5 text-xs outline-none focus:border-blue-500">
                            <option value="MONTHLY">Tous les mois</option>
                            <option value="YEARLY">Tous les ans</option>
                          </select>
                        </div>
                      </div>
                      <div className="space-y-3 pt-2 border-t border-border">
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-muted-foreground">Taux de réinvestissement</span>
                          <span className="font-bold text-foreground bg-accent px-2 py-0.5 rounded">{accFormData.reinvestmentRate}%</span>
                        </div>
                        <input type="range" name="reinvestmentRate" min="0" max="100" step="5" value={accFormData.reinvestmentRate} onChange={e => setAccFormData({ ...accFormData, reinvestmentRate: parseInt(e.target.value) })} className="w-full h-1.5 bg-accent rounded-lg appearance-none cursor-pointer accent-blue-500" />
                        {accFormData.reinvestmentRate < 100 && (
                          <div className="mt-2 animate-in fade-in">
                            <label className="block text-[10px] uppercase text-emerald-500 font-bold mb-1.5">Verser les gains ({100 - accFormData.reinvestmentRate}%) sur :</label>
                            <select name="targetAccountId" value={accFormData.targetAccountId} onChange={e => setAccFormData({ ...accFormData, targetAccountId: e.target.value })} className="w-full bg-background border border-emerald-500/30 text-emerald-600 rounded-xl p-2.5 text-xs outline-none focus:border-emerald-500">
                              <option value="">-- Choisir un compte --</option>
                              {accounts.filter((a: any) => a.id !== editingId).map((a: any) => <option key={a.id} value={a.id}>{a.name}</option>)}
                            </select>
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
                <button className="w-full bg-blue-600 hover:bg-blue-500 text-white py-3 rounded-xl text-sm font-bold transition-all">Valider</button>
              </form>
            </div>
          )}

          <div className="space-y-3">
            {accounts.map((account: any, idx: number) => (
              <div key={account.id} className="dashboard-card group bg-background border hover:border-blue-500/30 rounded-2xl p-4 flex items-center gap-4 transition-all">
                <div className="flex flex-col gap-1">
                  <button onClick={() => moveAccount(idx, 'up')} disabled={idx === 0} className="p-1 hover:text-blue-500 disabled:opacity-20 text-muted-foreground transition-colors"><ChevronUp size={20} /></button>
                  <button onClick={() => moveAccount(idx, 'down')} disabled={idx === accounts.length - 1} className="p-1 hover:text-blue-500 disabled:opacity-20 text-muted-foreground transition-colors"><ChevronDown size={20} /></button>
                </div>
                <div className="w-1.5 h-12 rounded-full flex-shrink-0" style={{ backgroundColor: account.color }}></div>
                <div className="flex-1 min-w-0">
                  <h3 className="font-bold text-foreground text-base truncate">{account.name}</h3>
                  {account.isYieldActive && (
                    <div className="flex items-center gap-1.5 text-xs text-emerald-500 mt-0.5">
                      <TrendingUp size={14} />
                      <span className="font-medium">{account.yieldType === 'FIXED' ? `${account.yieldMin}%` : `${account.yieldMin}-${account.yieldMax}%`}</span>
                      {account.reinvestmentRate < 100 && <span className="text-muted-foreground text-[10px] ml-1 bg-accent px-1.5 py-0.5 rounded border border-border">Loyer {(100 - account.reinvestmentRate)}%</span>}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-3">
                  <form action={async (fd) => { await updateBalanceDirect(fd); loadData(); }} className="flex items-baseline gap-1">
                    <input type="hidden" name="id" value={account.id} />
                    <input
                      type="text"
                      inputMode="decimal"
                      name="balance"
                      defaultValue={formatInputBalance(account.balance)}
                      className="bg-transparent w-28 text-right outline-none font-mono text-xl font-bold text-foreground focus:text-blue-500 focus:bg-accent rounded-lg px-1 transition-colors"
                    />
                    <span className="text-sm text-muted-foreground font-medium">€</span>
                    <button className="hidden group-hover:block text-blue-500 ml-1 p-2 hover:bg-accent rounded-lg transition-colors"><Save size={18} /></button>
                  </form>
                  <div className="flex flex-col gap-1">
                    <button onClick={() => handleEditClick(account)} className="p-2 text-muted-foreground hover:text-blue-500 hover:bg-accent rounded-lg transition-colors"><Pencil size={18} /></button>
                    <button onClick={async () => { if (confirm('Supprimer ?')) { await deleteAccount(account.id); loadData(); } }} className="p-2 text-muted-foreground hover:text-red-500 hover:bg-accent rounded-lg transition-colors"><Trash2 size={18} /></button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* COLONNE DROITE : OPÉRATIONS */}
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <h2 className="text-lg font-semibold flex items-center gap-2 text-foreground"><RefreshCw size={18} /> Opérations</h2>
            {!isCreatingRecurring && !editingRecurringId && (
              <button onClick={() => setIsCreatingRecurring(true)} className="text-xs bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 rounded-xl flex items-center gap-1 transition-all font-bold">
                <Plus size={16} /> Ajouter
              </button>
            )}
          </div>

          {(isCreatingRecurring || editingRecurringId) && (
            <div id="recurring-form" className="dashboard-card bg-background border border-blue-500/50 rounded-2xl p-4 relative animate-in slide-in-from-right-2">
              <button onClick={handleCancelRecurring} className="absolute top-3 right-3 text-muted-foreground hover:text-foreground"><X size={18} /></button>
              <h3 className="text-sm font-bold text-blue-500 mb-4">{editingRecurringId ? 'Modifier' : 'Nouvelle Opération'}</h3>
              <form action={handleCreateOrUpdateRecurring} className="space-y-4">
                {editingRecurringId && <input type="hidden" name="id" value={editingRecurringId} />}
                <div className="flex gap-3 mb-3">
                  <input type="text" name="description" placeholder="Libellé" required value={recurringForm.description} onChange={e => setRecurringForm({ ...recurringForm, description: e.target.value })} className="flex-1 bg-accent border border-border text-foreground rounded-xl p-2.5 text-sm outline-none focus:border-blue-500" />
                  <input type="number" step="0.01" name="amount" placeholder="€" required value={recurringForm.amount} onChange={e => setRecurringForm({ ...recurringForm, amount: e.target.value })} className="w-24 bg-accent border border-border text-foreground rounded-xl p-2.5 text-sm outline-none font-mono focus:border-blue-500" />
                  <input type="number" min="1" max="31" name="dayOfMonth" placeholder="J" required value={recurringForm.dayOfMonth} onChange={e => setRecurringForm({ ...recurringForm, dayOfMonth: e.target.value })} className="w-14 bg-accent border border-border text-foreground rounded-xl p-2.5 text-sm outline-none text-center focus:border-blue-500" />
                </div>
                <div className="flex gap-3">
                  <select name="type" value={recurringType} onChange={(e) => setRecurringType(e.target.value)} className="w-28 bg-accent border border-border text-foreground rounded-xl p-2.5 text-xs outline-none focus:border-blue-500">
                    <option value="expense">Sortie</option>
                    <option value="income">Entrée</option>
                    <option value="transfer">Virement</option>
                  </select>
                  <select name="accountId" required value={recurringSourceId} onChange={(e) => setRecurringSourceId(e.target.value)} className="flex-1 bg-accent border border-border text-foreground rounded-xl p-2.5 text-xs outline-none focus:border-blue-500">
                    <option value="">Compte...</option>
                    {accounts.map((acc: any) => <option key={acc.id} value={acc.id}>{acc.name}</option>)}
                  </select>
                  <button className="px-5 bg-blue-600 hover:bg-blue-500 rounded-xl text-white transition-all font-bold text-xs uppercase tracking-wide">Valider</button>
                </div>
                {recurringType === 'transfer' && (
                  <div className="mt-3 animate-in fade-in">
                    <select name="toAccountId" required value={recurringToId} onChange={(e) => setRecurringToId(e.target.value)} className="w-full bg-accent border border-border text-blue-500 rounded-xl p-2.5 text-xs outline-none font-medium focus:border-blue-500">
                      <option value="">➤ Vers quel compte ?</option>
                      {accounts.filter((acc: any) => acc.id.toString() !== recurringSourceId).map((acc: any) => <option key={acc.id} value={acc.id}>{acc.name}</option>)}
                    </select>
                  </div>
                )}
              </form>
            </div>
          )}

          <div className="dashboard-card bg-background border rounded-2xl overflow-hidden">
            {/* OVERSCROLL-NONE pour éviter le rebond de la page */}
            <div className="max-h-[600px] overflow-y-auto overflow-x-hidden w-full overscroll-none">
              <table className="w-full text-sm text-left text-muted-foreground table-fixed">
                <thead className="text-xs font-bold uppercase bg-background text-muted-foreground sticky top-0 z-20">
                  <tr>
                    {/* EN-TÊTE AVEC PSEUDO-BORDURE POUR RESTER VISIBLE AU SCROLL */}
                    <th className="relative px-2 md:px-4 py-3 bg-background after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[1px] after:bg-border cursor-pointer hover:text-foreground w-10 md:w-16 transition-colors" onClick={() => handleSort('dayOfMonth')}>J {sortConfig.key === 'dayOfMonth' && (sortConfig.direction === 'asc' ? '↓' : '↑')}</th>
                    <th className="relative px-2 md:px-4 py-3 bg-background after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[1px] after:bg-border w-auto">Libellé</th>
                    <th className="relative px-2 md:px-4 py-3 bg-background after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[1px] after:bg-border text-right cursor-pointer hover:text-foreground w-32 md:w-40 transition-colors" onClick={() => handleSort('amount')}>Montant {sortConfig.key === 'amount' && (sortConfig.direction === 'asc' ? '↓' : '↑')}</th>
                    <th className="relative px-2 md:px-4 py-3 bg-background after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[1px] after:bg-border w-20 md:w-24"></th>
                  </tr>
                </thead>
                {/* CORRECTION DOUBLE ÉPAISSEUR : On applique border-b à toutes les lignes SAUF la dernière */}
                <tbody className="[&_tr:not(:last-child)]:border-b [&_tr]:border-border">
                  {virtualYieldsOps.length > 0 && (
                    <>
                      <tr className="bg-accent">
                        <td colSpan={4} className="px-2 md:px-4 py-2 text-xs font-bold uppercase text-emerald-500/70 tracking-wider">Loyers perçus</td>
                      </tr>
                      {virtualYieldsOps.map((op: any) => (
                        <tr key={op.id} className="bg-emerald-900/5 hover:bg-emerald-500/10 transition-colors">
                          <td className="px-2 md:px-4 py-3 font-mono text-sm text-emerald-600/50 text-center border-r border-border">1</td>
                          <td className="px-2 md:px-4 py-3 text-emerald-600 dark:text-emerald-400 font-medium text-sm flex items-center gap-2 truncate">
                            <Target size={14} className="text-emerald-500 flex-shrink-0" />
                            <span className="truncate">{op.description}</span>
                          </td>
                          <td className="px-2 md:px-4 py-3 text-right font-mono font-bold text-sm text-emerald-500 whitespace-nowrap tabular-nums">+{formatMoney(op.amount)}</td>
                          <td className="px-2 md:px-4 py-3"></td>
                        </tr>
                      ))}
                      <tr className="bg-accent">
                        <td colSpan={4} className="px-2 md:px-4 py-2 text-xs font-bold uppercase text-muted-foreground tracking-wider">Opérations planifiées</td>
                      </tr>
                    </>
                  )}
                  {sortedRecurrings.map((op: any) => {
                    const isPassed = op.dayOfMonth < currentDay;
                    return (
                      <tr key={op.id} className={`hover:bg-accent transition-colors ${isPassed ? 'opacity-50' : ''}`}>
                        <td className="px-2 md:px-4 py-3 font-mono text-sm text-muted-foreground text-center border-r border-border">{op.dayOfMonth}</td>
                        <td className="px-2 md:px-4 py-3">
                          <div className="text-foreground font-medium text-sm truncate">{op.description}</div>
                          <div className="text-[11px] text-muted-foreground flex items-center gap-1 mt-0.5 truncate">
                            {op.toAccountId ? (
                              <span className="flex items-center gap-1 text-blue-500 bg-blue-500/10 px-1.5 py-0.5 rounded border border-blue-500/20 truncate">
                                {op.accountName} <ArrowRight size={10} /> {op.toAccountName}
                              </span>
                            ) : (
                              <span className="truncate">{op.accountName}</span>
                            )}
                          </div>
                        </td>
                        <td className={`px-2 md:px-4 py-3 text-right font-mono font-bold text-sm whitespace-nowrap tabular-nums ${op.toAccountId ? 'text-blue-500' : (op.amount > 0 ? 'text-emerald-500' : 'text-foreground')}`}>
                          {op.toAccountId ? '' : (op.amount > 0 ? '+' : '')}{formatMoney(Math.abs(op.amount))}
                        </td>
                        <td className="px-2 py-3 text-right">
                          <div className="flex items-center justify-end gap-1">
                            <button onClick={() => handleEditRecurring(op)} className="p-2 text-muted-foreground hover:text-blue-500 hover:bg-accent rounded-lg transition-colors"><Pencil size={18} /></button>
                            <button onClick={async () => { if (confirm('Supprimer cette opération ?')) { await deleteRecurring(op.id); loadData(); } }} className="p-2 text-muted-foreground hover:text-red-500 hover:bg-accent rounded-lg transition-colors"><Trash2 size={18} /></button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}