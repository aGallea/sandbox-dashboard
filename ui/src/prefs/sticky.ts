import { useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';

/**
 * Remembers the query string a page was left on, so a reload or a new tab opens
 * on the view you were using rather than the default one.
 *
 * The URL stays the source of truth and this only refills it when it is empty:
 * a link someone was sent always wins. Clearing every control is remembered as
 * such, so a page you deliberately reset does not come back filtered.
 *
 * ponytail: localStorage, one string per page. A server-side preference would
 * need an endpoint, a store and an identity the dashboard does not have.
 */

const PREFIX = 'view:';

/** Where you were, not how you look at it — never remembered. */
const EPHEMERAL = ['page'];

/** What to remember for a page whose query string is `search`. */
export function toStore(search: string): string {
  const next = new URLSearchParams(search);
  EPHEMERAL.forEach((k) => next.delete(k));
  return next.toString();
}

/** The query string to restore on arrival, or null to leave the URL alone. */
export function toRestore(search: string, saved: string | null): string | null {
  if (search) return null;
  return toStore(saved ?? '') || null;
}

/**
 * `page` names the page's own storage slot; the list pages pass their kind so
 * sandboxes and claims remember their filters separately.
 */
export function useStickyParams(page: string) {
  const [params, setParams] = useSearchParams();
  const key = PREFIX + page;
  // The key, not a flag: react-router reuses one ResourceListPage instance
  // across kinds, so a boolean would leave every kind but the first unrestored.
  const restoredFor = useRef<string | null>(null);

  useEffect(() => {
    const search = params.toString();
    if (restoredFor.current !== key) {
      restoredFor.current = key;
      const target = toRestore(search, localStorage.getItem(key));
      if (target) {
        setParams(target, { replace: true });
        return;
      }
    }
    localStorage.setItem(key, toStore(search));
  }, [params, setParams, key]);
}
