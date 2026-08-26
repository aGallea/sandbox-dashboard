import { describe, expect, it } from 'vitest';

import type { ResourceSummary, UsageResponse } from '../api/client';
import { informative, matchesQuery, podSample, shortId, shortNode, taskLabel } from './rows';

function sandbox(over: Partial<ResourceSummary> = {}): ResourceSummary {
  return {
    name: 'sandbox-11d3d4d8-ab61-415d-94f3-2901faa68cd2',
    namespace: 'default',
    kind: 'Sandbox',
    phase: 'Ready',
    ageSeconds: 60,
    ...over,
  };
}

describe('taskLabel', () => {
  it('names a sandbox by the task its session id starts with', () => {
    expect(taskLabel(sandbox({ sessionId: 'build-pmars__Ry5KMzg__env' }))).toEqual({
      label: 'build-pmars',
      token: 'Ry5KMzg',
    });
  });

  it('drops the hex blobs that swe-bench style session ids carry', () => {
    const it = sandbox({
      sessionId:
        'instance_tutao__tutanota-b4934a0f3c34d9d7649e944b183137e8fad3e859-vbc0d9ba8f0071fbe982809910959a6ff8884dbbf',
    });
    expect(taskLabel(it)).toEqual({ label: 'instance_tutao', token: '' });
  });

  it('clips a label too long to read and keeps the rest for the tooltip', () => {
    const long = 'a'.repeat(40);
    const { label } = taskLabel(sandbox({ sessionId: long }));
    expect(label).toHaveLength(34);
    expect(label.endsWith('…')).toBe(true);
  });

  it('falls back to the image when the sandbox carries no session', () => {
    const it = sandbox({ pod: pod({ image: 'alexgshaw/build-pmars:20251031' }) });
    expect(taskLabel(it).label).toBe('build-pmars');
  });

  // Sandboxes with no session id carry the same hex in the image tag instead, so
  // the fallback has to be cleaned by the same rule as the session.
  it('drops the hex blobs an image name carries too', () => {
    const it = sandbox({
      pod: pod({
        image:
          'us-docker.pkg.dev/p/studio-docker/instance_internetarchive__openlibrary-4a5d2a7d24c9e4c11d3069220c0685b736d5ecde-v13642507b4fc1f8d234172bf8129942da2c2ca26:latest',
      }),
    });
    expect(taskLabel(it).label).toBe('instance_internetarchive');
  });

  it('falls back to the object name when there is neither session nor image', () => {
    expect(taskLabel(sandbox({ name: 'warmpool-a' })).label).toBe('warmpool-a');
  });
});

describe('shortId', () => {
  it('picks the leading hex block out of a generated name', () => {
    expect(shortId('sandbox-11d3d4d8-ab61-415d-94f3-2901faa68cd2')).toBe('11d3d4d8');
    expect(shortId('fa53b8e9-8f4c-4361-9712-30e0a33909e2')).toBe('fa53b8e9');
  });

  // A name someone chose is already readable, so a truncation of it is noise.
  it('is empty for a name that is not a generated id', () => {
    expect(shortId('my-sandbox')).toBe('');
  });
});

describe('shortNode', () => {
  it('keeps the part of a node name that tells two nodes apart', () => {
    expect(shortNode('gke-main-nap-t2d-standar-32-9qmzbrd3-5e131c93-6957')).toBe('…5e131c93-6957');
  });

  it('is empty when the sandbox has no node', () => {
    expect(shortNode(undefined)).toBe('');
  });

  it('leaves a short node name alone', () => {
    expect(shortNode('node-1')).toBe('node-1');
  });
});

describe('matchesQuery', () => {
  const it_ = sandbox({
    sessionId: 'build-pmars__Ry5KMzg__env',
    owner: 'yuvalg',
    team: 'intelligent-gateway',
    pod: pod({ image: 'alexgshaw/build-pmars:20251031', node: 'gke-main-static-16-3e7c5225-jl6m' }),
  });

  it('matches every field a person might search by', () => {
    ['pmars', 'yuvalg', 'gateway', '3e7c5225', '11d3d4d8', 'alexgshaw'].forEach((q) =>
      expect(matchesQuery(it_, q)).toBe(true),
    );
  });

  it('ignores case and surrounding space', () => {
    expect(matchesQuery(it_, '  YUVALG ')).toBe(true);
  });

  it('does not match an unrelated term', () => {
    expect(matchesQuery(it_, 'tensorflow')).toBe(false);
  });

  // The overview hands a group's value to this search to drill into it, and the
  // groups are discovered from whatever labels a fleet stamps — so a label this
  // code has never heard of still has to be searchable.
  it('matches the value of any label the fleet stamps', () => {
    const labelled = sandbox({ labels: { project: 'e2e-eval', 'swe-instance-id': 'flipt-c1fd' } });
    expect(matchesQuery(labelled, 'e2e-eval')).toBe(true);
    expect(matchesQuery(labelled, 'flipt')).toBe(true);
  });

  it('keeps every row when the query is empty', () => {
    expect(matchesQuery(sandbox(), '')).toBe(true);
  });
});

describe('informative', () => {
  const ns = (it: ResourceSummary) => it.namespace;

  it('is false for a column whose every row says the same thing', () => {
    expect(informative([sandbox(), sandbox()], ns)).toBe(false);
  });

  it('is true once a column separates two rows', () => {
    expect(informative([sandbox(), sandbox({ namespace: 'team-a' })], ns)).toBe(true);
  });

  it('does not count blanks as a value of their own', () => {
    const owner = (it: ResourceSummary) => it.owner ?? '';
    expect(informative([sandbox({ owner: 'yuvalg' }), sandbox()], owner)).toBe(false);
    expect(informative([sandbox(), sandbox()], owner)).toBe(false);
  });
});

describe('podSample', () => {
  const usage: UsageResponse = {
    sampledAt: '2026-08-26T10:00:00Z',
    pods: { 'default/p1': { cpuCores: 0.25, memBytes: 1024 } },
  };

  it('finds the sample belonging to the sandbox pod', () => {
    expect(podSample(sandbox({ pod: pod({ name: 'p1' }) }), usage)).toEqual({
      cpuCores: 0.25,
      memBytes: 1024,
    });
  });

  // Absent and zero are different answers: one means Prometheus has nothing for
  // this pod yet, the other means the pod is genuinely idle.
  it('is undefined when the pod has no sample, no pod, or no Prometheus', () => {
    expect(podSample(sandbox({ pod: pod({ name: 'other' }) }), usage)).toBeUndefined();
    expect(podSample(sandbox(), usage)).toBeUndefined();
    expect(podSample(sandbox({ pod: pod({ name: 'p1' }) }), undefined)).toBeUndefined();
  });
});

function pod(over: Partial<NonNullable<ResourceSummary['pod']>> = {}) {
  return {
    name: 'sandbox-11d3d4d8-ab61-415d-94f3-2901faa68cd2',
    phase: 'Running',
    restarts: 0,
    cpuMillis: 1000,
    memBytes: 2 * 1024 ** 3,
    gpu: 0,
    ...over,
  };
}
