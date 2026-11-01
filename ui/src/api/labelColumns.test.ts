import { afterEach, describe, expect, it } from 'vitest';

import type { ResourceSummary } from './client';
import { labelColumns, labelValue } from './labelColumns';

const page = globalThis as { window?: { __LABEL_COLUMNS__?: unknown } };

afterEach(() => {
  delete page.window;
});

describe('labelColumns', () => {
  it('reads the columns the server wrote into the page', () => {
    page.window = { __LABEL_COLUMNS__: [{ name: 'Owner', labels: ['owner'] }] };
    expect(labelColumns()).toEqual([{ name: 'Owner', labels: ['owner'] }]);
  });

  it('is empty when the page carries none', () => {
    expect(labelColumns()).toEqual([]);
  });
});

describe('labelValue', () => {
  const owner = { name: 'Owner', labels: ['example.com/user', 'owner'] };
  const row = (labels?: Record<string, string>) => ({ labels }) as ResourceSummary;

  it('takes the first of its labels the sandbox has', () => {
    expect(labelValue(row({ 'example.com/user': 'a', owner: 'b' }), owner)).toBe('a');
    expect(labelValue(row({ owner: 'b' }), owner)).toBe('b');
  });

  it('skips a label set to the empty string', () => {
    expect(labelValue(row({ 'example.com/user': '', owner: 'b' }), owner)).toBe('b');
  });

  it('is empty for a sandbox with none of them', () => {
    expect(labelValue(row(), owner)).toBe('');
  });
});
