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

export type OsbState =
  | 'Pending'
  | 'Running'
  | 'Pausing'
  | 'Paused'
  | 'Resuming'
  | 'Stopping'
  | 'Terminated'
  | 'Failed';

export interface OsbView {
  state: OsbState | string;
  reason?: string;
  message?: string;
  expiresAt?: string;
  lastTransitionAt?: string;
  stateAgeSeconds: number;
  /** OpenSandbox's state disagrees with the Kubernetes Ready condition. */
  diverged: boolean;
  /** A transient OpenSandbox state has not advanced within the threshold. */
  stale: boolean;
}

export interface OsbStatus {
  status: 'ok' | 'unreachable';
  error?: string;
  fetchedAt?: string;
  reported: number;
  matched: number;
}

export interface SandboxOsbDetail {
  id: string;
  summary: string;
  events: string;
}

/** Pod-derived state for a sandbox: absent when the sandbox has no pod. */
export interface PodView {
  name: string;
  phase: string;
  node?: string;
  image?: string;
  restarts: number;
  waitingReason?: string;
  /** Requests summed over the pod's containers — what the cluster reserved. */
  cpuMillis: number;
  memBytes: number;
  gpu: number;
}

export interface ResourceSummary {
  name: string;
  namespace: string;
  kind: 'Sandbox' | 'SandboxClaim' | 'SandboxTemplate' | 'SandboxWarmPool';
  phase: '' | 'Ready' | 'NotReady' | 'Unknown' | 'Scaling';
  ageSeconds: number;
  /** Sandboxes only. */
  creator?: string;
  owner?: string;
  team?: string;
  experiment?: string;
  sessionId?: string;
  osb?: OsbView;
  pod?: PodView;
  /** Sandbox labels verbatim; the overview derives its grouping dimensions from them. */
  labels?: Record<string, string>;
}

export interface ListResponse {
  items: ResourceSummary[];
  /** Absent when no OpenSandbox URL is configured. */
  osb?: OsbStatus;
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
  params: {
    namespace?: string;
    phase?: string;
    creator?: string;
    osbState?: string;
    stale?: boolean;
  } = {},
): Promise<ListResponse> {
  const q = new URLSearchParams();
  if (params.namespace) q.set('namespace', params.namespace);
  if (params.phase) q.set('phase', params.phase);
  if (params.creator) q.set('creator', params.creator);
  if (params.osbState) q.set('osbState', params.osbState);
  if (params.stale) q.set('stale', 'true');
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

export async function fetchSandboxOsb(
  namespace: string,
  name: string,
): Promise<SandboxOsbDetail> {
  const res = await fetch(`/api/v1/sandboxes/${namespace}/${name}/osb`);
  if (res.status === 503) throw new Error('opensandbox-unconfigured');
  if (res.status === 404) throw new Error('not-an-opensandbox-sandbox');
  if (!res.ok) throw new Error(`opensandbox detail failed: ${res.status}`);
  return res.json();
}

// ----- live usage ----------------------------------------------------------

export interface PodUsage {
  cpuCores: number;
  memBytes: number;
}

export interface UsageResponse {
  sampledAt: string;
  /** Keyed "namespace/pod", matching `${it.namespace}/${it.pod.name}` on a sandbox row. */
  pods: Record<string, PodUsage>;
}

export async function fetchUsage(): Promise<UsageResponse> {
  const res = await fetch('/api/v1/usage');
  if (res.status === 503) throw new Error('prometheus-unconfigured');
  if (!res.ok) throw new Error(`usage failed: ${res.status}`);
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

export interface MetricInfo {
  name: string;
  title: string;
  description: string;
  unit: string;
}

export interface MetricSection {
  name: string;
  note?: string;
  metrics: MetricInfo[];
}

export interface MetricCatalog {
  sections: MetricSection[];
}

/** The charts this install offers, grouped for display. Served without Prometheus. */
export async function fetchMetricCatalog(): Promise<MetricCatalog> {
  const res = await fetch('/api/v1/metrics');
  if (!res.ok) throw new Error(`metric catalog failed: ${res.status}`);
  return res.json();
}

export async function fetchMetric(name: string, range: MetricRange = '1h'): Promise<MetricResponse> {
  const res = await fetch(`/api/v1/metrics/${encodeURIComponent(name)}?range=${range}`);
  if (res.status === 503) throw new Error('prometheus-unconfigured');
  if (!res.ok) throw new Error(`metric ${name} failed: ${res.status}`);
  return res.json();
}
