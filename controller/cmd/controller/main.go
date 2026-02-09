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
	"github.com/steveyegge/gastown/controller/internal/podmanager"
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
	// TODO: Replace StubReporter with RPC-based reporter that talks to daemon
	// directly (no bd binary needed in distroless container).
	status := statusreporter.NewStubReporter(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx, logger, cfg, watcher, pods, status); err != nil {
		logger.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

// run is the main controller loop. It reads beads events and dispatches
// pod operations. Separated from main() for testability.
func run(ctx context.Context, logger *slog.Logger, cfg *config.Config, watcher beadswatcher.Watcher, pods podmanager.Manager, status statusreporter.Reporter) error {
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
	go runPeriodicSync(ctx, logger, status, syncInterval)

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

// runPeriodicSync runs SyncAll at a regular interval to reconcile pod statuses.
func runPeriodicSync(ctx context.Context, logger *slog.Logger, status statusreporter.Reporter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := status.SyncAll(ctx); err != nil {
				logger.Warn("periodic sync failed", "error", err)
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
			"BEADS_AUTO_START_DAEMON": "false",
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

	// Wire Claude OAuth credentials (Max/Corp accounts) from config.
	if cfg.CredentialsSecret != "" {
		spec.CredentialsSecret = cfg.CredentialsSecret
	}

	// Wire daemon token so agent pods can authenticate to the daemon.
	if cfg.DaemonTokenSecret != "" {
		spec.DaemonTokenSecret = cfg.DaemonTokenSecret
	}

	// Agent image has coop built-in: use HTTP probes on agent container.
	if cfg.CoopBuiltin {
		spec.CoopBuiltin = true
	}

	// Wire Coop sidecar when image is configured (mutually exclusive with CoopBuiltin).
	if cfg.CoopImage != "" && !cfg.CoopBuiltin {
		spec.CoopSidecar = &podmanager.CoopSidecarSpec{
			Image: cfg.CoopImage,
		}
		// Wire NATS integration from event metadata.
		if natsURL := event.Metadata["nats_url"]; natsURL != "" {
			spec.CoopSidecar.NatsURL = natsURL
		}
		// Wire auth token secret if provided in event metadata.
		if authSecret := event.Metadata["coop_auth_secret"]; authSecret != "" {
			spec.CoopSidecar.AuthTokenSecret = authSecret
		}
		if natsSecret := event.Metadata["nats_token_secret"]; natsSecret != "" {
			spec.CoopSidecar.NatsTokenSecret = natsSecret
		}
	}

	return spec
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
