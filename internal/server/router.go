// Package server provides the HTTP API for the dashboard.
package server

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aGallea/sandbox-dashboard/internal/osb"
	"github.com/aGallea/sandbox-dashboard/internal/prom"
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
	// Prom is the optional Prometheus query client. If nil, /api/v1/metrics/*
	// and /api/v1/usage return 503.
	Prom PromQuerier
	// Metrics is the whitelisted metric registry. If nil, a registry for the
	// default controller scrape job is used.
	Metrics *prom.Registry
	// Osb is the optional OpenSandbox client. If nil, sandbox rows carry no
	// OpenSandbox state and the list response omits its osb block.
	Osb OsbClient
	// Logs reads pod logs straight from the API server. If nil,
	// /api/v1/sandboxes/{ns}/{name}/logs returns 503.
	Logs PodLogStreamer
	// WatchNamespaces is the namespace scope the informers were given. Empty
	// means every namespace. Reported on /api/v1/overview so the UI can say that
	// a narrowed install is showing a partial fleet.
	WatchNamespaces []string
	// BasePath is the URL prefix the dashboard is reached at from a browser,
	// e.g. "/sandbox-dashboard". Empty means the domain root.
	//
	// The server keeps serving everything at "/" — the proxy in front strips the
	// prefix before it arrives. This exists only to tell the browser, which does
	// see the prefix and has to ask for the assets and the API under it.
	BasePath string
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

// metrics returns the configured registry, defaulting to one scoped to the
// upstream controller job name.
func (d Deps) metrics() *prom.Registry {
	if d.Metrics == nil {
		return prom.NewRegistry(prom.DefaultControllerJob)
	}
	return d.Metrics
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
	// The sandbox list is the one big response here, and it is polled: a fleet of
	// ~600 sandboxes serialises to ~690 kB, re-fetched by every open tab. It is
	// repetitive JSON, so it compresses by around 90%. Applies to the embedded
	// SPA's assets too.
	r.Use(middleware.Compress(5))

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
		mr.Get("/", handleMetricCatalog(d))
		mr.Get("/{name}", handleMetric(d))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(requireCacheSynced(d.CacheSynced, d.Logger))
		if d.Client != nil {
			r.Get("/overview", handleOverview(d))
			r.Get("/usage", handleUsage(d))
			r.Get("/sandboxes", handleSandboxList(d))
			r.Get("/sandboxes/{namespace}/{name}", handleSandboxDetail(d))
			r.Get("/sandboxes/{namespace}/{name}/osb", handleSandboxOsb(d))
			r.Get("/sandboxes/{namespace}/{name}/logs", handleSandboxLogs(d))
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
		r.Handle("/apple-touch-icon.png", fileServer)
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
			serveIndex(w, req, d)
			return
		}
		http.NotFound(w, req)
	})

	base := normaliseBasePath(d.BasePath)
	if base == "" {
		return r
	}

	// Mounted under the prefix, so the proxy in front forwards the path
	// unchanged and needs no rewriting middleware — the app owns its prefix.
	//
	// StripPrefix rather than chi's Mount alone: Mount rewrites the routing path
	// but leaves URL.Path, and http.FileServer reads URL.Path, so the assets
	// would be looked up as "/sandbox-dashboard/assets/x" inside the embedded FS
	// and 404.
	outer := chi.NewRouter()
	// Probes stay at the root. The kubelet talks to the container port directly,
	// not through the proxy that adds the prefix.
	outer.Handle("/healthz", r)
	outer.Handle("/readyz", r)
	outer.Handle(base, http.RedirectHandler(base+"/", http.StatusMovedPermanently))
	outer.Handle(base+"/*", http.StripPrefix(base, r))
	return outer
}

// normaliseBasePath turns whatever was configured into either "" (the domain
// root) or "/prefix" with no trailing slash, so the rest of the code has one
// shape to reason about.
func normaliseBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// serveIndex sends index.html with the base path written into it.
//
// Vite emits relative asset URLs, which resolve against <base href> — that is
// what lets one build work at any prefix. The tag is emitted even at the root,
// because a deep link like /sandboxes/default/foo would otherwise resolve
// "./assets/x" against /sandboxes/default/ and 404.
func serveIndex(w http.ResponseWriter, req *http.Request, d Deps) {
	raw, err := fs.ReadFile(d.UIAssets, "index.html")
	if err != nil {
		// Plain text, not problem+json: this path is a browser navigation, and
		// only the API speaks that content type.
		if d.Logger != nil {
			d.Logger.Error("read index.html", "err", err)
		}
		http.Error(w, "the UI could not be read", http.StatusInternalServerError)
		return
	}

	base := normaliseBasePath(d.BasePath)
	// html.EscapeString on both: the value lands in an HTML attribute and in a
	// JS string literal, and a quote in either would break out of it.
	injected := fmt.Sprintf(
		"<base href=\"%s/\"><script>window.__BASE_PATH__ = \"%s\"</script>",
		html.EscapeString(base), html.EscapeString(base),
	)

	// Immediately after <head> so the base applies to every URL that follows.
	body := strings.Replace(string(raw), "<head>", "<head>"+injected, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The prefix is baked in per response, so this must not be cached by a proxy
	// shared between two installs mounted differently.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(body))
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
			// The kubelet probes every few seconds; at info those lines are most
			// of the log and say nothing anyone came to read.
			level := slog.LevelInfo
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				level = slog.LevelDebug
			}
			logger.Log(r.Context(), level, "http",
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
