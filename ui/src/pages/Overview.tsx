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
import { shortId, taskLabel } from '../list/rows';
import { useRefreshInterval } from '../api/refresh';
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
  DONUT_MAX,
  podPhases,
  reserved,
  used,
  valueCounts,
  valueOf,
  STATUS,
} from '../overview/aggregate';

export function OverviewPage() {
  const [params, setParams] = useSearchParams();
  const refetchInterval = useRefreshInterval();
  const slowRefetch = useRefreshInterval(6);

  const counts = useQuery<OverviewResponse>({
    queryKey: ['overview'],
    queryFn: fetchOverview,
    refetchInterval,
  });

  // Same key the sandbox list page uses, so the two share one fetch.
  const fleet = useQuery({
    queryKey: ['list', 'sandboxes', false],
    queryFn: () => fetchList('sandboxes'),
    refetchInterval,
  });

  // Prometheus-backed, so it refreshes on the metrics page's slower cadence and
  // fails on its own without touching anything else on the page.
  const usage = useQuery<UsageResponse>({
    queryKey: ['usage'],
    queryFn: fetchUsage,
    refetchInterval: slowRefetch,
    retry: false,
  });

  // Memoised so the rollups below are not invalidated on every render while
  // the list is still loading.
  const items = useMemo(() => fleet.data?.items ?? [], [fleet.data]);

  // Discovered from the whole fleet, never from the filtered subset: a filter
  // narrows to one value, which would make its own dimension stop dividing
  // anything and drop out of the list the filter is cleared from.
  const dimensions = useMemo(() => dimensionsFor(items), [items]);
  const dimension =
    dimensions.find((d) => d.key === params.get('by')) ?? dimensions[0];
  const selected = params.get('v') ?? undefined;

  /**
   * Every panel below reads this instead of the full fleet. The group panel is
   * the exception — it stays the switcher, so it keeps showing the whole
   * breakdown with the picked slice lit.
   */
  const scope = useMemo(
    () =>
      selected && dimension ? items.filter((it) => valueOf(it, dimension) === selected) : items,
    [items, dimension, selected],
  );
  const values = useMemo(() => (dimension ? valueCounts(items, dimension) : []), [items, dimension]);

  const view = useMemo(
    () => ({
      // A dimension that divides the fleet into a handful of parts reads as a
      // ring; a long tail — sixty images — reads as a ranked list.
      groups: dimension ? groupBy(items, dimension, 5, usage.data) : [],
      asRing: !!dimension && dimension.parts <= DONUT_MAX,
      footprint: dimension ? groupBy(scope, dimension, 5, usage.data) : [],
      phases: podPhases(scope),
      buckets: ageBuckets(scope),
      oldest: longestRunning(scope),
      reserved: reserved(scope),
      used: used(scope, usage.data),
      triage: alerts(scope),
      // The same readiness rule the server applies, over the list the rest of
      // this page is drawn from — not /api/v1/overview, which is a second poll
      // of the same truth and put "Ready 19 · Not ready 0" beside a strip
      // flagging one not ready, in one paint.
      //
      // Over the scope, like everything else: the card's headline and its own
      // breakdown come from one list, so they always add up, and the chip below
      // says what the whole fleet is.
      fleet: scope.reduce(
        (acc, it) => {
          if (it.phase === 'Ready') acc.ready += 1;
          else if (it.phase === 'NotReady') acc.notReady += 1;
          else acc.unknown += 1;
          return acc;
        },
        { ready: 0, notReady: 0, unknown: 0 },
      ),
    }),
    [items, scope, dimension, usage.data],
  );

  const setDimension = (key: string) => {
    const next = new URLSearchParams(params);
    next.set('by', key);
    // A value picked from the old dimension does not exist in the new one, so
    // keeping it would filter the page down to nothing.
    next.delete('v');
    setParams(next, { replace: true });
  };

  /** Clicking the picked slice again clears it — the same control both ways. */
  const toggleValue = (value: string) => {
    const next = new URLSearchParams(params);
    if (value === selected) next.delete('v');
    else next.set('v', value);
    setParams(next, { replace: true });
  };

  const clearValue = () => {
    const next = new URLSearchParams(params);
    next.delete('v');
    setParams(next, { replace: true });
  };

  if (counts.isLoading || fleet.isLoading) return <Loading className="p-6" />;
  if (counts.error) {
    return <div className="p-6 text-red-700">Error: {(counts.error as Error).message}</div>;
  }
  if (!counts.data) return null;

  const data = counts.data;
  const showGpu = view.reserved.gpu > 0;

  const control =
    'rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900';

  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      {/*
        One pair of controls for the whole page, above everything they scope:
        pick how the fleet divides, then which part of it to look at. The group
        panel further down offers the same choice by clicking a slice, but only
        for the largest five.
      */}
      {dimensions.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-600">
          <label className="flex items-center gap-2">
            Group by
            <select
              value={dimension?.key}
              onChange={(e) => setDimension(e.target.value)}
              className={control}
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
          <label className="flex items-center gap-2">
            Show
            <select
              value={selected ?? ''}
              onChange={(e) => (e.target.value ? toggleValue(e.target.value) : clearValue())}
              className={control}
              aria-label={`Which ${dimension?.label.toLowerCase() ?? 'group'} to show`}
            >
              <option value="">whole fleet ({items.length})</option>
              {values.map((v) => (
                <option key={v.value} value={v.value}>
                  {v.value} ({v.count})
                </option>
              ))}
            </select>
          </label>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {/*
          Counted from the same list every panel below is drawn from, not from
          /api/v1/overview. Two independent five-second polls of the same truth
          put "Ready 19 · Not ready 0" next to a strip flagging one not ready, in
          one paint — and a page that contradicts itself is not trusted again.
        */}
        <StatCard
          label="Sandboxes"
          value={scope.length}
          to="/sandboxes"
          parts={
            scope.length
              ? [
                  { label: 'Ready', value: view.fleet.ready, color: STATUS.ready },
                  { label: 'Not ready', value: view.fleet.notReady, color: STATUS.failed },
                  { label: 'Unknown', value: view.fleet.unknown, color: STATUS.idle },
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

      {/*
        A narrowed install counts a partial fleet and the cards above cannot say
        so — the scope only ever appeared in the startup log. One line, under the
        counts it qualifies.
      */}
      {data.scope && (
        <p className="text-xs text-slate-400">
          Watching {data.scope.namespaces.length === 1 ? 'namespace' : 'namespaces'}{' '}
          <span className="font-mono">{data.scope.namespaces.join(', ')}</span> only — sandboxes
          elsewhere in the cluster are not counted above.
        </p>
      )}

      {/*
        Says what the rest of the page is now about, and is the only way back —
        so it sits above everything it scopes rather than beside the chart that
        set it.
      */}
      {selected && dimension && (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-blue-200 bg-blue-50/60 px-4 py-2.5 text-sm">
          <span className="text-slate-600">
            Showing {dimension.label.toLowerCase()}{' '}
            <strong className="font-semibold text-slate-900">{selected}</strong> —{' '}
            <span className="tabular-nums">{scope.length}</span> of {items.length} sandboxes
          </span>
          {/*
            Only for a stamped label, whose value the list's search can actually
            match. "1 core" or "Running" would hand over a search that finds
            nothing, and a link that lies is worse than no link.
          */}
          {dimension.key.startsWith('label:') && (
            <Link
              to={`/sandboxes?q=${encodeURIComponent(selected)}`}
              className="text-blue-800 underline decoration-blue-300 hover:decoration-blue-800"
            >
              open in list
            </Link>
          )}
          <button
            type="button"
            onClick={clearValue}
            className="ml-auto rounded border border-slate-300 bg-white px-2 py-0.5 text-xs text-slate-600 hover:bg-slate-50"
          >
            Show whole fleet
          </button>
        </div>
      )}

      {!scope.length ? (
        <Panel title="Fleet health">
          <div className="py-6 text-center">
            {/* The fleet churns fast enough that a group can empty out while
                its filter is still on. Say which of the two happened. */}
            {selected ? (
              <p className="text-sm text-slate-600">
                Nothing in this group any more — every sandbox in it has gone. Clear the filter
                above to see the rest of the fleet.
              </p>
            ) : (
              <>
                <p className="text-sm text-slate-600">
                  No sandboxes in this cluster yet — nothing to chart.
                </p>
                <p className="mx-auto mt-2 max-w-xl text-xs text-slate-500">
                  Fleet health, reservations against live use, share and runtime breakdowns and the
                  resource footprint all appear here as soon as the first Sandbox is created. If you
                  expected some, check you are pointed at the right cluster and that the dashboard
                  can list sandboxes across namespaces.
                </p>
              </>
            )}
          </div>
        </Panel>
      ) : (
        <>
      <Panel
        title="Fleet health"
        hint={`${scope.length} ${scope.length === 1 ? 'sandbox' : 'sandboxes'}, oldest first`}
      >
        <TriageChips alerts={view.triage} />
        <div className="mt-4">
          <FleetStrip items={scope} />
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

      <h2 className="pt-2 text-sm font-semibold text-slate-700">How the fleet divides</h2>

      <div className="grid items-start gap-4 lg:grid-cols-2">
        <Panel title="Pod status">
          <Donut slices={view.phases} total={scope.length} />
        </Panel>
        <Panel
          title={dimension ? `By ${dimension.label.toLowerCase()}` : 'By group'}
          hint={
            dimension
              ? [
                  'Select a group to scope this page to it',
                  !view.asRing && `largest 5 of ${dimension.parts}, rest folded into Other`,
                ]
                  .filter(Boolean)
                  .join(' · ')
              : undefined
          }
        >
          {!dimension ? (
            <PanelNote>
              No label or field divides this fleet yet — every sandbox shares one value, or each
              carries its own.
            </PanelNote>
          ) : view.asRing ? (
            <Donut
              slices={view.groups}
              total={items.length}
              selected={selected}
              onSelect={toggleValue}
            />
          ) : (
            <ShareList
              slices={view.groups}
              total={items.length}
              selected={selected}
              onSelect={toggleValue}
            />
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
          <FootprintBars slices={view.footprint} showGpu={showGpu} />
        </Panel>
      )}
        </>
      )}
    </div>
  );
}
