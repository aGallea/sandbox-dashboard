package prom

import (
	"fmt"
	"regexp"
)

// Series is one labelled query that contributes to a chart. Each query must
// return exactly one series: the chart draws one line per entry here, and
// QueryRange reads the first series of the result.
type Series struct {
	Label string // human-readable label shown in the legend
	Query string // PromQL — never exposed to the client
}

// Metric is the unit of exposure: a name and one or more series.
type Metric struct {
	Name        string // stable kebab-cased name in the URL path
	Title       string // human-readable title for the chart
	Description string // one-line caption shown under the title
	Unit        string // y-axis unit hint, e.g. "ms" or "cores"
	Series      []Series
}

// Section groups the metrics a reader looks at together.
type Section struct {
	Name    string
	Note    string // why these belong together, or when they stay empty
	Metrics []Metric
}

// DefaultControllerJob is the scrape job the agent-sandbox controller lands in
// under the upstream Helm chart and the manifests in deploy/.
const DefaultControllerJob = "agent-sandbox-controller"

// SandboxPodLabelSelector matches the kube-state-metrics series for pods the
// agent-sandbox controller owns.
//
// Pod-level series from cAdvisor carry no sandbox identity — only namespace and
// pod — so a fleet chart has to join against kube_pod_labels to avoid counting
// every other pod in the namespace. The label is the one the controller stamps
// on every pod it creates (agents.x-k8s.io/sandbox-name-hash), in the
// underscored form kube-state-metrics exports. It requires a
// kube-state-metrics that exposes pod labels; where it does not, these charts
// report no samples rather than a wrong number.
const SandboxPodLabelSelector = `kube_pod_labels{label_agents_x_k8s_io_sandbox_name_hash!=""}`

// labelValue is the safe shape for a value interpolated into a PromQL matcher.
var labelValue = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)

// Registry holds the whitelisted metrics for one dashboard install. It is built
// per install because the controller's scrape job label is a deployment detail:
// the controller_runtime_* names are shared by every controller-runtime binary
// in the cluster, so without the job matcher the controller charts would sum
// unrelated controllers' work.
type Registry struct {
	sections []Section
	byName   map[string]Metric
}

// NewRegistry builds the registry, scoping the controller charts to the given
// scrape job. A job name that is not a usable label value falls back to
// DefaultControllerJob rather than being interpolated as-is.
func NewRegistry(controllerJob string) *Registry {
	if !labelValue.MatchString(controllerJob) {
		controllerJob = DefaultControllerJob
	}
	sections := []Section{
		{
			Name: "Fleet",
			Note: "The sandbox fleet over time — the counts the controller reports, and what its pods reserve against what they use.",
			Metrics: []Metric{
				fleetSize(),
				fleetExpired(),
				fleetCPU(),
				fleetMemory(),
			},
		},
		{
			Name: "Controller",
			Note: "Whether the agent-sandbox controller is keeping up with the fleet.",
			Metrics: []Metric{
				reconcileLatency(controllerJob),
				reconcileRate(controllerJob),
				queueWait(controllerJob),
			},
		},
		{
			Name: "Claims",
			Note: "Recorded only when sandboxes are launched through a SandboxClaim. A fleet that creates Sandbox objects directly leaves these empty — the controller never observes them, so no series exists to draw.",
			Metrics: []Metric{
				claimStartupLatency(),
				sandboxCreationLatency(),
				claimCreationRate(),
			},
		},
	}

	byName := map[string]Metric{}
	for _, s := range sections {
		for _, m := range s.Metrics {
			byName[m.Name] = m
		}
	}
	return &Registry{sections: sections, byName: byName}
}

// Lookup returns the Metric with the given name and whether it exists.
func (r *Registry) Lookup(name string) (Metric, bool) {
	m, ok := r.byName[name]
	return m, ok
}

// All returns every known Metric, in no particular order.
func (r *Registry) All() []Metric {
	out := make([]Metric, 0, len(r.byName))
	for _, m := range r.byName {
		out = append(out, m)
	}
	return out
}

// Sections returns the metrics grouped for display, in reading order.
func (r *Registry) Sections() []Section {
	return r.sections
}

// ----- fleet ---------------------------------------------------------------

// zeroFloor keeps a count or rate readable when its label combination has not
// appeared yet. Prometheus returns no series for `agent_sandboxes{expired="true"}`
// on a fleet where nothing has expired, which draws an empty chart although the
// honest answer is zero.
//
// It belongs on counts and rates only. Flooring a latency quantile would draw a
// flat 0 ms line — claiming instant where the truth is "never measured".
func zeroFloor(query string) string {
	return query + " or vector(0)"
}

func fleetSize() Metric {
	return Metric{
		Name:        "fleet_size",
		Title:       "Sandboxes",
		Description: "Sandboxes in the cluster, as the controller counts them.",
		Unit:        "sandboxes",
		Series: []Series{
			{Label: "ready", Query: zeroFloor(`sum(agent_sandboxes{ready_condition="true"})`)},
			{Label: "not ready", Query: zeroFloor(`sum(agent_sandboxes{ready_condition="false"})`)},
		},
	}
}

func fleetExpired() Metric {
	return Metric{
		Name:        "fleet_expired",
		Title:       "Expired sandboxes",
		Description: "Past their shutdown time and kept by a Retain policy. A rising line is a fleet nobody is collecting.",
		Unit:        "sandboxes",
		Series: []Series{
			{Label: "expired", Query: zeroFloor(`sum(agent_sandboxes{expired="true"})`)},
		},
	}
}

func fleetCPU() Metric {
	return Metric{
		Name:        "fleet_cpu",
		Title:       "CPU reserved against used",
		Description: "What the sandbox pods hold, against what they are running. The gap is idle reservation.",
		Unit:        "cores",
		Series: []Series{
			{Label: "reserved", Query: zeroFloor(joinSandboxPods(
				`sum(kube_pod_container_resource_requests{resource="cpu"}`))},
			{Label: "used", Query: zeroFloor(joinSandboxPods(
				`sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])`))},
		},
	}
}

func fleetMemory() Metric {
	return Metric{
		Name:        "fleet_memory",
		Title:       "Memory reserved against used",
		Description: "The same comparison for memory: reservations against working set.",
		Unit:        "GiB",
		Series: []Series{
			{Label: "reserved", Query: zeroFloor(joinSandboxPods(
				`sum(kube_pod_container_resource_requests{resource="memory"}`) + ` / 1024^3`)},
			{Label: "used", Query: zeroFloor(joinSandboxPods(
				`sum(container_memory_working_set_bytes{container!=""}`) + ` / 1024^3`)},
		},
	}
}

// joinSandboxPods closes an unfinished `sum(<selector>` with the label join that
// narrows it to sandbox pods. container!="" is already in the callers' selectors
// where it matters: cAdvisor also exports a pod-level cgroup series, which would
// count every pod twice.
func joinSandboxPods(sumPrefix string) string {
	return fmt.Sprintf(`%s * on (namespace, pod) group_left() %s)`, sumPrefix, SandboxPodLabelSelector)
}

// ----- controller ----------------------------------------------------------

func reconcileLatency(job string) Metric {
	return Metric{
		Name:        "controller_reconcile_latency",
		Title:       "Reconcile latency",
		Description: "How long the controller takes per reconcile, across its controllers.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: quantileMs(0.5, fmt.Sprintf(`controller_runtime_reconcile_time_seconds_bucket{job=%q}`, job))},
			{Label: "p95", Query: quantileMs(0.95, fmt.Sprintf(`controller_runtime_reconcile_time_seconds_bucket{job=%q}`, job))},
		},
	}
}

func reconcileRate(job string) Metric {
	return Metric{
		Name:        "controller_reconcile_rate",
		Title:       "Reconcile throughput",
		Description: "Reconciles and reconcile errors per minute. Errors that track the throughput line mean the controller is retrying, not working.",
		Unit:        "per min",
		Series: []Series{
			{Label: "reconciles", Query: zeroFloor(fmt.Sprintf(
				`sum(rate(controller_runtime_reconcile_total{job=%q}[5m])) * 60`, job))},
			{Label: "errors", Query: zeroFloor(fmt.Sprintf(
				`sum(rate(controller_runtime_reconcile_errors_total{job=%q}[5m])) * 60`, job))},
		},
	}
}

func queueWait(job string) Metric {
	return Metric{
		Name:        "controller_queue_wait",
		Title:       "Work queue wait",
		Description: "How long an item sits in the controller's queue before a worker picks it up. Rises before latency does when the controller falls behind.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p95", Query: quantileMs(0.95, fmt.Sprintf(`workqueue_queue_duration_seconds_bucket{job=%q}`, job))},
		},
	}
}

// quantileMs builds a histogram quantile over a seconds-based bucket metric and
// converts it to milliseconds, the unit the rest of the page uses.
func quantileMs(q float64, bucket string) string {
	return fmt.Sprintf(`histogram_quantile(%g, sum(rate(%s[5m])) by (le)) * 1000`, q, bucket)
}

// ----- claims --------------------------------------------------------------

func claimStartupLatency() Metric {
	return Metric{
		Name:        "claim_startup_latency",
		Title:       "Claim startup latency",
		Description: "End-to-end: SandboxClaim created until its Sandbox is Ready.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: quantileNativeMs(0.5, "agent_sandbox_claim_startup_latency_ms_bucket")},
			{Label: "p95", Query: quantileNativeMs(0.95, "agent_sandbox_claim_startup_latency_ms_bucket")},
		},
	}
}

func sandboxCreationLatency() Metric {
	return Metric{
		Name:        "sandbox_creation_latency",
		Title:       "Sandbox creation latency",
		Description: "Sandbox created until its pod is Ready. For a warm launch this is the controller's synchronisation overhead, since the pod already exists.",
		Unit:        "ms",
		Series: []Series{
			{Label: "p50", Query: quantileNativeMs(0.5, "agent_sandbox_creation_latency_ms_bucket")},
			{Label: "p95", Query: quantileNativeMs(0.95, "agent_sandbox_creation_latency_ms_bucket")},
		},
	}
}

func claimCreationRate() Metric {
	return Metric{
		Name:        "claim_creation_rate",
		Title:       "Claim creation rate",
		Description: "SandboxClaims created per minute, summed across namespaces.",
		Unit:        "per min",
		Series: []Series{
			{Label: "claims", Query: zeroFloor(`sum(rate(agent_sandbox_claim_creation_total[1m])) * 60`)},
		},
	}
}

// quantileNativeMs is quantileMs for a histogram already recorded in
// milliseconds, so no conversion is applied.
func quantileNativeMs(q float64, bucket string) string {
	return fmt.Sprintf(`histogram_quantile(%g, sum(rate(%s[5m])) by (le))`, q, bucket)
}
