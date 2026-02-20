package deacon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Default parameters for feed-stranded rate limiting.
const (
	// DefaultMaxFeedsPerCycle is the maximum number of convoys to feed in one invocation.
	// Prevents spawning too many dogs at once.
	DefaultMaxFeedsPerCycle = 3

	// DefaultFeedCooldown is the minimum time between feeding the same convoy.
	// Prevents re-dispatching a dog before the previous one finishes.
	DefaultFeedCooldown = 10 * time.Minute
)

// FeedStrandedState tracks feeding attempts per convoy.
// Persisted to deacon/feed-stranded-state.json.
type FeedStrandedState struct {
	// Convoys maps convoy ID to their feed tracking state.
	Convoys map[string]*ConvoyFeedState `json:"convoys"`

	// LastUpdated is when this state was last written.
	LastUpdated time.Time `json:"last_updated"`
}

// ConvoyFeedState tracks the feed history for a single convoy.
type ConvoyFeedState struct {
	// ConvoyID is the convoy identifier.
	ConvoyID string `json:"convoy_id"`

	// FeedCount is total number of feed dispatches for this convoy.
	FeedCount int `json:"feed_count"`

	// LastFeedTime is when the last feed was dispatched.
	LastFeedTime time.Time `json:"last_feed_time,omitempty"`
}

// StrandedConvoy holds info about a stranded convoy from `gt convoy stranded --json`.
type StrandedConvoy struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ReadyCount  int      `json:"ready_count"`
	ReadyIssues []string `json:"ready_issues"`
}

// FeedResult describes the outcome of a feed-stranded invocation.
type FeedResult struct {
	// Fed is the number of convoys dispatched to dogs for feeding.
	Fed int `json:"fed"`

	// Closed is the number of empty convoys auto-closed.
	Closed int `json:"closed"`

	// Skipped is the number of convoys skipped (cooldown).
	Skipped int `json:"skipped"`

	// Errors is the number of convoys that failed to process.
	Errors int `json:"errors"`

	// Details has per-convoy results.
	Details []FeedConvoyResult `json:"details"`
}

// FeedConvoyResult describes the outcome for a single convoy.
type FeedConvoyResult struct {
	ConvoyID string `json:"convoy_id"`
	Action   string `json:"action"` // "fed", "closed", "cooldown", "error", "limit"
	Message  string `json:"message"`
}

// FeedStrandedStateFile returns the path to the feed-stranded state file.
func FeedStrandedStateFile(townRoot string) string {
	return filepath.Join(townRoot, "deacon", "feed-stranded-state.json")
}

// LoadFeedStrandedState loads the feed-stranded state from disk.
// Returns empty state if file doesn't exist.
func LoadFeedStrandedState(townRoot string) (*FeedStrandedState, error) {
	stateFile := FeedStrandedStateFile(townRoot)

	data, err := os.ReadFile(stateFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return &FeedStrandedState{
				Convoys: make(map[string]*ConvoyFeedState),
			}, nil
		}
		return nil, fmt.Errorf("reading feed-stranded state: %w", err)
	}

	var state FeedStrandedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing feed-stranded state: %w", err)
	}

	if state.Convoys == nil {
		state.Convoys = make(map[string]*ConvoyFeedState)
	}

	return &state, nil
}

// SaveFeedStrandedState saves the feed-stranded state to disk.
func SaveFeedStrandedState(townRoot string, state *FeedStrandedState) error {
	stateFile := FeedStrandedStateFile(townRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}

	state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling feed-stranded state: %w", err)
	}

	return os.WriteFile(stateFile, data, 0600)
}

// GetConvoyState returns the feed state for a convoy, creating if needed.
func (s *FeedStrandedState) GetConvoyState(convoyID string) *ConvoyFeedState {
	if s.Convoys == nil {
		s.Convoys = make(map[string]*ConvoyFeedState)
	}

	state, ok := s.Convoys[convoyID]
	if !ok {
		state = &ConvoyFeedState{ConvoyID: convoyID}
		s.Convoys[convoyID] = state
	}
	return state
}

// IsInCooldown returns true if the convoy was recently fed.
func (s *ConvoyFeedState) IsInCooldown(cooldown time.Duration) bool {
	if s.LastFeedTime.IsZero() {
		return false
	}
	return time.Since(s.LastFeedTime) < cooldown
}

// CooldownRemaining returns how long until cooldown expires.
func (s *ConvoyFeedState) CooldownRemaining(cooldown time.Duration) time.Duration {
	if s.LastFeedTime.IsZero() {
		return 0
	}
	remaining := cooldown - time.Since(s.LastFeedTime)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RecordFeed records a feed dispatch for the convoy.
func (s *ConvoyFeedState) RecordFeed() {
	s.FeedCount++
	s.LastFeedTime = time.Now().UTC()
}

// FindStrandedConvoys runs `gt convoy stranded --json` and parses the output.
func FindStrandedConvoys(townRoot string) ([]StrandedConvoy, error) {
	cmd := exec.Command("gt", "convoy", "stranded", "--json")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running gt convoy stranded: %w", err)
	}

	var stranded []StrandedConvoy
	if err := json.Unmarshal(output, &stranded); err != nil {
		return nil, fmt.Errorf("parsing stranded convoys: %w", err)
	}

	return stranded, nil
}

// FeedStranded detects stranded convoys and dispatches dogs to feed them.
// Empty convoys are auto-closed directly. Feedable convoys get a dog dispatched.
// Rate limits by maxPerCycle and per-convoy cooldown.
func FeedStranded(townRoot string, maxPerCycle int, cooldown time.Duration) *FeedResult {
	result := &FeedResult{}

	if maxPerCycle <= 0 {
		maxPerCycle = DefaultMaxFeedsPerCycle
	}
	if cooldown <= 0 {
		cooldown = DefaultFeedCooldown
	}

	// Find stranded convoys
	stranded, err := FindStrandedConvoys(townRoot)
	if err != nil {
		result.Errors++
		result.Details = append(result.Details, FeedConvoyResult{
			Action:  "error",
			Message: fmt.Sprintf("failed to find stranded convoys: %v", err),
		})
		return result
	}

	if len(stranded) == 0 {
		return result
	}

	// Load state for cooldown tracking
	state, err := LoadFeedStrandedState(townRoot)
	if err != nil {
		result.Errors++
		result.Details = append(result.Details, FeedConvoyResult{
			Action:  "error",
			Message: fmt.Sprintf("failed to load feed state: %v", err),
		})
		return result
	}

	fedCount := 0

	for _, convoy := range stranded {
		// Handle empty convoys (auto-close) — no rate limit needed
		if convoy.ReadyCount == 0 {
			if err := closeEmptyConvoy(townRoot, convoy.ID); err != nil {
				result.Errors++
				result.Details = append(result.Details, FeedConvoyResult{
					ConvoyID: convoy.ID,
					Action:   "error",
					Message:  fmt.Sprintf("failed to auto-close empty convoy: %v", err),
				})
			} else {
				result.Closed++
				result.Details = append(result.Details, FeedConvoyResult{
					ConvoyID: convoy.ID,
					Action:   "closed",
					Message:  "auto-closed empty convoy (0 tracked issues)",
				})
			}
			continue
		}

		// Rate limit: check per-cycle cap
		if fedCount >= maxPerCycle {
			result.Details = append(result.Details, FeedConvoyResult{
				ConvoyID: convoy.ID,
				Action:   "limit",
				Message:  fmt.Sprintf("skipped: per-cycle limit reached (%d/%d)", fedCount, maxPerCycle),
			})
			continue
		}

		// Rate limit: check per-convoy cooldown
		convoyState := state.GetConvoyState(convoy.ID)
		if convoyState.IsInCooldown(cooldown) {
			remaining := convoyState.CooldownRemaining(cooldown)
			result.Skipped++
			result.Details = append(result.Details, FeedConvoyResult{
				ConvoyID: convoy.ID,
				Action:   "cooldown",
				Message:  fmt.Sprintf("in cooldown (remaining: %s)", remaining.Round(time.Second)),
			})
			continue
		}

		// Dispatch dog to feed the convoy
		if err := dispatchFeedDog(townRoot, convoy.ID); err != nil {
			result.Errors++
			result.Details = append(result.Details, FeedConvoyResult{
				ConvoyID: convoy.ID,
				Action:   "error",
				Message:  fmt.Sprintf("failed to dispatch feed dog: %v", err),
			})
			continue
		}

		convoyState.RecordFeed()
		fedCount++
		result.Fed++
		result.Details = append(result.Details, FeedConvoyResult{
			ConvoyID: convoy.ID,
			Action:   "fed",
			Message:  fmt.Sprintf("dispatched dog to feed (%d ready issues)", convoy.ReadyCount),
		})
	}

	// Save state
	if err := SaveFeedStrandedState(townRoot, state); err != nil {
		result.Details = append(result.Details, FeedConvoyResult{
			Action:  "error",
			Message: fmt.Sprintf("warning: failed to save feed state: %v", err),
		})
	}

	return result
}

// closeEmptyConvoy runs `gt convoy check <id>` to auto-close an empty convoy.
func closeEmptyConvoy(townRoot, convoyID string) error {
	cmd := exec.Command("gt", "convoy", "check", convoyID)
	cmd.Dir = townRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dispatchFeedDog dispatches a dog to feed a stranded convoy via gt sling.
func dispatchFeedDog(townRoot, convoyID string) error {
	cmd := exec.Command("gt", "sling", "mol-convoy-feed", "deacon/dogs",
		"--var", fmt.Sprintf("convoy=%s", convoyID))
	cmd.Dir = townRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PruneFeedStrandedState removes entries for convoys that are no longer open.
// Call periodically to prevent unbounded state growth.
func PruneFeedStrandedState(townRoot string) (int, error) {
	state, err := LoadFeedStrandedState(townRoot)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for convoyID := range state.Convoys {
		status := getConvoyStatus(townRoot, convoyID)
		if status == "closed" || status == "" {
			delete(state.Convoys, convoyID)
			pruned++
		}
	}

	if pruned > 0 {
		if err := SaveFeedStrandedState(townRoot, state); err != nil {
			return pruned, err
		}
	}

	return pruned, nil
}

// getConvoyStatus returns the current status of a convoy bead.
func getConvoyStatus(townRoot, convoyID string) string {
	cmd := exec.Command("bd", "show", convoyID, "--json")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &issues); err != nil || len(issues) == 0 {
		return ""
	}
	return issues[0].Status
}
