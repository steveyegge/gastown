package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/reliability"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/verify"
)

const (
	defaultMainBranchTestInterval = 30 * time.Minute
	defaultMainBranchTestTimeout  = 10 * time.Minute
)

// MainBranchTestConfig holds configuration for the main_branch_test patrol.
// This patrol periodically runs quality gates on each rig's main branch to
// catch regressions from direct-to-main pushes, bad merges, or sequential
// merge conflicts that individually pass but break together.
type MainBranchTestConfig struct {
	// Enabled controls whether the main-branch test runner runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to run, as a string (e.g., "30m").
	IntervalStr string `json:"interval,omitempty"`

	// TimeoutStr is the maximum time each rig's test run can take.
	// Default: "10m".
	TimeoutStr string `json:"timeout,omitempty"`

	// Rigs limits testing to specific rigs. If empty, all rigs are tested.
	Rigs []string `json:"rigs,omitempty"`
}

// mainBranchTestInterval returns the configured interval, or the default (30m).
func mainBranchTestInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.MainBranchTest != nil {
		if config.Patrols.MainBranchTest.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.MainBranchTest.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultMainBranchTestInterval
}

// mainBranchTestTimeout returns the configured per-rig timeout, or the default (10m).
func mainBranchTestTimeout(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.MainBranchTest != nil {
		if config.Patrols.MainBranchTest.TimeoutStr != "" {
			if d, err := time.ParseDuration(config.Patrols.MainBranchTest.TimeoutStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultMainBranchTestTimeout
}

// mainBranchTestRigs returns the configured rig filter, or nil (all rigs).
func mainBranchTestRigs(config *DaemonPatrolConfig) []string {
	if config != nil && config.Patrols != nil && config.Patrols.MainBranchTest != nil {
		return config.Patrols.MainBranchTest.Rigs
	}
	return nil
}

// rigGateConfig is the legacy gate/test view used by tests and compatibility code.
type rigGateConfig struct {
	TestCommand   string
	Gates         []verify.Gate
	GatesParallel bool
}

// loadRigGateConfig reads verification commands from a rig root. It preserves
// the legacy config.json merge_queue path for compatibility and falls back to
// the effective layered settings when present.
func loadRigGateConfig(rigPath string) (*rigGateConfig, error) {
	configPath := filepath.Join(rigPath, "config.json")
	data, err := os.ReadFile(configPath)
	if err == nil {
		var raw struct {
			MergeQueue json.RawMessage `json:"merge_queue"`
		}
		if json.Unmarshal(data, &raw) == nil && raw.MergeQueue != nil {
			var mq struct {
				TestCommand *string                    `json:"test_command"`
				Gates       map[string]json.RawMessage `json:"gates"`
			}
			if err := json.Unmarshal(raw.MergeQueue, &mq); err != nil {
				return nil, fmt.Errorf("parsing merge_queue: %w", err)
			}
			cfg := &rigGateConfig{}
			if mq.TestCommand != nil {
				cfg.TestCommand = *mq.TestCommand
			}
			if len(mq.Gates) > 0 {
				for name, rawGate := range mq.Gates {
					var gate struct {
						Cmd string `json:"cmd"`
					}
					if err := json.Unmarshal(rawGate, &gate); err == nil && gate.Cmd != "" {
						cfg.Gates = append(cfg.Gates, verify.Gate{Name: name, Cmd: gate.Cmd})
					}
				}
			}
			if cfg.TestCommand != "" || len(cfg.Gates) > 0 {
				return cfg, nil
			}
			return nil, nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	rigCtx, err := reliability.LoadRigContext(rigPath, filepath.Join(rigPath, "mayor", "rig"))
	if err != nil || rigCtx == nil || rigCtx.Settings == nil || rigCtx.Settings.MergeQueue == nil {
		return nil, err
	}
	cfg := &rigGateConfig{}
	if mq := rigCtx.Settings.MergeQueue; mq != nil {
		cfg.TestCommand = mq.TestCommand
		cfg.GatesParallel = mq.IsGatesParallelEnabled()
		for name, gate := range mq.Gates {
			if gate != nil && gate.Cmd != "" {
				cfg.Gates = append(cfg.Gates, verify.Gate{Name: name, Cmd: gate.Cmd})
			}
		}
	}
	if cfg.TestCommand == "" && len(cfg.Gates) == 0 {
		return nil, nil
	}
	return cfg, nil
}

// runMainBranchTests runs quality gates on each rig's main branch.
// It fetches the latest main, runs configured gates/tests, and escalates failures.
func (d *Daemon) runMainBranchTests() {
	if !d.isPatrolActive("main_branch_test") {
		return
	}

	d.logger.Printf("main_branch_test: starting patrol cycle")

	rigNames := d.getKnownRigs()
	if len(rigNames) == 0 {
		d.logger.Printf("main_branch_test: no rigs found")
		return
	}

	allowedRigs := mainBranchTestRigs(d.patrolConfig)
	timeout := mainBranchTestTimeout(d.patrolConfig)

	var tested, failed int
	var failures []string

	for _, rigName := range rigNames {
		if len(allowedRigs) > 0 && !sliceContains(allowedRigs, rigName) {
			continue
		}

		rigPath := filepath.Join(d.config.TownRoot, rigName)
		if err := d.testRigMainBranch(rigName, rigPath, timeout); err != nil {
			d.logger.Printf("main_branch_test: %s: FAILED: %v", rigName, err)
			failures = append(failures, fmt.Sprintf("%s: %v", rigName, err))
			failed++
		} else {
			d.logger.Printf("main_branch_test: %s: passed", rigName)
		}
		tested++
	}

	if len(failures) > 0 {
		msg := fmt.Sprintf("main branch test failures:\n%s", strings.Join(failures, "\n"))
		d.logger.Printf("main_branch_test: escalating %d failure(s)", len(failures))
		d.escalate("main_branch_test", msg)
	}

	d.logger.Printf("main_branch_test: patrol cycle complete (%d tested, %d failed)", tested, failed)
}

// testRigMainBranch tests a single rig's main branch.
func (d *Daemon) testRigMainBranch(rigName, rigPath string, timeout time.Duration) error {
	// Determine default branch
	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	// Create a temporary worktree for testing to avoid interfering with
	// the refinery's working directory.
	worktreePath := filepath.Join(rigPath, ".main-test-worktree")
	bareRepoPath := filepath.Join(rigPath, ".repo.git")

	// Verify bare repo exists
	if _, err := os.Stat(bareRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("bare repo not found at %s", bareRepoPath)
	}

	// Clean up stale worktree if it exists
	if _, err := os.Stat(worktreePath); err == nil {
		cleanupCmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
		cleanupCmd.Dir = bareRepoPath
		_ = cleanupCmd.Run()
	}

	ctx, cancel := context.WithTimeout(d.ctx, timeout)
	defer cancel()

	// Fetch latest main
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", defaultBranch)
	fetchCmd.Dir = bareRepoPath
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	// Create temporary worktree at origin/<default_branch>
	addCmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", worktreePath, "origin/"+defaultBranch)
	addCmd.Dir = bareRepoPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	// Always clean up the worktree
	defer func() {
		removeCmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
		removeCmd.Dir = bareRepoPath
		if err := removeCmd.Run(); err != nil {
			d.logger.Printf("main_branch_test: %s: warning: worktree cleanup failed: %v", rigName, err)
		}
	}()

	headSHABytes, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	headSHA := strings.TrimSpace(string(headSHABytes))

	rigCtx, err := reliability.LoadRigContext(rigPath, worktreePath)
	if err != nil {
		return fmt.Errorf("loading effective rig settings: %w", err)
	}
	if rigCtx != nil {
		if err := rigCtx.ValidateStrictPreconditions(); err != nil {
			return err
		}
	}

	if rigCtx == nil || rigCtx.Settings == nil || rigCtx.Settings.MergeQueue == nil {
		d.logger.Printf("main_branch_test: %s: no verification config, skipping", rigName)
		return nil
	}

	if _, err := rigCtx.RunVerificationPhase(ctx, verify.PhasePreMerge, nil, rigCtx.Settings.MergeQueue.IsGatesParallelEnabled()); err != nil {
		_ = d.recordMainBranchIncident(rigName, rigPath, headSHA, "local-verification", err.Error(), "")
		return err
	}
	if _, err := rigCtx.RunVerificationPhase(ctx, verify.PhasePostSquash, nil, false); err != nil {
		_ = d.recordMainBranchIncident(rigName, rigPath, headSHA, "post-squash-smoke", err.Error(), "")
		return err
	}

	if rigCtx.GitHubCI != nil && rigCtx.GitHubCI.IsRequired() {
		run, err := rigCtx.EnsureGitHubBranchCI(ctx, defaultBranch, headSHA, nil)
		if err != nil {
			runURL := ""
			if run != nil {
				runURL = run.URL
			}
			_ = d.recordMainBranchIncident(rigName, rigPath, headSHA, "github-ci", err.Error(), runURL)
			return err
		}
		if err := d.resolveMainBranchIncident(rigName, rigPath, headSHA, "github-ci"); err != nil {
			d.logger.Printf("main_branch_test: %s: warning: closing recovered github-ci incident: %v", rigName, err)
		}
	}

	if err := d.resolveMainBranchIncident(rigName, rigPath, headSHA, "local-verification"); err != nil {
		d.logger.Printf("main_branch_test: %s: warning: closing recovered local incident: %v", rigName, err)
	}
	if err := d.resolveMainBranchIncident(rigName, rigPath, headSHA, "post-squash-smoke"); err != nil {
		d.logger.Printf("main_branch_test: %s: warning: closing recovered smoke incident: %v", rigName, err)
	}
	return nil
}

// contains checks if a string slice contains a value.
func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func (d *Daemon) recordMainBranchIncident(rigName, rigPath, commitSHA, checkName, detail, workflowURL string) error {
	bd := beads.New(rigPath)
	title := fmt.Sprintf("CI failure: %s %s %s", rigName, checkName, shortSHA(commitSHA))
	mrIssue, _ := bd.FindMRForMergeCommit(commitSHA)
	sourceIssue := ""
	mrID := ""
	if mrIssue != nil {
		mrID = mrIssue.ID
		if fields := beads.ParseMRFields(mrIssue); fields != nil {
			sourceIssue = fields.SourceIssue
		}
	}

	description := buildIncidentDescription(rigName, commitSHA, checkName, detail, workflowURL, mrID, sourceIssue, "human-action-required")
	if existing, err := bd.FindLatestIssueByTitle(title); err == nil && existing != nil {
		priority := 1
		status := "open"
		return bd.Update(existing.ID, beads.UpdateOptions{
			Description: &description,
			Priority:    &priority,
			Status:      &status,
			SetLabels:   []string{"gt:incident", "gt:ci-failure"},
		})
	}

	_, err := bd.Create(beads.CreateOptions{
		Title:       title,
		Labels:      []string{"gt:incident", "gt:ci-failure"},
		Priority:    1,
		Description: description,
	})
	return err
}

func (d *Daemon) resolveMainBranchIncident(rigName, rigPath, commitSHA, checkName string) error {
	bd := beads.New(rigPath)
	title := fmt.Sprintf("CI failure: %s %s %s", rigName, checkName, shortSHA(commitSHA))
	issue, err := bd.FindLatestIssueByTitle(title)
	if err != nil || issue == nil || issue.Status == "closed" {
		return nil
	}
	recovered := buildIncidentDescription(rigName, commitSHA, checkName, "recovered automatically", "", "", "", "recovered")
	if err := bd.Update(issue.ID, beads.UpdateOptions{Description: &recovered}); err != nil {
		return err
	}
	return bd.CloseWithReason("recovered", issue.ID)
}

func buildIncidentDescription(rigName, commitSHA, checkName, detail, workflowURL, mrID, sourceIssue, statusClass string) string {
	lines := []string{
		"rig: " + rigName,
		"commit_sha: " + commitSHA,
		"check: " + checkName,
		"status_class: " + statusClass,
		"last_recovery_attempt_at: " + time.Now().UTC().Format(time.RFC3339),
		"last_recovery_result: " + statusClass,
	}
	if workflowURL != "" {
		lines = append(lines, "workflow_run: "+workflowURL)
	}
	if mrID != "" {
		lines = append(lines, "merge_request: "+mrID)
	}
	if sourceIssue != "" {
		lines = append(lines, "source_issue: "+sourceIssue)
	}
	if detail != "" {
		lines = append(lines, "", detail)
	}
	return strings.Join(lines, "\n")
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
