/**
 * Pure rollups behind the overview page. Everything here runs on the sandbox
 * list the page already fetches, so a new way to slice the fleet costs a
 * function rather than an endpoint.
 *
 * ponytail: aggregating a few hundred rows in the browser is free. Past ~5k
 * sandboxes, move these rollups behind /api/v1/overview and send the totals.
 */
import type { ResourceSummary, UsageResponse } from '../api/client';
import { AGE_RAMP, OTHER_COLOR, SERIES, STATUS } from '../viz/palette';

// ----- palette -------------------------------------------------------------
// The tokens live in one module so the overview and the metrics page draw from
// the same validated system.

export { SERIES, OTHER_COLOR, AGE_RAMP, STATUS } from '../viz/palette';

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
  /** How many sandboxes carry a value for it at all. */
  covered: number;
}

/** Marks a dimension as one the fleet stamps rather than one we derived. */
const LABEL_PREFIX = 'label:';

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
        key: `${LABEL_PREFIX}${key}`,
        label: key,
        of: (it: ResourceSummary) => it.labels?.[key] ?? '',
      })),
    ...INTRINSIC,
  ];

  const cap = Math.max(2, Math.floor(items.length / 2));
  const sized = candidates
    .map((d) => ({ d, ...spread(items, d, cap) }))
    .filter(({ parts }) => parts >= 2 && parts <= cap);

  // Three rankings, in order.
  //
  // A label someone chose to stamp says more about how the fleet is meant to be
  // read than any field the dashboard invented, and it stays ahead of one: every
  // pod has a CPU request, so ranking on coverage alone opens the page on "CPU
  // request" with 90% of the fleet in one slice.
  //
  // Then readability: a dimension splitting the fleet into a handful of parts
  // reads at a glance, one splitting it into fifty is a list, so the long-tailed
  // ones — image, node — stay one selection away.
  //
  // Then coverage, which is what settles it between labels: `swe-instance-id`
  // passed the size test on three sandboxes out of fifty-nine and opened the
  // page on a ring that was 89% "unset".
  return sized
    .sort(
      (a, b) =>
        stamped(b.d) - stamped(a.d) || band(a.parts) - band(b.parts) || b.covered - a.covered,
    )
    .map(({ d, parts, covered }) => ({ ...d, parts, covered }));
}

/** Distinct non-empty values (giving up past `cap`) and how many rows have one. */
function spread(
  items: ResourceSummary[],
  d: DimensionSpec,
  cap: number,
): { parts: number; covered: number } {
  const values = new Set<string>();
  let covered = 0;
  let gaveUp = false;
  for (const it of items) {
    const v = d.of(it);
    if (!v) continue;
    covered += 1;
    if (!gaveUp) {
      values.add(v);
      if (values.size > cap) gaveUp = true;
    }
  }
  return { parts: gaveUp ? Infinity : values.size, covered };
}

/** 1 for a label the fleet stamps, 0 for a field the dashboard derived. */
function stamped(d: DimensionSpec): number {
  return d.key.startsWith(LABEL_PREFIX) ? 1 : 0;
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

  // Ties break on the key, never on arrival order. The informer does not promise
  // an order, so equal groups would otherwise swap places on every poll — which
  // on a fleet grouped by node is most of them, all reading the same 3%.
  const ranked = Array.from(totals.values()).sort(
    (a, b) => b.count - a.count || a.key.localeCompare(b.key),
  );
  const kept = ranked.slice(0, limit);
  const tail = ranked.slice(limit);

  // Hue follows rank, so the same fleet always paints the same way.
  //
  // Pinning a hue to the group name reads better — a reader learns "orange is
  // that node" — but SERIES holds five hues and the limit keeps five groups, so
  // with a full list the assignment is a bijection: any group entering the top
  // five has to displace another one's colour. That is what made the previous
  // version repaint all five slices whenever membership changed. Ranking is
  // deterministic now, so in practice a hue only moves when the ranking really
  // does.
  kept.forEach((s, i) => {
    s.color = SERIES[i % SERIES.length];
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
  /** `mute` is a count worth seeing that is not worth acting on. */
  tone: 'bad' | 'warn' | 'mute';
  /** Where the list page can show exactly these rows; absent when it cannot. */
  to?: string;
}

/**
 * How long a sandbox may take to become ready before that is a problem rather
 * than a start-up. A fleet that churns from 0 to 300 sandboxes within hours
 * always has a few seconds-old sandboxes not ready yet; counting those leaves
 * the chip permanently red, and a chip that is always red is furniture.
 */
export const STARTUP_GRACE_SECONDS = 300;

/**
 * What is worth acting on, loudest first. Only non-zero entries are returned —
 * a row of zeroes reads as noise, and the caller shows "nothing to act on"
 * instead.
 */
export function alerts(items: ResourceSummary[]): Alert[] {
  const count = (p: (it: ResourceSummary) => boolean) => items.filter(p).length;
  const waiting = (it: ResourceSummary) => {
    const state = stateOf(it);
    return state === 'notReady' || state === 'pending';
  };
  const candidates: Alert[] = [
    {
      label: `not ready for over ${formatAge(STARTUP_GRACE_SECONDS)}`,
      count: count((it) => waiting(it) && it.ageSeconds > STARTUP_GRACE_SECONDS),
      tone: 'bad',
      to: '/sandboxes?f_phase=NotReady',
    },
    {
      label: 'still starting',
      count: count((it) => waiting(it) && it.ageSeconds <= STARTUP_GRACE_SECONDS),
      tone: 'mute',
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

// ----- diagnosis -----------------------------------------------------------

/** `stuck` and `failing` both need attention; only `failing` will not fix itself. */
export type Verdict = 'ready' | 'starting' | 'stuck' | 'failing';

export interface Diagnosis {
  verdict: Verdict;
  /** The evidence, in the words Kubernetes or OpenSandbox used. */
  why: string;
}

/**
 * Container waiting reasons that never resolve on their own. A pod pulling an
 * image and a pod that cannot pull it look identical from the outside — same
 * phase, same colour — so the reason is the only thing that tells them apart.
 */
const FATAL_WAITING = new Set([
  'ImagePullBackOff',
  'ErrImagePull',
  'InvalidImageName',
  'CrashLoopBackOff',
  'CreateContainerConfigError',
  'CreateContainerError',
]);

/**
 * Whether one sandbox is fine, still coming up, late, or broken — and the reason.
 *
 * Colour alone cannot answer the question an operator actually has in front of a
 * wall of amber cells: is this one going to start? A sandbox twenty seconds into
 * a pull and one that has been retrying a bad image tag for an hour are the same
 * state and completely different problems.
 */
export function diagnose(it: ResourceSummary): Diagnosis {
  const pod = it.pod;
  const restarts = pod?.restarts ?? 0;

  if (pod?.waitingReason && FATAL_WAITING.has(pod.waitingReason)) {
    return { verdict: 'failing', why: pod.waitingReason };
  }
  // Restarts outrank a Ready condition: a pod that keeps dying and coming back
  // reports itself ready in the window between crashes.
  if (restarts > 0) {
    return { verdict: 'failing', why: `restarted ${restarts}×` };
  }
  if (it.osb?.state === 'Failed') {
    return { verdict: 'failing', why: it.osb.reason || 'OpenSandbox reports Failed' };
  }

  const state = stateOf(it);
  if (state === 'ready') return { verdict: 'ready', why: it.osb?.reason || 'Ready' };

  // A transient OpenSandbox state that stopped advancing is late by OpenSandbox's
  // own reckoning, whatever the sandbox's age says.
  if (it.osb?.stale) {
    return {
      verdict: 'stuck',
      why: `${it.osb.state} for ${formatAge(it.osb.stateAgeSeconds)}`,
    };
  }
  if (it.ageSeconds > STARTUP_GRACE_SECONDS) {
    return {
      verdict: 'stuck',
      why: `${pod?.waitingReason || it.osb?.reason || STATE_LABEL[state]} for ${formatAge(
        it.ageSeconds,
      )}`,
    };
  }
  return {
    verdict: 'starting',
    why: pod?.waitingReason || it.osb?.reason || STATE_LABEL[state],
  };
}

export const VERDICT_LABEL: Record<Verdict, string> = {
  ready: 'Ready',
  starting: 'Still starting',
  stuck: 'Taking too long',
  failing: 'Failing',
};

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
