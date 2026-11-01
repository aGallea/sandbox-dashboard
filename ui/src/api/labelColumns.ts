import type { ResourceSummary } from './client';

/**
 * A list column read from a sandbox's labels: the first of `labels` the sandbox
 * has. Which labels say who owns a sandbox differs per fleet, so the install
 * names them (`--label-columns`), not the dashboard.
 */
export interface LabelColumn {
  name: string;
  labels: string[];
}

declare global {
  interface Window {
    __LABEL_COLUMNS__?: LabelColumn[];
  }
}

/** The columns the server wrote into index.html; none when it wrote nothing. */
export function labelColumns(): LabelColumn[] {
  return (typeof window !== 'undefined' && window.__LABEL_COLUMNS__) || [];
}

/** A sandbox's value for a column, or '' when it has none of its labels. */
export function labelValue(it: ResourceSummary, column: LabelColumn): string {
  for (const key of column.labels) {
    const value = it.labels?.[key];
    if (value) return value;
  }
  return '';
}
