// Package server provides the HTTP API for the dashboard.
package server

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deps is the set of collaborators the router needs. Tests can swap any of them.
type Deps struct {
	// Client reads from the controller-runtime cache.
	Client client.Reader
	// CacheSynced reports whether all informers have completed initial sync.
	CacheSynced func() bool
	// UIAssets is the embedded SPA filesystem. Optional; if nil, no SPA is served.
	UIAssets fs.FS
}

// New returns a fully wired chi router.
func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
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

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(requireCacheSynced(d.CacheSynced))
		if d.Client != nil {
			r.Get("/overview", handleOverview(d.Client))
		}
	})

	if d.UIAssets != nil {
		fileServer := http.FileServer(http.FS(d.UIAssets))
		r.Handle("/assets/*", fileServer)
		r.Handle("/favicon.ico", fileServer)
		r.Handle("/vite.svg", fileServer)
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			http.ServeFileFS(w, req, d.UIAssets, "index.html")
		})
	}

	return r
}

// requireCacheSynced is a middleware that returns 503 problem+json until the
// informer cache has completed its initial sync.
func requireCacheSynced(synced func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if synced == nil || !synced() {
				writeProblem(w, http.StatusServiceUnavailable, "cache-not-synced", "informer cache is still syncing; retry shortly")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
