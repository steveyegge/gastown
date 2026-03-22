package nudge

import (
	"crypto/md5" //nolint:gosec // MD5 is fine for content dedup (not security)
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
)

// Dedup cooldown durations by nudge type.
const (
	// CooldownWork is the dedup window for WORK directives.
	CooldownWork = 30 * time.Minute
	// CooldownAlert is the dedup window for alert escalations.
	CooldownAlert = 10 * time.Minute
	// CooldownWake is the dedup window for wake/limit-reset nudges.
	CooldownWake = 10 * time.Minute
)

// DedupResult describes the outcome of a dedup check.
type DedupResult int

const (
	// DedupDeliver means deliver the full message (no duplicate found).
	DedupDeliver DedupResult = iota
	// DedupShortRef means deliver a short reference (same session, same hash).
	DedupShortRef
	// DedupSuppress means skip delivery entirely (active agent, non-urgent).
	DedupSuppress
)

// DedupState tracks the last nudge delivered to a target.
type DedupState struct {
	Hash      string    `json:"hash"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Message   string    `json:"message_preview"` // First 80 chars for debugging
}

// ShortReference is the abbreviated message sent when a duplicate is detected
// in the same session.
const ShortReference = `<system-reminder>
[Reminder: your previous directive is still active — see earlier in session. Continue working.]
</system-reminder>`

// dedupStateDir returns the directory for nudge dedup state files.
// Path: <townRoot>/.runtime/nudge_dedup/
func dedupStateDir(townRoot string) string {
	return filepath.Join(townRoot, constants.DirRuntime, "nudge_dedup")
}

// stateFilePath returns the path to a target's dedup state file.
func stateFilePath(townRoot, target string) string {
	safe := strings.ReplaceAll(target, "/", "_")
	return filepath.Join(dedupStateDir(townRoot), safe+".json")
}

// HashMessage computes an MD5 hash of the message content for dedup comparison.
func HashMessage(message string) string {
	h := md5.Sum([]byte(message)) //nolint:gosec // dedup, not crypto
	return hex.EncodeToString(h[:])
}

// preview returns the first n characters of s for state file debugging.
func preview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// classifyCooldown determines the cooldown duration based on message content.
// This uses simple heuristics — callers can override with explicit cooldowns later.
func classifyCooldown(message string) time.Duration {
	lower := strings.ToLower(message)

	// Alert-related keywords get shorter cooldown
	if strings.Contains(lower, "alert") ||
		strings.Contains(lower, "firing") ||
		strings.Contains(lower, "critical") ||
		strings.Contains(lower, "pagerduty") {
		return CooldownAlert
	}

	// Wake/session-start keywords
	if strings.Contains(lower, "wake") ||
		strings.Contains(lower, "session-started") ||
		strings.Contains(lower, "limit reset") {
		return CooldownWake
	}

	// Default: WORK directive cooldown
	return CooldownWork
}

// CheckDedup determines whether a nudge should be delivered, shortened, or suppressed.
// townRoot is the Gas Town root directory.
// target is the session name (e.g., "gt-aegis-crew-malcolm").
// message is the full nudge content.
// currentSessionID is the target's current tmux session ID (empty if unknown).
//
// Returns the dedup result and nil error on success.
func CheckDedup(townRoot, target, message, currentSessionID string) (DedupResult, error) {
	hash := HashMessage(message)

	state, err := loadState(townRoot, target)
	if err != nil || state == nil {
		// No prior state — deliver full message
		return DedupDeliver, nil
	}

	// Different content — deliver full
	if state.Hash != hash {
		return DedupDeliver, nil
	}

	// Same hash — check cooldown
	cooldown := classifyCooldown(message)
	if time.Since(state.Timestamp) > cooldown {
		// Cooldown expired — deliver full
		return DedupDeliver, nil
	}

	// Same hash, within cooldown. Check session.
	if currentSessionID != "" && state.SessionID == currentSessionID {
		// Same session — send short reference
		return DedupShortRef, nil
	}

	// Different session (or unknown) — deliver full (new session needs full context)
	return DedupDeliver, nil
}

// RecordDelivery saves the dedup state after a successful delivery.
func RecordDelivery(townRoot, target, message, sessionID string) error {
	dir := dedupStateDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating dedup state dir: %w", err)
	}

	state := DedupState{
		Hash:      HashMessage(message),
		Timestamp: time.Now(),
		SessionID: sessionID,
		Message:   preview(message, 80),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling dedup state: %w", err)
	}

	path := stateFilePath(townRoot, target)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing dedup state: %w", err)
	}

	return nil
}

// loadState reads the dedup state for a target, returning nil if no state exists.
func loadState(townRoot, target string) (*DedupState, error) {
	path := stateFilePath(townRoot, target)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading dedup state: %w", err)
	}

	var state DedupState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing dedup state: %w", err)
	}

	return &state, nil
}
