/**
 * Pure rollups behind the overview page. Everything here runs on the sandbox
 * list the page already fetches, so a new way to slice the fleet costs a
 * function rather than an endpoint.
 *
 * ponytail: aggregating a few hundred rows in the browser is free. Past ~5k
 * sandboxes, move these rollups behind /api/v1/overview and send the totals.
 */
import type { ResourceSummary, UsageResponse } from '../api/client';

// ----- palette -------------------------------------------------------------
// Validated with the data-viz palette validator against the #ffffff card
// surface: the 5 categorical slots pass every gate (worst adjacent CVD ΔE 9.1),
// as does the 5-step ordinal blue ramp. Three of the categorical fills sit under
// 3:1 on white, so every chart using them ships visible labels.

export const SERIES = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4'];
/** The folded tail is a leftover, not an identity — it never takes a hue. */
export const OTHER_COLOR = '#94a3b8';
/** Age is ordered, so it gets one hue light→dark rather than five identities. */
export const AGE_RAMP = ['#86b6ef', '#5598e7', '#2a78d6', '#1c5cab', '#104281'];
export const STATUS = {
  ready: '#0ca30c',
  pending: '#fab219',
  failed: '#d03b3b',
  idle: '#94a3b8',
};

export const OTHER_KEY = 'Other';

// ----- fleet state ---------------------------------------------------------

export type FleetState = 'ready' | 'pending' | 'notReady' | 'unknown';

/**
 * One state per sandbox, from the Ready condition plus what the pod is doing.
 * Pending outranks NotReady: a sandbox waiting on a scheduler or an image pull
 * is a different problem from one whose pod is up but failing.
 */
export function stateOf(it: ResourceSummary): FleetState {
  const pod = it.pod;
  if (pod && (pod.phase === 'Pending' || pod.waitingReason)) return 'pending';
  if (it.phase === 'Ready') return 'ready';
  if (it.phase === 'NotReady') return 'notReady';
  return 'unknown';
}

export const STATE_LABEL: Record<FleetState, string> = {
  ready: 'Ready',
  pending: 'Pending',
  notReady: 'Not ready',
  unknown: 'Unknown',
};

export const STATE_COLOR: Record<FleetState, string> = {
  ready: STATUS.ready,
  pending: STATUS.pending,
  notReady: STATUS.failed,
  unknown: STATUS.idle,
};

/** Pod phase counts, including the sandboxes that have no pod at all. */
export function podPhases(items: ResourceSummary[]): Slice[] {
  const counts = new Map<string, number>();
  items.forEach((it) => {
    const phase = it.pod?.phase || 'No pod';
    counts.set(phase, (counts.get(phase) ?? 0) + 1);
  });
  const color: Record<string, string> = {
    Running: STATUS.ready,
    Pending: STATUS.pending,
    Failed: STATUS.failed,
    Succeeded: '#2a78d6',
    'No pod': STATUS.idle,
  };
  return Array.from(counts, ([key, count]) => ({
    ...emptySlice(key),
    count,
    color: color[key] ?? OTHER_COLOR,
  })).sort((a, b) => b.count - a.count);
}

// ----- grouping dimensions -------------------------------------------------

export interface DimensionSpec {
  key: string;
  label: string;
  of: (it: ResourceSummary) => string;
}

export interface Dimension extends DimensionSpec {
  /** How many distinct values this dimension has across the fleet. */
  parts: number;
}

const INTRINSIC: DimensionSpec[] = [
  { key: 'image', label: 'Image', of: (it) => shortImage(it.pod?.image) },
  { key: 'node', label: 'Node', of: (it) => it.pod?.node ?? '' },
  { key: 'namespace', label: 'Namespace', of: (it) => it.namespace },
  { key: 'cpu', label: 'CPU request', of: (it) => (it.pod ? coreLabel(it.pod.cpuMillis) : '') },
  {
    key: 'memory',
    label: 'Memory request',
    of: (it) => (it.pod ? formatBytes(it.pod.memBytes) : ''),
  },
  { key: 'readiness', label: 'Readiness', of: (it) => STATE_LABEL[stateOf(it)] },
  { key: 'osbState', label: 'OSB state', of: (it) => it.osb?.state ?? '' },
  { key: 'creator', label: 'Creator', of: (it) => it.creator ?? '' },
];

/**
 * The dimensions worth offering, discovered from the fleet rather than fixed.
 *
 * A dimension earns its place only if it actually divides the fleet: one value
 * puts everything in a single slice, and one value per sandbox groups nothing.
 * That single rule is what makes this work on any cluster — measured on
 * algo-studio it drops all three labels the sandboxes carry (`session_id` and
 * `opensandbox.io/id` are unique per sandbox, `policy.ai21.com/preemptible` has
 * one value) and keeps image, node and size, while a fleet that stamps `team=`
 * gets a Team dimension with no configuration.
 *
 * Labels come first: a key someone chose to stamp says more about how the fleet
 * is meant to be read than any field the dashboard invented.
 */
export function dimensionsFor(items: ResourceSummary[]): Dimension[] {
  const labelKeys = new Set<string>();
  items.forEach((it) => Object.keys(it.labels ?? {}).forEach((k) => labelKeys.add(k)));

  const candidates: DimensionSpec[] = [
    ...Array.from(labelKeys)
      .sort()
      .map((key) => ({
        key: `label:${key}`,
        label: key,
        of: (it: ResourceSummary) => it.labels?.[key] ?? '',
      })),
    ...INTRINSIC,
  ];

  const cap = Math.max(2, Math.floor(items.length / 2));
  const sized = candidates
    .map((d) => ({ d, parts: distinctCount(items, d, cap) }))
    .filter(({ parts }) => parts >= 2 && parts <= cap);

  // A dimension that splits the fleet into a handful of parts is readable at a
  // glance; one that splits it into fifty is a list. Offer the readable ones
  // first so the default lands on a dimension worth looking at, and keep the
  // long-tailed ones — image, node — one selection away.
  return sized
    .sort((a, b) => band(a.parts) - band(b.parts))
    .map(({ d, parts }) => ({ ...d, parts }));
}

/** Distinct non-empty values, giving up once past `cap`. */
function distinctCount(items: ResourceSummary[], d: DimensionSpec, cap: number): number {
  const values = new Set<string>();
  for (const it of items) {
    const v = d.of(it);
    if (v) values.add(v);
    if (values.size > cap) return Infinity;
  }
  return values.size;
}

/** 0 for a part-to-whole shape, 1 for a long tail. Order within a band holds. */
function band(parts: number): number {
  return parts <= DONUT_MAX ? 0 : 1;
}

/** Past six segments a ring stops being readable — the data-viz cap. */
export const DONUT_MAX = 6;

// ----- slices --------------------------------------------------------------

export interface Slice {
  key: string;
  count: number;
  cpuMillis: number;
  memBytes: number;
  gpu: number;
  /** Live use, summed over the group's pods; zero when Prometheus is absent. */
  usedCores: number;
  usedBytes: number;
  color: string;
}

/**
 * Groups the fleet along one dimension, keeping the largest `limit` groups and
 * folding the rest into a single Other.
 *
 * Hues are assigned by group name, never by rank, so the five-second refresh
 * cannot repaint a group the reader has already learned.
 */
export function groupBy(
  items: ResourceSummary[],
  d: DimensionSpec,
  limit = 5,
  usage?: UsageResponse,
): Slice[] {
  const totals = new Map<string, Slice>();
  items.forEach((it) => {
    const key = d.of(it) || 'unset';
    const slice = totals.get(key) ?? emptySlice(key);
    slice.count += 1;
    slice.cpuMillis += it.pod?.cpuMillis ?? 0;
    slice.memBytes += it.pod?.memBytes ?? 0;
    slice.gpu += it.pod?.gpu ?? 0;
    const sample = it.pod && usage?.pods[`${it.namespace}/${it.pod.name}`];
    if (sample) {
      slice.usedCores += sample.cpuCores;
      slice.usedBytes += sample.memBytes;
    }
    totals.set(key, slice);
  });

  const ranked = Array.from(totals.values()).sort((a, b) => b.count - a.count);
  const kept = ranked.slice(0, limit);
  const tail = ranked.slice(limit);

  const hueOf = new Map(
    kept
      .map((s) => s.key)
      .sort()
      .map((key, i) => [key, SERIES[i % SERIES.length]]),
  );
  kept.forEach((s) => {
    s.color = hueOf.get(s.key) ?? OTHER_COLOR;
  });

  if (!tail.length) return kept;
  return [
    ...kept,
    tail.reduce(
      (acc, s) => ({
        ...acc,
        count: acc.count + s.count,
        cpuMillis: acc.cpuMillis + s.cpuMillis,
        memBytes: acc.memBytes + s.memBytes,
        gpu: acc.gpu + s.gpu,
        usedCores: acc.usedCores + s.usedCores,
        usedBytes: acc.usedBytes + s.usedBytes,
      }),
      emptySlice(OTHER_KEY),
    ),
  ];
}

function emptySlice(key: string): Slice {
  return {
    key,
    count: 0,
    cpuMillis: 0,
    memBytes: 0,
    gpu: 0,
    usedCores: 0,
    usedBytes: 0,
    color: OTHER_COLOR,
  };
}

// ----- runtime -------------------------------------------------------------

export const AGE_BUCKETS = [
  { label: '<30m', until: 1_800 },
  { label: '30m–2h', until: 7_200 },
  { label: '2–6h', until: 21_600 },
  { label: '6–24h', until: 86_400 },
  { label: '>24h', until: Infinity },
];

export interface Bucket {
  label: string;
  count: number;
  color: string;
}

export function ageBuckets(items: ResourceSummary[]): Bucket[] {
  const counts = AGE_BUCKETS.map((b, i) => ({ label: b.label, count: 0, color: AGE_RAMP[i] }));
  items.forEach((it) => {
    const i = AGE_BUCKETS.findIndex((b) => it.ageSeconds < b.until);
    counts[i === -1 ? counts.length - 1 : i].count += 1;
  });
  return counts;
}

export function longestRunning(items: ResourceSummary[], limit = 5): ResourceSummary[] {
  return [...items].sort((a, b) => b.ageSeconds - a.ageSeconds).slice(0, limit);
}

// ----- totals --------------------------------------------------------------

export interface Reserved {
  cpuMillis: number;
  memBytes: number;
  gpu: number;
}

export function reserved(items: ResourceSummary[]): Reserved {
  return items.reduce<Reserved>(
    (acc, it) => ({
      cpuMillis: acc.cpuMillis + (it.pod?.cpuMillis ?? 0),
      memBytes: acc.memBytes + (it.pod?.memBytes ?? 0),
      gpu: acc.gpu + (it.pod?.gpu ?? 0),
    }),
    { cpuMillis: 0, memBytes: 0, gpu: 0 },
  );
}

/** Sums live usage over the sandbox pods only — the response may hold others. */
export function used(items: ResourceSummary[], usage?: UsageResponse) {
  if (!usage) return undefined;
  let cpuCores = 0;
  let memBytes = 0;
  items.forEach((it) => {
    const sample = it.pod && usage.pods[`${it.namespace}/${it.pod.name}`];
    if (!sample) return;
    cpuCores += sample.cpuCores;
    memBytes += sample.memBytes;
  });
  return { cpuCores, memBytes };
}

// ----- triage --------------------------------------------------------------

export interface Alert {
  label: string;
  count: number;
  tone: 'bad' | 'warn';
  /** Where the list page can show exactly these rows; absent when it cannot. */
  to?: string;
}

/**
 * What is worth acting on, loudest first. Only non-zero entries are returned —
 * a row of zeroes reads as noise, and the caller shows "nothing to act on"
 * instead.
 */
export function alerts(items: ResourceSummary[]): Alert[] {
  const count = (p: (it: ResourceSummary) => boolean) => items.filter(p).length;
  const candidates: Alert[] = [
    {
      label: 'not ready',
      count: count((it) => stateOf(it) === 'notReady'),
      tone: 'bad',
      to: '/sandboxes?f_phase=NotReady',
    },
    {
      label: 'pending',
      count: count((it) => stateOf(it) === 'pending'),
      tone: 'warn',
    },
    {
      label: 'restarting',
      count: count((it) => (it.pod?.restarts ?? 0) > 0),
      tone: 'warn',
    },
    {
      label: 'OSB state diverged',
      count: count((it) => !!it.osb?.diverged),
      tone: 'warn',
    },
    {
      label: 'OSB state stale',
      count: count((it) => !!it.osb?.stale),
      tone: 'warn',
      to: '/sandboxes?stale=true',
    },
    {
      label: 'running over a day',
      count: count((it) => it.ageSeconds > 86_400),
      tone: 'warn',
    },
  ];
  return candidates.filter((a) => a.count > 0);
}

// ----- formatting ----------------------------------------------------------

export function formatCores(millis: number): string {
  if (millis === 0) return '0';
  if (millis < 1000) return `${millis}m`;
  const cores = millis / 1000;
  return Number.isInteger(cores) ? `${cores}` : cores.toFixed(1);
}

/** A group label needs its unit: a slice reading "16" says nothing. */
export function coreLabel(millis: number): string {
  const cores = formatCores(millis);
  return `${cores} ${millis === 1000 ? 'core' : 'cores'}`;
}

export function formatBytes(bytes: number): string {
  const gib = bytes / 1024 ** 3;
  if (gib >= 1024) return `${(gib / 1024).toFixed(1)} TiB`;
  if (gib >= 1) return `${gib < 10 ? gib.toFixed(1) : Math.round(gib)} GiB`;
  return `${Math.round(bytes / 1024 ** 2)} MiB`;
}

export function formatAge(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

/** Drops the registry and digest so a chart label reads as the workload name. */
export function shortImage(image?: string): string {
  if (!image) return '';
  const withoutDigest = image.split('@')[0];
  const parts = withoutDigest.split('/');
  return parts[parts.length - 1];
}
