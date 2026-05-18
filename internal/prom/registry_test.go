package prom

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_LookupKnownMetrics(t *testing.T) {
	m, ok := Lookup("sandbox_creation_latency")
	require.True(t, ok)
	require.Equal(t, "sandbox_creation_latency", m.Name)
	require.Equal(t, "ms", m.Unit)
	require.Len(t, m.Series, 2)
	require.Equal(t, "p50", m.Series[0].Label)
	require.Contains(t, m.Series[0].Query, "histogram_quantile(0.5")
	require.Equal(t, "p95", m.Series[1].Label)
	require.Contains(t, m.Series[1].Query, "histogram_quantile(0.95")
}

func TestRegistry_AllExpectedMetrics(t *testing.T) {
	want := []string{
		"sandbox_creation_latency",
		"claim_startup_latency",
		"claim_controller_startup_latency",
		"claim_creation_rate",
	}
	for _, name := range want {
		_, ok := Lookup(name)
		require.True(t, ok, "missing metric %q", name)
	}
}

func TestRegistry_UnknownIsMiss(t *testing.T) {
	_, ok := Lookup("totally_made_up")
	require.False(t, ok)
}
