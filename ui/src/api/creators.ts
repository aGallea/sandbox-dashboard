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

export function creators(): CreatorMark[] {
  return (typeof window !== 'undefined' && window.__CREATORS__) || [];
}
