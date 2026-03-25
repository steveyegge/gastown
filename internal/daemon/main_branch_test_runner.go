package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
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

// rigGateConfig holds the gate/test configuration extracted from merge-queue settings.
type rigGateConfig struct {
	TestCommand   string
	Gates         []verify.Gate
	GatesParallel bool
}

type mainBranchCheckFailure struct {
	Check string
	Error string
}

type mainBranchTestFailure struct {
	RigName       string
	RigPath       string
	DefaultBranch string
	CommitSHA     string
	Checks        []mainBranchCheckFailure
}

func (f *mainBranchTestFailure) Error() string {
	if f == nil || len(f.Checks) == 0 {
		return "main branch verification failed"
	}
	parts := make([]string, 0, len(f.Checks))
	for _, check := range f.Checks {
		parts = append(parts, fmt.Sprintf("%s: %s", check.Check, check.Error))
	}
	return strings.Join(parts, "; ")
}

// loadRigGateConfig reads the merge_queue section from layered rig settings
// to discover what verification commands to run.
func loadRigGateConfig(rigPath string) (*rigGateConfig, error) {
	if mq, err := config.LoadEffectiveMergeQueueConfig(rigPath); err != nil {
		return nil, err
	} else if mq != nil {
		gates, err := verify.GatesForPhase(mq, verify.PhasePreMerge)
		if err != nil {
			return nil, err
		}
		if len(gates) == 0 && mq.TestCommand == "" {
			return nil, nil
		}
		return &rigGateConfig{
			TestCommand:   mq.TestCommand,
			Gates:         gates,
			GatesParallel: mq.IsGatesParallelEnabled(),
		}, nil
	}

	configPath := filepath.Join(rigPath, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No config, skip
		}
		return nil, err
	}

	var raw struct {
		MergeQueue json.RawMessage `json:"merge_queue"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config.json: %w", err)
	}

	if raw.MergeQueue == nil {
		return nil, nil // No merge_queue section
	}

	var mq struct {
		TestCommand   *string                    `json:"test_command"`
		Gates         map[string]json.RawMessage `json:"gates"`
		GatesParallel *bool                      `json:"gates_parallel"`
	}
	if err := json.Unmarshal(raw.MergeQueue, &mq); err != nil {
		return nil, fmt.Errorf("parsing merge_queue: %w", err)
	}

	cfg := &rigGateConfig{}

	// Extract gates (preferred over legacy test_command)
	if len(mq.Gates) > 0 {
		cfg.Gates = make([]verify.Gate, 0, len(mq.Gates))
		for name, rawGate := range mq.Gates {
			var gate struct {
				Cmd string `json:"cmd"`
			}
			if err := json.Unmarshal(rawGate, &gate); err == nil && gate.Cmd != "" {
				cfg.Gates = append(cfg.Gates, verify.Gate{Name: name, Cmd: gate.Cmd, Phase: verify.PhasePreMerge})
			}
		}
	}
	if mq.GatesParallel != nil {
		cfg.GatesParallel = *mq.GatesParallel
	}

	// Fall back to legacy test_command
	if mq.TestCommand != nil && *mq.TestCommand != "" {
		cfg.TestCommand = *mq.TestCommand
	}

	if len(cfg.Gates) == 0 && cfg.TestCommand == "" {
		return nil, nil // No runnable commands
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
			var testFailure *mainBranchTestFailure
			if errors.As(err, &testFailure) {
				for _, check := range testFailure.Checks {
					beadID, beadErr := d.recordMainBranchCIFailure(testFailure, check)
					if beadErr != nil {
						d.logger.Printf("main_branch_test: %s: could not record ci-failure bead for %s: %v", rigName, check.Check, beadErr)
						continue
					}
					if notifyErr := d.notifyMainBranchCIFailure(testFailure, check, beadID); notifyErr != nil {
						d.logger.Printf("main_branch_test: %s: could not notify witness/refinery for %s: %v", rigName, check.Check, notifyErr)
					}
				}
			}
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
	// Load gate config from the rig's merge-queue settings.
	gateCfg, err := loadRigGateConfig(rigPath)
	if err != nil {
		return fmt.Errorf("loading gate config: %w", err)
	}
	if gateCfg == nil {
		d.logger.Printf("main_branch_test: %s: no test commands configured, skipping", rigName)
		return nil
	}

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

	worktreeGit := git.NewGit(worktreePath)
	commitSHA, err := worktreeGit.Rev("HEAD")
	if err != nil {
		return fmt.Errorf("resolving main branch commit: %w", err)
	}

	// Run gates or legacy test command
	if len(gateCfg.Gates) > 0 {
		d.logger.Printf("main_branch_test: %s: running %d gate(s) on %s", rigName, len(gateCfg.Gates), commitSHA[:min(8, len(commitSHA))])
		summary := verify.Run(ctx, worktreePath, gateCfg.Gates, gateCfg.GatesParallel, func(format string, args ...interface{}) {
			d.logger.Printf("main_branch_test: %s: %s", rigName, fmt.Sprintf(format, args...))
		})
		if !summary.Success {
			failures := make([]mainBranchCheckFailure, 0, len(summary.Results))
			for _, result := range summary.Results {
				if !result.Success {
					failures = append(failures, mainBranchCheckFailure{
						Check: result.Name,
						Error: result.Error,
					})
				}
			}
			return &mainBranchTestFailure{
				RigName:       rigName,
				RigPath:       rigPath,
				DefaultBranch: defaultBranch,
				CommitSHA:     commitSHA,
				Checks:        failures,
			}
		}
		return nil
	}
	if err := d.runCommandOnWorktree(ctx, rigName, worktreePath, "test", gateCfg.TestCommand); err != nil {
		return &mainBranchTestFailure{
			RigName:       rigName,
			RigPath:       rigPath,
			DefaultBranch: defaultBranch,
			CommitSHA:     commitSHA,
			Checks: []mainBranchCheckFailure{
				{
					Check: "test",
					Error: err.Error(),
				},
			},
		}
	}
	return nil
}

// runCommandOnWorktree runs a single shell command in the given worktree directory.
func (d *Daemon) runCommandOnWorktree(ctx context.Context, rigName, workDir, label, command string) error {
	d.logger.Printf("main_branch_test: %s: running %s: %s", rigName, label, command)

	cmd := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // G204: command is from trusted rig config
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "CI=true") // Signal test environment

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Truncate output to last 50 lines for the error message
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		tail := lines
		if len(tail) > 50 {
			tail = tail[len(tail)-50:]
		}
		return fmt.Errorf("%s failed: %v\n%s", label, err, strings.Join(tail, "\n"))
	}
	return nil
}

func (d *Daemon) recordMainBranchCIFailure(failure *mainBranchTestFailure, check mainBranchCheckFailure) (string, error) {
	if failure == nil {
		return "", fmt.Errorf("main branch failure is nil")
	}

	bd := beads.New(filepath.Join(failure.RigPath, "mayor", "rig"))
	title := fmt.Sprintf("CI failure: %s %s @ %s", failure.RigName, check.Check, shortSHA(failure.CommitSHA))
	description := d.mainBranchFailureDescription(bd, failure, check)
	priority := 2

	existing, err := bd.FindLatestOpenIssueByTitleAndLabel(title, "ci-failure")
	if err == nil && existing != nil {
		if updateErr := bd.Update(existing.ID, beads.UpdateOptions{
			Description: &description,
			Priority:    &priority,
			AddLabels:   []string{"gt:task", "ci-failure", "source:main-branch-test", "rig:" + failure.RigName},
		}); updateErr != nil {
			return "", updateErr
		}
		return existing.ID, nil
	}
	if err != nil && !errors.Is(err, beads.ErrNotFound) {
		return "", err
	}

	issue, err := bd.Create(beads.CreateOptions{
		Title:       title,
		Description: description,
		Priority:    priority,
		Labels:      []string{"gt:task", "ci-failure", "source:main-branch-test", "rig:" + failure.RigName},
	})
	if err != nil {
		return "", err
	}
	return issue.ID, nil
}

func (d *Daemon) notifyMainBranchCIFailure(failure *mainBranchTestFailure, check mainBranchCheckFailure, beadID string) error {
	if failure == nil {
		return fmt.Errorf("main branch failure is nil")
	}

	router := mail.NewRouterWithTownRoot(d.config.TownRoot, d.config.TownRoot)
	defer router.WaitPendingNotifications()

	msg := &mail.Message{
		To:      failure.RigName + "/witness",
		From:    "daemon",
		Subject: fmt.Sprintf("MAIN_BRANCH_CI_FAILURE: %s %s @ %s", failure.RigName, check.Check, shortSHA(failure.CommitSHA)),
		Body: fmt.Sprintf("Rig: %s\nBranch: %s\nCommit: %s\nCheck: %s\nIssue: %s\n%s",
			failure.RigName, failure.DefaultBranch, failure.CommitSHA, check.Check, beadID, check.Error),
	}
	return router.Send(msg)
}

func (d *Daemon) mainBranchFailureDescription(bd *beads.Beads, failure *mainBranchTestFailure, check mainBranchCheckFailure) string {
	mrID := ""
	sourceIssue := ""
	if bd != nil {
		if mr, err := bd.FindMRForMergeCommit(failure.CommitSHA); err == nil && mr != nil {
			mrID = mr.ID
			if fields := beads.ParseMRFields(mr); fields != nil {
				sourceIssue = fields.SourceIssue
			}
		}
	}

	lines := []string{
		fmt.Sprintf("rig: %s", failure.RigName),
		fmt.Sprintf("default_branch: %s", failure.DefaultBranch),
		fmt.Sprintf("merge_commit: %s", failure.CommitSHA),
		fmt.Sprintf("failing_check: %s", check.Check),
		"detected_by: main_branch_test",
	}
	if mrID != "" {
		lines = append(lines, fmt.Sprintf("merge_request: %s", mrID))
	}
	if sourceIssue != "" {
		lines = append(lines, fmt.Sprintf("source_issue: %s", sourceIssue))
	}
	lines = append(lines, "", check.Error)
	return strings.Join(lines, "\n")
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
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
