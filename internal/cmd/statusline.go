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

// rigAbbreviations maps rig names to their abbreviations for the statusline.
var rigAbbreviations = map[string]string{
	"design_forge":  "df",
	"gastown":       "gt",
	"mediaforge":    "mf",
	"mt5optimizer":  "mt",
	"opencode":      "oc",
	"pod_automation": "pa",
	"promptforge":   "pf",
	"autoaffiliate": "aa",
}

// abbreviateRigName returns the abbreviated form of a rig name if one exists,
// otherwise returns the original name.
func abbreviateRigName(rigName string) string {
	if abbr, ok := rigAbbreviations[rigName]; ok {
		return abbr
	}
	return rigName
}

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
	abbrRigName := abbreviateRigName(rigName)
	if polecat != "" {
		icon = AgentTypeIcons[AgentPolecat]
		identity = fmt.Sprintf("%s/%s", abbrRigName, polecat)
	} else if crew != "" {
		icon = AgentTypeIcons[AgentCrew]
		identity = fmt.Sprintf("%s/crew/%s", abbrRigName, crew)
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
			parts = append(parts, fmt.Sprintf("🪝%s", hookedWork))
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
				parts = append(parts, fmt.Sprintf("\U0001F4EC%s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%d", unread))
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
		hasWitness   bool
		hasRefinery  bool
		polecatCount int
		opState      string // "OPERATIONAL", "PARKED", or "DOCKED"
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
		AgentPolecat:  {},
		AgentWitness:  {},
		AgentRefinery: {},
		AgentDeacon:   {},
	}

	// Single pass: track rig status AND agent health
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}

		// Track rig-level status (witness/refinery/polecat presence)
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			if rigStatuses[agent.Rig] == nil {
				rigStatuses[agent.Rig] = &rigStatus{}
			}
			switch agent.Type {
			case AgentWitness:
				rigStatuses[agent.Rig].hasWitness = true
			case AgentRefinery:
				rigStatuses[agent.Rig].hasRefinery = true
			case AgentPolecat:
				rigStatuses[agent.Rig].polecatCount++
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

	// Track per-agent-type health (working/idle counts)
	type sessionHealth struct {
		icon    string
		total   int
		working int
		idle    int
	}

	// Agent type icons
	agentIcons := map[AgentType]string{
		AgentPolecat:  "😺",
		AgentWitness:  "👁️",
		AgentRefinery: "🏭",
		AgentDeacon:   "⛪",
		AgentCrew:     "👷",
	}

	// Initialize health tracker for all agent types (always show all types)
	healthByType := make(map[AgentType]*sessionHealth)
	for _, agentType := range []AgentType{AgentPolecat, AgentWitness, AgentRefinery, AgentDeacon, AgentCrew} {
		healthByType[agentType] = &sessionHealth{
			icon: agentIcons[agentType],
		}
	}

	for _, s := range sessions {
		// Skip non-Gas Town sessions
		if !strings.HasPrefix(s, "gt-") && !strings.HasPrefix(s, "hq-") {
			continue
		}

		agent := categorizeSession(s)
		if agent == nil {
			continue
		}

		// Skip mayor (viewer of statusline, not tracked)
		if agent.Type == AgentMayor {
			continue
		}

		healthByType[agent.Type].total++

		// Detect working state (✻ = working, ❯ = idle)
		state := getSessionState(t, s)
		if state == "working" {
			healthByType[agent.Type].working++
		} else if state == "idle" {
			healthByType[agent.Type].idle++
		}
		// dead sessions counted in total but not working/idle
	}

	// Build status
	var parts []string

	// Add per-agent-type health in consistent order
	// Format: 😺1/10 (1 working out of 10 total, 9 idle inferred)
	// Always show all agent types, even when count is 0
	agentOrder := []AgentType{AgentPolecat, AgentWitness, AgentRefinery, AgentDeacon, AgentCrew}
	var agentParts []string
	for _, agentType := range agentOrder {
		health := healthByType[agentType]
		// Always show all agent types (no continue for total == 0)

		// Show working/total format (e.g., "1/10😺" = 1 working, 10 total)
		agentParts = append(agentParts, fmt.Sprintf("%d/%d%s", health.working, health.total, health.icon))
	}
	if len(agentParts) > 0 {
		parts = append(parts, "active: "+strings.Join(agentParts, " "))
	}

	// Build rig status display with LED indicators
	// 🟢 = both witness and refinery running (fully active)
	// 🟡 = one of witness/refinery running (partially active)
	// ⚫ = neither running (inactive)
	// ⏸️ = parked or docked (intentionally offline)
	var rigParts []string
	var rigNames []string
	for rigName := range rigStatuses {
		rigNames = append(rigNames, rigName)
	}

	// Sort rigs: active rigs first (alphabetically), then parked/stopped rigs (alphabetically)
	sort.Slice(rigNames, func(i, j int) bool {
		// Get operational state for each rig
		opStateI, _ := getRigOperationalState(townRoot, rigNames[i])
		opStateJ, _ := getRigOperationalState(townRoot, rigNames[j])

		// Determine if each rig is parked/stopped
		iParked := opStateI == "PARKED" || opStateI == "DOCKED"
		jParked := opStateJ == "PARKED" || opStateJ == "DOCKED"

		// If both are same state (both parked or both active), sort alphabetically
		if iParked == jParked {
			return rigNames[i] < rigNames[j]
		}

		// Parked rigs go to the right (i.e., sort after active rigs)
		return !iParked && jParked
	})

	// Create sortable rig list
	type rigInfo struct {
		name   string
		status *rigStatus
	}
	var rigs []rigInfo
	for rigName, status := range rigStatuses {
		rigs = append(rigs, rigInfo{name: rigName, status: status})
	}

	// Sort by: 1) running state, 2) polecat count (desc), 3) operational state, 4) alphabetical
	sort.Slice(rigs, func(i, j int) bool {
		isRunningI := rigs[i].status.hasWitness || rigs[i].status.hasRefinery
		isRunningJ := rigs[j].status.hasWitness || rigs[j].status.hasRefinery

		// Primary sort: running rigs before non-running rigs
		if isRunningI != isRunningJ {
			return isRunningI
		}

		// Secondary sort: polecat count (descending)
		if rigs[i].status.polecatCount != rigs[j].status.polecatCount {
			return rigs[i].status.polecatCount > rigs[j].status.polecatCount
		}

		// Tertiary sort: operational state (for non-running rigs: OPERATIONAL < PARKED < DOCKED)
		stateOrder := map[string]int{"OPERATIONAL": 0, "PARKED": 1, "DOCKED": 2}
		stateI := stateOrder[rigs[i].status.opState]
		stateJ := stateOrder[rigs[j].status.opState]
		if stateI != stateJ {
			return stateI < stateJ
		}

		// Quaternary sort: alphabetical
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
		rigParts = append(rigParts, led+abbreviateRigName(rigName))
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
		parts = append(parts, fmt.Sprintf("🪝%s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot("mayor/", 45, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%d", unread))
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
	polecatCount := 0
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		// Only count registered rigs
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			rigs[agent.Rig] = true
		}
		if agent.Type == AgentPolecat && registeredRigs[agent.Rig] {
			polecatCount++
		}
	}
	rigCount := len(rigs)

	// Build status
	var parts []string
	parts = append(parts, fmt.Sprintf("%d rigs", rigCount))
	parts = append(parts, fmt.Sprintf("%d😺", polecatCount))

	// Priority 1: Check for hooked work (town beads for deacon)
	hookedWork := ""
	if townRoot != "" {
		hookedWork = getHookedWork("deacon", 35, townRoot)
	}
	if hookedWork != "" {
		parts = append(parts, fmt.Sprintf("🪝%s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot("deacon/", 40, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runWitnessStatusLine outputs status for a witness session.
// Shows: polecat count, crew count, hook or mail preview
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

	// Count polecats and crew in this rig
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	polecatCount := 0
	crewCount := 0
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		if agent.Rig == rigName {
			if agent.Type == AgentPolecat {
				polecatCount++
			} else if agent.Type == AgentCrew {
				crewCount++
			}
		}
	}

	identity := fmt.Sprintf("%s/witness", abbreviateRigName(rigName))

	// Build status
	var parts []string
	parts = append(parts, fmt.Sprintf("%d😺", polecatCount))
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
		parts = append(parts, fmt.Sprintf("🪝%s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot(identity, 35, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%d", unread))
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

	identity := fmt.Sprintf("%s/refinery", abbreviateRigName(rigName))

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
		parts = append(parts, fmt.Sprintf("🪝%s", hookedWork))
	} else if townRoot != "" {
		// Priority 2: Fall back to mail preview
		unread, subject := getMailPreviewWithRoot(identity, 30, townRoot)
		if unread > 0 {
			if subject != "" {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%s", subject))
			} else {
				parts = append(parts, fmt.Sprintf("\U0001F4EC%d", unread))
			}
		}
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// getSessionState detects the state of a Claude Code session by capturing pane content.
// Returns "working" if Claude is active (✻ symbol present), "idle" if prompt is visible,
// or "unknown" if the state cannot be determined.
func getSessionState(t *tmux.Tmux, session string) string {
	// Capture last few lines of the pane
	lines, err := t.CapturePaneLines(session, 5)
	if err != nil || len(lines) == 0 {
		return "unknown"
	}

	// Check last visible line for state indicators
	lastLine := strings.TrimSpace(lines[len(lines)-1])

	// ✻ indicates Claude is working (varies: Incubating, Symbioting, Shenaniganing, etc.)
	if strings.Contains(lastLine, "✻") || strings.Contains(lastLine, "✻") {
		return "working"
	}

	// ❯ prompt indicates Claude is idle, waiting for input
	if strings.HasSuffix(lastLine, "❯") || strings.HasSuffix(lastLine, "❯") {
		return "idle"
	}

	// Check second-to-last line as well (prompt might be on previous line)
	if len(lines) >= 2 {
		prevLine := strings.TrimSpace(lines[len(lines)-2])
		if strings.HasSuffix(prevLine, "❯") || strings.HasSuffix(prevLine, "❯") {
			return "idle"
		}
		if strings.Contains(prevLine, "✻") || strings.Contains(prevLine, "✻") {
			return "working"
		}
	}

	// Check all lines for working indicator (in case it's not near the end)
	for _, line := range lines {
		if strings.Contains(line, "✻") || strings.Contains(line, "✻") {
			return "working"
		}
	}

	// Default to unknown if we can't determine state
	return "unknown"
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
