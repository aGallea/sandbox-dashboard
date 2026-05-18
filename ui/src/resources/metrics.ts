// Hardcoded mirror of the backend registry. Order matters — it determines
// rendering order on the Metrics page.
export const METRIC_NAMES = [
  'sandbox_creation_latency',
  'claim_startup_latency',
  'claim_controller_startup_latency',
  'claim_creation_rate',
] as const;

export type MetricName = (typeof METRIC_NAMES)[number];
