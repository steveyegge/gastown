package rpcserver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"connectrpc.com/connect"

	gastownv1 "github.com/steveyegge/gastown/gen/gastown/v1"
	"github.com/steveyegge/gastown/gen/gastown/v1/gastownv1connect"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/terminal"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// AgentServer implements the AgentService.
type AgentServer struct {
	townRoot string
	tmux     *tmux.Tmux
	backend  terminal.Backend
}

var _ gastownv1connect.AgentServiceHandler = (*AgentServer)(nil)

// NewAgentServer creates a new AgentServer.
func NewAgentServer(townRoot string) *AgentServer {
	t := tmux.NewTmux()
	return &AgentServer{
		townRoot: townRoot,
		tmux:     t,
		backend:  terminal.NewCoopBackend(terminal.CoopConfig{}),
	}
}

// NewAgentServerWithBackend creates an AgentServer with a custom terminal backend.
// Use this when the daemon runs in K8s with a coop backend.
func NewAgentServerWithBackend(townRoot string, backend terminal.Backend) *AgentServer {
	return &AgentServer{
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
		backend:  backend,
	}
}

func (s *AgentServer) ListAgents(
	ctx context.Context,
	req *connect.Request[gastownv1.ListAgentsRequest],
) (*connect.Response[gastownv1.ListAgentsResponse], error) {
	var agents []*gastownv1.Agent
	runningCount := 0

	// Get all tmux sessions for quick lookup
	sessions, _ := s.tmux.ListSessions()
	sessionSet := make(map[string]bool)
	for _, sess := range sessions {
		sessionSet[sess] = true
	}

	// Load rig config
	rigsConfigPath := fmt.Sprintf("%s/mayor/rigs.json", s.townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(s.townRoot)
	rigMgr := rig.NewManager(s.townRoot, rigsConfig, g)

	// Get rigs to scan
	var rigsToScan []*rig.Rig
	if req.Msg.Rig != "" {
		r, err := rigMgr.GetRig(req.Msg.Rig)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rig not found: %s", req.Msg.Rig))
		}
		rigsToScan = []*rig.Rig{r}
	} else {
		rigsToScan, _ = rigMgr.DiscoverRigs()
	}

	// Add global agents if requested
	if req.Msg.IncludeGlobal {
		for _, ga := range []struct {
			name    string
			session string
			atype   gastownv1.AgentType
		}{
			{"mayor", "gt-mayor", gastownv1.AgentType_AGENT_TYPE_MAYOR},
			{"deacon", "gt-deacon", gastownv1.AgentType_AGENT_TYPE_DEACON},
		} {
			running := sessionSet[ga.session]
			state := gastownv1.AgentState_AGENT_STATE_STOPPED
			if running {
				state = gastownv1.AgentState_AGENT_STATE_RUNNING
				runningCount++
			}
			if running || req.Msg.IncludeStopped {
				agents = append(agents, &gastownv1.Agent{
					Address: ga.name,
					Name:    ga.name,
					Type:    ga.atype,
					State:   state,
					Session: ga.session,
				})
			}
		}
	}

	// Scan each rig for crew and polecats
	for _, r := range rigsToScan {
		// Skip if filtering by type and this isn't a match
		if req.Msg.Type != gastownv1.AgentType_AGENT_TYPE_UNSPECIFIED {
			// We'll filter below
		}

		// Crew workers
		if req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_UNSPECIFIED ||
			req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_CREW {
			crewGit := git.NewGit(r.Path)
			crewMgr := crew.NewManager(r, crewGit)
			workers, _ := crewMgr.List()
			for _, w := range workers {
				session := fmt.Sprintf("gt-%s-crew-%s", r.Name, w.Name)
				running := sessionSet[session]
				state := gastownv1.AgentState_AGENT_STATE_STOPPED
				if running {
					state = gastownv1.AgentState_AGENT_STATE_RUNNING
					runningCount++
				}
				if running || req.Msg.IncludeStopped {
					agents = append(agents, &gastownv1.Agent{
						Address: fmt.Sprintf("%s/crew/%s", r.Name, w.Name),
						Name:    w.Name,
						Rig:     r.Name,
						Type:    gastownv1.AgentType_AGENT_TYPE_CREW,
						State:   state,
						Session: session,
						WorkDir: w.ClonePath,
						Branch:  w.Branch,
					})
				}
			}
		}

		// Polecats
		if req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_UNSPECIFIED ||
			req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_POLECAT {
			for _, p := range r.Polecats {
				session := fmt.Sprintf("gt-%s-%s", r.Name, p)
				running := sessionSet[session]
				state := gastownv1.AgentState_AGENT_STATE_STOPPED
				if running {
					state = gastownv1.AgentState_AGENT_STATE_WORKING
					runningCount++
				}
				if running || req.Msg.IncludeStopped {
					agents = append(agents, &gastownv1.Agent{
						Address: fmt.Sprintf("%s/polecats/%s", r.Name, p),
						Name:    p,
						Rig:     r.Name,
						Type:    gastownv1.AgentType_AGENT_TYPE_POLECAT,
						State:   state,
						Session: session,
					})
				}
			}
		}

		// Witness
		if (req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_UNSPECIFIED ||
			req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_WITNESS) && r.HasWitness {
			session := fmt.Sprintf("gt-%s-witness", r.Name)
			running := sessionSet[session]
			state := gastownv1.AgentState_AGENT_STATE_STOPPED
			if running {
				state = gastownv1.AgentState_AGENT_STATE_RUNNING
				runningCount++
			}
			if running || req.Msg.IncludeStopped {
				agents = append(agents, &gastownv1.Agent{
					Address: fmt.Sprintf("%s/witness", r.Name),
					Name:    "witness",
					Rig:     r.Name,
					Type:    gastownv1.AgentType_AGENT_TYPE_WITNESS,
					State:   state,
					Session: session,
				})
			}
		}

		// Refinery
		if (req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_UNSPECIFIED ||
			req.Msg.Type == gastownv1.AgentType_AGENT_TYPE_REFINERY) && r.HasRefinery {
			session := fmt.Sprintf("gt-%s-refinery", r.Name)
			running := sessionSet[session]
			state := gastownv1.AgentState_AGENT_STATE_STOPPED
			if running {
				state = gastownv1.AgentState_AGENT_STATE_RUNNING
				runningCount++
			}
			if running || req.Msg.IncludeStopped {
				agents = append(agents, &gastownv1.Agent{
					Address: fmt.Sprintf("%s/refinery", r.Name),
					Name:    "refinery",
					Rig:     r.Name,
					Type:    gastownv1.AgentType_AGENT_TYPE_REFINERY,
					State:   state,
					Session: session,
				})
			}
		}
	}

	return connect.NewResponse(&gastownv1.ListAgentsResponse{
		Agents:  agents,
		Total:   int32(len(agents)),
		Running: int32(runningCount),
	}), nil
}

func (s *AgentServer) GetAgent(
	ctx context.Context,
	req *connect.Request[gastownv1.GetAgentRequest],
) (*connect.Response[gastownv1.GetAgentResponse], error) {
	address := req.Msg.Agent
	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent address is required"))
	}

	// Parse address to determine type
	parts := strings.Split(address, "/")
	var agent *gastownv1.Agent
	var session string

	switch {
	case address == "mayor":
		session = "gt-mayor"
		agent = &gastownv1.Agent{
			Address: "mayor",
			Name:    "mayor",
			Type:    gastownv1.AgentType_AGENT_TYPE_MAYOR,
			Session: session,
		}
	case address == "deacon":
		session = "gt-deacon"
		agent = &gastownv1.Agent{
			Address: "deacon",
			Name:    "deacon",
			Type:    gastownv1.AgentType_AGENT_TYPE_DEACON,
			Session: session,
		}
	case len(parts) >= 2 && parts[1] == "witness":
		session = fmt.Sprintf("gt-%s-witness", parts[0])
		agent = &gastownv1.Agent{
			Address: address,
			Name:    "witness",
			Rig:     parts[0],
			Type:    gastownv1.AgentType_AGENT_TYPE_WITNESS,
			Session: session,
		}
	case len(parts) >= 2 && parts[1] == "refinery":
		session = fmt.Sprintf("gt-%s-refinery", parts[0])
		agent = &gastownv1.Agent{
			Address: address,
			Name:    "refinery",
			Rig:     parts[0],
			Type:    gastownv1.AgentType_AGENT_TYPE_REFINERY,
			Session: session,
		}
	case len(parts) >= 3 && parts[1] == "crew":
		session = fmt.Sprintf("gt-%s-crew-%s", parts[0], parts[2])
		agent = &gastownv1.Agent{
			Address: address,
			Name:    parts[2],
			Rig:     parts[0],
			Type:    gastownv1.AgentType_AGENT_TYPE_CREW,
			Session: session,
		}
	case len(parts) >= 3 && parts[1] == "polecats":
		session = fmt.Sprintf("gt-%s-%s", parts[0], parts[2])
		agent = &gastownv1.Agent{
			Address: address,
			Name:    parts[2],
			Rig:     parts[0],
			Type:    gastownv1.AgentType_AGENT_TYPE_POLECAT,
			Session: session,
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent address format: %s", address))
	}

	// Check if session exists
	exists, _ := s.backend.HasSession(session)
	if exists {
		agent.State = gastownv1.AgentState_AGENT_STATE_RUNNING
	} else {
		agent.State = gastownv1.AgentState_AGENT_STATE_STOPPED
	}

	// Get recent output
	var recentOutput []string
	if exists {
		output, err := s.backend.CapturePane(session, 20)
		if err == nil && output != "" {
			recentOutput = strings.Split(output, "\n")
		}
	}

	return connect.NewResponse(&gastownv1.GetAgentResponse{
		Agent:        agent,
		RecentOutput: recentOutput,
	}), nil
}

func (s *AgentServer) SpawnPolecat(
	ctx context.Context,
	req *connect.Request[gastownv1.SpawnPolecatRequest],
) (*connect.Response[gastownv1.SpawnPolecatResponse], error) {
	if req.Msg.Rig == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rig is required"))
	}

	// Build gt polecat spawn command
	args := []string{"polecat", "spawn", req.Msg.Rig}
	if req.Msg.Name != "" {
		args = append(args, "--name", req.Msg.Name)
	}
	if req.Msg.Account != "" {
		args = append(args, "--account", req.Msg.Account)
	}
	if req.Msg.AgentOverride != "" {
		args = append(args, "--agent", req.Msg.AgentOverride)
	}
	if req.Msg.HookBead != "" {
		args = append(args, "--hook", req.Msg.HookBead)
	}

	cmd := exec.CommandContext(ctx, "gt", args...)
	cmd.Dir = s.townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, cmdExecErr("spawn polecat", err, output)
	}

	// Parse output to extract polecat name
	outputStr := string(output)
	polecatName := req.Msg.Name
	if polecatName == "" {
		// Extract from output
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Allocated polecat:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					polecatName = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	session := fmt.Sprintf("gt-%s-%s", req.Msg.Rig, polecatName)
	agent := &gastownv1.Agent{
		Address:   fmt.Sprintf("%s/polecats/%s", req.Msg.Rig, polecatName),
		Name:      polecatName,
		Rig:       req.Msg.Rig,
		Type:      gastownv1.AgentType_AGENT_TYPE_POLECAT,
		State:     gastownv1.AgentState_AGENT_STATE_WORKING,
		Session:   session,
		StartedAt: timestamppb.Now(),
	}

	if req.Msg.HookBead != "" {
		agent.HookedBead = req.Msg.HookBead
	}

	return connect.NewResponse(&gastownv1.SpawnPolecatResponse{
		Agent:   agent,
		Session: session,
	}), nil
}

func (s *AgentServer) StartCrew(
	ctx context.Context,
	req *connect.Request[gastownv1.StartCrewRequest],
) (*connect.Response[gastownv1.StartCrewResponse], error) {
	if req.Msg.Rig == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rig is required"))
	}
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("crew name is required"))
	}

	// Resolve town name for bead ID generation.
	townName, err := workspace.GetTownName(s.townRoot)
	if err != nil {
		townName = "" // Fall back to no-town format
	}

	// Generate the agent bead ID using town-level format (hq- prefix).
	crewID := beads.CrewBeadIDTown(townName, req.Msg.Rig, req.Msg.Name)

	// Create the beads client against town-level beads.
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	bd := beads.New(townBeadsPath)

	// Build agent fields. Setting agent_state=spawning marks this as starting.
	fields := &beads.AgentFields{
		RoleType:   "crew",
		Rig:        req.Msg.Rig,
		AgentState: "spawning",
	}

	title := fmt.Sprintf("Crew worker %s in %s", req.Msg.Name, req.Msg.Rig)

	// CreateOrReopen handles re-starting a crew with the same name.
	issue, err := bd.CreateOrReopenAgentBead(crewID, title, fields)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("creating agent bead: %w", err))
	}

	// Determine if this was a reopen (crew existed before).
	created := issue.Status == "" || issue.Status == "open"

	// Add execution_target:k8s label so the controller reconciles this into a pod.
	if err := bd.AddLabel(crewID, "execution_target:k8s"); err != nil {
		// Non-fatal: controller may still discover via gt:agent label
		fmt.Printf("Warning: could not add execution_target label: %v\n", err)
	}

	// Set the bead status to in_progress to trigger the controller.
	inProgress := "in_progress"
	if err := bd.Update(crewID, beads.UpdateOptions{Status: &inProgress}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("setting bead status to in_progress: %w", err))
	}

	agent := &gastownv1.Agent{
		Address:   fmt.Sprintf("%s/crew/%s", req.Msg.Rig, req.Msg.Name),
		Name:      req.Msg.Name,
		Rig:       req.Msg.Rig,
		Type:      gastownv1.AgentType_AGENT_TYPE_CREW,
		State:     gastownv1.AgentState_AGENT_STATE_RUNNING,
		StartedAt: timestamppb.Now(),
	}

	return connect.NewResponse(&gastownv1.StartCrewResponse{
		Agent:   agent,
		Created: created,
	}), nil
}

func (s *AgentServer) StopAgent(
	ctx context.Context,
	req *connect.Request[gastownv1.StopAgentRequest],
) (*connect.Response[gastownv1.StopAgentResponse], error) {
	address := req.Msg.Agent
	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent address is required"))
	}

	// Parse address to get agent details
	parts := strings.Split(address, "/")

	// Resolve town name and beads client.
	townName, err := workspace.GetTownName(s.townRoot)
	if err != nil {
		townName = ""
	}
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	bd := beads.New(townBeadsPath)

	var beadID string
	switch {
	case len(parts) >= 3 && parts[1] == "crew":
		beadID = beads.CrewBeadIDTown(townName, parts[0], parts[2])
	case len(parts) >= 3 && parts[1] == "polecats":
		prefix := beads.GetPrefixForRig(s.townRoot, parts[0])
		beadID = beads.PolecatBeadIDWithPrefix(prefix, parts[0], parts[2])
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot stop agent: %s", address))
	}

	// Check if bead has incomplete work before closing.
	hadIncompleteWork := false
	if issue, fields, err := bd.GetAgentBead(beadID); err == nil && issue != nil && fields != nil {
		hadIncompleteWork = fields.HookBead != ""
	}

	// Close the bead. The controller reacts to the close event by deleting the pod.
	reason := req.Msg.Reason
	if reason == "" {
		reason = "stopped via RPC"
		if req.Msg.Force {
			reason = "force stopped via RPC"
		}
	}
	if err := bd.CloseAndClearAgentBead(beadID, reason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("closing agent bead %s: %w", beadID, err))
	}

	agent := &gastownv1.Agent{
		Address: address,
		State:   gastownv1.AgentState_AGENT_STATE_STOPPED,
	}

	return connect.NewResponse(&gastownv1.StopAgentResponse{
		Agent:             agent,
		HadIncompleteWork: hadIncompleteWork,
	}), nil
}

func (s *AgentServer) NudgeAgent(
	ctx context.Context,
	req *connect.Request[gastownv1.NudgeAgentRequest],
) (*connect.Response[gastownv1.NudgeAgentResponse], error) {
	if req.Msg.Agent == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent address is required"))
	}
	if req.Msg.Message == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("message is required"))
	}

	// Resolve agent address to bead ID.
	address := req.Msg.Agent
	parts := strings.Split(address, "/")

	townName, _ := workspace.GetTownName(s.townRoot)
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	bd := beads.New(townBeadsPath)

	var beadID string
	switch {
	case address == "mayor":
		beadID = beads.MayorBeadIDTown()
	case address == "deacon":
		beadID = beads.DeaconBeadIDTown()
	case len(parts) >= 2 && parts[1] == "witness":
		beadID = beads.WitnessBeadIDTown(townName, parts[0])
	case len(parts) >= 2 && parts[1] == "refinery":
		beadID = beads.RefineryBeadIDTown(townName, parts[0])
	case len(parts) >= 3 && parts[1] == "crew":
		beadID = beads.CrewBeadIDTown(townName, parts[0], parts[2])
	case len(parts) >= 3 && parts[1] == "polecats":
		prefix := beads.GetPrefixForRig(s.townRoot, parts[0])
		beadID = beads.PolecatBeadIDWithPrefix(prefix, parts[0], parts[2])
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent address: %s", address))
	}

	// Look up bead to get coop_url from notes.
	issue, err := bd.Show(beadID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent bead %s not found: %w", beadID, err))
	}

	coopURL := coopURLFromNotes(issue.Notes)
	if coopURL == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("agent %s has no coop_url in bead notes (not a K8s agent?)", beadID))
	}

	// Send nudge via coop API.
	backend := terminal.NewCoopBackend(terminal.CoopConfig{Timeout: 10 * time.Second})
	backend.AddSession("claude", coopURL)

	if err := backend.NudgeSession("claude", req.Msg.Message); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("nudge failed for %s: %w", beadID, err))
	}

	return connect.NewResponse(&gastownv1.NudgeAgentResponse{
		Delivered: true,
		Session:   beadID,
	}), nil
}

// coopURLFromNotes extracts a coop_url from agent bead notes.
// Notes contain key: value pairs, one per line.
func coopURLFromNotes(notes string) string {
	if notes == "" {
		return ""
	}
	for _, line := range strings.Split(notes, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == "coop_url" {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func (s *AgentServer) PeekAgent(
	ctx context.Context,
	req *connect.Request[gastownv1.PeekAgentRequest],
) (*connect.Response[gastownv1.PeekAgentResponse], error) {
	if req.Msg.Agent == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent address is required"))
	}

	// Parse address to get session name
	parts := strings.Split(req.Msg.Agent, "/")
	var session string

	switch {
	case req.Msg.Agent == "mayor":
		session = "gt-mayor"
	case req.Msg.Agent == "deacon":
		session = "gt-deacon"
	case len(parts) >= 2 && parts[1] == "witness":
		session = fmt.Sprintf("gt-%s-witness", parts[0])
	case len(parts) >= 2 && parts[1] == "refinery":
		session = fmt.Sprintf("gt-%s-refinery", parts[0])
	case len(parts) >= 3 && parts[1] == "crew":
		session = fmt.Sprintf("gt-%s-crew-%s", parts[0], parts[2])
	case len(parts) >= 3 && parts[1] == "polecats":
		session = fmt.Sprintf("gt-%s-%s", parts[0], parts[2])
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent address: %s", req.Msg.Agent))
	}

	exists, _ := s.backend.HasSession(session)
	if !exists {
		return connect.NewResponse(&gastownv1.PeekAgentResponse{
			Exists: false,
		}), nil
	}

	lines := int(req.Msg.Lines)
	if lines <= 0 {
		lines = 50
	}
	if lines > 1000 {
		lines = 1000
	}

	var output string
	var err error
	if req.Msg.All {
		output, err = s.backend.CapturePaneAll(session)
	} else {
		output, err = s.backend.CapturePane(session, lines)
	}
	if err != nil {
		return nil, unavailableErr("capturing terminal output", err, 2)
	}

	var lineSlice []string
	if output != "" {
		lineSlice = strings.Split(output, "\n")
	}

	return connect.NewResponse(&gastownv1.PeekAgentResponse{
		Output: output,
		Lines:  lineSlice,
		Exists: true,
	}), nil
}

func (s *AgentServer) WatchAgents(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchAgentsRequest],
	stream *connect.ServerStream[gastownv1.AgentUpdate],
) error {
	intervalMs := int(req.Msg.IntervalMs)
	if intervalMs < 1000 {
		intervalMs = 5000
	}

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	// Track previous state to detect changes
	prevStates := make(map[string]gastownv1.AgentState)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Get current agents
			listReq := &gastownv1.ListAgentsRequest{
				Rig:            req.Msg.Rig,
				Type:           req.Msg.Type,
				IncludeGlobal:  req.Msg.IncludeGlobal,
				IncludeStopped: true,
			}
			resp, err := s.ListAgents(ctx, connect.NewRequest(listReq))
			if err != nil {
				continue
			}

			// Detect changes
			currentAgents := make(map[string]bool)
			for _, agent := range resp.Msg.Agents {
				currentAgents[agent.Address] = true
				prevState, existed := prevStates[agent.Address]

				updateType := ""
				if !existed {
					updateType = "spawned"
				} else if prevState != agent.State {
					updateType = "state_changed"
				}

				if updateType != "" {
					if err := stream.Send(&gastownv1.AgentUpdate{
						Timestamp:  timestamppb.Now(),
						UpdateType: updateType,
						Agent:      agent,
					}); err != nil {
						return err
					}
				}

				prevStates[agent.Address] = agent.State
			}

			// Detect stopped/removed agents
			for addr, prevState := range prevStates {
				if !currentAgents[addr] && prevState != gastownv1.AgentState_AGENT_STATE_STOPPED {
					if err := stream.Send(&gastownv1.AgentUpdate{
						Timestamp:  timestamppb.Now(),
						UpdateType: "stopped",
						Agent: &gastownv1.Agent{
							Address: addr,
							State:   gastownv1.AgentState_AGENT_STATE_STOPPED,
						},
					}); err != nil {
						return err
					}
					prevStates[addr] = gastownv1.AgentState_AGENT_STATE_STOPPED
				}
			}
		}
	}
}

// CreateCrew creates a crew workspace by writing an agent bead.
// The controller watches bead events and creates the crew pod.
//
// Flow: gt crew add UI -> CreateCrew RPC -> daemon creates agent bead
//       -> controller watches bead event -> controller creates crew pod
func (s *AgentServer) CreateCrew(
	ctx context.Context,
	req *connect.Request[gastownv1.CreateCrewRequest],
) (*connect.Response[gastownv1.CreateCrewResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("crew name is required"))
	}
	if req.Msg.Rig == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rig is required"))
	}

	// Resolve town name for bead ID generation.
	townName, err := workspace.GetTownName(s.townRoot)
	if err != nil {
		townName = "" // Fall back to no-town format
	}

	// Generate the agent bead ID using town-level format (hq- prefix).
	// This matches gt crew add behavior: beads.CrewBeadIDTown(townName, rigName, name)
	crewID := beads.CrewBeadIDTown(townName, req.Msg.Rig, req.Msg.Name)

	// Create the beads client against town-level beads.
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	bd := beads.New(townBeadsPath)

	// Build agent fields. Setting agent_state=spawning marks this as a new crew.
	fields := &beads.AgentFields{
		RoleType:   "crew",
		Rig:        req.Msg.Rig,
		AgentState: "spawning",
	}

	title := fmt.Sprintf("Crew worker %s in %s", req.Msg.Name, req.Msg.Rig)

	// CreateOrReopen handles re-spawning a crew with the same name.
	issue, err := bd.CreateOrReopenAgentBead(crewID, title, fields)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("creating agent bead: %w", err))
	}

	// Determine if this was a reopen (issue existed before).
	reopened := issue.Status != "" && issue.Status != "open"

	// Set the bead status to in_progress to trigger the controller.
	// The controller's bead watcher maps status->in_progress to AgentSpawn.
	inProgress := "in_progress"
	if err := bd.Update(crewID, beads.UpdateOptions{Status: &inProgress}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("setting bead status to in_progress: %w", err))
	}

	agent := &gastownv1.Agent{
		Address:   fmt.Sprintf("%s/crew/%s", req.Msg.Rig, req.Msg.Name),
		Name:      req.Msg.Name,
		Rig:       req.Msg.Rig,
		Type:      gastownv1.AgentType_AGENT_TYPE_CREW,
		State:     gastownv1.AgentState_AGENT_STATE_RUNNING,
		StartedAt: timestamppb.Now(),
	}

	return connect.NewResponse(&gastownv1.CreateCrewResponse{
		BeadId:   crewID,
		Agent:    agent,
		Reopened: reopened,
	}), nil
}

// RemoveCrew removes a crew workspace by closing or deleting the agent bead.
// The controller reacts to the bead close/delete event to remove the pod.
//
// With purge=true, the bead is hard-deleted and the controller also deletes the PVC.
// Without purge, the bead is closed (soft delete), preserving history.
func (s *AgentServer) RemoveCrew(
	ctx context.Context,
	req *connect.Request[gastownv1.RemoveCrewRequest],
) (*connect.Response[gastownv1.RemoveCrewResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("crew name is required"))
	}
	if req.Msg.Rig == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rig is required"))
	}

	// Resolve town name and bead ID.
	townName, err := workspace.GetTownName(s.townRoot)
	if err != nil {
		townName = ""
	}
	crewID := beads.CrewBeadIDTown(townName, req.Msg.Rig, req.Msg.Name)

	// Create the beads client.
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	bd := beads.New(townBeadsPath)

	// Verify the bead exists.
	_, _, err = bd.GetAgentBead(crewID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent bead not found: %s", crewID))
	}

	reason := req.Msg.Reason
	if reason == "" {
		reason = "crew removed via RPC"
	}

	deleted := false
	if req.Msg.Purge {
		// Hard delete: signals controller to also remove PVC.
		// Add a purge label before deleting so the controller knows to clean up storage.
		_ = bd.AddLabel(crewID, "gt:purge")
		if err := bd.DeleteAgentBead(crewID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("deleting agent bead: %w", err))
		}
		deleted = true
	} else {
		// Soft delete: close the bead. Controller removes the pod but preserves PVC.
		if err := bd.CloseAndClearAgentBead(crewID, reason); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("closing agent bead: %w", err))
		}
	}

	return connect.NewResponse(&gastownv1.RemoveCrewResponse{
		BeadId:  crewID,
		Deleted: deleted,
	}), nil
}
