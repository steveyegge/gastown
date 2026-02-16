package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/github"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// github check flags
var (
	ghCheckWatch    bool
	ghCheckInterval time.Duration
	ghCheckAuthors  []string
	ghCheckJSON     bool
	ghCheckDryRun   bool
	ghCheckRepo     string
	ghCheckRig      string
)

func init() {
	rootCmd.AddCommand(githubCmd)
	githubCmd.AddCommand(githubCheckCmd)

	githubCheckCmd.Flags().BoolVar(&ghCheckWatch, "watch", false, "Poll continuously")
	githubCheckCmd.Flags().DurationVar(&ghCheckInterval, "interval", 5*time.Minute, "Poll interval for --watch mode")
	githubCheckCmd.Flags().StringSliceVar(&ghCheckAuthors, "author", nil, "Filter PRs by author login")
	githubCheckCmd.Flags().BoolVar(&ghCheckJSON, "json", false, "Output as JSON")
	githubCheckCmd.Flags().BoolVar(&ghCheckDryRun, "dry-run", false, "Show what beads would be created")
	githubCheckCmd.Flags().StringVar(&ghCheckRepo, "repo", "", "Override repo (default: from git remote)")
	githubCheckCmd.Flags().StringVar(&ghCheckRig, "rig", "", "Override rig for bead creation")
}

var githubCmd = &cobra.Command{
	Use:     "github",
	Aliases: []string{"gh"},
	GroupID: GroupDiag,
	Short:   "GitHub integration utilities",
	RunE:    requireSubcommand,
}

var githubCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check CI status on open PRs and create beads for failures",
	Long: `Check GitHub CI status on open pull requests and auto-create beads
for failed checks.

By default, runs a one-shot check: queries open PRs, inspects their CI checks,
and creates a bead for each failure (deduplicating against existing beads).

Use --watch for continuous monitoring with configurable poll interval.

Requires the gh CLI (https://cli.github.com/) to be installed and authenticated.

Examples:
  gt github check                          # One-shot check, current repo
  gt github check --author bot-user        # Filter to specific PR authors
  gt github check --watch --interval 10m   # Poll every 10 minutes
  gt github check --dry-run                # Preview without creating beads
  gt github check --repo owner/repo        # Specify repo explicitly`,
	RunE: runGitHubCheck,
}

func runGitHubCheck(cmd *cobra.Command, args []string) error {
	// Verify gh CLI is available
	if err := github.CheckGHAvailable(); err != nil {
		return err
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Resolve beads path
	beadsPath := townRoot
	if ghCheckRig != "" {
		beadsPath = fmt.Sprintf("%s/%s", townRoot, ghCheckRig)
	}

	// Resolve actor
	actor := os.Getenv("BD_ACTOR")

	// Resolve work directory for repo detection
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg := github.CheckerConfig{
		Repo:      ghCheckRepo,
		Authors:   ghCheckAuthors,
		DryRun:    ghCheckDryRun,
		BeadsPath: beadsPath,
		Actor:     actor,
		WorkDir:   workDir,
	}

	if ghCheckWatch {
		return runWatchMode(cfg)
	}

	return runOneShotCheck(cfg)
}

func runOneShotCheck(cfg github.CheckerConfig) error {
	result, err := github.RunCheck(cfg)
	if err != nil {
		return err
	}

	if ghCheckJSON {
		return printResultJSON(result)
	}
	printResultStyled(result)
	return nil
}

func runWatchMode(cfg github.CheckerConfig) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("Watching CI checks for %s (interval: %s)\n", cfg.Repo, ghCheckInterval)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// Run immediately on start
	result, err := github.RunCheck(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.ErrorPrefix, err)
	} else {
		if ghCheckJSON {
			_ = printResultJSON(result)
		} else {
			printResultStyled(result)
		}
	}

	ticker := time.NewTicker(ghCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopped.")
			return nil
		case <-ticker.C:
			result, err := github.RunCheck(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %v\n", style.ErrorPrefix, err)
				continue
			}
			if ghCheckJSON {
				_ = printResultJSON(result)
			} else {
				printResultStyled(result)
			}
		}
	}
}

func printResultStyled(r *github.CheckResult) {
	fmt.Printf("GitHub CI Check — %s\n\n", r.Repo)

	if r.PRCount == 0 {
		fmt.Println("  No open PRs found.")
		return
	}

	if len(r.Failures) == 0 {
		fmt.Printf("  %s All checks passing across %d PR(s).\n", style.SuccessPrefix, r.PRCount)
		return
	}

	// Group failures by PR
	byPR := make(map[int][]github.CheckFailure)
	for _, f := range r.Failures {
		byPR[f.PR.Number] = append(byPR[f.PR.Number], f)
	}

	for prNum, failures := range byPR {
		pr := failures[0].PR
		fmt.Printf("  PR #%d (%s)\n", prNum, pr.HeadRef)
		for _, f := range failures {
			action := "skipped (already tracked)"
			if !strings.Contains(strings.Join(r.Created, ","), "(dry-run)") {
				// Find the bead ID for this failure
				for _, id := range r.Created {
					if id != "(dry-run)" {
						action = fmt.Sprintf("Created bead %s", id)
						break
					}
				}
			} else {
				action = "(dry-run)"
			}
			fmt.Printf("    %s %s → %s\n", style.ErrorPrefix, f.Check.Name, action)
		}
		fmt.Println()
	}

	fmt.Printf("Summary: %d failure(s), %d bead(s) created, %d already tracked\n",
		len(r.Failures), len(r.Created), r.Skipped)

	for _, e := range r.Errors {
		fmt.Fprintf(os.Stderr, "  %s %v\n", style.WarningPrefix, e)
	}
}

// jsonResult is the JSON output format.
type jsonResult struct {
	Repo       string        `json:"repo"`
	Timestamp  string        `json:"timestamp"`
	PRsChecked int           `json:"prs_checked"`
	Failures   []jsonFailure `json:"failures"`
	Summary    jsonSummary   `json:"summary"`
}

type jsonFailure struct {
	PRNumber   int    `json:"pr_number"`
	PRTitle    string `json:"pr_title"`
	CheckName  string `json:"check_name"`
	CheckURL   string `json:"check_url"`
	Conclusion string `json:"conclusion"`
	BeadID     string `json:"bead_id,omitempty"`
	Action     string `json:"action"`
}

type jsonSummary struct {
	Failures     int `json:"failures"`
	BeadsCreated int `json:"beads_created"`
	BeadsSkipped int `json:"beads_skipped"`
}

func printResultJSON(r *github.CheckResult) error {
	out := jsonResult{
		Repo:       r.Repo,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		PRsChecked: r.PRCount,
		Summary: jsonSummary{
			Failures:     len(r.Failures),
			BeadsCreated: len(r.Created),
			BeadsSkipped: r.Skipped,
		},
	}

	createdIdx := 0
	for _, f := range r.Failures {
		jf := jsonFailure{
			PRNumber:   f.PR.Number,
			PRTitle:    f.PR.Title,
			CheckName:  f.Check.Name,
			CheckURL:   f.Check.URL,
			Conclusion: f.Check.Conclusion,
		}
		if createdIdx < len(r.Created) {
			jf.BeadID = r.Created[createdIdx]
			jf.Action = "created"
			createdIdx++
		} else {
			jf.Action = "skipped"
		}
		out.Failures = append(out.Failures, jf)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
