import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  fetchDetail,
  fetchSandboxOsb,
  type ResourceKind,
  type SandboxDetail,
  type ClaimDetail,
  type TemplateDetail,
  type WarmPoolDetail,
} from '../api/client';
import { ConditionsTable } from './ConditionsTable';
import { EventsList } from './EventsList';
import { YamlBlock } from './YamlBlock';

interface Props {
  kind: ResourceKind;
  namespace: string;
  name: string;
  /** URL we return to when the drawer closes (e.g. /sandboxes). */
  listUrl: string;
}

export function DetailDrawer({ kind, namespace, name, listUrl }: Props) {
  const navigate = useNavigate();
  const close = () => navigate(listUrl, { replace: true });

  const { data, isLoading, error } = useQuery({
    queryKey: ['detail', kind, namespace, name],
    queryFn: () =>
      fetchDetail<SandboxDetail | ClaimDetail | TemplateDetail | WarmPoolDetail>(
        kind,
        namespace,
        name,
      ),
    refetchInterval: 10_000,
  });

  return (
    <aside className="w-[36rem] max-w-full bg-white border-l border-slate-200 shadow-xl overflow-y-auto">
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

      {isLoading && <div className="p-4 text-sm text-slate-500">Loading…</div>}
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
      <OsbSection namespace={namespace} name={name} />
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

function OsbSection({ namespace, name }: { namespace: string; name: string }) {
  const { data, isFetching, error } = useQuery({
    queryKey: ['osb-detail', namespace, name],
    queryFn: () => fetchSandboxOsb(namespace, name),
    // Only poll once we actually have diagnostics. On any error the query keeps
    // data undefined, which makes react-query reset status to "pending" on every
    // refetch — that would blink a genuine error section out every 10s. It also
    // avoids polling /osb forever for the many sandboxes OpenSandbox never created.
    refetchInterval: (query) => (query.state.data ? 10_000 : false),
    retry: false,
  });

  // Render nothing until the query settles. The section legitimately does not
  // apply to most sandboxes (404) or to an unconfigured install (503), so
  // showing a heading first would make it flash and vanish on every one of them.
  if (isFetching && !data && !error) return null;

  // Not an OpenSandbox sandbox, or OpenSandbox is not configured: show nothing
  // rather than an error the operator can do nothing about.
  const msg = (error as Error | null)?.message;
  if (msg === 'not-an-opensandbox-sandbox' || msg === 'opensandbox-unconfigured') return null;

  return (
    <section>
      <h3 className="text-sm font-semibold mb-2">OpenSandbox</h3>
      {error && !data && <div className="text-sm text-red-700">{msg}</div>}
      {error && data && (
        <div className="text-sm text-amber-700">
          Showing the last successful fetch — the latest attempt failed: {msg}
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
    </section>
  );
}
