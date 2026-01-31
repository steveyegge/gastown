package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	preflightRig    string
	preflightDryRun bool
)

// PreflightReport summarizes preflight check results.
type PreflightReport struct {
	MailCleaned  int
	RigHealthy   bool
	StuckWorkers []string
	Warnings     []string
}

var preflightCmd = &cobra.Command{
	Use:     "preflight",
	GroupID: GroupWorkspace,
	Short:   "Prepare workspace for batch work",
	Long: `Run preflight checks and cleanup before starting batch work.

Preflight performs:
1. Clean stale mail in inboxes (messages older than 24h)
2. Check for stuck workers (warn only)
3. Check rig health (polecats, refinery)
4. Verify git state is clean
5. Run bd sync to ensure beads current

Use --dry-run to see what would be done without making changes.

Examples:
  gt preflight              # Check entire workspace
  gt preflight --rig myrig  # Check specific rig only
  gt preflight --dry-run    # Preview without changes`,
	RunE: runPreflight,
}

func init() {
	preflightCmd.Flags().StringVar(&preflightRig, "rig", "", "Check specific rig only")
	preflightCmd.Flags().BoolVar(&preflightDryRun, "dry-run", false, "Preview without making changes")
	rootCmd.AddCommand(preflightCmd)
}

func runPreflight(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	report := &PreflightReport{
		RigHealthy: true,
	}

	// Get rigs to check
	var rigs []*rig.Rig
	if preflightRig != "" {
		_, r, err := getRig(preflightRig)
		if err != nil {
			return fmt.Errorf("loading rig %s: %w", preflightRig, err)
		}
		rigs = []*rig.Rig{r}
	} else {
		allRigs, _, err := getAllRigs()
		if err != nil {
			return fmt.Errorf("listing rigs: %w", err)
		}
		rigs = allRigs
	}

	fmt.Printf("%s Running preflight checks...\n\n", style.Bold.Render("→"))

	// 1. Clean stale mail
	fmt.Printf("%s Checking stale mail...\n", style.Dim.Render("•"))
	mailCleaned, mailWarnings := cleanStaleMail(townRoot, rigs, preflightDryRun)
	report.MailCleaned = mailCleaned
	report.Warnings = append(report.Warnings, mailWarnings...)

	// 2. Check for stuck workers
	fmt.Printf("%s Checking for stuck workers...\n", style.Dim.Render("•"))
	stuckWorkers := checkStuckWorkers(townRoot, rigs)
	report.StuckWorkers = stuckWorkers
	if len(stuckWorkers) > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d stuck workers found", len(stuckWorkers)))
	}

	// 3. Check rig health
	fmt.Printf("%s Checking rig health...\n", style.Dim.Render("•"))
	healthWarnings := checkRigHealth(townRoot, rigs)
	if len(healthWarnings) > 0 {
		report.RigHealthy = false
		report.Warnings = append(report.Warnings, healthWarnings...)
	}

	// 4. Verify git state
	fmt.Printf("%s Verifying git state...\n", style.Dim.Render("•"))
	gitWarnings := checkGitState(townRoot, rigs)
	report.Warnings = append(report.Warnings, gitWarnings...)

	// 5. Run bd sync
	fmt.Printf("%s Syncing beads...\n", style.Dim.Render("•"))
	if !preflightDryRun {
		if err := runBdSync(townRoot); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("bd sync failed: %v", err))
		}
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("(dry-run: would run bd sync)"))
	}

	// Print report
	fmt.Println()
	printPreflightReport(report)

	return nil
}

// cleanStaleMail removes messages older than 24h from inboxes.
func cleanStaleMail(townRoot string, rigs []*rig.Rig, dryRun bool) (int, []string) {
	var totalCleaned int
	var warnings []string
	cutoff := time.Now().Add(-24 * time.Hour)

	// Check mayor/deacon inboxes at town level
	townBeadsDir := beads.ResolveBeadsDir(townRoot)
	for _, role := range []string{"mayor", "deacon"} {
		cleaned, err := cleanMailboxOlderThan(role, townRoot, townBeadsDir, cutoff, dryRun)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cleaning %s mail: %v", role, err))
		}
		totalCleaned += cleaned
	}

	// Check rig-level inboxes
	for _, r := range rigs {
		rigBeadsDir := beads.ResolveBeadsDir(r.Path)

		// Witness and refinery
		for _, role := range []string{r.Name + "/witness", r.Name + "/refinery"} {
			cleaned, err := cleanMailboxOlderThan(role, r.Path, rigBeadsDir, cutoff, dryRun)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("cleaning %s mail: %v", role, err))
			}
			totalCleaned += cleaned
		}

		// Crew workers
		crewMgr := crew.NewManager(r, git.NewGit(r.Path))
		workers, err := crewMgr.List()
		if err == nil {
			for _, w := range workers {
				identity := fmt.Sprintf("%s/crew/%s", r.Name, w.Name)
				cleaned, err := cleanMailboxOlderThan(identity, r.Path, rigBeadsDir, cutoff, dryRun)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("cleaning %s mail: %v", identity, err))
				}
				totalCleaned += cleaned
			}
		}

		// Polecats
		polecatGit := git.NewGit(r.Path)
		t := tmux.NewTmux()
		polecatMgr := polecat.NewManager(r, polecatGit, t)
		polecats, err := polecatMgr.List()
		if err == nil {
			for _, p := range polecats {
				identity := fmt.Sprintf("%s/polecats/%s", r.Name, p.Name)
				cleaned, err := cleanMailboxOlderThan(identity, r.Path, rigBeadsDir, cutoff, dryRun)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("cleaning %s mail: %v", identity, err))
				}
				totalCleaned += cleaned
			}
		}
	}

	if dryRun && totalCleaned > 0 {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(dry-run: would clean %d stale messages)", totalCleaned)))
	} else if totalCleaned > 0 {
		fmt.Printf("  Cleaned %d stale messages\n", totalCleaned)
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("No stale mail found"))
	}

	return totalCleaned, warnings
}

// cleanMailboxOlderThan removes messages older than cutoff from a mailbox.
func cleanMailboxOlderThan(identity, workDir, beadsDir string, cutoff time.Time, dryRun bool) (int, error) {
	mb := mail.NewMailboxWithBeadsDir(identity, workDir, beadsDir)
	messages, err := mb.List()
	if err != nil {
		return 0, err
	}

	var cleaned int
	for _, msg := range messages {
		if msg.Timestamp.Before(cutoff) {
			if !dryRun {
				if err := mb.Delete(msg.ID); err != nil {
					return cleaned, err
				}
			}
			cleaned++
		}
	}

	return cleaned, nil
}

// checkStuckWorkers finds agents in stuck state.
func checkStuckWorkers(townRoot string, rigs []*rig.Rig) []string {
	var stuck []string
	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Query for agents with state=stuck
	agents, err := bd.List(beads.ListOptions{
		Type:  "agent",
		Label: "gt:agent",
	})
	if err != nil {
		return stuck
	}

	for _, agent := range agents {
		if agent.AgentState == "stuck" {
			stuck = append(stuck, agent.ID)
			fmt.Printf("  %s Agent %s is stuck\n", style.WarningPrefix, agent.ID)
		}
	}

	if len(stuck) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("No stuck workers"))
	}

	return stuck
}

// checkRigHealth verifies polecats, witness, and refinery are operational.
func checkRigHealth(townRoot string, rigs []*rig.Rig) []string {
	var warnings []string
	t := tmux.NewTmux()

	for _, r := range rigs {
		// Check witness session
		witnessSession := fmt.Sprintf("gt-%s-witness", r.Name)
		if hasSession, _ := t.HasSession(witnessSession); !hasSession {
			warnings = append(warnings, fmt.Sprintf("rig %s: witness session not running", r.Name))
		}

		// Check refinery session
		refinerySession := fmt.Sprintf("gt-%s-refinery", r.Name)
		if hasSession, _ := t.HasSession(refinerySession); !hasSession {
			warnings = append(warnings, fmt.Sprintf("rig %s: refinery session not running", r.Name))
		}
	}

	if len(warnings) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("All rigs healthy"))
	} else {
		for _, w := range warnings {
			fmt.Printf("  %s %s\n", style.WarningPrefix, w)
		}
	}

	return warnings
}

// checkGitState verifies git state is clean in rig clones.
func checkGitState(townRoot string, rigs []*rig.Rig) []string {
	var warnings []string

	// Check town root
	townGit := git.NewGit(townRoot)
	if hasChanges, _ := townGit.HasUncommittedChanges(); hasChanges {
		warnings = append(warnings, "town root has uncommitted changes")
	}

	// Check rig clones
	for _, r := range rigs {
		// Mayor clone
		mayorClone := filepath.Join(r.Path, "mayor", "rig")
		if _, err := os.Stat(mayorClone); err == nil {
			mayorGit := git.NewGit(mayorClone)
			if hasChanges, _ := mayorGit.HasUncommittedChanges(); hasChanges {
				warnings = append(warnings, fmt.Sprintf("rig %s mayor clone has uncommitted changes", r.Name))
			}
		}
	}

	if len(warnings) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("Git state clean"))
	} else {
		for _, w := range warnings {
			fmt.Printf("  %s %s\n", style.WarningPrefix, w)
		}
	}

	return warnings
}

// runBdSync runs bd sync to ensure beads are current.
func runBdSync(townRoot string) error {
	cmd := exec.Command("bd", "sync")
	cmd.Dir = townRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// printPreflightReport prints a summary of the preflight results.
func printPreflightReport(report *PreflightReport) {
	fmt.Printf("%s Preflight Summary\n", style.Bold.Render("📋"))
	fmt.Printf("  Mail cleaned: %d\n", report.MailCleaned)
	fmt.Printf("  Stuck workers: %d\n", len(report.StuckWorkers))
	fmt.Printf("  Rig healthy: %v\n", report.RigHealthy)

	if len(report.Warnings) > 0 {
		fmt.Printf("\n%s Warnings (%d)\n", style.WarningPrefix, len(report.Warnings))
		for _, w := range report.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	} else {
		fmt.Printf("\n%s Workspace ready for batch work\n", style.Bold.Render("✓"))
	}
}
