// Package reconciler diffs desired agent bead state (from daemon) against
// actual K8s pod state and creates/deletes pods to converge.
package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	corev1 "k8s.io/api/core/v1"

	"github.com/steveyegge/gastown/controller/internal/config"
	"github.com/steveyegge/gastown/controller/internal/daemonclient"
	"github.com/steveyegge/gastown/controller/internal/podmanager"
)

// SpecBuilder constructs an AgentPodSpec from config and bead identity.
type SpecBuilder func(cfg *config.Config, rig, role, agentName string) podmanager.AgentPodSpec

// Reconciler diffs desired state (agent beads) against actual state (K8s pods)
// and creates/deletes pods to converge.
type Reconciler struct {
	lister      daemonclient.BeadLister
	pods        podmanager.Manager
	cfg         *config.Config
	logger      *slog.Logger
	specBuilder SpecBuilder
	mu          sync.Mutex // prevent concurrent reconciles
}

// New creates a Reconciler.
func New(
	lister daemonclient.BeadLister,
	pods podmanager.Manager,
	cfg *config.Config,
	logger *slog.Logger,
	specBuilder SpecBuilder,
) *Reconciler {
	return &Reconciler{
		lister:      lister,
		pods:        pods,
		cfg:         cfg,
		logger:      logger,
		specBuilder: specBuilder,
	}
}

// Reconcile performs a single reconciliation pass:
// 1. List desired beads from daemon
// 2. List actual pods from K8s
// 3. Create missing pods, delete orphan pods, recreate failed pods
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get desired state from daemon.
	beads, err := r.lister.ListAgentBeads(ctx)
	if err != nil {
		// Fail-safe: if we can't reach the daemon, do NOT delete any pods.
		return fmt.Errorf("listing agent beads: %w", err)
	}

	// Build desired pod name set.
	desired := make(map[string]daemonclient.AgentBead)
	for _, b := range beads {
		podName := fmt.Sprintf("gt-%s-%s-%s", b.Rig, b.Role, b.AgentName)
		desired[podName] = b
	}

	// Get actual state from K8s.
	actual, err := r.pods.ListAgentPods(ctx, r.cfg.Namespace, map[string]string{
		podmanager.LabelApp: podmanager.LabelAppValue,
	})
	if err != nil {
		return fmt.Errorf("listing agent pods: %w", err)
	}

	actualMap := make(map[string]corev1.Pod)
	for _, p := range actual {
		actualMap[p.Name] = p
	}

	// Delete orphan pods (exist in K8s but not in desired).
	for name, pod := range actualMap {
		if _, ok := desired[name]; !ok {
			r.logger.Info("deleting orphan pod", "pod", name)
			if err := r.pods.DeleteAgentPod(ctx, name, pod.Namespace); err != nil {
				return fmt.Errorf("deleting orphan pod %s: %w", name, err)
			}
		}
	}

	// Create missing pods and recreate failed pods.
	for name, bead := range desired {
		if pod, exists := actualMap[name]; exists {
			// Pod exists. Check if it's in a terminal failed state.
			if pod.Status.Phase == corev1.PodFailed {
				r.logger.Info("deleting failed pod for recreation", "pod", name)
				if err := r.pods.DeleteAgentPod(ctx, name, pod.Namespace); err != nil {
					return fmt.Errorf("deleting failed pod %s: %w", name, err)
				}
				// Fall through to create.
			} else {
				// Pod is Running or Pending — no action needed.
				continue
			}
		}

		// Create the pod.
		spec := r.specBuilder(r.cfg, bead.Rig, bead.Role, bead.AgentName)
		r.logger.Info("creating pod", "pod", name)
		if err := r.pods.CreateAgentPod(ctx, spec); err != nil {
			return fmt.Errorf("creating pod %s: %w", name, err)
		}
	}

	return nil
}
