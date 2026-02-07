package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"connectrpc.com/connect"

	gastownv1 "github.com/steveyegge/gastown/mobile/gen/gastown/v1"
	"github.com/steveyegge/gastown/mobile/gen/gastown/v1/gastownv1connect"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/eventbus"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/notify"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StatusServer implements the StatusService.
type StatusServer struct {
	townRoot string
}

var _ gastownv1connect.StatusServiceHandler = (*StatusServer)(nil)

func NewStatusServer(townRoot string) *StatusServer {
	return &StatusServer{townRoot: townRoot}
}

func (s *StatusServer) GetTownStatus(
	ctx context.Context,
	req *connect.Request[gastownv1.GetTownStatusRequest],
) (*connect.Response[gastownv1.GetTownStatusResponse], error) {
	status, err := s.collectTownStatus(req.Msg.Fast)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&gastownv1.GetTownStatusResponse{Status: status}), nil
}

func (s *StatusServer) GetRigStatus(
	ctx context.Context,
	req *connect.Request[gastownv1.GetRigStatusRequest],
) (*connect.Response[gastownv1.GetRigStatusResponse], error) {
	status, err := s.collectTownStatus(false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	for _, r := range status.Rigs {
		if r.Name == req.Msg.RigName {
			return connect.NewResponse(&gastownv1.GetRigStatusResponse{Status: r}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rig not found: %s", req.Msg.RigName))
}

func (s *StatusServer) GetAgentStatus(
	ctx context.Context,
	req *connect.Request[gastownv1.GetAgentStatusRequest],
) (*connect.Response[gastownv1.GetAgentStatusResponse], error) {
	if req.Msg.Address == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}

	status, err := s.collectTownStatus(false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	addr := req.Msg.Address

	// Check global agents first (mayor, deacon) - identified by Name only
	if addr.Rig == "" && addr.Name != "" {
		for _, agent := range status.GlobalAgents {
			if agent.Name == addr.Name {
				return connect.NewResponse(&gastownv1.GetAgentStatusResponse{Agent: agent}), nil
			}
		}
	}

	// Check rig agents
	for _, rig := range status.Rigs {
		// Match by rig name if specified
		if addr.Rig != "" && rig.Name != addr.Rig {
			continue
		}

		for _, agent := range rig.Agents {
			if matchesAgentAddress(agent.Address, addr) {
				return connect.NewResponse(&gastownv1.GetAgentStatusResponse{Agent: agent}), nil
			}
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent not found: %s", formatAgentAddressForError(addr)))
}

// matchesAgentAddress checks if an agent's address matches the requested address.
func matchesAgentAddress(agentAddr, reqAddr *gastownv1.AgentAddress) bool {
	if agentAddr == nil || reqAddr == nil {
		return false
	}

	// If rig is specified, it must match
	if reqAddr.Rig != "" && agentAddr.Rig != reqAddr.Rig {
		return false
	}

	// If role is specified, it must match
	if reqAddr.Role != "" && agentAddr.Role != reqAddr.Role {
		return false
	}

	// If name is specified, it must match
	if reqAddr.Name != "" && agentAddr.Name != reqAddr.Name {
		return false
	}

	// At least one field must be specified
	if reqAddr.Rig == "" && reqAddr.Role == "" && reqAddr.Name == "" {
		return false
	}

	return true
}

// formatAgentAddressForError formats an agent address for error messages.
func formatAgentAddressForError(addr *gastownv1.AgentAddress) string {
	if addr == nil {
		return "<nil>"
	}
	if addr.Rig != "" && addr.Role != "" && addr.Name != "" {
		return fmt.Sprintf("%s/%s/%s", addr.Rig, addr.Role, addr.Name)
	}
	if addr.Rig != "" && addr.Role != "" {
		return fmt.Sprintf("%s/%s", addr.Rig, addr.Role)
	}
	if addr.Rig != "" {
		return addr.Rig
	}
	return addr.Name
}

func (s *StatusServer) WatchStatus(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchStatusRequest],
	stream *connect.ServerStream[gastownv1.StatusUpdate],
) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			status, err := s.collectTownStatus(true)
			if err != nil {
				log.Printf("WatchStatus error: %v", err)
				continue
			}
			if err := stream.Send(&gastownv1.StatusUpdate{
				Timestamp: timestamppb.Now(),
				Update:    &gastownv1.StatusUpdate_Town{Town: status},
			}); err != nil {
				return err
			}
		}
	}
}

func (s *StatusServer) collectTownStatus(fast bool) (*gastownv1.TownStatus, error) {
	// Load configs
	townConfigPath := constants.MayorTownPath(s.townRoot)
	townConfig, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		townConfig = &config.TownConfig{Name: filepath.Base(s.townRoot)}
	}

	rigsConfigPath := constants.MayorRigsPath(s.townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Discover rigs
	g := git.NewGit(s.townRoot)
	mgr := rig.NewManager(s.townRoot, rigsConfig, g)
	rigs, err := mgr.DiscoverRigs()
	if err != nil {
		return nil, fmt.Errorf("discovering rigs: %w", err)
	}

	// Get tmux sessions
	t := tmux.NewTmux()
	allSessions := make(map[string]bool)
	if sessions, err := t.ListSessions(); err == nil {
		for _, sess := range sessions {
			allSessions[sess] = true
		}
	}

	// Overseer info
	var overseer *gastownv1.OverseerInfo
	if oc, err := config.LoadOrDetectOverseer(s.townRoot); err == nil && oc != nil {
		overseer = &gastownv1.OverseerInfo{
			Name:     oc.Name,
			Email:    oc.Email,
			Username: oc.Username,
		}
		if !fast {
			mailRouter := mail.NewRouter(s.townRoot)
			if mb, err := mailRouter.GetMailbox("overseer"); err == nil {
				_, unread, _ := mb.Count()
				overseer.UnreadMail = int32(unread)
			}
		}
	}

	// Build status
	status := &gastownv1.TownStatus{
		Name:     townConfig.Name,
		Location: s.townRoot,
		Overseer: overseer,
	}

	// Global agents
	for _, agent := range []struct{ name, session, role string }{
		{"mayor", "gt-mayor", "mayor"},
		{"deacon", "gt-deacon", "deacon"},
	} {
		status.GlobalAgents = append(status.GlobalAgents, &gastownv1.AgentRuntime{
			Name:    agent.name,
			Address: &gastownv1.AgentAddress{Name: agent.name},
			Session: agent.session,
			Role:    agent.role,
			Running: allSessions[agent.session],
		})
	}

	// Rig status
	for _, r := range rigs {
		rs := &gastownv1.RigStatus{
			Name:        r.Name,
			Path:        r.Path,
			Polecats:    r.Polecats,
			HasWitness:  r.HasWitness,
			HasRefinery: r.HasRefinery,
		}

		// Crew workers
		crewGit := git.NewGit(r.Path)
		crewMgr := crew.NewManager(r, crewGit)
		if workers, err := crewMgr.List(); err == nil {
			for _, w := range workers {
				rs.Crews = append(rs.Crews, w.Name)
			}
		}

		// Rig agents
		if r.HasWitness {
			session := fmt.Sprintf("gt-%s-witness", r.Name)
			rs.Agents = append(rs.Agents, &gastownv1.AgentRuntime{
				Name:    "witness",
				Address: &gastownv1.AgentAddress{Rig: r.Name, Role: "witness"},
				Session: session,
				Role:    "witness",
				Running: allSessions[session],
			})
		}
		if r.HasRefinery {
			session := fmt.Sprintf("gt-%s-refinery", r.Name)
			rs.Agents = append(rs.Agents, &gastownv1.AgentRuntime{
				Name:    "refinery",
				Address: &gastownv1.AgentAddress{Rig: r.Name, Role: "refinery"},
				Session: session,
				Role:    "refinery",
				Running: allSessions[session],
			})
		}
		for _, p := range r.Polecats {
			session := fmt.Sprintf("gt-%s-%s", r.Name, p)
			rs.Agents = append(rs.Agents, &gastownv1.AgentRuntime{
				Name:    p,
				Address: &gastownv1.AgentAddress{Rig: r.Name, Role: "polecats", Name: p},
				Session: session,
				Role:    "polecat",
				Running: allSessions[session],
			})
		}
		for _, c := range rs.Crews {
			session := fmt.Sprintf("gt-%s-crew-%s", r.Name, c)
			rs.Agents = append(rs.Agents, &gastownv1.AgentRuntime{
				Name:    c,
				Address: &gastownv1.AgentAddress{Rig: r.Name, Role: "crew", Name: c},
				Session: session,
				Role:    "crew",
				Running: allSessions[session],
			})
		}

		status.Rigs = append(status.Rigs, rs)
	}

	return status, nil
}

// DecisionServer implements the DecisionService.
type DecisionServer struct {
	townRoot string
	bus      *eventbus.Bus
	poller   *eventbus.DecisionPoller // Optional: marks RPC-created decisions as seen
}

var _ gastownv1connect.DecisionServiceHandler = (*DecisionServer)(nil)

func NewDecisionServer(townRoot string, bus *eventbus.Bus) *DecisionServer {
	return &DecisionServer{townRoot: townRoot, bus: bus}
}

// SetPoller sets the decision poller for marking RPC-created decisions as seen.
// This prevents duplicate notifications when the poller finds decisions that were
// already published to the event bus via RPC.
func (s *DecisionServer) SetPoller(poller *eventbus.DecisionPoller) {
	s.poller = poller
}

func (s *DecisionServer) ListPending(
	ctx context.Context,
	req *connect.Request[gastownv1.ListPendingRequest],
) (*connect.Response[gastownv1.ListPendingResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	issues, err := client.ListDecisions()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var decisions []*gastownv1.Decision
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		if fields == nil {
			continue
		}

		var options []*gastownv1.DecisionOption
		for _, opt := range fields.Options {
			options = append(options, &gastownv1.DecisionOption{
				Label:       opt.Label,
				Description: opt.Description,
				Recommended: opt.Recommended,
			})
		}

		decisions = append(decisions, &gastownv1.Decision{
			Id:          issue.ID,
			Question:    fields.Question,
			Context:     fields.Context,
			Options:     options,
			ChosenIndex: int32(fields.ChosenIndex),
			Rationale:   fields.Rationale,
			RequestedBy: &gastownv1.AgentAddress{Name: fields.RequestedBy},
			Urgency:     toUrgency(fields.Urgency),
			Blockers:    fields.Blockers,
			Resolved:    fields.ChosenIndex > 0,
		})
	}

	return connect.NewResponse(&gastownv1.ListPendingResponse{
		Decisions: decisions,
		Total:     int32(len(decisions)),
	}), nil
}

func (s *DecisionServer) GetDecision(
	ctx context.Context,
	req *connect.Request[gastownv1.GetDecisionRequest],
) (*connect.Response[gastownv1.GetDecisionResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	issue, fields, err := client.GetDecisionBead(req.Msg.DecisionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if issue == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("decision not found: %s", req.Msg.DecisionId))
	}

	var options []*gastownv1.DecisionOption
	for _, opt := range fields.Options {
		options = append(options, &gastownv1.DecisionOption{
			Label:       opt.Label,
			Description: opt.Description,
			Recommended: opt.Recommended,
		})
	}

	decision := &gastownv1.Decision{
		Id:              issue.ID,
		Question:        fields.Question,
		Context:         fields.Context,
		Options:         options,
		ChosenIndex:     int32(fields.ChosenIndex),
		Rationale:       fields.Rationale,
		ResolvedBy:      fields.ResolvedBy,
		RequestedBy:     &gastownv1.AgentAddress{Name: fields.RequestedBy},
		Urgency:         toUrgency(fields.Urgency),
		Blockers:        fields.Blockers,
		Resolved:        fields.ChosenIndex > 0,
		ParentBead:      fields.ParentBeadID,
		ParentBeadTitle: fields.ParentBeadTitle,
	}

	return connect.NewResponse(&gastownv1.GetDecisionResponse{Decision: decision}), nil
}

func (s *DecisionServer) CreateDecision(
	ctx context.Context,
	req *connect.Request[gastownv1.CreateDecisionRequest],
) (*connect.Response[gastownv1.CreateDecisionResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Convert proto options to beads options
	var options []beads.DecisionOption
	for _, opt := range req.Msg.Options {
		options = append(options, beads.DecisionOption{
			Label:       opt.Label,
			Description: opt.Description,
			Recommended: opt.Recommended,
		})
	}

	// Build decision fields
	fields := &beads.DecisionFields{
		Question:      req.Msg.Question,
		Context:       req.Msg.Context,
		Options:       options,
		RequestedBy:   formatAgentAddress(req.Msg.RequestedBy),
		RequestedAt:   time.Now().Format(time.RFC3339),
		Urgency:       fromUrgency(req.Msg.Urgency),
		Blockers:      req.Msg.Blockers,
		PredecessorID: req.Msg.PredecessorId,
	}

	// Handle parent bead for epic-based channel routing
	var parentBeadTitle string
	if req.Msg.ParentBead != "" {
		fields.ParentBeadID = req.Msg.ParentBead
		// Look up the parent bead to get its title
		parentIssue, err := client.Show(req.Msg.ParentBead)
		if err == nil && parentIssue != nil {
			parentBeadTitle = parentIssue.Title
			fields.ParentBeadTitle = parentBeadTitle
		}
	}

	// Create the decision using bd decision create (canonical storage, hq-946577.39)
	// Retry with exponential backoff to handle transient errors (gt-d8jv14)
	var issue *beads.Issue
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			backoff := time.Duration(100<<attempt) * time.Millisecond
			log.Printf("CreateDecision retry %d/%d after %v (previous error: %v)",
				attempt+1, maxRetries, backoff, lastErr)
			select {
			case <-ctx.Done():
				return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
			case <-time.After(backoff):
			}
		}

		issue, lastErr = client.CreateBdDecision(fields)
		if lastErr == nil {
			break
		}

		// Log the error for debugging intermittent failures
		log.Printf("CreateDecision attempt %d/%d failed: %v (requestedBy: %s, question: %.50s...)",
			attempt+1, maxRetries, lastErr, fields.RequestedBy, fields.Question)
	}

	if lastErr != nil {
		log.Printf("CreateDecision failed after %d attempts: %v", maxRetries, lastErr)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("creating decision: %w", lastErr))
	}

	// Build response decision
	var protoOptions []*gastownv1.DecisionOption
	for _, opt := range options {
		protoOptions = append(protoOptions, &gastownv1.DecisionOption{
			Label:       opt.Label,
			Description: opt.Description,
			Recommended: opt.Recommended,
		})
	}

	decision := &gastownv1.Decision{
		Id:              issue.ID,
		Question:        fields.Question,
		Context:         fields.Context,
		Options:         protoOptions,
		RequestedBy:     req.Msg.RequestedBy,
		Urgency:         req.Msg.Urgency,
		Blockers:        fields.Blockers,
		Resolved:        false,
		PredecessorId:   fields.PredecessorID,
		ParentBead:      fields.ParentBeadID,
		ParentBeadTitle: parentBeadTitle,
	}

	// Publish event to bus for real-time notification
	if s.bus != nil {
		s.bus.PublishDecisionCreated(issue.ID, decision)
	}

	// Mark as seen by poller to prevent duplicate notification when poller discovers it
	if s.poller != nil {
		s.poller.MarkSeen(issue.ID)
	}

	return connect.NewResponse(&gastownv1.CreateDecisionResponse{Decision: decision}), nil
}

func (s *DecisionServer) Resolve(
	ctx context.Context,
	req *connect.Request[gastownv1.ResolveRequest],
) (*connect.Response[gastownv1.ResolveResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Get the resolver identity from request header or default
	resolvedBy := req.Header().Get("X-GT-Resolved-By")
	if resolvedBy == "" {
		resolvedBy = "rpc-client"
	}

	// Special handling for chosen_index = 0: "Other" with custom text
	// In this case, rationale contains the user's custom response text
	if req.Msg.ChosenIndex == 0 {
		if req.Msg.Rationale == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("custom text is required for 'Other' responses (chosen_index=0)"))
		}
		// Resolve with custom text (no predefined option)
		if err := client.ResolveDecisionWithCustomText(
			req.Msg.DecisionId,
			req.Msg.Rationale,
			resolvedBy,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolving decision with custom text: %w", err))
		}
	} else {
		// Standard resolution with predefined option
		if err := client.ResolveDecision(
			req.Msg.DecisionId,
			int(req.Msg.ChosenIndex),
			req.Msg.Rationale,
			resolvedBy,
		); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolving decision: %w", err))
		}
	}

	// Fetch the updated decision
	issue, fields, err := client.GetDecisionBead(req.Msg.DecisionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetching resolved decision: %w", err))
	}

	var options []*gastownv1.DecisionOption
	for _, opt := range fields.Options {
		options = append(options, &gastownv1.DecisionOption{
			Label:       opt.Label,
			Description: opt.Description,
			Recommended: opt.Recommended,
		})
	}

	decision := &gastownv1.Decision{
		Id:          issue.ID,
		Question:    fields.Question,
		Context:     fields.Context,
		Options:     options,
		ChosenIndex: int32(fields.ChosenIndex),
		Rationale:   fields.Rationale,
		ResolvedBy:  fields.ResolvedBy,
		RequestedBy: &gastownv1.AgentAddress{Name: fields.RequestedBy},
		Urgency:     toUrgency(fields.Urgency),
		Blockers:    fields.Blockers,
		Resolved:    true,
	}

	// Publish resolution event to bus
	if s.bus != nil {
		s.bus.PublishDecisionResolved(issue.ID, decision)
	}

	// Notify the requesting agent (mail + nudge + unblock)
	// Get chosen label from fields, with fallback to request index and proto options
	chosenLabel := ""
	chosenIndex := int(req.Msg.ChosenIndex) // Use request index as ground truth
	if chosenIndex == 0 {
		// "Other" response with custom text - use truncated rationale as label
		chosenLabel = "Other"
		if len(req.Msg.Rationale) > 50 {
			chosenLabel = "Other: " + req.Msg.Rationale[:47] + "..."
		} else if req.Msg.Rationale != "" {
			chosenLabel = "Other: " + req.Msg.Rationale
		}
	} else if chosenIndex > 0 && chosenIndex <= len(fields.Options) {
		chosenLabel = fields.Options[chosenIndex-1].Label
	}
	// Fallback: if fields.Options is empty but proto options exist, use those
	if chosenLabel == "" && chosenIndex > 0 && chosenIndex <= len(options) {
		chosenLabel = options[chosenIndex-1].Label
		log.Printf("WARN: Used proto options fallback for decision %s chosenLabel", issue.ID)
	}
	// Final fallback: log warning if still empty
	if chosenLabel == "" && chosenIndex > 0 {
		log.Printf("WARN: Empty chosenLabel for decision %s: ChosenIndex=%d, fields.Options=%d, proto.Options=%d",
			issue.ID, chosenIndex, len(fields.Options), len(options))
	}
	go notify.DecisionResolved(s.townRoot, issue.ID, *fields, chosenLabel, fields.Rationale, resolvedBy)

	return connect.NewResponse(&gastownv1.ResolveResponse{Decision: decision}), nil
}

func (s *DecisionServer) Cancel(
	ctx context.Context,
	req *connect.Request[gastownv1.CancelRequest],
) (*connect.Response[gastownv1.CancelResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Close the decision bead with cancelled status
	reason := req.Msg.Reason
	if reason == "" {
		reason = "Cancelled via RPC"
	}

	// Update labels to mark as cancelled
	if err := client.Update(req.Msg.DecisionId, beads.UpdateOptions{
		AddLabels:    []string{"decision:cancelled"},
		RemoveLabels: []string{"decision:pending"},
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("updating decision: %w", err))
	}

	// Close the bead
	if err := client.Close(req.Msg.DecisionId, reason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("closing decision: %w", err))
	}

	// Publish cancellation event to bus
	if s.bus != nil {
		s.bus.PublishDecisionCanceled(req.Msg.DecisionId)
	}

	return connect.NewResponse(&gastownv1.CancelResponse{}), nil
}

// maxSeenMapSize is the maximum number of entries in the seen map before cleanup.
// This prevents unbounded memory growth in long-running streams.
const maxSeenMapSize = 10000

func (s *DecisionServer) WatchDecisions(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchDecisionsRequest],
	stream *connect.ServerStream[gastownv1.Decision],
) error {
	seen := make(map[string]bool)
	// Helper to add to seen map with size limit
	markSeen := func(id string) {
		if len(seen) >= maxSeenMapSize {
			// Clear map to prevent unbounded growth
			seen = make(map[string]bool)
		}
		seen[id] = true
	}

	// First, send all existing pending decisions
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)
	issues, err := client.ListDecisions()
	if err == nil {
		for _, issue := range issues {
			markSeen(issue.ID)
			fields := beads.ParseDecisionFields(issue.Description)
			if fields == nil {
				continue
			}

			var options []*gastownv1.DecisionOption
			for _, opt := range fields.Options {
				options = append(options, &gastownv1.DecisionOption{
					Label:       opt.Label,
					Description: opt.Description,
					Recommended: opt.Recommended,
				})
			}

			if err := stream.Send(&gastownv1.Decision{
				Id:          issue.ID,
				Question:    fields.Question,
				Context:     fields.Context,
				Options:     options,
				Urgency:     toUrgency(fields.Urgency),
				RequestedBy: &gastownv1.AgentAddress{Name: fields.RequestedBy},
			}); err != nil {
				return err
			}
		}
	}

	// Subscribe to event bus for real-time updates
	if s.bus != nil {
		events, unsubscribe := s.bus.Subscribe()
		defer unsubscribe()

		// Backup polling (30 seconds) to catch decisions created via CLI
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil

			case event, ok := <-events:
				if !ok {
					return nil // Bus closed
				}
				// Only process decision created events
				if event.Type != eventbus.EventDecisionCreated {
					continue
				}
				if seen[event.DecisionID] {
					continue
				}
				markSeen(event.DecisionID)

				// Extract decision from event data
				if decision, ok := event.Data.(*gastownv1.Decision); ok {
					if err := stream.Send(decision); err != nil {
						return err
					}
				}

			case <-ticker.C:
				// Backup poll for decisions created via CLI (not through RPC)
				issues, err := client.ListDecisions()
				if err != nil {
					continue
				}

				for _, issue := range issues {
					if seen[issue.ID] {
						continue
					}
					markSeen(issue.ID)

					fields := beads.ParseDecisionFields(issue.Description)
					if fields == nil {
						continue
					}

					var options []*gastownv1.DecisionOption
					for _, opt := range fields.Options {
						options = append(options, &gastownv1.DecisionOption{
							Label:       opt.Label,
							Description: opt.Description,
							Recommended: opt.Recommended,
						})
					}

					if err := stream.Send(&gastownv1.Decision{
						Id:          issue.ID,
						Question:    fields.Question,
						Context:     fields.Context,
						Options:     options,
						Urgency:     toUrgency(fields.Urgency),
						RequestedBy: &gastownv1.AgentAddress{Name: fields.RequestedBy},
					}); err != nil {
						return err
					}
				}
			}
		}
	}

	// Fallback: no event bus, use polling only
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			issues, err := client.ListDecisions()
			if err != nil {
				continue
			}

			for _, issue := range issues {
				if seen[issue.ID] {
					continue
				}
				markSeen(issue.ID)

				fields := beads.ParseDecisionFields(issue.Description)
				if fields == nil {
					continue
				}

				var options []*gastownv1.DecisionOption
				for _, opt := range fields.Options {
					options = append(options, &gastownv1.DecisionOption{
						Label:       opt.Label,
						Description: opt.Description,
						Recommended: opt.Recommended,
					})
				}

				if err := stream.Send(&gastownv1.Decision{
					Id:          issue.ID,
					Question:    fields.Question,
					Context:     fields.Context,
					Options:     options,
					Urgency:     toUrgency(fields.Urgency),
					RequestedBy: &gastownv1.AgentAddress{Name: fields.RequestedBy},
				}); err != nil {
					return err
				}
			}
		}
	}
}

func toUrgency(s string) gastownv1.Urgency {
	switch s {
	case "high":
		return gastownv1.Urgency_URGENCY_HIGH
	case "medium":
		return gastownv1.Urgency_URGENCY_MEDIUM
	case "low":
		return gastownv1.Urgency_URGENCY_LOW
	default:
		return gastownv1.Urgency_URGENCY_UNSPECIFIED
	}
}

func fromUrgency(u gastownv1.Urgency) string {
	switch u {
	case gastownv1.Urgency_URGENCY_HIGH:
		return "high"
	case gastownv1.Urgency_URGENCY_MEDIUM:
		return "medium"
	case gastownv1.Urgency_URGENCY_LOW:
		return "low"
	default:
		return "medium"
	}
}

func formatAgentAddress(addr *gastownv1.AgentAddress) string {
	if addr == nil {
		return ""
	}
	if addr.Rig != "" && addr.Role != "" && addr.Name != "" {
		return fmt.Sprintf("%s/%s/%s", addr.Rig, addr.Role, addr.Name)
	}
	if addr.Rig != "" && addr.Role != "" {
		return fmt.Sprintf("%s/%s", addr.Rig, addr.Role)
	}
	return addr.Name
}

// MailServer implements the MailService.
type MailServer struct {
	townRoot string
}

var _ gastownv1connect.MailServiceHandler = (*MailServer)(nil)

func NewMailServer(townRoot string) *MailServer {
	return &MailServer{townRoot: townRoot}
}

func (s *MailServer) ListInbox(
	ctx context.Context,
	req *connect.Request[gastownv1.ListInboxRequest],
) (*connect.Response[gastownv1.ListInboxResponse], error) {
	mailRouter := mail.NewRouter(s.townRoot)

	address := "overseer"
	if req.Msg.Address != nil && req.Msg.Address.Name != "" {
		address = req.Msg.Address.Name
	}

	mailbox, err := mailRouter.GetMailbox(address)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	total, unread, _ := mailbox.Count()
	msgs, err := mailbox.List()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var messages []*gastownv1.Message
	for _, m := range msgs {
		if req.Msg.UnreadOnly && m.Read {
			continue
		}
		messages = append(messages, &gastownv1.Message{
			Id:        m.ID,
			From:      &gastownv1.AgentAddress{Name: m.From},
			To:        &gastownv1.AgentAddress{Name: m.To},
			Subject:   m.Subject,
			Body:      m.Body,
			Timestamp: timestamppb.New(m.Timestamp),
			Read:      m.Read,
			Priority:  toPriority(string(m.Priority)),
		})
		if req.Msg.Limit > 0 && int32(len(messages)) >= req.Msg.Limit {
			break
		}
	}

	return connect.NewResponse(&gastownv1.ListInboxResponse{
		Messages: messages,
		Total:    int32(total),
		Unread:   int32(unread),
	}), nil
}

func (s *MailServer) ReadMessage(
	ctx context.Context,
	req *connect.Request[gastownv1.ReadMessageRequest],
) (*connect.Response[gastownv1.ReadMessageResponse], error) {
	mailRouter := mail.NewRouter(s.townRoot)

	// Get message from all known mailboxes
	// First try overseer, then search other mailboxes
	addresses := []string{"overseer"}

	for _, addr := range addresses {
		mailbox, err := mailRouter.GetMailbox(addr)
		if err != nil {
			continue
		}

		msg, err := mailbox.Get(req.Msg.MessageId)
		if err == nil && msg != nil {
			return connect.NewResponse(&gastownv1.ReadMessageResponse{
				Message: s.mailToProto(msg),
			}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found: %s", req.Msg.MessageId))
}

func (s *MailServer) SendMessage(
	ctx context.Context,
	req *connect.Request[gastownv1.SendMessageRequest],
) (*connect.Response[gastownv1.SendMessageResponse], error) {
	mailRouter := mail.NewRouter(s.townRoot)

	// Build message
	msg := &mail.Message{
		From:      "rpc-client",
		To:        formatAgentAddress(req.Msg.To),
		Subject:   req.Msg.Subject,
		Body:      req.Msg.Body,
		Timestamp: time.Now(),
		Priority:  fromPriority(req.Msg.Priority),
		Type:      fromMessageType(req.Msg.Type),
	}

	if req.Msg.ReplyTo != "" {
		msg.ReplyTo = req.Msg.ReplyTo
	}

	// Handle CC recipients
	if len(req.Msg.Cc) > 0 {
		var ccAddrs []string
		for _, cc := range req.Msg.Cc {
			ccAddrs = append(ccAddrs, formatAgentAddress(cc))
		}
		msg.CC = ccAddrs
	}

	// Send the message
	if err := mailRouter.Send(msg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("sending message: %w", err))
	}

	return connect.NewResponse(&gastownv1.SendMessageResponse{
		MessageId: msg.ID,
	}), nil
}

func (s *MailServer) MarkRead(
	ctx context.Context,
	req *connect.Request[gastownv1.MarkReadRequest],
) (*connect.Response[gastownv1.MarkReadResponse], error) {
	mailRouter := mail.NewRouter(s.townRoot)

	// Try overseer mailbox first
	mailbox, err := mailRouter.GetMailbox("overseer")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Try to mark as read
	if err := mailbox.MarkReadOnly(req.Msg.MessageId); err != nil {
		if err == mail.ErrMessageNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&gastownv1.MarkReadResponse{}), nil
}

func (s *MailServer) DeleteMessage(
	ctx context.Context,
	req *connect.Request[gastownv1.DeleteMessageRequest],
) (*connect.Response[gastownv1.DeleteMessageResponse], error) {
	mailRouter := mail.NewRouter(s.townRoot)

	// Try overseer mailbox first
	mailbox, err := mailRouter.GetMailbox("overseer")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Archive the message (marks as read and removes from inbox)
	if err := mailbox.Archive(req.Msg.MessageId); err != nil {
		if err == mail.ErrMessageNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&gastownv1.DeleteMessageResponse{}), nil
}

func (s *MailServer) WatchInbox(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchInboxRequest],
	stream *connect.ServerStream[gastownv1.Message],
) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	seen := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			mailRouter := mail.NewRouter(s.townRoot)
			address := "overseer"
			if req.Msg.Address != nil && req.Msg.Address.Name != "" {
				address = req.Msg.Address.Name
			}

			mailbox, err := mailRouter.GetMailbox(address)
			if err != nil {
				continue
			}

			msgs, _ := mailbox.List()
			for _, m := range msgs {
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true

				if err := stream.Send(&gastownv1.Message{
					Id:        m.ID,
					From:      &gastownv1.AgentAddress{Name: m.From},
					To:        &gastownv1.AgentAddress{Name: m.To},
					Subject:   m.Subject,
					Body:      m.Body,
					Timestamp: timestamppb.New(m.Timestamp),
					Read:      m.Read,
					Priority:  toPriority(string(m.Priority)),
				}); err != nil {
					return err
				}
			}
		}
	}
}

func toPriority(s string) gastownv1.Priority {
	switch s {
	case "urgent":
		return gastownv1.Priority_PRIORITY_URGENT
	case "high":
		return gastownv1.Priority_PRIORITY_HIGH
	case "normal":
		return gastownv1.Priority_PRIORITY_NORMAL
	case "low":
		return gastownv1.Priority_PRIORITY_LOW
	default:
		return gastownv1.Priority_PRIORITY_UNSPECIFIED
	}
}

func fromPriority(p gastownv1.Priority) mail.Priority {
	switch p {
	case gastownv1.Priority_PRIORITY_URGENT:
		return mail.PriorityUrgent
	case gastownv1.Priority_PRIORITY_HIGH:
		return mail.PriorityHigh
	case gastownv1.Priority_PRIORITY_LOW:
		return mail.PriorityLow
	default:
		return mail.PriorityNormal
	}
}

func fromMessageType(t gastownv1.MessageType) mail.MessageType {
	switch t {
	case gastownv1.MessageType_MESSAGE_TYPE_TASK:
		return mail.TypeTask
	case gastownv1.MessageType_MESSAGE_TYPE_SCAVENGE:
		return mail.TypeScavenge
	case gastownv1.MessageType_MESSAGE_TYPE_NOTIFICATION:
		return mail.TypeNotification
	case gastownv1.MessageType_MESSAGE_TYPE_REPLY:
		return mail.TypeReply
	default:
		return mail.TypeNotification
	}
}

func toMessageType(t mail.MessageType) gastownv1.MessageType {
	switch t {
	case mail.TypeTask:
		return gastownv1.MessageType_MESSAGE_TYPE_TASK
	case mail.TypeScavenge:
		return gastownv1.MessageType_MESSAGE_TYPE_SCAVENGE
	case mail.TypeNotification:
		return gastownv1.MessageType_MESSAGE_TYPE_NOTIFICATION
	case mail.TypeReply:
		return gastownv1.MessageType_MESSAGE_TYPE_REPLY
	default:
		return gastownv1.MessageType_MESSAGE_TYPE_UNSPECIFIED
	}
}

func (s *MailServer) mailToProto(msg *mail.Message) *gastownv1.Message {
	protoMsg := &gastownv1.Message{
		Id:        msg.ID,
		From:      &gastownv1.AgentAddress{Name: msg.From},
		To:        &gastownv1.AgentAddress{Name: msg.To},
		Subject:   msg.Subject,
		Body:      msg.Body,
		Timestamp: timestamppb.New(msg.Timestamp),
		Read:      msg.Read,
		Priority:  toPriority(string(msg.Priority)),
		Type:      toMessageType(msg.Type),
		ThreadId:  msg.ThreadID,
		ReplyTo:   msg.ReplyTo,
		Pinned:    msg.Pinned,
	}

	// Handle CC recipients
	for _, cc := range msg.CC {
		protoMsg.Cc = append(protoMsg.Cc, &gastownv1.AgentAddress{Name: cc})
	}

	return protoMsg
}

// APIKeyInterceptor validates API keys for authentication.
func APIKeyInterceptor(apiKey string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if apiKey == "" {
				return next(ctx, req) // No auth configured
			}
			key := req.Header().Get("X-GT-API-Key")
			if key != apiKey {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid API key"))
			}
			return next(ctx, req)
		}
	}
}

// LoadTLSConfig loads TLS certificates for HTTPS.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS cert: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// RecoveryMiddleware wraps an HTTP handler with panic recovery to prevent
// single request panics from crashing the entire server.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered in HTTP handler: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// NewSSEHandler creates an HTTP handler for Server-Sent Events streaming of decision events.
// This provides a browser-friendly alternative to Connect-RPC streaming.
func NewSSEHandler(bus *eventbus.Bus, townRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Check for flusher support
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to events
		events, unsubscribe := bus.Subscribe()
		defer unsubscribe()

		// Send initial pending decisions
		townBeadsPath := beads.GetTownBeadsPath(townRoot)
		client := beads.New(townBeadsPath)
		issues, err := client.ListDecisions()
		if err == nil {
			for _, issue := range issues {
				fields := beads.ParseDecisionFields(issue.Description)
				if fields == nil {
					continue
				}
				// Send as SSE event
				fmt.Fprintf(w, "event: decision\n")
				fmt.Fprintf(w, "data: {\"id\":\"%s\",\"question\":\"%s\",\"urgency\":\"%s\",\"type\":\"pending\"}\n\n",
					issue.ID, escapeJSON(fields.Question), fields.Urgency)
				flusher.Flush()
			}
		}

		// Send connected event
		fmt.Fprintf(w, "event: connected\n")
		fmt.Fprintf(w, "data: {\"status\":\"connected\",\"subscribers\":%d}\n\n", bus.SubscriberCount())
		flusher.Flush()

		// Track seen decisions to avoid duplicates
		seen := make(map[string]bool)
		for _, issue := range issues {
			seen[issue.ID] = true
		}

		// Stream events
		// NOTE: CLI-created decisions are handled by the DecisionPoller, which publishes
		// to the event bus. The backup poll was removed to avoid duplicate notifications.
		ctx := r.Context()
		ticker := time.NewTicker(30 * time.Second) // Keepalive timer only
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				// Skip if already sent to this client (e.g., in initial pending list)
				if event.Type == eventbus.EventDecisionCreated {
					if decision, ok := event.Data.(*gastownv1.Decision); ok && decision != nil {
						if seen[decision.Id] {
							continue // Already sent via backup poll
						}
						seen[decision.Id] = true
						fmt.Fprintf(w, "event: decision\n")
						fmt.Fprintf(w, "data: {\"id\":\"%s\",\"question\":\"%s\",\"urgency\":\"%s\",\"type\":\"created\"}\n\n",
							decision.Id, escapeJSON(decision.Question), fromUrgency(decision.Urgency))
						flusher.Flush()
						continue
					}
				}
				// For other event types (resolved, canceled), always send
				eventType := "unknown"
				switch event.Type {
				case eventbus.EventDecisionResolved:
					eventType = "resolved"
				case eventbus.EventDecisionCanceled:
					eventType = "canceled"
				}
				seen[event.DecisionID] = true
				fmt.Fprintf(w, "event: decision\n")
				fmt.Fprintf(w, "data: {\"id\":\"%s\",\"type\":\"%s\"}\n\n", event.DecisionID, eventType)
				flusher.Flush()

			case <-ticker.C:
				// Send keepalive to prevent connection timeout
				// NOTE: CLI-created decisions are handled by the DecisionPoller, which
				// publishes to the event bus. We removed the redundant backup poll here
				// to avoid duplicate notifications (the poller and backup poll were racing).
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

// escapeJSON escapes a string for JSON output.
func escapeJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ConvoyServer implements the ConvoyService.
type ConvoyServer struct {
	townRoot string
}

var _ gastownv1connect.ConvoyServiceHandler = (*ConvoyServer)(nil)

func NewConvoyServer(townRoot string) *ConvoyServer {
	return &ConvoyServer{townRoot: townRoot}
}

func (s *ConvoyServer) ListConvoys(
	ctx context.Context,
	req *connect.Request[gastownv1.ListConvoysRequest],
) (*connect.Response[gastownv1.ListConvoysResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Determine status filter for bd list
	statusFilter := "open"
	switch req.Msg.Status {
	case gastownv1.ConvoyStatusFilter_CONVOY_STATUS_FILTER_CLOSED:
		statusFilter = "closed"
	case gastownv1.ConvoyStatusFilter_CONVOY_STATUS_FILTER_ALL:
		statusFilter = "all"
	}

	// Get convoys from beads using List with Type option
	issues, err := client.List(beads.ListOptions{
		Type:     "convoy",
		Status:   statusFilter,
		Priority: -1,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var convoys []*gastownv1.Convoy
	for _, issue := range issues {
		convoy := s.issueToConvoy(*issue)

		// If tree view requested, get tracked issues
		if req.Msg.Tree {
			tracked := s.getTrackedIssues(townBeadsPath, issue.ID)
			convoy.TrackedCount = int32(len(tracked))
			completed := 0
			for _, t := range tracked {
				if t.Status == "closed" {
					completed++
				}
			}
			convoy.CompletedCount = int32(completed)
			convoy.Progress = fmt.Sprintf("%d/%d", completed, len(tracked))
		}

		convoys = append(convoys, convoy)
	}

	return connect.NewResponse(&gastownv1.ListConvoysResponse{
		Convoys: convoys,
		Total:   int32(len(convoys)),
	}), nil
}

func (s *ConvoyServer) GetConvoyStatus(
	ctx context.Context,
	req *connect.Request[gastownv1.GetConvoyStatusRequest],
) (*connect.Response[gastownv1.GetConvoyStatusResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	issue, err := client.Show(req.Msg.ConvoyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if issue == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("convoy not found: %s", req.Msg.ConvoyId))
	}

	convoy := s.issueToConvoy(*issue)
	tracked := s.getTrackedIssues(townBeadsPath, issue.ID)

	var protoTracked []*gastownv1.TrackedIssue
	completed := 0
	for _, t := range tracked {
		if t.Status == "closed" {
			completed++
		}
		protoTracked = append(protoTracked, &gastownv1.TrackedIssue{
			Id:        t.ID,
			Title:     t.Title,
			Status:    t.Status,
			IssueType: t.IssueType,
			Assignee:  t.Assignee,
			Worker:    t.Worker,
			WorkerAge: t.WorkerAge,
		})
	}

	convoy.TrackedCount = int32(len(tracked))
	convoy.CompletedCount = int32(completed)
	convoy.Progress = fmt.Sprintf("%d/%d", completed, len(tracked))

	return connect.NewResponse(&gastownv1.GetConvoyStatusResponse{
		Convoy:    convoy,
		Tracked:   protoTracked,
		Completed: int32(completed),
		Total:     int32(len(tracked)),
	}), nil
}

func (s *ConvoyServer) CreateConvoy(
	ctx context.Context,
	req *connect.Request[gastownv1.CreateConvoyRequest],
) (*connect.Response[gastownv1.CreateConvoyResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Build description with metadata
	description := fmt.Sprintf("Convoy tracking %d issues", len(req.Msg.IssueIds))
	if req.Msg.Owner != "" {
		description += fmt.Sprintf("\nOwner: %s", req.Msg.Owner)
	}
	if req.Msg.Notify != "" {
		description += fmt.Sprintf("\nNotify: %s", req.Msg.Notify)
	}
	if req.Msg.Molecule != "" {
		description += fmt.Sprintf("\nMolecule: %s", req.Msg.Molecule)
	}
	if req.Msg.Owned {
		description += "\nOwned: true"
	}
	if req.Msg.MergeStrategy != "" {
		description += fmt.Sprintf("\nMerge: %s", req.Msg.MergeStrategy)
	}

	// Create the convoy bead
	issue, err := client.Create(beads.CreateOptions{
		Type:        "convoy",
		Title:       req.Msg.Name,
		Description: description,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("creating convoy: %w", err))
	}

	// Add tracking dependencies for each issue
	trackedCount := 0
	for _, issueID := range req.Msg.IssueIds {
		if err := client.AddTypedDependency(issue.ID, issueID, "tracks"); err != nil {
			log.Printf("WARN: couldn't track %s: %v", issueID, err)
		} else {
			trackedCount++
		}
	}

	convoy := s.issueToConvoy(*issue)
	convoy.TrackedCount = int32(trackedCount)

	return connect.NewResponse(&gastownv1.CreateConvoyResponse{
		Convoy:       convoy,
		TrackedCount: int32(trackedCount),
	}), nil
}

func (s *ConvoyServer) AddToConvoy(
	ctx context.Context,
	req *connect.Request[gastownv1.AddToConvoyRequest],
) (*connect.Response[gastownv1.AddToConvoyResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Get current convoy
	issue, err := client.Show(req.Msg.ConvoyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if issue == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("convoy not found: %s", req.Msg.ConvoyId))
	}

	// Reopen if closed
	reopened := false
	if issue.Status == "closed" {
		openStatus := "open"
		if err := client.Update(issue.ID, beads.UpdateOptions{Status: &openStatus}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("reopening convoy: %w", err))
		}
		reopened = true
	}

	// Add tracking dependencies
	addedCount := 0
	for _, issueID := range req.Msg.IssueIds {
		if err := client.AddTypedDependency(issue.ID, issueID, "tracks"); err != nil {
			log.Printf("WARN: couldn't add %s: %v", issueID, err)
		} else {
			addedCount++
		}
	}

	convoy := s.issueToConvoy(*issue)
	if reopened {
		convoy.Status = "open"
	}

	return connect.NewResponse(&gastownv1.AddToConvoyResponse{
		Convoy:     convoy,
		AddedCount: int32(addedCount),
		Reopened:   reopened,
	}), nil
}

func (s *ConvoyServer) CloseConvoy(
	ctx context.Context,
	req *connect.Request[gastownv1.CloseConvoyRequest],
) (*connect.Response[gastownv1.CloseConvoyResponse], error) {
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	// Get current convoy
	issue, err := client.Show(req.Msg.ConvoyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if issue == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("convoy not found: %s", req.Msg.ConvoyId))
	}

	// Idempotent: if already closed, just return
	if issue.Status == "closed" {
		convoy := s.issueToConvoy(*issue)
		return connect.NewResponse(&gastownv1.CloseConvoyResponse{Convoy: convoy}), nil
	}

	// Close the convoy
	reason := req.Msg.Reason
	if reason == "" {
		reason = "Closed via RPC"
	}
	if err := client.CloseWithReason(reason, issue.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("closing convoy: %w", err))
	}

	convoy := s.issueToConvoy(*issue)
	convoy.Status = "closed"

	return connect.NewResponse(&gastownv1.CloseConvoyResponse{Convoy: convoy}), nil
}

func (s *ConvoyServer) WatchConvoys(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchConvoysRequest],
	stream *connect.ServerStream[gastownv1.ConvoyUpdate],
) error {
	// For now, implement as polling (similar to decision watch)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	seen := make(map[string]string) // convoy ID -> status

	// Initial fetch
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)

	issues, _ := client.List(beads.ListOptions{Type: "convoy", Status: "all", Priority: -1})
	for _, issue := range issues {
		seen[issue.ID] = issue.Status
		convoy := s.issueToConvoy(*issue)
		if err := stream.Send(&gastownv1.ConvoyUpdate{
			Timestamp:  timestamppb.Now(),
			ConvoyId:   issue.ID,
			UpdateType: "existing",
			Convoy:     convoy,
		}); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			issues, err := client.List(beads.ListOptions{Type: "convoy", Status: "all", Priority: -1})
			if err != nil {
				continue
			}

			for _, issue := range issues {
				oldStatus, exists := seen[issue.ID]
				if !exists {
					// New convoy
					seen[issue.ID] = issue.Status
					convoy := s.issueToConvoy(*issue)
					if err := stream.Send(&gastownv1.ConvoyUpdate{
						Timestamp:  timestamppb.Now(),
						ConvoyId:   issue.ID,
						UpdateType: "created",
						Convoy:     convoy,
					}); err != nil {
						return err
					}
				} else if oldStatus != issue.Status {
					// Status changed
					seen[issue.ID] = issue.Status
					convoy := s.issueToConvoy(*issue)
					updateType := "updated"
					if issue.Status == "closed" {
						updateType = "closed"
					}
					if err := stream.Send(&gastownv1.ConvoyUpdate{
						Timestamp:  timestamppb.Now(),
						ConvoyId:   issue.ID,
						UpdateType: updateType,
						Convoy:     convoy,
					}); err != nil {
						return err
					}
				}
			}
		}
	}
}

// issueToConvoy converts a beads issue to a proto Convoy.
func (s *ConvoyServer) issueToConvoy(issue beads.Issue) *gastownv1.Convoy {
	convoy := &gastownv1.Convoy{
		Id:     issue.ID,
		Title:  issue.Title,
		Status: issue.Status,
	}

	// Parse metadata from description
	if issue.Description != "" {
		for _, line := range strings.Split(issue.Description, "\n") {
			if strings.HasPrefix(line, "Owner: ") {
				convoy.Owner = strings.TrimPrefix(line, "Owner: ")
			} else if strings.HasPrefix(line, "Notify: ") {
				convoy.Notify = strings.TrimPrefix(line, "Notify: ")
			} else if strings.HasPrefix(line, "Molecule: ") {
				convoy.Molecule = strings.TrimPrefix(line, "Molecule: ")
			} else if strings.HasPrefix(line, "Owned: ") {
				convoy.Owned = strings.TrimPrefix(line, "Owned: ") == "true"
			} else if strings.HasPrefix(line, "Merge: ") {
				convoy.MergeStrategy = strings.TrimPrefix(line, "Merge: ")
			}
		}
	}

	// Parse timestamps
	if issue.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, issue.CreatedAt); err == nil {
			convoy.CreatedAt = timestamppb.New(t)
		}
	}
	if issue.ClosedAt != "" {
		if t, err := time.Parse(time.RFC3339, issue.ClosedAt); err == nil {
			convoy.ClosedAt = timestamppb.New(t)
		}
	}

	return convoy
}

// trackedIssueInfo holds info about an issue being tracked by a convoy.
type trackedIssueInfo struct {
	ID        string
	Title     string
	Status    string
	IssueType string
	Assignee  string
	Worker    string
	WorkerAge string
}

// getTrackedIssues fetches issues tracked by a convoy.
func (s *ConvoyServer) getTrackedIssues(townBeadsPath, convoyID string) []trackedIssueInfo {
	client := beads.New(townBeadsPath)

	deps, err := client.ListDependencies(convoyID, "down", "tracks")
	if err != nil {
		return nil
	}

	var tracked []trackedIssueInfo
	for _, dep := range deps {
		tracked = append(tracked, trackedIssueInfo{
			ID:        dep.ID,
			Title:     dep.Title,
			Status:    dep.Status,
			IssueType: dep.Type,
			Assignee:  dep.Assignee,
		})
	}

	return tracked
}

// ActivityServer implements the ActivityService.
type ActivityServer struct {
	townRoot string
}

var _ gastownv1connect.ActivityServiceHandler = (*ActivityServer)(nil)

func NewActivityServer(townRoot string) *ActivityServer {
	return &ActivityServer{townRoot: townRoot}
}

func (s *ActivityServer) ListEvents(
	ctx context.Context,
	req *connect.Request[gastownv1.ListEventsRequest],
) (*connect.Response[gastownv1.ListEventsResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var events []*gastownv1.ActivityEvent
	var totalCount int

	if req.Msg.Curated {
		// Read from curated feed file
		events, totalCount = s.readFeedEvents(req.Msg.Filter, limit)
	} else {
		// Read from raw events file
		events, totalCount = s.readRawEvents(req.Msg.Filter, limit)
	}

	return connect.NewResponse(&gastownv1.ListEventsResponse{
		Events:     events,
		TotalCount: int32(totalCount),
	}), nil
}

func (s *ActivityServer) WatchEvents(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchEventsRequest],
	stream *connect.ServerStream[gastownv1.ActivityEvent],
) error {
	// Determine which file to watch
	eventsFile := filepath.Join(s.townRoot, ".events.jsonl")
	if req.Msg.Curated {
		eventsFile = filepath.Join(s.townRoot, ".feed.jsonl")
	}

	// Send backfill if requested
	if req.Msg.IncludeBackfill {
		backfillCount := int(req.Msg.BackfillCount)
		if backfillCount <= 0 {
			backfillCount = 10
		}
		if backfillCount > 100 {
			backfillCount = 100
		}

		var events []*gastownv1.ActivityEvent
		if req.Msg.Curated {
			events, _ = s.readFeedEvents(req.Msg.Filter, backfillCount)
		} else {
			events, _ = s.readRawEvents(req.Msg.Filter, backfillCount)
		}

		// Send in reverse order (oldest first for backfill)
		for i := len(events) - 1; i >= 0; i-- {
			if err := stream.Send(events[i]); err != nil {
				return err
			}
		}
	}

	// Open file for tailing
	file, err := os.Open(eventsFile)
	if err != nil {
		// If file doesn't exist, create it
		file, err = os.OpenFile(eventsFile, os.O_RDONLY|os.O_CREATE, 0644)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("opening events file: %w", err))
		}
	}
	defer file.Close()

	// Seek to end
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("seeking to end: %w", err))
	}

	reader := bufio.NewReader(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Read available lines
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break // No more data
				}

				event := s.parseLine(line, req.Msg.Curated)
				if event == nil {
					continue
				}

				// Apply filter
				if !s.matchesFilter(event, req.Msg.Filter) {
					continue
				}

				if err := stream.Send(event); err != nil {
					return err
				}
			}
		}
	}
}

func (s *ActivityServer) EmitEvent(
	ctx context.Context,
	req *connect.Request[gastownv1.EmitEventRequest],
) (*connect.Response[gastownv1.EmitEventResponse], error) {
	if req.Msg.Type == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("event type is required"))
	}
	if req.Msg.Actor == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("actor is required"))
	}

	// Convert visibility
	visibility := "feed"
	switch req.Msg.Visibility {
	case gastownv1.Visibility_VISIBILITY_AUDIT:
		visibility = "audit"
	case gastownv1.Visibility_VISIBILITY_BOTH:
		visibility = "both"
	}

	// Convert payload from protobuf Struct to map
	var payload map[string]interface{}
	if req.Msg.Payload != nil {
		payload = structToMap(req.Msg.Payload)
	}

	// Write event using the events package
	timestamp := time.Now().UTC().Format(time.RFC3339)
	event := map[string]interface{}{
		"ts":         timestamp,
		"source":     "rpc",
		"type":       req.Msg.Type,
		"actor":      req.Msg.Actor,
		"visibility": visibility,
	}
	if payload != nil {
		event["payload"] = payload
	}

	// Append to events file
	eventsPath := filepath.Join(s.townRoot, ".events.jsonl")
	data, err := json.Marshal(event)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshaling event: %w", err))
	}
	data = append(data, '\n')

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("opening events file: %w", err))
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("writing event: %w", err))
	}

	return connect.NewResponse(&gastownv1.EmitEventResponse{
		Timestamp: timestamp,
		Success:   true,
	}), nil
}

func (s *ActivityServer) StreamLogs(
	ctx context.Context,
	req *connect.Request[gastownv1.StreamLogsRequest],
	stream *connect.ServerStream[gastownv1.LogEntry],
) error {
	agent := req.Msg.Agent
	logType := req.Msg.LogType
	if logType == "" {
		logType = "activity"
	}

	tailLines := int(req.Msg.TailLines)
	if tailLines <= 0 {
		tailLines = 50
	}
	if tailLines > 500 {
		tailLines = 500
	}

	// Resolve the log file path based on log type
	logFile, err := s.resolveLogFile(logType)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Send historical tail lines
	if err := s.sendTailLines(logFile, agent, logType, tailLines, stream); err != nil {
		if !os.IsNotExist(err) {
			return connect.NewError(connect.CodeUnavailable, fmt.Errorf("reading log file: %w", err))
		}
	}

	// If not following, we're done after the tail
	if !req.Msg.Follow {
		return nil
	}

	// Open file for tailing
	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			file, err = os.OpenFile(logFile, os.O_RDONLY|os.O_CREATE, 0644)
			if err != nil {
				return connect.NewError(connect.CodeUnavailable, fmt.Errorf("creating log file: %w", err))
			}
		} else {
			return connect.NewError(connect.CodeUnavailable, fmt.Errorf("opening log file: %w", err))
		}
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("seeking log file: %w", err))
	}

	reader := bufio.NewReader(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				entry := s.parseLogLineEntry(line, logType)
				if entry == nil {
					continue
				}

				if agent != "" && entry.Agent != "" && !strings.Contains(entry.Agent, agent) {
					continue
				}

				if err := stream.Send(entry); err != nil {
					return err
				}
			}
		}
	}
}

func (s *ActivityServer) resolveLogFile(logType string) (string, error) {
	switch logType {
	case "activity":
		return filepath.Join(s.townRoot, ".events.jsonl"), nil
	case "town":
		return filepath.Join(s.townRoot, "logs", "town.log"), nil
	case "daemon":
		return filepath.Join(s.townRoot, "daemon", "daemon.log"), nil
	default:
		return "", fmt.Errorf("unknown log type %q: must be activity, town, or daemon", logType)
	}
}

func (s *ActivityServer) sendTailLines(
	logFile, agent, logType string,
	tailLines int,
	stream *connect.ServerStream[gastownv1.LogEntry],
) error {
	data, err := os.ReadFile(logFile)
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	var entries []*gastownv1.LogEntry
	for i := len(lines) - 1; i >= 0 && len(entries) < tailLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		entry := s.parseLogLineEntry(line, logType)
		if entry == nil {
			continue
		}

		if agent != "" && entry.Agent != "" && !strings.Contains(entry.Agent, agent) {
			continue
		}

		entries = append(entries, entry)
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if err := stream.Send(entries[i]); err != nil {
			return err
		}
	}

	return nil
}

func (s *ActivityServer) parseLogLineEntry(line, logType string) *gastownv1.LogEntry {
	switch logType {
	case "activity":
		return s.parseActivityLogLine(line)
	case "town":
		return s.parseTownLogLine(line)
	case "daemon":
		return s.parseDaemonLogLine(line)
	default:
		return nil
	}
}

func (s *ActivityServer) parseActivityLogLine(line string) *gastownv1.LogEntry {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil
	}

	entry := &gastownv1.LogEntry{
		Source: "activity",
		Level:  "info",
	}

	if ts, ok := raw["ts"].(string); ok {
		entry.Timestamp = ts
	}
	if actor, ok := raw["actor"].(string); ok {
		entry.Agent = actor
	}
	if typ, ok := raw["type"].(string); ok {
		entry.EventType = typ
	}

	var parts []string
	if typ, ok := raw["type"].(string); ok {
		parts = append(parts, typ)
	}
	if actor, ok := raw["actor"].(string); ok {
		parts = append(parts, "by", actor)
	}
	if payload, ok := raw["payload"].(map[string]interface{}); ok {
		if summary, ok := payload["summary"].(string); ok {
			parts = append(parts, "-", summary)
		}
	}
	if summary, ok := raw["summary"].(string); ok && summary != "" {
		parts = append(parts, "-", summary)
	}
	entry.Message = strings.Join(parts, " ")

	return entry
}

func (s *ActivityServer) parseTownLogLine(line string) *gastownv1.LogEntry {
	entry := &gastownv1.LogEntry{
		Source: "town",
		Level:  "info",
	}

	if len(line) < 19 {
		entry.Message = line
		return entry
	}

	ts, err := time.Parse("2006-01-02 15:04:05", line[:19])
	if err != nil {
		entry.Message = line
		return entry
	}
	entry.Timestamp = ts.Format(time.RFC3339)

	rest := line[20:]

	if len(rest) > 2 && rest[0] == '[' {
		closeBracket := strings.Index(rest, "]")
		if closeBracket > 0 {
			entry.EventType = rest[1:closeBracket]
			rest = rest[closeBracket+1:]
			if len(rest) > 0 && rest[0] == ' ' {
				rest = rest[1:]
			}
		}
	}

	if spaceIdx := strings.Index(rest, " "); spaceIdx > 0 {
		entry.Agent = rest[:spaceIdx]
		entry.Message = rest[spaceIdx+1:]
	} else {
		entry.Agent = rest
	}

	switch entry.EventType {
	case "crash", "session_death", "mass_death":
		entry.Level = "error"
	case "escalation_sent", "polecat_nudged":
		entry.Level = "warn"
	}

	return entry
}

func (s *ActivityServer) parseDaemonLogLine(line string) *gastownv1.LogEntry {
	entry := &gastownv1.LogEntry{
		Source: "daemon",
		Level:  "info",
	}

	if len(line) >= 19 {
		ts, err := time.Parse("2006/01/02 15:04:05", line[:19])
		if err == nil {
			entry.Timestamp = ts.Format(time.RFC3339)
			if len(line) > 20 {
				entry.Message = line[20:]
			}
			msgLower := strings.ToLower(entry.Message)
			if strings.Contains(msgLower, "error") || strings.Contains(msgLower, "fatal") {
				entry.Level = "error"
			} else if strings.Contains(msgLower, "warn") {
				entry.Level = "warn"
			}
			return entry
		}
	}

	entry.Message = line
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return entry
}

// readRawEvents reads events from the raw events file.
func (s *ActivityServer) readRawEvents(filter *gastownv1.EventFilter, limit int) ([]*gastownv1.ActivityEvent, int) {
	eventsPath := filepath.Join(s.townRoot, ".events.jsonl")
	return s.readEventsFromFile(eventsPath, filter, limit, false)
}

// readFeedEvents reads events from the curated feed file.
func (s *ActivityServer) readFeedEvents(filter *gastownv1.EventFilter, limit int) ([]*gastownv1.ActivityEvent, int) {
	feedPath := filepath.Join(s.townRoot, ".feed.jsonl")
	return s.readEventsFromFile(feedPath, filter, limit, true)
}

// readEventsFromFile reads events from a JSONL file with filtering.
func (s *ActivityServer) readEventsFromFile(filePath string, filter *gastownv1.EventFilter, limit int, isCurated bool) ([]*gastownv1.ActivityEvent, int) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0
	}

	lines := strings.Split(string(data), "\n")
	var events []*gastownv1.ActivityEvent
	totalCount := 0

	// Read from end (newest first)
	for i := len(lines) - 1; i >= 0 && len(events) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		event := s.parseLine(line, isCurated)
		if event == nil {
			continue
		}

		totalCount++

		// Apply filter
		if !s.matchesFilter(event, filter) {
			continue
		}

		events = append(events, event)
	}

	return events, totalCount
}

// parseLine parses a JSON line into an ActivityEvent.
func (s *ActivityServer) parseLine(line string, isCurated bool) *gastownv1.ActivityEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil
	}

	event := &gastownv1.ActivityEvent{}

	if ts, ok := raw["ts"].(string); ok {
		event.Timestamp = ts
	}
	if source, ok := raw["source"].(string); ok {
		event.Source = source
	}
	if typ, ok := raw["type"].(string); ok {
		event.Type = typ
	}
	if actor, ok := raw["actor"].(string); ok {
		event.Actor = actor
	}
	if payload, ok := raw["payload"].(map[string]interface{}); ok {
		event.Payload = mapToStruct(payload)
	}
	if visibility, ok := raw["visibility"].(string); ok {
		switch visibility {
		case "audit":
			event.Visibility = gastownv1.Visibility_VISIBILITY_AUDIT
		case "feed":
			event.Visibility = gastownv1.Visibility_VISIBILITY_FEED
		case "both":
			event.Visibility = gastownv1.Visibility_VISIBILITY_BOTH
		}
	}

	// Curated feed has additional fields
	if isCurated {
		if summary, ok := raw["summary"].(string); ok {
			event.Summary = summary
		}
		if count, ok := raw["count"].(float64); ok {
			event.Count = int32(count)
		}
	}

	return event
}

// matchesFilter checks if an event matches the given filter.
func (s *ActivityServer) matchesFilter(event *gastownv1.ActivityEvent, filter *gastownv1.EventFilter) bool {
	if filter == nil {
		return true
	}

	// Filter by types
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by actor
	if filter.Actor != "" && event.Actor != filter.Actor {
		return false
	}

	// Filter by visibility
	if filter.Visibility != gastownv1.Visibility_VISIBILITY_UNSPECIFIED && event.Visibility != filter.Visibility {
		return false
	}

	// Filter by timestamp
	if filter.After != "" {
		afterTime, err := time.Parse(time.RFC3339, filter.After)
		if err == nil {
			eventTime, err := time.Parse(time.RFC3339, event.Timestamp)
			if err == nil && eventTime.Before(afterTime) {
				return false
			}
		}
	}

	if filter.Before != "" {
		beforeTime, err := time.Parse(time.RFC3339, filter.Before)
		if err == nil {
			eventTime, err := time.Parse(time.RFC3339, event.Timestamp)
			if err == nil && eventTime.After(beforeTime) {
				return false
			}
		}
	}

	return true
}

// structToMap converts a protobuf Struct to a Go map.
func structToMap(s *structpb.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range s.Fields {
		result[k] = valueToInterface(v)
	}
	return result
}

// valueToInterface converts a protobuf Value to a Go interface{}.
func valueToInterface(v *structpb.Value) interface{} {
	if v == nil {
		return nil
	}
	switch x := v.Kind.(type) {
	case *structpb.Value_NullValue:
		return nil
	case *structpb.Value_NumberValue:
		return x.NumberValue
	case *structpb.Value_StringValue:
		return x.StringValue
	case *structpb.Value_BoolValue:
		return x.BoolValue
	case *structpb.Value_StructValue:
		return structToMap(x.StructValue)
	case *structpb.Value_ListValue:
		var list []interface{}
		for _, item := range x.ListValue.Values {
			list = append(list, valueToInterface(item))
		}
		return list
	default:
		return nil
	}
}

// mapToStruct converts a Go map to a protobuf Struct.
func mapToStruct(m map[string]interface{}) *structpb.Struct {
	if m == nil {
		return nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return s
}

// TerminalServer implements the TerminalService.
type TerminalServer struct {
	tmux *tmux.Tmux
}

var _ gastownv1connect.TerminalServiceHandler = (*TerminalServer)(nil)

func NewTerminalServer() *TerminalServer {
	return &TerminalServer{tmux: tmux.NewTmux()}
}

func (s *TerminalServer) PeekSession(
	ctx context.Context,
	req *connect.Request[gastownv1.PeekSessionRequest],
) (*connect.Response[gastownv1.PeekSessionResponse], error) {
	session := req.Msg.Session
	if session == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session name is required"))
	}

	// Check if session exists
	exists, err := s.tmux.HasSession(session)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if !exists {
		return connect.NewResponse(&gastownv1.PeekSessionResponse{
			Exists: false,
		}), nil
	}

	// Determine how many lines to capture
	lines := int(req.Msg.Lines)
	if lines <= 0 {
		lines = 50
	}
	if lines > 1000 {
		lines = 1000
	}

	var output string
	if req.Msg.All {
		output, err = s.tmux.CapturePaneAll(session)
	} else {
		output, err = s.tmux.CapturePane(session, lines)
	}

	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("capturing pane: %w", err))
	}

	// Split into lines
	var lineSlice []string
	if output != "" {
		lineSlice = strings.Split(output, "\n")
	}

	return connect.NewResponse(&gastownv1.PeekSessionResponse{
		Output: output,
		Lines:  lineSlice,
		Exists: true,
	}), nil
}

func (s *TerminalServer) ListSessions(
	ctx context.Context,
	req *connect.Request[gastownv1.ListSessionsRequest],
) (*connect.Response[gastownv1.ListSessionsResponse], error) {
	sessions, err := s.tmux.ListSessions()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Apply prefix filter if specified
	if req.Msg.Prefix != "" {
		var filtered []string
		for _, sess := range sessions {
			if strings.HasPrefix(sess, req.Msg.Prefix) {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}

	return connect.NewResponse(&gastownv1.ListSessionsResponse{
		Sessions: sessions,
	}), nil
}

func (s *TerminalServer) HasSession(
	ctx context.Context,
	req *connect.Request[gastownv1.HasSessionRequest],
) (*connect.Response[gastownv1.HasSessionResponse], error) {
	if req.Msg.Session == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session name is required"))
	}

	exists, err := s.tmux.HasSession(req.Msg.Session)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&gastownv1.HasSessionResponse{
		Exists: exists,
	}), nil
}

func (s *TerminalServer) WatchSession(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchSessionRequest],
	stream *connect.ServerStream[gastownv1.TerminalUpdate],
) error {
	session := req.Msg.Session
	if session == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session name is required"))
	}

	lines := int(req.Msg.Lines)
	if lines <= 0 {
		lines = 50
	}
	if lines > 1000 {
		lines = 1000
	}

	intervalMs := int(req.Msg.IntervalMs)
	if intervalMs < 100 {
		intervalMs = 1000
	}

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Check if session exists
			exists, err := s.tmux.HasSession(session)
			if err != nil {
				// Send error update
				if err := stream.Send(&gastownv1.TerminalUpdate{
					Exists:    false,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					return err
				}
				continue
			}

			if !exists {
				// Session no longer exists
				if err := stream.Send(&gastownv1.TerminalUpdate{
					Exists:    false,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					return err
				}
				return nil // Stop watching when session dies
			}

			// Capture output
			output, err := s.tmux.CapturePane(session, lines)
			if err != nil {
				continue
			}

			var lineSlice []string
			if output != "" {
				lineSlice = strings.Split(output, "\n")
			}

			if err := stream.Send(&gastownv1.TerminalUpdate{
				Output:    output,
				Lines:     lineSlice,
				Exists:    true,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
}

func (s *TerminalServer) SendInput(
	ctx context.Context,
	req *connect.Request[gastownv1.SendInputRequest],
) (*connect.Response[gastownv1.SendInputResponse], error) {
	session := req.Msg.Session
	if session == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session name is required"))
	}

	if !strings.HasPrefix(session, "gt-") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session must start with 'gt-'"))
	}

	input := req.Msg.Input
	if input == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input text is required"))
	}

	exists, err := s.tmux.HasSession(session)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check session existence: %w", err))
	}
	if !exists {
		return connect.NewResponse(&gastownv1.SendInputResponse{
			Delivered: false,
			Error:     fmt.Sprintf("session %q does not exist", session),
		}), nil
	}

	if err := s.tmux.SendKeys(session, input); err != nil {
		return connect.NewResponse(&gastownv1.SendInputResponse{
			Delivered: false,
			Error:     fmt.Sprintf("failed to send input: %v", err),
		}), nil
	}

	return connect.NewResponse(&gastownv1.SendInputResponse{
		Delivered: true,
	}), nil
}
