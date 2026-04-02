package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/outdoorsea/faultline/internal/relay"
)

// cmdRelay runs the relay as a standalone store-and-forward proxy.
// It accepts Sentry SDK envelopes, buffers them in SQLite, and serves
// a poll/ack API for the main faultline server to drain.
//
// No Dolt dependency — stays up independently of faultline.
func cmdRelay() {
	addr := envOr("FAULTLINE_RELAY_ADDR", ":3031")
	dbPath := envOr("FAULTLINE_RELAY_DB", "relay.db")
	ttl := time.Duration(envOrInt("FAULTLINE_RELAY_TTL_HOURS", 72)) * time.Hour
	pollToken := os.Getenv("FAULTLINE_RELAY_TOKEN")
	projectPairs := strings.Split(envOr("FAULTLINE_PROJECTS", ""), ",")

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if len(projectPairs) == 1 && projectPairs[0] == "" {
		fmt.Fprintln(os.Stderr, "error: FAULTLINE_PROJECTS required (format: id:key[:rig],…)")
		os.Exit(1)
	}

	// Build relay auth from the same project pairs format.
	// relay.NewAuth wants "id:key" pairs.
	var relayPairs []string
	for _, p := range projectPairs {
		parts := strings.SplitN(p, ":", 3)
		if len(parts) >= 2 {
			relayPairs = append(relayPairs, parts[0]+":"+parts[1])
		}
	}

	auth, err := relay.NewAuth(relayPairs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid project config: %v\n", err)
		os.Exit(1)
	}

	store, err := relay.NewStore(dbPath, ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open relay store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	if pollToken == "" {
		log.Warn("FAULTLINE_RELAY_TOKEN not set — poll/ack endpoints are unauthenticated")
	}
	if strings.HasPrefix(dbPath, "/tmp") {
		log.Warn("relay DB is in /tmp — data will not survive container restarts", "path", dbPath)
	}

	handler := &relay.Handler{
		Store:     store,
		Auth:      auth,
		Log:       log,
		PollToken: pollToken,
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start purge loop.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
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

	log.Info("relay listening", "addr", addr, "db", dbPath, "ttl", ttl,
		"projects", len(relayPairs))

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("relay server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("relay shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
