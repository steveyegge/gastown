package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/outdoorsea/faultline/internal/relay"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	addr := envOr("RELAY_ADDR", ":9090")
	dbPath := envOr("RELAY_DB", "relay.db")
	projectPairs := strings.Split(envOr("RELAY_PROJECTS", "1:default_key"), ",")
	ttlDays := envOrInt("RELAY_TTL_DAYS", 7)
	purgeInterval := envOrInt("RELAY_PURGE_INTERVAL_MIN", 60)

	auth, err := relay.NewAuth(projectPairs)
	if err != nil {
		return fmt.Errorf("invalid project config: %w", err)
	}

	store, err := relay.NewStore(dbPath, time.Duration(ttlDays)*24*time.Hour)
	if err != nil {
		return fmt.Errorf("database open failed: %w", err)
	}
	defer func() { _ = store.Close() }()

	handler := &relay.Handler{
		Store: store,
		Auth:  auth,
		Log:   log,
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return context.Background() },
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background purge loop.
	go func() {
		ticker := time.NewTicker(time.Duration(purgeInterval) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n, err := store.Purge()
				if err != nil {
					log.Error("purge failed", "err", err)
				} else if n > 0 {
					log.Info("purged old envelopes", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Info("relay listening", "addr", addr, "db", dbPath)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Error("shutdown error", "err", err)
		}
	}

	log.Info("relay stopped")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func init() {
	if len(os.Args) > 1 && (os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("Usage: relay")
		fmt.Println()
		fmt.Println("Faultline public relay — dumb Sentry-compatible envelope store.")
		fmt.Println("Mobile SDKs POST envelopes here; local faultline polls to ingest them.")
		fmt.Println()
		fmt.Println("Environment variables:")
		fmt.Println("  RELAY_ADDR               Listen address (default :9090)")
		fmt.Println("  RELAY_DB                  SQLite database path (default relay.db)")
		fmt.Println("  RELAY_PROJECTS            Comma-separated project_id:public_key pairs")
		fmt.Println("  RELAY_TTL_DAYS            Days to keep pulled envelopes (default 7)")
		fmt.Println("  RELAY_PURGE_INTERVAL_MIN  Purge interval in minutes (default 60)")
		os.Exit(0)
	}
}
