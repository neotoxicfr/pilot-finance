'use client'
export const dynamic = 'force-dynamic';
import { useEffect, useState, lazy, Suspense } from "react";
import { getDashboardData } from "@/src/actions";
import { TrendingUp, PiggyBank, LayoutDashboard, ShieldCheck } from "lucide-react";

// Lazy load chart components for better initial bundle size
const ProjectionChart = lazy(() => import('@/src/components/ProjectionChart'));
const BalancePieChart = lazy(() => import('@/src/components/BalancePieChart'));

// Skeleton component for chart loading state
function ChartSkeleton() {
  return (
    <div className="w-full h-full animate-pulse">
      <div className="h-full w-full bg-accent rounded-xl"></div>
    </div>
  );
}
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
    }, 150);
    return () => clearTimeout(timer);
  }, [sliderValue]);
  if (!data) return <div className="min-h-screen bg-background flex items-center justify-center text-muted-foreground">Chargement...</div>;
  const { accounts, projection, totalInterests } = data;
  const totalBalance = accounts.reduce((acc: number, curr: any) => acc + curr.balance, 0);
  const pieData = accounts.filter((a:any) => a.balance > 0).map((a:any) => ({ name: a.name, value: a.balance, color: a.color }));
  return (
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8">
      {}
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
        {}
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
           <Suspense fallback={<ChartSkeleton />}>
             <ProjectionChart projection={projection} accounts={accounts} formatMoney={formatMoney} />
           </Suspense>
        </div>
        {}
        <div className="dashboard-card bg-background border rounded-2xl p-6 flex flex-col">
           <h3 className="text-lg font-bold text-foreground mb-4 flex items-center gap-2"><LayoutDashboard size={18} className="text-muted-foreground"/> Répartition</h3>
           <Suspense fallback={<ChartSkeleton />}>
             <BalancePieChart pieData={pieData} totalBalance={totalBalance} formatMoney={formatMoney} />
           </Suspense>
        </div>
      </div>
    </main>
  );
}