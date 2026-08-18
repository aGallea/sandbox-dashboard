import type { ResourceKind } from '../api/client';

export interface ResourceConfig {
  kind: ResourceKind;
  label: string;          // human label in nav
  singular: string;       // human label in drawer / detail
  showPhase: boolean;     // false for templates (no Ready cond)
  showOsb: boolean;       // true only for sandboxes: creator + OpenSandbox columns
}

export const RESOURCES: Record<ResourceKind, ResourceConfig> = {
  sandboxes: {
    kind: 'sandboxes',
    label: 'Sandboxes',
    singular: 'Sandbox',
    showPhase: true,
    showOsb: true,
  },
  claims: {
    kind: 'claims',
    label: 'Claims',
    singular: 'SandboxClaim',
    showPhase: true,
    showOsb: false,
  },
  templates: {
    kind: 'templates',
    label: 'Templates',
    singular: 'SandboxTemplate',
    showPhase: false,
    showOsb: false,
  },
  warmpools: {
    kind: 'warmpools',
    label: 'Warm Pools',
    singular: 'SandboxWarmPool',
    showPhase: true,
    showOsb: false,
  },
};
