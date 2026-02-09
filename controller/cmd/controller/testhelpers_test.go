package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/steveyegge/gastown/controller/internal/beadswatcher"
	"github.com/steveyegge/gastown/controller/internal/config"
	"github.com/steveyegge/gastown/controller/internal/statusreporter"
)

// channelWatcher implements beadswatcher.Watcher by reading events from a channel.
// Tests inject events by sending to the Send channel.
type channelWatcher struct {
	ch   chan beadswatcher.Event
	done chan struct{}
}

func newChannelWatcher(bufSize int) *channelWatcher {
	return &channelWatcher{
		ch:   make(chan beadswatcher.Event, bufSize),
		done: make(chan struct{}),
	}
}

func (w *channelWatcher) Start(ctx context.Context) error {
	<-ctx.Done()
	close(w.ch)
	close(w.done)
	return fmt.Errorf("watcher stopped: %w", ctx.Err())
}

func (w *channelWatcher) Events() <-chan beadswatcher.Event {
	return w.ch
}

// Send injects an event into the watcher's channel.
func (w *channelWatcher) Send(e beadswatcher.Event) {
	w.ch <- e
}

// statusRecord captures a single ReportPodStatus call.
type statusRecord struct {
	AgentName string
	Status    statusreporter.PodStatus
}

// backendRecord captures a single ReportBackendMetadata call.
type backendRecord struct {
	AgentName string
	Meta      statusreporter.BackendMetadata
}

// recordingReporter implements statusreporter.Reporter and records all calls.
type recordingReporter struct {
	mu          sync.Mutex
	reports     []statusRecord
	backendMeta []backendRecord
	syncRuns    int
	client      kubernetes.Interface
	ns          string
	logger      *slog.Logger
}

func newRecordingReporter(client kubernetes.Interface, ns string, logger *slog.Logger) *recordingReporter {
	return &recordingReporter{
		client: client,
		ns:     ns,
		logger: logger,
	}
}

func (r *recordingReporter) ReportPodStatus(_ context.Context, agentName string, status statusreporter.PodStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, statusRecord{AgentName: agentName, Status: status})
	return nil
}

func (r *recordingReporter) ReportBackendMetadata(_ context.Context, agentName string, meta statusreporter.BackendMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backendMeta = append(r.backendMeta, backendRecord{AgentName: agentName, Meta: meta})
	return nil
}

func (r *recordingReporter) SyncAll(ctx context.Context) error {
	r.mu.Lock()
	r.syncRuns++
	r.mu.Unlock()

	// List all gastown pods and report their statuses, like BdReporter does.
	pods, err := r.client.CoreV1().Pods(r.ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gastown",
	})
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}
	for _, pod := range pods.Items {
		agent := pod.Labels["gastown.io/agent"]
		rig := pod.Labels["gastown.io/rig"]
		role := pod.Labels["gastown.io/role"]
		if agent == "" || rig == "" || role == "" {
			continue
		}
		agentBeadID := fmt.Sprintf("gt-%s-%s-%s", rig, role, agent)
		status := statusreporter.PodStatus{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Phase:     string(pod.Status.Phase),
			Message:   pod.Status.Message,
		}
		_ = r.ReportPodStatus(ctx, agentBeadID, status)
	}
	return nil
}

func (r *recordingReporter) Metrics() statusreporter.MetricsSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return statusreporter.MetricsSnapshot{
		StatusReportsTotal: int64(len(r.reports)),
		SyncAllRuns:        int64(r.syncRuns),
	}
}

// Reports returns a copy of all recorded status reports.
func (r *recordingReporter) Reports() []statusRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]statusRecord, len(r.reports))
	copy(out, r.reports)
	return out
}

// BackendMeta returns a copy of all recorded backend metadata reports.
func (r *recordingReporter) BackendMeta() []backendRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]backendRecord, len(r.backendMeta))
	copy(out, r.backendMeta)
	return out
}

// SyncRunCount returns how many times SyncAll has been called.
func (r *recordingReporter) SyncRunCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.syncRuns
}

// testConfig creates a controller config for integration tests.
func testConfig() *config.Config {
	return &config.Config{
		DaemonHost:   "localhost",
		DaemonPort:   9876,
		Namespace:    "gastown",
		LogLevel:     "debug",
		TownRoot:     "/tmp/test-town",
		BdBinary:     "bd",
		DefaultImage: "gastown-agent:test",
		SyncInterval: 100 * time.Millisecond,
	}
}

// spawnEvent creates an AgentSpawn event for testing.
func spawnEvent(rig, role, agent string) beadswatcher.Event {
	return beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       rig,
		Role:      role,
		AgentName: agent,
		BeadID:    fmt.Sprintf("gt-%s-%s-%s-bead", rig, role, agent),
		Metadata: map[string]string{
			"namespace": "gastown",
			"image":     "gastown-agent:test",
		},
	}
}

// doneEvent creates an AgentDone event for testing.
func doneEvent(rig, role, agent string) beadswatcher.Event {
	return beadswatcher.Event{
		Type:      beadswatcher.AgentDone,
		Rig:       rig,
		Role:      role,
		AgentName: agent,
		BeadID:    fmt.Sprintf("gt-%s-%s-%s-bead", rig, role, agent),
		Metadata: map[string]string{
			"namespace": "gastown",
		},
	}
}

// killEvent creates an AgentKill event for testing.
func killEvent(rig, role, agent string) beadswatcher.Event {
	return beadswatcher.Event{
		Type:      beadswatcher.AgentKill,
		Rig:       rig,
		Role:      role,
		AgentName: agent,
		BeadID:    fmt.Sprintf("gt-%s-%s-%s-bead", rig, role, agent),
		Metadata: map[string]string{
			"namespace": "gastown",
		},
	}
}

// stuckEvent creates an AgentStuck event for testing.
func stuckEvent(rig, role, agent string) beadswatcher.Event {
	return beadswatcher.Event{
		Type:      beadswatcher.AgentStuck,
		Rig:       rig,
		Role:      role,
		AgentName: agent,
		BeadID:    fmt.Sprintf("gt-%s-%s-%s-bead", rig, role, agent),
		Metadata: map[string]string{
			"namespace": "gastown",
			"image":     "gastown-agent:test",
		},
	}
}

// waitForPod polls until a pod with the given name exists or timeout.
func waitForPod(ctx context.Context, client kubernetes.Interface, name, ns string, timeout time.Duration) (*corev1.Pod, error) {
	deadline := time.After(timeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for pod %s/%s", ns, name)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
			pod, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
			if err == nil {
				return pod, nil
			}
		}
	}
}

// waitForNoPod polls until a pod with the given name no longer exists or timeout.
func waitForNoPod(ctx context.Context, client kubernetes.Interface, name, ns string, timeout time.Duration) error {
	deadline := time.After(timeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for pod %s/%s to be deleted", ns, name)
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			_, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil // Pod is gone
			}
		}
	}
}

// waitForReports polls until the reporter has at least n reports or timeout.
func waitForReports(r *recordingReporter, n int, timeout time.Duration) error {
	deadline := time.After(timeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for %d reports, got %d", n, len(r.Reports()))
		case <-tick.C:
			if len(r.Reports()) >= n {
				return nil
			}
		}
	}
}
