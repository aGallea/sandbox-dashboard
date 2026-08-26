import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchList,
  fetchOverview,
  fetchUsage,
  type OverviewResponse,
  type ResourceSummary,
  type UsageResponse,
} from '../api/client';
import { shortId, taskLabel } from '../list/rows';
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
  OTHER_KEY,
  ageBuckets,
  alerts,
  dimensionsFor,
  formatAge,
  formatBytes,
  formatCores,
  coreLabel,
  groupBy,
  longestRunning,
  DONUT_MAX,
  podPhases,
  reserved,
  used,
  STATUS,
  type Dimension,
} from '../overview/aggregate';

/**
 * The one sentence worth reading after the count: what this fleet mostly is.
 *
 * Built from whichever dimension the fleet itself makes meaningful, so a cluster
 * stamping `experiment` reads differently from one stamping `team`, and a cluster
 * that stamps nothing gets no sentence rather than a filler one.
 */
function lead(items: ResourceSummary[], dimension?: Dimension): string {
  // Only for a label the fleet stamps. "Most of it is one cpu request: 1 core"
  // is a sentence about the dashboard's own vocabulary, not about the fleet, and
  // silence beats filler.
  if (!items.length || !dimension?.key.startsWith('label:')) return '';
  const top = groupBy(items, dimension, 1)[0];
  if (!top || top.key === OTHER_KEY || top.key === 'unset') return '';

  const what = dimension.label.toLowerCase();
  const value = top.key.length > 48 ? `${top.key.slice(0, 47)}…` : top.key;
  const most =
    top.count > items.length / 2
      ? `Most of it — ${top.count} — is one ${what}: ${value}.`
      : `Its largest ${what} is ${value}, with ${top.count}.`;

  const missing = items.length - dimension.covered;
  return missing > 0 ? `${most} ${missing} carry no ${what} label.` : most;
}

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
      nodes: new Set(items.map((it) => it.pod?.node).filter(Boolean)).size,
      lead: lead(items, dimension),
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
  const otherKinds = data.claims.total + data.templates.total + data.warmPools.total;

  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      {/*
        The fleet count comes from the same list every panel below is drawn from,
        not from /api/v1/overview. Two independent five-second polls of the same
        truth put "Ready 19 · Not ready 0" next to a strip flagging one not
        ready, in one render — and a page that contradicts itself is not trusted
        again.
      */}
      <header>
        <h1 className="text-2xl text-slate-900">
          {items.length === 0
            ? 'No sandboxes in scope'
            : `${items.length} ${items.length === 1 ? 'sandbox' : 'sandboxes'}${
                view.nodes ? ` on ${view.nodes} ${view.nodes === 1 ? 'node' : 'nodes'}` : ''
              }`}
        </h1>
        {view.lead && <p className="mt-1 text-sm text-slate-500">{view.lead}</p>}
        {/* A narrowed install counts a partial fleet; only the log said so. */}
        {data.scope && (
          <p className="mt-1 text-xs text-slate-400">
            Watching {data.scope.namespaces.length === 1 ? 'namespace' : 'namespaces'}{' '}
            <span className="font-mono">{data.scope.namespaces.join(', ')}</span> only — sandboxes
            elsewhere in the cluster are not counted here.
          </p>
        )}
      </header>

      {/* Four equal-weight cards, three of them reading zero, spend the top of
          the page on kinds this cluster does not use. */}
      {otherKinds === 0 ? (
        <p className="rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm text-slate-500">
          No claims, templates or warm pools in scope.
        </p>
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
          {data.claims.total > 0 && (
            <StatCard
              label="Claims"
              value={data.claims.total}
              to="/claims"
              parts={[
                { label: 'Ready', value: data.claims.ready, color: STATUS.ready },
                { label: 'Not ready', value: data.claims.notReady, color: STATUS.failed },
                { label: 'Unknown', value: data.claims.unknown, color: STATUS.idle },
              ]}
            />
          )}
          {data.templates.total > 0 && (
            <StatCard label="Templates" value={data.templates.total} to="/templates" />
          )}
          {data.warmPools.total > 0 && (
            <StatCard
              label="Warm pools"
              value={data.warmPools.total}
              to="/warmpools"
              parts={[
                {
                  label: 'Replicas ready',
                  value: data.warmPools.readyReplicas,
                  color: STATUS.ready,
                },
                { label: 'Desired', value: data.warmPools.replicas, color: STATUS.idle },
              ]}
            />
          )}
        </div>
      )}

      {!items.length ? (
        <Panel title="Fleet health">
          <div className="py-6 text-center">
            <p className="text-sm text-slate-600">
              No sandboxes in this cluster yet — nothing to chart.
            </p>
            <p className="mx-auto mt-2 max-w-xl text-xs text-slate-500">
              Fleet health, reservations against live use, share and runtime breakdowns and the
              resource footprint all appear here as soon as the first Sandbox is created. If you
              expected some, check you are pointed at the right cluster and that the dashboard can
              list sandboxes across namespaces.
            </p>
          </div>
        </Panel>
      ) : (
        <>
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
                  {/* Say what a dimension misses before it is picked, not after. */}
                  {d.label}
                  {d.covered < items.length ? ` (${d.covered} of ${items.length})` : ''}
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
                  {/*
                    Two spans separated by a margin read as one word to a screen
                    reader — "busybox:1.36demo-alp". The separator has to be a
                    character, not spacing. Names match the list page's, so the
                    same sandbox is called the same thing on both.
                  */}
                  <span className="min-w-0 truncate text-slate-700">
                    {taskLabel(it).label}
                    {shortId(it.name) && (
                      <span className="text-slate-400"> · {shortId(it.name)}</span>
                    )}
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
        </>
      )}
    </div>
  );
}
