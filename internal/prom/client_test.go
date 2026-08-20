package prom

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_QueryRange_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query_range", r.URL.Path)
		// The Prometheus client sends query params as POST form body (DoGetFallback).
		require.NoError(t, r.ParseForm())
		require.NotEmpty(t, r.FormValue("query"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [{
					"metric": {},
					"values": [
						[1715990400, "12.5"],
						[1715990430, "13.0"]
					]
				}]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	now := time.Unix(1715990430, 0)
	r, err := ParseRange("15m", now)
	require.NoError(t, err)

	pts, err := c.QueryRange(context.Background(), "up", r)
	require.NoError(t, err)
	require.Len(t, pts, 2)
	require.Equal(t, 12.5, pts[0].Value)
	require.Equal(t, 13.0, pts[1].Value)
}

func TestClient_QueryRange_PropagatesErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	r, _ := ParseRange("15m", time.Now())
	_, err = c.QueryRange(context.Background(), "up", r)
	require.Error(t, err)
}

func TestClient_QueryRange_LogsWarnings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "success",
			"warnings": ["lookback delta exceeded"],
			"data": {
				"resultType": "matrix",
				"result": [{"metric":{}, "values":[[1715990400,"1.0"]]}]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, err := NewClient(srv.URL, WithLogger(logger))
	require.NoError(t, err)

	now := time.Unix(1715990400, 0)
	r, _ := ParseRange("15m", now)
	_, err = c.QueryRange(context.Background(), "up", r)
	require.NoError(t, err)

	require.Contains(t, buf.String(), "lookback delta exceeded")
}

// A histogram_quantile over a window with no observations yields NaN. Nothing
// downstream can use it — and encoding.json refuses to marshal it, which turned
// a chart with a quiet stretch into a 200 with an empty body — so a non-finite
// sample is not a point.
func TestClient_QueryRange_DropsNonFiniteSamples(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [{
					"metric": {},
					"values": [
						[1715990400, "NaN"],
						[1715990430, "12.5"],
						[1715990460, "+Inf"],
						[1715990490, "13.5"]
					]
				}]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	r, err := ParseRange("15m", time.Unix(1715990490, 0))
	require.NoError(t, err)

	pts, err := c.QueryRange(context.Background(), "histogram_quantile(0.95, x)", r)
	require.NoError(t, err)
	require.Len(t, pts, 2)
	require.Equal(t, 12.5, pts[0].Value)
	require.Equal(t, 13.5, pts[1].Value)

	body, err := json.Marshal(pts)
	require.NoError(t, err, "points must survive JSON encoding")
	require.NotContains(t, string(body), "NaN")
}
