import { useQuery } from '@tanstack/react-query';
import { fetchOverview, type OverviewResponse } from '../api/client';
import { Loading } from '../components/Loading';

export function OverviewPage() {
  const { data, isLoading, error } = useQuery<OverviewResponse>({
    queryKey: ['overview'],
    queryFn: fetchOverview,
    refetchInterval: 5000,
  });

  if (isLoading) return <Loading className="p-6" />;
  if (error) return <div className="p-6 text-red-700">Error: {(error as Error).message}</div>;
  if (!data) return null;

  return (
    <div className="p-6 grid grid-cols-2 gap-4 max-w-4xl">
      <Card title="Sandboxes" total={data.sandboxes.total}>
        <Pill label="Ready" value={data.sandboxes.ready} tone="ok" />
        <Pill label="Not Ready" value={data.sandboxes.notReady} tone="warn" />
        <Pill label="Unknown" value={data.sandboxes.unknown} tone="muted" />
      </Card>
      <Card title="Claims" total={data.claims.total}>
        <Pill label="Ready" value={data.claims.ready} tone="ok" />
        <Pill label="Not Ready" value={data.claims.notReady} tone="warn" />
        <Pill label="Unknown" value={data.claims.unknown} tone="muted" />
      </Card>
      <Card title="Templates" total={data.templates.total} />
      <Card title="Warm Pools" total={data.warmPools.total}>
        <Pill label="Replicas" value={data.warmPools.replicas} tone="muted" />
        <Pill label="Ready" value={data.warmPools.readyReplicas} tone="ok" />
      </Card>
    </div>
  );
}

function Card({ title, total, children }: { title: string; total: number; children?: React.ReactNode }) {
  return (
    <div className="bg-white rounded-lg shadow p-4">
      <div className="flex items-baseline justify-between">
        <h2 className="font-semibold text-lg">{title}</h2>
        <span className="text-3xl tabular-nums">{total}</span>
      </div>
      {children && <div className="mt-3 flex gap-2 flex-wrap">{children}</div>}
    </div>
  );
}

function Pill({ label, value, tone }: { label: string; value: number; tone: 'ok' | 'warn' | 'muted' }) {
  const toneCls = {
    ok: 'bg-emerald-100 text-emerald-800',
    warn: 'bg-amber-100 text-amber-800',
    muted: 'bg-slate-100 text-slate-700',
  }[tone];
  return (
    <span className={`px-2 py-1 rounded text-xs ${toneCls}`}>
      {label}: <strong>{value}</strong>
    </span>
  );
}
