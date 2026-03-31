package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/outdoorsea/faultline/internal/api"
	"github.com/outdoorsea/faultline/internal/dashboard"
	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/ingest"
)

// Config holds server configuration.
type Config struct {
	Addr        string
	Handler     *ingest.Handler
	API         *api.Handler
	Dashboard   *dashboard.Handler
	RateLimiter *ingest.RateLimiter
	DB          *db.DB
	Log         *slog.Logger
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	mux := http.NewServeMux()

	// Sentry SDK endpoints (rate limited per project).
	envelope := cfg.Handler.HandleEnvelope
	store := cfg.Handler.HandleStore
	if cfg.RateLimiter != nil {
		envelope = cfg.RateLimiter.Wrap(envelope)
		store = cfg.RateLimiter.Wrap(store)
	}
	mux.HandleFunc("POST /api/{project_id}/envelope/", envelope)
	mux.HandleFunc("POST /api/{project_id}/envelope", envelope)
	mux.HandleFunc("POST /api/{project_id}/store/", store)
	mux.HandleFunc("POST /api/{project_id}/store", store)

	// Read/management API.
	if cfg.API != nil {
		cfg.API.RegisterRoutes(mux)
	}

	// Dashboard UI.
	if cfg.Dashboard != nil {
		cfg.Dashboard.RegisterRoutes(mux)
	}

	// Health check — verifies database connectivity.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := "ok"
		code := http.StatusOK
		if cfg.DB != nil {
			if err := cfg.DB.PingContext(r.Context()); err != nil {
				status = "degraded"
				code = http.StatusServiceUnavailable
				cfg.Log.Error("health check: database unreachable", "err", err)
			}
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	})

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		cfg.Log.Info("listening", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}
