// Command dashboard runs the agent-sandbox operational dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	"github.com/aGallea/sandbox-dashboard/internal/osb"
	"github.com/aGallea/sandbox-dashboard/internal/prom"
	"github.com/aGallea/sandbox-dashboard/internal/server"
	"github.com/aGallea/sandbox-dashboard/internal/ui"
)

func main() {
	var listenAddr string
	// Note: the "kubeconfig" flag is already registered by the init() in
	// sigs.k8s.io/controller-runtime/pkg/client/config — do not re-register it.
	flag.StringVar(&listenAddr, "listen-addr", ":8080", "HTTP bind address")
	var promURL string
	flag.StringVar(&promURL, "prometheus-url", "", "Optional Prometheus base URL (e.g. http://prometheus.monitoring.svc:9090). If empty, /api/v1/metrics/* returns 503.")
	var osbURL string
	flag.StringVar(&osbURL, "opensandbox-url", "", "Optional OpenSandbox base URL (e.g. http://opensandbox-server.default.svc). If empty, sandbox rows carry no OpenSandbox state.")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctrl.SetLogger(slogToLogr(logger))

	if promURL == "" {
		if env := os.Getenv("PROMETHEUS_URL"); env != "" {
			promURL = env
		}
	}
	var promClient *prom.Client
	if promURL != "" {
		var err error
		promClient, err = prom.NewClient(promURL, prom.WithLogger(logger))
		if err != nil {
			logger.Error("create prometheus client", "err", err)
			os.Exit(1)
		}
		logger.Info("prometheus client configured", "url", promURL)
	} else {
		logger.Info("prometheus URL not set — metrics endpoint will return 503")
	}

	if osbURL == "" {
		osbURL = os.Getenv("OPENSANDBOX_URL")
	}
	var osbClient server.OsbClient
	if osbURL != "" {
		apiKey := os.Getenv("OPENSANDBOX_API_KEY")
		if apiKey == "" {
			logger.Warn("OPENSANDBOX_API_KEY is empty — OpenSandbox requests will likely be rejected")
		}
		raw, err := osb.NewClient(osbURL, apiKey, osb.WithLogger(logger))
		if err != nil {
			logger.Error("create opensandbox client", "err", err)
			os.Exit(1)
		}
		cache := osb.NewCache(raw, durationFromEnv("AGENT_SANDBOX_DASHBOARD_OSB_TTL", 5*time.Second), time.Now)
		osbClient = osbAdapter{Cache: cache, client: raw}
		logger.Info("opensandbox client configured", "url", osbURL)
	} else {
		logger.Info("opensandbox URL not set — sandbox rows will carry no OpenSandbox state")
	}

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error("load kubeconfig", "err", err)
		os.Exit(1)
	}

	mgr, err := k8s.NewManager(cfg)
	if err != nil {
		logger.Error("create manager", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// Start the manager (informers) in the background.
	mgrErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		mgrErr <- mgr.Start(ctx)
	}()

	// Drive the cache-sync state from a single goroutine. WaitForCacheSync is
	// blocking, so the HTTP probes/handlers must not call it directly — they
	// read this atomic flag instead.
	var cacheSynced atomic.Bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		if mgr.GetCache().WaitForCacheSync(ctx) {
			cacheSynced.Store(true)
			logger.Info("informer cache synced")
		}
	}()

	assets, err := ui.Assets()
	if err != nil {
		logger.Error("load embedded UI assets", "err", err)
		os.Exit(1)
	}
	deps := server.Deps{
		Client:        mgr.GetClient(),
		CacheSynced:   cacheSynced.Load,
		UIAssets:      assets,
		Logger:        logger,
		OsbStaleAfter: durationFromEnv("AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER", server.DefaultOsbStaleAfter),
	}
	// Only assign Prom when the client was actually created. Assigning a typed
	// nil *prom.Client to a server.QueryRanger field would wrap it in a
	// non-nil interface value and bypass the handler's nil check.
	if promClient != nil {
		deps.Prom = promClient
	}
	// Same typed-nil hazard as Prom above: only assign when non-nil.
	if osbClient != nil {
		deps.Osb = osbClient
	}
	router := server.New(deps)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", listenAddr)
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-mgrErr:
		logger.Error("manager exited", "err", err)
	case err := <-srvErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server exited", "err", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "err", err)
	}
	cancel()
	wg.Wait()
	logger.Info("shutdown complete")
}

// slogToLogr adapts a slog.Logger to the logr.Logger that controller-runtime expects.
func slogToLogr(s *slog.Logger) logr.Logger {
	return logr.FromSlogHandler(s.Handler())
}

// osbAdapter combines a cached lister with the uncached client's diagnostics,
// satisfying server.OsbClient. Diagnostics are per-sandbox and only fetched
// when a drawer is opened, so they are deliberately not cached.
type osbAdapter struct {
	*osb.Cache
	client *osb.Client
}

func (a osbAdapter) Diagnostics(ctx context.Context, id string) (osb.Diagnostics, error) {
	return a.client.Diagnostics(ctx, id)
}

// durationFromEnv reads a Go duration string (e.g. "10s") from the environment,
// returning def when unset or invalid.
func durationFromEnv(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
