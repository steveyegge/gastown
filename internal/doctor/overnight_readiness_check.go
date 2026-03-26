package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/githubci"
	"github.com/steveyegge/gastown/internal/reliability"
)

// OvernightReadinessCheck verifies that a rig has the prerequisites for unattended operation.
type OvernightReadinessCheck struct {
	BaseCheck
}

// NewOvernightReadinessCheck creates a new overnight readiness check.
func NewOvernightReadinessCheck() *OvernightReadinessCheck {
	return &OvernightReadinessCheck{
		BaseCheck: BaseCheck{
			CheckName:        "overnight-readiness",
			CheckDescription: "Check strict verifier, GitHub CI fallback, and integration dependencies for overnight runs",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run verifies overnight prerequisites across the selected rigs.
func (c *OvernightReadinessCheck) Run(ctx *CheckContext) *CheckResult {
	rigNames := c.resolveRigs(ctx)
	if len(rigNames) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs selected for overnight readiness",
		}
	}

	var problems []string
	var warnings []string

	if _, err := exec.LookPath("tmux"); err != nil {
		problems = append(problems, "tmux not found in PATH")
	}

	ghClient := githubci.New()
	ghChecked := false
	ghErr := error(nil)

	for _, rigName := range rigNames {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		repoRoot := filepath.Join(rigPath, "mayor", "rig")
		rigCtx, err := reliability.LoadRigContext(rigPath, repoRoot)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: load effective settings: %v", rigName, err))
			continue
		}
		if rigCtx == nil || rigCtx.Settings == nil || rigCtx.Settings.RepoContract == nil {
			problems = append(problems, fmt.Sprintf("%s: repo_contract missing", rigName))
			continue
		}
		if err := config.ValidateStrictRepoContract(rigCtx.Settings); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", rigName, err))
		}
		if dirty, err := isDirtyRepo(repoRoot); err == nil && dirty {
			warnings = append(warnings, fmt.Sprintf("%s: mayor/rig has uncommitted changes", rigName))
		}
		if rigCtx.GitHubCI != nil && rigCtx.GitHubCI.IsRequired() {
			if !ghChecked {
				authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				ghErr = ghClient.CheckAuth(authCtx)
				cancel()
				ghChecked = true
			}
			if ghErr != nil {
				problems = append(problems, fmt.Sprintf("%s: gh auth status failed: %v", rigName, ghErr))
			}
			workflowFile, err := githubci.FindWorkflowFile(repoRoot, rigCtx.GitHubCI.WorkflowName())
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", rigName, err))
			} else if ok, err := githubci.WorkflowSupportsDispatch(workflowFile); err != nil {
				problems = append(problems, fmt.Sprintf("%s: read workflow %s: %v", rigName, workflowFile, err))
			} else if !ok {
				problems = append(problems, fmt.Sprintf("%s: workflow %s lacks workflow_dispatch fallback", rigName, filepath.Base(workflowFile)))
			}
		}
		if needsIntegration(rigCtx.Settings) {
			if _, err := exec.LookPath("docker"); err != nil {
				problems = append(problems, fmt.Sprintf("%s: docker not found but integration_command is configured", rigName))
			} else if err := dockerReachable(); err != nil {
				problems = append(problems, fmt.Sprintf("%s: docker unavailable: %v", rigName, err))
			}
			if _, err := exec.LookPath("dolt"); err != nil {
				problems = append(problems, fmt.Sprintf("%s: dolt not found but integration_command is configured", rigName))
			}
		}
	}

	if len(problems) > 0 {
		details := append([]string(nil), problems...)
		if len(warnings) > 0 {
			details = append(details, "", "Warnings:")
			details = append(details, warnings...)
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d overnight blocker(s)", len(problems)),
			Details: details,
			FixHint: "Add repo_contract verifier commands, enable workflow_dispatch, authenticate gh, and ensure Docker/Dolt are reachable",
		}
	}

	if len(warnings) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Overnight prerequisites met with %d warning(s)", len(warnings)),
			Details: warnings,
			FixHint: "Clean dirty mayor/rig clones before unattended runs",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d rig(s) ready for overnight operation", len(rigNames)),
	}
}

func (c *OvernightReadinessCheck) resolveRigs(ctx *CheckContext) []string {
	if ctx.RigName != "" {
		return []string{ctx.RigName}
	}
	rigs := loadRigNames(filepath.Join(ctx.TownRoot, "mayor", "rigs.json"))
	names := make([]string, 0, len(rigs))
	for name := range rigs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func needsIntegration(settings *config.RigSettings) bool {
	if settings == nil || settings.RepoContract == nil {
		return false
	}
	return strings.TrimSpace(settings.RepoContract.IntegrationCommand) != ""
}

func dockerReachable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info") //nolint:gosec // fixed command
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isDirtyRepo(repoRoot string) (bool, error) {
	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain") //nolint:gosec // fixed command
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}
