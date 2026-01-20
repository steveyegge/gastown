package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	statusLineSession string
)

var statusLineCmd = &cobra.Command{
	Use:    "status-line",
	Short:  "Output status line content for tmux (internal use)",
	Hidden: true, // Internal command called by tmux
	RunE:   runStatusLine,
}

func init() {
	rootCmd.AddCommand(statusLineCmd)
	statusLineCmd.Flags().StringVar(&statusLineSession, "session", "", "Tmux session name")
}

func runStatusLine(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	// Get session environment
	var rigName, polecat, crew, issue, role string

	if statusLineSession != "" {
		// Non-fatal: missing env vars are handled gracefully below
		rigName, _ = t.GetEnvironment(statusLineSession, "GT_RIG")
		polecat, _ = t.GetEnvironment(statusLineSession, "GT_POLECAT")
		crew, _ = t.GetEnvironment(statusLineSession, "GT_CREW")
		issue, _ = t.GetEnvironment(statusLineSession, "GT_ISSUE")
		role, _ = t.GetEnvironment(statusLineSession, "GT_ROLE")
	} else {
		// Fallback to process environment
		rigName = os.Getenv("GT_RIG")
		polecat = os.Getenv("GT_POLECAT")
		crew = os.Getenv("GT_CREW")
		issue = os.Getenv("GT_ISSUE")
		role = os.Getenv("GT_ROLE")
	}

	// Get session names for comparison
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()

	// Determine identity and output based on role
	if role == "mayor" || statusLineSession == mayorSession {
		return runMayorStatusLine(t)
	}

	// Deacon status line
	if role == "deacon" || statusLineSession == deaconSession {
		return runDeaconStatusLine(t)
	}

	// Witness status line (session naming: gt-<rig>-witness)
	if role == "witness" || strings.HasSuffix(statusLineSession, "-witness") {
		return runWitnessStatusLine(t, rigName)
	}

	// Refinery status line
	if role == "refinery" || strings.HasSuffix(statusLineSession, "-refinery") {
		return runRefineryStatusLine(t, rigName)
	}

	// Crew/Polecat status line
	return runWorkerStatusLine(t, statusLineSession, rigName, polecat, crew, issue)
}

// runWorkerStatusLine outputs status for crew or polecat sessions.
func runWorkerStatusLine(t *tmux.Tmux, session, rigName, polecat, crew, issue string) error {
	// Determine agent type and identity
	var icon, identity string
	if polecat != "" {
		icon = AgentTypeIcons[AgentPolecat]
		identity = fmt.Sprintf("%s/%s", rigName, polecat)
	} else if crew != "" {
		icon = AgentTypeIcons[AgentCrew]
		identity = fmt.Sprintf("%s/crew/%s", rigName, crew)
	}

	// Get pane's working directory to find workspace
	var townRoot string
	if session != "" {
		paneDir, err := t.GetPaneWorkDir(session)
		if err == nil && paneDir != "" {
			townRoot, _ = workspace.Find(paneDir)
		}
	}

	// Build status parts
	var parts []string

	// Priority 1: Check for hooked work (use rig beads)
	hookedWork := ""
	if identity != "" && rigName != "" && townRoot != "" {
		rigBeadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")
		hookedWork = getHookedWork(identity, 40, rigBeadsDir)
	}

	// Priority 2: Fall back to GT_ISSUE env var or in_progress beads
	currentWork := issue
	if currentWork == "" && hookedWork == "" && session != "" {
		currentWork = getCurrentWork(t, session, 40)
	}

	// Show hooked work (takes precedence)
	if hookedWork != "" {
		if icon != "" {
			parts = append(parts, fmt.Sprintf("%s 🪝 %s", icon, hookedWork))
		} else {
			parts = append(parts, fmt.Sprintf("🪝 %s", hookedWork))
		}
	} else if currentWork != "" {
		// Fall back to current work (in_progress)
		if icon != "" {
			parts = append(parts, fmt.Sprintf("%s %s", icon, currentWork))
		} else {
			parts = append(parts, currentWork)
		}
	} else if icon != "" {
		parts = append(parts, icon)
	}

	// Mail preview - only show if hook is empty
	if hookedWork == "" && identity != "" && townRoot != "" {
		unread, subject := getMailPreviewWithRoot(identity, 45, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %d", unread))
			}
		}
	}

	// Output
	if len(parts) > 0 {
		fmt.Print(strings.Join(parts, " | ") + " |")
	}

	return nil
}

func runMayorStatusLine(t *tmux.Tmux) error {
	// Count active sessions by listing tmux sessions
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	// Get town root from mayor pane's working directory
	var townRoot string
	mayorSession := getMayorSessionName()
	paneDir, err := t.GetPaneWorkDir(mayorSession)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Load registered rigs to validate against
	registeredRigs := make(map[string]bool)
	if townRoot != "" {
		rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
		if rigsConfig, err := config.LoadRigsConfig(rigsConfigPath); err == nil {
			for rigName := range rigsConfig.Rigs {
				registeredRigs[rigName] = true
			}
		}
	}

	// Track per-rig status for LED indicators and sorting
	type rigStatus struct {
		hasWitness  bool
		hasRefinery bool
		opState     string // "OPERATIONAL", "PARKED", or "DOCKED"
	}
	rigStatuses := make(map[string]*rigStatus)

	// Initialize for all registered rigs
	for rigName := range registeredRigs {
		rigStatuses[rigName] = &rigStatus{}
	}

	// Track per-agent-type health (working/zombie counts)
	type agentHealth struct {
		total   int
		working int
	}
	healthByType := map[AgentType]*agentHealth{
		AgentWitness:  {},
		AgentRefinery: {},
	}

	// Single pass: track rig status AND agent health
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}

		// Track rig-level status (witness/refinery presence)
		// Polecats are not tracked in tmux - they're a GC concern, not a display concern
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			if rigStatuses[agent.Rig] == nil {
				rigStatuses[agent.Rig] = &rigStatus{}
			}
			switch agent.Type {
			case AgentWitness:
				rigStatuses[agent.Rig].hasWitness = true
			case AgentRefinery:
				rigStatuses[agent.Rig].hasRefinery = true
			}
		}

		// Track agent health (skip Mayor and Crew)
		if health := healthByType[agent.Type]; health != nil {
			health.total++
			// Detect working state via ✻ symbol
			if isSessionWorking(t, s) {
				health.working++
			}
		}
	}

	// Get operational state for each rig
	for rigName, status := range rigStatuses {
		opState, _ := getRigOperationalState(townRoot, rigName)
		if opState == "PARKED" || opState == "DOCKED" {
			status.opState = opState
		} else {
			status.opState = "OPERATIONAL"
		}
	}

	// Build status
	var parts []string

	// Add per-agent-type health in consistent order
	// Format: "1/3 👁️" = 1 working out of 3 total
	// Only show agent types that have sessions
	// Note: Polecats and Deacon excluded - idle state display is misleading noise
	agentOrder := []AgentType{AgentWitness, AgentRefinery}
	var agentParts []string
	for _, agentType := range agentOrder {
		health := healthByType[agentType]
		if health.total == 0 {
			continue
		}
		icon := AgentTypeIcons[agentType]
		agentParts = append(agentParts, fmt.Sprintf("%d/%d %s", health.working, health.total, icon))
	}
	if len(agentParts) > 0 {
		parts = append(parts, strings.Join(agentParts, " "))
	}

	// Build rig status display with LED indicators
	// 🟢 = both witness and refinery running (fully active)
	// 🟡 = one of witness/refinery running (partially active)
	// 🅿️ = parked (nothing running, intentionally paused)
	// 🛑 = docked (nothing running, global shutdown)
	// ⚫ = operational but nothing running (unexpected state)

	// Create sortable rig list
	type rigInfo struct {
		name   string
		status *rigStatus
	}
	var rigs []rigInfo
	for rigName, status := range rigStatuses {
		rigs = append(rigs, rigInfo{name: rigName, status: status})
	}

	// Sort by: 1) running state, 2) operational state, 3) alphabetical
	sort.Slice(rigs, func(i, j int) bool {
		isRunningI := rigs[i].status.hasWitness || rigs[i].status.hasRefinery
		isRunningJ := rigs[j].status.hasWitness || rigs[j].status.hasRefinery

		// Primary sort: running rigs before non-running rigs
		if isRunningI != isRunningJ {
			return isRunningI
		}

		// Secondary sort: operational state (for non-running rigs: OPERATIONAL < PARKED < DOCKED)
		stateOrder := map[string]int{"OPERATIONAL": 0, "PARKED": 1, "DOCKED": 2}
		stateI := stateOrder[rigs[i].status.opState]
		stateJ := stateOrder[rigs[j].status.opState]
		if stateI != stateJ {
			return stateI < stateJ
		}

		// Tertiary sort: alphabetical
		return rigs[i].name < rigs[j].name
	})

	// Build display with group separators
	var rigParts []string
	var lastGroup string
	for _, rig := range rigs {
		isRunning := rig.status.hasWitness || rig.status.hasRefinery
		var currentGroup string
		if isRunning {
			currentGroup = "running"
		} else {
			currentGroup = "idle-" + rig.status.opState
		}

		// Add separator when group changes (running -> non-running, or different opStates within non-running)
		if lastGroup != "" && lastGroup != currentGroup {
			rigParts = append(rigParts, "|")
		}
		lastGroup = currentGroup

		status := rig.status
		var led string

		// Check if processes are running first (regardless of operational state)
		if status.hasWitness && status.hasRefinery {
			led = "🟢" // Both running - fully active
		} else if status.hasWitness || status.hasRefinery {
			led = "🟡" // One running - partially active
		} else {
			// Nothing running - show operational state
			switch status.opState {
			case "PARKED":
				led = "🅿️" // Parked - intentionally paused
			case "DOCKED":
				led = "🛑" // Docked - global shutdown
			default:
				led = "⚫" // Operational but nothing running
			}
		}

		// All icons get 1 space, Park gets 2
		space := " "
		if led == "🅿️" {
			space = "  "
		}
		rigParts = append(rigParts, led+space+rig.name)
	}

	if len(rigParts) > 0 {
		parts = append(parts, strings.Join(rigParts, " "))
	}

	// Priority 1: Check for hooked work (town beads for mayor)
	hookedWork := ""
	if townRoot != "" {
		hookedWork = getHookedWork("mayor", 40, townRoot)
	}
	if hookedWork != "" {
		parts = append(parts, fmt.Sprintf("🪝 %s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot("mayor/", 45, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runDeaconStatusLine outputs status for the deacon session.
// Shows: active rigs, polecat count, hook or mail preview
func runDeaconStatusLine(t *tmux.Tmux) error {
	// Count active rigs and polecats
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	// Get town root from deacon pane's working directory
	var townRoot string
	deaconSession := getDeaconSessionName()
	paneDir, err := t.GetPaneWorkDir(deaconSession)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Load registered rigs to validate against
	registeredRigs := make(map[string]bool)
	if townRoot != "" {
		rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
		if rigsConfig, err := config.LoadRigsConfig(rigsConfigPath); err == nil {
			for rigName := range rigsConfig.Rigs {
				registeredRigs[rigName] = true
			}
		}
	}

	rigs := make(map[string]bool)
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		// Only count registered rigs
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			rigs[agent.Rig] = true
		}
	}
	rigCount := len(rigs)

	// Build status
	// Note: Polecats excluded - they're ephemeral and idle detection is a GC concern
	var parts []string
	parts = append(parts, fmt.Sprintf("%d rigs", rigCount))

	// Priority 1: Check for hooked work (town beads for deacon)
	hookedWork := ""
	if townRoot != "" {
		hookedWork = getHookedWork("deacon", 35, townRoot)
	}
	if hookedWork != "" {
		parts = append(parts, fmt.Sprintf("🪝 %s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot("deacon/", 40, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runWitnessStatusLine outputs status for a witness session.
// Shows: crew count, hook or mail preview
// Note: Polecats excluded - they're ephemeral and idle detection is a GC concern
func runWitnessStatusLine(t *tmux.Tmux, rigName string) error {
	if rigName == "" {
		// Try to extract from session name: gt-<rig>-witness
		if strings.HasSuffix(statusLineSession, "-witness") && strings.HasPrefix(statusLineSession, "gt-") {
			rigName = strings.TrimPrefix(strings.TrimSuffix(statusLineSession, "-witness"), "gt-")
		}
	}

	// Get town root from witness pane's working directory
	var townRoot string
	sessionName := fmt.Sprintf("gt-%s-witness", rigName)
	paneDir, err := t.GetPaneWorkDir(sessionName)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Count crew in this rig (crew are persistent, worth tracking)
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	crewCount := 0
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		if agent.Rig == rigName && agent.Type == AgentCrew {
			crewCount++
		}
	}

	identity := fmt.Sprintf("%s/witness", rigName)

	// Build status
	var parts []string
	if crewCount > 0 {
		parts = append(parts, fmt.Sprintf("%d crew", crewCount))
	}

	// Priority 1: Check for hooked work (rig beads for witness)
	hookedWork := ""
	if townRoot != "" && rigName != "" {
		rigBeadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")
		hookedWork = getHookedWork(identity, 30, rigBeadsDir)
	}
	if hookedWork != "" {
		parts = append(parts, fmt.Sprintf("🪝 %s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot(identity, 35, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runRefineryStatusLine outputs status for a refinery session.
// Shows: MQ length, current item, hook or mail preview
func runRefineryStatusLine(t *tmux.Tmux, rigName string) error {
	if rigName == "" {
		// Try to extract from session name: gt-<rig>-refinery
		if strings.HasPrefix(statusLineSession, "gt-") && strings.HasSuffix(statusLineSession, "-refinery") {
			rigName = strings.TrimPrefix(statusLineSession, "gt-")
			rigName = strings.TrimSuffix(rigName, "-refinery")
		}
	}

	if rigName == "" {
		fmt.Printf("%s ? |", AgentTypeIcons[AgentRefinery])
		return nil
	}

	// Get town root from refinery pane's working directory
	var townRoot string
	sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
	paneDir, err := t.GetPaneWorkDir(sessionName)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Get refinery manager using shared helper
	mgr, _, _, err := getRefineryManager(rigName)
	if err != nil {
		// Fallback to simple status if we can't access refinery
		fmt.Printf("%s MQ: ? |", AgentTypeIcons[AgentRefinery])
		return nil
	}

	// Get queue
	queue, err := mgr.Queue()
	if err != nil {
		// Fallback to simple status if we can't read queue
		fmt.Printf("%s MQ: ? |", AgentTypeIcons[AgentRefinery])
		return nil
	}

	// Count pending items and find current item
	pending := 0
	var currentItem string
	for _, item := range queue {
		if item.Position == 0 && item.MR != nil {
			// Currently processing - show issue ID
			currentItem = item.MR.IssueID
		} else {
			pending++
		}
	}

	identity := fmt.Sprintf("%s/refinery", rigName)

	// Build status
	var parts []string
	if currentItem != "" {
		parts = append(parts, fmt.Sprintf("merging %s", currentItem))
		if pending > 0 {
			parts = append(parts, fmt.Sprintf("+%d queued", pending))
		}
	} else if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", pending))
	} else {
		parts = append(parts, "idle")
	}

	// Priority 1: Check for hooked work (rig beads for refinery)
	hookedWork := ""
	if townRoot != "" && rigName != "" {
		rigBeadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")
		hookedWork = getHookedWork(identity, 25, rigBeadsDir)
	}
	if hookedWork != "" {
		parts = append(parts, fmt.Sprintf("🪝 %s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot(identity, 30, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC %d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// isSessionWorking detects if a Claude Code session is actively working.
// Returns true if the ✶ symbol is visible in the pane (indicates Claude is processing).
// Returns false for idle sessions (showing ❯ prompt) or if state cannot be determined.
//
// Claude Code uses two different status indicators:
//   - ✶ (U+2736): Active - shown during thinking/tool execution ("Proposing fix...")
//   - ✻ (U+273B): Completed - shown after work finishes ("Worked for 1m 55s")
//
// We check for ✶ to detect active work, and optionally ✻ as a fallback for
// sessions that just completed work (still considered "working" for display purposes).
func isSessionWorking(t *tmux.Tmux, session string) bool {
	// Capture last few lines of the pane (increased from 5 to 10 for reliability)
	lines, err := t.CapturePaneLines(session, 10)
	if err != nil || len(lines) == 0 {
		return false
	}

	// Check all captured lines for working indicators
	// Primary: ✶ (U+2736) appears during active processing
	// Secondary: ✻ (U+273B) appears after completion (recent work)
	for _, line := range lines {
		if strings.Contains(line, "✶") || strings.Contains(line, "✻") {
			return true
		}
	}

	return false
}

// getUnreadMailCount returns unread mail count for an identity.
// Fast path - returns 0 on any error.
func getUnreadMailCount(identity string) int {
	// Find workspace
	workDir, err := findMailWorkDir()
	if err != nil {
		return 0
	}

	// Create mailbox using beads
	mailbox := mail.NewMailboxBeads(identity, workDir)

	// Get count
	_, unread, err := mailbox.Count()
	if err != nil {
		return 0
	}

	return unread
}

// getMailPreview returns unread count and a truncated subject of the first unread message.
// Returns (count, subject) where subject is empty if no unread mail.
func getMailPreview(identity string, maxLen int) (int, string) {
	workDir, err := findMailWorkDir()
	if err != nil {
		return 0, ""
	}

	mailbox := mail.NewMailboxBeads(identity, workDir)

	// Get unread messages
	messages, err := mailbox.ListUnread()
	if err != nil || len(messages) == 0 {
		return 0, ""
	}

	// Get first message subject, truncated
	subject := messages[0].Subject
	if len(subject) > maxLen {
		subject = subject[:maxLen-1] + "…"
	}

	return len(messages), subject
}

// getMailPreviewWithRoot is like getMailPreview but uses an explicit town root.
func getMailPreviewWithRoot(identity string, maxLen int, townRoot string) (int, string) {
	// Use NewMailboxFromAddress to normalize identity (e.g., gastown/crew/gus -> gastown/gus)
	mailbox := mail.NewMailboxFromAddress(identity, townRoot)

	// Get unread messages
	messages, err := mailbox.ListUnread()
	if err != nil || len(messages) == 0 {
		return 0, ""
	}

	// Get first message subject, truncated
	subject := messages[0].Subject
	if len(subject) > maxLen {
		subject = subject[:maxLen-1] + "…"
	}

	return len(messages), subject
}

// getHookedWork returns a truncated title of the hooked bead for an agent.
// Returns empty string if nothing is hooked.
// beadsDir should be the directory containing .beads (for rig-level) or
// empty to use the town root (for town-level roles).
func getHookedWork(identity string, maxLen int, beadsDir string) string {
	// If no beadsDir specified, use town root
	if beadsDir == "" {
		var err error
		beadsDir, err = findMailWorkDir()
		if err != nil {
			return ""
		}
	}

	b := beads.New(beadsDir)

	// Query for hooked beads assigned to this agent
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: identity,
		Priority: -1,
	})
	if err != nil || len(hookedBeads) == 0 {
		return ""
	}

	// Return first hooked bead's ID and title, truncated
	bead := hookedBeads[0]
	display := fmt.Sprintf("%s: %s", bead.ID, bead.Title)
	if len(display) > maxLen {
		display = display[:maxLen-1] + "…"
	}
	return display
}

// getCurrentWork returns a truncated title of the first in_progress issue.
// Uses the pane's working directory to find the beads.
func getCurrentWork(t *tmux.Tmux, session string, maxLen int) string {
	// Get the pane's working directory
	workDir, err := t.GetPaneWorkDir(session)
	if err != nil || workDir == "" {
		return ""
	}

	// Check if there's a .beads directory
	beadsDir := filepath.Join(workDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return ""
	}

	// Query beads for in_progress issues
	b := beads.New(workDir)
	issues, err := b.List(beads.ListOptions{
		Status:   "in_progress",
		Priority: -1,
	})
	if err != nil || len(issues) == 0 {
		return ""
	}

	// Return first issue's ID and title, truncated
	issue := issues[0]
	display := fmt.Sprintf("%s: %s", issue.ID, issue.Title)
	if len(display) > maxLen {
		display = display[:maxLen-1] + "…"
	}
	return display
}
