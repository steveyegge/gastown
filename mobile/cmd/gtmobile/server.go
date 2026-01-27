package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"connectrpc.com/connect"

	gastownv1 "github.com/steveyegge/gastown/mobile/gen/gastown/v1"
	"github.com/steveyegge/gastown/mobile/gen/gastown/v1/gastownv1connect"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
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
}

var _ gastownv1connect.DecisionServiceHandler = (*DecisionServer)(nil)

func NewDecisionServer(townRoot string) *DecisionServer {
	return &DecisionServer{townRoot: townRoot}
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
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GetDecision not yet implemented"))
}

func (s *DecisionServer) Resolve(
	ctx context.Context,
	req *connect.Request[gastownv1.ResolveRequest],
) (*connect.Response[gastownv1.ResolveResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("Resolve not yet implemented"))
}

func (s *DecisionServer) Cancel(
	ctx context.Context,
	req *connect.Request[gastownv1.CancelRequest],
) (*connect.Response[gastownv1.CancelResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("Cancel not yet implemented"))
}

func (s *DecisionServer) WatchDecisions(
	ctx context.Context,
	req *connect.Request[gastownv1.WatchDecisionsRequest],
	stream *connect.ServerStream[gastownv1.Decision],
) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	seen := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
			client := beads.New(townBeadsPath)
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
