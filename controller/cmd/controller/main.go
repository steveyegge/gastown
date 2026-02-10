// Command controller is the Gas Town K8s controller — a thin reactive bridge
// between beads lifecycle events and Kubernetes pod operations.
//
// Architecture: Beads IS the control plane. This controller watches beads
// events via BD Daemon and translates them to pod create/delete operations.
// It does NOT use controller-runtime or CRD reconciliation loops.
//
// See docs/design/k8s-crd-schema.md and docs/design/k8s-reconciliation-loops.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/steveyegge/gastown/controller/internal/beadswatcher"
	"github.com/steveyegge/gastown/controller/internal/config"
	"github.com/steveyegge/gastown/controller/internal/daemonclient"
	"github.com/steveyegge/gastown/controller/internal/podmanager"
	"github.com/steveyegge/gastown/controller/internal/reconciler"
	"github.com/steveyegge/gastown/controller/internal/statusreporter"
)

func main() {
	cfg := config.Parse()

	logger := setupLogger(cfg.LogLevel)
	logger.Info("starting gastown controller",
		"daemon", fmt.Sprintf("%s:%d", cfg.DaemonHost, cfg.DaemonPort),
		"namespace", cfg.Namespace)

	k8sClient, err := buildK8sClient(cfg.KubeConfig)
	if err != nil {
		logger.Error("failed to create K8s client", "error", err)
		os.Exit(1)
	}

	watcher := beadswatcher.NewSSEWatcher(beadswatcher.Config{
		DaemonHTTPURL: fmt.Sprintf("http://%s:%d", cfg.DaemonHost, cfg.DaemonHTTPPort),
		DaemonToken:   cfg.DaemonToken,
		Namespace:     cfg.Namespace,
		DefaultImage:  cfg.DefaultImage,
		DaemonHost:    cfg.DaemonHost,
		DaemonPort:    fmt.Sprintf("%d", cfg.DaemonPort),
	}, logger)
	pods := podmanager.New(k8sClient, logger)

	// Daemon client for HTTP API access (used by reconciler and status reporter).
	daemon := daemonclient.New(daemonclient.Config{
		BaseURL: fmt.Sprintf("http://%s:%d", cfg.DaemonHost, cfg.DaemonHTTPPort),
		Token:   cfg.DaemonToken,
	})
	status := statusreporter.NewHTTPReporter(daemon, k8sClient, cfg.Namespace, logger)

	// Populate rig cache from daemon rig beads.
	cfg.RigCache = make(map[string]config.RigCacheEntry)
	refreshRigCache(context.Background(), logger, daemon, cfg)

	rec := reconciler.New(daemon, pods, cfg, logger, BuildSpecFromBeadInfo)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx, logger, cfg, watcher, pods, status, rec, daemon); err != nil {
		logger.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

// run is the main controller loop. It reads beads events and dispatches
// pod operations. Separated from main() for testability.
func run(ctx context.Context, logger *slog.Logger, cfg *config.Config, watcher beadswatcher.Watcher, pods podmanager.Manager, status statusreporter.Reporter, rec *reconciler.Reconciler, daemon *daemonclient.DaemonClient) error {
	// Run reconciler once at startup to catch beads created during downtime.
	if rec != nil {
		logger.Info("running startup reconciliation")
		if err := rec.Reconcile(ctx); err != nil {
			logger.Warn("startup reconciliation failed", "error", err)
		}
	}

	// Start beads watcher in background.
	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Start(ctx)
	}()

	// Start periodic SyncAll reconciliation.
	syncInterval := 60 * time.Second
	if cfg.SyncInterval > 0 {
		syncInterval = cfg.SyncInterval
	}
	go runPeriodicSync(ctx, logger, status, rec, daemon, cfg, syncInterval)

	logger.Info("controller ready, waiting for beads events",
		"sync_interval", syncInterval)

	for {
		select {
		case event, ok := <-watcher.Events():
			if !ok {
				return nil // channel closed, watcher shut down
			}
			if err := handleEvent(ctx, logger, cfg, event, pods, status); err != nil {
				logger.Error("failed to handle event", "type", event.Type, "agent", event.AgentName, "error", err)
			}

		case err := <-watcherDone:
			return fmt.Errorf("watcher stopped: %w", err)

		case <-ctx.Done():
			logger.Info("shutting down controller")
			return nil
		}
	}
}

// runPeriodicSync runs SyncAll, rig cache refresh, and reconciliation at a regular interval.
func runPeriodicSync(ctx context.Context, logger *slog.Logger, status statusreporter.Reporter, rec *reconciler.Reconciler, daemon *daemonclient.DaemonClient, cfg *config.Config, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := status.SyncAll(ctx); err != nil {
				logger.Warn("periodic status sync failed", "error", err)
			}
			// Refresh rig cache from daemon.
			refreshRigCache(ctx, logger, daemon, cfg)
			// Run reconciler to converge desired vs actual state.
			if rec != nil {
				if err := rec.Reconcile(ctx); err != nil {
					logger.Warn("periodic reconciliation failed", "error", err)
				}
			}
			// Log metrics snapshot after each sync.
			m := status.Metrics()
			logger.Info("metrics",
				"reports_total", m.StatusReportsTotal,
				"report_errors", m.StatusReportErrors,
				"sync_runs", m.SyncAllRuns,
				"sync_errors", m.SyncAllErrors)
		case <-ctx.Done():
			return
		}
	}
}

// handleEvent translates a beads lifecycle event into K8s pod operations.
func handleEvent(ctx context.Context, logger *slog.Logger, cfg *config.Config, event beadswatcher.Event, pods podmanager.Manager, status statusreporter.Reporter) error {
	logger.Info("handling beads event",
		"type", event.Type, "rig", event.Rig, "role", event.Role,
		"agent", event.AgentName, "bead", event.BeadID)

	agentBeadID := fmt.Sprintf("gt-%s-%s-%s", event.Rig, event.Role, event.AgentName)

	switch event.Type {
	case beadswatcher.AgentSpawn:
		spec := buildAgentPodSpec(cfg, event)
		if err := pods.CreateAgentPod(ctx, spec); err != nil {
			return err
		}
		// Report backend metadata so ResolveBackend() can find this agent.
		if spec.CoopSidecar != nil {
			coopPort := spec.CoopSidecar.Port
			if coopPort == 0 {
				coopPort = podmanager.CoopDefaultPort
			}
			_ = status.ReportBackendMetadata(ctx, agentBeadID, statusreporter.BackendMetadata{
				PodName:   spec.PodName(),
				Namespace: spec.Namespace,
				Backend:   "coop",
				CoopURL:   fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", spec.PodName(), spec.Namespace, coopPort),
			})
		}
		// Report spawning status to beads.
		_ = status.ReportPodStatus(ctx, agentBeadID, statusreporter.PodStatus{
			PodName:   spec.PodName(),
			Namespace: spec.Namespace,
			Phase:     string("Pending"),
			Ready:     false,
		})
		return nil

	case beadswatcher.AgentDone, beadswatcher.AgentKill:
		podName := fmt.Sprintf("gt-%s-%s-%s", event.Rig, event.Role, event.AgentName)
		ns := namespaceFromEvent(event, cfg.Namespace)
		err := pods.DeleteAgentPod(ctx, podName, ns)
		// Clear backend metadata so stale Coop URLs don't linger.
		_ = status.ReportBackendMetadata(ctx, agentBeadID, statusreporter.BackendMetadata{})
		// Report done status to beads regardless of delete error.
		phase := "Succeeded"
		if event.Type == beadswatcher.AgentKill {
			phase = "Failed"
		}
		_ = status.ReportPodStatus(ctx, agentBeadID, statusreporter.PodStatus{
			PodName:   podName,
			Namespace: ns,
			Phase:     phase,
			Ready:     false,
		})
		return err

	case beadswatcher.AgentStuck:
		// Delete and recreate the pod to restart the agent.
		podName := fmt.Sprintf("gt-%s-%s-%s", event.Rig, event.Role, event.AgentName)
		ns := namespaceFromEvent(event, cfg.Namespace)
		if err := pods.DeleteAgentPod(ctx, podName, ns); err != nil {
			logger.Warn("failed to delete stuck pod (may not exist)", "pod", podName, "error", err)
		}
		spec := buildAgentPodSpec(cfg, event)
		if err := pods.CreateAgentPod(ctx, spec); err != nil {
			return err
		}
		// Report restarting status.
		_ = status.ReportPodStatus(ctx, agentBeadID, statusreporter.PodStatus{
			PodName:   spec.PodName(),
			Namespace: spec.Namespace,
			Phase:     string("Pending"),
			Ready:     false,
			Message:   "restarted due to stuck detection",
		})
		return nil

	default:
		logger.Warn("unknown event type", "type", event.Type)
		return nil
	}
}

// BuildSpecFromBeadInfo constructs an AgentPodSpec from config and bead identity,
// without an SSE event. Used by the reconciler to produce specs identical to
// those created by handleEvent, using controller config for all metadata.
func BuildSpecFromBeadInfo(cfg *config.Config, rig, role, agentName string) podmanager.AgentPodSpec {
	spec := podmanager.AgentPodSpec{
		Rig:       rig,
		Role:      role,
		AgentName: agentName,
		Image:     cfg.DefaultImage,
		Namespace: cfg.Namespace,
		Env: map[string]string{
			"BD_DAEMON_HOST":          cfg.DaemonHost,
			"BD_DAEMON_PORT":          fmt.Sprintf("%d", cfg.DaemonPort),
			"BD_DAEMON_HTTP_PORT":     fmt.Sprintf("%d", cfg.DaemonHTTPPort),
			"BD_DAEMON_HTTP_URL":      fmt.Sprintf("http://%s:%d", cfg.DaemonHost, cfg.DaemonHTTPPort),
			"BEADS_AUTO_START_DAEMON": "false",
			"BEADS_DOLT_SERVER_MODE":  "1",
			"GT_TOWN_NAME":            cfg.TownName,
		},
	}

	defaults := podmanager.DefaultPodDefaultsForRole(role)
	podmanager.ApplyDefaults(&spec, defaults)

	applyCommonConfig(cfg, &spec)

	return spec
}

// buildAgentPodSpec constructs a full AgentPodSpec from an event and config.
// It applies role-specific defaults, then overlays event metadata.
func buildAgentPodSpec(cfg *config.Config, event beadswatcher.Event) podmanager.AgentPodSpec {
	ns := namespaceFromEvent(event, cfg.Namespace)

	spec := podmanager.AgentPodSpec{
		Rig:       event.Rig,
		Role:      event.Role,
		AgentName: event.AgentName,
		Image:     event.Metadata["image"],
		Namespace: ns,
		Env: map[string]string{
			"BD_DAEMON_HOST":          metadataOr(event, "daemon_host", cfg.DaemonHost),
			"BD_DAEMON_PORT":          metadataOr(event, "daemon_port", fmt.Sprintf("%d", cfg.DaemonPort)),
			"BD_DAEMON_HTTP_PORT":     metadataOr(event, "daemon_http_port", fmt.Sprintf("%d", cfg.DaemonHTTPPort)),
			"BD_DAEMON_HTTP_URL":      fmt.Sprintf("http://%s:%d", metadataOr(event, "daemon_host", cfg.DaemonHost), cfg.DaemonHTTPPort),
			"BEADS_AUTO_START_DAEMON": "false",
			"BEADS_DOLT_SERVER_MODE":  "1",
			"GT_TOWN_NAME":            cfg.TownName,
		},
	}

	// Apply role-specific defaults (workspace storage, resources).
	defaults := podmanager.DefaultPodDefaultsForRole(event.Role)
	podmanager.ApplyDefaults(&spec, defaults)

	// Overlay event metadata for optional fields.
	if sa := event.Metadata["service_account"]; sa != "" {
		spec.ServiceAccountName = sa
	}
	if cm := event.Metadata["configmap"]; cm != "" {
		spec.ConfigMapName = cm
	}

	// Wire up ANTHROPIC_API_KEY from event metadata or controller config.
	apiKeySecret := event.Metadata["api_key_secret"]
	if apiKeySecret == "" {
		apiKeySecret = cfg.APIKeySecret
	}
	if apiKeySecret != "" {
		secretKey := metadataOr(event, "api_key_secret_key", "ANTHROPIC_API_KEY")
		spec.SecretEnv = append(spec.SecretEnv, podmanager.SecretEnvSource{
			EnvName:    "ANTHROPIC_API_KEY",
			SecretName: apiKeySecret,
			SecretKey:  secretKey,
		})
	}

	// Apply common config (credentials, daemon token, coop, NATS).
	applyCommonConfig(cfg, &spec)

	// Wire Coop sidecar NATS overrides from event metadata.
	if spec.CoopSidecar != nil {
		if natsURL := event.Metadata["nats_url"]; natsURL != "" {
			spec.CoopSidecar.NatsURL = natsURL
		}
		if authSecret := event.Metadata["coop_auth_secret"]; authSecret != "" {
			spec.CoopSidecar.AuthTokenSecret = authSecret
		}
		if natsSecret := event.Metadata["nats_token_secret"]; natsSecret != "" {
			spec.CoopSidecar.NatsTokenSecret = natsSecret
		}
	}

	return spec
}

// applyCommonConfig wires controller-level config into an AgentPodSpec.
// Shared by both BuildSpecFromBeadInfo (reconciler) and buildAgentPodSpec (events).
func applyCommonConfig(cfg *config.Config, spec *podmanager.AgentPodSpec) {
	if cfg.CredentialsSecret != "" {
		spec.CredentialsSecret = cfg.CredentialsSecret
	}
	if cfg.DaemonTokenSecret != "" {
		spec.DaemonTokenSecret = cfg.DaemonTokenSecret
	}

	// Coop: either built-in (HTTP probes on agent) or sidecar (separate container).
	if cfg.CoopBuiltin {
		spec.CoopBuiltin = true
	}
	if cfg.CoopImage != "" && !cfg.CoopBuiltin {
		spec.CoopSidecar = &podmanager.CoopSidecarSpec{
			Image: cfg.CoopImage,
		}
	}

	// Wire git mirror info from rig cache.
	if entry, ok := cfg.RigCache[spec.Rig]; ok {
		if entry.GitMirrorSvc != "" {
			spec.GitMirrorService = entry.GitMirrorSvc
		}
		if entry.GitURL != "" {
			spec.GitURL = entry.GitURL
		}
		if entry.DefaultBranch != "" {
			spec.GitDefaultBranch = entry.DefaultBranch
		}
	}

	// Build GT_RIGS env var from rig cache for entrypoint rig registration.
	if len(cfg.RigCache) > 0 {
		var rigEntries []string
		for name, entry := range cfg.RigCache {
			if entry.GitURL != "" {
				prefix := "hq" // default prefix
				// Extract prefix from the entry if available
				if p := entry.DefaultBranch; p != "" {
					_ = p // branch, not prefix — ignore
				}
				// Use rig name as-is — entrypoint expects name=url:prefix
				rigEntries = append(rigEntries, fmt.Sprintf("%s=%s:%s", name, entry.GitURL, prefix))
			}
		}
		if len(rigEntries) > 0 {
			spec.Env["GT_RIGS"] = strings.Join(rigEntries, ",")
		}
	}

	// Wire NATS config: sidecar gets dedicated env vars, built-in gets plain env.
	if cfg.NatsURL != "" {
		if spec.CoopSidecar != nil {
			spec.CoopSidecar.NatsURL = cfg.NatsURL
		} else {
			spec.Env["COOP_NATS_URL"] = cfg.NatsURL
		}
	}
	if cfg.NatsTokenSecret != "" {
		if spec.CoopSidecar != nil {
			spec.CoopSidecar.NatsTokenSecret = cfg.NatsTokenSecret
		} else {
			spec.SecretEnv = append(spec.SecretEnv, podmanager.SecretEnvSource{
				EnvName:    "COOP_NATS_TOKEN",
				SecretName: cfg.NatsTokenSecret,
				SecretKey:  "token",
			})
		}
	}
}

// namespaceFromEvent returns the namespace from event metadata or a default.
func namespaceFromEvent(event beadswatcher.Event, defaultNS string) string {
	if ns := event.Metadata["namespace"]; ns != "" {
		return ns
	}
	return defaultNS
}

// metadataOr returns the event metadata value for key, or fallback if empty.
func metadataOr(event beadswatcher.Event, key, fallback string) string {
	if v := event.Metadata[key]; v != "" {
		return v
	}
	return fallback
}

func buildK8sClient(kubeconfig string) (kubernetes.Interface, error) {
	var cfg *rest.Config
	var err error

	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("building k8s config: %w", err)
	}

	return kubernetes.NewForConfig(cfg)
}

// refreshRigCache queries the daemon for rig beads and updates cfg.RigCache.
func refreshRigCache(ctx context.Context, logger *slog.Logger, daemon *daemonclient.DaemonClient, cfg *config.Config) {
	rigs, err := daemon.ListRigBeads(ctx)
	if err != nil {
		logger.Warn("failed to refresh rig cache", "error", err)
		return
	}
	for name, info := range rigs {
		cfg.RigCache[name] = config.RigCacheEntry{
			GitMirrorSvc:  info.GitMirrorSvc,
			GitURL:        info.GitURL,
			DefaultBranch: info.DefaultBranch,
		}
	}
	logger.Info("refreshed rig cache", "count", len(rigs))
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}
