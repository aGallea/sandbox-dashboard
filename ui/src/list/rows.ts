/**
 * Row-level rollups for the resource list. Pure functions over one summary, so
 * they test without a DOM or a cluster — the same bargain aggregate.ts makes.
 */
import type { PodUsage, ResourceSummary, UsageResponse } from '../api/client';
import { shortImage } from '../overview/aggregate';

const LABEL_MAX = 34;
const TOKEN_MAX = 14;

/**
 * A run of hex long enough to be a generated id rather than a word. Session ids
 * range from `build-pmars__Ry5KMzg__env` to
 * `instance_tutao__tutanota-<40 hex>-<40 hex>`, and only the words identify the task.
 */
const HEXY = /^[0-9a-f]{12,}$/;

export interface TaskLabel {
  /** What the row is called: readable, clipped, never empty. */
  label: string;
  /** A short disambiguator when the session carries one; '' otherwise. */
  token: string;
}

/**
 * Names a sandbox the way the person who launched it would: by its task. The
 * object name is a generated UUID, which identifies a row but describes nothing.
 */
export function taskLabel(it: ResourceSummary): TaskLabel {
  // A sandbox with no session id carries the same task name — and the same hex —
  // in its image tag, so both go through one cleanup.
  const source = it.sessionId || shortImage(it.pod?.image).split(':')[0];
  if (!source) return { label: clip(it.name, LABEL_MAX), token: '' };

  const parts = source.split('__').filter(Boolean);
  const head = parts[0] ?? source;
  const words = head.split('-').filter((w) => !HEXY.test(w));
  return { label: clip(words.join('-') || head, LABEL_MAX), token: tokenOf(parts[1] ?? '') };
}

function tokenOf(token: string): string {
  if (token.length > TOKEN_MAX) return '';
  return token.split('-').some((w) => HEXY.test(w)) ? '' : token;
}

/**
 * The leading block of a generated id — enough to recognise a row and to paste
 * into a kubectl command. Empty for a name someone chose, since truncating a
 * readable name only makes it unreadable.
 */
export function shortId(name: string): string {
  return /([0-9a-f]{8})-[0-9a-f]{4}/.exec(name)?.[1] ?? '';
}

/**
 * Node names differ only in their last two segments
 * (`gke-main-nap-t2d-standar-32-9qmzbrd3-5e131c93-6957`), so that is the part
 * worth column width. The full value belongs in a title attribute.
 */
export function shortNode(node?: string): string {
  if (!node) return '';
  const tail = node.split('-').slice(-2).join('-');
  return tail === node ? node : `…${tail}`;
}

/** Case-insensitive match across every field someone might search a fleet by. */
export function matchesQuery(it: ResourceSummary, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return [
    it.name,
    it.namespace,
    it.sessionId,
    it.owner,
    it.team,
    it.experiment,
    it.creator,
    it.pod?.image,
    it.pod?.node,
  ]
    .join(' ')
    .toLowerCase()
    .includes(q);
}

/**
 * Whether a column tells two rows apart. A column whose every row says
 * `default` costs width and teaches nothing; it comes back on its own once the
 * fleet spans more than one value. Blanks are not a value of their own.
 */
export function informative(
  items: ResourceSummary[],
  value: (it: ResourceSummary) => string,
): boolean {
  const seen = new Set<string>();
  for (const it of items) {
    const v = value(it);
    if (v) seen.add(v);
    if (seen.size > 1) return true;
  }
  return false;
}

/**
 * Live usage for one sandbox's pod, or undefined when Prometheus has nothing for
 * it. Absent and zero are different answers — one means "not sampled yet", the
 * other means "genuinely idle" — so this never falls back to a zero.
 */
export function podSample(it: ResourceSummary, usage?: UsageResponse): PodUsage | undefined {
  if (!it.pod || !usage) return undefined;
  return usage.pods[`${it.namespace}/${it.pod.name}`];
}

function clip(text: string, max: number): string {
  return text.length <= max ? text : `${text.slice(0, max - 1)}…`;
}
