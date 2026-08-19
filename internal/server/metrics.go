package server

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	"github.com/aGallea/sandbox-dashboard/internal/prom"
)

// PromQuerier is the subset of *prom.Client the handlers depend on: ranged
// queries for the metrics charts, instant queries for the usage rollup.
type PromQuerier interface {
	QueryRange(ctx context.Context, query string, r prom.Range) ([]prom.Point, error)
	Query(ctx context.Context, query string, at time.Time) ([]prom.Sample, error)
}

// MetricSeries is one line on the chart.
type MetricSeries struct {
	Label  string       `json:"label"`
	Points []prom.Point `json:"points"`
}

// MetricInfo describes one chart. It deliberately carries no PromQL: the
// queries stay server-side, which is the point of the whitelist.
type MetricInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
}

// MetricSection is a group of charts a reader looks at together.
type MetricSection struct {
	Name    string       `json:"name"`
	Note    string       `json:"note,omitempty"`
	Metrics []MetricInfo `json:"metrics"`
}

// MetricCatalog is the JSON returned by GET /api/v1/metrics. The page renders
// itself from this rather than from a list duplicated in the SPA, so adding a
// chart is a change in one place.
type MetricCatalog struct {
	// PrometheusConfigured lets the page say once that the integration is off,
	// instead of asking for every chart and printing the same 503 ten times.
	PrometheusConfigured bool            `json:"prometheusConfigured"`
	Sections             []MetricSection `json:"sections"`
}

// handleMetricCatalog lists the available charts. It answers without Prometheus:
// the page should still show its shape, with each chart reporting the
// unconfigured state itself.
func handleMetricCatalog(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		catalog := MetricCatalog{PrometheusConfigured: d.Prom != nil}
		for _, s := range d.metrics().Sections() {
			section := MetricSection{Name: s.Name, Note: s.Note, Metrics: make([]MetricInfo, 0, len(s.Metrics))}
			for _, m := range s.Metrics {
				section.Metrics = append(section.Metrics, MetricInfo{
					Name:        m.Name,
					Title:       m.Title,
					Description: m.Description,
					Unit:        m.Unit,
				})
			}
			catalog.Sections = append(catalog.Sections, section)
		}
		writeJSON(w, http.StatusOK, catalog)
	}
}

// MetricResponse is the JSON returned by /api/v1/metrics/{name}.
type MetricResponse struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Unit        string         `json:"unit"`
	Range       string         `json:"range"`
	Series      []MetricSeries `json:"series"`
}

func handleMetric(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Prom == nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusServiceUnavailable,
				Type:   "prometheus-unconfigured",
				Detail: "Prometheus URL not configured on this dashboard install",
			})
			return
		}
		name := chi.URLParam(r, "name")
		metric, ok := d.metrics().Lookup(name)
		if !ok {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound, Type: "metric-not-found",
				Detail: "no such metric",
			})
			return
		}
		token := r.URL.Query().Get("range")
		rng, err := prom.ParseRange(token, time.Now())
		if err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusBadRequest, Type: "bad-range",
				Detail: err.Error(),
			})
			return
		}
		resp := MetricResponse{
			Name:        metric.Name,
			Title:       metric.Title,
			Description: metric.Description,
			Unit:        metric.Unit,
			Range:       chooseRangeLabel(token),
			Series:      make([]MetricSeries, len(metric.Series)),
		}

		perSeriesTimeout := perSeriesTimeoutFromEnv()
		g, gctx := errgroup.WithContext(r.Context())
		var mu sync.Mutex
		g.SetLimit(8)
		for i, s := range metric.Series {
			i, s := i, s
			g.Go(func() error {
				ctx, cancel := context.WithTimeout(gctx, perSeriesTimeout)
				defer cancel()
				pts, err := d.Prom.QueryRange(ctx, s.Query, rng)
				if err != nil {
					return err
				}
				mu.Lock()
				resp.Series[i] = MetricSeries{Label: s.Label, Points: pts}
				mu.Unlock()
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusBadGateway,
				Type:      "prometheus-unreachable",
				Detail:    "Prometheus query failed",
				LogReason: err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func chooseRangeLabel(token string) string {
	if token == "" {
		return "1h"
	}
	return token
}

// perSeriesTimeoutFromEnv reads AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT (e.g. "10s").
// Returns 10s on missing/invalid input.
func perSeriesTimeoutFromEnv() time.Duration {
	const def = 10 * time.Second
	v, ok := os.LookupEnv("AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT")
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
