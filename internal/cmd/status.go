package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var statusJSON bool
var statusFast bool

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"stat"},
	GroupID: GroupDiag,
	Short:   "Show overall town status",
	Long: `Display the current status of the Gas Town workspace.

Shows town name, registered rigs, active polecats, and witness status.

Use --fast to skip mail lookups for faster execution.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
	statusCmd.Flags().BoolVar(&statusFast, "fast", false, "Skip mail lookups for faster execution")
	rootCmd.AddCommand(statusCmd)
}

// TownStatus represents the overall status of the workspace.
type TownStatus struct {
	Name     string         `json:"name"`
	Location string         `json:"location"`
	Overseer *OverseerInfo  `json:"overseer,omitempty"` // Human operator
	Agents   []AgentRuntime `json:"agents"`             // Global agents (Mayor, Deacon)
	Rigs     []RigStatus    `json:"rigs"`
	Summary  StatusSum      `json:"summary"`
}

// OverseerInfo represents the human operator's identity and status.
type OverseerInfo struct {
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Username   string `json:"username,omitempty"`
	Source     string `json:"source"`
	UnreadMail int    `json:"unread_mail"`
}

// AgentRuntime represents the runtime state of an agent.
type AgentRuntime struct {
	Name         string `json:"name"`                    // Display name (e.g., "mayor", "witness")
	Address      string `json:"address"`                 // Full address (e.g., "greenplace/witness")
	Session      string `json:"session"`                 // tmux session name
	Role         string `json:"role"`                    // Role type
	Running      bool   `json:"running"`                 // Is tmux session running?
	HasWork      bool   `json:"has_work"`                // Has pinned work?
	WorkTitle    string `json:"work_title,omitempty"`    // Title of pinned work
	HookBead     string `json:"hook_bead,omitempty"`     // Pinned bead ID from agent bead
	State        string `json:"state,omitempty"`         // Agent state from agent bead
	UnreadMail   int    `json:"unread_mail"`             // Number of unread messages
	FirstSubject string `json:"first_subject,omitempty"` // Subject of first unread message
}

// RigStatus represents status of a single rig.
type RigStatus struct {
	Name         string          `json:"name"`
	Polecats     []string        `json:"polecats"`
	PolecatCount int             `json:"polecat_count"`
	Crews        []string        `json:"crews"`
	CrewCount    int             `json:"crew_count"`
	HasWitness   bool            `json:"has_witness"`
	HasRefinery  bool            `json:"has_refinery"`
	Hooks        []AgentHookInfo `json:"hooks,omitempty"`
	Agents       []AgentRuntime  `json:"agents,omitempty"` // Runtime state of all agents in rig
	MQ           *MQSummary      `json:"mq,omitempty"`     // Merge queue summary
}

// MQSummary represents the merge queue status for a rig.
type MQSummary struct {
	Pending  int    `json:"pending"`   // Open MRs ready to merge (no blockers)
	InFlight int    `json:"in_flight"` // MRs currently being processed
	Blocked  int    `json:"blocked"`   // MRs waiting on dependencies
	State    string `json:"state"`     // idle, processing, or blocked
	Health   string `json:"health"`    // healthy, stale, or empty
}

// AgentHookInfo represents an agent's hook (pinned work) status.
type AgentHookInfo struct {
	Agent    string `json:"agent"`              // Agent address (e.g., "greenplace/toast", "greenplace/witness")
	Role     string `json:"role"`               // Role type (polecat, crew, witness, refinery)
	HasWork  bool   `json:"has_work"`           // Whether agent has pinned work
	Molecule string `json:"molecule,omitempty"` // Attached molecule ID
	Title    string `json:"title,omitempty"`    // Pinned bead title
}

// StatusSum provides summary counts.
type StatusSum struct {
	RigCount      int `json:"rig_count"`
	PolecatCount  int `json:"polecat_count"`
	CrewCount     int `json:"crew_count"`
	WitnessCount  int `json:"witness_count"`
	RefineryCount int `json:"refinery_count"`
	ActiveHooks   int `json:"active_hooks"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Check bd daemon health and attempt restart if needed
	// This is non-blocking - if daemons can't be started, we show a warning but continue
	bdWarning := beads.EnsureBdDaemonHealth(townRoot)

	// Load town config
	townConfigPath := constants.MayorTownPath(townRoot)
	townConfig, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		// Try to continue without config
		townConfig = &config.TownConfig{Name: filepath.Base(townRoot)}
	}

	// Load rigs config
	rigsConfigPath := constants.MayorRigsPath(townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		// Empty config if file doesn't exist
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Create rig manager
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	// Create tmux instance for runtime checks
	t := tmux.NewTmux()

	// Pre-fetch all tmux sessions for O(1) lookup
	allSessions := make(map[string]bool)
	if sessions, err := t.ListSessions(); err == nil {
		for _, s := range sessions {
			allSessions[s] = true
		}
	}

	// Discover rigs
	rigs, err := mgr.DiscoverRigs()
	if err != nil {
		return fmt.Errorf("discovering rigs: %w", err)
	}

	// Pre-fetch agent beads across all rig-specific beads DBs.
	allAgentBeads := make(map[string]*beads.Issue)
	allHookBeads := make(map[string]*beads.Issue)

	// Fetch town-level agent beads (Mayor, Deacon) from town beads
	townBeadsPath := beads.GetTownBeadsPath(townRoot)
	townBeadsClient := beads.New(townBeadsPath)
	townAgentBeads, _ := townBeadsClient.ListAgentBeads()
	for id, issue := range townAgentBeads {
		allAgentBeads[id] = issue
	}

	// Fetch hook beads from town beads
	var townHookIDs []string
	for _, issue := range townAgentBeads {
		hookID := issue.HookBead
		if hookID == "" {
			fields := beads.ParseAgentFields(issue.Description)
			if fields != nil {
				hookID = fields.HookBead
			}
		}
		if hookID != "" {
			townHookIDs = append(townHookIDs, hookID)
		}
	}
	if len(townHookIDs) > 0 {
		townHookBeads, _ := townBeadsClient.ShowMultiple(townHookIDs)
		for id, issue := range townHookBeads {
			allHookBeads[id] = issue
		}
	}

	// Fetch rig-level agent beads
	for _, r := range rigs {
		rigBeadsPath := filepath.Join(r.Path, "mayor", "rig")
		rigBeads := beads.New(rigBeadsPath)
		rigAgentBeads, _ := rigBeads.ListAgentBeads()
		if rigAgentBeads == nil {
			continue
		}
		for id, issue := range rigAgentBeads {
			allAgentBeads[id] = issue
		}

		var hookIDs []string
		for _, issue := range rigAgentBeads {
			// Use the HookBead field from the database column; fall back for legacy beads.
			hookID := issue.HookBead
			if hookID == "" {
				fields := beads.ParseAgentFields(issue.Description)
				if fields != nil {
					hookID = fields.HookBead
				}
			}
			if hookID != "" {
				hookIDs = append(hookIDs, hookID)
			}
		}

		if len(hookIDs) == 0 {
			continue
		}
		hookBeads, _ := rigBeads.ShowMultiple(hookIDs)
		for id, issue := range hookBeads {
			allHookBeads[id] = issue
		}
	}

	// Create mail router for inbox lookups
	mailRouter := mail.NewRouter(townRoot)

	// Load overseer config
	var overseerInfo *OverseerInfo
	if overseerConfig, err := config.LoadOrDetectOverseer(townRoot); err == nil && overseerConfig != nil {
		overseerInfo = &OverseerInfo{
			Name:     overseerConfig.Name,
			Email:    overseerConfig.Email,
			Username: overseerConfig.Username,
			Source:   overseerConfig.Source,
		}
		// Get overseer mail count
		if mailbox, err := mailRouter.GetMailbox("overseer"); err == nil {
			_, unread, _ := mailbox.Count()
			overseerInfo.UnreadMail = unread
		}
	}

	// Build status - parallel fetch global agents and rigs
	status := TownStatus{
		Name:     townConfig.Name,
		Location: townRoot,
		Overseer: overseerInfo,
		Rigs:     make([]RigStatus, len(rigs)),
	}

	var wg sync.WaitGroup

	// Fetch global agents in parallel with rig discovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		status.Agents = discoverGlobalAgents(allSessions, allAgentBeads, allHookBeads, mailRouter, statusFast)
	}()

	// Process all rigs in parallel
	rigActiveHooks := make([]int, len(rigs)) // Track hooks per rig for thread safety
	for i, r := range rigs {
		wg.Add(1)
		go func(idx int, r *rig.Rig) {
			defer wg.Done()

			rs := RigStatus{
				Name:         r.Name,
				Polecats:     r.Polecats,
				PolecatCount: len(r.Polecats),
				HasWitness:   r.HasWitness,
				HasRefinery:  r.HasRefinery,
			}

			// Count crew workers
			crewGit := git.NewGit(r.Path)
			crewMgr := crew.NewManager(r, crewGit)
			if workers, err := crewMgr.List(); err == nil {
				for _, w := range workers {
					rs.Crews = append(rs.Crews, w.Name)
				}
				rs.CrewCount = len(workers)
			}

			// Discover hooks for all agents in this rig
			rs.Hooks = discoverRigHooks(r, rs.Crews)
			activeHooks := 0
			for _, hook := range rs.Hooks {
				if hook.HasWork {
					activeHooks++
				}
			}
			rigActiveHooks[idx] = activeHooks

			// Discover runtime state for all agents in this rig
			rs.Agents = discoverRigAgents(allSessions, r, rs.Crews, allAgentBeads, allHookBeads, mailRouter, statusFast)

			// Get MQ summary if rig has a refinery
			rs.MQ = getMQSummary(r)

			status.Rigs[idx] = rs
		}(i, r)
	}

	wg.Wait()

	// Aggregate summary (after parallel work completes)
	for i, rs := range status.Rigs {
		status.Summary.PolecatCount += rs.PolecatCount
		status.Summary.CrewCount += rs.CrewCount
		status.Summary.ActiveHooks += rigActiveHooks[i]
		if rs.HasWitness {
			status.Summary.WitnessCount++
		}
		if rs.HasRefinery {
			status.Summary.RefineryCount++
		}
	}
	status.Summary.RigCount = len(rigs)

	// Output
	if statusJSON {
		return outputStatusJSON(status)
	}
	if err := outputStatusText(status); err != nil {
		return err
	}

	// Show bd daemon warning at the end if there were issues
	if bdWarning != "" {
		fmt.Printf("%s %s\n", style.Warning.Render("⚠"), bdWarning)
		fmt.Printf("  Run 'bd daemon killall && bd daemon --start' to restart daemons\n")
	}

	return nil
}

func outputStatusJSON(status TownStatus) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func outputStatusText(status TownStatus) error {
	// Header
	fmt.Printf("%s %s\n", style.Bold.Render("Town:"), status.Name)
	fmt.Printf("%s\n\n", style.Dim.Render(status.Location))

	// Overseer info
	if status.Overseer != nil {
		overseerDisplay := status.Overseer.Name
		if status.Overseer.Email != "" {
			overseerDisplay = fmt.Sprintf("%s <%s>", status.Overseer.Name, status.Overseer.Email)
		} else if status.Overseer.Username != "" && status.Overseer.Username != status.Overseer.Name {
			overseerDisplay = fmt.Sprintf("%s (@%s)", status.Overseer.Name, status.Overseer.Username)
		}
		fmt.Printf("👤 %s %s\n", style.Bold.Render("Overseer:"), overseerDisplay)
		if status.Overseer.UnreadMail > 0 {
			fmt.Printf("   📬 %d unread\n", status.Overseer.UnreadMail)
		}
		fmt.Println()
	}

	// Role icons - uses centralized emojis from constants package
	roleIcons := map[string]string{
		constants.RoleMayor:    constants.EmojiMayor,
		constants.RoleDeacon:   constants.EmojiDeacon,
		constants.RoleWitness:  constants.EmojiWitness,
		constants.RoleRefinery: constants.EmojiRefinery,
		constants.RoleCrew:     constants.EmojiCrew,
		constants.RolePolecat:  constants.EmojiPolecat,
		// Legacy names for backwards compatibility
		"coordinator":  constants.EmojiMayor,
		"health-check": constants.EmojiDeacon,
	}

	// Global Agents (Mayor, Deacon)
	for _, agent := range status.Agents {
		icon := roleIcons[agent.Role]
		if icon == "" {
			icon = roleIcons[agent.Name]
		}
		fmt.Printf("%s %s\n", icon, style.Bold.Render(capitalizeFirst(agent.Name)))
		renderAgentDetails(agent, "   ", nil, status.Location)
		fmt.Println()
	}

	if len(status.Rigs) == 0 {
		fmt.Printf("%s\n", style.Dim.Render("No rigs registered. Use 'gt rig add' to add one."))
		return nil
	}

	// Rigs
	for _, r := range status.Rigs {
		// Rig header with separator
		fmt.Printf("─── %s ───────────────────────────────────────────\n\n", style.Bold.Render(r.Name+"/"))

		// Group agents by role
		var witnesses, refineries, crews, polecats []AgentRuntime
		for _, agent := range r.Agents {
			switch agent.Role {
			case "witness":
				witnesses = append(witnesses, agent)
			case "refinery":
				refineries = append(refineries, agent)
			case "crew":
				crews = append(crews, agent)
			case "polecat":
				polecats = append(polecats, agent)
			}
		}

		// Witness
		if len(witnesses) > 0 {
			fmt.Printf("%s %s\n", roleIcons["witness"], style.Bold.Render("Witness"))
			for _, agent := range witnesses {
				renderAgentDetails(agent, "   ", r.Hooks, status.Location)
			}
			fmt.Println()
		}

		// Refinery
		if len(refineries) > 0 {
			fmt.Printf("%s %s\n", roleIcons["refinery"], style.Bold.Render("Refinery"))
			for _, agent := range refineries {
				renderAgentDetails(agent, "   ", r.Hooks, status.Location)
			}
			// MQ summary (shown under refinery)
			if r.MQ != nil {
				mqParts := []string{}
				if r.MQ.Pending > 0 {
					mqParts = append(mqParts, fmt.Sprintf("%d pending", r.MQ.Pending))
				}
				if r.MQ.InFlight > 0 {
					mqParts = append(mqParts, style.Warning.Render(fmt.Sprintf("%d in-flight", r.MQ.InFlight)))
				}
				if r.MQ.Blocked > 0 {
					mqParts = append(mqParts, style.Dim.Render(fmt.Sprintf("%d blocked", r.MQ.Blocked)))
				}
				if len(mqParts) > 0 {
					// Add state indicator
					stateIcon := "○" // idle
					switch r.MQ.State {
					case "processing":
						stateIcon = style.Success.Render("●")
					case "blocked":
						stateIcon = style.Error.Render("○")
					}
					// Add health warning if stale
					healthSuffix := ""
					if r.MQ.Health == "stale" {
						healthSuffix = style.Error.Render(" [stale]")
					}
					fmt.Printf("   MQ: %s %s%s\n", stateIcon, strings.Join(mqParts, ", "), healthSuffix)
				}
			}
			fmt.Println()
		}

		// Crew
		if len(crews) > 0 {
			fmt.Printf("%s %s (%d)\n", roleIcons["crew"], style.Bold.Render("Crew"), len(crews))
			for _, agent := range crews {
				renderAgentDetails(agent, "   ", r.Hooks, status.Location)
			}
			fmt.Println()
		}

		// Polecats
		if len(polecats) > 0 {
			fmt.Printf("%s %s (%d)\n", roleIcons["polecat"], style.Bold.Render("Polecats"), len(polecats))
			for _, agent := range polecats {
				renderAgentDetails(agent, "   ", r.Hooks, status.Location)
			}
			fmt.Println()
		}

		// No agents
		if len(witnesses) == 0 && len(refineries) == 0 && len(crews) == 0 && len(polecats) == 0 {
			fmt.Printf("   %s\n\n", style.Dim.Render("(no agents)"))
		}
	}

	return nil
}

// renderAgentDetails renders full agent bead details
func renderAgentDetails(agent AgentRuntime, indent string, hooks []AgentHookInfo, townRoot string) { //nolint:unparam // indent kept for future customization
	// Line 1: Agent bead ID + status
	// Reconcile bead state with tmux session state to surface mismatches
	// States: "running" (active), "idle" (waiting), "stopped", "dead", etc.
	beadState := agent.State
	sessionExists := agent.Running

	// "idle" is a normal operational state (running but waiting for work)
	// Treat it the same as "running" for reconciliation purposes
	beadSaysRunning := beadState == "running" || beadState == "idle" || beadState == ""

	var statusStr string
	var stateInfo string

	switch {
	case beadSaysRunning && sessionExists:
		// Normal running state - session exists and bead agrees
		statusStr = style.Success.Render("running")
	case beadSaysRunning && !sessionExists:
		// Bead thinks running but session is gone - stale bead state
		statusStr = style.Error.Render("running")
		stateInfo = style.Warning.Render(" [dead]")
	case !beadSaysRunning && sessionExists:
		// Session exists but bead says stopped/dead - mismatch!
		// This is the key case: tmux says alive, bead says dead/stopped
		statusStr = style.Success.Render("running")
		stateInfo = style.Warning.Render(" [bead: " + beadState + "]")
	default:
		// Both agree: stopped
		statusStr = style.Error.Render("stopped")
	}

	// Add agent state info if not already shown and state is interesting
	// Skip "idle" and "running" as they're normal operational states
	if stateInfo == "" && beadState != "" && beadState != "idle" && beadState != "running" {
		stateInfo = style.Dim.Render(fmt.Sprintf(" [%s]", beadState))
	}

	// Build agent bead ID using canonical naming: prefix-rig-role-name
	agentBeadID := "gt-" + agent.Name
	if agent.Address != "" && agent.Address != agent.Name {
		// Use address for full path agents like gastown/crew/joe → gt-gastown-crew-joe
		addr := strings.TrimSuffix(agent.Address, "/") // Remove trailing slash for global agents
		parts := strings.Split(addr, "/")
		if len(parts) == 1 {
			// Global agent: mayor/, deacon/ → gt-mayor, gt-deacon
			agentBeadID = beads.AgentBeadID("", parts[0], "")
		} else if len(parts) >= 2 {
			rig := parts[0]
			prefix := beads.GetPrefixForRig(townRoot, rig)
			if parts[1] == "crew" && len(parts) >= 3 {
				agentBeadID = beads.CrewBeadIDWithPrefix(prefix, rig, parts[2])
			} else if parts[1] == "witness" {
				agentBeadID = beads.WitnessBeadIDWithPrefix(prefix, rig)
			} else if parts[1] == "refinery" {
				agentBeadID = beads.RefineryBeadIDWithPrefix(prefix, rig)
			} else if len(parts) == 2 {
				// polecat: rig/name
				agentBeadID = beads.PolecatBeadIDWithPrefix(prefix, rig, parts[1])
			}
		}
	}

	fmt.Printf("%s%s %s%s\n", indent, style.Dim.Render(agentBeadID), statusStr, stateInfo)

	// Line 2: Hook bead (pinned work)
	hookStr := style.Dim.Render("(none)")
	hookBead := agent.HookBead
	hookTitle := agent.WorkTitle

	// Fall back to hooks array if agent bead doesn't have hook info
	if hookBead == "" && hooks != nil {
		for _, h := range hooks {
			if h.Agent == agent.Address && h.HasWork {
				hookBead = h.Molecule
				hookTitle = h.Title
				break
			}
		}
	}

	if hookBead != "" {
		if hookTitle != "" {
			hookStr = fmt.Sprintf("%s → %s", hookBead, truncateWithEllipsis(hookTitle, 40))
		} else {
			hookStr = hookBead
		}
	} else if hookTitle != "" {
		// Has title but no molecule ID
		hookStr = truncateWithEllipsis(hookTitle, 50)
	}

	fmt.Printf("%s  hook: %s\n", indent, hookStr)

	// Line 3: Mail (if any unread)
	if agent.UnreadMail > 0 {
		mailStr := fmt.Sprintf("📬 %d unread", agent.UnreadMail)
		if agent.FirstSubject != "" {
			mailStr = fmt.Sprintf("📬 %d unread → %s", agent.UnreadMail, truncateWithEllipsis(agent.FirstSubject, 35))
		}
		fmt.Printf("%s  mail: %s\n", indent, mailStr)
	}
}

// formatHookInfo formats the hook bead and title for display
func formatHookInfo(hookBead, title string, maxLen int) string {
	if hookBead == "" {
		return ""
	}
	if title == "" {
		return fmt.Sprintf(" → %s", hookBead)
	}
	title = truncateWithEllipsis(title, maxLen)
	return fmt.Sprintf(" → %s", title)
}

// truncateWithEllipsis shortens a string to maxLen, adding "..." if truncated
func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// discoverRigHooks finds all hook attachments for agents in a rig.
// It scans polecats, crew workers, witness, and refinery for handoff beads.
func discoverRigHooks(r *rig.Rig, crews []string) []AgentHookInfo {
	var hooks []AgentHookInfo

	// Create beads instance for the rig
	b := beads.New(r.Path)

	// Check polecats
	for _, name := range r.Polecats {
		hook := getAgentHook(b, name, r.Name+"/"+name, "polecat")
		hooks = append(hooks, hook)
	}

	// Check crew workers
	for _, name := range crews {
		hook := getAgentHook(b, name, r.Name+"/crew/"+name, "crew")
		hooks = append(hooks, hook)
	}

	// Check witness
	if r.HasWitness {
		hook := getAgentHook(b, "witness", r.Name+"/witness", "witness")
		hooks = append(hooks, hook)
	}

	// Check refinery
	if r.HasRefinery {
		hook := getAgentHook(b, "refinery", r.Name+"/refinery", "refinery")
		hooks = append(hooks, hook)
	}

	return hooks
}

// discoverGlobalAgents checks runtime state for town-level agents (Mayor, Deacon).
// Uses parallel fetching for performance. If skipMail is true, mail lookups are skipped.
// allSessions is a preloaded map of tmux sessions for O(1) lookup.
// allAgentBeads is a preloaded map of agent beads for O(1) lookup.
// allHookBeads is a preloaded map of hook beads for O(1) lookup.
func discoverGlobalAgents(allSessions map[string]bool, allAgentBeads map[string]*beads.Issue, allHookBeads map[string]*beads.Issue, mailRouter *mail.Router, skipMail bool) []AgentRuntime {
	// Get session names dynamically
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()

	// Define agents to discover
	// Note: Mayor and Deacon are town-level agents with hq- prefix bead IDs
	agentDefs := []struct {
		name    string
		address string
		session string
		role    string
		beadID  string
	}{
		{"mayor", "mayor/", mayorSession, "coordinator", beads.MayorBeadIDTown()},
		{"deacon", "deacon/", deaconSession, "health-check", beads.DeaconBeadIDTown()},
	}

	agents := make([]AgentRuntime, len(agentDefs))
	var wg sync.WaitGroup

	for i, def := range agentDefs {
		wg.Add(1)
		go func(idx int, d struct {
			name    string
			address string
			session string
			role    string
			beadID  string
		}) {
			defer wg.Done()

			agent := AgentRuntime{
				Name:    d.name,
				Address: d.address,
				Session: d.session,
				Role:    d.role,
			}

			// Check tmux session from preloaded map (O(1))
			agent.Running = allSessions[d.session]

			// Look up agent bead from preloaded map (O(1))
			if issue, ok := allAgentBeads[d.beadID]; ok {
				// Prefer SQLite columns over description parsing
				// HookBead column is authoritative (cleared by unsling)
				agent.HookBead = issue.HookBead
				agent.State = issue.AgentState
				if agent.HookBead != "" {
					agent.HasWork = true
					// Get hook title from preloaded map
					if pinnedIssue, ok := allHookBeads[agent.HookBead]; ok {
						agent.WorkTitle = pinnedIssue.Title
					}
				}
				// Fallback to description for legacy beads without SQLite columns
				if agent.State == "" {
					fields := beads.ParseAgentFields(issue.Description)
					if fields != nil {
						agent.State = fields.AgentState
					}
				}
			}

			// Get mail info (skip if --fast)
			if !skipMail {
				populateMailInfo(&agent, mailRouter)
			}

			agents[idx] = agent
		}(i, def)
	}

	wg.Wait()
	return agents
}

// populateMailInfo fetches unread mail count and first subject for an agent
func populateMailInfo(agent *AgentRuntime, router *mail.Router) {
	if router == nil {
		return
	}
	mailbox, err := router.GetMailbox(agent.Address)
	if err != nil {
		return
	}
	_, unread, _ := mailbox.Count()
	agent.UnreadMail = unread
	if unread > 0 {
		if messages, err := mailbox.ListUnread(); err == nil && len(messages) > 0 {
			agent.FirstSubject = messages[0].Subject
		}
	}
}

// agentDef defines an agent to discover
type agentDef struct {
	name    string
	address string
	session string
	role    string
	beadID  string
}

// discoverRigAgents checks runtime state for all agents in a rig.
// Uses parallel fetching for performance. If skipMail is true, mail lookups are skipped.
// allSessions is a preloaded map of tmux sessions for O(1) lookup.
// allAgentBeads is a preloaded map of agent beads for O(1) lookup.
// allHookBeads is a preloaded map of hook beads for O(1) lookup.
func discoverRigAgents(allSessions map[string]bool, r *rig.Rig, crews []string, allAgentBeads map[string]*beads.Issue, allHookBeads map[string]*beads.Issue, mailRouter *mail.Router, skipMail bool) []AgentRuntime {
	// Build list of all agents to discover
	var defs []agentDef
	townRoot := filepath.Dir(r.Path)
	prefix := beads.GetPrefixForRig(townRoot, r.Name)

	// Witness
	if r.HasWitness {
		defs = append(defs, agentDef{
			name:    "witness",
			address: r.Name + "/witness",
			session: witnessSessionName(r.Name),
			role:    "witness",
			beadID:  beads.WitnessBeadIDWithPrefix(prefix, r.Name),
		})
	}

	// Refinery
	if r.HasRefinery {
		defs = append(defs, agentDef{
			name:    "refinery",
			address: r.Name + "/refinery",
			session: fmt.Sprintf("gt-%s-refinery", r.Name),
			role:    "refinery",
			beadID:  beads.RefineryBeadIDWithPrefix(prefix, r.Name),
		})
	}

	// Polecats
	for _, name := range r.Polecats {
		defs = append(defs, agentDef{
			name:    name,
			address: r.Name + "/" + name,
			session: fmt.Sprintf("gt-%s-%s", r.Name, name),
			role:    "polecat",
			beadID:  beads.PolecatBeadIDWithPrefix(prefix, r.Name, name),
		})
	}

	// Crew
	for _, name := range crews {
		defs = append(defs, agentDef{
			name:    name,
			address: r.Name + "/crew/" + name,
			session: crewSessionName(r.Name, name),
			role:    "crew",
			beadID:  beads.CrewBeadIDWithPrefix(prefix, r.Name, name),
		})
	}

	if len(defs) == 0 {
		return nil
	}

	// Fetch all agents in parallel
	agents := make([]AgentRuntime, len(defs))
	var wg sync.WaitGroup

	for i, def := range defs {
		wg.Add(1)
		go func(idx int, d agentDef) {
			defer wg.Done()

			agent := AgentRuntime{
				Name:    d.name,
				Address: d.address,
				Session: d.session,
				Role:    d.role,
			}

			// Check tmux session from preloaded map (O(1))
			agent.Running = allSessions[d.session]

			// Look up agent bead from preloaded map (O(1))
			if issue, ok := allAgentBeads[d.beadID]; ok {
				// Prefer SQLite columns over description parsing
				// HookBead column is authoritative (cleared by unsling)
				agent.HookBead = issue.HookBead
				agent.State = issue.AgentState
				if agent.HookBead != "" {
					agent.HasWork = true
					// Get hook title from preloaded map
					if pinnedIssue, ok := allHookBeads[agent.HookBead]; ok {
						agent.WorkTitle = pinnedIssue.Title
					}
				}
				// Fallback to description for legacy beads without SQLite columns
				if agent.State == "" {
					fields := beads.ParseAgentFields(issue.Description)
					if fields != nil {
						agent.State = fields.AgentState
					}
				}
			}

			// Get mail info (skip if --fast)
			if !skipMail {
				populateMailInfo(&agent, mailRouter)
			}

			agents[idx] = agent
		}(i, def)
	}

	wg.Wait()
	return agents
}

// getMQSummary queries beads for merge-request issues and returns a summary.
// Returns nil if the rig has no refinery or no MQ issues.
func getMQSummary(r *rig.Rig) *MQSummary {
	if !r.HasRefinery {
		return nil
	}

	// Create beads instance for the rig
	b := beads.New(r.BeadsPath())

	// Query for all open merge-request type issues
	opts := beads.ListOptions{
		Type:     "merge-request",
		Status:   "open",
		Priority: -1, // No priority filter
	}
	openMRs, err := b.List(opts)
	if err != nil {
		return nil
	}

	// Query for in-progress merge-requests
	opts.Status = "in_progress"
	inProgressMRs, err := b.List(opts)
	if err != nil {
		return nil
	}

	// Count pending (open with no blockers) vs blocked
	pending := 0
	blocked := 0
	for _, mr := range openMRs {
		if len(mr.BlockedBy) > 0 || mr.BlockedByCount > 0 {
			blocked++
		} else {
			pending++
		}
	}

	// Determine queue state
	state := "idle"
	if len(inProgressMRs) > 0 {
		state = "processing"
	} else if pending > 0 {
		state = "idle" // Has work but not processing yet
	} else if blocked > 0 {
		state = "blocked" // Only blocked items, nothing processable
	}

	// Determine queue health
	health := "empty"
	total := pending + len(inProgressMRs) + blocked
	if total > 0 {
		health = "healthy"
		// Check for potential issues
		if pending > 10 && len(inProgressMRs) == 0 {
			// Large queue but nothing processing - may be stuck
			health = "stale"
		}
	}

	// Only return summary if there's something to show
	if pending == 0 && len(inProgressMRs) == 0 && blocked == 0 {
		return nil
	}

	return &MQSummary{
		Pending:  pending,
		InFlight: len(inProgressMRs),
		Blocked:  blocked,
		State:    state,
		Health:   health,
	}
}

// getAgentHook retrieves hook status for a specific agent.
func getAgentHook(b *beads.Beads, role, agentAddress, roleType string) AgentHookInfo {
	hook := AgentHookInfo{
		Agent: agentAddress,
		Role:  roleType,
	}

	// Find handoff bead for this role
	handoff, err := b.FindHandoffBead(role)
	if err != nil || handoff == nil {
		return hook
	}

	// Check for attachment
	attachment := beads.ParseAttachmentFields(handoff)
	if attachment != nil && attachment.AttachedMolecule != "" {
		hook.HasWork = true
		hook.Molecule = attachment.AttachedMolecule
		hook.Title = handoff.Title
	} else if handoff.Description != "" {
		// Has content but no molecule - still has work
		hook.HasWork = true
		hook.Title = handoff.Title
	}

	return hook
}
