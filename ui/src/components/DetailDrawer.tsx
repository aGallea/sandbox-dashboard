import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  fetchDetail,
  fetchSandboxOsb,
  type ResourceKind,
  type SandboxDetail,
  type ClaimDetail,
  type TemplateDetail,
  type WarmPoolDetail,
  type OsbView,
} from '../api/client';
import { useRefreshInterval } from '../api/refresh';
import { ConditionsTable } from './ConditionsTable';
import { Loading } from './Loading';
import { EventsList } from './EventsList';
import { YamlBlock } from './YamlBlock';

interface Props {
  kind: ResourceKind;
  namespace: string;
  name: string;
  /** URL we return to when the drawer closes (e.g. /sandboxes). */
  listUrl: string;
}

const MIN_WIDTH = 360;
const DEFAULT_WIDTH = 576;
const WIDTH_KEY = 'drawerWidth';
const clampWidth = (w: number) =>
  Math.round(Math.min(Math.max(w, MIN_WIDTH), window.innerWidth - 120));

export function DetailDrawer({ kind, namespace, name, listUrl }: Props) {
  const navigate = useNavigate();
  const panel = useRef<HTMLElement>(null);
  const close = useCallback(() => navigate(listUrl, { replace: true }), [navigate, listUrl]);

  // Dismissed by anything outside the panel, and by Escape — a panel you can
  // open but only close through one small ✕ is the part that felt stuck.
  useEffect(() => {
    // mousedown, not click: pressing another row closes this drawer and lets
    // that row's own click open its one, rather than the two fighting.
    const onDown = (e: MouseEvent) => {
      if (!panel.current?.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [close]);

  const [width, setWidth] = useState(
    () => Number(localStorage.getItem(WIDTH_KEY)) || DEFAULT_WIDTH,
  );

  // Dragging the left edge widens the drawer. Tracked in a local rather than
  // read back from state so the persisted value is the one the drag ended on.
  const startResize = (e: React.PointerEvent) => {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = width;
    let latest = startWidth;
    const onMove = (ev: PointerEvent) => {
      latest = clampWidth(startWidth + (startX - ev.clientX));
      setWidth(latest);
    };
    const onUp = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      localStorage.setItem(WIDTH_KEY, String(latest));
    };
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
  };

  const nudge = (by: number) => {
    const next = clampWidth(width + by);
    setWidth(next);
    localStorage.setItem(WIDTH_KEY, String(next));
  };

  const { data, isLoading, error } = useQuery({
    queryKey: ['detail', kind, namespace, name],
    queryFn: () =>
      fetchDetail<SandboxDetail | ClaimDetail | TemplateDetail | WarmPoolDetail>(
        kind,
        namespace,
        name,
      ),
    refetchInterval: useRefreshInterval(2),
  });

  // Overlays the list rather than sitting beside it: the table keeps its full
  // width, so opening a row never reflows the columns underneath.
  return (
    <aside
      ref={panel}
      role="dialog"
      aria-label={`${kind} ${name}`}
      className="drawer-enter absolute inset-y-0 right-0 z-40 flex max-w-full bg-white border-l border-slate-200 shadow-xl"
      style={{ width }}
    >
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize detail panel"
        tabIndex={0}
        onPointerDown={startResize}
        onKeyDown={(e) => {
          if (e.key === 'ArrowLeft') nudge(40);
          else if (e.key === 'ArrowRight') nudge(-40);
        }}
        className="w-1.5 shrink-0 cursor-col-resize hover:bg-slate-300 focus:bg-slate-400 focus:outline-none"
      />
      <div className="flex-1 overflow-y-auto">
      <header className="px-4 py-3 border-b border-slate-200 flex items-center justify-between">
        <div>
          <div className="text-xs text-slate-500">{kind}</div>
          <h2 className="font-semibold">{name}</h2>
          <div className="text-xs text-slate-500">ns: {namespace}</div>
        </div>
        <button
          onClick={close}
          className="text-slate-500 hover:text-slate-900"
          aria-label="Close detail"
        >
          ✕
        </button>
      </header>

        {isLoading && <Loading className="p-4" />}
      {error && (
        <div className="p-4 text-sm text-red-700">
          {(error as Error).message === 'not-found'
            ? 'Not found.'
            : (error as Error).message}
        </div>
      )}

      {data && (
        <div className="p-4 space-y-4">
          {kind === 'sandboxes' && (
            <SandboxBody d={data as SandboxDetail} namespace={namespace} name={name} />
          )}
          {kind === 'claims' && <ClaimBody d={data as ClaimDetail} />}
          {kind === 'templates' && <TemplateBody d={data as TemplateDetail} />}
          {kind === 'warmpools' && <WarmPoolBody d={data as WarmPoolDetail} />}
        </div>
        )}
      </div>
    </aside>
  );
}

function SandboxBody({
  d,
  namespace,
  name,
}: {
  d: SandboxDetail;
  namespace: string;
  name: string;
}) {
  return (
    <>
      <section>
        <h3 className="text-sm font-semibold mb-2">Status</h3>
        <ConditionsTable conditions={d.conditions} />
        <dl className="mt-3 grid grid-cols-2 gap-x-3 text-sm">
          <dt className="text-slate-500">Replicas</dt>
          <dd className="tabular-nums">{d.replicas}</dd>
          <dt className="text-slate-500">Service</dt>
          <dd className="break-all">{d.serviceFqdn || '—'}</dd>
          <dt className="text-slate-500">Pod IPs</dt>
          <dd>{(d.podIPs ?? []).join(', ') || '—'}</dd>
        </dl>
      </section>
      <YamlBlock value={d.spec} />
      <section>
        <h3 className="text-sm font-semibold mb-2">Events</h3>
        <EventsList events={d.events} />
      </section>
      <OsbSection namespace={namespace} name={name} osb={d.summary.osb} />
    </>
  );
}

function ClaimBody({ d }: { d: ClaimDetail }) {
  return (
    <>
      <section>
        <h3 className="text-sm font-semibold mb-2">Status</h3>
        <ConditionsTable conditions={d.conditions} />
        <dl className="mt-3 grid grid-cols-2 gap-x-3 text-sm">
          <dt className="text-slate-500">Template</dt>
          <dd>{d.templateRef || '—'}</dd>
          <dt className="text-slate-500">Bound Sandbox</dt>
          <dd>{d.sandboxStatus?.name || '—'}</dd>
          <dt className="text-slate-500">Pod IPs</dt>
          <dd>{(d.sandboxStatus?.podIPs ?? []).join(', ') || '—'}</dd>
        </dl>
      </section>
      <YamlBlock value={d.spec} />
      <section>
        <h3 className="text-sm font-semibold mb-2">Events</h3>
        <EventsList events={d.events} />
      </section>
    </>
  );
}

function TemplateBody({ d }: { d: TemplateDetail }) {
  return <YamlBlock value={d.spec} defaultOpen />;
}

function WarmPoolBody({ d }: { d: WarmPoolDetail }) {
  return (
    <>
      <section>
        <dl className="grid grid-cols-2 gap-x-3 text-sm">
          <dt className="text-slate-500">Replicas</dt>
          <dd className="tabular-nums">
            {d.readyReplicas} / {d.replicas}
          </dd>
          <dt className="text-slate-500">Selector</dt>
          <dd className="break-all">{d.selector || '—'}</dd>
        </dl>
      </section>
      <YamlBlock value={d.spec} />
    </>
  );
}

function OsbSection({
  namespace,
  name,
  osb,
}: {
  namespace: string;
  name: string;
  /** OpenSandbox's own watch-fed view, from the sandbox's ResourceSummary. */
  osb?: OsbView;
}) {
  const detailRefetch = useRefreshInterval(2);
  const { data, isFetching, error, dataUpdatedAt } = useQuery({
    queryKey: ['osb-detail', namespace, name],
    queryFn: () => fetchSandboxOsb(namespace, name),
    // Only poll once we actually have diagnostics. On any error the query keeps
    // data undefined, which makes react-query reset status to "pending" on every
    // refetch — that would blink a genuine error section out every 10s. It also
    // avoids polling /osb forever for the many sandboxes OpenSandbox never created.
    refetchInterval: (query) => (query.state.data ? detailRefetch : false),
    retry: false,
  });

  const msg = (error as Error | null)?.message;
  // The diagnostics half legitimately does not apply to most sandboxes (404)
  // or to an unconfigured install (503).
  const diagnosticsInapplicable =
    msg === 'not-an-opensandbox-sandbox' || msg === 'opensandbox-unconfigured';

  // Without a watch-fed view to fall back on, keep the original contract:
  // nothing until the diagnostics query settles (avoids a flash on every
  // sandbox), and nothing when diagnostics do not apply. With a watch-fed
  // view, that view is exactly what an operator needs when diagnostics are
  // unavailable, so the section always renders.
  if (!osb) {
    if (isFetching && !data && !error) return null;
    if (diagnosticsInapplicable) return null;
  }

  return (
    <section className="space-y-3">
      {osb && (
        <div>
          <h3 className="text-sm font-semibold mb-2">OpenSandbox state (from its watch)</h3>
          <dl className="grid grid-cols-2 gap-x-3 text-sm">
            <dt className="text-slate-500">State</dt>
            <dd>{osb.state || '—'}</dd>
            <dt className="text-slate-500">Reason</dt>
            <dd>{osb.reason || '—'}</dd>
            <dt className="text-slate-500">Message</dt>
            <dd className="break-words">{osb.message || '—'}</dd>
            <dt className="text-slate-500">Expiry</dt>
            <dd>{osb.expiresAt ? new Date(osb.expiresAt).toLocaleString() : '—'}</dd>
            <dt className="text-slate-500">State age</dt>
            <dd className="tabular-nums">{osb.stateAgeSeconds}s</dd>
          </dl>
        </div>
      )}
      {!diagnosticsInapplicable && (
        <div>
          <h3 className="text-sm font-semibold mb-2">Diagnostics (read on demand)</h3>
          {isFetching && !data && !error && <Loading label="Reading diagnostics…" />}
          {error && !data && <div className="text-sm text-red-700">{msg}</div>}
          {error && data && (
            <div className="text-sm text-amber-700">
              Showing the last successful fetch from {new Date(dataUpdatedAt).toLocaleTimeString()}{' '}
              — the latest attempt failed: {msg}
            </div>
          )}
          {data && (
            <>
              <div className="text-xs text-slate-500 mb-1">id: {data.id}</div>
              <pre className="text-xs bg-slate-50 border border-slate-200 rounded p-2 overflow-x-auto whitespace-pre">
                {data.summary}
              </pre>
              <h4 className="text-xs font-semibold mt-3 mb-1">OpenSandbox events</h4>
              <pre className="text-xs bg-slate-50 border border-slate-200 rounded p-2 overflow-x-auto whitespace-pre">
                {data.events}
              </pre>
            </>
          )}
        </div>
      )}
    </section>
  );
}
