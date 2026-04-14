package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	// defaultForemanDispatcherInterval is the check interval for PR comment polling.
	defaultForemanDispatcherInterval = 60 * time.Second

	// foremanDispatcherDiagEveryN logs a diagnostic summary every N scans.
	foremanDispatcherDiagEveryN = 10

	// foremanBeadQueryTimeout is the max time to wait for bd subprocess calls.
	foremanBeadQueryTimeout = 30 * time.Second
)

// foremanDispatcherScanCount tracks how many scans have run.
var foremanDispatcherScanCount atomic.Int64

// ForemanDispatcherConfig holds configuration for the foreman_dispatcher patrol.
type ForemanDispatcherConfig struct {
	Enabled     bool   `json:"enabled"`
	IntervalStr string `json:"interval,omitempty"`
}

// foremanDispatcherInterval returns the configured interval, or the default.
func foremanDispatcherInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.ForemanDispatcher != nil {
		if config.Patrols.ForemanDispatcher.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.ForemanDispatcher.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultForemanDispatcherInterval
}

// foremanBead represents a bead with role=foreman and a PR to track.
type foremanBead struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Assignee   string `json:"assignee"`
	Labels     []string `json:"labels"`
	Title      string `json:"title"`
	Description string `json:"description"`
	// Parsed from description
	PRURL      string
	PRNumber   string
	SourceBead string
	Rig        string
}

// foremanDispatcherState tracks per-PR comment counts to detect new activity.
type foremanDispatcherState struct {
	PRStates map[string]*prState `json:"pr_states"` // keyed by bead ID
}

type prState struct {
	LastCommentCount int    `json:"last_comment_count"`
	LastReviewCount  int    `json:"last_review_count"`
	LastReviewDecision string `json:"last_review_decision"`
	LastCheckedAt    string `json:"last_checked_at"`
}

// dispatchForemen is the daemon patrol method that polls for PR comment activity
// on open foreman beads and nudges/spawns foremen as needed.
func (d *Daemon) dispatchForemen() {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Printf("foreman_dispatcher: recovered from panic: %v", r)
		}
	}()

	if !d.isPatrolActive("foreman_dispatcher") {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, foremanBeadQueryTimeout)
	defer cancel()

	// Find open beads with role=foreman in description
	foremen, err := findForemanBeads(ctx, d.config.TownRoot)
	if err != nil {
		d.logger.Printf("foreman_dispatcher: error finding foreman beads: %v", err)
		return
	}

	scanNum := foremanDispatcherScanCount.Add(1)

	if len(foremen) == 0 {
		if scanNum%foremanDispatcherDiagEveryN == 1 {
			d.logger.Printf("foreman_dispatcher: alive (scan #%d, no foreman beads)", scanNum)
		}
		return
	}

	// Load state file
	state := loadForemanState(d.config.TownRoot)

	t := tmux.NewTmux()
	nudged := 0
	dispatched := 0

	for _, fb := range foremen {
		if fb.PRURL == "" {
			continue
		}

		// Check GitHub for new comments
		newActivity, commentCount, reviewCount, reviewDecision := checkPRActivity(ctx, fb)
		ps := state.PRStates[fb.ID]
		if ps == nil {
			ps = &prState{}
			state.PRStates[fb.ID] = ps
		}

		hasNew := commentCount > ps.LastCommentCount || reviewCount > ps.LastReviewCount ||
			(reviewDecision != ps.LastReviewDecision && isActionableDecision(reviewDecision))

		ps.LastCommentCount = commentCount
		ps.LastReviewCount = reviewCount
		ps.LastReviewDecision = reviewDecision
		ps.LastCheckedAt = time.Now().UTC().Format(time.RFC3339)

		if !hasNew && !newActivity {
			continue
		}

		d.logger.Printf("foreman_dispatcher: %s has new PR activity (comments=%d reviews=%d decision=%s)",
			fb.ID, commentCount, reviewCount, reviewDecision)

		// Check if foreman session is alive
		if fb.Assignee != "" {
			parts := strings.Split(fb.Assignee, "/")
			if len(parts) >= 3 {
				prefix := session.PrefixFor(parts[0])
				sessName := session.PolecatSessionName(prefix, parts[len(parts)-1])
				if alive, _ := t.HasSession(sessName); alive {
					// Nudge existing session
					_ = t.NudgeSession(sessName, fmt.Sprintf("New PR activity on %s", fb.PRURL))
					nudged++
					continue
				}
			}
		}

		// No live session — dispatch a new foreman via gt sling
		rig := fb.Rig
		if rig == "" {
			if len(fb.Labels) > 0 {
				rig = fb.Labels[0]
			}
		}
		if rig == "" {
			d.logger.Printf("foreman_dispatcher: %s has no rig label, skipping dispatch", fb.ID)
			continue
		}

		d.logger.Printf("foreman_dispatcher: dispatching foreman for %s to %s", fb.ID, rig)
		slingCmd := exec.CommandContext(ctx, "gt", "sling", fb.ID, rig,
			"--force", "--no-boot", "--formula", "mol-foreman-pr-response")
		slingCmd.Dir = d.config.TownRoot
		slingCmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(d.config.TownRoot))
		util.SetDetachedProcessGroup(slingCmd)
		if err := slingCmd.Run(); err != nil {
			d.logger.Printf("foreman_dispatcher: sling failed for %s: %v", fb.ID, err)
		} else {
			dispatched++
		}
	}

	if nudged > 0 || dispatched > 0 {
		d.logger.Printf("foreman_dispatcher: scanned=%d nudged=%d dispatched=%d",
			len(foremen), nudged, dispatched)
	} else if scanNum%foremanDispatcherDiagEveryN == 1 {
		d.logger.Printf("foreman_dispatcher: alive (scan #%d, foremen=%d, no new activity)",
			scanNum, len(foremen))
	}

	saveForemanState(d.config.TownRoot, state)
}

// findForemanBeads queries for open beads that have role=foreman in their description.
func findForemanBeads(ctx context.Context, townRoot string) ([]*foremanBead, error) {
	cmd := exec.CommandContext(ctx, "bd", "list", "--status=open", "--json", "--flat")
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(townRoot))
	util.SetDetachedProcessGroup(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var allBeads []foremanBead
	if err := json.Unmarshal(bytes.TrimSpace(output), &allBeads); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	var foremen []*foremanBead
	for i := range allBeads {
		b := &allBeads[i]
		if !strings.Contains(b.Description, "role: foreman") {
			continue
		}
		// Parse structured fields from description
		for _, line := range strings.Split(b.Description, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pr_url:") {
				b.PRURL = strings.TrimSpace(strings.TrimPrefix(line, "pr_url:"))
				// Extract PR number from URL
				parts := strings.Split(b.PRURL, "/")
				if len(parts) > 0 {
					b.PRNumber = parts[len(parts)-1]
				}
			} else if strings.HasPrefix(line, "source_bead:") {
				b.SourceBead = strings.TrimSpace(strings.TrimPrefix(line, "source_bead:"))
			}
		}
		// Derive rig from labels
		if len(b.Labels) > 0 {
			b.Rig = b.Labels[0]
		}
		foremen = append(foremen, b)
	}
	return foremen, nil
}

// checkPRActivity checks GitHub for comment/review activity on a PR.
// Returns (hasNew, commentCount, reviewCount, reviewDecision).
func checkPRActivity(ctx context.Context, fb *foremanBead) (bool, int, int, string) {
	if fb.PRNumber == "" {
		return false, 0, 0, ""
	}

	// Extract owner/repo from PR URL
	// Format: https://github.com/<owner>/<repo>/pull/<number>
	parts := strings.Split(strings.TrimSuffix(fb.PRURL, "/"), "/")
	if len(parts) < 5 {
		return false, 0, 0, ""
	}
	repo := parts[len(parts)-4] + "/" + parts[len(parts)-3]

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fb.PRNumber,
		"--repo", repo,
		"--json", "comments,reviews,reviewDecision")
	util.SetDetachedProcessGroup(cmd)

	output, err := cmd.Output()
	if err != nil {
		return false, 0, 0, ""
	}

	var pr struct {
		Comments       []json.RawMessage `json:"comments"`
		Reviews        []json.RawMessage `json:"reviews"`
		ReviewDecision string            `json:"reviewDecision"`
	}
	if err := json.Unmarshal(output, &pr); err != nil {
		return false, 0, 0, ""
	}

	return true, len(pr.Comments), len(pr.Reviews), pr.ReviewDecision
}

func isActionableDecision(d string) bool {
	return d == "CHANGES_REQUESTED" || d == "APPROVED"
}

// State persistence

func foremanStatePath(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "foreman-state.json")
}

func loadForemanState(townRoot string) *foremanDispatcherState {
	state := &foremanDispatcherState{PRStates: make(map[string]*prState)}
	data, err := os.ReadFile(foremanStatePath(townRoot))
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, state)
	if state.PRStates == nil {
		state.PRStates = make(map[string]*prState)
	}
	return state
}

func saveForemanState(townRoot string, state *foremanDispatcherState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(foremanStatePath(townRoot), data, 0644)
}
