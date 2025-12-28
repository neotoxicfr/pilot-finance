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

  if (!data) return <div className="min-h-screen bg-slate-950 flex items-center justify-center text-slate-500">Chargement...</div>;

  const { accounts, projection, totalInterests } = data;
  const totalBalance = accounts.reduce((acc: number, curr: any) => acc + curr.balance, 0);
  const pieData = accounts.filter((a:any) => a.balance > 0).map((a:any) => ({ name: a.name, value: a.balance, color: a.color }));

  return (
    // AJOUT DE 'w-full' et 'flex-1' pour forcer la largeur et l'occupation de l'espace
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8">
      
      <style jsx global>{`
        .recharts-sector:focus, .recharts-wrapper:focus, .recharts-surface:focus { outline: none !important; }
      `}</style>
      
      {/* KPIs */}
      {/* MODIFICATION : Passage à 'sm:grid-cols-2 md:grid-cols-3' pour avoir des colonnes plus tôt */}
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-6">
        <div className="bg-slate-900 border border-slate-800 p-6 rounded-2xl relative overflow-hidden group hover:border-slate-700 transition-colors">
          <div className="absolute top-0 right-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity"><ShieldCheck size={80} /></div>
          <p className="text-xs text-slate-400 font-bold uppercase tracking-wider mb-2">Patrimoine Net</p>
          <h2 className="text-3xl md:text-4xl font-bold text-white font-mono tracking-tight">{formatMoney(totalBalance)}</h2>
        </div>
        <div className="bg-slate-900 border border-slate-800 p-6 rounded-2xl relative overflow-hidden group hover:border-slate-700 transition-colors">
          <div className="absolute top-0 right-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity"><TrendingUp size={80} /></div>
          <p className="text-xs text-slate-400 font-bold uppercase tracking-wider mb-2">Intérêts Composés</p>
          <h2 className="text-3xl md:text-4xl font-bold text-emerald-400 font-mono tracking-tight">+{formatMoney(totalInterests)}</h2>
        </div>
        {/* Ce bloc prendra toute la largeur sur mobile/tablette (col-span-full sur sm si besoin, sinon auto) */}
        <div className="bg-slate-900 border border-slate-800 p-6 rounded-2xl relative overflow-hidden group hover:border-slate-700 transition-colors sm:col-span-2 md:col-span-1">
          <div className="absolute top-0 right-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity"><PiggyBank size={80} /></div>
          <p className="text-xs text-slate-400 font-bold uppercase tracking-wider mb-2">Projection à {sliderValue} ans</p>
          <h2 className="text-3xl md:text-4xl font-bold text-blue-400 font-mono tracking-tight">{formatMoney(projection[projection.length - 1]?.totalAvg)}</h2>
        </div>
      </div>

      {/* GRAPHIQUES */}
      {/* MODIFICATION CRITIQUE : 'md:grid-cols-3' au lieu de 'lg:grid-cols-3'. 
          Cela force l'affichage côte-à-côte dès le format "Tablette/Petit Laptop" au lieu d'attendre les grands écrans */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
        
        {/* GRAPHIQUE TRAJECTOIRE */}
        <div className="md:col-span-2 bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl">
           <div className="flex flex-wrap justify-between items-center mb-6 gap-4">
              <h3 className="text-lg font-bold text-slate-200 flex items-center gap-2"><TrendingUp size={18} className="text-slate-400"/> Trajectoire</h3>
              <div className="flex items-center gap-4 bg-slate-950 px-4 py-2 rounded-xl border border-slate-800 w-full md:w-auto">
                  <span className="text-xs text-slate-400 whitespace-nowrap">Projection : <strong>{sliderValue} ans</strong></span>
                  <input type="range" min="1" max="30" value={sliderValue} onChange={(e) => setSliderValue(parseInt(e.target.value))} className="w-full md:w-48 h-1.5 bg-slate-700 rounded-lg appearance-none cursor-pointer accent-blue-500" />
              </div>
           </div>
           <div className="h-[350px] w-full">
             <ResponsiveContainer width="100%" height="100%">
               <AreaChart data={projection} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
                 <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" vertical={false} />
                 <XAxis dataKey="name" stroke="#64748b" fontSize={11} tickLine={false} axisLine={false} dy={10} />
                 <YAxis stroke="#64748b" fontSize={11} tickLine={false} axisLine={false} tickFormatter={(value) => `${(value / 1000).toFixed(0)}k`} />
                 <RechartsTooltip 
                    contentStyle={{ backgroundColor: '#0f172a', borderColor: '#1e293b', borderRadius: '12px', boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.5)' }} 
                    itemStyle={{ color: '#e2e8f0', fontSize: '12px', fontWeight: '500' }}
                    labelStyle={{ color: '#94a3b8', marginBottom: '8px', fontSize: '12px' }}
                    formatter={(value: any, name: any) => [formatMoney(Number(value) || 0), name === 'totalMin' ? 'Pessimiste' : (name === 'totalMax' ? 'Optimiste' : name)]}
                    filterNull={true}
                 />
                 {accounts.map((acc: any) => (<Area key={acc.id} type="monotone" dataKey={acc.name} stackId="1" stroke={acc.color} fill={acc.color} fillOpacity={0.8} strokeWidth={1}/>))}
                 <Area type="monotone" dataKey="totalMax" stroke="#10b981" strokeDasharray="5 5" strokeWidth={2} fill="transparent" />
                 <Area type="monotone" dataKey="totalMin" stroke="#ef4444" strokeDasharray="5 5" strokeWidth={2} fill="transparent" />
               </AreaChart>
             </ResponsiveContainer>
           </div>
        </div>

        {/* CAMEMBERT REPARTITION */}
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col">
           <h3 className="text-lg font-bold text-slate-200 mb-4 flex items-center gap-2"><LayoutDashboard size={18} className="text-slate-400"/> Répartition</h3>
           <div className="flex-1 min-h-[300px] relative">
             <ResponsiveContainer width="100%" height="100%">
               <PieChart>
                 <Pie data={pieData} cx="50%" cy="50%" innerRadius="55%" outerRadius="80%" paddingAngle={4} dataKey="value" startAngle={90} endAngle={-270} stroke="none">
                   {pieData.map((entry: any, index: number) => (<Cell key={`cell-${index}`} fill={entry.color || COLORS[index % COLORS.length]} className="focus:outline-none hover:opacity-80 transition-opacity" />))}
                 </Pie>
                 <RechartsTooltip formatter={(value: any) => formatMoney(Number(value) || 0)} contentStyle={{ backgroundColor: '#0f172a', borderColor: '#1e293b', borderRadius: '12px' }} itemStyle={{ color: '#e2e8f0', fontSize: '12px' }} />
                 <Legend layout="horizontal" verticalAlign="bottom" align="center" iconType="circle" wrapperStyle={{ fontSize: '11px', color: '#94a3b8', paddingTop: '20px' }} />
               </PieChart>
             </ResponsiveContainer>
             <div className="absolute inset-0 flex items-center justify-center pointer-events-none pb-8">
                <div className="text-center">
                    <div className="text-2xl font-bold text-white tracking-tight">{totalBalance > 10000 ? (totalBalance/1000).toFixed(0)+'k' : totalBalance}</div>
                    <div className="text-[10px] text-slate-500 uppercase font-bold tracking-wider">Total</div>
                </div>
             </div>
           </div>
        </div>
      </div>
    </main>
  );
}