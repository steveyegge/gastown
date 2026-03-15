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
	Short: "Show details of a wanted item",
	Args:  cobra.ExactArgs(1),
	RunE:  runWLShow,
	Long: `Show full details of a wanted item from the commons board.

Displays the complete title, description, and metadata for a single
wanted item. Useful when gt wl browse truncates long titles.

EXAMPLES:
  gt wl show w-gt-001                # Show full details
  gt wl show w-hop-003 --json        # JSON output`,
}

func init() {
	wlShowCmd.Flags().BoolVar(&wlShowJSON, "json", false, "Output as JSON")
	wlCmd.AddCommand(wlShowCmd)
}

func runWLShow(cmd *cobra.Command, args []string) error {
	workID := args[0]

	if _, err := workspace.FindFromCwdOrError(); err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return fmt.Errorf("dolt not found in PATH — install from https://docs.dolthub.com/introduction/installation")
	}

	tmpDir, err := os.MkdirTemp("", "wl-show-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	commonsOrg := "hop"
	commonsDB := "wl-commons"
	cloneDir := filepath.Join(tmpDir, commonsDB)

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

	query := fmt.Sprintf(
		"SELECT id, title, description, project, type, priority, tags, posted_by, status, effort_level, created_at, updated_at FROM wanted WHERE id = '%s'",
		doltserver.EscapeSQL(workID),
	)

	if wlShowJSON {
		sqlCmd := exec.Command(doltPath, "sql", "-q", query, "-r", "json")
		sqlCmd.Dir = cloneDir
		sqlCmd.Stdout = os.Stdout
		sqlCmd.Stderr = os.Stderr
		return sqlCmd.Run()
	}

	sqlCmd := exec.Command(doltPath, "sql", "-q", query, "-r", "csv")
	sqlCmd.Dir = cloneDir
	output, err := sqlCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("query failed: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("running query: %w", err)
	}

	rows := wlParseCSV(string(output))
	if len(rows) <= 1 {
		return fmt.Errorf("wanted item %q not found", workID)
	}

	return renderWLShowItem(rows[0], rows[1])
}

func renderWLShowItem(headers, row []string) error {
	// Build a map for easy access
	fields := make(map[string]string)
	for i, h := range headers {
		if i < len(row) {
			fields[h] = row[i]
		}
	}

	pri := wlFormatPriority(fields["priority"])

	fmt.Printf("\n%s %s\n", style.Bold.Render(fields["id"]), style.Bold.Render(fields["title"]))
	fmt.Printf("  Status: %s  Priority: %s  Type: %s  Effort: %s\n",
		fields["status"], pri, fields["type"], fields["effort_level"])
	fmt.Printf("  Project: %s  Posted by: %s\n", fields["project"], fields["posted_by"])

	if tags := fields["tags"]; tags != "" && tags != "null" && tags != "NULL" {
		// Try to format JSON tags nicely
		var tagList []string
		if err := json.Unmarshal([]byte(tags), &tagList); err == nil {
			fmt.Printf("  Tags: %s\n", strings.Join(tagList, ", "))
		} else {
			fmt.Printf("  Tags: %s\n", tags)
		}
	}

	fmt.Printf("  Created: %s  Updated: %s\n", fields["created_at"], fields["updated_at"])

	if desc := fields["description"]; desc != "" && desc != "null" && desc != "NULL" {
		fmt.Printf("\n%s\n%s\n", style.Bold.Render("Description:"), desc)
	}

	return nil
}
