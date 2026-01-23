package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var workloadJSON bool

var workloadCmd = &cobra.Command{
	Use:     "workload [agent]",
	GroupID: GroupWork,
	Short:   "Show all hooked work for an agent",
	Long: `Show all work (beads with status=hooked) assigned to an agent.

Unlike 'gt hook' which shows only the current work, workload shows the FULL
queue of hooked issues for an agent. This is essential for workload management
when multiple issues are slung to the same target.

With no arguments, shows your own workload. With an agent argument, shows that
agent's workload.

Examples:
  gt workload                           # My workload
  gt workload gastown/crew/dave         # Dave's workload
  gt workload gastown/polecats/nux      # Nux's workload
  gt workload --json                    # Machine-readable output

The output shows:
  - Total count of hooked issues
  - Each issue ID, title, and priority
  - Which convoy (if any) tracks each issue`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkload,
}

func init() {
	workloadCmd.Flags().BoolVar(&workloadJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(workloadCmd)
}

// hookedBeadInfo holds info about a hooked bead.
type hookedBeadInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
	Convoy   string `json:"convoy,omitempty"`
	HookedAt string `json:"hooked_at,omitempty"`
}

// workloadInfo holds the full workload for an agent.
type workloadInfo struct {
	Agent  string           `json:"agent"`
	Count  int              `json:"count"`
	Beads  []hookedBeadInfo `json:"beads"`
}

func runWorkload(cmd *cobra.Command, args []string) error {
	var target string
	if len(args) > 0 {
		target = args[0]
	} else {
		// Auto-detect current agent from context
		agentID, _, _, err := resolveSelfTarget()
		if err != nil {
			return fmt.Errorf("auto-detecting agent (use explicit argument): %w", err)
		}
		target = agentID
	}

	// Get town root for cross-rig queries
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Query all hooked beads for this agent across all rigs
	hooked, err := getHookedBeadsForAgent(townRoot, target)
	if err != nil {
		return fmt.Errorf("querying workload: %w", err)
	}

	// Build workload info
	workload := workloadInfo{
		Agent: target,
		Count: len(hooked),
		Beads: hooked,
	}

	if workloadJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(workload)
	}

	// Human-readable output
	if len(hooked) == 0 {
		fmt.Printf("%s has no hooked work\n", target)
		return nil
	}

	fmt.Printf("%s Workload for %s (%d issues):\n\n", style.Bold.Render("📋"), target, len(hooked))
	for i, bead := range hooked {
		// Priority indicator
		priorityStr := ""
		if bead.Priority > 0 {
			priorityStr = fmt.Sprintf(" P%d", bead.Priority)
		}

		// Convoy info
		convoyStr := ""
		if bead.Convoy != "" {
			convoyStr = fmt.Sprintf(" [%s]", bead.Convoy)
		}

		fmt.Printf("  %d. %s: %s%s%s\n", i+1, bead.ID, bead.Title, priorityStr, convoyStr)
	}

	return nil
}

// getHookedBeadsForAgent queries all hooked beads assigned to an agent.
// Searches across all rig beads databases and town beads.
//
//nolint:unparam // error return kept for future use
func getHookedBeadsForAgent(townRoot, agentID string) ([]hookedBeadInfo, error) {
	var allHooked []hookedBeadInfo
	seen := make(map[string]bool)

	// Find all beads databases
	beadsDirs := findAllBeadsDirs(townRoot)

	// Query each beads database
	for _, beadsDir := range beadsDirs {
		hooked := queryHookedBeads(beadsDir, agentID)
		for _, h := range hooked {
			if !seen[h.ID] {
				seen[h.ID] = true
				// Look up convoy for this bead
				h.Convoy = isTrackedByConvoy(h.ID)
				allHooked = append(allHooked, h)
			}
		}
	}

	return allHooked, nil
}

// findAllBeadsDirs finds all .beads directories in the town.
func findAllBeadsDirs(townRoot string) []string {
	var dirs []string

	// Town-level beads
	townBeads := filepath.Join(townRoot, ".beads")
	if _, err := os.Stat(townBeads); err == nil {
		dirs = append(dirs, townBeads)
	}

	// Rig-level beads (in each rig's mayor/rig/.beads)
	rigDirs, _ := filepath.Glob(filepath.Join(townRoot, "*", "polecats"))
	for _, polecatsDir := range rigDirs {
		rigDir := filepath.Dir(polecatsDir)
		rigBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
		if _, err := os.Stat(rigBeads); err == nil {
			dirs = append(dirs, rigBeads)
		}
		// Also check rig root for beads (some rigs use this layout)
		altRigBeads := filepath.Join(rigDir, ".beads")
		if _, err := os.Stat(altRigBeads); err == nil && altRigBeads != townBeads {
			dirs = append(dirs, altRigBeads)
		}
	}

	return dirs
}

// queryHookedBeads queries a beads database for hooked issues assigned to an agent.
func queryHookedBeads(beadsDir, agentID string) []hookedBeadInfo {
	dbPath := filepath.Join(beadsDir, "beads.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}

	// Query for hooked issues assigned to this agent
	// Escape single quotes in agentID to prevent SQL injection
	safeAgentID := strings.ReplaceAll(agentID, "'", "''")
	query := fmt.Sprintf(`
		SELECT id, title, priority, updated_at
		FROM issues
		WHERE status = 'hooked' AND assignee = '%s'
		ORDER BY priority DESC, updated_at ASC
	`, safeAgentID)

	queryCmd := exec.Command("sqlite3", "-json", dbPath, query)
	var stdout bytes.Buffer
	queryCmd.Stdout = &stdout

	if err := queryCmd.Run(); err != nil {
		return nil
	}

	var results []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Priority  int    `json:"priority"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil
	}

	var hooked []hookedBeadInfo
	for _, r := range results {
		hooked = append(hooked, hookedBeadInfo{
			ID:       r.ID,
			Title:    r.Title,
			Priority: r.Priority,
			HookedAt: r.UpdatedAt,
		})
	}

	return hooked
}

// countHookedBeadsForAgent counts how many hooked beads are assigned to an agent.
// This is a fast count-only query used for workload warnings.
func countHookedBeadsForAgent(townRoot, agentID string) int {
	count := 0
	beadsDirs := findAllBeadsDirs(townRoot)

	for _, beadsDir := range beadsDirs {
		dbPath := filepath.Join(beadsDir, "beads.db")
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}

		// Escape single quotes in agentID to prevent SQL injection
		safeAgentID := strings.ReplaceAll(agentID, "'", "''")
		query := fmt.Sprintf(`
			SELECT COUNT(*) FROM issues
			WHERE status = 'hooked' AND assignee = '%s'
		`, safeAgentID)

		queryCmd := exec.Command("sqlite3", dbPath, query)
		out, err := queryCmd.Output()
		if err != nil {
			continue
		}

		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n); err == nil {
			count += n
		}
	}

	return count
}
