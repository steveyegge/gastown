package cmd

import (
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
	Use:   "show <wanted-id>",
	Short: "Show details of a wanted item",
	Long: `Show full details of a wanted item from the commons board.

Displays the complete title, description, and metadata for a single wanted item.

Examples:
  gt wl show w-gt-001
  gt wl show w-hop-003 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runWlShow,
}

func init() {
	wlShowCmd.Flags().BoolVar(&wlShowJSON, "json", false, "Output as JSON")
	wlCmd.AddCommand(wlShowCmd)
}

func runWlShow(cmd *cobra.Command, args []string) error {
	wantedID := args[0]

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
	cloneCmd := exec.Command(doltPath, "clone", remote, cloneDir)
	if !wlShowJSON {
		cloneCmd.Stderr = os.Stderr
	}
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("cloning %s: %w", remote, err)
	}

	query := fmt.Sprintf(
		"SELECT id, title, project, type, priority, posted_by, status, effort_level, description, created_at, updated_at, claimed_by FROM wanted WHERE id = '%s'",
		doltserver.EscapeSQL(wantedID),
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
		return fmt.Errorf("wanted item %s not found", wantedID)
	}

	row := rows[1]
	if len(row) < 12 {
		return fmt.Errorf("unexpected query result format")
	}

	// Fields: id, title, project, type, priority, posted_by, status, effort_level, description, created_at, updated_at, claimed_by
	fmt.Printf("%s %s\n", style.Bold.Render(row[0]), row[1])
	fmt.Println()
	fmt.Printf("  Project:   %s\n", row[2])
	fmt.Printf("  Type:      %s\n", row[3])
	fmt.Printf("  Priority:  %s\n", wlFormatPriority(row[4]))
	fmt.Printf("  Status:    %s\n", row[6])
	fmt.Printf("  Effort:    %s\n", row[7])
	fmt.Printf("  Posted by: %s\n", row[5])
	if row[11] != "" {
		fmt.Printf("  Claimed by: %s\n", row[11])
	}
	fmt.Printf("  Created:   %s\n", row[9])
	if row[10] != "" && row[10] != row[9] {
		fmt.Printf("  Updated:   %s\n", row[10])
	}

	if desc := strings.TrimSpace(row[8]); desc != "" {
		fmt.Println()
		fmt.Println("  DESCRIPTION")
		for _, line := range strings.Split(desc, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

