package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
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
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"

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
	// TODO: Implement agent-specific lookup
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GetAgentStatus not yet implemented"))
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
}

var _ gastownv1connect.DecisionServiceHandler = (*DecisionServer)(nil)

func NewDecisionServer(townRoot string, bus *eventbus.Bus) *DecisionServer {
	return &DecisionServer{townRoot: townRoot, bus: bus}
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
		Resolved:    fields.ChosenIndex > 0,
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
		Question:    req.Msg.Question,
		Context:     req.Msg.Context,
		Options:     options,
		RequestedBy: formatAgentAddress(req.Msg.RequestedBy),
		RequestedAt: time.Now().Format(time.RFC3339),
		Urgency:     fromUrgency(req.Msg.Urgency),
		Blockers:    req.Msg.Blockers,
	}

	// Create the decision bead
	issue, err := client.CreateDecisionBead(req.Msg.Question, fields)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("creating decision: %w", err))
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
		Id:          issue.ID,
		Question:    fields.Question,
		Context:     fields.Context,
		Options:     protoOptions,
		RequestedBy: req.Msg.RequestedBy,
		Urgency:     req.Msg.Urgency,
		Blockers:    fields.Blockers,
		Resolved:    false,
	}

	// Publish event to bus for real-time notification
	if s.bus != nil {
		s.bus.PublishDecisionCreated(issue.ID, decision)
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

	// Resolve the decision
	if err := client.ResolveDecision(
		req.Msg.DecisionId,
		int(req.Msg.ChosenIndex),
		req.Msg.Rationale,
		resolvedBy,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolving decision: %w", err))
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

func (s *DecisionServer) WatchDecisions(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchDecisionsRequest],
	stream *connect.ServerStream[gastownv1.Decision],
) error {
	seen := make(map[string]bool)

	// First, send all existing pending decisions
	townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
	client := beads.New(townBeadsPath)
	issues, err := client.ListDecisions()
	if err == nil {
		for _, issue := range issues {
			seen[issue.ID] = true
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
				seen[event.DecisionID] = true

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
					seen[issue.ID] = true

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
				seen[issue.ID] = true

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
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("ReadMessage not yet implemented"))
}

func (s *MailServer) SendMessage(
	ctx context.Context,
	req *connect.Request[gastownv1.SendMessageRequest],
) (*connect.Response[gastownv1.SendMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("SendMessage not yet implemented"))
}

func (s *MailServer) MarkRead(
	ctx context.Context,
	req *connect.Request[gastownv1.MarkReadRequest],
) (*connect.Response[gastownv1.MarkReadResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("MarkRead not yet implemented"))
}

func (s *MailServer) DeleteMessage(
	ctx context.Context,
	req *connect.Request[gastownv1.DeleteMessageRequest],
) (*connect.Response[gastownv1.DeleteMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("DeleteMessage not yet implemented"))
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

		// Stream events
		ctx := r.Context()
		ticker := time.NewTicker(30 * time.Second) // Keepalive
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				// Send event based on type
				eventType := "unknown"
				switch event.Type {
				case eventbus.EventDecisionCreated:
					eventType = "created"
				case eventbus.EventDecisionResolved:
					eventType = "resolved"
				case eventbus.EventDecisionCanceled:
					eventType = "canceled"
				}
				fmt.Fprintf(w, "event: decision\n")
				fmt.Fprintf(w, "data: {\"id\":\"%s\",\"type\":\"%s\"}\n\n", event.DecisionID, eventType)
				flusher.Flush()

			case <-ticker.C:
				// Keepalive comment
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

// escapeJSON escapes a string for JSON output.
func escapeJSON(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '"':
			result += `\"`
		case '\\':
			result += `\\`
		case '\n':
			result += `\n`
		case '\r':
			result += `\r`
		case '\t':
			result += `\t`
		default:
			result += string(c)
		}
	}
	return result
}
