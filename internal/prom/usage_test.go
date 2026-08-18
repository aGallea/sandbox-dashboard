package prom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Query_ReturnsVectorSamples(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{"metric": {"namespace": "default", "pod": "sb-a"}, "value": [1715990430, "0.052"]},
					{"metric": {"namespace": "evals", "pod": "sb-b"}, "value": [1715990430, "3"]}
				]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	got, err := c.Query(context.Background(), "up", time.Unix(1715990430, 0))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "default", got[0].Labels["namespace"])
	require.Equal(t, "sb-a", got[0].Labels["pod"])
	require.Equal(t, 0.052, got[0].Value)
	require.Equal(t, 3.0, got[1].Value)
}

func TestClient_Query_RejectsANonVectorResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	_, err = c.Query(context.Background(), "up", time.Now())
	require.ErrorContains(t, err, "vector")
}

func TestUsageQueries_ScopeToTheGivenNamespaces(t *testing.T) {
	cpu, mem := UsageQueries([]string{"evals", "default"})

	// Sorted, so the query — and any cache keyed on it — is stable across calls.
	require.Contains(t, cpu, `namespace=~"default|evals"`)
	require.Contains(t, mem, `namespace=~"default|evals"`)
	// The pod-level cgroup series must stay out of the sum, or every pod counts twice.
	require.Contains(t, cpu, `container!=""`)
	require.Contains(t, mem, `container!=""`)
	require.Contains(t, cpu, "container_cpu_usage_seconds_total")
	require.Contains(t, mem, "container_memory_working_set_bytes")
}

// The names are cluster state, not user input, but they are interpolated into
// PromQL — so anything that is not a legal namespace name is dropped rather
// than trusted.
func TestUsageQueries_DropNamesThatAreNotLegalNamespaces(t *testing.T) {
	cpu, _ := UsageQueries([]string{`default"} or vector(1) #`, "UPPER", "ok-ns"})
	require.Contains(t, cpu, `namespace=~"ok-ns"`)
	require.NotContains(t, cpu, "vector(1)")
	require.NotContains(t, cpu, "UPPER")
}

func TestUsageQueries_UnscopedWhenNoNamespacesSurvive(t *testing.T) {
	cpu, mem := UsageQueries(nil)
	require.NotContains(t, cpu, "namespace=~")
	require.NotContains(t, mem, "namespace=~")
	require.Contains(t, cpu, `container!=""`)
}
