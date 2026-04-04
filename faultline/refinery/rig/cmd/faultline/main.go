package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/outdoorsea/faultline/internal/api"
	"github.com/outdoorsea/faultline/internal/ci"
	"github.com/outdoorsea/faultline/internal/dashboard"
	"github.com/outdoorsea/faultline/internal/crypto"
	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/dbmon"
	"github.com/outdoorsea/faultline/internal/gastown"
	"github.com/outdoorsea/faultline/internal/healthmon"
	"github.com/outdoorsea/faultline/internal/ingest"
	"github.com/outdoorsea/faultline/internal/integrations"
	_ "github.com/outdoorsea/faultline/internal/integrations/github" // register github_issues integration
	slackint "github.com/outdoorsea/faultline/internal/integrations/slack" // register slack_bot integration
	_ "github.com/lib/pq" // register PostgreSQL driver for dbmon
	"github.com/outdoorsea/faultline/internal/notify"
	"github.com/outdoorsea/faultline/internal/slackdm"
	"github.com/outdoorsea/faultline/internal/poller"
	"github.com/outdoorsea/faultline/internal/relay"
	"github.com/outdoorsea/faultline/internal/selfmon"
	"github.com/outdoorsea/faultline/internal/server"
	"github.com/outdoorsea/faultline/internal/uptimemon"
)

func main() {
	// Subcommand routing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "start":
			cmdStart()
			return
		case "stop":
			cmdStop()
			return
		case "status":
			cmdStatus()
			return
		case "register":
			cmdRegister()
			return
		case "relay":
			cmdRelay()
			return
		case "serve":
			// Fall through to server startup below.
		case "help", "-h", "--help":
			fmt.Println("Usage: faultline [command]")
			fmt.Println()
			fmt.Println("Commands:")
			fmt.Println("  serve      Run server in foreground (default)")
			fmt.Println("  start      Start server as background daemon")
			fmt.Println("  stop       Stop the background daemon")
			fmt.Println("  status     Show daemon status and health")
			fmt.Println("  register   Register a new project")
			fmt.Println("  relay      Run store-and-forward relay (SQLite, no Dolt)")
			fmt.Println("  help       Show this help")
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
			fmt.Fprintln(os.Stderr, "Run 'faultline help' for usage.")
			os.Exit(1)
		}
	}

	if err := runServe(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("fatal", "err", err)
		os.Exit(1)
	}
}

func runServe() error {
	// Write pidfile when running as daemon.
	if os.Getenv("FAULTLINE_DAEMON") == "1" {
		if err := writePID(); err != nil {
			return fmt.Errorf("error writing pidfile: %w", err)
		}
		defer removePID()
	}

	// Cloud mode: relay-only server (SQLite, no Dolt).
	if envOr("FAULTLINE_MODE", "") == "cloud" {
		return runCloud()
	}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})

	addr := envOr("FAULTLINE_ADDR", ":8080")
	dsn := envOr("FAULTLINE_DSN", "root@tcp(127.0.0.1:3307)/faultline")
	rateLimit := envOrFloat("FAULTLINE_RATE_LIMIT", 100) // events per second per project
	retentionDays := envOrInt("FAULTLINE_RETENTION_DAYS", 90)
	scrubPII := envOrBool("FAULTLINE_SCRUB_PII", true)

	dolt, err := db.Open(dsn)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer func() { _ = dolt.Close() }()

	// Bootstrap: if FAULTLINE_PROJECTS is set, seed those projects into the DB.
	// This is for first-run only — after that, use `faultline register`.
	if envProjects := os.Getenv("FAULTLINE_PROJECTS"); envProjects != "" {
		for _, p := range strings.Split(envProjects, ",") {
			parts := strings.SplitN(p, ":", 4)
			if len(parts) < 2 {
				continue
			}
			var id int64
			if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
				continue
			}
			name := parts[1]
			if len(parts) >= 3 && parts[2] != "" {
				name = parts[2]
			}
			_ = dolt.EnsureProject(context.Background(), id, name, name, parts[1])
			if len(parts) == 4 && parts[3] != "" {
				existing, _ := dolt.GetProject(context.Background(), id)
				if existing != nil && (existing.Config == nil || existing.Config.URL == "") {
					cfg := &db.ProjectConfig{URL: parts[3], DeploymentType: db.DeployLocal}
					if existing.Config != nil {
						cfg = existing.Config
						cfg.URL = parts[3]
					}
					_ = dolt.UpdateProjectConfig(context.Background(), id, cfg)
				}
			}
		}
	}

	// Load project auth from database — the DB is the source of truth.
	projects, err := dolt.ListProjects(context.Background())
	if err != nil {
		return fmt.Errorf("load projects: %w", err)
	}
	var records []ingest.ProjectRecord
	for _, p := range projects {
		records = append(records, ingest.ProjectRecord{
			ID:        p.ID,
			PublicKey: p.DSNPublicKey,
			Slug:      p.Slug,
		})
	}
	auth := ingest.NewProjectAuthFromRecords(records)

	// Self-monitoring: wrap the logger so error-level messages are reported
	// as Sentry events to faultline's own ingest endpoint.
	selfmonKey := os.Getenv("FAULTLINE_SELFMON_KEY")
	selfmonProjectID := int64(1)
	if selfmonKey == "" && len(records) > 0 {
		selfmonKey = records[0].PublicKey
		selfmonProjectID = records[0].ID
	}
	selfmonEndpoint := fmt.Sprintf("http://localhost%s/api/%d/store/", addr, selfmonProjectID)
	smHandler := selfmon.NewHandler(jsonHandler, selfmon.Config{
		Endpoint:  selfmonEndpoint,
		SentryKey: selfmonKey,
		MinLevel:  slog.LevelError,
	})
	log := slog.New(smHandler)
	log.Info("self-monitoring enabled", "endpoint", selfmonEndpoint)
	log.Info("loaded projects from database", "count", len(records))

	// Gas Town integration bridge.
	gtCfg := gastown.DefaultConfig()
	if apiURL := os.Getenv("FAULTLINE_API_URL"); apiURL != "" {
		gtCfg.APIBaseURL = apiURL
	}
	bridge := gastown.NewBridge(dolt, log, gtCfg, auth.RigForProject)

	// Webhook notifications: global fallback (env var) + per-project (DB config).
	var globalNotifier notify.Notifier
	if slackURL := os.Getenv("FAULTLINE_SLACK_WEBHOOK"); slackURL != "" {
		globalNotifier = notify.NewSlackWebhook(slackURL, gtCfg.APIBaseURL, log)
		log.Info("global slack notifications enabled")
	}
	provider := notify.NewDBProvider(dolt, log)
	dispatcher := notify.NewDispatcher(globalNotifier, provider, gtCfg.APIBaseURL, log)

	// Integration plugins: fire alongside webhooks for all lifecycle events.
	integDispatcher := integrations.NewDispatcher(
		&integrations.DBAdapter{DB: dolt},
		log.With("component", "integrations"),
	)
	dispatcher.SetIntegrations(integDispatcher)

	bridge.SetNotifier(dispatcher)

	autoRegister := envOr("FAULTLINE_AUTO_REGISTER", "true") == "true"
	handler := &ingest.Handler{
		DB:           dolt,
		Auth:         auth,
		Log:          log,
		OnEvent:      bridge.OnEvent,
		ScrubPII:     scrubPII,
		AutoRegister: autoRegister,
	}

	slackDMs := &slackdm.Sender{
		DB:  dolt,
		Log: log.With("component", "slack-dm"),
	}

	encKey, err := crypto.LoadKey()
	if err != nil {
		log.Warn("failed to load encryption key; database monitoring encryption disabled", "err", err)
	}

	apiHandler := &api.Handler{
		DB:            dolt,
		Log:           log,
		BaseURL:       gtCfg.APIBaseURL,
		Auth:          auth,
		HookSecret:    os.Getenv("FAULTLINE_CI_WEBHOOK_SECRET"), // HMAC for resolve hook
		SlackDMs:      slackDMs,
		EncryptionKey: encKey,
	}

	dash := &dashboard.Handler{
		DB:            dolt,
		Log:           log,
		SlackDMs:      slackDMs,
		EncryptionKey: encKey,
	}

	// CI webhook handler — converts GitHub Actions failures to faultline events.
	ciWebhookSecret := os.Getenv("FAULTLINE_CI_WEBHOOK_SECRET")
	ciHandler := &ci.Handler{
		Secret: ciWebhookSecret,
		Log:    log,
		LookupProject: func(repo string) int64 {
			// Map GitHub repos to faultline project IDs.
			// Projects register their repo via config.
			projects, err := dolt.ListProjects(context.Background())
			if err != nil {
				return 0
			}
			for _, p := range projects {
				if p.Slug == repo || p.Name == repo {
					return p.ID
				}
				// Also match by repo name only (without owner).
				parts := strings.Split(repo, "/")
				if len(parts) == 2 && (p.Slug == parts[1] || p.Name == parts[1]) {
					return p.ID
				}
			}
			return 0
		},
		OnFailure: func(ctx context.Context, projectID int64, evt ci.CIEvent) error {
			// Convert CI failure to a Sentry event and ingest it.
			raw := ci.ConvertToSentryEvent(evt)
			eventID := fmt.Sprintf("ci-%d-%s", evt.RunID, evt.Repo)
			return handler.ProcessCIEvent(ctx, projectID, eventID, raw)
		},
		OnSuccess: func(ctx context.Context, projectID int64, evt ci.CIEvent) error {
			// Store CI success in ci_runs for fix verification.
			return dolt.InsertCIRun(ctx, projectID, evt.Repo, evt.Branch, evt.Commit,
				evt.Workflow, evt.RunID, evt.RunURL, evt.Conclusion, evt.Actor, evt.Timestamp)
		},
	}

	// Slack bot integration — receives interaction webhooks from Slack.
	var slackHandler *slackint.WebhookHandler
	slackSigningSecret := os.Getenv("FAULTLINE_SLACK_SIGNING_SECRET")
	slackBotToken := os.Getenv("FAULTLINE_SLACK_BOT_TOKEN")
	if slackBotToken != "" {
		log.Info("slack bot integration enabled")
	}
	if slackSigningSecret != "" {
		slackHandler = &slackint.WebhookHandler{
			SigningSecret: slackSigningSecret,
			DB:            dolt,
			Log:           log.With("component", "slack"),
		}
		log.Info("slack interaction webhook enabled")
	}
	_ = slackBotToken // used by per-project integrations via integrations_config

	rl := ingest.NewRateLimiter(rateLimit, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start background Dolt commit loop (60s interval).
	committer := db.NewCommitter(dolt, log, 60*time.Second)
	go committer.Run(ctx)

	// Start Gas Town bead resolution poller.
	go bridge.RunPoller(ctx)

	// Start slow-burn sweep (files beads for issues that missed the threshold).
	go bridge.RunSlowBurn(ctx)

	// Start Dolt/beads health monitor.
	hmCfg := healthmon.DefaultConfig()
	hmCfg.RunGTDoctor = envOrBool("FAULTLINE_HEALTHMON_DOCTOR", false)
	hm := healthmon.New(dolt, log, hmCfg, func(severity, message string) {
		if severity == "error" {
			log.Error("healthmon: " + message)
		} else {
			log.Warn("healthmon: " + message)
		}
	})
	go hm.Run(ctx)

	// Start uptime monitor — periodic HTTP health checks for project URLs.
	uptimeInterval := time.Duration(envOrInt("FAULTLINE_UPTIME_INTERVAL_SECS", 60)) * time.Second
	um := uptimemon.New(
		&uptimemon.DBProvider{DB: dolt.DB},
		log,
		uptimeInterval,
		10*time.Second,
	)
	// In Docker on Mac, localhost URLs need to be rewritten to reach the host.
	if hostRewrite := os.Getenv("FAULTLINE_UPTIME_HOST"); hostRewrite != "" {
		um.HostRewrite = hostRewrite
	} else if strings.Contains(dsn, "host.docker.internal") {
		um.HostRewrite = "host.docker.internal"
	}
	um.OnStateChange = func(projectID int64, up bool, responseMS int, statusCode int) {
		p, _ := dolt.GetProject(context.Background(), projectID)
		name := fmt.Sprintf("project-%d", projectID)
		if p != nil {
			name = p.Name
		}
		if up {
			log.Info("project recovered", "project", name, "response_ms", responseMS)
		} else {
			log.Error("project down", "project", name, "status_code", statusCode)
		}
	}
	go um.Run(ctx)

	// Start database monitor — periodic health checks for monitored databases.
	dm := dbmon.New(
		&dbmon.SQLDBProvider{DB: dolt.DB},
		log,
		10*time.Second,
	)
	dm.RegisterChecker("postgres", dbmon.NewPostgresChecker())
	dm.OnStateChange = func(target dbmon.DatabaseTarget, oldStatus, newStatus dbmon.Status, results []dbmon.CheckResult) {
		if target.ProjectID == nil {
			log.Warn("dbmon: state change for target without project_id, skipping event",
				"database", target.Name, "old", string(oldStatus), "new", string(newStatus))
			return
		}
		projectID := *target.ProjectID

		// Map status to Sentry level.
		level := "info"
		switch newStatus {
		case dbmon.StatusDown:
			level = "error"
		case dbmon.StatusDegraded:
			level = "warning"
		}

		// Create one event per check result that contributed to the transition.
		for _, r := range results {
			title := fmt.Sprintf("%s %s: %s — %s", target.DBType, target.Name, r.CheckType, r.Message)
			fingerprint := fmt.Sprintf("dbmon|%s|%s", target.ID, r.CheckType)
			eventID := fmt.Sprintf("dbmon-%s-%s-%d", target.ID, r.CheckType, time.Now().UnixNano())

			raw, _ := json.Marshal(map[string]interface{}{
				"event_id":    eventID,
				"platform":    "database",
				"level":       level,
				"message":     title,
				"fingerprint": []string{fingerprint},
				"tags": map[string]string{
					"db_type":    target.DBType,
					"db_name":    target.Name,
					"db_id":      target.ID,
					"check_type": r.CheckType,
					"old_status": string(oldStatus),
					"new_status": string(newStatus),
				},
			})

			if err := handler.ProcessCIEvent(context.Background(), projectID, eventID, raw); err != nil {
				log.Error("dbmon: ingest event failed", "err", err, "database", target.Name, "check", r.CheckType)
			}
		}
	}
	go dm.Run(ctx)

	// Start relay poller — pulls events from the public relay for mobile/external apps.
	// Disabled by default; set FAULTLINE_RELAY_URL to enable (e.g. https://faultline.live).
	if relayURL := os.Getenv("FAULTLINE_RELAY_URL"); relayURL != "" {
		relayInterval := time.Duration(envOrInt("FAULTLINE_RELAY_POLL_SECS", 30)) * time.Second
		rp := poller.NewRelayPoller(relayURL, relayInterval, func(ctx context.Context, projectID int64, publicKey string, payload []byte) error {
			return handler.IngestRaw(ctx, projectID, payload)
		}, log)
		if token := os.Getenv("FAULTLINE_RELAY_TOKEN"); token != "" {
			rp.SetPollToken(token)
		}
		// Wire CI webhook processing into the relay poller.
		rp.SetCIWebhookHandler(func(ctx context.Context, payload []byte) error {
			// Unwrap the relay's CI webhook wrapper.
			var wrapped struct {
				Type    string          `json:"type"`
				Source  string          `json:"source"`
				Event   string          `json:"event"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(payload, &wrapped); err != nil {
				return fmt.Errorf("unwrap ci webhook: %w", err)
			}
			if wrapped.Type != "ci_webhook" || wrapped.Event != "workflow_run" {
				return nil // not a workflow_run, skip
			}
			// Create a fake HTTP request to reuse the CI handler.
			req, _ := http.NewRequestWithContext(ctx, "POST", "/api/hooks/ci/github",
				bytes.NewReader(wrapped.Payload))
			req.Header.Set("X-GitHub-Event", wrapped.Event)
			rec := &discardResponseWriter{}
			ciHandler.HandleGitHub(rec, req)
			return nil
		})
		go rp.Run(ctx)
	}

	// Start snooze sweep (60s interval) — expires snoozed issues.
	snoozeSweep := db.NewSnoozeSweep(dolt, log, 60*time.Second)
	go snoozeSweep.Run(ctx)

	// Start data retention worker.
	retCfg := db.DefaultRetentionConfig()
	retCfg.EventTTL = time.Duration(retentionDays) * 24 * time.Hour
	retCfg.SessionTTL = time.Duration(retentionDays) * 24 * time.Hour
	retention := db.NewRetentionWorker(dolt, log, retCfg)
	go retention.Run(ctx)

	return server.Run(ctx, server.Config{
		Addr:        addr,
		Handler:     handler,
		API:         apiHandler,
		Dashboard:   dash,
		CI:          ciHandler,
		Slack:       slackHandler,
		RateLimiter: rl,
		DB:          dolt,
		Log:         log,
	})
}

// runCloud starts the server in cloud/relay mode: SQLite-backed envelope
// storage with relay poll/ack API. No Dolt, no dashboard, no bridge, no monitors.
func runCloud() error {
	addr := envOr("FAULTLINE_ADDR", ":8080")
	dbPath := envOr("FAULTLINE_RELAY_DB", "relay.db")
	ttl := time.Duration(envOrInt("FAULTLINE_RELAY_TTL_HOURS", 72)) * time.Hour
	pollToken := os.Getenv("FAULTLINE_RELAY_TOKEN")
	projectPairs := strings.Split(envOr("FAULTLINE_PROJECTS", ""), ",")

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if len(projectPairs) == 1 && projectPairs[0] == "" {
		return fmt.Errorf("FAULTLINE_PROJECTS required in cloud mode (format: id:key,…)")
	}

	var relayPairs []string
	for _, p := range projectPairs {
		parts := strings.SplitN(p, ":", 3)
		if len(parts) >= 2 {
			relayPairs = append(relayPairs, parts[0]+":"+parts[1])
		}
	}

	auth, err := relay.NewAuth(relayPairs)
	if err != nil {
		return fmt.Errorf("invalid project config: %w", err)
	}

	store, err := relay.NewStore(dbPath, ttl)
	if err != nil {
		return fmt.Errorf("open relay store: %w", err)
	}
	defer func() { _ = store.Close() }()

	if pollToken == "" {
		log.Warn("FAULTLINE_RELAY_TOKEN not set — poll/ack endpoints are unauthenticated")
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

	// Purge loop for expired envelopes.
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

	log.Info("cloud mode listening", "addr", addr, "db", dbPath, "projects", len(relayPairs))

	errCh := make(chan error, 1)
	go func() {
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

// discardResponseWriter is a no-op http.ResponseWriter for internal handler calls.
type discardResponseWriter struct {
	code int
}

func (d *discardResponseWriter) Header() http.Header         { return http.Header{} }
func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(code int)        { d.code = code }
