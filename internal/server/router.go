// Package server provides the HTTP API for the dashboard.
package server

import (
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

	return r
}
