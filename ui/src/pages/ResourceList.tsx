import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMemo } from 'react';
import { fetchList, fetchUsage } from '../api/client';
import { RESOURCES } from '../resources/config';
import { DetailDrawer } from '../components/DetailDrawer';
import { Loading } from '../components/Loading';
import { ColumnHeader, Pager, type SortDir } from '../components/TableControls';
import { formatAge, formatBytes } from '../overview/aggregate';
import { informative, matchesQuery, podSample, shortId, shortNode, taskLabel } from '../list/rows';
import { useRefreshInterval } from '../api/refresh';
import type { ResourceKind, ResourceSummary, UsageResponse } from '../api/client';

interface Props {
  kind: ResourceKind;
}

// Columns worth picking values from a list. The task label is near-unique per
// row and Age is a number that moves every refresh, so neither makes a useful
// value filter.
const FILTERABLE = ['namespace', 'phase', 'creator', 'owner', 'node'];

const paramFor = (col: string) => `f_${col}`;

const PAGE_SIZES = [10, 25, 50, 100, 200];
const DEFAULT_PAGE_SIZE = 25;

export function ResourceListPage({ kind }: Props) {
  const cfg = RESOURCES[kind];
  const navigate = useNavigate();
  const params = useParams<{ namespace?: string; name?: string }>();
  const drawerOpen = !!(params.namespace && params.name);
  const refetchInterval = useRefreshInterval();
  const slowRefetch = useRefreshInterval(6);

  const [searchParams, setSearchParams] = useSearchParams();
  const staleOnly = searchParams.get('stale') === 'true';
  const query = searchParams.get('q') ?? '';
  const sortKey = searchParams.get('sort') ?? 'age';
  const sortDir: SortDir = searchParams.get('dir') === 'desc' ? 'desc' : 'asc';
  const search = searchParams.toString() ? `?${searchParams.toString()}` : '';

  // Column filters are client-side: the list is one unpaginated fetch, so
  // filtering it here costs nothing and keeps every column's value list built
  // from the same response the table is showing.
  const filters = useMemo(() => {
    const active = new Map<string, Set<string>>();
    FILTERABLE.forEach((col) => {
      const values = searchParams.getAll(paramFor(col));
      if (values.length) active.set(col, new Set(values));
    });
    return active;
  }, [searchParams]);

  const { data, isLoading, error } = useQuery({
    queryKey: ['list', kind, staleOnly],
    queryFn: () => fetchList(kind, { stale: staleOnly || undefined }),
    refetchInterval,
  });

  // Same key the overview uses, so the two share one fetch. `retry: false`
  // because a 503 here means Prometheus was never configured — the usage columns
  // simply do not appear, and nothing on the page reports an error for it.
  const usage = useQuery<UsageResponse>({
    queryKey: ['usage'],
    queryFn: fetchUsage,
    refetchInterval: slowRefetch,
    retry: false,
    enabled: cfg.showOsb,
  });

  // One accessor per column, shared by sorting and value filtering so a column
  // can never sort by one thing and filter by another. Rebuilt when usage
  // arrives, which is what lets the CPU and memory columns sort by live load.
  const columnValue = useMemo<Record<string, (it: ResourceSummary) => string | number>>(
    () => ({
      name: (it) => it.name,
      task: (it) => taskLabel(it).label,
      namespace: (it) => it.namespace,
      phase: (it) => it.phase,
      creator: (it) => it.creator ?? '',
      sessionId: (it) => it.sessionId ?? '',
      owner: (it) => it.owner ?? '',
      node: (it) => it.pod?.node ?? '',
      cpu: (it) => podSample(it, usage.data)?.cpuCores ?? -1,
      mem: (it) => podSample(it, usage.data)?.memBytes ?? -1,
      age: (it) => it.ageSeconds,
    }),
    [usage.data],
  );

  // Value lists come from the whole response, not the filtered rows, so a
  // selection never removes the options next to it.
  const options = useMemo(() => {
    const byColumn: Record<string, string[]> = {};
    FILTERABLE.forEach((col) => {
      const seen = new Set<string>();
      data?.items.forEach((it) => {
        const value = String(columnValue[col](it) ?? '');
        if (value) seen.add(value);
      });
      byColumn[col] = Array.from(seen).sort();
    });
    return byColumn;
  }, [data, columnValue]);

  const rows = useMemo(() => {
    const value = columnValue[sortKey] ?? columnValue.age;
    const items = (data?.items ?? [])
      .filter((it) => matchesQuery(it, query))
      .filter((it) =>
        Array.from(filters).every(([col, wanted]) =>
          wanted.has(String(columnValue[col](it) ?? '')),
        ),
      );
    items.sort((a, b) => {
      const x = value(a);
      const y = value(b);
      const cmp =
        typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y));
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return items;
  }, [data, filters, query, sortKey, sortDir, columnValue]);

  const pageSize = PAGE_SIZES.includes(Number(searchParams.get('size')))
    ? Number(searchParams.get('size'))
    : DEFAULT_PAGE_SIZE;
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  // Clamped rather than written back: a filter that shrinks the result should
  // land you on the last page, not on an empty one, and not rewrite your URL.
  const page = Math.min(Math.max(1, Number(searchParams.get('page')) || 1), pageCount);
  const visible = rows.slice((page - 1) * pageSize, page * pageSize);

  // Owner/team/experiment are optional labels the eval harness stamps only for
  // some workloads; a column of nothing but em-dashes is worse than no column.
  // One owner across the whole fleet is still worth a column — unlike the
  // columns below, nothing else on the row says who it belongs to.
  const showOwner = cfg.showOsb && options.owner?.length > 0;

  // A column whose every row says `default` costs width and teaches nothing.
  // Both come back on their own once the fleet spans more than one value.
  const items = data?.items ?? [];
  const showNamespace = informative(items, (it) => it.namespace);
  const showCreator = cfg.showOsb && informative(items, (it) => it.creator ?? '');
  const showNode = cfg.showOsb && options.node?.length > 0;

  // Live usage needs Prometheus. Gated on the query succeeding rather than on a
  // configuration flag, so the columns are present exactly when they have values.
  const showUsage = cfg.showOsb && usage.isSuccess;

  // The response omits `osb` entirely when no OPENSANDBOX_URL is set, which is
  // how the UI can tell "not configured" from "configured but unreachable".
  // OpenSandbox state appears in the Status cell only for the former — the same
  // em-dash rule as Owner above.
  const osbConfigured = cfg.showOsb && !!data?.osb;

  // Stale is computed by OpenSandbox, so a filtered empty list means "could not
  // compute", not "nothing matched". Gated on cfg.showOsb because the other
  // resource kinds never return an osb field, which would otherwise make this
  // true for them and hide their rows.
  const osbFilterUnsatisfiable =
    cfg.showOsb && staleOnly && !!data && data.osb?.status !== 'ok';

  // Anything that changes which rows exist sends you back to page 1; paging
  // itself is the one caller that keeps it.
  const commit = (next: URLSearchParams, keepPage = false) => {
    if (!keepPage) next.delete('page');
    setSearchParams(next, { replace: true });
  };

  const setQuery = (text: string) => {
    const next = new URLSearchParams(searchParams);
    if (text) next.set('q', text);
    else next.delete('q');
    commit(next);
  };

  const setStale = (on: boolean) => {
    const next = new URLSearchParams(searchParams);
    if (on) next.set('stale', 'true');
    else next.delete('stale');
    commit(next);
  };

  const setPage = (to: number) => {
    const next = new URLSearchParams(searchParams);
    if (to <= 1) next.delete('page');
    else next.set('page', String(to));
    commit(next, true);
  };

  const setPageSize = (size: number) => {
    const next = new URLSearchParams(searchParams);
    if (size === DEFAULT_PAGE_SIZE) next.delete('size');
    else next.set('size', String(size));
    commit(next);
  };

  const toggleFilter = (col: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    const selected = new Set(next.getAll(paramFor(col)));
    if (selected.has(value)) selected.delete(value);
    else selected.add(value);
    next.delete(paramFor(col));
    selected.forEach((v) => next.append(paramFor(col), v));
    commit(next);
  };

  const clearFilter = (col: string) => {
    const next = new URLSearchParams(searchParams);
    next.delete(paramFor(col));
    commit(next);
  };

  const clearAllFilters = () => {
    const next = new URLSearchParams(searchParams);
    FILTERABLE.forEach((col) => next.delete(paramFor(col)));
    commit(next);
  };

  const toggleSort = (key: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('sort', key);
    // Same column flips direction; a new column starts ascending — for Age that
    // means newest first, which is the useful default.
    if (key === sortKey && sortDir === 'asc') next.set('dir', 'desc');
    else next.delete('dir');
    commit(next);
  };

  const sort = { key: sortKey, dir: sortDir, onSort: toggleSort };
  const filterFor = (col: string) => ({
    options: options[col] ?? [],
    selected: filters.get(col) ?? EMPTY,
    onToggle: toggleFilter,
    onClear: clearFilter,
  });

  return (
    <div className="relative flex h-[calc(100vh-3.25rem)]">
      <div className="flex-1 overflow-y-auto">
        <div className="px-6 py-3 flex items-center gap-3 sticky top-0 z-30 bg-slate-50 border-b border-slate-200">
          <h1 className="font-semibold">{cfg.label}</h1>
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={cfg.showOsb ? 'Search task, owner, image, node, id…' : 'Search…'}
            aria-label={`Search ${cfg.label.toLowerCase()}`}
            className="w-72 rounded-lg border border-slate-300 px-3 py-1 text-sm"
          />
          {/* Only the filter effect: the pager already carries the total. */}
          {data && rows.length !== data.items.length && (
            <span className="text-sm text-slate-500 tabular-nums">
              {rows.length} of {data.items.length} match
            </span>
          )}
          {filters.size > 0 && (
            <button
              type="button"
              className="text-sm text-slate-500 underline hover:text-slate-800"
              onClick={clearAllFilters}
            >
              clear filters
            </button>
          )}
          <span className="ml-auto" />
          {data && rows.length > 0 && (
            <Pager
              page={page}
              pageCount={pageCount}
              pageSize={pageSize}
              total={rows.length}
              sizes={PAGE_SIZES}
              onPage={setPage}
              onPageSize={setPageSize}
            />
          )}
        </div>

        {/*
          The checkbox is gone, but the overview's "OSB state stale" chip still
          links here — so the filter has to explain itself and offer a way out,
          the way the overview's own scope chip does.
        */}
        {staleOnly && !osbFilterUnsatisfiable && (
          <div className="mx-6 mt-3 flex flex-wrap items-center gap-3 rounded-xl border border-blue-200 bg-blue-50/60 px-4 py-2.5 text-sm">
            <span className="text-slate-600">
              Showing only sandboxes whose OpenSandbox state has stopped advancing.
            </span>
            <button
              type="button"
              onClick={() => setStale(false)}
              className="ml-auto rounded border border-slate-300 bg-white px-2 py-0.5 text-xs text-slate-600 hover:bg-slate-50"
            >
              Show all {cfg.label.toLowerCase()}
            </button>
          </div>
        )}

        {osbFilterUnsatisfiable && (
          <div className="mx-6 mt-3 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
            This filter needs OpenSandbox, which is{' '}
            {data.osb?.status === 'unreachable' ? 'unreachable' : 'not configured'}. An empty result
            here does not mean nothing matched — the state could not be computed.
          </div>
        )}

        {!osbFilterUnsatisfiable && data?.osb?.status === 'unreachable' && (
          <div className="mx-6 mt-3 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
            OpenSandbox is unreachable — showing Kubernetes state only.
          </div>
        )}
        {data?.osb?.status === 'ok' &&
          data.osb.reported - data.osb.matched > Math.max(5, data.osb.reported * 0.1) && (
            <div className="mx-6 mt-3 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
              OpenSandbox reported {data.osb.reported} sandboxes; only {data.osb.matched} matched a
              Kubernetes resource. A small deficit is expected briefly after deletions — the
              cached OpenSandbox inventory can lag the live cluster state by up to the cache TTL —
              but a gap this large usually means the join key stopped being stamped.
            </div>
          )}

        {isLoading && <Loading className="p-6" />}
        {error && <div className="p-6 text-red-700">Error: {(error as Error).message}</div>}

        {osbFilterUnsatisfiable ? null : data && (
          <table className="w-full text-sm">
            <thead className="text-left text-slate-500 bg-white">
              <tr>
                <ColumnHeader
                  label={cfg.showOsb ? 'Sandbox' : 'Name'}
                  col="task"
                  pad="px-6"
                  sort={sort}
                />
                {showNamespace && (
                  <ColumnHeader
                    label="Namespace"
                    col="namespace"
                    sort={sort}
                    filter={filterFor('namespace')}
                  />
                )}
                {cfg.showPhase && (
                  <ColumnHeader label="Status" col="phase" sort={sort} filter={filterFor('phase')} />
                )}
                {showCreator && (
                  <ColumnHeader
                    label="Creator"
                    col="creator"
                    sort={sort}
                    filter={filterFor('creator')}
                  />
                )}
                {showOwner && (
                  <ColumnHeader label="Owner" col="owner" sort={sort} filter={filterFor('owner')} />
                )}
                {showUsage && <ColumnHeader label="CPU cores" col="cpu" sort={sort} />}
                {showUsage && <ColumnHeader label="Memory" col="mem" sort={sort} />}
                {showNode && (
                  <ColumnHeader label="Node" col="node" sort={sort} filter={filterFor('node')} />
                )}
                <ColumnHeader label="Age" col="age" sort={sort} />
              </tr>
            </thead>
            <tbody>
              {visible.map((it) => {
                const selected =
                  it.namespace === params.namespace && it.name === params.name;
                const href = `/${cfg.kind}/${it.namespace}/${it.name}${search}`;
                return (
                  <tr
                    key={`${it.namespace}/${it.name}`}
                    // The whole row opens the drawer; the name stays a real link
                    // so middle- and cmd-click still open a tab.
                    onClick={() => navigate(href)}
                    className={`border-t border-slate-100 hover:bg-slate-100 cursor-pointer ${
                      selected ? 'bg-slate-100' : ''
                    }`}
                  >
                    <td className="px-6 py-2">
                      <Link to={href} className="block" title={it.sessionId ?? it.name}>
                        <span className="font-medium text-slate-900">{taskLabel(it).label}</span>
                        <SubLine it={it} />
                      </Link>
                    </td>
                    {showNamespace && (
                      <td className="px-3 py-2 text-slate-600">{it.namespace}</td>
                    )}
                    {cfg.showPhase && (
                      <td className="px-3 py-2">
                        <StatusCell it={it} osbConfigured={osbConfigured} />
                      </td>
                    )}
                    {showCreator && (
                      <td className="px-3 py-2 text-slate-600">
                        {it.creator ? (
                          <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                            <CreatorIcon creator={it.creator} />
                            {it.creator}
                          </span>
                        ) : (
                          '—'
                        )}
                      </td>
                    )}
                    {showOwner && (
                      <td className="px-3 py-2 text-slate-600">
                        {it.owner ? (
                          <>
                            <div className="whitespace-nowrap">{it.owner}</div>
                            {it.team && <div className="text-xs text-slate-400">{it.team}</div>}
                          </>
                        ) : (
                          ''
                        )}
                      </td>
                    )}
                    {showUsage && (
                      <td className="px-3 py-2 text-slate-600 tabular-nums">
                        <Load
                          used={podSample(it, usage.data)?.cpuCores}
                          reserved={(it.pod?.cpuMillis ?? 0) / 1000}
                          format={cores}
                        />
                      </td>
                    )}
                    {showUsage && (
                      <td className="px-3 py-2 text-slate-600 tabular-nums">
                        <Load
                          used={podSample(it, usage.data)?.memBytes}
                          reserved={it.pod?.memBytes ?? 0}
                          format={formatBytes}
                        />
                      </td>
                    )}
                    {showNode && (
                      <td
                        className="px-3 py-2 font-mono text-xs text-slate-500 whitespace-nowrap"
                        title={it.pod?.node}
                      >
                        {shortNode(it.pod?.node) || '—'}
                      </td>
                    )}
                    <td className="px-3 py-2 text-slate-600 tabular-nums">
                      {formatAge(it.ageSeconds)}
                    </td>
                  </tr>
                );
              })}
              {rows.length === 0 && !isLoading && (
                <tr>
                  <td className="px-6 py-6 text-slate-500" colSpan={9}>
                    {filters.size > 0
                      ? 'No rows match the selected column values.'
                      : `No ${cfg.label.toLowerCase()} found.`}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>

      {drawerOpen && (
        <DetailDrawer
          kind={kind}
          namespace={params.namespace!}
          name={params.name!}
          listUrl={`/${cfg.kind}${search}`}
        />
      )}
    </div>
  );
}

const EMPTY: Set<string> = new Set();

/**
 * Cores rather than millicores: a `39m` in a table whose next column reads `1m`
 * for one minute is a unit collision, and CPU is the one people misread.
 */
const cores = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(2));

function PhasePill({ phase }: { phase: string }) {
  const cls =
    phase === 'Ready'
      ? 'bg-emerald-100 text-emerald-800'
      : phase === 'NotReady' || phase === 'Scaling'
      ? 'bg-amber-100 text-amber-800'
      : 'bg-slate-100 text-slate-700';
  return <span className={`px-2 py-0.5 rounded text-xs ${cls}`}>{phase || '—'}</span>;
}

/**
 * A glyph for what created the sandbox. Decorative — the name stays next to it,
 * so this is aria-hidden rather than labelled.
 *
 * Two shapes are named because two creators mean something specific: a sandbox
 * the OpenSandbox server made, and one with no creator annotation at all — made
 * directly against the API, usually by a harness. Anything else a fleet reports
 * gets the neutral mark rather than no mark, so a creator nobody here has heard
 * of still renders as a creator.
 */
function CreatorIcon({ creator }: { creator: string }) {
  const shape =
    creator === 'opensandbox' ? (
      // A box, for the thing that boxes workloads up.
      <>
        <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3z" />
        <path d="M4 7.5l8 4.5 8-4.5" />
        <path d="M12 12v9" />
      </>
    ) : creator === 'unknown' ? (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M9.6 9.2a2.5 2.5 0 1 1 3.4 2.4c-.6.3-1 .9-1 1.6v.3" />
        <path d="M12 17.2h.01" />
      </>
    ) : (
      <>
        <circle cx="12" cy="12" r="9" />
        <circle cx="12" cy="12" r="3.5" />
      </>
    );

  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      className="h-3.5 w-3.5 shrink-0 text-slate-400"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {shape}
    </svg>
  );
}

/**
 * The generated id and the session's own token, under the task label. The id is
 * what a kubectl command needs; the token is what tells two runs of the same
 * task apart.
 */
function SubLine({ it }: { it: ResourceSummary }) {
  const { token } = taskLabel(it);
  const id = shortId(it.name);
  if (!id && !token) return null;
  return (
    <div className="font-mono text-xs text-slate-400">
      {[id, token].filter(Boolean).join(' · ')}
    </div>
  );
}

/**
 * Kubernetes readiness and the OpenSandbox state agree on the overwhelming
 * majority of rows, so this says it once. The OpenSandbox state appears only
 * when it adds something the phase does not — a Ready/Running row is just
 * "Ready" — and disagreement gets called out rather than left to be spotted.
 */
function StatusCell({ it, osbConfigured }: { it: ResourceSummary; osbConfigured: boolean }) {
  const osb = osbConfigured ? it.osb : undefined;
  const redundant = it.phase === 'Ready' && osb?.state === 'Running';
  return (
    <div>
      <PhasePill phase={it.phase} />
      {/* The reason belongs in the drawer: in a cell it doubles the column width. */}
      {osb && !redundant && (
        <div className="mt-0.5 text-xs text-slate-500 whitespace-nowrap" title={osb.reason}>
          OpenSandbox: {osb.state}
        </div>
      )}
      {osb?.diverged && (
        <div
          className="text-xs text-red-700"
          title="OpenSandbox disagrees with the Kubernetes Ready condition"
        >
          ⚠ diverged
        </div>
      )}
      {osb?.stale && (
        <div
          className="text-xs text-red-700 tabular-nums"
          title={`OpenSandbox has not advanced this state in ${formatAge(osb.stateAgeSeconds)}`}
        >
          ⏱ stuck {formatAge(osb.stateAgeSeconds)}
        </div>
      )}
    </div>
  );
}

/**
 * Live use against what the pod reserved. An absent sample is an em-dash, never
 * a zero: Prometheus not having scraped a pod yet and a pod being idle are
 * different facts, and rounding one into the other invents idle capacity.
 */
function Load({
  used,
  reserved,
  format,
}: {
  used?: number;
  reserved: number;
  format: (n: number) => string;
}) {
  const pct = used !== undefined && reserved > 0 ? (used / reserved) * 100 : undefined;
  return (
    <div className="min-w-[7rem]">
      <div>
        {used === undefined ? <span className="text-slate-400">—</span> : format(used)}
        {reserved > 0 && <span className="text-slate-400"> of {format(reserved)}</span>}
      </div>
      {pct !== undefined && (
        <div className="mt-1 h-1 w-20 rounded-full bg-slate-100">
          <div
            className={`h-1 rounded-full ${pct > 90 ? 'bg-red-400' : 'bg-blue-500'}`}
            style={{ width: `${Math.min(100, Math.max(2, pct))}%` }}
          />
        </div>
      )}
    </div>
  );
}
