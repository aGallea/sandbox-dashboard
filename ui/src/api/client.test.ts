import { afterEach, describe, expect, it, vi } from 'vitest';

import { apiUrl, basePath } from './basePath';
import { fetchList, fetchOverview, fetchUsage } from './client';

/** Pretends the page was served under a prefix, the way index.html reports it. */
function servedUnder(prefix: string) {
  (globalThis as { window?: { __BASE_PATH__?: string } }).window = { __BASE_PATH__: prefix };
}

/** Captures the URL a fetch was called with and answers with empty JSON. */
function captureFetch(): { urls: string[] } {
  const urls: string[] = [];
  vi.stubGlobal('fetch', (input: string) => {
    urls.push(String(input));
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
  });
  return { urls };
}

afterEach(() => {
  delete (globalThis as { window?: unknown }).window;
  vi.unstubAllGlobals();
});

describe('basePath', () => {
  it('is empty at the domain root, so apiUrl is a no-op', () => {
    expect(basePath()).toBe('');
    expect(apiUrl('/api/v1/overview')).toBe('/api/v1/overview');
  });

  it('normalises whatever the server reported', () => {
    for (const given of ['/sandbox-dashboard', 'sandbox-dashboard', '/sandbox-dashboard/']) {
      servedUnder(given);
      expect(basePath(), given).toBe('/sandbox-dashboard');
    }
  });
});

/**
 * A request that skips the prefix still works at the domain root, so it passes
 * every other test and every manual check — it only 404s on installs that use a
 * prefix, which is where nobody is looking.
 *
 * `fetchList` is here by name because it is the one that got missed: it builds
 * its URL through a nested template literal, so a sweep of the obvious call
 * shapes did not touch it, and the sandbox list 404'd behind a prefix while the
 * assets and every other endpoint resolved fine.
 */
describe('every request carries the prefix', () => {
  it('prefixes the list, the overview and the usage endpoints', async () => {
    servedUnder('/sandbox-dashboard');
    const { urls } = captureFetch();

    await fetchList('sandboxes');
    await fetchList('sandboxes', { stale: true });
    await fetchOverview();
    await fetchUsage();

    expect(urls).toEqual([
      '/sandbox-dashboard/api/v1/sandboxes',
      '/sandbox-dashboard/api/v1/sandboxes?stale=true',
      '/sandbox-dashboard/api/v1/overview',
      '/sandbox-dashboard/api/v1/usage',
    ]);
  });

  it('leaves the URLs alone at the domain root', async () => {
    const { urls } = captureFetch();
    await fetchList('claims');
    await fetchOverview();
    expect(urls).toEqual(['/api/v1/claims', '/api/v1/overview']);
  });
});
