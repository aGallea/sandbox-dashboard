package server

import (
	"context"
	"math"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/aGallea/sandbox-dashboard/internal/prom"
)

// PodUsage is what one sandbox pod is actually consuming right now, against the
// reservation reported in the sandbox list's pod block.
type PodUsage struct {
	CPUCores float64 `json:"cpuCores"`
	MemBytes float64 `json:"memBytes"`
}

// UsageResponse is the JSON returned by GET /api/v1/usage.
type UsageResponse struct {
	SampledAt time.Time `json:"sampledAt"`
	// Pods is keyed "namespace/pod", the key a client can build from the sandbox
	// list's pod block without a second lookup.
	Pods map[string]PodUsage `json:"pods"`
}

// handleUsage reports live CPU and memory use per sandbox pod.
//
// This lives outside the sandbox list on purpose: joining it there would put
// Prometheus in the request path of the dashboard's main table, so an outage
// would cost the whole list rather than one panel.
func handleUsage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Prom == nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusServiceUnavailable,
				Type:   "prometheus-unconfigured",
				Detail: "Prometheus URL not configured on this dashboard install",
			})
			return
		}

		pods, err := podsBySandboxUID(r.Context(), d.Client, "")
		if err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-pods",
				Detail:    "could not list pods",
				LogReason: err.Error(),
			})
			return
		}

		// Which pods to report, and which namespaces to scope the queries to.
		wanted := make(map[string]bool, len(pods))
		namespaces := map[string]bool{}
		for _, pod := range pods {
			wanted[pod.Namespace+"/"+pod.Name] = true
			namespaces[pod.Namespace] = true
		}

		now := d.now()
		if len(wanted) == 0 {
			// No sandbox pods: an unscoped query would sweep every pod in the
			// cluster to report nothing.
			writeJSON(w, http.StatusOK, UsageResponse{SampledAt: now, Pods: map[string]PodUsage{}})
			return
		}

		cpuQuery, memQuery := prom.UsageQueries(keys(namespaces))
		var (
			mu    sync.Mutex
			usage = make(map[string]PodUsage, len(wanted))
		)
		// assign folds one query's samples in, keeping only sandbox pods. Both
		// queries are per-pod sums, so at most one sample per pod arrives.
		assign := func(samples []prom.Sample, set func(*PodUsage, float64)) {
			mu.Lock()
			defer mu.Unlock()
			for _, s := range samples {
				// A NaN or Inf would fail JSON encoding after the status line is
				// already written, so one odd sample would break the response.
				if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
					continue
				}
				key := s.Labels["namespace"] + "/" + s.Labels["pod"]
				if !wanted[key] {
					continue
				}
				entry := usage[key]
				set(&entry, s.Value)
				usage[key] = entry
			}
		}

		timeout := perSeriesTimeoutFromEnv()
		g, gctx := errgroup.WithContext(r.Context())
		g.Go(func() error {
			samples, err := queryWithin(gctx, d.Prom, cpuQuery, now, timeout)
			if err != nil {
				return err
			}
			assign(samples, func(u *PodUsage, v float64) { u.CPUCores = v })
			return nil
		})
		g.Go(func() error {
			samples, err := queryWithin(gctx, d.Prom, memQuery, now, timeout)
			if err != nil {
				return err
			}
			assign(samples, func(u *PodUsage, v float64) { u.MemBytes = v })
			return nil
		})
		if err := g.Wait(); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusBadGateway,
				Type:      "prometheus-unreachable",
				Detail:    "Prometheus query failed",
				LogReason: err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, UsageResponse{SampledAt: now, Pods: usage})
	}
}

func queryWithin(ctx context.Context, p PromQuerier, query string, at time.Time, timeout time.Duration) ([]prom.Sample, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return p.Query(ctx, query, at)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
