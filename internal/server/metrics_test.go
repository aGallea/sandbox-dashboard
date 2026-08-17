package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aGallea/sandbox-dashboard/internal/prom"
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
	points  map[string][]prom.Point
	err     error
	slowFor time.Duration
}

func (s *stubProm) QueryRange(ctx context.Context, q string, _ prom.Range) ([]prom.Point, error) {
	if s.slowFor > 0 {
		select {
		case <-time.After(s.slowFor):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	pts := s.points[q]
	if pts == nil {
		return []prom.Point{}, nil
	}
	return pts, nil
}

func TestMetrics_502OnSlowProm(t *testing.T) {
	slow := &stubProm{slowFor: 200 * time.Millisecond}
	r := New(Deps{CacheSynced: func() bool { return true }, Prom: slow})

	t.Setenv("AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT", "50ms")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/sandbox_creation_latency?range=15m", nil)
	rec := httptest.NewRecorder()
	tStart := time.Now()
	r.ServeHTTP(rec, req)
	elapsed := time.Since(tStart)

	require.Equal(t, http.StatusBadGateway, rec.Code,
		"a per-series timeout should surface as 502 (took %s)", elapsed)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Less(t, elapsed, 500*time.Millisecond, "request should fail fast on timeout, not wait for slowFor")
}
