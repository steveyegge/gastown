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

	watcher := beadswatcher.NewActivityWatcher(beadswatcher.Config{
		TownRoot:     cfg.TownRoot,
		BdBinary:     cfg.BdBinary,
		Namespace:    cfg.Namespace,
		DefaultImage: cfg.DefaultImage,
		DaemonHost:   cfg.DaemonHost,
		DaemonPort:   fmt.Sprintf("%d", cfg.DaemonPort),
	}, logger)
	pods := podmanager.New(k8sClient, logger)
	status := statusreporter.NewStubReporter(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx, logger, watcher, pods, status); err != nil {
		logger.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

// run is the main controller loop. It reads beads events and dispatches
// pod operations. Separated from main() for testability.
func run(ctx context.Context, logger *slog.Logger, watcher beadswatcher.Watcher, pods podmanager.Manager, status statusreporter.Reporter) error {
	// Start beads watcher in background.
	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Start(ctx)
	}()

	logger.Info("controller ready, waiting for beads events")

	for {
		select {
		case event, ok := <-watcher.Events():
			if !ok {
				return nil // channel closed, watcher shut down
			}
			if err := handleEvent(ctx, logger, event, pods, status); err != nil {
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

// handleEvent translates a beads lifecycle event into K8s pod operations.
func handleEvent(ctx context.Context, logger *slog.Logger, event beadswatcher.Event, pods podmanager.Manager, status statusreporter.Reporter) error {
	logger.Info("handling beads event",
		"type", event.Type, "rig", event.Rig, "role", event.Role,
		"agent", event.AgentName, "bead", event.BeadID)

	switch event.Type {
	case beadswatcher.AgentSpawn:
		return pods.CreateAgentPod(ctx, podmanager.AgentPodSpec{
			Rig:       event.Rig,
			Role:      event.Role,
			AgentName: event.AgentName,
			Image:     event.Metadata["image"],
			Namespace: event.Metadata["namespace"],
			Env: map[string]string{
				"BD_DAEMON_HOST": event.Metadata["daemon_host"],
				"BD_DAEMON_PORT": event.Metadata["daemon_port"],
			},
		})

	case beadswatcher.AgentDone, beadswatcher.AgentKill:
		podName := fmt.Sprintf("gt-%s-%s-%s", event.Rig, event.Role, event.AgentName)
		ns := event.Metadata["namespace"]
		if ns == "" {
			ns = "gastown"
		}
		return pods.DeleteAgentPod(ctx, podName, ns)

	case beadswatcher.AgentStuck:
		// Delete and recreate the pod to restart the agent.
		podName := fmt.Sprintf("gt-%s-%s-%s", event.Rig, event.Role, event.AgentName)
		ns := event.Metadata["namespace"]
		if ns == "" {
			ns = "gastown"
		}
		if err := pods.DeleteAgentPod(ctx, podName, ns); err != nil {
			logger.Warn("failed to delete stuck pod (may not exist)", "pod", podName, "error", err)
		}
		return pods.CreateAgentPod(ctx, podmanager.AgentPodSpec{
			Rig:       event.Rig,
			Role:      event.Role,
			AgentName: event.AgentName,
			Image:     event.Metadata["image"],
			Namespace: ns,
		})

	default:
		logger.Warn("unknown event type", "type", event.Type)
		return nil
	}
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
