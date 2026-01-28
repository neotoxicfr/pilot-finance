'use client'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip as RechartsTooltip, ResponsiveContainer } from 'recharts';

interface ProjectionChartProps {
  projection: any[];
  accounts: any[];
  formatMoney: (amount: number) => string;
}

export default function ProjectionChart({ projection, accounts, formatMoney }: ProjectionChartProps) {
  return (
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
  );
}
