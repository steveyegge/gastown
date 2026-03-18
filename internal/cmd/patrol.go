// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	// Patrol digest flags
	patrolDigestYesterday bool
	patrolDigestDate      string
	patrolDigestDryRun    bool
	patrolDigestVerbose   bool
)

// OverseerDutyConfig stores the formulas assigned to the Overseer.
// Persisted at mayor/overseer-patrol.json.
type OverseerDutyConfig struct {
	Formulas []string `json:"formulas"`
}

var patrolCmd = &cobra.Command{
	Use:     "patrol",
	GroupID: GroupDiag,
	Short:   "Patrol management (digest + overseer duties)",
	Long: `Manage patrol cycle digests and overseer duty assignments.

Patrol cycles (Deacon, Witness, Refinery) create ephemeral per-cycle digests.
The overseer duty commands manage which formulas the Overseer executes.

Examples:
  gt patrol digest --yesterday      # Aggregate yesterday's patrol digests
  gt patrol digest --dry-run        # Preview what would be aggregated
  gt patrol duties                  # List formulas assigned to overseer
  gt patrol assign mol-my-formula   # Assign a formula to overseer
  gt patrol unassign mol-my-formula # Remove a formula from overseer`,
}

var patrolDigestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Aggregate patrol cycle digests into a daily summary bead",
	Long: `Aggregate ephemeral patrol cycle digests into a permanent daily summary.

This command is intended to be run by Deacon patrol (daily) or manually.
It queries patrol digests for a target date, creates a single aggregate
"Patrol Report YYYY-MM-DD" bead, then deletes the source digests.

The resulting digest bead is permanent (synced via git) and provides
an audit trail without per-cycle ephemeral pollution.

Examples:
  gt patrol digest --yesterday   # Digest yesterday's patrols (for daily patrol)
  gt patrol digest --date 2026-01-15
  gt patrol digest --yesterday --dry-run`,
	RunE: runPatrolDigest,
}

var patrolDutiesCmd = &cobra.Command{
	Use:     "duties",
	Aliases: []string{"list", "ls"},
	Short:   "List formulas assigned to the Overseer",
	RunE:    runPatrolDuties,
}

var patrolAssignCmd = &cobra.Command{
	Use:   "assign <formula>",
	Short: "Assign a formula to the Overseer's patrol duties",
	Long: `Assign a formula to the Overseer's patrol duties.

The formula must exist as an embedded formula (compiled into gt).
Use 'gt formulas' to see available formulas.

Examples:
  gt patrol assign mol-my-formula`,
	Args: cobra.ExactArgs(1),
	RunE: runPatrolAssign,
}

var patrolUnassignCmd = &cobra.Command{
	Use:     "unassign <formula>",
	Aliases: []string{"remove", "rm"},
	Short:   "Remove a formula from the Overseer's patrol duties",
	Args:    cobra.ExactArgs(1),
	RunE:    runPatrolUnassign,
}

func init() {
	patrolCmd.AddCommand(patrolDigestCmd)
	patrolCmd.AddCommand(patrolNewCmd)
	patrolCmd.AddCommand(patrolReportCmd)
	patrolCmd.AddCommand(patrolDutiesCmd)
	patrolCmd.AddCommand(patrolAssignCmd)
	patrolCmd.AddCommand(patrolUnassignCmd)
	rootCmd.AddCommand(patrolCmd)

	// Patrol digest flags
	patrolDigestCmd.Flags().BoolVar(&patrolDigestYesterday, "yesterday", false, "Digest yesterday's patrol cycles")
	patrolDigestCmd.Flags().StringVar(&patrolDigestDate, "date", "", "Digest patrol cycles for specific date (YYYY-MM-DD)")
	patrolDigestCmd.Flags().BoolVar(&patrolDigestDryRun, "dry-run", false, "Preview what would be created without creating")
	patrolDigestCmd.Flags().BoolVarP(&patrolDigestVerbose, "verbose", "v", false, "Verbose output")
}

// PatrolDigest represents the aggregated daily patrol report.
type PatrolDigest struct {
	Date         string                   `json:"date"`
	TotalCycles  int                      `json:"total_cycles"`
	ByRole       map[string]int           `json:"by_role"`        // deacon, witness, refinery
	Cycles       []PatrolCycleEntry       `json:"cycles"`
}

// PatrolCycleEntry represents a single patrol cycle in the digest.
type PatrolCycleEntry struct {
	ID          string    `json:"id"`
	Role        string    `json:"role"`         // deacon, witness, refinery
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
}

// runPatrolDigest aggregates patrol cycle digests into a daily digest bead.
func runPatrolDigest(cmd *cobra.Command, args []string) error {
	// Determine target date
	var targetDate time.Time

	if patrolDigestDate != "" {
		parsed, err := time.Parse("2006-01-02", patrolDigestDate)
		if err != nil {
			return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
		}
		targetDate = parsed
	} else if patrolDigestYesterday {
		targetDate = time.Now().AddDate(0, 0, -1)
	} else {
		return fmt.Errorf("specify --yesterday or --date YYYY-MM-DD")
	}

	dateStr := targetDate.Format("2006-01-02")

	// Idempotency check: see if digest already exists for this date
	existingID, err := findExistingPatrolDigest(dateStr)
	if err != nil {
		// Non-fatal: continue with creation attempt
		if patrolDigestVerbose {
			fmt.Fprintf(os.Stderr, "[patrol] warning: failed to check existing digest: %v\n", err)
		}
	} else if existingID != "" {
		fmt.Printf("%s Patrol digest already exists for %s (bead: %s)\n",
			style.Dim.Render("○"), dateStr, existingID)
		return nil
	}

	// Query ephemeral patrol digest beads for target date
	cycles, err := queryPatrolDigests(targetDate)
	if err != nil {
		return fmt.Errorf("querying patrol digests: %w", err)
	}

	if len(cycles) == 0 {
		fmt.Printf("%s No patrol digests found for %s\n", style.Dim.Render("○"), dateStr)
		return nil
	}

	// Build digest
	digest := PatrolDigest{
		Date:   dateStr,
		Cycles: cycles,
		ByRole: make(map[string]int),
	}

	for _, c := range cycles {
		digest.TotalCycles++
		digest.ByRole[c.Role]++
	}

	if patrolDigestDryRun {
		fmt.Printf("%s [DRY RUN] Would create Patrol Report %s:\n", style.Bold.Render("📊"), dateStr)
		fmt.Printf("  Total cycles: %d\n", digest.TotalCycles)
		fmt.Printf("  By Role:\n")
		roles := make([]string, 0, len(digest.ByRole))
		for role := range digest.ByRole {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			fmt.Printf("    %s: %d cycles\n", role, digest.ByRole[role])
		}
		return nil
	}

	// Create permanent digest bead
	digestID, err := createPatrolDigestBead(digest)
	if err != nil {
		return fmt.Errorf("creating digest bead: %w", err)
	}

	// Delete source digests (they're ephemeral)
	deletedCount, deleteErr := deletePatrolDigests(targetDate)
	if deleteErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to delete some source digests: %v\n", deleteErr)
	}

	fmt.Printf("%s Created Patrol Report %s (bead: %s)\n", style.Success.Render("✓"), dateStr, digestID)
	fmt.Printf("  Total: %d cycles\n", digest.TotalCycles)
	for role, count := range digest.ByRole {
		fmt.Printf("    %s: %d\n", role, count)
	}
	if deletedCount > 0 {
		fmt.Printf("  Deleted %d source digests\n", deletedCount)
	}

	return nil
}

// queryPatrolDigests queries ephemeral patrol digest beads for a target date.
func queryPatrolDigests(targetDate time.Time) ([]PatrolCycleEntry, error) {
	// List closed issues with "digest" label that are ephemeral
	// Patrol digests have titles like "Digest: mol-deacon-patrol", "Digest: mol-witness-patrol"
	listCmd := exec.Command("bd", "list",
		"--status=closed",
		"--label=digest",
		"--json",
		"--limit=0", // Get all
	)
	listOutput, err := listCmd.Output()
	if err != nil {
		if patrolDigestVerbose {
			fmt.Fprintf(os.Stderr, "[patrol] bd list failed: %v\n", err)
		}
		return nil, nil
	}

	var issues []struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
		ClosedAt    time.Time `json:"closed_at"`
		Ephemeral   bool      `json:"ephemeral"`
	}

	if err := json.Unmarshal(listOutput, &issues); err != nil {
		return nil, fmt.Errorf("parsing issue list: %w", err)
	}

	targetDay := targetDate.Format("2006-01-02")
	var patrolDigests []PatrolCycleEntry

	for _, issue := range issues {
		// Only process ephemeral patrol digests
		if !issue.Ephemeral {
			continue
		}

		// Must be a patrol digest (title starts with "Digest: mol-")
		if !strings.HasPrefix(issue.Title, "Digest: mol-") {
			continue
		}

		// Check if created on target date
		if issue.CreatedAt.Format("2006-01-02") != targetDay {
			continue
		}

		// Extract role from title (e.g., "Digest: mol-deacon-patrol" -> "deacon")
		role := extractPatrolRole(issue.Title)

		patrolDigests = append(patrolDigests, PatrolCycleEntry{
			ID:          issue.ID,
			Role:        role,
			Title:       issue.Title,
			Description: issue.Description,
			CreatedAt:   issue.CreatedAt,
			ClosedAt:    issue.ClosedAt,
		})
	}

	return patrolDigests, nil
}

// extractPatrolRole extracts the role from a patrol digest title.
// "Digest: mol-deacon-patrol" -> "deacon"
// "Digest: mol-witness-patrol" -> "witness"
// "Digest: gt-wisp-abc123" -> "unknown"
func extractPatrolRole(title string) string {
	// Remove "Digest: " prefix
	title = strings.TrimPrefix(title, "Digest: ")

	// Extract role from "mol-<role>-patrol" or "gt-wisp-<id>"
	if strings.HasPrefix(title, "mol-") && strings.HasSuffix(title, "-patrol") {
		// "mol-deacon-patrol" -> "deacon"
		role := strings.TrimPrefix(title, "mol-")
		role = strings.TrimSuffix(role, "-patrol")
		return role
	}

	// For wisp digests, try to extract from description or return generic
	return "patrol"
}

// createPatrolDigestBead creates a permanent bead for the daily patrol digest.
func createPatrolDigestBead(digest PatrolDigest) (string, error) {
	// Build description with aggregate data
	var desc strings.Builder
	desc.WriteString(fmt.Sprintf("Daily patrol aggregate for %s.\n\n", digest.Date))
	desc.WriteString(fmt.Sprintf("**Total Cycles:** %d\n\n", digest.TotalCycles))

	if len(digest.ByRole) > 0 {
		desc.WriteString("## By Role\n")
		roles := make([]string, 0, len(digest.ByRole))
		for role := range digest.ByRole {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			desc.WriteString(fmt.Sprintf("- %s: %d cycles\n", role, digest.ByRole[role]))
		}
		desc.WriteString("\n")
	}

	// Build payload JSON with cycle details
	payloadJSON, err := json.Marshal(digest)
	if err != nil {
		return "", fmt.Errorf("marshaling digest payload: %w", err)
	}

	// Create the digest bead (NOT ephemeral - this is permanent)
	title := fmt.Sprintf("Patrol Report %s", digest.Date)
	bdArgs := []string{
		"create",
		"--type=event",
		"--title=" + title,
		"--event-category=patrol.digest",
		"--event-payload=" + string(payloadJSON),
		"--description=" + desc.String(),
		"--silent",
	}

	bdCmd := exec.Command("bd", bdArgs...)
	output, err := bdCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating digest bead: %w\nOutput: %s", err, string(output))
	}

	digestID := strings.TrimSpace(string(output))

	// Auto-close the digest (it's an audit record, not work)
	closeCmd := exec.Command("bd", "close", digestID, "--reason=daily patrol digest")
	_ = closeCmd.Run() // Best effort

	return digestID, nil
}

// findExistingPatrolDigest checks if a patrol digest already exists for the given date.
// Returns the bead ID if found, empty string if not found.
func findExistingPatrolDigest(dateStr string) (string, error) {
	expectedTitle := fmt.Sprintf("Patrol Report %s", dateStr)

	// Query event beads with patrol.digest category
	listCmd := exec.Command("bd", "list",
		"--type=event",
		"--json",
		"--limit=50", // Recent events only
	)
	listOutput, err := listCmd.Output()
	if err != nil {
		return "", err
	}

	var events []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	if err := json.Unmarshal(listOutput, &events); err != nil {
		return "", err
	}

	for _, evt := range events {
		if evt.Title == expectedTitle {
			return evt.ID, nil
		}
	}

	return "", nil
}

// deletePatrolDigests deletes ephemeral patrol digest beads for a target date.
func deletePatrolDigests(targetDate time.Time) (int, error) {
	// Query patrol digests for the target date
	cycles, err := queryPatrolDigests(targetDate)
	if err != nil {
		return 0, err
	}

	if len(cycles) == 0 {
		return 0, nil
	}

	// Collect IDs to delete
	var idsToDelete []string
	for _, cycle := range cycles {
		idsToDelete = append(idsToDelete, cycle.ID)
	}

	// Delete in batch
	deleteArgs := append([]string{"delete", "--force"}, idsToDelete...)
	deleteCmd := exec.Command("bd", deleteArgs...)
	if err := deleteCmd.Run(); err != nil {
		return 0, fmt.Errorf("deleting patrol digests: %w", err)
	}

	return len(idsToDelete), nil
}

// --- Overseer duty management ---

// overseerDutyPath returns the path to the overseer duty config file.
func overseerDutyPath(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "overseer-patrol.json")
}

// loadOverseerDuties loads the overseer duty config.
func loadOverseerDuties(townRoot string) (*OverseerDutyConfig, error) {
	path := overseerDutyPath(townRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OverseerDutyConfig{}, nil
		}
		return nil, fmt.Errorf("reading duty config: %w", err)
	}

	var cfg OverseerDutyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing duty config: %w", err)
	}
	return &cfg, nil
}

// saveOverseerDuties saves the overseer duty config.
func saveOverseerDuties(townRoot string, cfg *OverseerDutyConfig) error {
	path := overseerDutyPath(townRoot)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling duty config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing duty config: %w", err)
	}
	return nil
}

func runPatrolDuties(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cfg, err := loadOverseerDuties(townRoot)
	if err != nil {
		return err
	}

	if len(cfg.Formulas) == 0 {
		fmt.Printf("%s No formulas assigned to the Overseer\n", style.Dim.Render("○"))
		fmt.Printf("  Add formulas with: %s\n", style.Dim.Render("gt patrol assign <formula>"))
		return nil
	}

	fmt.Printf("Overseer patrol formulas (%d):\n", len(cfg.Formulas))
	for _, f := range cfg.Formulas {
		fmt.Printf("  %s %s\n", style.Bold.Render("•"), f)
	}
	return nil
}

func runPatrolAssign(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Validate the formula exists
	if _, err := formula.GetEmbeddedFormulaContent(formulaName); err != nil {
		return fmt.Errorf("formula %q not found: %w\nUse 'gt formulas' to see available formulas", formulaName, err)
	}

	cfg, err := loadOverseerDuties(townRoot)
	if err != nil {
		return err
	}

	// Check for duplicates
	for _, f := range cfg.Formulas {
		if f == formulaName {
			fmt.Printf("%s Formula %q is already assigned to the Overseer\n",
				style.Dim.Render("○"), formulaName)
			return nil
		}
	}

	cfg.Formulas = append(cfg.Formulas, formulaName)
	sort.Strings(cfg.Formulas)

	if err := saveOverseerDuties(townRoot, cfg); err != nil {
		return err
	}

	fmt.Printf("%s Assigned formula %q to Overseer patrol\n",
		style.Bold.Render("✓"), formulaName)
	return nil
}

func runPatrolUnassign(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cfg, err := loadOverseerDuties(townRoot)
	if err != nil {
		return err
	}

	// Find and remove
	found := false
	var updated []string
	for _, f := range cfg.Formulas {
		if f == formulaName {
			found = true
		} else {
			updated = append(updated, f)
		}
	}

	if !found {
		return fmt.Errorf("formula %q is not assigned to the Overseer", formulaName)
	}

	cfg.Formulas = updated
	if err := saveOverseerDuties(townRoot, cfg); err != nil {
		return err
	}

	fmt.Printf("%s Removed formula %q from Overseer patrol\n",
		style.Bold.Render("✓"), formulaName)
	return nil
}
