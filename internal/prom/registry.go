package prom

// Series is one labelled query that contributes to a chart.
type Series struct {
	Label string // human-readable label shown in the legend
	Query string // PromQL — never exposed to the client
}

// Metric is the unit of exposure: a name and one or more series.
type Metric struct {
	Name        string // stable kebab-cased name in the URL path
	Title       string // human-readable title for the chart
	Description string // one-line caption shown under the title
	Unit        string // y-axis unit hint, e.g. "ms" or "req/min"
	Series      []Series
}

// registry maps name → Metric.
var registry = map[string]Metric{
	"sandbox_creation_latency": {
		Name:        "sandbox_creation_latency",
		Title:       "Sandbox creation latency",
		Description: "Wall-clock time from Sandbox apply to Ready=True.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: "histogram_quantile(0.5, sum(rate(agent_sandbox_creation_latency_ms_bucket[5m])) by (le))"},
			{Label: "p95", Query: "histogram_quantile(0.95, sum(rate(agent_sandbox_creation_latency_ms_bucket[5m])) by (le))"},
		},
	},
	"claim_startup_latency": {
		Name:        "claim_startup_latency",
		Title:       "Claim startup latency",
		Description: "Time from SandboxClaim create to ServiceFQDN reachable.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: "histogram_quantile(0.5, sum(rate(agent_sandbox_claim_startup_latency_ms_bucket[5m])) by (le))"},
			{Label: "p95", Query: "histogram_quantile(0.95, sum(rate(agent_sandbox_claim_startup_latency_ms_bucket[5m])) by (le))"},
		},
	},
	"claim_controller_startup_latency": {
		Name:        "claim_controller_startup_latency",
		Title:       "Claim controller startup latency",
		Description: "Reconcile-side latency from create event to first Sandbox bound.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: "histogram_quantile(0.5, sum(rate(agent_sandbox_claim_controller_startup_latency_ms_bucket[5m])) by (le))"},
			{Label: "p95", Query: "histogram_quantile(0.95, sum(rate(agent_sandbox_claim_controller_startup_latency_ms_bucket[5m])) by (le))"},
		},
	},
	"claim_creation_rate": {
		Name:        "claim_creation_rate",
		Title:       "Claim creation rate",
		Description: "SandboxClaim creations per minute, summed across namespaces.",
		Unit:        "req/min",
		Series: []Series{
			{Label: "claims/min", Query: "sum(rate(agent_sandbox_claim_creation_total[1m])) * 60"},
		},
	},
}

// Lookup returns the Metric with the given name and whether it exists.
func Lookup(name string) (Metric, bool) {
	m, ok := registry[name]
	return m, ok
}

// All returns every known Metric.
func All() []Metric {
	out := make([]Metric, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	return out
}
