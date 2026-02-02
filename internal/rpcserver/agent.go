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

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// AgentServer implements the AgentService.
type AgentServer struct {
	townRoot string
	tmux     *tmux.Tmux
}

var _ gastownv1connect.AgentServiceHandler = (*AgentServer)(nil)

// NewAgentServer creates a new AgentServer.
func NewAgentServer(townRoot string) *AgentServer {
	return &AgentServer{
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
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
	exists, _ := s.tmux.HasSession(session)
	if exists {
		agent.State = gastownv1.AgentState_AGENT_STATE_RUNNING
	} else {
		agent.State = gastownv1.AgentState_AGENT_STATE_STOPPED
	}

	// Get recent output
	var recentOutput []string
	if exists {
		output, err := s.tmux.CapturePane(session, 20)
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
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("spawn failed: %w\n%s", err, string(output)))
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

	// Build gt crew start command
	args := []string{"crew", "start", req.Msg.Name, "--rig", req.Msg.Rig}
	if req.Msg.Account != "" {
		args = append(args, "--account", req.Msg.Account)
	}
	if req.Msg.AgentOverride != "" {
		args = append(args, "--agent", req.Msg.AgentOverride)
	}
	if req.Msg.Create {
		args = append(args, "--create")
	}

	cmd := exec.CommandContext(ctx, "gt", args...)
	cmd.Dir = s.townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("crew start failed: %w\n%s", err, string(output)))
	}

	session := fmt.Sprintf("gt-%s-crew-%s", req.Msg.Rig, req.Msg.Name)
	agent := &gastownv1.Agent{
		Address:   fmt.Sprintf("%s/crew/%s", req.Msg.Rig, req.Msg.Name),
		Name:      req.Msg.Name,
		Rig:       req.Msg.Rig,
		Type:      gastownv1.AgentType_AGENT_TYPE_CREW,
		State:     gastownv1.AgentState_AGENT_STATE_RUNNING,
		Session:   session,
		StartedAt: timestamppb.Now(),
	}

	created := strings.Contains(string(output), "Created")

	return connect.NewResponse(&gastownv1.StartCrewResponse{
		Agent:   agent,
		Session: session,
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
	var cmd *exec.Cmd

	switch {
	case len(parts) >= 3 && parts[1] == "crew":
		args := []string{"crew", "stop", parts[2], "--rig", parts[0]}
		if req.Msg.Force {
			args = append(args, "--force")
		}
		cmd = exec.CommandContext(ctx, "gt", args...)
	case len(parts) >= 3 && parts[1] == "polecats":
		// For polecats, we send shutdown to witness
		args := []string{"polecat", "kill", parts[2], "--rig", parts[0]}
		if req.Msg.Force {
			args = append(args, "--force")
		}
		cmd = exec.CommandContext(ctx, "gt", args...)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot stop agent: %s", address))
	}

	cmd.Dir = s.townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("stop failed: %w\n%s", err, string(output)))
	}

	agent := &gastownv1.Agent{
		Address: address,
		State:   gastownv1.AgentState_AGENT_STATE_STOPPED,
	}

	return connect.NewResponse(&gastownv1.StopAgentResponse{
		Agent:             agent,
		HadIncompleteWork: strings.Contains(string(output), "incomplete"),
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

	// Build gt nudge command
	args := []string{"nudge", req.Msg.Agent, req.Msg.Message}

	cmd := exec.CommandContext(ctx, "gt", args...)
	cmd.Dir = s.townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("nudge failed: %w\n%s", err, string(output)))
	}

	// Extract session from output or construct it
	session := ""
	parts := strings.Split(req.Msg.Agent, "/")
	if len(parts) >= 3 && parts[1] == "crew" {
		session = fmt.Sprintf("gt-%s-crew-%s", parts[0], parts[2])
	} else if len(parts) >= 3 && parts[1] == "polecats" {
		session = fmt.Sprintf("gt-%s-%s", parts[0], parts[2])
	}

	return connect.NewResponse(&gastownv1.NudgeAgentResponse{
		Delivered: true,
		Session:   session,
	}), nil
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

	exists, _ := s.tmux.HasSession(session)
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
		output, err = s.tmux.CapturePaneAll(session)
	} else {
		output, err = s.tmux.CapturePane(session, lines)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("capturing pane: %w", err))
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
				Rig:           req.Msg.Rig,
				Type:          req.Msg.Type,
				IncludeGlobal: req.Msg.IncludeGlobal,
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
