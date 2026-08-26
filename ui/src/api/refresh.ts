import { useSyncExternalStore } from 'react';

/**
 * How often the browser re-asks the server for data.
 *
 * The server does not poll Kubernetes — it holds watch-backed informer caches,
 * so it already knows the moment a sandbox changes. The polling is only the last
 * hop, browser to server, and it is what this setting controls.
 *
 * A module-level store read through useSyncExternalStore rather than a context:
 * every query in the app needs the value and nothing needs to provide it, so a
 * provider would be plumbing for its own sake.
 */
const KEY = 'refreshMs';

export const DEFAULT_REFRESH_MS = 5_000;

/** Off is a real choice: reading a page while the fleet churns needs it to hold still. */
export const REFRESH_OPTIONS = [
  { ms: 2_000, label: '2s' },
  { ms: 5_000, label: '5s' },
  { ms: 15_000, label: '15s' },
  { ms: 60_000, label: '1m' },
  { ms: 0, label: 'Off' },
] as const;

const VALID = new Set<number>(REFRESH_OPTIONS.map((o) => o.ms));

function read(): number {
  const stored = Number(localStorage.getItem(KEY));
  return VALID.has(stored) ? stored : DEFAULT_REFRESH_MS;
}

let current = read();
const listeners = new Set<() => void>();

export function subscribeRefresh(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function setRefreshMs(ms: number) {
  current = VALID.has(ms) ? ms : DEFAULT_REFRESH_MS;
  localStorage.setItem(KEY, String(current));
  listeners.forEach((fn) => fn());
}

export function refreshMs(): number {
  return current;
}

/**
 * The interval to hand react-query, in the shape it wants: `false` pauses.
 *
 * `every` multiplies the base for data that costs more to produce than a read
 * from an informer cache — a Prometheus range query against a few hundred pods
 * has no business running as often as a list does.
 */
export function useRefreshInterval(every = 1): number | false {
  const ms = useSyncExternalStore(subscribeRefresh, refreshMs, () => DEFAULT_REFRESH_MS);
  return ms === 0 ? false : ms * every;
}
