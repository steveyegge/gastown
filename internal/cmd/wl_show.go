package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlShowJSON bool

var wlShowCmd = &cobra.Command{
	Use:   "show <work-id>",
	Short: "Show full details of a wanted item",
	Args:  cobra.ExactArgs(1),
	RunE:  runWLShow,
	Long: `Show the full details of a wasteland wanted item by ID.

Uses the local fork directory if available (fast), otherwise clones
the commons database temporarily.

EXAMPLES:
  gt wl show w-gc-003          # Show details of work item
  gt wl show w-gc-003 --json   # JSON output`,
}

func init() {
	wlShowCmd.Flags().BoolVar(&wlShowJSON, "json", false, "Output as JSON")
	wlCmd.AddCommand(wlShowCmd)
}

func runWLShow(cmd *cobra.Command, args []string) error {
	workID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return fmt.Errorf("dolt not found in PATH — install from https://docs.dolthub.com/introduction/installation")
	}

	// Try local fork first (fast path)
	forkDir := findWLCommonsFork(townRoot)
	cloneDir := forkDir

	// No local fork — clone fresh (like wl browse)
	if cloneDir == "" {
		tmpDir, tmpErr := os.MkdirTemp("", "wl-show-*")
		if tmpErr != nil {
			return fmt.Errorf("creating temp directory: %w", tmpErr)
		}
		defer os.RemoveAll(tmpDir)

		commonsOrg := "hop"
		commonsDB := "wl-commons"
		cloneDir = filepath.Join(tmpDir, commonsDB)
		remote := fmt.Sprintf("%s/%s", commonsOrg, commonsDB)

		if !wlShowJSON {
			fmt.Printf("Cloning %s...\n", style.Bold.Render(remote))
		}

		cloneCmd := exec.Command(doltPath, "clone", remote, cloneDir)
		if !wlShowJSON {
			cloneCmd.Stderr = os.Stderr
		}
		if err := cloneCmd.Run(); err != nil {
			return fmt.Errorf("cloning %s: %w", remote, err)
		}
	}

	// Query the wanted item by ID
	query := fmt.Sprintf(
		"SELECT id, title, description, project, type, priority, tags, posted_by, status, effort_level, created_at, updated_at FROM wanted WHERE id = '%s'",
		doltserver.EscapeSQL(workID),
	)

	sqlCmd := exec.Command(doltPath, "sql", "-q", query, "-r", "json")
	sqlCmd.Dir = cloneDir
	output, err := sqlCmd.Output()
	if err != nil {
		return fmt.Errorf("querying wanted item: %w", err)
	}

	// Parse JSON response — dolt returns {"rows": [...]}
	var result struct {
		Rows []map[string]interface{} `json:"rows"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Rows) == 0 {
		return fmt.Errorf("wanted item %q not found", workID)
	}

	item := result.Rows[0]

	if wlShowJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(item)
	}

	// Human-readable output
	fmt.Printf("%s %s\n", style.Bold.Render("ID:"), getString(item, "id"))
	fmt.Printf("%s %s\n", style.Bold.Render("Title:"), getString(item, "title"))
	fmt.Printf("%s %s\n", style.Bold.Render("Project:"), getString(item, "project"))
	fmt.Printf("%s %s\n", style.Bold.Render("Type:"), getString(item, "type"))
	fmt.Printf("%s %s\n", style.Bold.Render("Priority:"), getString(item, "priority"))
	fmt.Printf("%s %s\n", style.Bold.Render("Status:"), getString(item, "status"))
	fmt.Printf("%s %s\n", style.Bold.Render("Effort:"), getString(item, "effort_level"))
	fmt.Printf("%s %s\n", style.Bold.Render("Posted by:"), getString(item, "posted_by"))
	fmt.Printf("%s %s\n", style.Bold.Render("Tags:"), getString(item, "tags"))
	fmt.Printf("%s %s\n", style.Bold.Render("Created:"), getString(item, "created_at"))
	fmt.Printf("%s %s\n", style.Bold.Render("Updated:"), getString(item, "updated_at"))

	desc := getString(item, "description")
	if desc != "" {
		fmt.Printf("\n%s\n%s\n", style.Bold.Render("Description:"), desc)
	}

	// Also show completions for this item
	compQuery := fmt.Sprintf(
		"SELECT id, completed_by, evidence, completed_at FROM completions WHERE wanted_id = '%s'",
		doltserver.EscapeSQL(workID),
	)
	compCmd := exec.Command(doltPath, "sql", "-q", compQuery, "-r", "json")
	compCmd.Dir = cloneDir
	compOutput, compErr := compCmd.Output()
	if compErr == nil {
		var compResult struct {
			Rows []map[string]interface{} `json:"rows"`
		}
		if json.Unmarshal(compOutput, &compResult) == nil && len(compResult.Rows) > 0 {
			fmt.Printf("\n%s (%d)\n", style.Bold.Render("Completions:"), len(compResult.Rows))
			for _, comp := range compResult.Rows {
				fmt.Printf("  %s by %s (%s)\n",
					getString(comp, "id"),
					getString(comp, "completed_by"),
					getString(comp, "completed_at"),
				)
				if ev := getString(comp, "evidence"); ev != "" {
					// Show first 200 chars of evidence
					if len(ev) > 200 {
						ev = ev[:200] + "..."
					}
					fmt.Printf("    %s\n", strings.ReplaceAll(ev, "\n", "\n    "))
				}
			}
		}
	}

	return nil
}

// getString safely extracts a string from a map value.
func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
