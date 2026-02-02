package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tui/convoy"
	"github.com/steveyegge/gastown/internal/workspace"
)

// generateShortID generates a short random ID (5 lowercase chars).
func generateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

// looksLikeIssueID checks if a string looks like a beads issue ID.
// Issue IDs have the format: prefix-id (e.g., gt-abc, bd-xyz, hq-123).
func looksLikeIssueID(s string) bool {
	// Common beads prefixes
	prefixes := []string{"gt-", "bd-", "hq-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	// Also check for pattern: 2-3 lowercase letters followed by hyphen
	// This catches custom prefixes defined in routes.jsonl
	if len(s) >= 4 && s[2] == '-' || (len(s) >= 5 && s[3] == '-') {
		hyphenIdx := strings.Index(s, "-")
		if hyphenIdx >= 2 && hyphenIdx <= 3 {
			prefix := s[:hyphenIdx]
			// Check if prefix is all lowercase letters
			allLower := true
			for _, c := range prefix {
				if c < 'a' || c > 'z' {
					allLower = false
					break
				}
			}
			return allLower
		}
	}
	return false
}

// Convoy command flags
var (
	convoyMolecule     string
	convoyNotify       string
	convoyOwner        string
	convoyStatusJSON   bool
	convoyListJSON     bool
	convoyListStatus   string
	convoyListAll      bool
	convoyListTree     bool
	convoyInteractive  bool
	convoyStrandedJSON bool
	convoyCloseReason  string
	convoyCloseNotify  string
	convoyCheckDryRun  bool
)

var convoyCmd = &cobra.Command{
	Use:     "convoy",
	GroupID: GroupWork,
	Short:   "Track batches of work across rigs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if convoyInteractive {
			return runConvoyTUI()
		}
		return requireSubcommand(cmd, args)
	},
	Long: `Manage convoys - the primary unit for tracking batched work.

A convoy is a persistent tracking unit that monitors related issues across
rigs. When you kick off work (even a single issue), a convoy tracks it so
you can see when it lands and what was included.

WHAT IS A CONVOY:
  - Persistent tracking unit with an ID (hq-*)
  - Tracks issues across rigs (frontend+backend, beads+gastown, etc.)
  - Auto-closes when all tracked issues complete → notifies subscribers
  - Can be reopened by adding more issues

WHAT IS A SWARM:
  - Ephemeral: "the workers currently assigned to a convoy's issues"
  - No separate ID - uses the convoy ID
  - Dissolves when work completes

TRACKING SEMANTICS:
  - 'tracks' relation is non-blocking (tracked issues don't block convoy)
  - Cross-prefix capable (convoy in hq-* tracks issues in gt-*, bd-*)
  - Landed: all tracked issues closed → notification sent to subscribers

COMMANDS:
  create    Create a convoy tracking specified issues
  add       Add issues to an existing convoy (reopens if closed)
  close     Close a convoy (manually, regardless of tracked issue status)
  status    Show convoy progress, tracked issues, and active workers
  list      List convoys (the dashboard view)`,
}

var convoyCreateCmd = &cobra.Command{
	Use:   "create <name> [issues...]",
	Short: "Create a new convoy",
	Long: `Create a new convoy that tracks the specified issues.

The convoy is created in town-level beads (hq-* prefix) and can track
issues across any rig.

The --owner flag specifies who requested the convoy (receives completion
notification by default). If not specified, defaults to created_by.
The --notify flag adds additional subscribers beyond the owner.

Examples:
  gt convoy create "Deploy v2.0" gt-abc bd-xyz
  gt convoy create "Release prep" gt-abc --notify           # defaults to mayor/
  gt convoy create "Release prep" gt-abc --notify ops/      # notify ops/
  gt convoy create "Feature rollout" gt-a gt-b --owner mayor/ --notify ops/
  gt convoy create "Feature rollout" gt-a gt-b gt-c --molecule mol-release`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConvoyCreate,
}

var convoyStatusCmd = &cobra.Command{
	Use:   "status [convoy-id]",
	Short: "Show convoy status",
	Long: `Show detailed status for a convoy.

Displays convoy metadata, tracked issues, and completion progress.
Without an ID, shows status of all active convoys.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConvoyStatus,
}

var convoyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List convoys",
	Long: `List convoys, showing open convoys by default.

Examples:
  gt convoy list              # Open convoys only (default)
  gt convoy list --all        # All convoys (open + closed)
  gt convoy list --status=closed  # Recently landed
  gt convoy list --tree       # Show convoy + child status tree
  gt convoy list --json`,
	RunE: runConvoyList,
}

var convoyAddCmd = &cobra.Command{
	Use:   "add <convoy-id> <issue-id> [issue-id...]",
	Short: "Add issues to an existing convoy",
	Long: `Add issues to an existing convoy.

If the convoy is closed, it will be automatically reopened.

Examples:
  gt convoy add hq-cv-abc gt-new-issue
  gt convoy add hq-cv-abc gt-issue1 gt-issue2 gt-issue3`,
	Args: cobra.MinimumNArgs(2),
	RunE: runConvoyAdd,
}

var convoyCheckCmd = &cobra.Command{
	Use:   "check [convoy-id]",
	Short: "Check and auto-close completed convoys",
	Long: `Check convoys and auto-close any where all tracked issues are complete.

Without arguments, checks all open convoys. With a convoy ID, checks only that convoy.

This handles cross-rig convoy completion: convoys in town beads tracking issues
in rig beads won't auto-close via bd close alone. This command bridges that gap.

Can be run manually or by deacon patrol to ensure convoys close promptly.

Examples:
  gt convoy check              # Check all open convoys
  gt convoy check hq-cv-abc    # Check specific convoy
  gt convoy check --dry-run    # Preview what would close without acting`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConvoyCheck,
}

var convoyStrandedCmd = &cobra.Command{
	Use:   "stranded",
	Short: "Find stranded convoys with ready work but no workers",
	Long: `Find convoys that have ready issues but no workers processing them.

A convoy is "stranded" when:
- Convoy is open
- Has tracked issues where:
  - status = open (not in_progress, not closed)
  - not blocked (all dependencies met)
  - no assignee OR assignee session is dead

Use this to detect convoys that need feeding. The Deacon patrol runs this
periodically and dispatches dogs to feed stranded convoys.

Examples:
  gt convoy stranded              # Show stranded convoys
  gt convoy stranded --json       # Machine-readable output for automation`,
	RunE: runConvoyStranded,
}

var convoyCloseCmd = &cobra.Command{
	Use:   "close <convoy-id>",
	Short: "Close a convoy",
	Long: `Close a convoy, optionally with a reason.

Closes the convoy regardless of tracked issue status. Use this to:
- Force-close abandoned convoys no longer relevant
- Close convoys where work completed outside the tracked path
- Manually close stuck convoys

The close is idempotent - closing an already-closed convoy is a no-op.

Examples:
  gt convoy close hq-cv-abc
  gt convoy close hq-cv-abc --reason="work done differently"
  gt convoy close hq-cv-xyz --notify mayor/`,
	Args: cobra.ExactArgs(1),
	RunE: runConvoyClose,
}

func init() {
	// Create flags
	convoyCreateCmd.Flags().StringVar(&convoyMolecule, "molecule", "", "Associated molecule ID")
	convoyCreateCmd.Flags().StringVar(&convoyOwner, "owner", "", "Owner who requested convoy (gets completion notification)")
	convoyCreateCmd.Flags().StringVar(&convoyNotify, "notify", "", "Additional address to notify on completion (default: mayor/ if flag used without value)")
	convoyCreateCmd.Flags().Lookup("notify").NoOptDefVal = "mayor/"

	// Status flags
	convoyStatusCmd.Flags().BoolVar(&convoyStatusJSON, "json", false, "Output as JSON")

	// List flags
	convoyListCmd.Flags().BoolVar(&convoyListJSON, "json", false, "Output as JSON")
	convoyListCmd.Flags().StringVar(&convoyListStatus, "status", "", "Filter by status (open, closed)")
	convoyListCmd.Flags().BoolVar(&convoyListAll, "all", false, "Show all convoys (open and closed)")
	convoyListCmd.Flags().BoolVar(&convoyListTree, "tree", false, "Show convoy + child status tree")

	// Interactive TUI flag (on parent command)
	convoyCmd.Flags().BoolVarP(&convoyInteractive, "interactive", "i", false, "Interactive tree view")

	// Check flags
	convoyCheckCmd.Flags().BoolVar(&convoyCheckDryRun, "dry-run", false, "Preview what would close without acting")

	// Stranded flags
	convoyStrandedCmd.Flags().BoolVar(&convoyStrandedJSON, "json", false, "Output as JSON")

	// Close flags
	convoyCloseCmd.Flags().StringVar(&convoyCloseReason, "reason", "", "Reason for closing the convoy")
	convoyCloseCmd.Flags().StringVar(&convoyCloseNotify, "notify", "", "Agent to notify on close (e.g., mayor/)")

	// Add subcommands
	convoyCmd.AddCommand(convoyCreateCmd)
	convoyCmd.AddCommand(convoyStatusCmd)
	convoyCmd.AddCommand(convoyListCmd)
	convoyCmd.AddCommand(convoyAddCmd)
	convoyCmd.AddCommand(convoyCheckCmd)
	convoyCmd.AddCommand(convoyStrandedCmd)
	convoyCmd.AddCommand(convoyCloseCmd)

	rootCmd.AddCommand(convoyCmd)
}

// getTownBeadsDir returns the path to town-level beads directory.
func getTownBeadsDir() (string, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	return filepath.Join(townRoot, ".beads"), nil
}

func runConvoyCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	trackedIssues := args[1:]

	// If first arg looks like an issue ID (has beads prefix), treat all args as issues
	// and auto-generate a name from the first issue's title
	if looksLikeIssueID(name) {
		trackedIssues = args // All args are issue IDs
		// Get the first issue's title to use as convoy name
		if details := getIssueDetails(args[0]); details != nil && details.Title != "" {
			name = details.Title
		} else {
			name = fmt.Sprintf("Tracking %s", args[0])
		}
	}

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Ensure custom types (including 'convoy') are registered in town beads.
	// This handles cases where install didn't complete or beads was initialized manually.
	if err := beads.EnsureCustomTypes(townBeads); err != nil {
		return fmt.Errorf("ensuring custom types: %w", err)
	}

	// Create convoy issue in town beads
	description := fmt.Sprintf("Convoy tracking %d issues", len(trackedIssues))

	// Default owner to creator identity if not specified
	owner := convoyOwner
	if owner == "" {
		owner = detectSender()
	}
	if owner != "" {
		description += fmt.Sprintf("\nOwner: %s", owner)
	}
	if convoyNotify != "" {
		description += fmt.Sprintf("\nNotify: %s", convoyNotify)
	}
	if convoyMolecule != "" {
		description += fmt.Sprintf("\nMolecule: %s", convoyMolecule)
	}

	// Generate convoy ID with cv- prefix
	convoyID := fmt.Sprintf("hq-cv-%s", generateShortID())

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + name,
		"--description=" + description,
		"--json",
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeads
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	createCmd.Stdout = &stdout
	createCmd.Stderr = &stderr

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("creating convoy: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	// Notify address is stored in description (line 166-168) and read from there

	// Add 'tracks' relations for each tracked issue
	trackedCount := 0
	for _, issueID := range trackedIssues {
		// Use --type=tracks for non-blocking tracking relation
		depArgs := []string{"dep", "add", convoyID, issueID, "--type=tracks"}
		depCmd := exec.Command("bd", depArgs...)
		depCmd.Dir = townBeads
		var depStderr bytes.Buffer
		depCmd.Stderr = &depStderr

		if err := depCmd.Run(); err != nil {
			errMsg := strings.TrimSpace(depStderr.String())
			if errMsg == "" {
				errMsg = err.Error()
			}
			style.PrintWarning("couldn't track %s: %s", issueID, errMsg)
		} else {
			trackedCount++
		}
	}

	// Output
	fmt.Printf("%s Created convoy 🚚 %s\n\n", style.Bold.Render("✓"), convoyID)
	fmt.Printf("  Name:     %s\n", name)
	fmt.Printf("  Tracking: %d issues\n", trackedCount)
	if len(trackedIssues) > 0 {
		fmt.Printf("  Issues:   %s\n", strings.Join(trackedIssues, ", "))
	}
	if owner != "" {
		fmt.Printf("  Owner:    %s\n", owner)
	}
	if convoyNotify != "" {
		fmt.Printf("  Notify:   %s\n", convoyNotify)
	}
	if convoyMolecule != "" {
		fmt.Printf("  Molecule: %s\n", convoyMolecule)
	}

	fmt.Printf("\n  %s\n", style.Dim.Render("Convoy auto-closes when all tracked issues complete"))

	return nil
}

func runConvoyAdd(cmd *cobra.Command, args []string) error {
	convoyID := args[0]
	issuesToAdd := args[1:]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Validate convoy exists and get its status
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	var convoys []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Type   string `json:"issue_type"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy data: %w", err)
	}

	if len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]

	// Verify it's actually a convoy type
	if convoy.Type != "convoy" {
		return fmt.Errorf("'%s' is not a convoy (type: %s)", convoyID, convoy.Type)
	}

	// If convoy is closed, reopen it
	reopened := false
	if convoy.Status == "closed" {
		reopenArgs := []string{"update", convoyID, "--status=open"}
		reopenCmd := exec.Command("bd", reopenArgs...)
		reopenCmd.Dir = townBeads
		if err := reopenCmd.Run(); err != nil {
			return fmt.Errorf("couldn't reopen convoy: %w", err)
		}
		reopened = true
		fmt.Printf("%s Reopened convoy %s\n", style.Bold.Render("↺"), convoyID)
	}

	// Add 'tracks' relations for each issue
	addedCount := 0
	for _, issueID := range issuesToAdd {
		depArgs := []string{"dep", "add", convoyID, issueID, "--type=tracks"}
		depCmd := exec.Command("bd", depArgs...)
		depCmd.Dir = townBeads
		var depStderr bytes.Buffer
		depCmd.Stderr = &depStderr

		if err := depCmd.Run(); err != nil {
			errMsg := strings.TrimSpace(depStderr.String())
			if errMsg == "" {
				errMsg = err.Error()
			}
			style.PrintWarning("couldn't add %s: %s", issueID, errMsg)
		} else {
			addedCount++
		}
	}

	// Output
	if reopened {
		fmt.Println()
	}
	fmt.Printf("%s Added %d issue(s) to convoy 🚚 %s\n", style.Bold.Render("✓"), addedCount, convoyID)
	if addedCount > 0 {
		fmt.Printf("  Issues: %s\n", strings.Join(issuesToAdd[:addedCount], ", "))
	}

	return nil
}

func runConvoyCheck(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// If a specific convoy ID is provided, check only that convoy
	if len(args) == 1 {
		convoyID := args[0]
		return checkSingleConvoy(townBeads, convoyID, convoyCheckDryRun)
	}

	// Check all open convoys
	closed, err := checkAndCloseCompletedConvoys(townBeads, convoyCheckDryRun)
	if err != nil {
		return err
	}

	if len(closed) == 0 {
		fmt.Println("No convoys ready to close.")
	} else {
		if convoyCheckDryRun {
			fmt.Printf("%s Would auto-close %d convoy(s):\n", style.Warning.Render("⚠"), len(closed))
		} else {
			fmt.Printf("%s Auto-closed %d convoy(s):\n", style.Bold.Render("✓"), len(closed))
		}
		for _, c := range closed {
			fmt.Printf("  🚚 %s: %s\n", c.ID, c.Title)
		}
	}

	return nil
}

// checkSingleConvoy checks a specific convoy and closes it if all tracked issues are complete.
func checkSingleConvoy(townBeads, convoyID string, dryRun bool) error {
	// Get convoy details
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	var convoys []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Type        string `json:"issue_type"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy data: %w", err)
	}

	if len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]

	// Verify it's actually a convoy type
	if convoy.Type != "convoy" {
		return fmt.Errorf("'%s' is not a convoy (type: %s)", convoyID, convoy.Type)
	}

	// Check if convoy is already closed
	if convoy.Status == "closed" {
		fmt.Printf("%s Convoy %s is already closed\n", style.Dim.Render("○"), convoyID)
		return nil
	}

	// Get tracked issues
	tracked := getTrackedIssues(townBeads, convoyID)
	if len(tracked) == 0 {
		fmt.Printf("%s Convoy %s has no tracked issues\n", style.Dim.Render("○"), convoyID)
		return nil
	}

	// Check if all tracked issues are closed
	allClosed := true
	openCount := 0
	for _, t := range tracked {
		if t.Status != "closed" && t.Status != "tombstone" {
			allClosed = false
			openCount++
		}
	}

	if !allClosed {
		fmt.Printf("%s Convoy %s has %d open issue(s) remaining\n", style.Dim.Render("○"), convoyID, openCount)
		return nil
	}

	// All tracked issues are complete - close the convoy
	if dryRun {
		fmt.Printf("%s Would auto-close convoy 🚚 %s: %s\n", style.Warning.Render("⚠"), convoyID, convoy.Title)
		return nil
	}

	// Actually close the convoy
	closeArgs := []string{"close", convoyID, "-r", "All tracked issues completed"}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = townBeads

	if err := closeCmd.Run(); err != nil {
		return fmt.Errorf("closing convoy: %w", err)
	}

	fmt.Printf("%s Auto-closed convoy 🚚 %s: %s\n", style.Bold.Render("✓"), convoyID, convoy.Title)

	// Send completion notification
	notifyConvoyCompletion(townBeads, convoyID, convoy.Title)

	return nil
}

func runConvoyClose(cmd *cobra.Command, args []string) error {
	convoyID := args[0]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Get convoy details
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	var convoys []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Type        string `json:"issue_type"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy data: %w", err)
	}

	if len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]

	// Verify it's actually a convoy type
	if convoy.Type != "convoy" {
		return fmt.Errorf("'%s' is not a convoy (type: %s)", convoyID, convoy.Type)
	}

	// Idempotent: if already closed, just report it
	if convoy.Status == "closed" {
		fmt.Printf("%s Convoy %s is already closed\n", style.Dim.Render("○"), convoyID)
		return nil
	}

	// Build close reason
	reason := convoyCloseReason
	if reason == "" {
		reason = "Manually closed"
	}

	// Close the convoy
	closeArgs := []string{"close", convoyID, "-r", reason}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = townBeads

	if err := closeCmd.Run(); err != nil {
		return fmt.Errorf("closing convoy: %w", err)
	}

	fmt.Printf("%s Closed convoy 🚚 %s: %s\n", style.Bold.Render("✓"), convoyID, convoy.Title)
	if convoyCloseReason != "" {
		fmt.Printf("  Reason: %s\n", convoyCloseReason)
	}

	// Send notification if --notify flag provided
	if convoyCloseNotify != "" {
		sendCloseNotification(convoyCloseNotify, convoyID, convoy.Title, reason)
	} else {
		// Check if convoy has a notify address in description
		notifyConvoyCompletion(townBeads, convoyID, convoy.Title)
	}

	return nil
}

// sendCloseNotification sends a notification about convoy closure.
func sendCloseNotification(addr, convoyID, title, reason string) {
	subject := fmt.Sprintf("🚚 Convoy closed: %s", title)
	body := fmt.Sprintf("Convoy %s has been closed.\n\nReason: %s", convoyID, reason)

	mailArgs := []string{"mail", "send", addr, "-s", subject, "-m", body}
	mailCmd := exec.Command("gt", mailArgs...)
	if err := mailCmd.Run(); err != nil {
		style.PrintWarning("couldn't send notification: %v", err)
	} else {
		fmt.Printf("  Notified: %s\n", addr)
	}
}

// strandedConvoyInfo holds info about a stranded convoy.
type strandedConvoyInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ReadyCount  int      `json:"ready_count"`
	ReadyIssues []string `json:"ready_issues"`
}

// readyIssueInfo holds info about a ready (stranded) issue.
type readyIssueInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority string `json:"priority"`
}

func runConvoyStranded(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	stranded, err := findStrandedConvoys(townBeads)
	if err != nil {
		return err
	}

	if convoyStrandedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stranded)
	}

	if len(stranded) == 0 {
		fmt.Println("No stranded convoys found.")
		return nil
	}

	fmt.Printf("%s Found %d stranded convoy(s):\n\n", style.Warning.Render("⚠"), len(stranded))
	for _, s := range stranded {
		fmt.Printf("  🚚 %s: %s\n", s.ID, s.Title)
		fmt.Printf("     Ready issues: %d\n", s.ReadyCount)
		for _, issueID := range s.ReadyIssues {
			fmt.Printf("       • %s\n", issueID)
		}
		fmt.Println()
	}

	fmt.Println("To feed stranded convoys, run:")
	for _, s := range stranded {
		fmt.Printf("  gt sling mol-convoy-feed deacon/dogs --var convoy=%s\n", s.ID)
	}

	return nil
}

// findStrandedConvoys finds convoys with ready work but no workers.
func findStrandedConvoys(townBeads string) ([]strandedConvoyInfo, error) {
	var stranded []strandedConvoyInfo

	// Get blocked issues (we need this to filter out blocked issues)
	blockedIssues := getBlockedIssueIDs()

	// List all open convoys
	listArgs := []string{"list", "--type=convoy", "--status=open", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return nil, fmt.Errorf("parsing convoy list: %w", err)
	}

	// Check each convoy for stranded state
	for _, convoy := range convoys {
		tracked := getTrackedIssues(townBeads, convoy.ID)
		if len(tracked) == 0 {
			continue
		}

		// Find ready issues (open, not blocked, no live assignee)
		var readyIssues []string
		for _, t := range tracked {
			if isReadyIssue(t, blockedIssues) {
				readyIssues = append(readyIssues, t.ID)
			}
		}

		if len(readyIssues) > 0 {
			stranded = append(stranded, strandedConvoyInfo{
				ID:          convoy.ID,
				Title:       convoy.Title,
				ReadyCount:  len(readyIssues),
				ReadyIssues: readyIssues,
			})
		}
	}

	return stranded, nil
}

// getBlockedIssueIDs returns a set of issue IDs that are currently blocked.
func getBlockedIssueIDs() map[string]bool {
	blocked := make(map[string]bool)

	// Run bd blocked --json
	blockedCmd := exec.Command("bd", "blocked", "--json")
	var stdout bytes.Buffer
	blockedCmd.Stdout = &stdout

	if err := blockedCmd.Run(); err != nil {
		return blocked // Return empty set on error
	}

	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return blocked
	}

	for _, issue := range issues {
		blocked[issue.ID] = true
	}

	return blocked
}

// isReadyIssue checks if an issue is ready for dispatch (stranded).
// An issue is ready if:
// - status = "open" (not in_progress, closed, hooked)
// - not in blocked set
// - no assignee OR assignee session is dead
func isReadyIssue(t trackedIssueInfo, blockedIssues map[string]bool) bool {
	// Must be open status (not in_progress, closed, hooked)
	if t.Status != "open" {
		return false
	}

	// Must not be blocked
	if blockedIssues[t.ID] {
		return false
	}

	// Check assignee
	if t.Assignee == "" {
		return true // No assignee = ready
	}

	// Has assignee - check if session is alive
	// Use the shared assigneeToSessionName from rig.go
	sessionName, _ := assigneeToSessionName(t.Assignee)
	if sessionName == "" {
		return true // Can't determine session = treat as ready
	}

	// Check if tmux session exists
	checkCmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := checkCmd.Run(); err != nil {
		return true // Session doesn't exist = ready
	}

	return false // Session exists = not ready (worker is active)
}

// checkAndCloseCompletedConvoys finds open convoys where all tracked issues are closed
// and auto-closes them. Returns the list of convoys that were closed (or would be closed in dry-run mode).
// If dryRun is true, no changes are made and the function returns what would have been closed.
func checkAndCloseCompletedConvoys(townBeads string, dryRun bool) ([]struct{ ID, Title string }, error) {
	var closed []struct{ ID, Title string }

	// List all open convoys
	listArgs := []string{"list", "--type=convoy", "--status=open", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return nil, fmt.Errorf("parsing convoy list: %w", err)
	}

	// Check each convoy
	for _, convoy := range convoys {
		tracked := getTrackedIssues(townBeads, convoy.ID)
		if len(tracked) == 0 {
			continue // No tracked issues, nothing to check
		}

		// Check if all tracked issues are closed
		allClosed := true
		for _, t := range tracked {
			if t.Status != "closed" && t.Status != "tombstone" {
				allClosed = false
				break
			}
		}

		if allClosed {
			if dryRun {
				// In dry-run mode, just record what would be closed
				closed = append(closed, struct{ ID, Title string }{convoy.ID, convoy.Title})
				continue
			}

			// Close the convoy
			closeArgs := []string{"close", convoy.ID, "-r", "All tracked issues completed"}
			closeCmd := exec.Command("bd", closeArgs...)
			closeCmd.Dir = townBeads

			if err := closeCmd.Run(); err != nil {
				style.PrintWarning("couldn't close convoy %s: %v", convoy.ID, err)
				continue
			}

			closed = append(closed, struct{ ID, Title string }{convoy.ID, convoy.Title})

			// Check if convoy has notify address and send notification
			notifyConvoyCompletion(townBeads, convoy.ID, convoy.Title)
		}
	}

	return closed, nil
}

// notifyConvoyCompletion sends notifications to owner and any notify addresses.
func notifyConvoyCompletion(townBeads, convoyID, title string) {
	// Get convoy description to find owner and notify addresses
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return
	}

	var convoys []struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil || len(convoys) == 0 {
		return
	}

	// Parse owner and notify addresses from description
	desc := convoys[0].Description
	notified := make(map[string]bool) // Track who we've notified to avoid duplicates

	for _, line := range strings.Split(desc, "\n") {
		var addr string
		if strings.HasPrefix(line, "Owner: ") {
			addr = strings.TrimPrefix(line, "Owner: ")
		} else if strings.HasPrefix(line, "Notify: ") {
			addr = strings.TrimPrefix(line, "Notify: ")
		}

		if addr != "" && !notified[addr] {
			// Send notification via gt mail
			mailArgs := []string{"mail", "send", addr,
				"-s", fmt.Sprintf("🚚 Convoy landed: %s", title),
				"-m", fmt.Sprintf("Convoy %s has completed.\n\nAll tracked issues are now closed.", convoyID)}
			mailCmd := exec.Command("gt", mailArgs...)
			_ = mailCmd.Run() // Best effort, ignore errors
			notified[addr] = true
		}
	}
}

func runConvoyStatus(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// If no ID provided, show all active convoys
	if len(args) == 0 {
		return showAllConvoyStatus(townBeads)
	}

	convoyID := args[0]

	// Check if it's a numeric shortcut (e.g., "1" instead of "hq-cv-xyz")
	if n, err := strconv.Atoi(convoyID); err == nil && n > 0 {
		resolved, err := resolveConvoyNumber(townBeads, n)
		if err != nil {
			return err
		}
		convoyID = resolved
	}

	// Get convoy details
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	// Parse convoy data
	var convoys []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Description string   `json:"description"`
		CreatedAt   string   `json:"created_at"`
		ClosedAt    string   `json:"closed_at,omitempty"`
		DependsOn   []string `json:"depends_on,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy data: %w", err)
	}

	if len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]

	// Get tracked issues by querying SQLite directly
	// (bd dep list doesn't properly show cross-rig external dependencies)
	type trackedIssue struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Type      string `json:"dependency_type"`
		IssueType string `json:"issue_type"`
	}

	tracked := getTrackedIssues(townBeads, convoyID)

	// Count completed
	completed := 0
	for _, t := range tracked {
		if t.Status == "closed" {
			completed++
		}
	}

	if convoyStatusJSON {
		type jsonStatus struct {
			ID        string             `json:"id"`
			Title     string             `json:"title"`
			Status    string             `json:"status"`
			Tracked   []trackedIssueInfo `json:"tracked"`
			Completed int                `json:"completed"`
			Total     int                `json:"total"`
		}
		out := jsonStatus{
			ID:        convoy.ID,
			Title:     convoy.Title,
			Status:    convoy.Status,
			Tracked:   tracked,
			Completed: completed,
			Total:     len(tracked),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Human-readable output
	fmt.Printf("🚚 %s %s\n\n", style.Bold.Render(convoy.ID+":"), convoy.Title)
	fmt.Printf("  Status:    %s\n", formatConvoyStatus(convoy.Status))
	fmt.Printf("  Progress:  %d/%d completed\n", completed, len(tracked))
	fmt.Printf("  Created:   %s\n", convoy.CreatedAt)
	if convoy.ClosedAt != "" {
		fmt.Printf("  Closed:    %s\n", convoy.ClosedAt)
	}

	if len(tracked) > 0 {
		fmt.Printf("\n  %s\n", style.Bold.Render("Tracked Issues:"))
		for _, t := range tracked {
			// Status symbol: ✓ closed, ▶ in_progress/hooked, ○ other
			status := "○"
			switch t.Status {
			case "closed":
				status = "✓"
			case "in_progress", "hooked":
				status = "▶"
			}

			// Show assignee in brackets (extract short name from path like gastown/polecats/goose -> goose)
			bracketContent := t.IssueType
			if t.Assignee != "" {
				parts := strings.Split(t.Assignee, "/")
				bracketContent = parts[len(parts)-1] // Last part of path
			} else if bracketContent == "" {
				bracketContent = "unassigned"
			}

			line := fmt.Sprintf("    %s %s: %s [%s]", status, t.ID, t.Title, bracketContent)
			if t.Worker != "" {
				workerDisplay := "@" + t.Worker
				if t.WorkerAge != "" {
					workerDisplay += fmt.Sprintf(" (%s)", t.WorkerAge)
				}
				line += fmt.Sprintf("  %s", style.Dim.Render(workerDisplay))
			}
			fmt.Println(line)
		}
	}

	return nil
}

func showAllConvoyStatus(townBeads string) error {
	// List all convoy-type issues
	listArgs := []string{"list", "--type=convoy", "--status=open", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy list: %w", err)
	}

	if len(convoys) == 0 {
		fmt.Println("No active convoys.")
		fmt.Println("Create a convoy with: gt convoy create <name> [issues...]")
		return nil
	}

	if convoyStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(convoys)
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Active Convoys"))
	for _, c := range convoys {
		fmt.Printf("  🚚 %s: %s\n", c.ID, c.Title)
	}
	fmt.Printf("\nUse 'gt convoy status <id>' for detailed status.\n")

	return nil
}

func runConvoyList(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// List convoy-type issues
	listArgs := []string{"list", "--type=convoy", "--json"}
	if convoyListStatus != "" {
		listArgs = append(listArgs, "--status="+convoyListStatus)
	} else if convoyListAll {
		listArgs = append(listArgs, "--all")
	}
	// Default (no flags) = open only (bd's default behavior)

	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return fmt.Errorf("parsing convoy list: %w", err)
	}

	if convoyListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(convoys)
	}

	if len(convoys) == 0 {
		fmt.Println("No convoys found.")
		fmt.Println("Create a convoy with: gt convoy create <name> [issues...]")
		return nil
	}

	// Tree view: show convoys with their child issues
	if convoyListTree {
		return printConvoyTree(townBeads, convoys)
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Convoys"))
	for i, c := range convoys {
		status := formatConvoyStatus(c.Status)
		fmt.Printf("  %d. 🚚 %s: %s %s\n", i+1, c.ID, c.Title, status)
	}
	fmt.Printf("\nUse 'gt convoy status <id>' or 'gt convoy status <n>' for detailed view.\n")

	return nil
}

// printConvoyTree displays convoys with their child issues in a tree format.
func printConvoyTree(townBeads string, convoys []struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}) error {
	for _, c := range convoys {
		// Get tracked issues for this convoy
		tracked := getTrackedIssues(townBeads, c.ID)

		// Count completed
		completed := 0
		for _, t := range tracked {
			if t.Status == "closed" {
				completed++
			}
		}

		// Print convoy header with progress
		total := len(tracked)
		progress := ""
		if total > 0 {
			progress = fmt.Sprintf(" (%d/%d)", completed, total)
		}
		fmt.Printf("🚚 %s: %s%s\n", c.ID, c.Title, progress)

		// Print tracked issues as tree children
		for i, t := range tracked {
			// Determine tree connector
			isLast := i == len(tracked)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}

			// Status symbol: ✓ closed, ▶ in_progress/hooked, ○ other
			status := "○"
			switch t.Status {
			case "closed":
				status = "✓"
			case "in_progress", "hooked":
				status = "▶"
			}

			fmt.Printf("%s %s %s: %s\n", connector, status, t.ID, t.Title)
		}

		// Add blank line between convoys
		fmt.Println()
	}

	return nil
}

func formatConvoyStatus(status string) string {
	switch status {
	case "open":
		return style.Warning.Render("●")
	case "closed":
		return style.Success.Render("✓")
	case "in_progress":
		return style.Info.Render("→")
	default:
		return status
	}
}

// trackedIssueInfo holds info about an issue being tracked by a convoy.
type trackedIssueInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Type      string `json:"dependency_type"`
	IssueType string `json:"issue_type"`
	Assignee  string `json:"assignee,omitempty"`   // Assigned agent (e.g., gastown/polecats/goose)
	Worker    string `json:"worker,omitempty"`     // Worker currently assigned (e.g., gastown/nux)
	WorkerAge string `json:"worker_age,omitempty"` // How long worker has been on this issue
}

// getTrackedIssues uses bd dep list to get issues tracked by a convoy.
// Returns issue details including status, type, and worker info.
func getTrackedIssues(townBeads, convoyID string) []trackedIssueInfo {
	// Use bd dep list to get tracked dependencies
	// Run from town root (parent of .beads) so bd routes correctly
	townRoot := filepath.Dir(townBeads)
	depCmd := exec.Command("bd", "--no-daemon", "dep", "list", convoyID, "--direction=down", "--type=tracks", "--json")
	depCmd.Dir = townRoot

	var stdout bytes.Buffer
	depCmd.Stdout = &stdout
	if err := depCmd.Run(); err != nil {
		return nil
	}

	// Parse the JSON output - bd dep list returns full issue details
	var deps []struct {
		ID             string   `json:"id"`
		Title          string   `json:"title"`
		Status         string   `json:"status"`
		IssueType      string   `json:"issue_type"`
		Assignee       string   `json:"assignee"`
		DependencyType string   `json:"dependency_type"`
		Labels         []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &deps); err != nil {
		return nil
	}

	// Collect non-closed issue IDs for worker lookup
	openIssueIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		if dep.Status != "closed" {
			openIssueIDs = append(openIssueIDs, dep.ID)
		}
	}
	workersMap := getWorkersForIssues(openIssueIDs)

	// Build result
	var tracked []trackedIssueInfo
	for _, dep := range deps {
		info := trackedIssueInfo{
			ID:        dep.ID,
			Title:     dep.Title,
			Status:    dep.Status,
			Type:      dep.DependencyType,
			IssueType: dep.IssueType,
			Assignee:  dep.Assignee,
		}

		// Add worker info if available
		if worker, ok := workersMap[dep.ID]; ok {
			info.Worker = worker.Worker
			info.WorkerAge = worker.Age
		}

		tracked = append(tracked, info)
	}

	return tracked
}

// getExternalIssueDetails fetches issue details from an external rig database.
// townBeads: path to town .beads directory
// rigName: name of the rig (e.g., "claycantrell")
// issueID: the issue ID to look up
func getExternalIssueDetails(townBeads, rigName, issueID string) *issueDetails {
	// Resolve rig directory path: town parent + rig name
	townParent := filepath.Dir(townBeads)
	rigDir := filepath.Join(townParent, rigName)

	// Check if rig directory exists
	if _, err := os.Stat(rigDir); os.IsNotExist(err) {
		return nil
	}

	// Query the rig database by running bd show from the rig directory
	// Use --allow-stale to handle cases where JSONL and DB are out of sync
	showCmd := exec.Command("bd", "--no-daemon", "show", issueID, "--json", "--allow-stale")
	showCmd.Dir = rigDir // Set working directory to rig directory
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return nil
	}
	if stdout.Len() == 0 {
		return nil
	}

	var issues []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		IssueType string `json:"issue_type"`
		Assignee  string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil
	}
	if len(issues) == 0 {
		return nil
	}

	issue := issues[0]
	return &issueDetails{
		ID:        issue.ID,
		Title:     issue.Title,
		Status:    issue.Status,
		IssueType: issue.IssueType,
		Assignee:  issue.Assignee,
	}
}

// issueDetails holds basic issue info.
type issueDetails struct {
	ID        string
	Title     string
	Status    string
	IssueType string
	Assignee  string
}

// getIssueDetailsBatch fetches details for multiple issues in a single bd show call.
// Returns a map from issue ID to details. Missing/invalid issues are omitted from the map.
func getIssueDetailsBatch(issueIDs []string) map[string]*issueDetails {
	result := make(map[string]*issueDetails)
	if len(issueIDs) == 0 {
		return result
	}

	// Build args: bd --no-daemon show id1 id2 id3 ... --json
	// Use --no-daemon to ensure fresh data (avoid stale cache from daemon)
	args := append([]string{"--no-daemon", "show"}, issueIDs...)
	args = append(args, "--json")

	showCmd := exec.Command("bd", args...)
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		// Batch failed - fall back to individual lookups for robustness
		// This handles cases where some IDs are invalid/missing
		for _, id := range issueIDs {
			if details := getIssueDetails(id); details != nil {
				result[id] = details
			}
		}
		return result
	}

	var issues []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		IssueType string `json:"issue_type"`
		Assignee  string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return result
	}

	for _, issue := range issues {
		result[issue.ID] = &issueDetails{
			ID:        issue.ID,
			Title:     issue.Title,
			Status:    issue.Status,
			IssueType: issue.IssueType,
			Assignee:  issue.Assignee,
		}
	}

	return result
}

// getIssueDetails fetches issue details by trying to show it via bd.
// Prefer getIssueDetailsBatch for multiple issues to avoid N+1 subprocess calls.
func getIssueDetails(issueID string) *issueDetails {
	// Use bd show with routing - it should find the issue in the right rig
	// Use --no-daemon to ensure fresh data (avoid stale cache)
	showCmd := exec.Command("bd", "--no-daemon", "show", issueID, "--json")
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return nil
	}
	// Handle bd --no-daemon exit 0 bug: empty stdout means not found
	if stdout.Len() == 0 {
		return nil
	}

	var issues []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		IssueType string `json:"issue_type"`
		Assignee  string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil || len(issues) == 0 {
		return nil
	}

	return &issueDetails{
		ID:        issues[0].ID,
		Title:     issues[0].Title,
		Status:    issues[0].Status,
		IssueType: issues[0].IssueType,
		Assignee:  issues[0].Assignee,
	}
}

// workerInfo holds info about a worker assigned to an issue.
type workerInfo struct {
	Worker string // Agent identity (e.g., gastown/nux)
	Age    string // How long assigned (e.g., "12m")
}

// getWorkersForIssues finds workers currently assigned to the given issues.
// Returns a map from issue ID to worker info.
//
// Optimized to batch queries per rig (O(R) instead of O(N×R)) and
// parallelize across rigs.
func getWorkersForIssues(issueIDs []string) map[string]*workerInfo {
	result := make(map[string]*workerInfo)
	if len(issueIDs) == 0 {
		return result
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return result
	}

	// Build a set of target issue IDs for fast lookup
	targetIDs := make(map[string]bool, len(issueIDs))
	for _, id := range issueIDs {
		targetIDs[id] = true
	}

	// Discover rigs with beads directories
	rigDirs, _ := filepath.Glob(filepath.Join(townRoot, "*", "polecats"))
	var beadsDirs []string
	for _, polecatsDir := range rigDirs {
		rigDir := filepath.Dir(polecatsDir)
		beadsDir := filepath.Join(rigDir, "mayor", "rig", ".beads")
		if info, err := os.Stat(beadsDir); err == nil && info.IsDir() {
			beadsDirs = append(beadsDirs, filepath.Join(rigDir, "mayor", "rig"))
		}
	}

	if len(beadsDirs) == 0 {
		return result
	}

	// Query all rigs in parallel using bd list
	type rigResult struct {
		agents []struct {
			ID           string `json:"id"`
			HookBead     string `json:"hook_bead"`
			LastActivity string `json:"last_activity"`
		}
	}

	resultChan := make(chan rigResult, len(beadsDirs))
	var wg sync.WaitGroup

	for _, dir := range beadsDirs {
		wg.Add(1)
		go func(beadsDir string) {
			defer wg.Done()

			cmd := exec.Command("bd", "list", "--type=agent", "--status=open", "--json", "--limit=0")
			cmd.Dir = beadsDir
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Run(); err != nil {
				resultChan <- rigResult{}
				return
			}

			var rr rigResult
			if err := json.Unmarshal(stdout.Bytes(), &rr.agents); err != nil {
				resultChan <- rigResult{}
				return
			}
			resultChan <- rr
		}(dir)
	}

	// Wait for all queries to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results from all rigs, filtering by target issue IDs
	for rr := range resultChan {
		for _, agent := range rr.agents {
			// Only include agents working on issues we care about
			if !targetIDs[agent.HookBead] {
				continue
			}

			// Skip if we already found a worker for this issue
			if _, ok := result[agent.HookBead]; ok {
				continue
			}

			// Parse agent ID to get worker identity
			workerID := parseWorkerFromAgentBead(agent.ID)
			if workerID == "" {
				continue
			}

			// Calculate age from last_activity
			age := ""
			if agent.LastActivity != "" {
				if t, err := time.Parse(time.RFC3339, agent.LastActivity); err == nil {
					age = formatWorkerAge(time.Since(t))
				}
			}

			result[agent.HookBead] = &workerInfo{
				Worker: workerID,
				Age:    age,
			}
		}
	}

	return result
}

// parseWorkerFromAgentBead extracts worker identity from agent bead ID.
// Input: "gt-gastown-polecat-nux" -> Output: "gastown/nux"
// Input: "gt-beads-crew-amber" -> Output: "beads/crew/amber"
func parseWorkerFromAgentBead(agentID string) string {
	// Remove prefix (gt-, bd-, etc.)
	parts := strings.Split(agentID, "-")
	if len(parts) < 3 {
		return ""
	}

	// Skip prefix
	parts = parts[1:]

	// Reconstruct as path
	return strings.Join(parts, "/")
}

// formatWorkerAge formats a duration as a short string (e.g., "5m", "2h", "1d")
func formatWorkerAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// runConvoyTUI launches the interactive convoy TUI.
func runConvoyTUI() error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	m := convoy.New(townBeads)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// resolveConvoyNumber converts a numeric shortcut (1, 2, 3...) to a convoy ID.
// Numbers correspond to the order shown in 'gt convoy list'.
func resolveConvoyNumber(townBeads string, n int) (string, error) {
	// Get convoy list (same query as runConvoyList)
	listArgs := []string{"list", "--type=convoy", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return "", fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return "", fmt.Errorf("parsing convoy list: %w", err)
	}

	if n < 1 || n > len(convoys) {
		return "", fmt.Errorf("convoy %d not found (have %d convoys)", n, len(convoys))
	}

	return convoys[n-1].ID, nil
}
