// Package statusreporter syncs K8s pod status back to beads via bd CLI.
// This gives beads visibility into pod health, allowing existing Gas Town
// health monitoring (Witness, Deacon) to incorporate K8s state.
package statusreporter

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodStatus represents the K8s pod state to report back to beads.
type PodStatus struct {
	PodName   string
	Namespace string
	Phase     string // Pending, Running, Succeeded, Failed, Unknown
	Ready     bool
	Message   string
}

// BackendMetadata holds connection info written to agent bead notes
// so that ResolveBackend() can discover how to connect to the agent.
type BackendMetadata struct {
	PodName   string // K8s pod name
	Namespace string // K8s namespace
	Backend   string // "coop" or "k8s"
	CoopURL   string // e.g., "http://gt-gastown-polecat-furiosa.gastown.svc.cluster.local:8080"
	CoopToken string // auth token (optional)
}

// Reporter syncs pod status back to beads.
type Reporter interface {
	// ReportPodStatus sends a single pod's status to beads.
	// agentName is the agent bead ID (e.g., "gt-gastown-polecat-furiosa").
	ReportPodStatus(ctx context.Context, agentName string, status PodStatus) error

	// ReportBackendMetadata writes backend connection info to the agent bead's notes.
	ReportBackendMetadata(ctx context.Context, agentName string, meta BackendMetadata) error

	// SyncAll reconciles all agent pod statuses with beads.
	SyncAll(ctx context.Context) error

	// Metrics returns the current metrics snapshot.
	Metrics() MetricsSnapshot
}

// MetricsSnapshot holds current metric values for logging/monitoring.
type MetricsSnapshot struct {
	StatusReportsTotal int64
	StatusReportErrors int64
	SyncAllRuns        int64
	SyncAllErrors      int64
	AgentsByState      map[string]int64 // state -> count
}

// BdConfig configures the bd CLI reporter.
type BdConfig struct {
	BdBinary  string // path to bd executable
	TownRoot  string // working directory for bd commands
	Namespace string // K8s namespace for pod listing
}

// BdReporter reports pod status to beads via bd CLI commands.
// It uses "bd agent state" to update agent_state in beads,
// mapping K8s pod phases to beads agent states.
type BdReporter struct {
	cfg    BdConfig
	client kubernetes.Interface
	logger *slog.Logger

	// Metrics counters.
	reportsTotal atomic.Int64
	reportErrors atomic.Int64
	syncRuns     atomic.Int64
	syncErrors   atomic.Int64
}

// NewBdReporter creates a reporter that syncs pod status to beads via bd CLI.
func NewBdReporter(cfg BdConfig, client kubernetes.Interface, logger *slog.Logger) *BdReporter {
	if cfg.BdBinary == "" {
		cfg.BdBinary = "bd"
	}
	return &BdReporter{
		cfg:    cfg,
		client: client,
		logger: logger,
	}
}

// ReportPodStatus updates the agent's state in beads based on pod phase.
// agentName should be the agent bead ID (e.g., "gt-gastown-polecat-furiosa").
func (r *BdReporter) ReportPodStatus(ctx context.Context, agentName string, status PodStatus) error {
	r.reportsTotal.Add(1)

	state := PhaseToAgentState(status.Phase)
	if state == "" {
		r.logger.Debug("skipping status report for unknown phase",
			"agent", agentName, "phase", status.Phase)
		return nil
	}

	r.logger.Info("reporting pod status to beads",
		"agent", agentName, "pod", status.PodName,
		"phase", status.Phase, "state", state, "ready", status.Ready)

	if err := r.runBdAgentState(ctx, agentName, state); err != nil {
		r.reportErrors.Add(1)
		// Log but don't fail - status reporting is best-effort.
		r.logger.Warn("failed to report pod status to beads",
			"agent", agentName, "state", state, "error", err)
		return fmt.Errorf("reporting status for %s: %w", agentName, err)
	}

	// On pod failure, create an escalation bead for the witness.
	if status.Phase == string(corev1.PodFailed) {
		if err := r.createEscalation(ctx, agentName, status); err != nil {
			r.logger.Warn("failed to create escalation bead",
				"agent", agentName, "error", err)
		}
	}

	return nil
}

// ReportBackendMetadata writes backend connection info to the agent bead's
// notes field. This structured data is parsed by ResolveBackend() to determine
// how to connect to the agent (Coop, SSH, or local tmux).
func (r *BdReporter) ReportBackendMetadata(ctx context.Context, agentName string, meta BackendMetadata) error {
	r.reportsTotal.Add(1)

	// Build structured notes content with key: value lines.
	var lines []string
	if meta.Backend != "" {
		lines = append(lines, fmt.Sprintf("backend: %s", meta.Backend))
	}
	if meta.PodName != "" {
		lines = append(lines, fmt.Sprintf("pod_name: %s", meta.PodName))
	}
	if meta.Namespace != "" {
		lines = append(lines, fmt.Sprintf("pod_namespace: %s", meta.Namespace))
	}
	if meta.CoopURL != "" {
		lines = append(lines, fmt.Sprintf("coop_url: %s", meta.CoopURL))
	}
	if meta.CoopToken != "" {
		lines = append(lines, fmt.Sprintf("coop_token: %s", meta.CoopToken))
	}

	if len(lines) == 0 {
		return nil
	}

	notes := strings.Join(lines, "\n")
	args := []string{"update", agentName, "--notes", notes}

	r.logger.Info("reporting backend metadata to beads",
		"agent", agentName, "backend", meta.Backend, "coop_url", meta.CoopURL)

	if err := r.runBd(ctx, args...); err != nil {
		r.reportErrors.Add(1)
		r.logger.Warn("failed to report backend metadata",
			"agent", agentName, "error", err)
		return fmt.Errorf("reporting backend metadata for %s: %w", agentName, err)
	}
	return nil
}

// SyncAll lists all gastown pods and reports their statuses to beads.
func (r *BdReporter) SyncAll(ctx context.Context) error {
	r.syncRuns.Add(1)

	pods, err := r.client.CoreV1().Pods(r.cfg.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gastown",
	})
	if err != nil {
		r.syncErrors.Add(1)
		return fmt.Errorf("listing agent pods: %w", err)
	}

	var errs []string
	for _, pod := range pods.Items {
		agentLabel := pod.Labels["gastown.io/agent"]
		rigLabel := pod.Labels["gastown.io/rig"]
		roleLabel := pod.Labels["gastown.io/role"]
		if agentLabel == "" || rigLabel == "" || roleLabel == "" {
			continue
		}

		agentBeadID := fmt.Sprintf("gt-%s-%s-%s", rigLabel, roleLabel, agentLabel)
		status := PodStatus{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Phase:     string(pod.Status.Phase),
			Ready:     isPodReady(&pod),
			Message:   pod.Status.Message,
		}

		if err := r.ReportPodStatus(ctx, agentBeadID, status); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		r.syncErrors.Add(1)
		return fmt.Errorf("sync errors: %s", strings.Join(errs, "; "))
	}

	r.logger.Info("sync completed", "pods", len(pods.Items))
	return nil
}

// Metrics returns a snapshot of current metric values.
func (r *BdReporter) Metrics() MetricsSnapshot {
	return MetricsSnapshot{
		StatusReportsTotal: r.reportsTotal.Load(),
		StatusReportErrors: r.reportErrors.Load(),
		SyncAllRuns:        r.syncRuns.Load(),
		SyncAllErrors:      r.syncErrors.Load(),
	}
}

// runBdAgentState executes "bd agent state <agentID> <state>".
func (r *BdReporter) runBdAgentState(ctx context.Context, agentID, state string) error {
	args := []string{"agent", "state", agentID, state}
	return r.runBd(ctx, args...)
}

// createEscalation creates an escalation bead when a pod fails.
func (r *BdReporter) createEscalation(ctx context.Context, agentName string, status PodStatus) error {
	title := fmt.Sprintf("Pod failed: %s", agentName)
	desc := fmt.Sprintf("Pod %s in namespace %s entered Failed state.\nMessage: %s",
		status.PodName, status.Namespace, status.Message)
	args := []string{"create", "-t", "bug", title, "-d", desc}
	return r.runBd(ctx, args...)
}

// runBd executes a bd CLI command.
func (r *BdReporter) runBd(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, r.cfg.BdBinary, args...)
	if r.cfg.TownRoot != "" {
		cmd.Dir = r.cfg.TownRoot
	}
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s: %w (output: %s)", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// PhaseToAgentState maps a K8s pod phase to a beads agent_state.
func PhaseToAgentState(phase string) string {
	switch corev1.PodPhase(phase) {
	case corev1.PodPending:
		return "spawning"
	case corev1.PodRunning:
		return "working"
	case corev1.PodSucceeded:
		return "done"
	case corev1.PodFailed:
		return "failed"
	default:
		return ""
	}
}

// isPodReady checks if a pod has the Ready condition set to True.
func isPodReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// StubReporter is a no-op implementation for testing.
type StubReporter struct {
	logger *slog.Logger
}

// NewStubReporter creates a reporter that logs but takes no action.
func NewStubReporter(logger *slog.Logger) *StubReporter {
	return &StubReporter{logger: logger}
}

// ReportPodStatus logs the status but does not send to beads.
func (r *StubReporter) ReportPodStatus(_ context.Context, agentName string, status PodStatus) error {
	r.logger.Debug("stub: would report pod status",
		"agent", agentName, "pod", status.PodName, "phase", status.Phase, "ready", status.Ready)
	return nil
}

// ReportBackendMetadata logs but does not send to beads.
func (r *StubReporter) ReportBackendMetadata(_ context.Context, agentName string, meta BackendMetadata) error {
	r.logger.Debug("stub: would report backend metadata",
		"agent", agentName, "backend", meta.Backend, "coop_url", meta.CoopURL)
	return nil
}

// SyncAll is a no-op in the stub.
func (r *StubReporter) SyncAll(_ context.Context) error {
	r.logger.Debug("stub: would sync all pod statuses to beads")
	return nil
}

// Metrics returns empty metrics for the stub.
func (r *StubReporter) Metrics() MetricsSnapshot {
	return MetricsSnapshot{}
}
