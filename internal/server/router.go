// Package server provides the HTTP API for the dashboard.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aGallea/sandbox-dashboard/internal/osb"
)

// Deps is the set of collaborators the router needs. Tests can swap any of them.
type Deps struct {
	// Client reads from the controller-runtime cache.
	Client client.Reader
	// CacheSynced reports whether all informers have completed initial sync.
	CacheSynced func() bool
	// UIAssets is the embedded SPA filesystem. Optional; if nil, no SPA is served.
	UIAssets fs.FS
	// Logger is used for structured error logging. Optional; if nil, errors are not logged.
	Logger *slog.Logger
	// Prom is the optional Prometheus query client. If nil, /api/v1/metrics/* returns 503.
	Prom QueryRanger
	// Osb is the optional OpenSandbox client. If nil, sandbox rows carry no
	// OpenSandbox state and the list response omits its osb block.
	Osb OsbClient
	// Now supplies the current time; tests substitute a fixed clock.
	// If nil, time.Now is used.
	Now func() time.Time
	// OsbStaleAfter is how long a transient OpenSandbox state may sit before it
	// is reported stale. If zero, DefaultOsbStaleAfter is used.
	OsbStaleAfter time.Duration
	// OsbTimeout bounds a single OpenSandbox inventory fetch. If zero,
	// DefaultOsbTimeout is used.
	OsbTimeout time.Duration
}

// OsbClient is the subset of *osb.Client the sandbox handlers depend on.
type OsbClient interface {
	ListSandboxes(ctx context.Context) (map[string]osb.Sandbox, error)
	Diagnostics(ctx context.Context, id string) (osb.Diagnostics, error)
}

// now returns the Deps clock, defaulting to time.Now.
func (d Deps) now() time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

// staleAfter returns the configured staleness threshold, defaulting to DefaultOsbStaleAfter.
func (d Deps) staleAfter() time.Duration {
	if d.OsbStaleAfter <= 0 {
		return DefaultOsbStaleAfter
	}
	return d.OsbStaleAfter
}

// osbTimeout returns the configured OpenSandbox fetch timeout, defaulting to DefaultOsbTimeout.
func (d Deps) osbTimeout() time.Duration {
	if d.OsbTimeout <= 0 {
		return DefaultOsbTimeout
	}
	return d.OsbTimeout
}

// New returns a fully wired chi router.
func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(slogRequestMiddleware(d.Logger))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if d.CacheSynced == nil || !d.CacheSynced() {
			http.Error(w, "informer cache not synced", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Metrics intentionally lives outside requireCacheSynced — Prometheus is
	// independent of the informer cache, and the metrics page should remain
	// usable during dashboard startup.
	r.Route("/api/v1/metrics", func(mr chi.Router) {
		mr.Get("/{name}", handleMetric(d))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(requireCacheSynced(d.CacheSynced, d.Logger))
		if d.Client != nil {
			r.Get("/overview", handleOverview(d))
			r.Get("/sandboxes", handleSandboxList(d))
			r.Get("/sandboxes/{namespace}/{name}", handleSandboxDetail(d))
			r.Get("/sandboxes/{namespace}/{name}/osb", handleSandboxOsb(d))
			r.Get("/claims", handleClaimList(d))
			r.Get("/claims/{namespace}/{name}", handleClaimDetail(d))
			r.Get("/templates", handleTemplateList(d))
			r.Get("/templates/{namespace}/{name}", handleTemplateDetail(d))
			r.Get("/warmpools", handleWarmPoolList(d))
			r.Get("/warmpools/{namespace}/{name}", handleWarmPoolDetail(d))
		}
	})

	// Static SPA assets — only the predictable paths Vite emits.
	if d.UIAssets != nil {
		fileServer := http.FileServer(http.FS(d.UIAssets))
		r.Handle("/assets/*", fileServer)
		r.Handle("/favicon.ico", fileServer)
		r.Handle("/favicon.svg", fileServer)
	}

	// Single source of truth for unmatched paths:
	//   /api/*  → JSON problem+json 404
	//   anything else with UIAssets → serve index.html (SPA client-side routing)
	//   anything else without UIAssets → plain 404
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound, Type: "not-found",
				Detail: "no such resource",
			})
			return
		}
		if d.UIAssets != nil {
			http.ServeFileFS(w, req, d.UIAssets, "index.html")
			return
		}
		http.NotFound(w, req)
	})

	return r
}

// slogRequestMiddleware logs each HTTP request using structured slog output.
// If logger is nil, requests pass through without logging.
func slogRequestMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

// requireCacheSynced is a middleware that returns 503 problem+json until the
// informer cache has completed its initial sync.
func requireCacheSynced(synced func() bool, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if synced == nil || !synced() {
				writeProblem(w, logger, problemArgs{
					Status: http.StatusServiceUnavailable,
					Type:   "cache-not-synced",
					Detail: "informer cache is still syncing; retry shortly",
					// no LogReason — expected during startup
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
