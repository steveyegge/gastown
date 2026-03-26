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
				cfg.Gates = make(map[string]string, len(mq.Gates))
				for name, rawGate := range mq.Gates {
					var gate struct {
						Cmd string `json:"cmd"`
					}
					if err := json.Unmarshal(rawGate, &gate); err == nil && gate.Cmd != "" {
						cfg.Gates[name] = gate.Cmd
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
