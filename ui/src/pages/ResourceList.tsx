import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMemo } from 'react';
import { fetchList } from '../api/client';
import { RESOURCES } from '../resources/config';
import { DetailDrawer } from '../components/DetailDrawer';
import { Loading } from '../components/Loading';
import { ColumnHeader, Pager, type SortDir } from '../components/TableControls';
import type { OsbView, ResourceKind, ResourceSummary } from '../api/client';

interface Props {
  kind: ResourceKind;
}

// One accessor per column, shared by sorting and value filtering so a column can
// never sort by one thing and filter by another.
const COLUMN_VALUE: Record<string, (it: ResourceSummary) => string | number> = {
  name: (it) => it.name,
  namespace: (it) => it.namespace,
  phase: (it) => it.phase,
  osbState: (it) => it.osb?.state ?? '',
  creator: (it) => it.creator ?? '',
  sessionId: (it) => it.sessionId ?? '',
  owner: (it) => it.owner ?? '',
  age: (it) => it.ageSeconds,
};

// Columns worth picking values from a list. Name is unique per row and Age is a
// number that moves every refresh, so neither makes a useful value filter.
const FILTERABLE = ['namespace', 'phase', 'osbState', 'creator', 'sessionId', 'owner'];

const paramFor = (col: string) => `f_${col}`;

const PAGE_SIZES = [10, 25, 50, 100, 200];
const DEFAULT_PAGE_SIZE = 25;

export function ResourceListPage({ kind }: Props) {
  const cfg = RESOURCES[kind];
  const navigate = useNavigate();
  const params = useParams<{ namespace?: string; name?: string }>();
  const drawerOpen = !!(params.namespace && params.name);

  const [searchParams, setSearchParams] = useSearchParams();
  const staleOnly = searchParams.get('stale') === 'true';
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
    refetchInterval: 5_000,
  });

  // Value lists come from the whole response, not the filtered rows, so a
  // selection never removes the options next to it.
  const options = useMemo(() => {
    const byColumn: Record<string, string[]> = {};
    FILTERABLE.forEach((col) => {
      const seen = new Set<string>();
      data?.items.forEach((it) => {
        const value = String(COLUMN_VALUE[col](it) ?? '');
        if (value) seen.add(value);
      });
      byColumn[col] = Array.from(seen).sort();
    });
    return byColumn;
  }, [data]);

  const rows = useMemo(() => {
    const value = COLUMN_VALUE[sortKey] ?? COLUMN_VALUE.age;
    const items = (data?.items ?? []).filter((it) =>
      Array.from(filters).every(([col, wanted]) =>
        wanted.has(String(COLUMN_VALUE[col](it) ?? '')),
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
  }, [data, filters, sortKey, sortDir]);

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
  const showOwner = cfg.showOsb && options.owner?.length > 0;

  // The response omits `osb` entirely when no OPENSANDBOX_URL is set, which is
  // how the UI can tell "not configured" from "configured but unreachable". The
  // OpenSandbox column and its stale filter appear only for the former — the
  // same em-dash rule as Owner above, and a filter that could only ever return
  // nothing is worse than no filter.
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
          {osbConfigured && (
            <label className="flex items-center gap-1 text-sm text-slate-600">
              <input
                type="checkbox"
                checked={staleOnly}
                onChange={(e) => setStale(e.target.checked)}
              />
              stale only
            </label>
          )}
        </div>

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
                <ColumnHeader label="Name" col="name" pad="px-6" sort={sort} />
                <ColumnHeader
                  label="Namespace"
                  col="namespace"
                  sort={sort}
                  filter={filterFor('namespace')}
                />
                {cfg.showPhase && (
                  <ColumnHeader label="Phase" col="phase" sort={sort} filter={filterFor('phase')} />
                )}
                {osbConfigured && (
                  <ColumnHeader
                    label="OSB State"
                    col="osbState"
                    sort={sort}
                    filter={filterFor('osbState')}
                  />
                )}
                {cfg.showOsb && (
                  <ColumnHeader
                    label="Creator"
                    col="creator"
                    sort={sort}
                    filter={filterFor('creator')}
                  />
                )}
                {cfg.showOsb && (
                  <ColumnHeader
                    label="Session"
                    col="sessionId"
                    sort={sort}
                    filter={filterFor('sessionId')}
                  />
                )}
                {showOwner && (
                  <ColumnHeader label="Owner" col="owner" sort={sort} filter={filterFor('owner')} />
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
                      <Link to={href} className="block">
                        {it.name}
                      </Link>
                    </td>
                    <td className="px-3 py-2 text-slate-600">{it.namespace}</td>
                    {cfg.showPhase && (
                      <td className="px-3 py-2">
                        <PhasePill phase={it.phase} />
                      </td>
                    )}
                    {osbConfigured && (
                      <td className="px-3 py-2">
                        <OsbStatePill osb={it.osb} />
                      </td>
                    )}
                    {cfg.showOsb && (
                      <td className="px-3 py-2 text-slate-600">{it.creator ?? '—'}</td>
                    )}
                    {cfg.showOsb && (
                      <td
                        className="px-3 py-2 text-slate-600 max-w-[16rem] truncate"
                        title={it.sessionId}
                      >
                        {it.sessionId ?? '—'}
                      </td>
                    )}
                    {showOwner && (
                      <td className="px-3 py-2 text-slate-600">{it.owner || '—'}</td>
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

function PhasePill({ phase }: { phase: string }) {
  const cls =
    phase === 'Ready'
      ? 'bg-emerald-100 text-emerald-800'
      : phase === 'NotReady' || phase === 'Scaling'
      ? 'bg-amber-100 text-amber-800'
      : 'bg-slate-100 text-slate-700';
  return <span className={`px-2 py-0.5 rounded text-xs ${cls}`}>{phase || '—'}</span>;
}

function OsbStatePill({ osb }: { osb?: OsbView }) {
  if (!osb) return <span className="text-slate-400">—</span>;

  const cls =
    osb.state === 'Running'
      ? 'bg-emerald-100 text-emerald-800'
      : osb.state === 'Failed' || osb.state === 'Terminated'
      ? 'bg-red-100 text-red-800'
      : 'bg-amber-100 text-amber-800';

  return (
    <span className="inline-flex items-center gap-1">
      <span className={`px-2 py-0.5 rounded text-xs ${cls}`}>{osb.state}</span>
      {osb.diverged && (
        <span title="OpenSandbox disagrees with the Kubernetes Ready condition">⚠</span>
      )}
      {osb.stale && (
        <span
          className="text-xs text-red-700 tabular-nums"
          title={`OpenSandbox has not advanced this state in ${formatAge(osb.stateAgeSeconds)}`}
        >
          ⏱ {formatAge(osb.stateAgeSeconds)}
        </span>
      )}
    </span>
  );
}

function formatAge(secs: number) {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h`;
  return `${Math.floor(secs / 86400)}d`;
}
