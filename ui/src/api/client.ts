export interface ResourceCounts {
  total: number;
  ready: number;
  notReady: number;
  unknown: number;
}

export interface TemplateCounts {
  total: number;
}

export interface WarmPoolCounts {
  total: number;
  replicas: number;
  readyReplicas: number;
}

export interface OverviewResponse {
  sandboxes: ResourceCounts;
  claims: ResourceCounts;
  templates: TemplateCounts;
  warmPools: WarmPoolCounts;
}

export async function fetchOverview(): Promise<OverviewResponse> {
  const res = await fetch('/api/v1/overview');
  if (!res.ok) {
    throw new Error(`overview failed: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

// ----- list / detail / event types -----------------------------------------

export type ResourceKind = 'sandboxes' | 'claims' | 'templates' | 'warmpools';

export interface ResourceSummary {
  name: string;
  namespace: string;
  kind: 'Sandbox' | 'SandboxClaim' | 'SandboxTemplate' | 'SandboxWarmPool';
  phase: '' | 'Ready' | 'NotReady' | 'Unknown' | 'Scaling';
  ageSeconds: number;
}

export interface ListResponse {
  items: ResourceSummary[];
}

export interface Condition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

export interface EventEntry {
  time: string;
  type: 'Normal' | 'Warning';
  reason: string;
  message: string;
  source: string;
  count: number;
}

export interface SandboxDetail {
  summary: ResourceSummary;
  spec: unknown;
  conditions: Condition[];
  replicas: number;
  podIPs: string[];
  serviceFqdn: string;
  events: EventEntry[];
}

export interface ClaimDetail {
  summary: ResourceSummary;
  spec: unknown;
  conditions: Condition[];
  templateRef: string;
  sandboxStatus: {
    name: string;
    podIPs: string[];
  };
  events: EventEntry[];
}

export interface TemplateDetail {
  summary: ResourceSummary;
  spec: unknown;
}

export interface WarmPoolDetail {
  summary: ResourceSummary;
  spec: unknown;
  replicas: number;
  readyReplicas: number;
  selector: string;
}

export async function fetchList(
  kind: ResourceKind,
  params: { namespace?: string; phase?: string } = {},
): Promise<ListResponse> {
  const q = new URLSearchParams();
  if (params.namespace) q.set('namespace', params.namespace);
  if (params.phase) q.set('phase', params.phase);
  const qs = q.toString();
  const url = `/api/v1/${kind}${qs ? `?${qs}` : ''}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${kind} list failed: ${res.status}`);
  return res.json();
}

export async function fetchDetail<T>(kind: ResourceKind, namespace: string, name: string): Promise<T> {
  const res = await fetch(`/api/v1/${kind}/${namespace}/${name}`);
  if (res.status === 404) throw new Error('not-found');
  if (!res.ok) throw new Error(`${kind} detail failed: ${res.status}`);
  return res.json();
}

// ----- metrics types -------------------------------------------------------

export type MetricRange = '15m' | '1h' | '6h' | '24h';

export interface MetricPoint {
  time: string;   // RFC 3339
  value: number;
}

export interface MetricSeries {
  label: string;
  points: MetricPoint[];
}

export interface MetricResponse {
  name: string;
  title: string;
  description: string;
  unit: string;
  range: MetricRange;
  series: MetricSeries[];
}

export async function fetchMetric(name: string, range: MetricRange = '1h'): Promise<MetricResponse> {
  const res = await fetch(`/api/v1/metrics/${encodeURIComponent(name)}?range=${range}`);
  if (res.status === 503) throw new Error('prometheus-unconfigured');
  if (!res.ok) throw new Error(`metric ${name} failed: ${res.status}`);
  return res.json();
}
