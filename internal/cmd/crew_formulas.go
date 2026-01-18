package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var crewFormulasCmd = &cobra.Command{
	Use:   "formulas",
	Short: "Show owned formulas for current crew member",
	Long: `Show the formulas owned by the current crew member.

Crew members can own formulas - reusable workflow templates that they are
responsible for maintaining and improving. This command shows which formulas
you own and their status.

The owned_formulas field in your agent bead determines which formulas you maintain.
When you own a formula:
1. Monitor execution quality when polecats run it
2. Iterate and improve based on feedback
3. Keep the formula healthy and up-to-date

Examples:
  gt crew formulas                    # Show your owned formulas
  gt crew formulas --json             # JSON output`,
	RunE: runCrewFormulas,
}

func init() {
	crewFormulasCmd.Flags().BoolVar(&crewJSON, "json", false, "Output as JSON")
	crewCmd.AddCommand(crewFormulasCmd)
}

func runCrewFormulas(cmd *cobra.Command, args []string) error {
	// Determine current crew member from BD_ACTOR or directory
	actor := os.Getenv("BD_ACTOR")
	var rigName, crewName string

	if actor != "" {
		// Parse BD_ACTOR format: rig/crew/name
		parts := strings.Split(actor, "/")
		if len(parts) >= 3 && parts[1] == "crew" {
			rigName = parts[0]
			crewName = parts[2]
		}
	}

	// If not found from BD_ACTOR, try to detect from cwd
	if crewName == "" {
		townRoot, err := workspace.FindFromCwd()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace and BD_ACTOR not set")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine current directory: %w", err)
		}

		// Check if we're in a crew directory: <rig>/crew/<name>
		rel, err := filepath.Rel(townRoot, cwd)
		if err == nil {
			parts := strings.Split(rel, string(filepath.Separator))
			for i, p := range parts {
				if p == "crew" && i+1 < len(parts) && i > 0 {
					rigName = parts[i-1]
					crewName = parts[i+1]
					break
				}
			}
		}
	}

	if crewName == "" {
		return fmt.Errorf("cannot determine crew member identity (run from crew workspace or set BD_ACTOR)")
	}

	// Get town root for beads lookup
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Get crew agent bead
	townBeads := beads.New(filepath.Join(townRoot, ".beads"))
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	crewBeadID := beads.CrewBeadIDWithPrefix(prefix, rigName, crewName)

	issue, fields, err := townBeads.GetAgentBead(crewBeadID)
	if err != nil {
		return fmt.Errorf("getting agent bead %s: %w", crewBeadID, err)
	}
	if issue == nil {
		return fmt.Errorf("agent bead %s not found", crewBeadID)
	}

	// Check for owned formulas
	if fields == nil || len(fields.OwnedFormulas) == 0 {
		fmt.Printf("%s No owned formulas for %s/%s\n",
			style.Dim.Render("ℹ"), rigName, crewName)
		fmt.Printf("\nTo assign formulas, update your agent bead:\n")
		fmt.Printf("  bd update %s --description=\"...owned_formulas: formula1,formula2...\"\n", crewBeadID)
		return nil
	}

	// Display owned formulas
	fmt.Printf("%s Owned formulas for %s/%s:\n\n",
		style.Bold.Render("📋"), rigName, crewName)

	for _, formulaName := range fields.OwnedFormulas {
		fmt.Printf("  %s %s\n", style.Bold.Render("•"), formulaName)

		// Try to find and show formula details
		formulaPath, err := findFormulaFileForCrew(townRoot, formulaName, rigName)
		if err == nil && formulaPath != "" {
			fmt.Printf("    %s %s\n", style.Dim.Render("Path:"), formulaPath)
		}
		fmt.Println()
	}

	fmt.Printf("%s\n", style.Dim.Render("Run formulas with: gt formula run <name>"))

	return nil
}

// findFormulaFileForCrew searches for a formula file, similar to findFormulaFile
// but returns the path for display purposes
func findFormulaFileForCrew(townRoot, name, rigName string) (string, error) {
	searchPaths := []string{}

	// 1. Rig's .beads/formulas/
	searchPaths = append(searchPaths, filepath.Join(townRoot, rigName, ".beads", "formulas"))

	// 2. Town .beads/formulas/
	searchPaths = append(searchPaths, filepath.Join(townRoot, ".beads", "formulas"))

	// 3. User ~/.beads/formulas/
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".beads", "formulas"))
	}

	// Try each path with common extensions
	extensions := []string{".formula.toml", ".formula.json"}
	for _, basePath := range searchPaths {
		for _, ext := range extensions {
			path := filepath.Join(basePath, name+ext)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("formula '%s' not found", name)
}
