import { describe, expect, it } from 'vitest';

import { toRestore, toStore } from './sticky';

describe('toStore', () => {
  it('keeps the controls and drops the page you happened to be on', () => {
    expect(toStore('sort=owner&dir=desc&page=7')).toBe('sort=owner&dir=desc');
  });

  // Clearing every filter has to be remembered as cleared, or the page comes
  // back filtered on the next reload and there is no way to make it stop.
  it('remembers an empty view as empty', () => {
    expect(toStore('')).toBe('');
  });
});

describe('toRestore', () => {
  it('refills an empty URL from what was stored', () => {
    expect(toRestore('', 'by=owner&v=yuvalg')).toBe('by=owner&v=yuvalg');
  });

  it('leaves a URL that already carries a view alone, so a shared link wins', () => {
    expect(toRestore('by=node', 'by=owner')).toBeNull();
  });

  it('restores nothing when nothing was stored', () => {
    expect(toRestore('', null)).toBeNull();
    expect(toRestore('', '')).toBeNull();
  });

  it('drops a page number left in an older stored view', () => {
    expect(toRestore('', 'sort=owner&page=7')).toBe('sort=owner');
  });
});
