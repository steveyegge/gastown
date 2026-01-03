// Package mrqueue provides merge request queue storage.
// MRs are stored locally in .beads/mq/ and deleted after merge.
// This avoids sync overhead for transient MR state.
package mrqueue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MR represents a merge request in the queue.
type MR struct {
	ID          string    `json:"id"`
	Branch      string    `json:"branch"`       // Source branch (e.g., "polecat/nux")
	Target      string    `json:"target"`       // Target branch (e.g., "main")
	SourceIssue string    `json:"source_issue"` // The work item being merged
	Worker      string    `json:"worker"`       // Who did the work
	Rig         string    `json:"rig"`          // Which rig
	Title       string    `json:"title"`        // MR title
	Priority    int       `json:"priority"`     // Priority (lower = higher priority)
	CreatedAt   time.Time `json:"created_at"`
	AgentBead   string    `json:"agent_bead,omitempty"` // Agent bead ID that created this MR (for traceability)

	// Priority scoring fields
	RetryCount      int        `json:"retry_count,omitempty"`       // Conflict retry count for priority penalty
	ConvoyID        string     `json:"convoy_id,omitempty"`         // Parent convoy ID if part of a convoy
	ConvoyCreatedAt *time.Time `json:"convoy_created_at,omitempty"` // Convoy creation time for starvation prevention

	// Claiming fields for parallel refinery workers
	ClaimedBy string     `json:"claimed_by,omitempty"` // Worker ID that claimed this MR
	ClaimedAt *time.Time `json:"claimed_at,omitempty"` // When the MR was claimed

	// Blocking fields for non-blocking delegation
	BlockedBy string `json:"blocked_by,omitempty"` // Task ID that blocks this MR (e.g., conflict resolution task)
}

// Queue manages the MR storage.
type Queue struct {
	dir string // .beads/mq/ directory
}

// New creates a new MR queue for the given rig path.
func New(rigPath string) *Queue {
	return &Queue{
		dir: filepath.Join(rigPath, ".beads", "mq"),
	}
}

// NewFromWorkdir creates a queue by finding the rig root from a working directory.
func NewFromWorkdir(workdir string) (*Queue, error) {
	// Walk up to find .beads or rig root
	dir := workdir
	for {
		beadsDir := filepath.Join(dir, ".beads")
		if info, err := os.Stat(beadsDir); err == nil && info.IsDir() {
			return &Queue{dir: filepath.Join(beadsDir, "mq")}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("could not find .beads directory from %s", workdir)
		}
		dir = parent
	}
}

// EnsureDir creates the MQ directory if it doesn't exist.
func (q *Queue) EnsureDir() error {
	return os.MkdirAll(q.dir, 0755)
}

// generateID creates a unique MR ID.
func generateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("mr-%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}

// Submit adds a new MR to the queue.
func (q *Queue) Submit(mr *MR) error {
	if err := q.EnsureDir(); err != nil {
		return fmt.Errorf("creating mq directory: %w", err)
	}

	if mr.ID == "" {
		mr.ID = generateID()
	}
	if mr.CreatedAt.IsZero() {
		mr.CreatedAt = time.Now()
	}

	data, err := json.MarshalIndent(mr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MR: %w", err)
	}

	path := filepath.Join(q.dir, mr.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing MR file: %w", err)
	}

	return nil
}

// List returns all pending MRs, sorted by priority then creation time.
// Deprecated: Use ListByScore for priority-aware ordering.
func (q *Queue) List() ([]*MR, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty queue
		}
		return nil, fmt.Errorf("reading mq directory: %w", err)
	}

	var mrs []*MR
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		mr, err := q.load(filepath.Join(q.dir, entry.Name()))
		if err != nil {
			continue // Skip malformed files
		}
		mrs = append(mrs, mr)
	}

	// Sort by priority (lower first), then by creation time (older first)
	sort.Slice(mrs, func(i, j int) bool {
		if mrs[i].Priority != mrs[j].Priority {
			return mrs[i].Priority < mrs[j].Priority
		}
		return mrs[i].CreatedAt.Before(mrs[j].CreatedAt)
	})

	return mrs, nil
}

// ListByScore returns all pending MRs sorted by priority score (highest first).
// Uses the ScoreMR function which considers:
//   - Convoy age (prevents starvation)
//   - Issue priority (P0-P4)
//   - Retry count (prevents thrashing)
//   - MR age (FIFO tiebreaker)
func (q *Queue) ListByScore() ([]*MR, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty queue
		}
		return nil, fmt.Errorf("reading mq directory: %w", err)
	}

	now := time.Now()
	var mrs []*MR
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		mr, err := q.load(filepath.Join(q.dir, entry.Name()))
		if err != nil {
			continue // Skip malformed files
		}
		mrs = append(mrs, mr)
	}

	// Sort by score (higher first = higher priority)
	sort.Slice(mrs, func(i, j int) bool {
		return mrs[i].ScoreAt(now) > mrs[j].ScoreAt(now)
	})

	return mrs, nil
}

// Get retrieves a specific MR by ID.
func (q *Queue) Get(id string) (*MR, error) {
	path := filepath.Join(q.dir, id+".json")
	return q.load(path)
}

// load reads an MR from a file path.
func (q *Queue) load(path string) (*MR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var mr MR
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, err
	}

	return &mr, nil
}

// Remove deletes an MR from the queue (after successful merge).
func (q *Queue) Remove(id string) error {
	path := filepath.Join(q.dir, id+".json")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Already removed
	}
	return err
}

// Count returns the number of pending MRs.
func (q *Queue) Count() int {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

// Dir returns the queue directory path.
func (q *Queue) Dir() string {
	return q.dir
}

// ClaimStaleTimeout is how long before a claimed MR is considered stale.
// If a worker claims an MR but doesn't process it within this time,
// another worker can reclaim it.
const ClaimStaleTimeout = 10 * time.Minute

// Claim attempts to claim an MR for processing by a specific worker.
// Returns nil if successful, ErrAlreadyClaimed if another worker has it,
// or ErrNotFound if the MR doesn't exist.
// Uses atomic file operations to prevent race conditions.
func (q *Queue) Claim(id, workerID string) error {
	path := filepath.Join(q.dir, id+".json")

	// Read current state
	mr, err := q.load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("loading MR: %w", err)
	}

	// Check if already claimed by another worker
	if mr.ClaimedBy != "" && mr.ClaimedBy != workerID {
		// Check if claim is stale (worker may have crashed)
		if mr.ClaimedAt != nil && time.Since(*mr.ClaimedAt) < ClaimStaleTimeout {
			return ErrAlreadyClaimed
		}
		// Stale claim - allow reclaim
	}

	// Claim the MR
	now := time.Now()
	mr.ClaimedBy = workerID
	mr.ClaimedAt = &now

	// Write atomically
	data, err := json.MarshalIndent(mr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MR: %w", err)
	}

	// Write to temp file first, then rename (atomic on most filesystems)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath) // cleanup
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// Release releases a claimed MR back to the queue.
// Called when processing fails and the MR should be retried.
func (q *Queue) Release(id string) error {
	path := filepath.Join(q.dir, id+".json")

	mr, err := q.load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("loading MR: %w", err)
	}

	// Clear claim
	mr.ClaimedBy = ""
	mr.ClaimedAt = nil

	data, err := json.MarshalIndent(mr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MR: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// ListUnclaimed returns MRs that are not claimed or have stale claims.
// Sorted by priority then creation time.
func (q *Queue) ListUnclaimed() ([]*MR, error) {
	all, err := q.List()
	if err != nil {
		return nil, err
	}

	var unclaimed []*MR
	for _, mr := range all {
		if mr.ClaimedBy == "" {
			unclaimed = append(unclaimed, mr)
			continue
		}
		// Check if claim is stale
		if mr.ClaimedAt != nil && time.Since(*mr.ClaimedAt) >= ClaimStaleTimeout {
			unclaimed = append(unclaimed, mr)
		}
	}

	return unclaimed, nil
}

// ListClaimedBy returns MRs claimed by a specific worker.
func (q *Queue) ListClaimedBy(workerID string) ([]*MR, error) {
	all, err := q.List()
	if err != nil {
		return nil, err
	}

	var claimed []*MR
	for _, mr := range all {
		if mr.ClaimedBy == workerID {
			claimed = append(claimed, mr)
		}
	}

	return claimed, nil
}

// Common errors for claiming
var (
	ErrNotFound       = fmt.Errorf("merge request not found")
	ErrAlreadyClaimed = fmt.Errorf("merge request already claimed by another worker")
)

// SetBlockedBy marks an MR as blocked by a task (e.g., conflict resolution).
// When the blocking task closes, the MR becomes ready for processing again.
func (q *Queue) SetBlockedBy(mrID, taskID string) error {
	path := filepath.Join(q.dir, mrID+".json")

	mr, err := q.load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("loading MR: %w", err)
	}

	mr.BlockedBy = taskID

	data, err := json.MarshalIndent(mr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MR: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// ClearBlockedBy removes the blocking task from an MR.
func (q *Queue) ClearBlockedBy(mrID string) error {
	return q.SetBlockedBy(mrID, "")
}

// IsBlocked checks if an MR is blocked by a task that is still open.
// If blocked, returns true and the blocking task ID.
// checkStatus is a function that checks if a bead is still open.
func (mr *MR) IsBlocked(checkStatus func(beadID string) (isOpen bool, err error)) (bool, string, error) {
	if mr.BlockedBy == "" {
		return false, "", nil
	}

	isOpen, err := checkStatus(mr.BlockedBy)
	if err != nil {
		// If we can't check status, assume not blocked (fail open)
		return false, "", nil
	}

	return isOpen, mr.BlockedBy, nil
}

// BeadStatusChecker is a function type that checks if a bead is open.
// Returns true if the bead is open (not closed), false if closed or not found.
type BeadStatusChecker func(beadID string) (isOpen bool, err error)

// ListReady returns MRs that are ready for processing:
// - Not claimed by another worker (or claim is stale)
// - Not blocked by an open task
// Sorted by priority score (highest first).
// The checkStatus function is used to check if blocking tasks are still open.
func (q *Queue) ListReady(checkStatus BeadStatusChecker) ([]*MR, error) {
	all, err := q.ListByScore()
	if err != nil {
		return nil, err
	}

	var ready []*MR
	for _, mr := range all {
		// Skip if claimed by another worker (and not stale)
		if mr.ClaimedBy != "" {
			if mr.ClaimedAt != nil && time.Since(*mr.ClaimedAt) < ClaimStaleTimeout {
				continue
			}
			// Stale claim - include in ready list
		}

		// Skip if blocked by an open task
		if mr.BlockedBy != "" && checkStatus != nil {
			isOpen, err := checkStatus(mr.BlockedBy)
			if err == nil && isOpen {
				// Blocked by an open task - skip
				continue
			}
			// If error or task closed, proceed (fail open)
		}

		ready = append(ready, mr)
	}

	return ready, nil
}

// ListBlocked returns MRs that are blocked by open tasks.
// Useful for reporting/monitoring.
func (q *Queue) ListBlocked(checkStatus BeadStatusChecker) ([]*MR, error) {
	all, err := q.List()
	if err != nil {
		return nil, err
	}

	var blocked []*MR
	for _, mr := range all {
		if mr.BlockedBy == "" {
			continue
		}
		if checkStatus != nil {
			isOpen, err := checkStatus(mr.BlockedBy)
			if err == nil && isOpen {
				blocked = append(blocked, mr)
			}
		}
	}

	return blocked, nil
}
