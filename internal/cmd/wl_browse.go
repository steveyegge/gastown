package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	wlBrowseProject  string
	wlBrowseStatus   string
	wlBrowseType     string
	wlBrowsePriority int
	wlBrowseLimit    int
	wlBrowseJSON     bool
)

var wlBrowseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Browse wanted items on the commons board",
	Args:  cobra.NoArgs,
	RunE:  runWLBrowse,
	Long: `Browse the Wasteland wanted board (hop/wl-commons).

Uses the clone-then-discard pattern: clones the commons database to a
temporary directory, queries it, then deletes the clone.

EXAMPLES:
  gt wl browse                          # All open wanted items
  gt wl browse --project gastown        # Filter by project
  gt wl browse --type bug               # Only bugs
  gt wl browse --status claimed         # Claimed items
  gt wl browse --priority 0             # Critical priority only
  gt wl browse --limit 5               # Show 5 items
  gt wl browse --json                   # JSON output`,
}

func init() {
	wlBrowseCmd.Flags().StringVar(&wlBrowseProject, "project", "", "Filter by project (e.g., gastown, beads, hop)")
	wlBrowseCmd.Flags().StringVar(&wlBrowseStatus, "status", "open", "Filter by status (open, claimed, in_review, completed, withdrawn)")
	wlBrowseCmd.Flags().StringVar(&wlBrowseType, "type", "", "Filter by type (feature, bug, design, rfc, docs)")
	wlBrowseCmd.Flags().IntVar(&wlBrowsePriority, "priority", -1, "Filter by priority (0=critical, 2=medium, 4=backlog)")
	wlBrowseCmd.Flags().IntVar(&wlBrowseLimit, "limit", 50, "Maximum items to display")
	wlBrowseCmd.Flags().BoolVar(&wlBrowseJSON, "json", false, "Output as JSON")

	wlCmd.AddCommand(wlBrowseCmd)
}

func runWLBrowse(cmd *cobra.Command, args []string) error {
	if _, err := workspace.FindFromCwdOrError(); err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return fmt.Errorf("dolt not found in PATH — install from https://docs.dolthub.com/introduction/installation")
	}

	tmpDir, err := os.MkdirTemp("", "wl-browse-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	commonsOrg := "hop"
	commonsDB := "wl-commons"
	cloneDir := filepath.Join(tmpDir, commonsDB)

	remote := fmt.Sprintf("%s/%s", commonsOrg, commonsDB)
	fmt.Printf("Cloning %s...\n", style.Bold.Render(remote))

	cloneCmd := exec.Command(doltPath, "clone", remote, cloneDir)
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("cloning %s: %w\nEnsure the database exists on DoltHub: https://www.dolthub.com/%s", remote, err, remote)
	}
	fmt.Printf("%s Cloned successfully\n\n", style.Bold.Render("✓"))

	query := buildWLBrowseQuery()

	if wlBrowseJSON {
		sqlCmd := exec.Command(doltPath, "sql", "-q", query, "-r", "json")
		sqlCmd.Dir = cloneDir
		sqlCmd.Stdout = os.Stdout
		sqlCmd.Stderr = os.Stderr
		return sqlCmd.Run()
	}

	return renderWLBrowseTable(doltPath, cloneDir, query)
}

func buildWLBrowseQuery() string {
	var conditions []string

	if wlBrowseStatus != "" {
		conditions = append(conditions, fmt.Sprintf("status = '%s'", wlEscapeSQL(wlBrowseStatus)))
	}
	if wlBrowseProject != "" {
		conditions = append(conditions, fmt.Sprintf("project = '%s'", wlEscapeSQL(wlBrowseProject)))
	}
	if wlBrowseType != "" {
		conditions = append(conditions, fmt.Sprintf("type = '%s'", wlEscapeSQL(wlBrowseType)))
	}
	if wlBrowsePriority >= 0 {
		conditions = append(conditions, fmt.Sprintf("priority = %d", wlBrowsePriority))
	}

	query := "SELECT id, title, project, type, priority, posted_by, status, effort_level FROM wanted"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY priority ASC, created_at DESC"
	query += fmt.Sprintf(" LIMIT %d", wlBrowseLimit)

	return query
}

func wlEscapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func renderWLBrowseTable(doltPath, cloneDir, query string) error {
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
		fmt.Println("No wanted items found matching your filters.")
		return nil
	}

	tbl := style.NewTable(
		style.Column{Name: "ID", Width: 12},
		style.Column{Name: "TITLE", Width: 40},
		style.Column{Name: "PROJECT", Width: 12},
		style.Column{Name: "TYPE", Width: 10},
		style.Column{Name: "PRI", Width: 4, Align: style.AlignRight},
		style.Column{Name: "POSTED BY", Width: 16},
		style.Column{Name: "STATUS", Width: 10},
		style.Column{Name: "EFFORT", Width: 8},
	)

	for _, row := range rows[1:] {
		if len(row) < 8 {
			continue
		}
		pri := wlFormatPriority(row[4])
		tbl.AddRow(row[0], row[1], row[2], row[3], pri, row[5], row[6], row[7])
	}

	fmt.Printf("Wanted items (%d):\n\n", len(rows)-1)
	fmt.Print(tbl.Render())

	return nil
}

func wlParseCSV(data string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if line == "" {
			continue
		}
		rows = append(rows, wlParseCSVLine(line))
	}
	return rows
}

func wlParseCSVLine(line string) []string {
	var fields []string
	var field strings.Builder
	inQuote := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case ch == '"' && !inQuote:
			inQuote = true
		case ch == '"' && inQuote:
			if i+1 < len(line) && line[i+1] == '"' {
				field.WriteByte('"')
				i++
			} else {
				inQuote = false
			}
		case ch == ',' && !inQuote:
			fields = append(fields, field.String())
			field.Reset()
		default:
			field.WriteByte(ch)
		}
	}
	fields = append(fields, field.String())
	return fields
}

func wlFormatPriority(pri string) string {
	switch pri {
	case "0":
		return "P0"
	case "1":
		return "P1"
	case "2":
		return "P2"
	case "3":
		return "P3"
	case "4":
		return "P4"
	default:
		return pri
	}
}
