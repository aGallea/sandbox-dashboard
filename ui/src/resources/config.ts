import type { ResourceKind } from '../api/client';

export interface ResourceConfig {
  kind: ResourceKind;
  label: string;          // human label in nav
  singular: string;       // human label in drawer / detail
  showPhase: boolean;     // false for templates (no Ready cond)
  phases: string[];       // filter values for the phase dropdown; [] for templates
}

export const RESOURCES: Record<ResourceKind, ResourceConfig> = {
  sandboxes: {
    kind: 'sandboxes',
    label: 'Sandboxes',
    singular: 'Sandbox',
    showPhase: true,
    phases: ['Ready', 'NotReady', 'Unknown'],
  },
  claims: {
    kind: 'claims',
    label: 'Claims',
    singular: 'SandboxClaim',
    showPhase: true,
    phases: ['Ready', 'NotReady', 'Unknown'],
  },
  templates: {
    kind: 'templates',
    label: 'Templates',
    singular: 'SandboxTemplate',
    showPhase: false,
    phases: [],
  },
  warmpools: {
    kind: 'warmpools',
    label: 'Warm Pools',
    singular: 'SandboxWarmPool',
    showPhase: true,
    phases: ['Ready', 'Scaling', 'Unknown'],
  },
};
