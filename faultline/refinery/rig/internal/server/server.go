package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/outdoorsea/faultline/internal/api"
	"github.com/outdoorsea/faultline/internal/ci"
	"github.com/outdoorsea/faultline/internal/dashboard"
	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/ingest"
	slackint "github.com/outdoorsea/faultline/internal/integrations/slack"
)

// Config holds server configuration.
type Config struct {
	Addr        string
	Handler     *ingest.Handler
	API         *api.Handler
	Dashboard   *dashboard.Handler
	CI          *ci.Handler
	Slack       *slackint.WebhookHandler
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
	mux.HandleFunc("POST /api/{project_id}/heartbeat", cfg.Handler.HandleHeartbeat)
	mux.HandleFunc("POST /api/{project_id}/heartbeat/", cfg.Handler.HandleHeartbeat)

	// Read/management API.
	if cfg.API != nil {
		cfg.API.RegisterRoutes(mux)
	}

	// Dashboard UI.
	if cfg.Dashboard != nil {
		cfg.Dashboard.RegisterRoutes(mux)
	}

	// CI webhook handlers.
	if cfg.CI != nil {
		cfg.CI.RegisterRoutes(mux)
	}

	// Slack interaction webhook handler.
	if cfg.Slack != nil {
		cfg.Slack.RegisterRoutes(mux)
	}

	// Root redirect to dashboard.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
	})

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
		if err := json.NewEncoder(w).Encode(map[string]string{"status": status}); err != nil {
			cfg.Log.Error("health check: encode response", "err", err)
		}
	})

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      securityHeaders(mux),
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

// securityHeaders adds standard security response headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// CSP: allow self + HTMX CDN + inline scripts (for live stream).
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' https://unpkg.com 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
