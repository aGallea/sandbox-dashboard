/**
 * A creator the install names (`--creators`). The server matches sandboxes to
 * it; the browser only needs what to show.
 */
export interface CreatorMark {
  name: string;
  icon?: string;
}

declare global {
  interface Window {
    __CREATORS__?: CreatorMark[];
  }
}

/** The creators the server wrote into index.html; none when it wrote nothing. */
export function creators(): CreatorMark[] {
  return (typeof window !== 'undefined' && window.__CREATORS__) || [];
}
