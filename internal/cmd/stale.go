package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/version"
)

var staleJSON bool
var staleQuiet bool

var staleCmd = &cobra.Command{
	Use:     "stale",
	GroupID: GroupDiag,
	Short:   "Check if the gt binary is stale",
	Long: `Check if the gt binary was built from an older commit than the current repo HEAD.

This command compares the commit hash embedded in the binary at build time
with the current HEAD of the gastown repository.

Examples:
  gt stale              # Human-readable output
  gt stale --json       # Machine-readable JSON output
  gt stale --quiet      # Exit code only (0=stale, 1=fresh)

Exit codes:
  0 - Binary is stale (needs rebuild)
  1 - Binary is fresh (up to date)
  2 - Error (could not determine staleness)`,
	RunE: runStale,
}

func init() {
	staleCmd.Flags().BoolVar(&staleJSON, "json", false, "Output as JSON")
	staleCmd.Flags().BoolVarP(&staleQuiet, "quiet", "q", false, "Exit code only (0=stale, 1=fresh)")
	rootCmd.AddCommand(staleCmd)
}

// StaleOutput represents the JSON output structure.
type StaleOutput struct {
	Stale         bool   `json:"stale"`
	BinaryCommit  string `json:"binary_commit"`
	RepoCommit    string `json:"repo_commit"`
	CommitsBehind int    `json:"commits_behind,omitempty"`
	Error         string `json:"error,omitempty"`
}

func runStale(cmd *cobra.Command, args []string) error {
	// Find the gastown repo
	repoRoot, err := version.GetRepoRoot()
	if err != nil {
		if staleQuiet {
			os.Exit(2)
		}
		if staleJSON {
			return outputStaleJSON(StaleOutput{Error: err.Error()})
		}
		return fmt.Errorf("cannot find gastown repo: %w", err)
	}

	// Check staleness
	info := version.CheckStaleBinary(repoRoot)

	// Handle errors
	if info.Error != nil {
		if staleQuiet {
			os.Exit(2)
		}
		if staleJSON {
			return outputStaleJSON(StaleOutput{Error: info.Error.Error()})
		}
		return fmt.Errorf("staleness check failed: %w", info.Error)
	}

	// Quiet mode: just exit with appropriate code
	if staleQuiet {
		if info.IsStale {
			os.Exit(0)
		}
		os.Exit(1)
	}

	// Build output
	output := StaleOutput{
		Stale:         info.IsStale,
		BinaryCommit:  info.BinaryCommit,
		RepoCommit:    info.RepoCommit,
		CommitsBehind: info.CommitsBehind,
	}

	if staleJSON {
		return outputStaleJSON(output)
	}

	return outputStaleText(output)
}

func outputStaleJSON(output StaleOutput) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputStaleText(output StaleOutput) error {
	if output.Stale {
		fmt.Printf("%s Binary is stale\n", style.Warning.Render("⚠"))
		fmt.Printf("  Binary: %s\n", version.ShortCommit(output.BinaryCommit))
		fmt.Printf("  Repo:   %s\n", version.ShortCommit(output.RepoCommit))
		if output.CommitsBehind > 0 {
			fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(%d commits behind)", output.CommitsBehind)))
		}
		fmt.Printf("\n  Run 'go install ./cmd/gt' to rebuild\n")
	} else {
		fmt.Printf("%s Binary is fresh\n", style.Success.Render("✓"))
		fmt.Printf("  Commit: %s\n", version.ShortCommit(output.BinaryCommit))
	}
	return nil
}
