import { useQuery } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMemo } from 'react';
import { fetchList } from '../api/client';
import { RESOURCES } from '../resources/config';
import { DetailDrawer } from '../components/DetailDrawer';
import type { OsbView, ResourceKind } from '../api/client';

interface Props {
  kind: ResourceKind;
}

export function ResourceListPage({ kind }: Props) {
  const cfg = RESOURCES[kind];
  const params = useParams<{ namespace?: string; name?: string }>();
  const drawerOpen = !!(params.namespace && params.name);

  const [searchParams, setSearchParams] = useSearchParams();
  const nsFilter = searchParams.get('namespace') ?? '';
  const phaseFilter = searchParams.get('phase') ?? '';
  const creatorFilter = searchParams.get('creator') ?? '';
  const osbStateFilter = searchParams.get('osbState') ?? '';
  const staleOnly = searchParams.get('stale') === 'true';
  const search = searchParams.toString() ? `?${searchParams.toString()}` : '';

  const { data, isLoading, error } = useQuery({
    queryKey: ['list', kind, nsFilter, phaseFilter, creatorFilter, osbStateFilter, staleOnly],
    queryFn: () =>
      fetchList(kind, {
        namespace: nsFilter || undefined,
        phase: phaseFilter || undefined,
        creator: creatorFilter || undefined,
        osbState: osbStateFilter || undefined,
        stale: staleOnly || undefined,
      }),
    refetchInterval: 5_000,
  });

  const namespaces = useMemo(() => {
    const set = new Set<string>();
    data?.items.forEach((it) => set.add(it.namespace));
    return Array.from(set).sort();
  }, [data]);

  // These filters can only be satisfied when OpenSandbox data is present, so a
  // filtered empty list means "could not compute", not "nothing matched".
  // Gated on cfg.showOsb because the other resource kinds never return an osb
  // field, which would otherwise make this true for them and hide their rows.
  const osbFilterActive = cfg.showOsb && (staleOnly || osbStateFilter !== '');
  const osbFilterUnsatisfiable = osbFilterActive && !!data && data.osb?.status !== 'ok';

  const updateFilter = (key: 'namespace' | 'phase' | 'creator' | 'osbState' | 'stale', value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    setSearchParams(next, { replace: true });
  };

  return (
    <div className="flex h-[calc(100vh-3.25rem)]">
      <div className="flex-1 overflow-y-auto">
        <div className="px-6 py-3 flex items-center gap-3 sticky top-0 bg-slate-50 border-b border-slate-200">
          <h1 className="font-semibold">{cfg.label}</h1>
          <select
            className="border border-slate-300 rounded px-2 py-1 text-sm"
            value={nsFilter}
            onChange={(e) => updateFilter('namespace', e.target.value)}
          >
            <option value="">all namespaces</option>
            {namespaces.map((ns) => (
              <option key={ns} value={ns}>
                {ns}
              </option>
            ))}
          </select>
          {cfg.showPhase && (
            <select
              className="border border-slate-300 rounded px-2 py-1 text-sm"
              value={phaseFilter}
              onChange={(e) => updateFilter('phase', e.target.value)}
            >
              <option value="">any phase</option>
              {cfg.phases.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          )}
          {cfg.showOsb && (
            <>
              <select
                className="border border-slate-300 rounded px-2 py-1 text-sm"
                value={creatorFilter}
                onChange={(e) => updateFilter('creator', e.target.value)}
              >
                <option value="">any creator</option>
                <option value="opensandbox">opensandbox</option>
                <option value="unknown">unknown</option>
              </select>
              <select
                className="border border-slate-300 rounded px-2 py-1 text-sm"
                value={osbStateFilter}
                onChange={(e) => updateFilter('osbState', e.target.value)}
              >
                <option value="">any OSB state</option>
                {cfg.osbStates.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
              <label className="flex items-center gap-1 text-sm text-slate-600">
                <input
                  type="checkbox"
                  checked={staleOnly}
                  onChange={(e) => updateFilter('stale', e.target.checked ? 'true' : '')}
                />
                stale only
              </label>
            </>
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

        {isLoading && <div className="p-6 text-slate-500">Loading…</div>}
        {error && <div className="p-6 text-red-700">Error: {(error as Error).message}</div>}

        {osbFilterUnsatisfiable ? null : data && (
          <table className="w-full text-sm">
            <thead className="text-left text-slate-500 bg-white">
              <tr>
                <th className="px-6 py-2">Name</th>
                <th className="px-3 py-2">Namespace</th>
                {cfg.showPhase && <th className="px-3 py-2">Phase</th>}
                {cfg.showOsb && <th className="px-3 py-2">OSB State</th>}
                {cfg.showOsb && <th className="px-3 py-2">Creator</th>}
                {cfg.showOsb && <th className="px-3 py-2">Session</th>}
                {cfg.showOsb && <th className="px-3 py-2">Owner</th>}
                <th className="px-3 py-2">Age</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((it) => {
                const selected =
                  it.namespace === params.namespace && it.name === params.name;
                return (
                  <tr
                    key={`${it.namespace}/${it.name}`}
                    className={`border-t border-slate-100 hover:bg-slate-100 cursor-pointer ${
                      selected ? 'bg-slate-100' : ''
                    }`}
                  >
                    <td className="px-6 py-2">
                      <Link
                        to={`/${cfg.kind}/${it.namespace}/${it.name}${search}`}
                        className="block"
                      >
                        {it.name}
                      </Link>
                    </td>
                    <td className="px-3 py-2 text-slate-600">{it.namespace}</td>
                    {cfg.showPhase && (
                      <td className="px-3 py-2">
                        <PhasePill phase={it.phase} />
                      </td>
                    )}
                    {cfg.showOsb && (
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
                    {cfg.showOsb && (
                      <td className="px-3 py-2 text-slate-600">{it.owner || '—'}</td>
                    )}
                    <td className="px-3 py-2 text-slate-600 tabular-nums">
                      {formatAge(it.ageSeconds)}
                    </td>
                  </tr>
                );
              })}
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
