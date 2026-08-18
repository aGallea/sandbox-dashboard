import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchList,
  fetchOverview,
  fetchUsage,
  type OverviewResponse,
  type UsageResponse,
} from '../api/client';
import { Loading } from '../components/Loading';
import { FleetStrip } from '../components/overview/FleetStrip';
import {
  AgeHistogram,
  Donut,
  FootprintBars,
  Meter,
  ShareList,
} from '../components/overview/Charts';
import { Panel, PanelNote, StatCard, TriageChips } from '../components/overview/Panels';
import {
  ageBuckets,
  alerts,
  dimensionsFor,
  formatAge,
  formatBytes,
  formatCores,
  coreLabel,
  groupBy,
  longestRunning,
  shortImage,
  DONUT_MAX,
  podPhases,
  reserved,
  used,
  STATUS,
} from '../overview/aggregate';

export function OverviewPage() {
  const [params, setParams] = useSearchParams();

  const counts = useQuery<OverviewResponse>({
    queryKey: ['overview'],
    queryFn: fetchOverview,
    refetchInterval: 5_000,
  });

  // Same key the sandbox list page uses, so the two share one fetch.
  const fleet = useQuery({
    queryKey: ['list', 'sandboxes', false],
    queryFn: () => fetchList('sandboxes'),
    refetchInterval: 5_000,
  });

  // Prometheus-backed, so it refreshes on the metrics page's slower cadence and
  // fails on its own without touching anything else on the page.
  const usage = useQuery<UsageResponse>({
    queryKey: ['usage'],
    queryFn: fetchUsage,
    refetchInterval: 30_000,
    retry: false,
  });

  // Memoised so the rollups below are not invalidated on every render while
  // the list is still loading.
  const items = useMemo(() => fleet.data?.items ?? [], [fleet.data]);

  const dimensions = useMemo(() => dimensionsFor(items), [items]);
  const dimension =
    dimensions.find((d) => d.key === params.get('by')) ?? dimensions[0];

  const view = useMemo(
    () => ({
      // A dimension that divides the fleet into a handful of parts reads as a
      // ring; a long tail — sixty images — reads as a ranked list.
      groups: dimension ? groupBy(items, dimension, 5, usage.data) : [],
      asRing: !!dimension && dimension.parts <= DONUT_MAX,
      phases: podPhases(items),
      buckets: ageBuckets(items),
      oldest: longestRunning(items),
      reserved: reserved(items),
      used: used(items, usage.data),
      triage: alerts(items),
    }),
    [items, dimension, usage.data],
  );

  const setDimension = (key: string) => {
    const next = new URLSearchParams(params);
    next.set('by', key);
    setParams(next, { replace: true });
  };

  if (counts.isLoading || fleet.isLoading) return <Loading className="p-6" />;
  if (counts.error) {
    return <div className="p-6 text-red-700">Error: {(counts.error as Error).message}</div>;
  }
  if (!counts.data) return null;

  const data = counts.data;
  const showGpu = view.reserved.gpu > 0;

  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Sandboxes"
          value={data.sandboxes.total}
          to="/sandboxes"
          parts={
            data.sandboxes.total
              ? [
                  { label: 'Ready', value: data.sandboxes.ready, color: STATUS.ready },
                  { label: 'Not ready', value: data.sandboxes.notReady, color: STATUS.failed },
                  { label: 'Unknown', value: data.sandboxes.unknown, color: STATUS.idle },
                ]
              : undefined
          }
        />
        <StatCard
          label="Claims"
          value={data.claims.total}
          to="/claims"
          parts={
            data.claims.total
              ? [
                  { label: 'Ready', value: data.claims.ready, color: STATUS.ready },
                  { label: 'Not ready', value: data.claims.notReady, color: STATUS.failed },
                  { label: 'Unknown', value: data.claims.unknown, color: STATUS.idle },
                ]
              : undefined
          }
        />
        <StatCard label="Templates" value={data.templates.total} to="/templates" />
        <StatCard
          label="Warm pools"
          value={data.warmPools.total}
          to="/warmpools"
          parts={
            data.warmPools.total
              ? [
                  {
                    label: 'Replicas ready',
                    value: data.warmPools.readyReplicas,
                    color: STATUS.ready,
                  },
                  { label: 'Desired', value: data.warmPools.replicas, color: STATUS.idle },
                ]
              : undefined
          }
        />
      </div>

      <Panel title="Fleet health" hint={`${items.length} sandboxes, oldest first`}>
        <TriageChips alerts={view.triage} />
        <div className="mt-4">
          <FleetStrip items={items} />
        </div>
      </Panel>

      <Panel
        title="Reserved against used"
        hint="Live use from Prometheus, against what the pods reserve"
      >
        {usage.isError ? (
          <PanelNote>
            {(usage.error as Error).message === 'prometheus-unconfigured'
              ? 'Set PROMETHEUS_URL on the dashboard to compare reservations against live use.'
              : `Couldn't read live usage: ${(usage.error as Error).message}`}
          </PanelNote>
        ) : !view.used ? (
          <Loading label="Reading live usage…" />
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            <Meter
              label="CPU"
              reserved={view.reserved.cpuMillis / 1000}
              used={view.used.cpuCores}
              format={(v) => `${v < 10 ? v.toFixed(2) : Math.round(v)} cores`}
            />
            <Meter
              label="Memory"
              reserved={view.reserved.memBytes}
              used={view.used.memBytes}
              format={formatBytes}
            />
          </div>
        )}
      </Panel>

      {/* One control for every panel it scopes, rather than a selector per card. */}
      <div className="flex flex-wrap items-baseline justify-between gap-3 pt-2">
        <h2 className="text-sm font-semibold text-slate-700">How the fleet divides</h2>
        {dimensions.length > 0 && (
          <label className="flex items-center gap-2 text-sm text-slate-600">
            Group by
            <select
              value={dimension?.key}
              onChange={(e) => setDimension(e.target.value)}
              className="rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900"
            >
              {dimensions.map((d) => (
                <option key={d.key} value={d.key}>
                  {d.label}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <Panel title="Pod status">
          <Donut slices={view.phases} total={items.length} />
        </Panel>
        <Panel
          title={dimension ? `By ${dimension.label.toLowerCase()}` : 'By group'}
          hint={
            dimension && !view.asRing
              ? `Largest 5 of ${dimension.parts}, rest folded into Other`
              : undefined
          }
        >
          {!dimension ? (
            <PanelNote>
              No label or field divides this fleet yet — every sandbox shares one value, or each
              carries its own.
            </PanelNote>
          ) : view.asRing ? (
            <Donut slices={view.groups} total={items.length} />
          ) : (
            <ShareList slices={view.groups} total={items.length} />
          )}
        </Panel>
      </div>

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <Panel title="How long they run" hint="Time since the sandbox was created">
          <AgeHistogram buckets={view.buckets} />
        </Panel>
        <Panel title="Longest running">
          <ul className="divide-y divide-slate-100">
            {view.oldest.map((it) => (
              <li key={`${it.namespace}/${it.name}`} className="py-2 first:pt-0 last:pb-0">
                <Link
                  to={`/sandboxes/${it.namespace}/${it.name}`}
                  title={it.name}
                  className="flex items-baseline justify-between gap-3 text-sm hover:underline"
                >
                  <span className="min-w-0 truncate text-slate-700">
                    {shortImage(it.pod?.image) || it.name}
                    <span className="ml-2 text-slate-400">
                      {it.name.replace(/^sandbox-/, '').slice(0, 8)}
                    </span>
                  </span>
                  <span className="shrink-0 tabular-nums text-slate-500">
                    {formatAge(it.ageSeconds)}
                    {it.pod && ` · ${coreLabel(it.pod.cpuMillis)}`}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Panel>
      </div>

      {dimension && (
        <Panel
          title="Resource footprint"
          hint={`Reserved by ${dimension.label.toLowerCase()}, largest first`}
          right={
            <span className="text-sm tabular-nums text-slate-500">
              {formatCores(view.reserved.cpuMillis)} cores ·{' '}
              {formatBytes(view.reserved.memBytes)}
              {showGpu && ` · ${view.reserved.gpu} GPU`}
            </span>
          }
        >
          <FootprintBars slices={view.groups} showGpu={showGpu} />
        </Panel>
      )}
    </div>
  );
}
