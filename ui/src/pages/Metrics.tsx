import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { MetricChart } from '../components/MetricChart';
import { Loading } from '../components/Loading';
import { fetchMetricCatalog, type MetricRange } from '../api/client';

const RANGES: MetricRange[] = ['15m', '1h', '6h', '24h'];

export function MetricsPage() {
  const [params, setParams] = useSearchParams();
  const raw = params.get('range');
  const range: MetricRange = (RANGES as readonly string[]).includes(raw ?? '')
    ? (raw as MetricRange)
    : '1h';

  // The catalog comes from the backend registry, so the page never holds a
  // second copy of the metric list to drift out of sync.
  const catalog = useQuery({
    queryKey: ['metric-catalog'],
    queryFn: fetchMetricCatalog,
    staleTime: 5 * 60_000,
  });

  const setRange = (r: MetricRange) => {
    const next = new URLSearchParams(params);
    next.set('range', r);
    setParams(next, { replace: true });
  };

  return (
    <div className="mx-auto max-w-6xl space-y-8 p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h1 className="font-semibold text-lg">Metrics</h1>
          <p className="text-sm text-slate-500">
            The fleet and its controller over time. Refreshes every 30s.
          </p>
        </div>
        <div className="inline-flex overflow-hidden rounded-lg border border-slate-300 text-sm">
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={`px-3 py-1 ${
                r === range
                  ? 'bg-slate-900 text-white'
                  : 'bg-white text-slate-700 hover:bg-slate-100'
              }`}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      {catalog.isLoading && <Loading />}
      {catalog.error && (
        <p className="text-sm text-red-700">
          Couldn&apos;t load the metric list: {(catalog.error as Error).message}
        </p>
      )}

      {/* One clear statement beats the same sentence on ten cards — and asking
          for charts we know cannot answer only fills the console with 503s. */}
      {catalog.data && !catalog.data.prometheusConfigured && (
        <div className="rounded-xl border border-slate-200 bg-white p-6">
          <h2 className="text-sm font-semibold text-slate-900">Prometheus is not configured</h2>
          <p className="mt-1 max-w-2xl text-sm text-slate-600">
            These charts read from Prometheus, which this install has no URL for. Set one and the
            page fills in — nothing else about the dashboard changes.
          </p>
          <pre className="mt-3 overflow-x-auto rounded-lg bg-slate-50 p-3 text-xs text-slate-700">
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --set prometheus.url=http://prometheus.monitoring.svc:9090</pre>
          <p className="mt-3 text-xs text-slate-500">
            {catalog.data.sections.reduce((n, s) => n + s.metrics.length, 0)} charts are waiting:{' '}
            {catalog.data.sections.map((s) => s.name.toLowerCase()).join(', ')}.
          </p>
        </div>
      )}

      {catalog.data?.prometheusConfigured &&
        catalog.data.sections.map((section) => (
        <section key={section.name} className="space-y-3">
          <div className="border-b border-slate-200 pb-2">
            <h2 className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">
              {section.name}
            </h2>
            {section.note && <p className="mt-1 max-w-3xl text-xs text-slate-400">{section.note}</p>}
          </div>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {section.metrics.map((m) => (
              <div key={m.name} className="rounded-xl border border-slate-200 bg-white p-4">
                <MetricChart name={m.name} range={range} />
              </div>
            ))}
          </div>
        </section>
        ))}
    </div>
  );
}
