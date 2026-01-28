'use client'
import { PieChart, Pie, Cell, Legend, Tooltip as RechartsTooltip, ResponsiveContainer } from 'recharts';

const COLORS = ["#3b82f6", "#8b5cf6", "#10b981", "#06b6d4", "#f59e0b", "#f97316", "#ef4444", "#ec4899"];

interface BalancePieChartProps {
  pieData: any[];
  totalBalance: number;
  formatMoney: (amount: number) => string;
}

export default function BalancePieChart({ pieData, totalBalance, formatMoney }: BalancePieChartProps) {
  return (
    <div className="flex-1 min-h-[300px] relative">
      <ResponsiveContainer width="100%" height="100%">
        <PieChart>
          <Pie data={pieData} cx="50%" cy="50%" innerRadius="60%" outerRadius="85%" paddingAngle={4} dataKey="value" startAngle={90} endAngle={-270} stroke="none">
            {pieData.map((entry: any, index: number) => (
              <Cell key={`cell-${index}`} fill={entry.color || COLORS[index % COLORS.length]} />
            ))}
          </Pie>
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
          <div className="text-2xl font-bold text-foreground tracking-tight">{totalBalance > 10000 ? (totalBalance / 1000).toFixed(0) + 'k' : totalBalance}</div>
          <div className="text-[10px] text-muted-foreground uppercase font-bold tracking-wider">Total</div>
        </div>
      </div>
    </div>
  );
}
