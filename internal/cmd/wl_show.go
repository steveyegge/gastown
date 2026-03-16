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
	Long: `Show the full details of a wanted item by ID.

Unlike browse (which truncates titles), show displays all fields
including the full description and evidence URL.

EXAMPLES:
  gt wl show wl-abc123             # Show item details
  gt wl show wl-abc123 --json      # JSON output`,
}

func init() {
	wlShowCmd.Flags().BoolVar(&wlShowJSON, "json", false, "Output as JSON")
	wlCmd.AddCommand(wlShowCmd)
}

func runWLShow(cmd *cobra.Command, args []string) error {
	if _, err := workspace.FindFromCwdOrError(); err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	workID := args[0]

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
		"SELECT id, title, description, project, type, priority, tags, posted_by, claimed_by, status, effort_level, evidence_url, created_at, updated_at FROM wanted WHERE id = '%s'",
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

	return renderWLShowDetail(rows[0], rows[1])
}

func renderWLShowDetail(headers, row []string) error {
	if len(row) < 14 {
		return fmt.Errorf("unexpected row length: got %d, want 14", len(row))
	}

	// Build a map for easier access
	fields := make(map[string]string)
	for i, h := range headers {
		if i < len(row) {
			fields[h] = row[i]
		}
	}

	pri := wlFormatPriority(fields["priority"])

	fmt.Printf("\n%s %s\n", style.Bold.Render(fields["id"]), pri)
	fmt.Printf("%s\n\n", style.Bold.Render(fields["title"]))

	printField("Project", fields["project"])
	printField("Type", fields["type"])
	printField("Status", fields["status"])
	printField("Effort", fields["effort_level"])
	printField("Posted by", fields["posted_by"])
	if fields["claimed_by"] != "" {
		printField("Claimed by", fields["claimed_by"])
	}
	printField("Created", fields["created_at"])
	if fields["updated_at"] != "" && fields["updated_at"] != fields["created_at"] {
		printField("Updated", fields["updated_at"])
	}
	if fields["evidence_url"] != "" {
		printField("Evidence", fields["evidence_url"])
	}
	if fields["tags"] != "" && fields["tags"] != "null" {
		var tags []string
		if err := json.Unmarshal([]byte(fields["tags"]), &tags); err == nil && len(tags) > 0 {
			printField("Tags", strings.Join(tags, ", "))
		}
	}

	if desc := fields["description"]; desc != "" {
		fmt.Printf("\n%s\n%s\n", style.Bold.Render("Description:"), desc)
	}

	fmt.Println()
	return nil
}

func printField(label, value string) {
	if value == "" {
		return
	}
	fmt.Printf("  %-12s %s\n", label+":", value)
}
