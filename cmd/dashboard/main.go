// Command dashboard runs the agent-sandbox operational dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/aGallea/agent-sandbox-dashboard/internal/k8s"
	"github.com/aGallea/agent-sandbox-dashboard/internal/server"
)

func main() {
	var listenAddr string
	// Note: the "kubeconfig" flag is already registered by the init() in
	// sigs.k8s.io/controller-runtime/pkg/client/config — do not re-register it.
	flag.StringVar(&listenAddr, "listen-addr", ":8080", "HTTP bind address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctrl.SetLogger(slogToLogr(logger))

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

	// Start the manager (informers) in the background.
	mgrErr := make(chan error, 1)
	go func() { mgrErr <- mgr.Start(ctx) }()

	router := server.New(server.Deps{
		Client:      mgr.GetClient(),
		CacheSynced: func() bool { return mgr.GetCache().WaitForCacheSync(ctx) },
	})

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
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
	<-mgrErr
	fmt.Fprintln(os.Stderr, "bye")
}

// slogToLogr adapts a slog.Logger to the logr.Logger that controller-runtime expects.
func slogToLogr(s *slog.Logger) logr.Logger {
	return logr.FromSlogHandler(s.Handler())
}
