import { describe, expect, it } from 'vitest';

import type { ResourceSummary, UsageResponse } from '../api/client';
import {
  DONUT_MAX,
  OTHER_KEY,
  ageBuckets,
  alerts,
  formatAge,
  formatBytes,
  formatCores,
  groupBy,
  longestRunning,
  podPhases,
  reserved,
  shortImage,
  stateOf,
  used,
} from './aggregate';

/** A Ready sandbox with a Running pod; override only what a case is about. */
function sandbox(over: Partial<ResourceSummary> = {}): ResourceSummary {
  return {
    name: 'sb',
    namespace: 'default',
    kind: 'Sandbox',
    phase: 'Ready',
    ageSeconds: 60,
    ...over,
    // `'pod' in over` rather than a value check, so `{ pod: undefined }` really
    // means "no pod" instead of quietly getting the default one.
    pod: 'pod' in over
      ? over.pod
      : { name: 'sb-0', phase: 'Running', restarts: 0, cpuMillis: 0, memBytes: 0, gpu: 0 },
  };
}

describe('stateOf', () => {
  it('reports pending for a pod that has not started, whatever the sandbox phase says', () => {
    const waiting = sandbox({
      phase: 'Ready',
      pod: { name: 'p', phase: 'Pending', restarts: 0, cpuMillis: 0, memBytes: 0, gpu: 0 },
    });
    expect(stateOf(waiting)).toBe('pending');

    // A Running pod stuck pulling an image is pending too — the reason outranks the phase.
    const pulling = sandbox({
      pod: {
        name: 'p',
        phase: 'Running',
        restarts: 0,
        cpuMillis: 0,
        memBytes: 0,
        gpu: 0,
        waitingReason: 'ImagePullBackOff',
      },
    });
    expect(stateOf(pulling)).toBe('pending');
  });

  it('falls back to the sandbox phase when the pod is unremarkable', () => {
    expect(stateOf(sandbox())).toBe('ready');
    expect(stateOf(sandbox({ phase: 'NotReady' }))).toBe('notReady');
    expect(stateOf(sandbox({ phase: 'Scaling' }))).toBe('unknown');
    expect(stateOf(sandbox({ phase: 'Ready', pod: undefined }))).toBe('ready');
  });
});

describe('groupBy', () => {
  const dim = { key: 'team', label: 'Team', of: (it: ResourceSummary) => it.team ?? '' };

  it('sums reservations per group and orders by size', () => {
    const items = [
      sandbox({ team: 'a', pod: { name: 'p1', phase: 'Running', restarts: 0, cpuMillis: 500, memBytes: 1024, gpu: 1 } }),
      sandbox({ team: 'a', pod: { name: 'p2', phase: 'Running', restarts: 0, cpuMillis: 250, memBytes: 512, gpu: 0 } }),
      sandbox({ team: 'b' }),
    ];
    const [first, second] = groupBy(items, dim);
    expect(first).toMatchObject({ key: 'a', count: 2, cpuMillis: 750, memBytes: 1536, gpu: 1 });
    expect(second).toMatchObject({ key: 'b', count: 1 });
  });

  it('labels a missing dimension value rather than grouping it under the empty string', () => {
    expect(groupBy([sandbox()], dim)[0].key).toBe('unset');
  });

  it('folds everything past the limit into one Other slice', () => {
    const items = ['a', 'b', 'c'].flatMap((team) => [sandbox({ team }), sandbox({ team })]);
    const slices = groupBy(items, dim, 2);
    expect(slices).toHaveLength(3);
    expect(slices[2]).toMatchObject({ key: OTHER_KEY, count: 2 });
  });

  it('assigns colours by name so a refresh cannot repaint a group the reader knows', () => {
    const many = [sandbox({ team: 'a' }), sandbox({ team: 'a' }), sandbox({ team: 'b' })];
    const byCount = groupBy(many, dim);
    // Same fleet, opposite arrival order: 'b' now leads on count.
    const flipped = groupBy([...many.slice(2), ...many.slice(0, 2), sandbox({ team: 'b' })], dim);
    const colourOf = (s: { key: string; color: string }[], k: string) =>
      s.find((x) => x.key === k)!.color;
    expect(colourOf(flipped, 'a')).toBe(colourOf(byCount, 'a'));
    expect(colourOf(flipped, 'b')).toBe(colourOf(byCount, 'b'));
  });

  it('joins live usage by namespace/pod, ignoring samples for other pods', () => {
    const items = [
      sandbox({ namespace: 'ns', team: 'a', pod: { name: 'p1', phase: 'Running', restarts: 0, cpuMillis: 0, memBytes: 0, gpu: 0 } }),
    ];
    const usage: UsageResponse = {
      sampledAt: '2026-08-23T00:00:00Z',
      pods: { 'ns/p1': { cpuCores: 1.5, memBytes: 2048 }, 'ns/other': { cpuCores: 99, memBytes: 99 } },
    };
    expect(groupBy(items, dim, 5, usage)[0]).toMatchObject({ usedCores: 1.5, usedBytes: 2048 });
  });
});

describe('podPhases', () => {
  it('counts sandboxes with no pod as their own phase', () => {
    const slices = podPhases([sandbox(), sandbox({ pod: undefined })]);
    expect(slices.map((s) => [s.key, s.count])).toEqual(
      expect.arrayContaining([
        ['Running', 1],
        ['No pod', 1],
      ]),
    );
  });
});

describe('ageBuckets', () => {
  it('places each age in exactly one bucket, including the open-ended last one', () => {
    const buckets = ageBuckets([
      sandbox({ ageSeconds: 0 }),
      sandbox({ ageSeconds: 1_799 }),
      sandbox({ ageSeconds: 1_800 }),
      sandbox({ ageSeconds: 86_400 }),
      sandbox({ ageSeconds: 10_000_000 }),
    ]);
    expect(buckets.map((b) => b.count)).toEqual([2, 1, 0, 0, 2]);
  });
});

describe('longestRunning', () => {
  it('returns the oldest first without reordering the caller’s array', () => {
    const items = [sandbox({ ageSeconds: 1 }), sandbox({ ageSeconds: 9 }), sandbox({ ageSeconds: 5 })];
    expect(longestRunning(items, 2).map((it) => it.ageSeconds)).toEqual([9, 5]);
    expect(items.map((it) => it.ageSeconds)).toEqual([1, 9, 5]);
  });
});

describe('reserved and used', () => {
  it('sums requests across pods, skipping sandboxes that have none', () => {
    const items = [
      sandbox({ pod: { name: 'p', phase: 'Running', restarts: 0, cpuMillis: 100, memBytes: 8, gpu: 2 } }),
      sandbox({ pod: undefined }),
    ];
    expect(reserved(items)).toEqual({ cpuMillis: 100, memBytes: 8, gpu: 2 });
  });

  it('returns undefined usage when Prometheus is absent, so the caller can hide the panel', () => {
    expect(used([sandbox()], undefined)).toBeUndefined();
  });

  it('sums only the sandbox pods present in the usage response', () => {
    const usage: UsageResponse = {
      sampledAt: '2026-08-23T00:00:00Z',
      pods: { 'default/sb-0': { cpuCores: 2, memBytes: 4 }, 'default/stranger': { cpuCores: 7, memBytes: 7 } },
    };
    expect(used([sandbox()], usage)).toEqual({ cpuCores: 2, memBytes: 4 });
  });
});

describe('alerts', () => {
  it('drops the zero rows so a quiet fleet shows nothing to act on', () => {
    expect(alerts([sandbox()])).toEqual([]);
  });

  it('leads with the bad states and keeps every non-zero warning', () => {
    const found = alerts([
      sandbox({ phase: 'NotReady' }),
      sandbox({ pod: { name: 'p', phase: 'Running', restarts: 3, cpuMillis: 0, memBytes: 0, gpu: 0 } }),
      sandbox({ ageSeconds: 90_000 }),
    ]);
    expect(found[0]).toMatchObject({ label: 'not ready', count: 1, tone: 'bad' });
    expect(found.map((a) => a.label)).toEqual(['not ready', 'restarting', 'running over a day']);
  });
});

describe('formatting', () => {
  it('shows sub-core reservations in millicores and whole cores without a decimal', () => {
    expect(formatCores(0)).toBe('0');
    expect(formatCores(999)).toBe('999m');
    expect(formatCores(1_000)).toBe('1');
    expect(formatCores(1_500)).toBe('1.5');
  });

  it('picks a byte unit that keeps three significant digits', () => {
    expect(formatBytes(512 * 1024 ** 2)).toBe('512 MiB');
    expect(formatBytes(1.5 * 1024 ** 3)).toBe('1.5 GiB');
    expect(formatBytes(64 * 1024 ** 3)).toBe('64 GiB');
    expect(formatBytes(2 * 1024 ** 4)).toBe('2.0 TiB');
  });

  it('drops the smallest unit once a coarser one is meaningful', () => {
    expect(formatAge(59)).toBe('59s');
    expect(formatAge(60)).toBe('1m');
    expect(formatAge(3_660)).toBe('1h 1m');
    expect(formatAge(90_000)).toBe('1d 1h');
  });

  it('reduces an image reference to the workload name', () => {
    expect(shortImage()).toBe('');
    expect(shortImage('ghcr.io/org/tool:v1')).toBe('tool:v1');
    expect(shortImage('ghcr.io/org/tool@sha256:abc')).toBe('tool');
  });
});

it('keeps the donut group cap above the default groupBy limit', () => {
  // groupBy(…, 5) plus Other must still fit the donut, or a slice goes missing.
  expect(DONUT_MAX).toBeGreaterThanOrEqual(6);
});
