package prom

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_SectionsAreOrderedAndEveryMetricBelongsToOne(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	names := make([]string, 0, len(r.Sections()))
	for _, s := range r.Sections() {
		names = append(names, s.Name)
		require.NotEmpty(t, s.Metrics, "section %q has no metrics", s.Name)
	}
	require.Equal(t, []string{"Fleet", "Controller", "Claims"}, names)

	declared := map[string]bool{}
	for _, s := range r.Sections() {
		for _, m := range s.Metrics {
			require.False(t, declared[m.Name], "metric %q listed twice", m.Name)
			declared[m.Name] = true
			require.NotEmpty(t, m.Title, "%s has no title", m.Name)
			require.NotEmpty(t, m.Description, "%s has no description", m.Name)
			require.NotEmpty(t, m.Unit, "%s has no unit", m.Name)
			require.NotEmpty(t, m.Series, "%s has no series", m.Name)
		}
	}
	for _, m := range r.All() {
		require.True(t, declared[m.Name], "metric %q is not in any section", m.Name)
	}
}

func TestRegistry_LookupFindsAMetricAndMissesAnUnknownOne(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	m, ok := r.Lookup("fleet_size")
	require.True(t, ok)
	require.Equal(t, "sandboxes", m.Unit)

	_, ok = r.Lookup("no_such_metric")
	require.False(t, ok)
}

// The controller-runtime metric names are shared by every controller-runtime
// binary in the cluster, so the job label is the only thing separating the
// agent-sandbox controller's numbers from everyone else's — and its value is an
// install detail, not a constant.
func TestRegistry_ControllerQueriesScopeToTheConfiguredJob(t *testing.T) {
	r := NewRegistry("sandbox-ctrl")

	found := 0
	for _, s := range r.Sections() {
		if s.Name != "Controller" {
			continue
		}
		for _, m := range s.Metrics {
			for _, series := range m.Series {
				require.Contains(t, series.Query, `job="sandbox-ctrl"`)
				found++
			}
		}
	}
	require.NotZero(t, found)
}

func TestRegistry_RejectsAJobNameThatIsNotALabelValue(t *testing.T) {
	r := NewRegistry(`ctrl"} or vector(1) #`)

	for _, m := range r.All() {
		for _, s := range m.Series {
			require.NotContains(t, s.Query, "vector(1)")
		}
	}
	// Falls back rather than building a query that cannot be trusted.
	m, ok := r.Lookup("controller_reconcile_latency")
	require.True(t, ok)
	require.Contains(t, m.Series[0].Query, `job="`+DefaultControllerJob+`"`)
}

// A gauge whose label combination has not appeared yet returns no series, which
// renders as an empty chart even though the honest answer is zero. Flooring it
// with `or vector(0)` fixes that — but the same trick on a latency quantile
// would draw a flat 0 ms line, claiming instant when the truth is "never
// measured". So the floor belongs on counts and rates only.
func TestRegistry_LatencyQuantilesAreNotFlooredToZero(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	for _, m := range r.All() {
		for _, s := range m.Series {
			if !strings.Contains(s.Query, "histogram_quantile") {
				continue
			}
			require.NotContains(t, s.Query, "or vector(0)",
				"%s/%s floors a quantile, which reads as 0ms rather than 'no samples'", m.Name, s.Label)
		}
	}
}

func TestRegistry_CountAndRateQueriesAreFlooredToZero(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	for _, name := range []string{"fleet_size", "fleet_expired", "fleet_cpu", "fleet_memory"} {
		m, ok := r.Lookup(name)
		require.True(t, ok, name)
		for _, s := range m.Series {
			require.Contains(t, s.Query, "or vector(0)", "%s/%s", name, s.Label)
		}
	}
}

// Pod-level series carry no sandbox identity of their own; the join against
// kube_pod_labels is what keeps the fleet charts from counting every pod in the
// namespace.
func TestRegistry_FleetResourceQueriesJoinOnTheSandboxPodLabel(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	for _, name := range []string{"fleet_cpu", "fleet_memory"} {
		m, _ := r.Lookup(name)
		for _, s := range m.Series {
			require.Contains(t, s.Query, "kube_pod_labels")
			require.Contains(t, s.Query, "label_agents_x_k8s_io_sandbox_name_hash")
		}
	}
}

func TestRegistry_HoldsTheExpectedMetrics(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	want := []string{
		"fleet_size", "fleet_expired", "fleet_cpu", "fleet_memory",
		"controller_reconcile_latency", "controller_reconcile_rate", "controller_queue_wait",
		"claim_startup_latency", "sandbox_creation_latency", "claim_creation_rate",
	}
	for _, name := range want {
		_, ok := r.Lookup(name)
		require.True(t, ok, "missing metric %q", name)
	}
	require.Len(t, r.All(), len(want), "registry holds a metric this test does not know about")

	// claim_controller_startup_latency was dropped: it measures the reconcile-side
	// slice of claim_startup_latency, so it drew a near-duplicate chart.
	_, ok := r.Lookup("claim_controller_startup_latency")
	require.False(t, ok)
}

func TestRegistry_LatencyMetricsCarryP50AndP95(t *testing.T) {
	r := NewRegistry(DefaultControllerJob)

	for _, name := range []string{"sandbox_creation_latency", "claim_startup_latency", "controller_reconcile_latency"} {
		m, ok := r.Lookup(name)
		require.True(t, ok, name)
		require.Equal(t, "ms", m.Unit)
		require.Len(t, m.Series, 2, name)
		require.Equal(t, "p50", m.Series[0].Label)
		require.Contains(t, m.Series[0].Query, "histogram_quantile(0.5")
		require.Equal(t, "p95", m.Series[1].Label)
		require.Contains(t, m.Series[1].Query, "histogram_quantile(0.95")
	}
}
