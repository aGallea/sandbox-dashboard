import { describe, expect, it } from 'vitest';

import type { ResourceSummary, UsageResponse } from '../api/client';
import {
  DONUT_MAX,
  OTHER_KEY,
  SERIES,
  STARTUP_GRACE_SECONDS,
  ageBuckets,
  alerts,
  diagnose,
  dimensionsFor,
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
  type Slice,
  valueCounts,
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

  // The overview polls every few seconds. Anything that reorders equal groups or
  // moves a hue between them shows up as flicker — measured on a live fleet
  // grouped by node, ten recolourings in eight polls, with every row otherwise
  // unchanged.
  describe('is stable across a refresh that changes nothing the reader can see', () => {
    it('breaks count ties on the key, so equal groups cannot swap places', () => {
      const fleet = ['b', 'a', 'c'].map((team) => sandbox({ team }));
      const order = (items: ResourceSummary[]) => groupBy(items, dim).map((s) => s.key);

      expect(order(fleet)).toEqual(['a', 'b', 'c']);
      // Same fleet, different arrival order — the informer does not promise one.
      expect(order([...fleet].reverse())).toEqual(['a', 'b', 'c']);
    });

    it('keeps a group its colour when a smaller group joins below it', () => {
      const before = groupBy([sandbox({ team: 'a' }), sandbox({ team: 'a' })], dim);
      const after = groupBy(
        [sandbox({ team: 'a' }), sandbox({ team: 'a' }), sandbox({ team: 'b' })],
        dim,
      );
      const colourOf = (slices: Slice[], key: string) => slices.find((s) => s.key === key)?.color;

      expect(colourOf(after, 'a')).toBe(colourOf(before, 'a'));
    });

    // SERIES holds five hues and the limit keeps five groups, so colour cannot
    // also be pinned to the name — with a full list any membership change would
    // force a reshuffle. Rank is what the reader is looking at, so rank wins.
    it('gives the largest group the first hue, whatever it is called', () => {
      const zed = groupBy([sandbox({ team: 'z' }), sandbox({ team: 'z' }), sandbox({ team: 'a' })], dim);
      expect(zed[0]).toMatchObject({ key: 'z', color: SERIES[0] });
      expect(zed[1]).toMatchObject({ key: 'a', color: SERIES[1] });
    });
  });

  it('assigns colours by rank so the same fleet always paints the same way', () => {
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
      sandbox({ phase: 'NotReady', ageSeconds: 600 }),
      sandbox({ pod: { name: 'p', phase: 'Running', restarts: 3, cpuMillis: 0, memBytes: 0, gpu: 0 } }),
      sandbox({ ageSeconds: 90_000 }),
    ]);
    expect(found[0]).toMatchObject({ count: 1, tone: 'bad' });
    expect(found.map((a) => a.label)).toEqual([
      'not ready for over 5m',
      'restarting',
      'running over a day',
    ]);
  });

  // A fleet that churns from 0 to 300 sandboxes in hours always has a few
  // sandboxes seconds into starting up. Counting those as "not ready" leaves the
  // chip permanently red, which teaches the reader to ignore it.
  describe('readiness is judged against how long the sandbox has had', () => {
    it('does not raise a sandbox still inside the startup grace period', () => {
      const found = alerts([sandbox({ phase: 'NotReady', ageSeconds: 8 })]);
      expect(found.map((a) => a.tone)).not.toContain('bad');
      expect(found).toEqual([{ label: 'still starting', count: 1, tone: 'mute' }]);
    });

    it('raises the same sandbox once it has had long enough', () => {
      const found = alerts([sandbox({ phase: 'NotReady', ageSeconds: STARTUP_GRACE_SECONDS + 1 })]);
      expect(found).toEqual([
        {
          label: 'not ready for over 5m',
          count: 1,
          tone: 'bad',
          to: '/sandboxes?f_phase=NotReady',
        },
      ]);
    });

    it('counts a pod stuck in Pending the same way', () => {
      const pending = { name: 'p', phase: 'Pending', restarts: 0, cpuMillis: 0, memBytes: 0, gpu: 0 };
      expect(alerts([sandbox({ ageSeconds: 8, pod: pending })])[0].tone).toBe('mute');
      expect(alerts([sandbox({ ageSeconds: 900, pod: pending })])[0].tone).toBe('bad');
    });
  });
});

describe('diagnose', () => {
  const podWith = (over: Partial<NonNullable<ResourceSummary['pod']>>) => ({
    name: 'p',
    phase: 'Running',
    restarts: 0,
    cpuMillis: 0,
    memBytes: 0,
    gpu: 0,
    ...over,
  });

  it('calls a ready sandbox ready', () => {
    expect(diagnose(sandbox()).verdict).toBe('ready');
  });

  // The distinction the fleet strip exists to make: a sandbox seconds into
  // pulling an image looks identical to one that will never start, and only one
  // of them is worth waking up for.
  it('separates a sandbox still starting from one that has had long enough', () => {
    const young = sandbox({ phase: 'NotReady', ageSeconds: 20, pod: podWith({ phase: 'Pending' }) });
    expect(diagnose(young).verdict).toBe('starting');

    const old = sandbox({ phase: 'NotReady', ageSeconds: 900, pod: podWith({ phase: 'Pending' }) });
    expect(diagnose(old).verdict).toBe('stuck');
  });

  it('calls a pod that cannot pull its image failing, however young it is', () => {
    const it_ = sandbox({
      phase: 'NotReady',
      ageSeconds: 5,
      pod: podWith({ waitingReason: 'ImagePullBackOff' }),
    });
    expect(diagnose(it_)).toMatchObject({ verdict: 'failing', why: 'ImagePullBackOff' });
  });

  it('calls a restarting pod failing even while Kubernetes reports it ready', () => {
    const it_ = sandbox({ phase: 'Ready', pod: podWith({ restarts: 4 }) });
    expect(diagnose(it_)).toMatchObject({ verdict: 'failing' });
    expect(diagnose(it_).why).toContain('4');
  });

  it('trusts an OpenSandbox failure over the phase', () => {
    const it_ = sandbox({
      phase: 'NotReady',
      ageSeconds: 5,
      osb: { state: 'Failed', stateAgeSeconds: 5, diverged: false, stale: false },
    });
    expect(diagnose(it_).verdict).toBe('failing');
  });

  it('treats a transient OpenSandbox state that stopped advancing as stuck', () => {
    const it_ = sandbox({
      phase: 'NotReady',
      ageSeconds: 30,
      osb: { state: 'Pausing', stateAgeSeconds: 30, diverged: false, stale: true },
    });
    expect(diagnose(it_).verdict).toBe('stuck');
  });
});

describe('dimensionsFor', () => {
  // Ranking readable-first alone put `swe-instance-id` — set on 3 of 59
  // sandboxes — ahead of `team`, so the overview opened on a ring that was 89%
  // "unset". A dimension nearly nothing carries cannot be the default.
  it('prefers the dimension the most sandboxes actually carry', () => {
    const items = [
      ...Array.from({ length: 4 }, () => sandbox({ labels: { team: 'algo' } })),
      ...Array.from({ length: 5 }, () => sandbox({ labels: { team: 'gateway' } })),
      sandbox({ labels: { team: 'algo', 'swe-instance-id': 'a' } }),
      sandbox({ labels: { 'swe-instance-id': 'b' } }),
    ];
    const found = dimensionsFor(items);
    expect(found[0].key).toBe('label:team');
    expect(found.map((d) => d.key)).toContain('label:swe-instance-id');
  });

  // Coverage is a tie-breaker among labels, not a way for an intrinsic field to
  // outrank them: every pod has a CPU request, so ranking on coverage alone
  // opened the page on "CPU request", 90% of it one value.
  it('keeps a stamped label ahead of a field every pod happens to have', () => {
    const items = [
      ...Array.from({ length: 5 }, () => sandbox({ labels: { team: 'algo' } })),
      ...Array.from({ length: 4 }, () => sandbox({ labels: { team: 'gateway' } })),
      sandbox({ pod: { name: 'p', phase: 'Running', restarts: 0, cpuMillis: 4000, memBytes: 0, gpu: 0 } }),
    ];
    expect(dimensionsFor(items)[0].key).toBe('label:team');
  });

  it('reports how much of the fleet a dimension covers', () => {
    const items = [sandbox({ labels: { team: 'algo' } }), sandbox({ labels: { team: 'gw' } }), sandbox()];
    expect(dimensionsFor(items).find((d) => d.key === 'label:team')?.covered).toBe(2);
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

describe('valueCounts', () => {
  it('lists every value of a dimension, largest first, with the blank folded into unset', () => {
    const items = [
      sandbox({ labels: { owner: 'b' } }),
      sandbox({ labels: { owner: 'a' } }),
      sandbox({ labels: { owner: 'a' } }),
      sandbox({}),
    ];
    const owner = dimensionsFor(items).find((d) => d.key === 'label:owner')!;
    expect(valueCounts(items, owner)).toEqual([
      { value: 'a', count: 2 },
      { value: 'b', count: 1 },
      { value: 'unset', count: 1 },
    ]);
  });
});
