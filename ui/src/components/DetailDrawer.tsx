import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  fetchDetail,
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
  const close = () => navigate(listUrl);

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
          {kind === 'sandboxes' && <SandboxBody d={data as SandboxDetail} />}
          {kind === 'claims' && <ClaimBody d={data as ClaimDetail} />}
          {kind === 'templates' && <TemplateBody d={data as TemplateDetail} />}
          {kind === 'warmpools' && <WarmPoolBody d={data as WarmPoolDetail} />}
        </div>
      )}
    </aside>
  );
}

function SandboxBody({ d }: { d: SandboxDetail }) {
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
