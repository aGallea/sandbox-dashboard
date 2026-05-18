export interface ResourceCounts {
  total: number;
  ready: number;
  notReady: number;
  unknown: number;
}

export interface TemplateCounts {
  total: number;
}

export interface WarmPoolCounts {
  total: number;
  replicas: number;
  readyReplicas: number;
}

export interface OverviewResponse {
  sandboxes: ResourceCounts;
  claims: ResourceCounts;
  templates: TemplateCounts;
  warmPools: WarmPoolCounts;
}

export async function fetchOverview(): Promise<OverviewResponse> {
  const res = await fetch('/api/v1/overview');
  if (!res.ok) {
    throw new Error(`overview failed: ${res.status} ${res.statusText}`);
  }
  return res.json();
}
