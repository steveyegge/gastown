package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

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
	"golang.org/x/term"
)

var statusJSON bool
var statusFast bool
var statusWatch bool
var statusInterval int
var statusVerbose bool

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"stat"},
	GroupID: GroupDiag,
	Short:   "Show overall town status",
	Long: `Display the current status of the Gas Town workspace.

Shows town name, registered rigs, active polecats, and witness status.

Use --fast to skip mail lookups for faster execution.
Use --watch to continuously refresh status at regular intervals.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
	statusCmd.Flags().BoolVar(&statusFast, "fast", false, "Skip mail lookups for faster execution")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Watch mode: refresh status continuously")
	statusCmd.Flags().IntVarP(&statusInterval, "interval", "n", 2, "Refresh interval in seconds")
	statusCmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "Show detailed multi-line output per agent")
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
	if statusWatch {
		return runStatusWatch(cmd, args)
	}
	return runStatusOnce(cmd, args)
}

func runStatusWatch(cmd *cobra.Command, args []string) error {
	if statusJSON {
		return fmt.Errorf("--json and --watch cannot be used together")
	}
	if statusInterval <= 0 {
		return fmt.Errorf("interval must be positive, got %d", statusInterval)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	ticker := time.NewTicker(time.Duration(statusInterval) * time.Second)
	defer ticker.Stop()

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	for {
		if isTTY {
			fmt.Print("\033[H\033[2J") // ANSI: cursor home + clear screen
		}

		timestamp := time.Now().Format("15:04:05")
		header := fmt.Sprintf("[%s] gt status --watch (every %ds, Ctrl+C to stop)", timestamp, statusInterval)
		if isTTY {
			fmt.Printf("%s\n\n", style.Dim.Render(header))
		} else {
			fmt.Printf("%s\n\n", header)
		}

		if err := runStatusOnce(cmd, args); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		select {
		case <-sigChan:
			if isTTY {
				fmt.Println("\nStopped.")
			}
			return nil
		case <-ticker.C:
		}
	}
}

func runStatusOnce(_ *cobra.Command, _ []string) error {
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
		fmt.Printf("  Run 'bd daemon killall && bd daemon start' to restart daemons\n")
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
		constants.RoleBoot:     constants.EmojiBoot,
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
		if statusVerbose {
			fmt.Printf("%s %s\n", icon, style.Bold.Render(capitalizeFirst(agent.Name)))
			renderAgentDetails(agent, "   ", nil, status.Location)
			fmt.Println()
		} else {
			// Compact: icon + name on one line
			renderAgentCompact(agent, icon+" ", nil, status.Location)
		}
	}
	if !statusVerbose && len(status.Agents) > 0 {
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
			if statusVerbose {
				fmt.Printf("%s %s\n", roleIcons["witness"], style.Bold.Render("Witness"))
				for _, agent := range witnesses {
					renderAgentDetails(agent, "   ", r.Hooks, status.Location)
				}
				fmt.Println()
			} else {
				for _, agent := range witnesses {
					renderAgentCompact(agent, roleIcons["witness"]+" ", r.Hooks, status.Location)
				}
			}
		}

		// Refinery
		if len(refineries) > 0 {
			if statusVerbose {
				fmt.Printf("%s %s\n", roleIcons["refinery"], style.Bold.Render("Refinery"))
				for _, agent := range refineries {
					renderAgentDetails(agent, "   ", r.Hooks, status.Location)
				}
				// MQ summary (shown under refinery)
				if r.MQ != nil {
					mqStr := formatMQSummary(r.MQ)
					if mqStr != "" {
						fmt.Printf("   MQ: %s\n", mqStr)
					}
				}
				fmt.Println()
			} else {
				for _, agent := range refineries {
					// Compact: include MQ on same line if present
					mqSuffix := ""
					if r.MQ != nil {
						mqStr := formatMQSummaryCompact(r.MQ)
						if mqStr != "" {
							mqSuffix = "  " + mqStr
						}
					}
					renderAgentCompactWithSuffix(agent, roleIcons["refinery"]+" ", r.Hooks, status.Location, mqSuffix)
				}
			}
		}

		// Crew
		if len(crews) > 0 {
			if statusVerbose {
				fmt.Printf("%s %s (%d)\n", roleIcons["crew"], style.Bold.Render("Crew"), len(crews))
				for _, agent := range crews {
					renderAgentDetails(agent, "   ", r.Hooks, status.Location)
				}
				fmt.Println()
			} else {
				fmt.Printf("%s %s (%d)\n", roleIcons["crew"], style.Bold.Render("Crew"), len(crews))
				for _, agent := range crews {
					renderAgentCompact(agent, "   ", r.Hooks, status.Location)
				}
			}
		}

		// Polecats
		if len(polecats) > 0 {
			if statusVerbose {
				fmt.Printf("%s %s (%d)\n", roleIcons["polecat"], style.Bold.Render("Polecats"), len(polecats))
				for _, agent := range polecats {
					renderAgentDetails(agent, "   ", r.Hooks, status.Location)
				}
				fmt.Println()
			} else {
				fmt.Printf("%s %s (%d)\n", roleIcons["polecat"], style.Bold.Render("Polecats"), len(polecats))
				for _, agent := range polecats {
					renderAgentCompact(agent, "   ", r.Hooks, status.Location)
				}
			}
		}

		// No agents
		if len(witnesses) == 0 && len(refineries) == 0 && len(crews) == 0 && len(polecats) == 0 {
			fmt.Printf("   %s\n", style.Dim.Render("(no agents)"))
		}
		fmt.Println()
	}

	return nil
}

// renderAgentDetails renders full agent bead details
func renderAgentDetails(agent AgentRuntime, indent string, hooks []AgentHookInfo, townRoot string) { //nolint:unparam // indent kept for future customization
	// Line 1: Agent bead ID + status
	// Per gt-zecmc: derive status from tmux (observable reality), not bead state.
	// "Discover, don't track" - agent liveness is observable from tmux session.
	sessionExists := agent.Running

	var statusStr string
	var stateInfo string

	if sessionExists {
		statusStr = style.Success.Render("running")
	} else {
		statusStr = style.Error.Render("stopped")
	}

	// Show non-observable states that represent intentional agent decisions.
	// These can't be discovered from tmux and are legitimately recorded in beads.
	beadState := agent.State
	switch beadState {
	case "stuck":
		// Agent escalated - needs help
		stateInfo = style.Warning.Render(" [stuck]")
	case "awaiting-gate":
		// Agent waiting for external trigger (phase gate)
		stateInfo = style.Dim.Render(" [awaiting-gate]")
	case "muted", "paused", "degraded":
		// Other intentional non-observable states
		stateInfo = style.Dim.Render(fmt.Sprintf(" [%s]", beadState))
	// Ignore observable states: "running", "idle", "dead", "done", "stopped", ""
	// These should be derived from tmux, not bead.
	}

	// Build agent bead ID using canonical naming: prefix-rig-role-name
	agentBeadID := "gt-" + agent.Name
	if agent.Address != "" && agent.Address != agent.Name {
		// Use address for full path agents like gastown/crew/joe → gt-gastown-crew-joe
		addr := strings.TrimSuffix(agent.Address, "/") // Remove trailing slash for global agents
		parts := strings.Split(addr, "/")
		if len(parts) == 1 {
			// Global agent: mayor/, deacon/ → hq-mayor, hq-deacon
			agentBeadID = beads.AgentBeadIDWithPrefix(beads.TownBeadsPrefix, "", parts[0], "")
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
				// polecat: rig/name - uses hq- prefix for town beads (fix for gt-myc)
				agentBeadID = beads.PolecatBeadIDTown(rig, parts[1])
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

// formatMQSummary formats the MQ status for verbose display
func formatMQSummary(mq *MQSummary) string {
	if mq == nil {
		return ""
	}
	mqParts := []string{}
	if mq.Pending > 0 {
		mqParts = append(mqParts, fmt.Sprintf("%d pending", mq.Pending))
	}
	if mq.InFlight > 0 {
		mqParts = append(mqParts, style.Warning.Render(fmt.Sprintf("%d in-flight", mq.InFlight)))
	}
	if mq.Blocked > 0 {
		mqParts = append(mqParts, style.Dim.Render(fmt.Sprintf("%d blocked", mq.Blocked)))
	}
	if len(mqParts) == 0 {
		return ""
	}
	// Add state indicator
	stateIcon := "○" // idle
	switch mq.State {
	case "processing":
		stateIcon = style.Success.Render("●")
	case "blocked":
		stateIcon = style.Error.Render("○")
	}
	// Add health warning if stale
	healthSuffix := ""
	if mq.Health == "stale" {
		healthSuffix = style.Error.Render(" [stale]")
	}
	return fmt.Sprintf("%s %s%s", stateIcon, strings.Join(mqParts, ", "), healthSuffix)
}

// formatMQSummaryCompact formats MQ status for compact single-line display
func formatMQSummaryCompact(mq *MQSummary) string {
	if mq == nil {
		return ""
	}
	// Very compact: "MQ:12" or "MQ:12 [stale]"
	total := mq.Pending + mq.InFlight + mq.Blocked
	if total == 0 {
		return ""
	}
	healthSuffix := ""
	if mq.Health == "stale" {
		healthSuffix = style.Error.Render("[stale]")
	}
	return fmt.Sprintf("MQ:%d%s", total, healthSuffix)
}

// renderAgentCompactWithSuffix renders a single-line agent status with an extra suffix
func renderAgentCompactWithSuffix(agent AgentRuntime, indent string, hooks []AgentHookInfo, _ string, suffix string) {
	// Build status indicator (gt-zecmc: use tmux state, not bead state)
	statusIndicator := buildStatusIndicator(agent)

	// Get hook info
	hookBead := agent.HookBead
	hookTitle := agent.WorkTitle
	if hookBead == "" && hooks != nil {
		for _, h := range hooks {
			if h.Agent == agent.Address && h.HasWork {
				hookBead = h.Molecule
				hookTitle = h.Title
				break
			}
		}
	}

	// Build hook suffix
	hookSuffix := ""
	if hookBead != "" {
		if hookTitle != "" {
			hookSuffix = style.Dim.Render(" → ") + truncateWithEllipsis(hookTitle, 30)
		} else {
			hookSuffix = style.Dim.Render(" → ") + hookBead
		}
	} else if hookTitle != "" {
		hookSuffix = style.Dim.Render(" → ") + truncateWithEllipsis(hookTitle, 30)
	}

	// Mail indicator
	mailSuffix := ""
	if agent.UnreadMail > 0 {
		mailSuffix = fmt.Sprintf(" 📬%d", agent.UnreadMail)
	}

	// Print single line: name + status + hook + mail + suffix
	fmt.Printf("%s%-12s %s%s%s%s\n", indent, agent.Name, statusIndicator, hookSuffix, mailSuffix, suffix)
}

// renderAgentCompact renders a single-line agent status
func renderAgentCompact(agent AgentRuntime, indent string, hooks []AgentHookInfo, _ string) {
	// Build status indicator (gt-zecmc: use tmux state, not bead state)
	statusIndicator := buildStatusIndicator(agent)

	// Get hook info
	hookBead := agent.HookBead
	hookTitle := agent.WorkTitle
	if hookBead == "" && hooks != nil {
		for _, h := range hooks {
			if h.Agent == agent.Address && h.HasWork {
				hookBead = h.Molecule
				hookTitle = h.Title
				break
			}
		}
	}

	// Build hook suffix
	hookSuffix := ""
	if hookBead != "" {
		if hookTitle != "" {
			hookSuffix = style.Dim.Render(" → ") + truncateWithEllipsis(hookTitle, 30)
		} else {
			hookSuffix = style.Dim.Render(" → ") + hookBead
		}
	} else if hookTitle != "" {
		hookSuffix = style.Dim.Render(" → ") + truncateWithEllipsis(hookTitle, 30)
	}

	// Mail indicator
	mailSuffix := ""
	if agent.UnreadMail > 0 {
		mailSuffix = fmt.Sprintf(" 📬%d", agent.UnreadMail)
	}

	// Print single line: name + status + hook + mail
	fmt.Printf("%s%-12s %s%s%s\n", indent, agent.Name, statusIndicator, hookSuffix, mailSuffix)
}

// buildStatusIndicator creates the visual status indicator for an agent.
// Per gt-zecmc: uses tmux state (observable reality), not bead state.
// Non-observable states (stuck, awaiting-gate, muted, etc.) are shown as suffixes.
func buildStatusIndicator(agent AgentRuntime) string {
	sessionExists := agent.Running

	// Base indicator from tmux state
	var indicator string
	if sessionExists {
		indicator = style.Success.Render("●")
	} else {
		indicator = style.Error.Render("○")
	}

	// Add non-observable state suffix if present
	beadState := agent.State
	switch beadState {
	case "stuck":
		indicator += style.Warning.Render(" stuck")
	case "awaiting-gate":
		indicator += style.Dim.Render(" gate")
	case "muted", "paused", "degraded":
		indicator += style.Dim.Render(" " + beadState)
	// Ignore observable states: running, idle, dead, done, stopped, ""
	}

	return indicator
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
	bootSession := getBootSessionName()

	// Define agents to discover
	// Note: Mayor, Deacon, and Boot are town-level agents with hq- prefix bead IDs
	agentDefs := []struct {
		name    string
		address string
		session string
		role    string
		beadID  string
	}{
		{"mayor", "mayor/", mayorSession, "coordinator", beads.MayorBeadIDTown()},
		{"deacon", "deacon/", deaconSession, "health-check", beads.DeaconBeadIDTown()},
		{"boot", "boot/", bootSession, "boot", beads.BootBeadIDTown()},
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

	// Polecats - use hq- prefix for town beads (fix for gt-myc)
	for _, name := range r.Polecats {
		defs = append(defs, agentDef{
			name:    name,
			address: r.Name + "/" + name,
			session: fmt.Sprintf("gt-%s-%s", r.Name, name),
			role:    "polecat",
			beadID:  beads.PolecatBeadIDTown(r.Name, name),
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

	// Query for all open MRs by label (bd-3q6.10: MRs now use type=task)
	opts := beads.ListOptions{
		Label:    "gt:merge-request",
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
