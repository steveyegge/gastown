package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// ErrUnknownList indicates a mailing list name was not found in configuration.
var ErrUnknownList = errors.New("unknown mailing list")

// ErrUnknownQueue indicates a queue name was not found in configuration.
var ErrUnknownQueue = errors.New("unknown queue")

// ErrUnknownAnnounce indicates an announce channel name was not found in configuration.
var ErrUnknownAnnounce = errors.New("unknown announce channel")

// Router handles message delivery via beads.
// It routes messages to the correct beads database based on address:
// - Town-level (mayor/, deacon/) -> {townRoot}/.beads
// - Rig-level (rig/polecat) -> {townRoot}/{rig}/.beads
type Router struct {
	workDir  string // fallback directory to run bd commands in
	townRoot string // town root directory (e.g., ~/gt)
	tmux     *tmux.Tmux
}

// NewRouter creates a new mail router.
// workDir should be a directory containing a .beads database.
// The town root is auto-detected from workDir if possible.
func NewRouter(workDir string) *Router {
	// Try to detect town root from workDir
	townRoot := detectTownRoot(workDir)

	return &Router{
		workDir:  workDir,
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
	}
}

// NewRouterWithTownRoot creates a router with an explicit town root.
func NewRouterWithTownRoot(workDir, townRoot string) *Router {
	return &Router{
		workDir:  workDir,
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
	}
}

// isListAddress returns true if the address uses list:name syntax.
func isListAddress(address string) bool {
	return strings.HasPrefix(address, "list:")
}

// parseListName extracts the list name from a list:name address.
func parseListName(address string) string {
	return strings.TrimPrefix(address, "list:")
}

// isQueueAddress returns true if the address uses queue:name syntax.
func isQueueAddress(address string) bool {
	return strings.HasPrefix(address, "queue:")
}

// parseQueueName extracts the queue name from a queue:name address.
func parseQueueName(address string) string {
	return strings.TrimPrefix(address, "queue:")
}

// isAnnounceAddress returns true if the address uses announce:name syntax.
func isAnnounceAddress(address string) bool {
	return strings.HasPrefix(address, "announce:")
}

// parseAnnounceName extracts the announce channel name from an announce:name address.
func parseAnnounceName(address string) string {
	return strings.TrimPrefix(address, "announce:")
}

// isChannelAddress returns true if the address uses channel:name syntax (beads-native channels).
func isChannelAddress(address string) bool {
	return strings.HasPrefix(address, "channel:")
}

// parseChannelName extracts the channel name from a channel:name address.
func parseChannelName(address string) string {
	return strings.TrimPrefix(address, "channel:")
}

// expandFromConfig is a generic helper for config-based expansion.
// It loads the messaging config and calls the getter to extract the desired value.
// This consolidates the common pattern of: check townRoot, load config, lookup in map.
func expandFromConfig[T any](r *Router, name string, getter func(*config.MessagingConfig) (T, bool), errType error) (T, error) {
	var zero T
	if r.townRoot == "" {
		return zero, fmt.Errorf("%w: %s (no town root)", errType, name)
	}

	configPath := config.MessagingConfigPath(r.townRoot)
	cfg, err := config.LoadMessagingConfig(configPath)
	if err != nil {
		return zero, fmt.Errorf("loading messaging config: %w", err)
	}

	result, ok := getter(cfg)
	if !ok {
		return zero, fmt.Errorf("%w: %s", errType, name)
	}

	return result, nil
}

// expandList returns the recipients for a mailing list.
// Returns ErrUnknownList if the list is not found.
func (r *Router) expandList(listName string) ([]string, error) {
	recipients, err := expandFromConfig(r, listName, func(cfg *config.MessagingConfig) ([]string, bool) {
		r, ok := cfg.Lists[listName]
		return r, ok
	}, ErrUnknownList)
	if err != nil {
		return nil, err
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("%w: %s (empty list)", ErrUnknownList, listName)
	}

	return recipients, nil
}

// expandQueue returns the QueueConfig for a queue name.
// Returns ErrUnknownQueue if the queue is not found.
func (r *Router) expandQueue(queueName string) (*config.QueueConfig, error) {
	return expandFromConfig(r, queueName, func(cfg *config.MessagingConfig) (*config.QueueConfig, bool) {
		qc, ok := cfg.Queues[queueName]
		if !ok {
			return nil, false
		}
		return &qc, true
	}, ErrUnknownQueue)
}

// expandAnnounce returns the AnnounceConfig for an announce channel name.
// Returns ErrUnknownAnnounce if the channel is not found.
func (r *Router) expandAnnounce(announceName string) (*config.AnnounceConfig, error) {
	return expandFromConfig(r, announceName, func(cfg *config.MessagingConfig) (*config.AnnounceConfig, bool) {
		ac, ok := cfg.Announces[announceName]
		if !ok {
			return nil, false
		}
		return &ac, true
	}, ErrUnknownAnnounce)
}

// detectTownRoot finds the town root using workspace.Find.
// This ensures consistent detection with the rest of the codebase,
// supporting both primary (mayor/town.json) and secondary (mayor/) markers.
func detectTownRoot(startDir string) string {
	townRoot, err := workspace.Find(startDir)
	if err != nil {
		return ""
	}
	return townRoot
}

// resolveBeadsDir returns the correct .beads directory for the given address.
//
// Two-level beads architecture:
// - ALL mail uses town beads ({townRoot}/.beads) regardless of address
// - Rig-level beads ({rig}/.beads) are for project issues only, not mail
//
// This ensures messages are visible to all agents in the town.
func (r *Router) resolveBeadsDir(_ string) string { // address unused: all mail uses town-level beads
	// If no town root, fall back to workDir's .beads
	if r.townRoot == "" {
		return filepath.Join(r.workDir, ".beads")
	}

	// All mail uses town-level beads
	return filepath.Join(r.townRoot, ".beads")
}

// isTownLevelAddress returns true if the address is for a town-level agent or the overseer.
func isTownLevelAddress(address string) bool {
	addr := strings.TrimSuffix(address, "/")
	return addr == "mayor" || addr == "deacon" || addr == "overseer"
}

// isGroupAddress returns true if the address is a @group address.
// Group addresses start with @ and resolve to multiple recipients.
func isGroupAddress(address string) bool {
	return strings.HasPrefix(address, "@")
}

// GroupType represents the type of group address.
type GroupType string

const (
	GroupTypeRig      GroupType = "rig"      // @rig/<rigname> - all agents in a rig
	GroupTypeTown     GroupType = "town"     // @town - all town-level agents
	GroupTypeRole     GroupType = "role"     // @witnesses, @dogs, etc. - all agents of a role
	GroupTypeRigRole  GroupType = "rig-role" // @crew/<rigname>, @polecats/<rigname> - role in a rig
	GroupTypeOverseer GroupType = "overseer" // @overseer - human operator
)

// ParsedGroup represents a parsed @group address.
type ParsedGroup struct {
	Type      GroupType
	RoleType  string // witness, crew, polecat, dog, etc.
	Rig       string // rig name for rig-scoped groups
	Original  string // original @group string
}

// parseGroupAddress parses a @group address into its components.
// Returns nil if the address is not a valid group address.
//
// Supported patterns:
//   - @rig/<rigname>: All agents in a rig
//   - @town: All town-level agents (mayor, deacon)
//   - @witnesses: All witnesses across rigs
//   - @crew/<rigname>: Crew workers in a specific rig
//   - @polecats/<rigname>: Polecats in a specific rig
//   - @dogs: All Deacon dogs
//   - @overseer: Human operator (special case)
func parseGroupAddress(address string) *ParsedGroup {
	if !isGroupAddress(address) {
		return nil
	}

	// Remove @ prefix
	group := strings.TrimPrefix(address, "@")

	// Special cases that don't require parsing
	switch group {
	case "overseer":
		return &ParsedGroup{Type: GroupTypeOverseer, Original: address}
	case "town":
		return &ParsedGroup{Type: GroupTypeTown, Original: address}
	case "witnesses":
		return &ParsedGroup{Type: GroupTypeRole, RoleType: "witness", Original: address}
	case "dogs":
		return &ParsedGroup{Type: GroupTypeRole, RoleType: "dog", Original: address}
	case "refineries":
		return &ParsedGroup{Type: GroupTypeRole, RoleType: "refinery", Original: address}
	case "deacons":
		return &ParsedGroup{Type: GroupTypeRole, RoleType: "deacon", Original: address}
	}

	// Parse patterns with slashes: @rig/<name>, @crew/<rig>, @polecats/<rig>
	parts := strings.SplitN(group, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil // Invalid format
	}

	prefix, qualifier := parts[0], parts[1]

	switch prefix {
	case "rig":
		return &ParsedGroup{Type: GroupTypeRig, Rig: qualifier, Original: address}
	case "crew":
		return &ParsedGroup{Type: GroupTypeRigRole, RoleType: "crew", Rig: qualifier, Original: address}
	case "polecats":
		return &ParsedGroup{Type: GroupTypeRigRole, RoleType: "polecat", Rig: qualifier, Original: address}
	default:
		return nil // Unknown group type
	}
}

// agentBead represents an agent bead as returned by bd list --type=agent.
type agentBead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// agentBeadToAddress converts an agent bead to a mail address.
// Uses the agent bead ID to derive the address:
//   - gt-mayor → mayor/
//   - gt-deacon → deacon/
//   - gt-gastown-witness → gastown/witness
//   - gt-gastown-crew-max → gastown/max
//   - gt-gastown-polecat-Toast → gastown/Toast
func agentBeadToAddress(bead *agentBead) string {
	if bead == nil {
		return ""
	}

	id := bead.ID
	if !strings.HasPrefix(id, "gt-") {
		return "" // Not a valid agent bead ID
	}

	// Strip prefix
	rest := strings.TrimPrefix(id, "gt-")
	parts := strings.Split(rest, "-")

	switch len(parts) {
	case 1:
		// Town-level: gt-mayor, gt-deacon
		return parts[0] + "/"
	case 2:
		// Rig singleton: gt-gastown-witness
		return parts[0] + "/" + parts[1]
	default:
		// Rig named agent: gt-gastown-crew-max, gt-gastown-polecat-Toast
		// Skip the role part (parts[1]) and use rig/name format
		if len(parts) >= 3 {
			// Rejoin if name has hyphens: gt-gastown-polecat-my-agent
			name := strings.Join(parts[2:], "-")
			return parts[0] + "/" + name
		}
		return ""
	}
}

// ResolveGroupAddress resolves a @group address to individual recipient addresses.
// Returns the list of resolved addresses and any error.
// This is the public entry point for group resolution.
func (r *Router) ResolveGroupAddress(address string) ([]string, error) {
	group := parseGroupAddress(address)
	if group == nil {
		return nil, fmt.Errorf("invalid group address: %s", address)
	}
	return r.resolveGroup(group)
}

// resolveGroup resolves a @group address to individual recipient addresses.
// Returns the list of resolved addresses and any error.
func (r *Router) resolveGroup(group *ParsedGroup) ([]string, error) {
	if group == nil {
		return nil, errors.New("nil group")
	}

	switch group.Type {
	case GroupTypeOverseer:
		return r.resolveOverseer()
	case GroupTypeTown:
		return r.resolveTownAgents()
	case GroupTypeRole:
		return r.resolveAgentsByRole(group.RoleType, "")
	case GroupTypeRig:
		return r.resolveAgentsByRig(group.Rig)
	case GroupTypeRigRole:
		return r.resolveAgentsByRole(group.RoleType, group.Rig)
	default:
		return nil, fmt.Errorf("unknown group type: %s", group.Type)
	}
}

// resolveOverseer resolves @overseer to the human operator's address.
// Loads the overseer config and returns "overseer" as the address.
func (r *Router) resolveOverseer() ([]string, error) {
	if r.townRoot == "" {
		return nil, errors.New("town root not set, cannot resolve @overseer")
	}

	// Load overseer config to verify it exists
	configPath := config.OverseerConfigPath(r.townRoot)
	_, err := config.LoadOverseerConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolving @overseer: %w", err)
	}

	// Return the overseer address
	return []string{"overseer"}, nil
}

// resolveTownAgents resolves @town to all town-level agents (mayor, deacon).
func (r *Router) resolveTownAgents() ([]string, error) {
	// Town-level agents have rig=null in their description
	agents, err := r.queryAgents("rig: null")
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, agent := range agents {
		if addr := agentBeadToAddress(agent); addr != "" {
			addresses = append(addresses, addr)
		}
	}

	return addresses, nil
}

// resolveAgentsByRole resolves agents by their role_type.
// If rig is non-empty, also filters by rig.
func (r *Router) resolveAgentsByRole(roleType, rig string) ([]string, error) {
	// Build query filter
	query := "role_type: " + roleType
	agents, err := r.queryAgents(query)
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, agent := range agents {
		// Filter by rig if specified
		if rig != "" {
			// Check if agent's description contains matching rig
			if !strings.Contains(agent.Description, "rig: "+rig) {
				continue
			}
		}
		if addr := agentBeadToAddress(agent); addr != "" {
			addresses = append(addresses, addr)
		}
	}

	return addresses, nil
}

// resolveAgentsByRig resolves @rig/<rigname> to all agents in that rig.
func (r *Router) resolveAgentsByRig(rig string) ([]string, error) {
	// Query for agents with matching rig in description
	query := "rig: " + rig
	agents, err := r.queryAgents(query)
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, agent := range agents {
		if addr := agentBeadToAddress(agent); addr != "" {
			addresses = append(addresses, addr)
		}
	}

	return addresses, nil
}

// queryAgents queries agent beads using bd list with description filtering.
func (r *Router) queryAgents(descContains string) ([]*agentBead, error) {
	beadsDir := r.resolveBeadsDir("")
	args := []string{"list", "--type=agent", "--json", "--limit=0"}

	if descContains != "" {
		args = append(args, "--desc-contains="+descContains)
	}

	stdout, err := runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}

	var agents []*agentBead
	if err := json.Unmarshal(stdout, &agents); err != nil {
		return nil, fmt.Errorf("parsing agent query result: %w", err)
	}

	// Filter for open agents only (closed agents are inactive)
	var active []*agentBead
	for _, agent := range agents {
		if agent.Status == "open" || agent.Status == "in_progress" {
			active = append(active, agent)
		}
	}

	return active, nil
}

// shouldBeWisp determines if a message should be stored as a wisp.
// Returns true if:
// - Message.Wisp is explicitly set
// - Subject matches lifecycle message patterns (POLECAT_*, NUDGE, etc.)
func (r *Router) shouldBeWisp(msg *Message) bool {
	if msg.Wisp {
		return true
	}
	// Auto-detect lifecycle messages by subject prefix
	subjectLower := strings.ToLower(msg.Subject)
	wispPrefixes := []string{
		"polecat_started",
		"polecat_done",
		"start_work",
		"nudge",
	}
	for _, prefix := range wispPrefixes {
		if strings.HasPrefix(subjectLower, prefix) {
			return true
		}
	}
	return false
}

// Send delivers a message via beads message.
// Routes the message to the correct beads database based on recipient address.
// Supports fan-out for:
// - Mailing lists (list:name) - fans out to all list members
// - @group addresses - resolves and fans out to matching agents
// Supports single-copy delivery for:
// - Queues (queue:name) - stores single message for worker claiming
// - Announces (announce:name) - bulletin board, no claiming, retention-limited
func (r *Router) Send(msg *Message) error {
	// Check for mailing list address
	if isListAddress(msg.To) {
		return r.sendToList(msg)
	}

	// Check for queue address - single message for claiming
	if isQueueAddress(msg.To) {
		return r.sendToQueue(msg)
	}

	// Check for announce address - bulletin board (single copy, no claiming)
	if isAnnounceAddress(msg.To) {
		return r.sendToAnnounce(msg)
	}

	// Check for beads-native channel address - broadcast with retention
	if isChannelAddress(msg.To) {
		return r.sendToChannel(msg)
	}

	// Check for @group address - resolve and fan-out
	if isGroupAddress(msg.To) {
		return r.sendToGroup(msg)
	}

	// Single recipient - send directly
	return r.sendToSingle(msg)
}

// sendToGroup resolves a @group address and sends individual messages to each member.
func (r *Router) sendToGroup(msg *Message) error {
	group := parseGroupAddress(msg.To)
	if group == nil {
		return fmt.Errorf("invalid group address: %s", msg.To)
	}

	recipients, err := r.resolveGroup(group)
	if err != nil {
		return fmt.Errorf("resolving group %s: %w", msg.To, err)
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no recipients found for group: %s", msg.To)
	}

	// Fan-out: send a copy to each recipient
	var errs []string
	for _, recipient := range recipients {
		// Create a copy of the message for this recipient
		msgCopy := *msg
		msgCopy.To = recipient

		if err := r.sendToSingle(&msgCopy); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", recipient, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some group sends failed: %s", strings.Join(errs, "; "))
	}

	return nil
}

// sendToSingle sends a message to a single recipient.
func (r *Router) sendToSingle(msg *Message) error {
	// Convert addresses to beads identities
	toIdentity := addressToIdentity(msg.To)

	// Build labels for from/thread/reply-to/cc
	var labels []string
	labels = append(labels, "from:"+msg.From)
	if msg.ThreadID != "" {
		labels = append(labels, "thread:"+msg.ThreadID)
	}
	if msg.ReplyTo != "" {
		labels = append(labels, "reply-to:"+msg.ReplyTo)
	}
	// Add CC labels (one per recipient)
	for _, cc := range msg.CC {
		ccIdentity := addressToIdentity(cc)
		labels = append(labels, "cc:"+ccIdentity)
	}

	// Build command: bd create <subject> --type=message --assignee=<recipient> -d <body>
	args := []string{"create", msg.Subject,
		"--type", "message",
		"--assignee", toIdentity,
		"-d", msg.Body,
	}

	// Add priority flag
	beadsPriority := PriorityToBeads(msg.Priority)
	args = append(args, "--priority", fmt.Sprintf("%d", beadsPriority))

	// Add labels
	if len(labels) > 0 {
		args = append(args, "--labels", strings.Join(labels, ","))
	}

	// Add actor for attribution (sender identity)
	args = append(args, "--actor", msg.From)

	// Add --ephemeral flag for ephemeral messages (stored in single DB, filtered from JSONL export)
	if r.shouldBeWisp(msg) {
		args = append(args, "--ephemeral")
	}

	beadsDir := r.resolveBeadsDir(msg.To)
	_, err := runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	// Notify recipient if they have an active session (best-effort notification)
	// Skip notification for self-mail (handoffs to future-self don't need present-self notified)
	if !isSelfMail(msg.From, msg.To) {
		_ = r.notifyRecipient(msg)
	}

	return nil
}

// sendToList expands a mailing list and sends individual copies to each recipient.
// Each recipient gets their own message copy with the same content.
// Returns a ListDeliveryResult with details about the fan-out.
func (r *Router) sendToList(msg *Message) error {
	listName := parseListName(msg.To)
	recipients, err := r.expandList(listName)
	if err != nil {
		return err
	}

	// Send to each recipient
	var lastErr error
	successCount := 0
	for _, recipient := range recipients {
		// Create a copy of the message for this recipient
		copy := *msg
		copy.To = recipient

		if err := r.Send(&copy); err != nil {
			lastErr = err
			continue
		}
		successCount++
	}

	// If all sends failed, return the last error
	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("sending to list %s: %w", listName, lastErr)
	}

	return nil
}

// ExpandListAddress expands a list:name address to its recipients.
// Returns ErrUnknownList if the list is not found.
// This is exported for use by commands that want to show fan-out details.
func (r *Router) ExpandListAddress(address string) ([]string, error) {
	if !isListAddress(address) {
		return nil, fmt.Errorf("not a list address: %s", address)
	}
	return r.expandList(parseListName(address))
}

// sendToQueue delivers a message to a queue for worker claiming.
// Unlike sendToList, this creates a SINGLE message (no fan-out).
// The message is stored in town-level beads with queue metadata.
// Workers claim messages using bd update --claimed-by.
func (r *Router) sendToQueue(msg *Message) error {
	queueName := parseQueueName(msg.To)

	// Validate queue exists in messaging config
	_, err := r.expandQueue(queueName)
	if err != nil {
		return err
	}

	// Build labels for from/thread/reply-to/cc plus queue metadata
	var labels []string
	labels = append(labels, "from:"+msg.From)
	labels = append(labels, "queue:"+queueName)
	if msg.ThreadID != "" {
		labels = append(labels, "thread:"+msg.ThreadID)
	}
	if msg.ReplyTo != "" {
		labels = append(labels, "reply-to:"+msg.ReplyTo)
	}
	for _, cc := range msg.CC {
		ccIdentity := addressToIdentity(cc)
		labels = append(labels, "cc:"+ccIdentity)
	}

	// Build command: bd create <subject> --type=message --assignee=queue:<name> -d <body>
	// Use queue:<name> as assignee so inbox queries can filter by queue
	args := []string{"create", msg.Subject,
		"--type", "message",
		"--assignee", msg.To, // queue:name
		"-d", msg.Body,
	}

	// Add priority flag
	beadsPriority := PriorityToBeads(msg.Priority)
	args = append(args, "--priority", fmt.Sprintf("%d", beadsPriority))

	// Add labels (includes queue name for filtering)
	if len(labels) > 0 {
		args = append(args, "--labels", strings.Join(labels, ","))
	}

	// Add actor for attribution (sender identity)
	args = append(args, "--actor", msg.From)

	// Queue messages are never ephemeral - they need to persist until claimed
	// (deliberately not checking shouldBeWisp)

	// Queue messages go to town-level beads (shared location)
	beadsDir := r.resolveBeadsDir("")
	_, err = runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return fmt.Errorf("sending to queue %s: %w", queueName, err)
	}

	// No notification for queue messages - workers poll or check on their own schedule

	return nil
}

// sendToAnnounce delivers a message to an announce channel (bulletin board).
// Unlike sendToQueue, no claiming is supported - messages persist until retention limit.
// ONE copy is stored in town-level beads with announce_channel metadata.
func (r *Router) sendToAnnounce(msg *Message) error {
	announceName := parseAnnounceName(msg.To)

	// Validate announce channel exists and get config
	announceCfg, err := r.expandAnnounce(announceName)
	if err != nil {
		return err
	}

	// Apply retention pruning BEFORE creating new message
	if announceCfg.RetainCount > 0 {
		if err := r.pruneAnnounce(announceName, announceCfg.RetainCount); err != nil {
			// Log but don't fail - pruning is best-effort
			// The new message should still be created
			_ = err
		}
	}

	// Build labels for from/thread/reply-to/cc plus announce metadata
	var labels []string
	labels = append(labels, "from:"+msg.From)
	labels = append(labels, "announce:"+announceName)
	if msg.ThreadID != "" {
		labels = append(labels, "thread:"+msg.ThreadID)
	}
	if msg.ReplyTo != "" {
		labels = append(labels, "reply-to:"+msg.ReplyTo)
	}
	for _, cc := range msg.CC {
		ccIdentity := addressToIdentity(cc)
		labels = append(labels, "cc:"+ccIdentity)
	}

	// Build command: bd create <subject> --type=message --assignee=announce:<name> -d <body>
	// Use announce:<name> as assignee so queries can filter by channel
	args := []string{"create", msg.Subject,
		"--type", "message",
		"--assignee", msg.To, // announce:name
		"-d", msg.Body,
	}

	// Add priority flag
	beadsPriority := PriorityToBeads(msg.Priority)
	args = append(args, "--priority", fmt.Sprintf("%d", beadsPriority))

	// Add labels (includes announce name for filtering)
	if len(labels) > 0 {
		args = append(args, "--labels", strings.Join(labels, ","))
	}

	// Add actor for attribution (sender identity)
	args = append(args, "--actor", msg.From)

	// Announce messages are never ephemeral - they need to persist for readers
	// (deliberately not checking shouldBeWisp)

	// Announce messages go to town-level beads (shared location)
	beadsDir := r.resolveBeadsDir("")
	_, err = runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return fmt.Errorf("sending to announce %s: %w", announceName, err)
	}

	// No notification for announce messages - readers poll or check on their own schedule

	return nil
}

// sendToChannel delivers a message to a beads-native channel.
// Creates a message with channel:<name> label for channel queries.
// Retention is enforced by the channel's EnforceChannelRetention after message creation.
func (r *Router) sendToChannel(msg *Message) error {
	channelName := parseChannelName(msg.To)

	// Validate channel exists as a beads-native channel
	if r.townRoot == "" {
		return fmt.Errorf("town root not set, cannot send to channel: %s", channelName)
	}
	b := beads.New(r.townRoot)
	_, fields, err := b.GetChannelBead(channelName)
	if err != nil {
		return fmt.Errorf("getting channel %s: %w", channelName, err)
	}
	if fields == nil {
		return fmt.Errorf("channel not found: %s", channelName)
	}
	if fields.Status == beads.ChannelStatusClosed {
		return fmt.Errorf("channel %s is closed", channelName)
	}

	// Build labels for from/thread/reply-to/cc plus channel metadata
	var labels []string
	labels = append(labels, "from:"+msg.From)
	labels = append(labels, "channel:"+channelName)
	if msg.ThreadID != "" {
		labels = append(labels, "thread:"+msg.ThreadID)
	}
	if msg.ReplyTo != "" {
		labels = append(labels, "reply-to:"+msg.ReplyTo)
	}
	for _, cc := range msg.CC {
		ccIdentity := addressToIdentity(cc)
		labels = append(labels, "cc:"+ccIdentity)
	}

	// Build command: bd create <subject> --type=message --assignee=channel:<name> -d <body>
	// Use channel:<name> as assignee so queries can filter by channel
	args := []string{"create", msg.Subject,
		"--type", "message",
		"--assignee", msg.To, // channel:name
		"-d", msg.Body,
	}

	// Add priority flag
	beadsPriority := PriorityToBeads(msg.Priority)
	args = append(args, "--priority", fmt.Sprintf("%d", beadsPriority))

	// Add labels (includes channel name for filtering)
	if len(labels) > 0 {
		args = append(args, "--labels", strings.Join(labels, ","))
	}

	// Add actor for attribution (sender identity)
	args = append(args, "--actor", msg.From)

	// Channel messages are never ephemeral - they persist according to retention policy
	// (deliberately not checking shouldBeWisp)

	// Channel messages go to town-level beads (shared location)
	beadsDir := r.resolveBeadsDir("")
	_, err = runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return fmt.Errorf("sending to channel %s: %w", channelName, err)
	}

	// Enforce channel retention policy (on-write cleanup)
	_ = b.EnforceChannelRetention(channelName)

	// No notification for channel messages - readers poll or check on their own schedule

	return nil
}

// pruneAnnounce deletes oldest messages from an announce channel to enforce retention.
// If the channel has >= retainCount messages, deletes the oldest until count < retainCount.
func (r *Router) pruneAnnounce(announceName string, retainCount int) error {
	if retainCount <= 0 {
		return nil // No retention limit
	}

	beadsDir := r.resolveBeadsDir("")

	// Query existing messages in this announce channel
	// Use bd list with labels filter to find messages with announce:<name> label
	args := []string{"list",
		"--type=message",
		"--labels=announce:" + announceName,
		"--json",
		"--limit=0", // Get all
		"--sort=created",
		"--asc", // Oldest first
	}

	stdout, err := runBdCommand(args, filepath.Dir(beadsDir), beadsDir)
	if err != nil {
		return fmt.Errorf("querying announce messages: %w", err)
	}

	// Parse message list
	var messages []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout, &messages); err != nil {
		return fmt.Errorf("parsing announce messages: %w", err)
	}

	// Calculate how many to delete (we're about to add 1 more)
	// If we have N messages and retainCount is R, we need to keep at most R-1 after pruning
	// so the new message makes it exactly R
	toDelete := len(messages) - (retainCount - 1)
	if toDelete <= 0 {
		return nil // No pruning needed
	}

	// Delete oldest messages
	for i := 0; i < toDelete && i < len(messages); i++ {
		deleteArgs := []string{"close", messages[i].ID, "--reason=retention pruning"}
		// Best-effort deletion - don't fail if one delete fails
		_, _ = runBdCommand(deleteArgs, filepath.Dir(beadsDir), beadsDir)
	}

	return nil
}

// isSelfMail returns true if sender and recipient are the same identity.
// Normalizes addresses by removing trailing slashes for comparison.
func isSelfMail(from, to string) bool {
	fromNorm := strings.TrimSuffix(from, "/")
	toNorm := strings.TrimSuffix(to, "/")
	return fromNorm == toNorm
}

// GetMailbox returns a Mailbox for the given address.
// Routes to the correct beads database based on the address.
func (r *Router) GetMailbox(address string) (*Mailbox, error) {
	beadsDir := r.resolveBeadsDir(address)
	workDir := filepath.Dir(beadsDir) // Parent of .beads
	return NewMailboxFromAddress(address, workDir), nil
}

// notifyRecipient sends a notification to a recipient's tmux session.
// Uses NudgeSession to add the notification to the agent's conversation history.
// Supports mayor/, rig/polecat, and rig/refinery addresses.
func (r *Router) notifyRecipient(msg *Message) error {
	sessionID := addressToSessionID(msg.To)
	if sessionID == "" {
		return nil // Unable to determine session ID
	}

	// Check if session exists
	hasSession, err := r.tmux.HasSession(sessionID)
	if err != nil || !hasSession {
		return nil // No active session, skip notification
	}

	// Send notification to the agent's conversation history
	notification := fmt.Sprintf("📬 You have new mail from %s. Subject: %s. Run 'gt mail inbox' to read.", msg.From, msg.Subject)
	return r.tmux.NudgeSession(sessionID, notification)
}

// addressToSessionID converts a mail address to a tmux session ID.
// Returns empty string if address format is not recognized.
func addressToSessionID(address string) string {
	// Mayor address: "mayor/" or "mayor"
	if strings.HasPrefix(address, "mayor") {
		return session.MayorSessionName()
	}

	// Deacon address: "deacon/" or "deacon"
	if strings.HasPrefix(address, "deacon") {
		return session.DeaconSessionName()
	}

	// Rig-based address: "rig/target"
	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return ""
	}

	rig := parts[0]
	target := parts[1]

	// Polecat: gt-rig-polecat
	// Refinery: gt-rig-refinery (if refinery has its own session)
	return fmt.Sprintf("gt-%s-%s", rig, target)
}
