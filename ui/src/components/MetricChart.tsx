import { useQuery } from '@tanstack/react-query';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts';
import { fetchMetric, type MetricRange, type MetricResponse } from '../api/client';

interface Props {
  name: string;
  range: MetricRange;
}

const COLORS = ['#0f766e', '#b45309', '#7c3aed', '#475569'];

export function MetricChart({ name, range }: Props) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['metric', name, range],
    queryFn: () => fetchMetric(name, range),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return <div className="h-64 flex items-center justify-center text-slate-500 text-sm">Loading…</div>;
  }

  if (error) {
    const msg = (error as Error).message;
    return (
      <div className="h-64 flex items-center justify-center text-center text-sm text-slate-500">
        {msg === 'prometheus-unconfigured'
          ? 'Prometheus is not configured on this dashboard install.'
          : `Couldn't load metric: ${msg}`}
      </div>
    );
  }

  if (!data) return null;

  return <Chart data={data} />;
}

function Chart({ data }: { data: MetricResponse }) {
  // Reshape: merge all series onto one row-per-timestamp object so Recharts
  // can render multiple lines.
  const byTime = new Map<string, Record<string, number | string>>();
  data.series.forEach((s) => {
    s.points.forEach((p) => {
      const key = p.time;
      const row = byTime.get(key) ?? { time: new Date(p.time).getTime() };
      row[s.label] = p.value;
      byTime.set(key, row);
    });
  });
  const rows = Array.from(byTime.values()).sort((a, b) => (a.time as number) - (b.time as number));

  return (
    <div className="h-64">
      <div className="flex items-baseline justify-between mb-1">
        <div>
          <h3 className="font-semibold text-sm">{data.title}</h3>
          <p className="text-xs text-slate-500">{data.description}</p>
        </div>
        <span className="text-xs text-slate-500">{data.unit}</span>
      </div>
      <ResponsiveContainer width="100%" height="85%">
        <LineChart data={rows} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
          <CartesianGrid stroke="#e2e8f0" strokeDasharray="3 3" />
          <XAxis
            dataKey="time"
            type="number"
            domain={['dataMin', 'dataMax']}
            tickFormatter={(t) => new Date(t).toLocaleTimeString()}
            stroke="#94a3b8"
            fontSize={11}
          />
          <YAxis stroke="#94a3b8" fontSize={11} />
          <Tooltip
            labelFormatter={(t) => new Date(t as number).toLocaleString()}
            contentStyle={{ fontSize: 12 }}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          {data.series.map((s, i) => (
            <Line
              key={s.label}
              type="monotone"
              dataKey={s.label}
              stroke={COLORS[i % COLORS.length]}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
