package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aGallea/agent-sandbox-dashboard/internal/prom"
)

func TestMetrics_503WhenProm_Unconfigured(t *testing.T) {
	r := New(Deps{
		CacheSynced: func() bool { return true },
		// Prom: nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/sandbox_creation_latency", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestMetrics_404OnUnknownMetric(t *testing.T) {
	r := New(Deps{
		CacheSynced: func() bool { return true },
		Prom:        &stubProm{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/totally_made_up", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetrics_HappyPathReturnsSeries(t *testing.T) {
	stub := &stubProm{
		points: map[string][]prom.Point{
			"histogram_quantile(0.5, sum(rate(agent_sandbox_creation_latency_ms_bucket[5m])) by (le))": {
				{Value: 12.5}, {Value: 13.0},
			},
			"histogram_quantile(0.95, sum(rate(agent_sandbox_creation_latency_ms_bucket[5m])) by (le))": {
				{Value: 22.0},
			},
		},
	}
	r := New(Deps{CacheSynced: func() bool { return true }, Prom: stub})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/sandbox_creation_latency?range=15m", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got MetricResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "sandbox_creation_latency", got.Name)
	require.Equal(t, "ms", got.Unit)
	require.Len(t, got.Series, 2)
	require.Equal(t, "p50", got.Series[0].Label)
	require.Len(t, got.Series[0].Points, 2)
	require.Equal(t, 12.5, got.Series[0].Points[0].Value)
}

func TestMetrics_DoesNotRequireCacheSync(t *testing.T) {
	// Critical: cache-sync gate must NOT block metrics.
	r := New(Deps{
		CacheSynced: func() bool { return false },
		Prom:        &stubProm{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/sandbox_creation_latency", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

// stubProm satisfies the QueryRanger interface defined in metrics.go.
type stubProm struct {
	points map[string][]prom.Point
	err    error
}

func (s *stubProm) QueryRange(_ context.Context, q string, _ prom.Range) ([]prom.Point, error) {
	if s.err != nil {
		return nil, s.err
	}
	pts := s.points[q]
	if pts == nil {
		return []prom.Point{}, nil
	}
	return pts, nil
}
