package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// PodInfo contains information about an agent's K8s pod.
type PodInfo struct {
	AgentID       string    `json:"agent_id"`
	PodName       string    `json:"pod_name"`
	PodIP         string    `json:"pod_ip"`
	PodNode       string    `json:"pod_node"`
	PodStatus     string    `json:"pod_status"`
	ScreenSession string    `json:"screen_session"`
	Connected     bool      `json:"-"`
	LastSeen      time.Time `json:"-"`
}

// PodEventType represents the type of pod lifecycle event.
type PodEventType int

const (
	// PodAdded indicates a new pod was discovered.
	PodAdded PodEventType = iota
	// PodRemoved indicates a pod was deregistered.
	PodRemoved
	// PodUpdated indicates a pod's metadata changed (e.g., new pod name after restart).
	PodUpdated
)

// PodEvent represents a change in pod inventory.
type PodEvent struct {
	Type PodEventType
	Pod  *PodInfo
}

// PodSource provides pod information from the beads database.
// Implementations include CLIPodSource (shells out to bd) and can be mocked for tests.
type PodSource interface {
	ListPods(ctx context.Context) ([]*PodInfo, error)
}

// podListResponse is the JSON output format from bd agent pod-list --json.
type podListResponse struct {
	Agents []podListEntry `json:"agents"`
}

// podListEntry represents one agent in the pod-list response.
type podListEntry struct {
	AgentID       string `json:"agent_id"`
	PodName       string `json:"pod_name"`
	PodIP         string `json:"pod_ip"`
	PodNode       string `json:"pod_node"`
	PodStatus     string `json:"pod_status"`
	ScreenSession string `json:"screen_session"`
}

// CLIPodSource queries bd agent pod-list for pod information.
type CLIPodSource struct {
	Rig string // Optional rig filter
}

// ListPods queries bd agent pod-list --json and returns all pods.
func (s *CLIPodSource) ListPods(ctx context.Context) ([]*PodInfo, error) {
	args := []string{"agent", "pod-list", "--json"}
	if s.Rig != "" {
		args = append(args, "--rig="+s.Rig)
	}
	cmd := exec.CommandContext(ctx, "bd", args...) //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd agent pod-list: %w", err)
	}

	var resp podListResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("parsing pod-list output: %w", err)
	}

	pods := make([]*PodInfo, 0, len(resp.Agents))
	for _, entry := range resp.Agents {
		pods = append(pods, &PodInfo{
			AgentID:       entry.AgentID,
			PodName:       entry.PodName,
			PodIP:         entry.PodIP,
			PodNode:       entry.PodNode,
			PodStatus:     entry.PodStatus,
			ScreenSession: entry.ScreenSession,
			LastSeen:      time.Now(),
		})
	}
	return pods, nil
}

// DefaultPollInterval is the default interval for polling beads for pod changes.
const DefaultPollInterval = 5 * time.Second

// PodInventoryConfig configures a PodInventory.
type PodInventoryConfig struct {
	Source       PodSource
	PollInterval time.Duration  // Default 5s
	OnChange     func(PodEvent) // Optional callback for pod lifecycle events
}

// PodInventory maintains an in-memory inventory of agent pods.
// Thread-safe for concurrent access.
type PodInventory struct {
	source       PodSource
	pods         map[string]*PodInfo // agent_id → pod info
	mu           sync.RWMutex
	pollInterval time.Duration
	onChange     func(PodEvent)
}

// NewPodInventory creates a new PodInventory with the given config.
func NewPodInventory(cfg PodInventoryConfig) *PodInventory {
	interval := cfg.PollInterval
	if interval == 0 {
		interval = DefaultPollInterval
	}
	return &PodInventory{
		source:       cfg.Source,
		pods:         make(map[string]*PodInfo),
		pollInterval: interval,
		onChange:      cfg.OnChange,
	}
}

// Refresh fetches the current pod list from the source and updates the inventory.
// Detects added, removed, and updated pods and emits events for each.
func (pi *PodInventory) Refresh(ctx context.Context) error {
	pods, err := pi.source.ListPods(ctx)
	if err != nil {
		return err
	}

	// Build new pod map, filtering out stale entries
	newPods := make(map[string]*PodInfo, len(pods))
	for _, p := range pods {
		if p.PodStatus == "failed" || p.PodStatus == "terminated" {
			continue
		}
		newPods[p.AgentID] = p
	}

	// Diff under lock, collect events
	var events []PodEvent

	pi.mu.Lock()

	// Detect removed pods (in old but not in new)
	for agentID, oldPod := range pi.pods {
		if _, exists := newPods[agentID]; !exists {
			copied := *oldPod
			events = append(events, PodEvent{Type: PodRemoved, Pod: &copied})
		}
	}

	// Detect added and updated pods
	for agentID, newPod := range newPods {
		oldPod, existed := pi.pods[agentID]
		if !existed {
			events = append(events, PodEvent{Type: PodAdded, Pod: newPod})
		} else if oldPod.PodName != newPod.PodName || oldPod.PodIP != newPod.PodIP {
			events = append(events, PodEvent{Type: PodUpdated, Pod: newPod})
		}
	}

	pi.pods = newPods
	pi.mu.Unlock()

	// Emit events outside lock to prevent deadlock if callback accesses inventory
	for _, event := range events {
		if pi.onChange != nil {
			pi.onChange(event)
		}
	}

	return nil
}

// GetPod returns the PodInfo for the given agent ID, or nil if not found.
// Returns a copy to prevent data races.
func (pi *PodInventory) GetPod(agentID string) *PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	pod, exists := pi.pods[agentID]
	if !exists {
		return nil
	}
	copied := *pod
	return &copied
}

// ListPods returns all pods in the inventory.
// Returns copies to prevent data races.
func (pi *PodInventory) ListPods() []*PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	result := make([]*PodInfo, 0, len(pi.pods))
	for _, pod := range pi.pods {
		copied := *pod
		result = append(result, &copied)
	}
	return result
}

// Count returns the number of pods in the inventory.
func (pi *PodInventory) Count() int {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return len(pi.pods)
}

// Watch starts polling for pod changes at the configured interval.
// Performs an initial refresh, then polls on each tick.
// Blocks until the context is cancelled. Returns ctx.Err() on cancellation.
// Individual poll failures are silently ignored (will retry on next tick).
func (pi *PodInventory) Watch(ctx context.Context) error {
	// Initial refresh - best effort, will retry on next tick
	_ = pi.Refresh(ctx)

	ticker := time.NewTicker(pi.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = pi.Refresh(ctx)
		}
	}
}
