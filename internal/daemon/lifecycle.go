package daemon

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentpkg "github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/rig"
)

// BeadsMessage represents a message from gt mail inbox --json.
type BeadsMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Priority  string `json:"priority"`
	Type      string `json:"type"`
}

// MaxLifecycleMessageAge is the maximum age of a lifecycle message before it's ignored.
// Messages older than this are considered stale and deleted without execution.
const MaxLifecycleMessageAge = 6 * time.Hour

// ProcessLifecycleRequests checks for and processes lifecycle requests from the deacon inbox.
func (d *Daemon) ProcessLifecycleRequests() {
	// Get mail for deacon identity (using gt mail, not bd mail)
	cmd := exec.Command("gt", "mail", "inbox", "--identity", "deacon/", "--json")
	cmd.Dir = d.config.TownRoot

	output, err := cmd.Output()
	if err != nil {
		// gt mail might not be available or inbox empty
		return
	}

	if len(output) == 0 || string(output) == "[]" || string(output) == "[]\n" {
		return
	}

	var messages []BeadsMessage
	if err := json.Unmarshal(output, &messages); err != nil {
		d.logger.Printf("Error parsing mail: %v", err)
		return
	}

	for _, msg := range messages {
		if msg.Read {
			continue // Already processed
		}

		request := d.parseLifecycleRequest(&msg)
		if request == nil {
			continue // Not a lifecycle request
		}

		// Check message age - ignore stale lifecycle requests
		if msgTime, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
			age := time.Since(msgTime)
			if age > MaxLifecycleMessageAge {
				d.logger.Printf("Ignoring stale lifecycle request from %s (age: %v, max: %v) - deleting",
					request.From, age.Round(time.Minute), MaxLifecycleMessageAge)
				if err := d.closeMessage(msg.ID); err != nil {
					d.logger.Printf("Warning: failed to delete stale message %s: %v", msg.ID, err)
				}
				continue
			}
		}

		d.logger.Printf("Processing lifecycle request from %s: %s", request.From, request.Action)

		// CRITICAL: Delete message FIRST, before executing action.
		// This prevents stale messages from being reprocessed on every heartbeat.
		// "Claim then execute" pattern: claim by deleting, then execute.
		// Even if action fails, the message is gone - sender must re-request.
		if err := d.closeMessage(msg.ID); err != nil {
			d.logger.Printf("Warning: failed to delete message %s before execution: %v", msg.ID, err)
			// Continue anyway - better to attempt action than leave stale message
		}

		if err := d.executeLifecycleAction(request); err != nil {
			d.logger.Printf("Error executing lifecycle action: %v", err)
			continue
		}
	}
}

// LifecycleBody is the structured body format for lifecycle requests.
// Claude should send mail with JSON body: {"action": "cycle"} or {"action": "shutdown"}
type LifecycleBody struct {
	Action string `json:"action"`
}

// parseLifecycleRequest extracts a lifecycle request from a message.
// Uses structured body parsing instead of keyword matching on subject.
func (d *Daemon) parseLifecycleRequest(msg *BeadsMessage) *LifecycleRequest {
	// Gate: subject must start with "LIFECYCLE:"
	subject := strings.ToLower(msg.Subject)
	if !strings.HasPrefix(subject, "lifecycle:") {
		return nil
	}

	// Parse structured body for action
	var body LifecycleBody
	if err := json.Unmarshal([]byte(msg.Body), &body); err != nil {
		// Fallback: check for simple action strings in body
		bodyLower := strings.ToLower(strings.TrimSpace(msg.Body))
		switch {
		case bodyLower == "restart" || bodyLower == "action: restart":
			body.Action = "restart"
		case bodyLower == "shutdown" || bodyLower == "action: shutdown" || bodyLower == "stop":
			body.Action = "shutdown"
		case bodyLower == "cycle" || bodyLower == "action: cycle":
			body.Action = "cycle"
		default:
			d.logger.Printf("Lifecycle request with unparseable body: %q", msg.Body)
			return nil
		}
	}

	// Map action string to enum
	var action LifecycleAction
	switch strings.ToLower(body.Action) {
	case "restart":
		action = ActionRestart
	case "shutdown", "stop":
		action = ActionShutdown
	case "cycle":
		action = ActionCycle
	default:
		d.logger.Printf("Unknown lifecycle action: %q", body.Action)
		return nil
	}

	return &LifecycleRequest{
		From:      msg.From,
		Action:    action,
		Timestamp: time.Now(),
	}
}

// executeLifecycleAction performs the requested lifecycle action.
func (d *Daemon) executeLifecycleAction(request *LifecycleRequest) error {
	// Convert identity to AgentID for logging
	agentID, err := identityToAgentID(request.From)
	if err != nil {
		return fmt.Errorf("unknown agent identity: %s", request.From)
	}

	d.logger.Printf("Executing %s for agent %s", request.Action, agentID)

	// Check agent bead state (ZFC: trust what agent reports) - gt-39ttg
	agentBeadID := d.identityToAgentBeadID(request.From)
	if agentBeadID != "" {
		if beadState, err := d.getAgentBeadState(agentBeadID); err == nil {
			d.logger.Printf("Agent bead %s reports state: %s", agentBeadID, beadState)
		}
	}

	// Check if agent exists using factory
	running := d.isAgentRunning(request.From)

	switch request.Action {
	case ActionShutdown:
		if running {
			if err := d.stopAgent(request.From); err != nil {
				return fmt.Errorf("stopping agent: %w", err)
			}
			d.logger.Printf("Stopped agent %s", agentID)
		}
		return nil

	case ActionCycle, ActionRestart:
		if running {
			// Stop the agent first
			if err := d.stopAgent(request.From); err != nil {
				return fmt.Errorf("stopping agent: %w", err)
			}
			d.logger.Printf("Stopped agent %s for restart", agentID)

			// Wait a moment
			time.Sleep(constants.ShutdownNotifyDelay)
		}

		// Restart using Respawn if the agent was running, otherwise use factory
		if err := d.restartAgent(request.From); err != nil {
			return fmt.Errorf("restarting agent: %w", err)
		}
		d.logger.Printf("Restarted agent %s", agentID)
		return nil

	default:
		return fmt.Errorf("unknown action: %s", request.Action)
	}
}

// ParsedIdentity holds the components extracted from an agent identity string.
// This is used to look up the appropriate role bead for lifecycle config.
type ParsedIdentity struct {
	RoleType  string // mayor, deacon, witness, refinery, crew, polecat
	RigName   string // Empty for town-level agents (mayor, deacon)
	AgentName string // Empty for singletons (mayor, deacon, witness, refinery)
}

// parseIdentity extracts role type, rig name, and agent name from an identity string.
// This is the ONLY place where identity string patterns are parsed.
// All other functions should use the extracted components to look up role beads.
func parseIdentity(identity string) (*ParsedIdentity, error) {
	switch identity {
	case "mayor":
		return &ParsedIdentity{RoleType: "mayor"}, nil
	case "deacon":
		return &ParsedIdentity{RoleType: "deacon"}, nil
	}

	// Pattern: <rig>-witness → witness role
	if strings.HasSuffix(identity, "-witness") {
		rigName := strings.TrimSuffix(identity, "-witness")
		return &ParsedIdentity{RoleType: "witness", RigName: rigName}, nil
	}

	// Pattern: <rig>-refinery → refinery role
	if strings.HasSuffix(identity, "-refinery") {
		rigName := strings.TrimSuffix(identity, "-refinery")
		return &ParsedIdentity{RoleType: "refinery", RigName: rigName}, nil
	}

	// Pattern: <rig>-crew-<name> → crew role
	if strings.Contains(identity, "-crew-") {
		parts := strings.SplitN(identity, "-crew-", 2)
		if len(parts) == 2 {
			return &ParsedIdentity{RoleType: "crew", RigName: parts[0], AgentName: parts[1]}, nil
		}
	}

	// Pattern: <rig>-polecat-<name> → polecat role
	if strings.Contains(identity, "-polecat-") {
		parts := strings.SplitN(identity, "-polecat-", 2)
		if len(parts) == 2 {
			return &ParsedIdentity{RoleType: "polecat", RigName: parts[0], AgentName: parts[1]}, nil
		}
	}

	// Pattern: <rig>/polecats/<name> → polecat role (slash format)
	if strings.Contains(identity, "/polecats/") {
		parts := strings.Split(identity, "/polecats/")
		if len(parts) == 2 {
			return &ParsedIdentity{RoleType: "polecat", RigName: parts[0], AgentName: parts[1]}, nil
		}
	}

	return nil, fmt.Errorf("unknown identity format: %s", identity)
}

// getRoleConfigForIdentity looks up the role bead for an identity and returns its config.
// Falls back to default config if role bead doesn't exist or has no config.
func (d *Daemon) getRoleConfigForIdentity(identity string) (*beads.RoleConfig, *ParsedIdentity, error) {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return nil, nil, err
	}

	// Look up role bead
	b := beads.New(d.config.TownRoot)

	roleBeadID := beads.RoleBeadIDTown(parsed.RoleType)
	roleConfig, err := b.GetRoleConfig(roleBeadID)
	if err != nil {
		d.logger.Printf("Warning: failed to get role config for %s: %v", roleBeadID, err)
	}

	// Backward compatibility: fall back to legacy role bead IDs.
	if roleConfig == nil {
		legacyRoleBeadID := beads.RoleBeadID(parsed.RoleType) // gt-<role>-role
		if legacyRoleBeadID != roleBeadID {
			legacyCfg, legacyErr := b.GetRoleConfig(legacyRoleBeadID)
			if legacyErr != nil {
				d.logger.Printf("Warning: failed to get legacy role config for %s: %v", legacyRoleBeadID, legacyErr)
			} else if legacyCfg != nil {
				roleConfig = legacyCfg
			}
		}
	}

	// Return parsed identity even if config is nil (caller can use defaults)
	return roleConfig, parsed, nil
}

// identityToAgentID converts a beads identity to an AgentID.
func identityToAgentID(identity string) (agentpkg.AgentID, error) {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return agentpkg.AgentID{}, err
	}

	switch parsed.RoleType {
	case "mayor":
		return agentpkg.MayorAddress, nil
	case "deacon":
		return agentpkg.DeaconAddress, nil
	case "witness":
		return agentpkg.WitnessAddress(parsed.RigName), nil
	case "refinery":
		return agentpkg.RefineryAddress(parsed.RigName), nil
	case "crew":
		return agentpkg.CrewAddress(parsed.RigName, parsed.AgentName), nil
	case "polecat":
		return agentpkg.PolecatAddress(parsed.RigName, parsed.AgentName), nil
	default:
		return agentpkg.AgentID{}, fmt.Errorf("unknown role type: %s", parsed.RoleType)
	}
}

// restartAgent starts an agent using the factory functions.
// This ensures consistent startup behavior (env vars, theming, GUPP, etc.).
func (d *Daemon) restartAgent(identity string) error {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return fmt.Errorf("parsing identity: %w", err)
	}

	// Check rig operational state for rig-level agents
	if parsed.RigName != "" {
		if operational, reason := d.isRigOperational(parsed.RigName); !operational {
			d.logger.Printf("Skipping agent restart for %s: %s", identity, reason)
			return fmt.Errorf("cannot restart agent: %s", reason)
		}
	}

	switch parsed.RoleType {
	case "mayor":
		_, err := factory.Start(d.config.TownRoot, agentpkg.MayorAddress)
		return err

	case "deacon":
		_, err := factory.Start(d.config.TownRoot, agentpkg.DeaconAddress)
		return err

	case "witness":
		witnessID := agentpkg.WitnessAddress(parsed.RigName)
		_, err := factory.Start(d.config.TownRoot, witnessID)
		return err

	case "refinery":
		refineryID := agentpkg.RefineryAddress(parsed.RigName)
		_, err := factory.Start(d.config.TownRoot, refineryID)
		return err

	case "crew":
		crewID := agentpkg.CrewAddress(parsed.RigName, parsed.AgentName)
		_, err := factory.Start(d.config.TownRoot, crewID)
		return err

	case "polecat":
		polecatID := agentpkg.PolecatAddress(parsed.RigName, parsed.AgentName)
		_, err := factory.Start(d.config.TownRoot, polecatID)
		return err

	default:
		return fmt.Errorf("unknown role type: %s", parsed.RoleType)
	}
}

// isAgentRunning checks if an agent is running using factory.Agents().Exists().
func (d *Daemon) isAgentRunning(identity string) bool {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return false
	}

	agents := factory.Agents()

	switch parsed.RoleType {
	case "mayor":
		return agents.Exists(agentpkg.MayorAddress)

	case "deacon":
		return agents.Exists(agentpkg.DeaconAddress)

	case "witness":
		return agents.Exists(agentpkg.WitnessAddress(parsed.RigName))

	case "refinery":
		return agents.Exists(agentpkg.RefineryAddress(parsed.RigName))

	case "crew":
		return agents.Exists(agentpkg.CrewAddress(parsed.RigName, parsed.AgentName))

	case "polecat":
		return agents.Exists(agentpkg.PolecatAddress(parsed.RigName, parsed.AgentName))

	default:
		return false
	}
}

// stopAgent stops an agent using factory.Agents().Stop().
func (d *Daemon) stopAgent(identity string) error {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return err
	}

	agents := factory.Agents()

	switch parsed.RoleType {
	case "mayor":
		return agents.Stop(agentpkg.MayorAddress, true)

	case "deacon":
		return agents.Stop(agentpkg.DeaconAddress, true)

	case "witness":
		witnessID := agentpkg.WitnessAddress(parsed.RigName)
		return agents.Stop(witnessID, true)

	case "refinery":
		refineryID := agentpkg.RefineryAddress(parsed.RigName)
		return agents.Stop(refineryID, true)

	case "crew":
		crewID := agentpkg.CrewAddress(parsed.RigName, parsed.AgentName)
		return agents.Stop(crewID, true)

	case "polecat":
		polecatID := agentpkg.PolecatAddress(parsed.RigName, parsed.AgentName)
		return agents.Stop(polecatID, true)

	default:
		return fmt.Errorf("unknown role type: %s", parsed.RoleType)
	}
}







// syncWorkspace syncs a git workspace before starting a new session.
// This ensures agents with persistent clones (like refinery) start with current code.
func (d *Daemon) syncWorkspace(workDir string) {
	// Determine default branch from rig config
	// workDir is like <townRoot>/<rigName>/<role>/rig or <townRoot>/<rigName>/crew/<name>
	defaultBranch := "main" // fallback
	rel, err := filepath.Rel(d.config.TownRoot, workDir)
	if err == nil {
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 0 {
			rigPath := filepath.Join(d.config.TownRoot, parts[0])
			if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
				defaultBranch = rigCfg.DefaultBranch
			}
		}
	}

	// Fetch latest from origin
	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Dir = workDir
	if err := fetchCmd.Run(); err != nil {
		d.logger.Printf("Warning: git fetch failed in %s: %v", workDir, err)
	}

	// Pull with rebase to incorporate changes
	pullCmd := exec.Command("git", "pull", "--rebase", "origin", defaultBranch)
	pullCmd.Dir = workDir
	if err := pullCmd.Run(); err != nil {
		d.logger.Printf("Warning: git pull failed in %s: %v", workDir, err)
		// Don't fail - agent can handle conflicts
	}

	// Sync beads
	bdCmd := exec.Command("bd", "sync")
	bdCmd.Dir = workDir
	if err := bdCmd.Run(); err != nil {
		d.logger.Printf("Warning: bd sync failed in %s: %v", workDir, err)
	}
}

// closeMessage removes a lifecycle mail message after processing.
// We use delete instead of read because gt mail read intentionally
// doesn't mark messages as read (to preserve handoff messages).
func (d *Daemon) closeMessage(id string) error {
	// Use gt mail delete to actually remove the message
	cmd := exec.Command("gt", "mail", "delete", id)
	cmd.Dir = d.config.TownRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt mail delete %s: %v (output: %s)", id, err, string(output))
	}
	d.logger.Printf("Deleted lifecycle message: %s", id)
	return nil
}

// AgentBeadInfo represents the parsed fields from an agent bead.
type AgentBeadInfo struct {
	ID         string `json:"id"`
	Type       string `json:"issue_type"`
	State      string // Parsed from description: agent_state
	HookBead   string // Parsed from description: hook_bead
	RoleBead   string // Parsed from description: role_bead
	RoleType   string // Parsed from description: role_type
	Rig        string // Parsed from description: rig
	LastUpdate string `json:"updated_at"`
}

// getAgentBeadState reads non-observable agent state from an agent bead.
// Per gt-zecmc: Observable states (running, dead, idle) are derived from tmux.
// Only non-observable states (stuck, awaiting-gate, muted, paused) are stored in beads.
// Returns the agent_state field value or empty string if not found.
func (d *Daemon) getAgentBeadState(agentBeadID string) (string, error) {
	info, err := d.getAgentBeadInfo(agentBeadID)
	if err != nil {
		return "", err
	}
	return info.State, nil
}

// getAgentBeadInfo fetches and parses an agent bead by ID.
func (d *Daemon) getAgentBeadInfo(agentBeadID string) (*AgentBeadInfo, error) {
	cmd := exec.Command("bd", "show", agentBeadID, "--json")
	cmd.Dir = d.config.TownRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", agentBeadID, err)
	}

	// bd show --json returns an array with one element
	var issues []struct {
		ID          string `json:"id"`
		Type        string `json:"issue_type"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updated_at"`
		HookBead    string `json:"hook_bead"`   // Read from database column
		AgentState  string `json:"agent_state"` // Read from database column
	}

	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("agent bead not found: %s", agentBeadID)
	}

	issue := issues[0]
	if issue.Type != "agent" {
		return nil, fmt.Errorf("bead %s is not an agent bead (type=%s)", agentBeadID, issue.Type)
	}

	// Parse agent fields from description for role/state info
	fields := beads.ParseAgentFieldsFromDescription(issue.Description)

	info := &AgentBeadInfo{
		ID:         issue.ID,
		Type:       issue.Type,
		LastUpdate: issue.UpdatedAt,
	}

	if fields != nil {
		info.State = fields.AgentState
		info.RoleBead = fields.RoleBead
		info.RoleType = fields.RoleType
		info.Rig = fields.Rig
	}

	// Use HookBead from database column directly (not from description)
	// The description may contain stale data - the slot is the source of truth.
	info.HookBead = issue.HookBead

	return info, nil
}

// identityToAgentBeadID maps a daemon identity to an agent bead ID.
// Uses parseIdentity to extract components, then uses beads package helpers.
func (d *Daemon) identityToAgentBeadID(identity string) string {
	parsed, err := parseIdentity(identity)
	if err != nil {
		return ""
	}

	switch parsed.RoleType {
	case "deacon":
		return beads.DeaconBeadIDTown()
	case "mayor":
		return beads.MayorBeadIDTown()
	case "witness":
		prefix := config.GetRigPrefix(d.config.TownRoot, parsed.RigName)
		return beads.WitnessBeadIDWithPrefix(prefix, parsed.RigName)
	case "refinery":
		prefix := config.GetRigPrefix(d.config.TownRoot, parsed.RigName)
		return beads.RefineryBeadIDWithPrefix(prefix, parsed.RigName)
	case "crew":
		prefix := config.GetRigPrefix(d.config.TownRoot, parsed.RigName)
		return beads.CrewBeadIDWithPrefix(prefix, parsed.RigName, parsed.AgentName)
	case "polecat":
		prefix := config.GetRigPrefix(d.config.TownRoot, parsed.RigName)
		return beads.PolecatBeadIDWithPrefix(prefix, parsed.RigName, parsed.AgentName)
	default:
		return ""
	}
}

// NOTE: checkStaleAgents() and markAgentDead() were removed in gt-zecmc.
// Agent liveness is now discovered from tmux, not recorded in beads.
// "Discover, don't track" principle: observable state should not be recorded.

// identityToBDActor converts a daemon identity to BD_ACTOR format (with slashes).
// Uses parseIdentity to extract components, then builds the slash format.
func identityToBDActor(identity string) string {
	// Handle already-slash-formatted identities
	if strings.Contains(identity, "/polecats/") || strings.Contains(identity, "/crew/") ||
		strings.Contains(identity, "/witness") || strings.Contains(identity, "/refinery") {
		return identity
	}

	parsed, err := parseIdentity(identity)
	if err != nil {
		return identity // Unknown format - return as-is
	}

	switch parsed.RoleType {
	case "mayor", "deacon":
		return parsed.RoleType
	case "witness":
		return parsed.RigName + "/witness"
	case "refinery":
		return parsed.RigName + "/refinery"
	case "crew":
		return parsed.RigName + "/crew/" + parsed.AgentName
	case "polecat":
		return parsed.RigName + "/polecats/" + parsed.AgentName
	default:
		return identity
	}
}

// GUPPViolationTimeout is how long an agent can have work on hook without
// progressing before it's considered a GUPP (Gas Town Universal Propulsion
// Principle) violation. GUPP states: if you have work on your hook, you run it.
const GUPPViolationTimeout = 30 * time.Minute

// checkGUPPViolations looks for agents that have work-on-hook but aren't
// progressing. This is a GUPP violation: agents with hooked work must execute.
// The daemon detects these and notifies the relevant Witness for remediation.
func (d *Daemon) checkGUPPViolations() {
	// Check polecat agents - they're the ones with work-on-hook
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		d.checkRigGUPPViolations(rigName)
	}
}

// checkRigGUPPViolations checks polecats in a specific rig for GUPP violations.
func (d *Daemon) checkRigGUPPViolations(rigName string) {
	// List polecat agent beads for this rig
	// Pattern: <prefix>-<rig>-polecat-<name> (e.g., gt-gastown-polecat-Toast)
	cmd := exec.Command("bd", "list", "--type=agent", "--json")
	cmd.Dir = d.config.TownRoot

	output, err := cmd.Output()
	if err != nil {
		return // Silently fail - bd might not be available
	}

	var agents []struct {
		ID          string `json:"id"`
		Type        string `json:"issue_type"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updated_at"`
		HookBead    string `json:"hook_bead"` // Read from database column, not description
		AgentState  string `json:"agent_state"`
	}

	if err := json.Unmarshal(output, &agents); err != nil {
		return
	}

	// Use the rig's configured prefix (e.g., "gt" for gastown, "bd" for beads)
	rigPrefix := config.GetRigPrefix(d.config.TownRoot, rigName)
	// Pattern: <prefix>-<rig>-polecat-<name>
	prefix := rigPrefix + "-" + rigName + "-polecat-"
	for _, agent := range agents {
		// Only check polecats for this rig
		if !strings.HasPrefix(agent.ID, prefix) {
			continue
		}

		// Check if agent has work on hook
		// Use HookBead from database column directly (not parsed from description)
		if agent.HookBead == "" {
			continue // No hooked work - no GUPP violation possible
		}

		// Per gt-zecmc: derive running state from tmux, not agent_state
		// Extract polecat name from agent ID (<prefix>-<rig>-polecat-<name> -> <name>)
		polecatName := strings.TrimPrefix(agent.ID, prefix)

		// Check if agent is running using factory.Agents().Exists()
		polecatID := agentpkg.PolecatAddress(rigName, polecatName)
		running := factory.Agents().Exists(polecatID)

		if running {
			// Session is alive - check if it's been stuck too long
			updatedAt, err := time.Parse(time.RFC3339, agent.UpdatedAt)
			if err != nil {
				continue
			}

			age := time.Since(updatedAt)
			if age > GUPPViolationTimeout {
				d.logger.Printf("GUPP violation: agent %s has hook_bead=%s but hasn't updated in %v (timeout: %v)",
					agent.ID, agent.HookBead, age.Round(time.Minute), GUPPViolationTimeout)

				// Notify the witness for this rig
				d.notifyWitnessOfGUPP(rigName, agent.ID, agent.HookBead, age)
			}
		}
	}
}

// notifyWitnessOfGUPP sends a mail to the rig's witness about a GUPP violation.
func (d *Daemon) notifyWitnessOfGUPP(rigName, agentID, hookBead string, stuckDuration time.Duration) {
	witnessAddr := rigName + "/witness"
	subject := fmt.Sprintf("GUPP_VIOLATION: %s stuck for %v", agentID, stuckDuration.Round(time.Minute))
	body := fmt.Sprintf(`Agent %s has work on hook but isn't progressing.

hook_bead: %s
stuck_duration: %v

Action needed: Check if agent is alive and responsive. Consider restarting if stuck.`,
		agentID, hookBead, stuckDuration.Round(time.Minute))

	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body)
	cmd.Dir = d.config.TownRoot

	if err := cmd.Run(); err != nil {
		d.logger.Printf("Warning: failed to notify witness of GUPP violation: %v", err)
	} else {
		d.logger.Printf("Notified %s of GUPP violation for %s", witnessAddr, agentID)
	}
}

// checkOrphanedWork looks for work assigned to dead agents.
// Orphaned work needs to be reassigned or the agent needs to be restarted.
// Per gt-zecmc: derive agent liveness from tmux, not agent_state.
func (d *Daemon) checkOrphanedWork() {
	// Check all polecat agents with hooked work
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		d.checkRigOrphanedWork(rigName)
	}
}

// checkRigOrphanedWork checks polecats in a specific rig for orphaned work.
func (d *Daemon) checkRigOrphanedWork(rigName string) {
	cmd := exec.Command("bd", "list", "--type=agent", "--json")
	cmd.Dir = d.config.TownRoot

	output, err := cmd.Output()
	if err != nil {
		return
	}

	var agents []struct {
		ID       string `json:"id"`
		HookBead string `json:"hook_bead"`
	}

	if err := json.Unmarshal(output, &agents); err != nil {
		return
	}

	// Use the rig's configured prefix (e.g., "gt" for gastown, "bd" for beads)
	rigPrefix := config.GetRigPrefix(d.config.TownRoot, rigName)
	// Pattern: <prefix>-<rig>-polecat-<name>
	prefix := rigPrefix + "-" + rigName + "-polecat-"
	for _, agent := range agents {
		// Only check polecats for this rig
		if !strings.HasPrefix(agent.ID, prefix) {
			continue
		}

		// No hooked work = nothing to orphan
		if agent.HookBead == "" {
			continue
		}

		// Check if agent is alive (derive state from factory.Agents().Exists(), not bead)
		polecatName := strings.TrimPrefix(agent.ID, prefix)

		// Check if agent is running using factory.Agents().Exists()
		polecatID := agentpkg.PolecatAddress(rigName, polecatName)
		running := factory.Agents().Exists(polecatID)

		// Agent running = not orphaned (work is being processed)
		if running {
			continue
		}

		// Session dead but has hooked work = orphaned!
		d.logger.Printf("Orphaned work detected: agent %s session is dead but has hook_bead=%s",
			agent.ID, agent.HookBead)

		d.notifyWitnessOfOrphanedWork(rigName, agent.ID, agent.HookBead)
	}
}

// extractRigFromAgentID extracts the rig name from a polecat agent ID.
// Example: gt-gastown-polecat-max → gastown
func (d *Daemon) extractRigFromAgentID(agentID string) string {
	// Use the beads package helper to correctly parse agent bead IDs.
	// Pattern: <prefix>-<rig>-polecat-<name> (e.g., gt-gastown-polecat-Toast)
	rig, role, _, ok := beads.ParseAgentBeadID(agentID)
	if !ok || role != "polecat" {
		return ""
	}
	return rig
}

// notifyWitnessOfOrphanedWork sends a mail to the rig's witness about orphaned work.
func (d *Daemon) notifyWitnessOfOrphanedWork(rigName, agentID, hookBead string) {
	witnessAddr := rigName + "/witness"
	subject := fmt.Sprintf("ORPHANED_WORK: %s has hooked work but is dead", agentID)
	body := fmt.Sprintf(`Agent %s is dead but has work on its hook.

hook_bead: %s

Action needed: Either restart the agent or reassign the work.`,
		agentID, hookBead)

	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body)
	cmd.Dir = d.config.TownRoot

	if err := cmd.Run(); err != nil {
		d.logger.Printf("Warning: failed to notify witness of orphaned work: %v", err)
	} else {
		d.logger.Printf("Notified %s of orphaned work for %s", witnessAddr, agentID)
	}
}
