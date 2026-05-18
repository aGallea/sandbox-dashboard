package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aGallea/agent-sandbox-dashboard/internal/prom"
)

// QueryRanger is the subset of *prom.Client the handler depends on.
type QueryRanger interface {
	QueryRange(ctx context.Context, query string, r prom.Range) ([]prom.Point, error)
}

// MetricSeries is one line on the chart.
type MetricSeries struct {
	Label  string       `json:"label"`
	Points []prom.Point `json:"points"`
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
		metric, ok := prom.Lookup(name)
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
			Series:      make([]MetricSeries, 0, len(metric.Series)),
		}
		for _, s := range metric.Series {
			pts, err := d.Prom.QueryRange(r.Context(), s.Query, rng)
			if err != nil {
				writeProblem(w, d.Logger, problemArgs{
					Status:    http.StatusBadGateway,
					Type:      "prometheus-unreachable",
					Detail:    "Prometheus query failed",
					LogReason: err.Error(),
				})
				return
			}
			resp.Series = append(resp.Series, MetricSeries{
				Label:  s.Label,
				Points: pts,
			})
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
