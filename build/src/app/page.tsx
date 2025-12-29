'use client'

export const dynamic = 'force-dynamic';
import { useEffect, useState } from "react";
import { getDashboardData } from "@/src/actions";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip as RechartsTooltip, ResponsiveContainer, PieChart, Pie, Cell, Legend } from 'recharts';
import { TrendingUp, PiggyBank, LayoutDashboard, ShieldCheck } from "lucide-react";

const COLORS = ["#3b82f6", "#8b5cf6", "#10b981", "#06b6d4", "#f59e0b", "#f97316", "#ef4444", "#ec4899"];
const formatMoney = (amount: number) => {
    const decimals = amount % 1 === 0 ? 0 : 2;
    return new Intl.NumberFormat('fr-FR', { 
        style: 'currency', 
        currency: 'EUR', 
        minimumFractionDigits: decimals, 
        maximumFractionDigits: decimals 
    }).format(amount);
};

export default function Dashboard() {
  const [data, setData] = useState<any>(null);
  const [sliderValue, setSliderValue] = useState(5);

  useEffect(() => {
    const timer = setTimeout(() => {
        getDashboardData(sliderValue).then(setData);
    }, 500);
    return () => clearTimeout(timer);
  }, [sliderValue]);

  if (!data) return <div className="min-h-screen bg-background flex items-center justify-center text-muted-foreground">Chargement...</div>;

  const { accounts, projection, totalInterests } = data;
  const totalBalance = accounts.reduce((acc: number, curr: any) => acc + curr.balance, 0);
  const pieData = accounts.filter((a:any) => a.balance > 0).map((a:any) => ({ name: a.name, value: a.balance, color: a.color }));

  return (
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8">
      
      <style jsx global>{`
        .recharts-sector:focus, .recharts-wrapper:focus, .recharts-surface:focus { outline: none !important; }
        .dashboard-card { border-color: var(--border) !important; }
        
        input[type='range']::-webkit-slider-thumb {
          appearance: none;
          width: 18px;
          height: 18px;
          background: #3b82f6;
          border-radius: 50%;
          cursor: pointer;
          border: 2px solid var(--background);
          box-shadow: 0 1px 3px rgba(0,0,0,0.3);
        }
      `}</style>
      
      {/* ... (Le bloc des cartes KPI reste identique, je ne le répète pas pour gagner de la place) ... */}
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-6">
         <div className="dashboard-card bg-background border p-6 rounded-2xl relative overflow-hidden transition-colors">
          <div className="absolute top-0 right-0 p-4 opacity-5 text-foreground"><ShieldCheck size={80} /></div>
          <p className="text-xs text-muted-foreground font-bold uppercase tracking-wider mb-2">Patrimoine Net</p>
          <h2 className="text-3xl md:text-4xl font-bold text-foreground font-mono tracking-tight">{formatMoney(totalBalance)}</h2>
        </div>
        <div className="dashboard-card bg-background border p-6 rounded-2xl relative overflow-hidden transition-colors">
          <div className="absolute top-0 right-0 p-4 opacity-5 text-foreground"><TrendingUp size={80} /></div>
          <p className="text-xs text-muted-foreground font-bold uppercase tracking-wider mb-2">Intérêts Composés</p>
          <h2 className="text-3xl md:text-4xl font-bold text-emerald-500 font-mono tracking-tight">+{formatMoney(totalInterests)}</h2>
        </div>
        <div className="dashboard-card bg-background border p-6 rounded-2xl relative overflow-hidden transition-colors sm:col-span-2 md:col-span-1">
          <div className="absolute top-0 right-0 p-4 opacity-5 text-foreground"><PiggyBank size={80} /></div>
          <p className="text-xs text-muted-foreground font-bold uppercase tracking-wider mb-2">Projection à {sliderValue} ans</p>
          <h2 className="text-3xl md:text-4xl font-bold text-blue-500 font-mono tracking-tight">{formatMoney(projection[projection.length - 1]?.totalAvg)}</h2>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
        {/* GRAPHIQUE AREA CHART (Déjà correct mais je remets pour contexte) */}
        <div className="dashboard-card md:col-span-2 bg-background border rounded-2xl p-6">
           <div className="flex flex-wrap justify-between items-center mb-6 gap-4">
              <h3 className="text-lg font-bold text-foreground flex items-center gap-2"><TrendingUp size={18} className="text-muted-foreground"/> Trajectoire</h3>
              <div className="dashboard-card flex items-center gap-4 bg-accent px-4 py-2 rounded-xl border w-full md:w-auto">
                  <span className="text-xs text-muted-foreground whitespace-nowrap font-medium">Projection : <strong>{sliderValue} ans</strong></span>
                  <input 
                    type="range" 
                    min="1" 
                    max="30" 
                    value={sliderValue} 
                    onChange={(e) => setSliderValue(parseInt(e.target.value))} 
                    className="w-full md:w-48 h-1.5 bg-slate-300 dark:bg-slate-700 rounded-lg appearance-none cursor-pointer accent-blue-600" 
                  />
              </div>
           </div>
           
           <div className="h-[350px] w-full">
             <ResponsiveContainer width="100%" height="100%">
               <AreaChart data={projection} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
                 <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                 <XAxis dataKey="name" stroke="var(--muted-foreground)" fontSize={11} tickLine={false} axisLine={false} dy={10} />
                 <YAxis stroke="var(--muted-foreground)" fontSize={11} tickLine={false} axisLine={false} tickFormatter={(value) => `${(value / 1000).toFixed(0)}k`} />
                 <RechartsTooltip 
                    contentStyle={{ backgroundColor: 'var(--background)', borderColor: 'var(--border)', borderRadius: '12px' }} 
                    itemStyle={{ color: 'var(--foreground)', fontSize: '12px' }}
                    labelStyle={{ color: 'var(--muted-foreground)', fontSize: '12px' }}
                    formatter={(value: any, name: any) => [formatMoney(Number(value) || 0), name === 'totalMin' ? 'Pessimiste' : (name === 'totalMax' ? 'Optimiste' : name)]}
                 />
                 {accounts.map((acc: any) => (
                   <Area key={acc.id} type="monotone" dataKey={acc.name} stackId="1" stroke={acc.color} fill={acc.color} fillOpacity={1} strokeWidth={2} />
                 ))}
                 <Area type="monotone" dataKey="totalMax" stroke="#10b981" strokeDasharray="5 5" strokeWidth={2} fill="transparent" />
                 <Area type="monotone" dataKey="totalMin" stroke="#ef4444" strokeDasharray="5 5" strokeWidth={2} fill="transparent" />
               </AreaChart>
             </ResponsiveContainer>
           </div>
        </div>

        {/* CAMEMBERT (PieChart) - CORRECTION ICI */}
        <div className="dashboard-card bg-background border rounded-2xl p-6 flex flex-col">
           <h3 className="text-lg font-bold text-foreground mb-4 flex items-center gap-2"><LayoutDashboard size={18} className="text-muted-foreground"/> Répartition</h3>
           <div className="flex-1 min-h-[300px] relative">
             <ResponsiveContainer width="100%" height="100%">
               <PieChart>
                 <Pie data={pieData} cx="50%" cy="50%" innerRadius="60%" outerRadius="85%" paddingAngle={4} dataKey="value" startAngle={90} endAngle={-270} stroke="none">
                   {pieData.map((entry: any, index: number) => (
                     <Cell key={`cell-${index}`} fill={entry.color || COLORS[index % COLORS.length]} />
                   ))}
                 </Pie>
                 {/* AJOUT DE itemStyle POUR CORRIGER LA COULEUR DU TEXTE */}
                 <RechartsTooltip 
                    formatter={(value: any) => formatMoney(Number(value) || 0)} 
                    contentStyle={{ backgroundColor: 'var(--background)', borderColor: 'var(--border)', borderRadius: '12px' }}
                    itemStyle={{ color: 'var(--foreground)', fontWeight: 'bold' }} 
                 />
                 <Legend verticalAlign="bottom" align="center" iconType="circle" wrapperStyle={{ fontSize: '11px', color: 'var(--muted-foreground)', paddingTop: '20px' }} />
               </PieChart>
             </ResponsiveContainer>
             <div className="absolute inset-0 flex items-center justify-center pointer-events-none pb-8">
                <div className="text-center">
                    <div className="text-2xl font-bold text-foreground tracking-tight">{totalBalance > 10000 ? (totalBalance/1000).toFixed(0)+'k' : totalBalance}</div>
                    <div className="text-[10px] text-muted-foreground uppercase font-bold tracking-wider">Total</div>
                </div>
             </div>
           </div>
        </div>
      </div>
    </main>
  );
}