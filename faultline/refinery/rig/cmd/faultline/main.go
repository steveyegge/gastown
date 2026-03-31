package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/outdoorsea/faultline/internal/api"
	"github.com/outdoorsea/faultline/internal/dashboard"
	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/gastown"
	"github.com/outdoorsea/faultline/internal/ingest"
	"github.com/outdoorsea/faultline/internal/notify"
	"github.com/outdoorsea/faultline/internal/selfmon"
	"github.com/outdoorsea/faultline/internal/server"
)

func main() {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})

	addr := envOr("FAULTLINE_ADDR", ":8080")
	dsn := envOr("FAULTLINE_DSN", "root@tcp(127.0.0.1:3307)/faultline")
	projectPairs := strings.Split(envOr("FAULTLINE_PROJECTS", "1:default_key"), ",")
	rateLimit := envOrFloat("FAULTLINE_RATE_LIMIT", 100) // events per second per project
	retentionDays := envOrInt("FAULTLINE_RETENTION_DAYS", 90)
	scrubPII := envOrBool("FAULTLINE_SCRUB_PII", true)

	auth, err := ingest.NewProjectAuth(projectPairs)
	if err != nil {
		slog.New(jsonHandler).Error("invalid project auth config", "err", err)
		os.Exit(1)
	}

	// Self-monitoring: wrap the logger so error-level messages are reported
	// as Sentry events to faultline's own ingest endpoint.
	selfmonKey := envOr("FAULTLINE_SELFMON_KEY", firstKey(projectPairs))
	selfmonEndpoint := fmt.Sprintf("http://localhost%s/api/0/store/", addr)
	smHandler := selfmon.NewHandler(jsonHandler, selfmon.Config{
		Endpoint:  selfmonEndpoint,
		SentryKey: selfmonKey,
		MinLevel:  slog.LevelError,
	})
	log := slog.New(smHandler)
	log.Info("self-monitoring enabled", "endpoint", selfmonEndpoint)

	dolt, err := db.Open(dsn)
	if err != nil {
		log.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer dolt.Close()

	// Gas Town integration bridge.
	gtCfg := gastown.DefaultConfig()
	if apiURL := os.Getenv("FAULTLINE_API_URL"); apiURL != "" {
		gtCfg.APIBaseURL = apiURL
	}
	bridge := gastown.NewBridge(dolt, log, gtCfg, auth.RigForProject)

	// Slack webhook notifications (optional).
	if slackURL := os.Getenv("FAULTLINE_SLACK_WEBHOOK"); slackURL != "" {
		slack := notify.NewSlackWebhook(slackURL, gtCfg.APIBaseURL, log)
		if slack != nil {
			bridge.SetNotifier(slack)
			log.Info("slack notifications enabled")
		}
	}

	handler := &ingest.Handler{
		DB:       dolt,
		Auth:     auth,
		Log:      log,
		OnEvent:  bridge.OnEvent,
		ScrubPII: scrubPII,
	}

	apiHandler := &api.Handler{
		DB:       dolt,
		Log:      log,
		Projects: buildProjectInfo(projectPairs, auth),
		BaseURL:  gtCfg.APIBaseURL,
	}

	dash := &dashboard.Handler{
		DB:  dolt,
		Log: log,
	}

	rl := ingest.NewRateLimiter(rateLimit, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start background Dolt commit loop (60s interval).
	committer := db.NewCommitter(dolt, log, 60*time.Second)
	go committer.Run(ctx)

	// Start Gas Town bead resolution poller.
	go bridge.RunPoller(ctx)

	// Start data retention worker.
	retCfg := db.DefaultRetentionConfig()
	retCfg.EventTTL = time.Duration(retentionDays) * 24 * time.Hour
	retCfg.SessionTTL = time.Duration(retentionDays) * 24 * time.Hour
	retention := db.NewRetentionWorker(dolt, log, retCfg)
	go retention.Run(ctx)

	if err := server.Run(ctx, server.Config{
		Addr:        addr,
		Handler:     handler,
		API:         apiHandler,
		Dashboard:   dash,
		RateLimiter: rl,
		DB:          dolt,
		Log:         log,
	}); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}

func buildProjectInfo(pairs []string, auth *ingest.ProjectAuth) []api.ProjectInfo {
	var projects []api.ProjectInfo
	for _, p := range pairs {
		parts := strings.SplitN(p, ":", 3)
		if len(parts) < 2 {
			continue
		}
		var id int64
		fmt.Sscanf(parts[0], "%d", &id)
		info := api.ProjectInfo{
			ID:        id,
			PublicKey: parts[1],
			Rig:       auth.RigForProject(id),
		}
		projects = append(projects, info)
	}
	return projects
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

func envOrBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return fallback
}

func envOrFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// firstKey extracts the public key from the first project pair ("id:key[:rig]").
func firstKey(pairs []string) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := strings.SplitN(pairs[0], ":", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
