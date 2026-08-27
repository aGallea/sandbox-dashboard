import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';
import { fetchPodLogs, type PodView } from '../api/client';
import { useRefreshInterval } from '../api/refresh';
import { parseLogs, type Level } from '../logfmt/format';
import { Loading } from './Loading';

const LINE_COUNTS = [50, 100, 200, 500];

const LEVEL_CLASS: Record<Level, string> = {
  error: 'text-red-600',
  warn: 'text-yellow-600',
  info: 'text-emerald-600',
  debug: 'text-slate-400',
  unknown: 'text-slate-600',
};

const ERRORS: Record<string, string> = {
  'container-not-started': 'The container has not started yet, so it has no logs.',
  'no-pod': 'The sandbox has no pod yet, or it is already gone.',
  'logs-unconfigured': 'This install cannot read pod logs.',
};

/**
 * The pod's log lines, read on demand: nothing is fetched until the drawer is
 * open, and only "Follow" keeps re-reading.
 *
 * ponytail: follow is a poll on the refresh interval, not a streamed
 * connection. The server's 15s write timeout would cut a real stream anyway;
 * switch to SSE when a tail that re-reads N lines is not enough.
 */
export function PodLogs({ namespace, name, pod }: { namespace: string; name: string; pod: PodView }) {
  const [from, setFrom] = useState<'tail' | 'head'>('tail');
  const [lines, setLines] = useState(LINE_COUNTS[0]);
  const [container, setContainer] = useState(pod.containers?.[0] ?? '');
  const [pretty, setPretty] = useState(true);
  const [follow, setFollow] = useState(false);
  const interval = useRefreshInterval();

  const following = follow && from === 'tail';
  const { data, error, isFetching, dataUpdatedAt, refetch } = useQuery({
    queryKey: ['logs', namespace, name, container, from, lines],
    queryFn: () => fetchPodLogs(namespace, name, { from, lines, container: container || undefined }),
    // Follow has to poll even when the rest of the page is paused, or the
    // checkbox would silently do nothing.
    refetchInterval: following ? interval || 2_000 : false,
    retry: false,
  });

  const parsed = useMemo(() => parseLogs(data ?? ''), [data]);

  const box = useRef<HTMLPreElement>(null);
  useEffect(() => {
    if (following && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [parsed, following]);

  const msg = (error as Error | null)?.message;
  const select = 'rounded border border-slate-300 px-1.5 py-0.5 text-xs';

  return (
    <section>
      <h3 className="text-sm font-semibold mb-2">Logs (read on demand)</h3>
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-600 mb-2">
        <select className={select} value={from} onChange={(e) => setFrom(e.target.value as 'tail' | 'head')} aria-label="Which end of the log">
          <option value="tail">Last</option>
          <option value="head">First</option>
        </select>
        <select className={select} value={lines} onChange={(e) => setLines(Number(e.target.value))} aria-label="How many lines">
          {LINE_COUNTS.map((n) => (
            <option key={n} value={n}>
              {n} lines
            </option>
          ))}
        </select>
        {(pod.containers?.length ?? 0) > 1 && (
          <select className={select} value={container} onChange={(e) => setContainer(e.target.value)} aria-label="Container">
            {pod.containers?.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        )}
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={pretty} onChange={(e) => setPretty(e.target.checked)} />
          Format
        </label>
        <label className="flex items-center gap-1" title="Re-read the tail on the refresh interval">
          <input
            type="checkbox"
            checked={following}
            disabled={from === 'head'}
            onChange={(e) => setFollow(e.target.checked)}
          />
          Follow
        </label>
        <button
          type="button"
          className="rounded border border-slate-300 px-2 py-0.5 hover:bg-slate-100 disabled:opacity-40"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          Refresh
        </button>
        {dataUpdatedAt > 0 && (
          <span className="text-slate-400 tabular-nums">{new Date(dataUpdatedAt).toLocaleTimeString()}</span>
        )}
      </div>

      {isFetching && !data && !error && <Loading label="Reading logs…" />}
      {error && (
        <div className="text-sm text-amber-700">{ERRORS[msg ?? ''] ?? `Could not read logs: ${msg}`}</div>
      )}
      {data !== undefined && !error && (
        <pre
          ref={box}
          className="text-xs bg-slate-50 border border-slate-200 rounded p-2 max-h-96 overflow-auto whitespace-pre"
        >
          {parsed.length === 0 && <span className="text-slate-400">(no output yet)</span>}
          {parsed.map((l, i) => (
            <div key={i} className={pretty ? LEVEL_CLASS[l.level] : ''}>
              {pretty ? l.text : l.raw}
            </div>
          ))}
        </pre>
      )}
    </section>
  );
}
