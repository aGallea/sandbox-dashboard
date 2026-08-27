/**
 * The URL prefix this dashboard is served under, e.g. "/sandbox-dashboard", or
 * "" at a domain root.
 *
 * Read at runtime rather than baked in at build time, because one published
 * image is installed at different prefixes on different clusters — a Vite
 * `base` would make the image itself path-specific. The server writes the value
 * into index.html; see serveIndex in internal/server/router.go.
 *
 * Assets need no help from this: Vite emits relative URLs and the server also
 * injects a matching <base href>. This is for the two things that resolve
 * outside the document — fetch() and the router.
 */
declare global {
  interface Window {
    __BASE_PATH__?: string;
  }
}

/**
 * Normalised the same way the server normalises it: "" or "/prefix".
 *
 * Read on each call rather than captured at module load, so a test can set it
 * without controlling import order.
 */
export function basePath(): string {
  const raw = (typeof window !== 'undefined' && window.__BASE_PATH__) || '';
  const trimmed = raw.replace(/^\/+|\/+$/g, '');
  return trimmed ? `/${trimmed}` : '';
}

/** Absolute URL for an API path, honouring the prefix. */
export function apiUrl(path: string): string {
  return `${basePath()}${path}`;
}
