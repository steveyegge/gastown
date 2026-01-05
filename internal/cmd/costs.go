// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

var (
	costsJSON   bool
	costsToday  bool
	costsWeek   bool
	costsByRole bool
	costsByRig  bool

	// Record subcommand flags
	recordSession  string
	recordWorkItem string
)

var costsCmd = &cobra.Command{
	Use:     "costs",
	GroupID: GroupDiag,
	Short:   "Show costs for running Claude sessions",
	Long: `Display costs for Claude Code sessions in Gas Town.

By default, shows live costs scraped from running tmux sessions.

Examples:
  gt costs              # Live costs from running sessions
  gt costs --today      # Today's total from session events
  gt costs --week       # This week's total
  gt costs --by-role    # Breakdown by role (polecat, witness, etc.)
  gt costs --by-rig     # Breakdown by rig
  gt costs --json       # Output as JSON`,
	RunE: runCosts,
}

var costsRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record session cost as a bead event (called by Stop hook)",
	Long: `Record the final cost of a session as a session.ended event in beads.

This command is intended to be called from a Claude Code Stop hook.
It captures the final cost from the tmux session and creates an event
bead with the cost data.

Examples:
  gt costs record --session gt-gastown-toast
  gt costs record --session gt-gastown-toast --work-item gt-abc123`,
	RunE: runCostsRecord,
}

func init() {
	rootCmd.AddCommand(costsCmd)
	costsCmd.Flags().BoolVar(&costsJSON, "json", false, "Output as JSON")
	costsCmd.Flags().BoolVar(&costsToday, "today", false, "Show today's total from session events")
	costsCmd.Flags().BoolVar(&costsWeek, "week", false, "Show this week's total from session events")
	costsCmd.Flags().BoolVar(&costsByRole, "by-role", false, "Show breakdown by role")
	costsCmd.Flags().BoolVar(&costsByRig, "by-rig", false, "Show breakdown by rig")

	// Add record subcommand
	costsCmd.AddCommand(costsRecordCmd)
	costsRecordCmd.Flags().StringVar(&recordSession, "session", "", "Tmux session name to record")
	costsRecordCmd.Flags().StringVar(&recordWorkItem, "work-item", "", "Work item ID (bead) for attribution")
}

// SessionCost represents cost info for a single session.
type SessionCost struct {
	Session string  `json:"session"`
	Role    string  `json:"role"`
	Rig     string  `json:"rig,omitempty"`
	Worker  string  `json:"worker,omitempty"`
	Cost    float64 `json:"cost_usd"`
	Running bool    `json:"running"`
}

// CostEntry is a ledger entry for historical cost tracking.
type CostEntry struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Rig       string    `json:"rig,omitempty"`
	Worker    string    `json:"worker,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	WorkItem  string    `json:"work_item,omitempty"`
}

// CostsOutput is the JSON output structure.
type CostsOutput struct {
	Sessions []SessionCost      `json:"sessions,omitempty"`
	Total    float64            `json:"total_usd"`
	ByRole   map[string]float64 `json:"by_role,omitempty"`
	ByRig    map[string]float64 `json:"by_rig,omitempty"`
	Period   string             `json:"period,omitempty"`
}

// costRegex matches cost patterns like "$1.23" or "$12.34"
var costRegex = regexp.MustCompile(`\$(\d+\.\d{2})`)

func runCosts(cmd *cobra.Command, args []string) error {
	// If querying ledger, use ledger functions
	if costsToday || costsWeek || costsByRole || costsByRig {
		return runCostsFromLedger()
	}

	// Default: show live costs from running sessions
	return runLiveCosts()
}

func runLiveCosts() error {
	t := tmux.NewTmux()

	// Get all tmux sessions
	sessions, err := t.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	var costs []SessionCost
	var total float64

	for _, session := range sessions {
		// Only process Gas Town sessions (start with "gt-")
		if !strings.HasPrefix(session, constants.SessionPrefix) {
			continue
		}

		// Parse session name to get role/rig/worker
		role, rig, worker := parseSessionName(session)

		// Capture pane content
		content, err := t.CapturePaneAll(session)
		if err != nil {
			continue // Skip sessions we can't capture
		}

		// Extract cost from content
		cost := extractCost(content)

		// Check if Claude is running
		running := t.IsClaudeRunning(session)

		costs = append(costs, SessionCost{
			Session: session,
			Role:    role,
			Rig:     rig,
			Worker:  worker,
			Cost:    cost,
			Running: running,
		})
		total += cost
	}

	// Sort by session name
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Session < costs[j].Session
	})

	if costsJSON {
		return outputCostsJSON(CostsOutput{
			Sessions: costs,
			Total:    total,
		})
	}

	return outputCostsHuman(costs, total)
}

func runCostsFromLedger() error {
	// Query session events from beads
	entries, err := querySessionEvents()
	if err != nil {
		return fmt.Errorf("querying session events: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println(style.Dim.Render("No session events found. Costs are recorded when sessions end."))
		return nil
	}

	// Filter entries by time period
	var filtered []CostEntry
	now := time.Now()

	for _, entry := range entries {
		if costsToday {
			// Today: same day
			if entry.EndedAt.Year() == now.Year() &&
				entry.EndedAt.YearDay() == now.YearDay() {
				filtered = append(filtered, entry)
			}
		} else if costsWeek {
			// This week: within 7 days
			weekAgo := now.AddDate(0, 0, -7)
			if entry.EndedAt.After(weekAgo) {
				filtered = append(filtered, entry)
			}
		} else {
			// No time filter
			filtered = append(filtered, entry)
		}
	}

	// Calculate totals
	var total float64
	byRole := make(map[string]float64)
	byRig := make(map[string]float64)

	for _, entry := range filtered {
		total += entry.CostUSD
		byRole[entry.Role] += entry.CostUSD
		if entry.Rig != "" {
			byRig[entry.Rig] += entry.CostUSD
		}
	}

	// Build output
	output := CostsOutput{
		Total: total,
	}

	if costsByRole {
		output.ByRole = byRole
	}
	if costsByRig {
		output.ByRig = byRig
	}

	// Set period label
	if costsToday {
		output.Period = "today"
	} else if costsWeek {
		output.Period = "this week"
	}

	if costsJSON {
		return outputCostsJSON(output)
	}

	return outputLedgerHuman(output, filtered)
}

// SessionEvent represents a session.ended event from beads.
type SessionEvent struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	EventKind string    `json:"event_kind"`
	Actor     string    `json:"actor"`
	Target    string    `json:"target"`
	Payload   string    `json:"payload"`
}

// SessionPayload represents the JSON payload of a session event.
type SessionPayload struct {
	CostUSD   float64 `json:"cost_usd"`
	SessionID string  `json:"session_id"`
	Role      string  `json:"role"`
	Rig       string  `json:"rig"`
	Worker    string  `json:"worker"`
	EndedAt   string  `json:"ended_at"`
}

// EventListItem represents an event from bd list (minimal fields).
type EventListItem struct {
	ID string `json:"id"`
}

// querySessionEvents queries beads for session.ended events and converts them to CostEntry.
func querySessionEvents() ([]CostEntry, error) {
	// Step 1: Get list of event IDs
	listArgs := []string{
		"list",
		"--type=event",
		"--all",
		"--limit=0",
		"--json",
	}

	listCmd := exec.Command("bd", listArgs...)
	listOutput, err := listCmd.Output()
	if err != nil {
		// If bd fails (e.g., no beads database), return empty list
		return nil, nil
	}

	var listItems []EventListItem
	if err := json.Unmarshal(listOutput, &listItems); err != nil {
		return nil, fmt.Errorf("parsing event list: %w", err)
	}

	if len(listItems) == 0 {
		return nil, nil
	}

	// Step 2: Get full details for all events using bd show
	// (bd list doesn't include event_kind, actor, payload)
	showArgs := []string{"show", "--json"}
	for _, item := range listItems {
		showArgs = append(showArgs, item.ID)
	}

	showCmd := exec.Command("bd", showArgs...)
	showOutput, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("showing events: %w", err)
	}

	var events []SessionEvent
	if err := json.Unmarshal(showOutput, &events); err != nil {
		return nil, fmt.Errorf("parsing event details: %w", err)
	}

	var entries []CostEntry
	for _, event := range events {
		// Filter for session.ended events only
		if event.EventKind != "session.ended" {
			continue
		}

		// Parse payload
		var payload SessionPayload
		if event.Payload != "" {
			if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
				continue // Skip malformed payloads
			}
		}

		// Parse ended_at from payload, fall back to created_at
		endedAt := event.CreatedAt
		if payload.EndedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, payload.EndedAt); err == nil {
				endedAt = parsed
			}
		}

		entries = append(entries, CostEntry{
			SessionID: payload.SessionID,
			Role:      payload.Role,
			Rig:       payload.Rig,
			Worker:    payload.Worker,
			CostUSD:   payload.CostUSD,
			EndedAt:   endedAt,
			WorkItem:  event.Target,
		})
	}

	return entries, nil
}

// parseSessionName extracts role, rig, and worker from a session name.
// Session names follow the pattern: gt-<rig>-<worker> or gt-<global-agent>
// Examples:
//   - gt-mayor -> role=mayor, rig="", worker="mayor"
//   - gt-deacon -> role=deacon, rig="", worker="deacon"
//   - gt-gastown-toast -> role=polecat, rig=gastown, worker=toast
//   - gt-gastown-witness -> role=witness, rig=gastown, worker=""
//   - gt-gastown-refinery -> role=refinery, rig=gastown, worker=""
//   - gt-gastown-crew-joe -> role=crew, rig=gastown, worker=joe
func parseSessionName(session string) (role, rig, worker string) {
	// Remove gt- prefix
	name := strings.TrimPrefix(session, constants.SessionPrefix)

	// Check for global agents
	switch name {
	case "mayor":
		return constants.RoleMayor, "", "mayor"
	case "deacon":
		return constants.RoleDeacon, "", "deacon"
	}

	// Parse rig-based session: rig-worker or rig-crew-name
	parts := strings.SplitN(name, "-", 3)
	if len(parts) < 2 {
		return "unknown", "", name
	}

	rig = parts[0]
	worker = parts[1]

	// Check for crew pattern: rig-crew-name
	if worker == "crew" && len(parts) >= 3 {
		return constants.RoleCrew, rig, parts[2]
	}

	// Check for special workers
	switch worker {
	case "witness":
		return constants.RoleWitness, rig, ""
	case "refinery":
		return constants.RoleRefinery, rig, ""
	}

	// Default to polecat
	return constants.RolePolecat, rig, worker
}

// extractCost finds the most recent cost value in pane content.
// Claude Code displays cost in the format "$X.XX" in the status area.
func extractCost(content string) float64 {
	matches := costRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return 0.0
	}

	// Get the last (most recent) match
	lastMatch := matches[len(matches)-1]
	if len(lastMatch) < 2 {
		return 0.0
	}

	var cost float64
	_, _ = fmt.Sscanf(lastMatch[1], "%f", &cost)
	return cost
}


func outputCostsJSON(output CostsOutput) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputCostsHuman(costs []SessionCost, total float64) error {
	if len(costs) == 0 {
		fmt.Println(style.Dim.Render("No Gas Town sessions found"))
		return nil
	}

	fmt.Printf("\n%s Live Session Costs\n\n", style.Bold.Render("💰"))

	// Print table header
	fmt.Printf("%-25s %-10s %-15s %10s %8s\n",
		"Session", "Role", "Rig/Worker", "Cost", "Status")
	fmt.Println(strings.Repeat("─", 75))

	// Print each session
	for _, c := range costs {
		statusIcon := style.Success.Render("●")
		if !c.Running {
			statusIcon = style.Dim.Render("○")
		}

		rigWorker := c.Rig
		if c.Worker != "" && c.Worker != c.Rig {
			if rigWorker != "" {
				rigWorker += "/" + c.Worker
			} else {
				rigWorker = c.Worker
			}
		}

		fmt.Printf("%-25s %-10s %-15s %10s %8s\n",
			c.Session,
			c.Role,
			rigWorker,
			fmt.Sprintf("$%.2f", c.Cost),
			statusIcon)
	}

	// Print total
	fmt.Println(strings.Repeat("─", 75))
	fmt.Printf("%s %s\n", style.Bold.Render("Total:"), fmt.Sprintf("$%.2f", total))

	return nil
}

func outputLedgerHuman(output CostsOutput, entries []CostEntry) error {
	periodStr := ""
	if output.Period != "" {
		periodStr = fmt.Sprintf(" (%s)", output.Period)
	}

	fmt.Printf("\n%s Cost Summary%s\n\n", style.Bold.Render("📊"), periodStr)

	// Total
	fmt.Printf("%s $%.2f\n", style.Bold.Render("Total:"), output.Total)

	// By role breakdown
	if output.ByRole != nil && len(output.ByRole) > 0 {
		fmt.Printf("\n%s\n", style.Bold.Render("By Role:"))
		for role, cost := range output.ByRole {
			icon := constants.RoleEmoji(role)
			fmt.Printf("  %s %-12s $%.2f\n", icon, role, cost)
		}
	}

	// By rig breakdown
	if output.ByRig != nil && len(output.ByRig) > 0 {
		fmt.Printf("\n%s\n", style.Bold.Render("By Rig:"))
		for rig, cost := range output.ByRig {
			fmt.Printf("  %-15s $%.2f\n", rig, cost)
		}
	}

	// Session count
	fmt.Printf("\n%s %d sessions\n", style.Dim.Render("Entries:"), len(entries))

	return nil
}

// runCostsRecord captures the final cost from a session and records it as a bead event.
// This is called by the Claude Code Stop hook.
func runCostsRecord(cmd *cobra.Command, args []string) error {
	// Get session from flag or try to detect from environment
	session := recordSession
	if session == "" {
		session = os.Getenv("GT_SESSION")
	}
	if session == "" {
		// Derive session name from GT_* environment variables
		session = deriveSessionName()
	}
	if session == "" {
		// Try to detect current tmux session (works when running inside tmux)
		session = detectCurrentTmuxSession()
	}
	if session == "" {
		return fmt.Errorf("--session flag required (or set GT_SESSION env var, or GT_RIG/GT_ROLE)")
	}

	t := tmux.NewTmux()

	// Capture pane content
	content, err := t.CapturePaneAll(session)
	if err != nil {
		// Session may already be gone - that's OK, we'll record with zero cost
		content = ""
	}

	// Extract cost
	cost := extractCost(content)

	// Parse session name
	role, rig, worker := parseSessionName(session)

	// Build agent path for actor field
	agentPath := buildAgentPath(role, rig, worker)

	// Build event title
	title := fmt.Sprintf("Session ended: %s", session)
	if recordWorkItem != "" {
		title = fmt.Sprintf("Session: %s completed %s", session, recordWorkItem)
	}

	// Build payload JSON
	payload := map[string]interface{}{
		"cost_usd":   cost,
		"session_id": session,
		"role":       role,
		"ended_at":   time.Now().Format(time.RFC3339),
	}
	if rig != "" {
		payload["rig"] = rig
	}
	if worker != "" {
		payload["worker"] = worker
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	// Build bd create command
	bdArgs := []string{
		"create",
		"--type=event",
		"--title=" + title,
		"--event-category=session.ended",
		"--event-actor=" + agentPath,
		"--event-payload=" + string(payloadJSON),
		"--silent",
	}

	// Add work item as event target if specified
	if recordWorkItem != "" {
		bdArgs = append(bdArgs, "--event-target="+recordWorkItem)
	}

	// NOTE: We intentionally don't use --rig flag here because it causes
	// event fields (event_kind, actor, payload) to not be stored properly.
	// The bd command will auto-detect the correct rig from cwd.
	// TODO: File beads bug about --rig flag losing event fields.

	// Execute bd create
	bdCmd := exec.Command("bd", bdArgs...)
	output, err := bdCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating session event: %w\nOutput: %s", err, string(output))
	}

	eventID := strings.TrimSpace(string(output))

	// Auto-close session events immediately after creation.
	// These are informational audit events that don't need to stay open.
	// The event data is preserved in the closed bead and remains queryable.
	closeCmd := exec.Command("bd", "close", eventID, "--reason=auto-closed session event")
	if closeErr := closeCmd.Run(); closeErr != nil {
		// Non-fatal: event was created, just couldn't auto-close
		// The witness patrol can clean these up if needed
		fmt.Fprintf(os.Stderr, "warning: could not auto-close session event %s: %v\n", eventID, closeErr)
	}

	// Output confirmation (silent if cost is zero and no work item)
	if cost > 0 || recordWorkItem != "" {
		fmt.Printf("%s Recorded $%.2f for %s (event: %s)", style.Success.Render("✓"), cost, session, eventID)
		if recordWorkItem != "" {
			fmt.Printf(" (work: %s)", recordWorkItem)
		}
		fmt.Println()
	}

	return nil
}

// deriveSessionName derives the tmux session name from GT_* environment variables.
// Session naming patterns:
//   - Polecats: gt-{rig}-{polecat} (e.g., gt-gastown-toast)
//   - Crew: gt-{rig}-crew-{crew} (e.g., gt-gastown-crew-max)
//   - Witness/Refinery: gt-{rig}-{role} (e.g., gt-gastown-witness)
//   - Mayor/Deacon: gt-{town}-{role} (e.g., gt-ai-mayor)
func deriveSessionName() string {
	role := os.Getenv("GT_ROLE")
	rig := os.Getenv("GT_RIG")
	polecat := os.Getenv("GT_POLECAT")
	crew := os.Getenv("GT_CREW")
	town := os.Getenv("GT_TOWN")

	// Polecat: gt-{rig}-{polecat}
	if polecat != "" && rig != "" {
		return fmt.Sprintf("gt-%s-%s", rig, polecat)
	}

	// Crew: gt-{rig}-crew-{crew}
	if crew != "" && rig != "" {
		return fmt.Sprintf("gt-%s-crew-%s", rig, crew)
	}

	// Town-level roles (mayor, deacon): gt-{town}-{role}
	if (role == "mayor" || role == "deacon") && town != "" {
		return fmt.Sprintf("gt-%s-%s", town, role)
	}

	// Rig-based roles (witness, refinery): gt-{rig}-{role}
	if role != "" && rig != "" {
		return fmt.Sprintf("gt-%s-%s", rig, role)
	}

	return ""
}

// detectCurrentTmuxSession returns the current tmux session name if running inside tmux.
// Uses `tmux display-message -p '#S'` which prints the session name.
func detectCurrentTmuxSession() string {
	// Check if we're inside tmux
	if os.Getenv("TMUX") == "" {
		return ""
	}

	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	session := strings.TrimSpace(string(output))
	// Only return if it looks like a Gas Town session
	if strings.HasPrefix(session, constants.SessionPrefix) {
		return session
	}
	return ""
}

// buildAgentPath builds the agent path from role, rig, and worker.
// Examples: "mayor", "gastown/witness", "gastown/polecats/toast"
func buildAgentPath(role, rig, worker string) string {
	switch role {
	case constants.RoleMayor, constants.RoleDeacon:
		return role
	case constants.RoleWitness, constants.RoleRefinery:
		if rig != "" {
			return rig + "/" + role
		}
		return role
	case constants.RolePolecat:
		if rig != "" && worker != "" {
			return rig + "/polecats/" + worker
		}
		if rig != "" {
			return rig + "/polecat"
		}
		return "polecat/" + worker
	case constants.RoleCrew:
		if rig != "" && worker != "" {
			return rig + "/crew/" + worker
		}
		if rig != "" {
			return rig + "/crew"
		}
		return "crew/" + worker
	default:
		if rig != "" && worker != "" {
			return rig + "/" + worker
		}
		if rig != "" {
			return rig
		}
		return worker
	}
}
