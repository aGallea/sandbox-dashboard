import { useQuery } from '@tanstack/react-query';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useMemo } from 'react';
import { fetchList } from '../api/client';
import { RESOURCES } from '../resources/config';
import { DetailDrawer } from '../components/DetailDrawer';
import type { ResourceKind } from '../api/client';

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
  const search = searchParams.toString() ? `?${searchParams.toString()}` : '';

  const { data, isLoading, error } = useQuery({
    queryKey: ['list', kind, nsFilter, phaseFilter],
    queryFn: () =>
      fetchList(kind, {
        namespace: nsFilter || undefined,
        phase: phaseFilter || undefined,
      }),
    refetchInterval: 5_000,
  });

  const namespaces = useMemo(() => {
    const set = new Set<string>();
    data?.items.forEach((it) => set.add(it.namespace));
    return Array.from(set).sort();
  }, [data]);

  const updateFilter = (key: 'namespace' | 'phase', value: string) => {
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
        </div>

        {isLoading && <div className="p-6 text-slate-500">Loading…</div>}
        {error && <div className="p-6 text-red-700">Error: {(error as Error).message}</div>}

        {data && (
          <table className="w-full text-sm">
            <thead className="text-left text-slate-500 bg-white">
              <tr>
                <th className="px-6 py-2">Name</th>
                <th className="px-3 py-2">Namespace</th>
                {cfg.showPhase && <th className="px-3 py-2">Phase</th>}
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

function formatAge(secs: number) {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h`;
  return `${Math.floor(secs / 86400)}d`;
}
