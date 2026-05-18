import { useSearchParams } from 'react-router-dom';
import { MetricChart } from '../components/MetricChart';
import { METRIC_NAMES } from '../resources/metrics';
import type { MetricRange } from '../api/client';

const RANGES: MetricRange[] = ['15m', '1h', '6h', '24h'];

export function MetricsPage() {
  const [params, setParams] = useSearchParams();
  const raw = params.get('range');
  const range: MetricRange = (RANGES as readonly string[]).includes(raw ?? '')
    ? (raw as MetricRange)
    : '1h';

  const setRange = (r: MetricRange) => {
    const next = new URLSearchParams(params);
    next.set('range', r);
    setParams(next, { replace: true });
  };

  return (
    <div className="p-6 space-y-6 max-w-6xl">
      <div className="flex items-baseline justify-between">
        <div>
          <h1 className="font-semibold text-lg">Metrics</h1>
          <p className="text-sm text-slate-500">
            Time-series from the agent-sandbox controller. Refreshes every 30s.
          </p>
        </div>
        <div className="inline-flex rounded border border-slate-300 overflow-hidden text-sm">
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={`px-3 py-1 ${
                r === range ? 'bg-slate-900 text-white' : 'bg-white text-slate-700 hover:bg-slate-100'
              }`}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {METRIC_NAMES.map((name) => (
          <div key={name} className="bg-white rounded-lg shadow p-4">
            <MetricChart name={name} range={range} />
          </div>
        ))}
      </div>
    </div>
  );
}
