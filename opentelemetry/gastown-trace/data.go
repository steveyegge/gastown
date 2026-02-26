package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Session ───────────────────────────────────────────────────────────────────

type Session struct {
	ID              string
	Role            string
	Actor           string
	Topic           string
	Prompt          string
	StartedAt       time.Time
	StoppedAt       time.Time
	Running         bool
	ClaudeSessionIDs []string // correlated Claude session UUIDs
}

func (s Session) Duration() time.Duration {
	if s.Running {
		return time.Since(s.StartedAt)
	}
	return s.StoppedAt.Sub(s.StartedAt)
}

func loadSessions(cfg *Config, since time.Time) ([]Session, error) {
	starts, err := vlQuery(cfg.LogsURL, "session.start", 500, since, time.Time{})
	if err != nil {
		return nil, err
	}
	stops, _ := vlQuery(cfg.LogsURL, "session.stop", 500, since, time.Time{})

	stopTimes := map[string]time.Time{}
	for _, ev := range stops {
		id := ev.Str("session_id")
		if id != "" {
			stopTimes[id] = ev.Time()
		}
	}

	seen := map[string]bool{}
	var sessions []Session
	for _, ev := range starts {
		id := ev.Str("session_id")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		stream := ev.Str("_stream")
		role := coalesce(ev.Str("role"), ev.Str("gt.role"), extractStreamField(stream, "gt.role"))
		actor := coalesce(ev.Str("gt.actor"), extractStreamField(stream, "gt.actor"))
		topic := coalesce(ev.Str("gt.topic"), extractStreamField(stream, "gt.topic"))
		prompt := coalesce(ev.Str("gt.prompt"), extractStreamField(stream, "gt.prompt"))

		stopped, hasStopped := stopTimes[id]
		s := Session{
			ID:        id,
			Role:      role,
			Actor:     actor,
			Topic:     topic,
			Prompt:    prompt,
			StartedAt: ev.Time(),
			Running:   !hasStopped,
		}
		if hasStopped {
			s.StoppedAt = stopped
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.After(sessions[j].StartedAt)
	})
	return sessions, nil
}

// correlateClaudeSessions matches tmux session names to Claude session UUIDs
// by finding claude_code.api_request events that first appear within 30s of a session.start.
func correlateClaudeSessions(cfg *Config, sessions []Session, since time.Time) map[string][]string {
	reqs, err := vlQuery(cfg.LogsURL, "claude_code.api_request", 2000, since, time.Time{})
	if err != nil {
		return nil
	}

	// Find first occurrence of each Claude session.id
	claudeFirst := map[string]time.Time{}
	for _, r := range reqs {
		sid := r.Str("session.id")
		if sid == "" {
			continue
		}
		t := r.Time()
		if existing, ok := claudeFirst[sid]; !ok || t.Before(existing) {
			claudeFirst[sid] = t
		}
	}

	result := map[string][]string{} // tmuxID → []claudeSessionID
	for _, s := range sessions {
		for claudeID, firstSeen := range claudeFirst {
			delta := firstSeen.Sub(s.StartedAt)
			if delta >= -5*time.Second && delta <= 60*time.Second {
				result[s.ID] = append(result[s.ID], claudeID)
			}
		}
	}
	return result
}

// ── Tool calls ────────────────────────────────────────────────────────────────

type ToolCall struct {
	Time       time.Time
	SessionID  string
	ToolName   string
	Command    string
	DurationMs float64
	Success    bool
}

func loadToolCalls(cfg *Config, since time.Time) ([]ToolCall, error) {
	events, err := vlQuery(cfg.LogsURL, "claude_code.tool_result", 500, since, time.Time{})
	if err != nil {
		return nil, err
	}
	var calls []ToolCall
	for _, ev := range events {
		dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
		// Extract human-readable command from tool_parameters JSON
		cmd := extractToolCommand(ev.Str("tool_parameters"))
		calls = append(calls, ToolCall{
			Time:       ev.Time(),
			SessionID:  ev.Str("session.id"),
			ToolName:   ev.Str("tool_name"),
			Command:    cmd,
			DurationMs: dur,
			Success:    ev.Str("success") == "true",
		})
	}
	return calls, nil
}

func extractToolCommand(params string) string {
	var m map[string]string
	if json.Unmarshal([]byte(params), &m) != nil {
		return params
	}
	if cmd := m["full_command"]; cmd != "" {
		return cmd
	}
	if cmd := m["bash_command"]; cmd != "" {
		return cmd
	}
	if desc := m["description"]; desc != "" {
		return desc
	}
	return params
}

// ── API costs ─────────────────────────────────────────────────────────────────

type APIRequest struct {
	Time         time.Time
	SessionID    string
	Model        string
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CostUSD      float64
	DurationMs   float64
}

type CostSummary struct {
	TotalUSD     float64
	ByModel      map[string]float64
	TotalInput   int64
	TotalOutput  int64
	TotalCache   int64
	RequestCount int
}

func loadAPIRequests(cfg *Config, since time.Time) ([]APIRequest, error) {
	events, err := vlQuery(cfg.LogsURL, "claude_code.api_request", 2000, since, time.Time{})
	if err != nil {
		return nil, err
	}
	var reqs []APIRequest
	for _, ev := range events {
		cost, _ := strconv.ParseFloat(ev.Str("cost_usd"), 64)
		dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
		in, _ := strconv.ParseInt(ev.Str("input_tokens"), 10, 64)
		out, _ := strconv.ParseInt(ev.Str("output_tokens"), 10, 64)
		cr, _ := strconv.ParseInt(ev.Str("cache_read_tokens"), 10, 64)
		reqs = append(reqs, APIRequest{
			Time:         ev.Time(),
			SessionID:    ev.Str("session.id"),
			Model:        ev.Str("model"),
			InputTokens:  in,
			OutputTokens: out,
			CacheRead:    cr,
			CostUSD:      cost,
			DurationMs:   dur,
		})
	}
	return reqs, nil
}

func computeCosts(reqs []APIRequest) CostSummary {
	s := CostSummary{ByModel: map[string]float64{}}
	for _, r := range reqs {
		s.TotalUSD += r.CostUSD
		s.ByModel[r.Model] += r.CostUSD
		s.TotalInput += r.InputTokens
		s.TotalOutput += r.OutputTokens
		s.TotalCache += r.CacheRead
		s.RequestCount++
	}
	return s
}

// ── BD calls ──────────────────────────────────────────────────────────────────

type BDCall struct {
	Time       time.Time
	Subcommand string
	Args       string
	DurationMs float64
	Status     string
	Stdout     string
	Actor      string // from _stream gt.actor
	GTSession  string // from _stream gt.session
}

func loadBDCalls(cfg *Config, since time.Time, filter string) ([]BDCall, error) {
	q := "bd.call"
	if filter != "" {
		q = fmt.Sprintf("bd.call AND (%s)", filter)
	}
	events, err := vlQuery(cfg.LogsURL, q, 500, since, time.Time{})
	if err != nil {
		return nil, err
	}
	var calls []BDCall
	for _, ev := range events {
		dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
		stream := ev.Str("_stream")
		calls = append(calls, BDCall{
			Time:       ev.Time(),
			Subcommand: ev.Str("subcommand"),
			Args:       ev.Str("args"),
			DurationMs: dur,
			Status:     ev.Str("status"),
			Stdout:     ev.Str("stdout"),
			Actor:      extractStreamField(stream, "gt.actor"),
			GTSession:  extractStreamField(stream, "gt.session"),
		})
	}
	return calls, nil
}

// ── Bead lifecycle ────────────────────────────────────────────────────────────

type BeadLifecycle struct {
	ID          string
	Title       string
	Type        string
	CreatedAt   time.Time
	InProgAt    time.Time
	DoneAt      time.Time
	CreatedBy   string
	Assignee    string
	HookBead    string
	TimeToStart time.Duration // created → in_progress
	TotalTime   time.Duration // created → done
}

func (b BeadLifecycle) State() string {
	if !b.DoneAt.IsZero() {
		return "done"
	}
	if !b.InProgAt.IsZero() {
		return "in_progress"
	}
	return "open"
}

func loadBeadLifecycles(cfg *Config, since time.Time) ([]BeadLifecycle, error) {
	creates, err := vlQuery(cfg.LogsURL, `bd.call AND subcommand:"create"`, 500, since, time.Time{})
	if err != nil {
		return nil, err
	}
	updates, _ := vlQuery(cfg.LogsURL, `bd.call AND subcommand:"update"`, 1000, since, time.Time{})

	// Map bead ID → lifecycle
	beads := map[string]*BeadLifecycle{}

	for _, ev := range creates {
		args := ev.Str("args")
		stdout := ev.Str("stdout")
		if stdout == "" {
			continue
		}

		// Parse bead ID from --id=X or from stdout JSON
		id := extractArg(args, "--id")
		if id == "" {
			var m map[string]string
			if json.Unmarshal([]byte(stdout), &m) == nil {
				id = m["id"]
			}
		}
		if id == "" {
			continue
		}

		title := extractArg(args, "--title")
		beadType := extractArg(args, "--type")
		createdBy := extractArg(args, "--actor")
		if createdBy == "" {
			stream := ev.Str("_stream")
			createdBy = extractStreamField(stream, "gt.actor")
		}

		// Extract hook_bead from description
		hookBead := ""
		desc := extractArg(args, "--description")
		if idx := strings.Index(desc, "hook_bead: "); idx >= 0 {
			rest := desc[idx+11:]
			end := strings.IndexAny(rest, "\n\r")
			if end < 0 {
				end = len(rest)
			}
			hookBead = strings.TrimSpace(rest[:end])
			if hookBead == "null" {
				hookBead = ""
			}
		}

		beads[id] = &BeadLifecycle{
			ID:        id,
			Title:     title,
			Type:      beadType,
			CreatedAt: ev.Time(),
			CreatedBy: createdBy,
			HookBead:  hookBead,
		}
	}

	// Apply updates: in_progress, done, closed, hooked
	for _, ev := range updates {
		args := ev.Str("args")
		// First positional arg after "update" subcommand is the bead ID
		fields := strings.Fields(args)
		if len(fields) < 1 {
			continue
		}
		id := fields[0]
		b, ok := beads[id]
		if !ok {
			continue
		}
		status := extractArg(args, "--status")
		assignee := extractArg(args, "--assignee")
		if assignee != "" {
			b.Assignee = assignee
		}
		t := ev.Time()
		switch status {
		case "in_progress":
			if b.InProgAt.IsZero() {
				b.InProgAt = t
				b.TimeToStart = t.Sub(b.CreatedAt)
			}
		case "done", "closed":
			if b.DoneAt.IsZero() {
				b.DoneAt = t
				b.TotalTime = t.Sub(b.CreatedAt)
			}
		}
	}

	var result []BeadLifecycle
	for _, b := range beads {
		result = append(result, *b)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// ── Delegation chain ──────────────────────────────────────────────────────────

type DelegationNode struct {
	BeadID    string
	Title     string
	CreatedBy string
	Assignee  string
	CreatedAt time.Time
	DoneAt    time.Time
	Children  []*DelegationNode
}

// buildDelegationTree reconstructs the mayor→deacon→agent delegation graph
// using bead lifecycle data (hook_bead links child → parent).
func buildDelegationTree(beads []BeadLifecycle) []*DelegationNode {
	nodes := map[string]*DelegationNode{}
	for i := range beads {
		b := &beads[i]
		nodes[b.ID] = &DelegationNode{
			BeadID:    b.ID,
			Title:     b.Title,
			CreatedBy: b.CreatedBy,
			Assignee:  b.Assignee,
			CreatedAt: b.CreatedAt,
			DoneAt:    b.DoneAt,
		}
	}

	// Link children to parents via hook_bead
	var roots []*DelegationNode
	for i := range beads {
		b := &beads[i]
		node := nodes[b.ID]
		if b.HookBead != "" {
			if parent, ok := nodes[b.HookBead]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				roots = append(roots, node)
			}
		} else {
			roots = append(roots, node)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].CreatedAt.After(roots[j].CreatedAt)
	})
	return roots
}

// ── Pane output ───────────────────────────────────────────────────────────────

type PaneLine struct {
	Time    time.Time
	Session string
	Content string
}

func loadPaneOutput(cfg *Config, session string, since time.Time, limit int) ([]PaneLine, error) {
	q := "pane.output"
	if session != "" {
		q = fmt.Sprintf(`pane.output AND session:"%s"`, session)
	}
	events, err := vlQuery(cfg.LogsURL, q, limit, since, time.Time{})
	if err != nil {
		return nil, err
	}
	var lines []PaneLine
	for _, ev := range events {
		lines = append(lines, PaneLine{
			Time:    ev.Time(),
			Session: ev.Str("session"),
			Content: ev.Str("content"),
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Time.Before(lines[j].Time)
	})
	return lines, nil
}

// ── Bead full detail ──────────────────────────────────────────────────────────

// BeadTransition is a single bd update event for a bead.
type BeadTransition struct {
	Time     time.Time
	Status   string
	Assignee string
	Actor    string
	Elapsed  time.Duration // from bead creation
	Args     string
}

// BeadFull is a completely loaded bead: content, all transitions, pane output.
type BeadFull struct {
	ID          string
	Title       string
	Description string
	Type        string
	CreatedAt   time.Time
	InProgAt    time.Time
	DoneAt      time.Time
	CreatedBy   string
	Assignee    string
	HookBead    string
	TimeToStart time.Duration
	TotalTime   time.Duration
	Transitions []BeadTransition
	PaneChunks  []PaneLine
	CreateArgs  string // raw create args
}

func (b *BeadFull) State() string {
	if !b.DoneAt.IsZero() {
		return "done"
	}
	if !b.InProgAt.IsZero() {
		return "in_progress"
	}
	return "open"
}

func loadBeadFull(cfg *Config, beadID string) (*BeadFull, error) {
	since := time.Now().Add(-7 * 24 * time.Hour)
	q := fmt.Sprintf(`bd.call AND args:"%s"`, beadID)
	evs, err := vlQuery(cfg.LogsURL, q, 500, since, time.Time{})
	if err != nil {
		return nil, err
	}

	bf := &BeadFull{ID: beadID}

	for _, ev := range evs {
		sub := ev.Str("subcommand")
		args := ev.Str("args")
		stream := ev.Str("_stream")
		actor := coalesce(extractStreamField(stream, "gt.actor"), extractStreamField(stream, "gt.session"))

		if sub == "create" {
			id := coalesce(extractArgQ(args, "--id"), extractArg(args, "--id"))
			if id != beadID {
				continue
			}
			bf.Title = coalesce(extractArgQ(args, "--title"), extractArg(args, "--title"))
			bf.Type = coalesce(extractArgQ(args, "--type"), extractArg(args, "--type"))
			bf.Description = coalesce(extractArgQ(args, "--description"), extractArg(args, "--description"))
			bf.CreatedAt = ev.Time()
			bf.CreatedBy = coalesce(extractArgQ(args, "--actor"), extractArg(args, "--actor"), actor)
			bf.CreateArgs = args
			// hook_bead in description
			if idx := strings.Index(bf.Description, "hook_bead: "); idx >= 0 {
				rest := bf.Description[idx+11:]
				end := strings.IndexAny(rest, "\n\r")
				if end < 0 {
					end = len(rest)
				}
				hb := strings.TrimSpace(rest[:end])
				if hb != "null" {
					bf.HookBead = hb
				}
			}
		} else if sub == "update" {
			fields := strings.Fields(args)
			if len(fields) == 0 || fields[0] != beadID {
				continue
			}
			status := extractArg(args, "--status")
			assignee := extractArg(args, "--assignee")
			if assignee != "" {
				bf.Assignee = assignee
			}
			t := ev.Time()
			switch status {
			case "in_progress":
				if bf.InProgAt.IsZero() {
					bf.InProgAt = t
					if !bf.CreatedAt.IsZero() {
						bf.TimeToStart = t.Sub(bf.CreatedAt)
					}
				}
			case "done", "closed":
				if bf.DoneAt.IsZero() {
					bf.DoneAt = t
					if !bf.CreatedAt.IsZero() {
						bf.TotalTime = t.Sub(bf.CreatedAt)
					}
				}
			}
			tr := BeadTransition{
				Time:     t,
				Status:   status,
				Assignee: assignee,
				Actor:    actor,
				Args:     args,
			}
			if !bf.CreatedAt.IsZero() {
				tr.Elapsed = t.Sub(bf.CreatedAt)
			}
			bf.Transitions = append(bf.Transitions, tr)
		}
	}

	sort.Slice(bf.Transitions, func(i, j int) bool {
		return bf.Transitions[i].Time.Before(bf.Transitions[j].Time)
	})

	// Load pane output from assignee session during bead lifetime
	if bf.Assignee != "" && !bf.CreatedAt.IsZero() {
		paneEnd := bf.DoneAt
		if paneEnd.IsZero() {
			paneEnd = time.Now()
		}
		paneQ := fmt.Sprintf(`pane.output AND session:"%s"`, bf.Assignee)
		pEvs, _ := vlQuery(cfg.LogsURL, paneQ, 200, bf.CreatedAt.Add(-30*time.Second), paneEnd.Add(2*time.Minute))
		for _, pev := range pEvs {
			bf.PaneChunks = append(bf.PaneChunks, PaneLine{
				Time:    pev.Time(),
				Session: pev.Str("session"),
				Content: pev.Str("content"),
			})
		}
		sort.Slice(bf.PaneChunks, func(i, j int) bool {
			return bf.PaneChunks[i].Time.Before(bf.PaneChunks[j].Time)
		})
	}

	return bf, nil
}

// ── Flow events ───────────────────────────────────────────────────────────────

// FlowEvent is a unified event for the /flow timeline view.
type FlowEvent struct {
	Time       time.Time
	Elapsed    time.Duration // from first event in result set
	Step       time.Duration // from previous event
	Kind       string        // bd_create, bd_update, sess_start, sess_stop, api_req, agent_*
	Role       string
	Actor      string
	BeadID     string
	Summary    string
	Detail     string
	ShowInline bool    // show Detail inline (no <details> click required)
	DurMs      float64
	CostUSD    float64
}

func (f FlowEvent) KindLabel() string {
	switch f.Kind {
	case "bd_create":
		return "bid.create"
	case "bd_update":
		return "bid.update"
	case "sess_start":
		return "session.start"
	case "sess_stop":
		return "session.stop"
	case "api_req":
		return "nlm.call"
	case "agent_text":
		return "text"
	case "agent_tool_use":
		return "tool_use"
	case "agent_tool_result":
		return "tool_result"
	case "agent_thinking":
		return "thinking"
	}
	return f.Kind
}

func (f FlowEvent) KindCSS() string {
	switch f.Kind {
	case "bd_create":
		return "kind-create"
	case "bd_update":
		return "kind-update"
	case "sess_start", "sess_stop":
		return "kind-session"
	case "api_req":
		return "kind-api"
	case "agent_text":
		return "kind-agent-text"
	case "agent_tool_use":
		return "kind-agent-tool"
	case "agent_tool_result":
		return "kind-agent-result"
	case "agent_thinking":
		return "kind-agent-think"
	}
	return ""
}

func loadFlowEvents(cfg *Config, since time.Time, roleFilter, kindFilter, beadFilter string) ([]FlowEvent, error) {
	var events []FlowEvent

	// BD calls
	if kindFilter == "" || kindFilter == "bd" {
		q := "bd.call"
		if beadFilter != "" {
			q = fmt.Sprintf(`bd.call AND args:"%s"`, beadFilter)
		}
		bdEvs, _ := vlQuery(cfg.LogsURL, q, 500, since, time.Time{})
		for _, ev := range bdEvs {
			sub := ev.Str("subcommand")
			args := ev.Str("args")
			stream := ev.Str("_stream")
			role := extractStreamField(stream, "gt.role")
			actor := coalesce(extractStreamField(stream, "gt.actor"), extractStreamField(stream, "gt.session"))
			if roleFilter != "" && role != roleFilter {
				continue
			}
			beadID := ""
			summary := ""
			if sub == "create" {
				beadID = coalesce(extractArgQ(args, "--id"), extractArg(args, "--id"))
				title := coalesce(extractArgQ(args, "--title"), extractArg(args, "--title"))
				bType := coalesce(extractArgQ(args, "--type"), extractArg(args, "--type"))
				summary = fmt.Sprintf("[%s] %s", bType, title)
			} else if sub == "update" {
				fields := strings.Fields(args)
				if len(fields) > 0 {
					beadID = fields[0]
				}
				status := extractArg(args, "--status")
				assignee := extractArg(args, "--assignee")
				summary = "→ " + status
				if assignee != "" {
					summary += " → " + assignee
				}
			} else {
				summary = sub
			}
			if beadFilter != "" && beadID != beadFilter {
				continue
			}
			dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
			events = append(events, FlowEvent{
				Time:    ev.Time(),
				Kind:    "bd_" + sub,
				Role:    role,
				Actor:   actor,
				BeadID:  beadID,
				Summary: summary,
				Detail:  args,
				DurMs:   dur,
			})
		}
	}

	// Sessions
	if kindFilter == "" || kindFilter == "session" {
		for _, sub := range []string{"start", "stop"} {
			sessEvs, _ := vlQuery(cfg.LogsURL, "session."+sub, 500, since, time.Time{})
			for _, ev := range sessEvs {
				stream := ev.Str("_stream")
				role := coalesce(ev.Str("role"), ev.Str("gt.role"), extractStreamField(stream, "gt.role"))
				actor := coalesce(ev.Str("gt.actor"), extractStreamField(stream, "gt.actor"))
				if roleFilter != "" && role != roleFilter {
					continue
				}
				topic := coalesce(ev.Str("gt.topic"), extractStreamField(stream, "gt.topic"))
				prompt := coalesce(ev.Str("gt.prompt"), extractStreamField(stream, "gt.prompt"))
				summary := role
				if topic != "" {
					summary = topic
				}
				events = append(events, FlowEvent{
					Time:       ev.Time(),
					Kind:       "sess_" + sub,
					Role:       role,
					Actor:      actor,
					Summary:    summary,
					Detail:     prompt,
					ShowInline: sub == "start" && prompt != "",
				})
			}
		}
	}

	// NLM / API requests
	if kindFilter == "" || kindFilter == "api" {
		apiEvs, _ := vlQuery(cfg.LogsURL, "claude_code.api_request", 1000, since, time.Time{})
		for _, ev := range apiEvs {
			in, _ := strconv.ParseInt(ev.Str("input_tokens"), 10, 64)
			out, _ := strconv.ParseInt(ev.Str("output_tokens"), 10, 64)
			cr, _ := strconv.ParseInt(ev.Str("cache_read_tokens"), 10, 64)
			cost, _ := strconv.ParseFloat(ev.Str("cost_usd"), 64)
			dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
			model := ev.Str("model")
			if idx := strings.LastIndex(model, "-202"); idx > 0 {
				model = model[:idx]
			}
			sessID := ev.Str("session.id")
			summary := fmt.Sprintf("↑%dk ↓%dk cache:%dk  %s",
				(in+500)/1000, (out+500)/1000, (cr+500)/1000, model)
			events = append(events, FlowEvent{
				Time:    ev.Time(),
				Kind:    "api_req",
				Actor:   sessID,
				Summary: summary,
				DurMs:   dur,
				CostUSD: cost,
			})
		}
	}

	// Enrich NLM events with real pane content (fallback when agent events unavailable)
	if kindFilter == "" || kindFilter == "api" {
		enrichNLMWithPane(cfg, since, events)
	}

	// Enrich NLM events with structured agent.event data (preferred over pane output).
	// Overwrites pane-enriched Detail when agent events exist for the same window.
	// Also adds api_req events that were missed by the limit=1000 query above.
	if kindFilter == "" || kindFilter == "api" {
		events = enrichNLMWithAgentEvents(cfg, since, events)
	}

	// Agent conversation events (from gt agent-log JSONL watcher)
	if kindFilter == "" || strings.HasPrefix(kindFilter, "agent") {
		agentQ := "agent.event"
		agentEvs, _ := vlQuery(cfg.LogsURL, agentQ, 2000, since, time.Time{})
		for _, ev := range agentEvs {
			session := ev.Str("session")
			eventType := ev.Str("event_type")
			content := ev.Str("content")
			if content == "" {
				continue
			}
			// Apply kind sub-filter (e.g. kindFilter="agent_tool_use")
			kind := "agent_" + eventType
			if kindFilter != "" && kindFilter != "agent" && kindFilter != kind {
				continue
			}
			// Role filter: derive role from session name heuristic
			role := roleFromSession(session)
			if roleFilter != "" && role != roleFilter {
				continue
			}
			events = append(events, FlowEvent{
				Time:       ev.Time(),
				Kind:       kind,
				Role:       role,
				Actor:      session,
				Summary:    content,
				Detail:     content,
				ShowInline: true,
			})
		}
	}

	// Sort chronologically and compute timing
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	var t0 time.Time
	for i := range events {
		if t0.IsZero() {
			t0 = events[i].Time
		}
		events[i].Elapsed = events[i].Time.Sub(t0)
		if i > 0 {
			events[i].Step = events[i].Time.Sub(events[i-1].Time)
		}
	}

	return events, nil
}

// enrichNLMWithPane fills FlowEvent.Detail for api_req events with real pane content
// captured from the correlated tmux session during the API call time window.
func enrichNLMWithPane(cfg *Config, since time.Time, events []FlowEvent) {
	paneEvs, err := vlQuery(cfg.LogsURL, "pane.output", 2000, since, time.Time{})
	if err != nil || len(paneEvs) == 0 {
		return
	}

	// Index pane chunks by tmux session name.
	paneBySession := map[string][]PaneLine{}
	for _, ev := range paneEvs {
		sess := ev.Str("session")
		paneBySession[sess] = append(paneBySession[sess], PaneLine{
			Time:    ev.Time(),
			Session: sess,
			Content: ev.Str("content"),
		})
	}
	for sess := range paneBySession {
		sort.Slice(paneBySession[sess], func(i, j int) bool {
			return paneBySession[sess][i].Time.Before(paneBySession[sess][j].Time)
		})
	}

	// Invert correlateClaudeSessions: Claude UUID → tmux session name.
	sessions, err := loadSessions(cfg, since)
	if err == nil {
		corrMap := correlateClaudeSessions(cfg, sessions, since)
		claudeToTmux := map[string]string{}
		for tmuxID, claudeIDs := range corrMap {
			for _, cid := range claudeIDs {
				if _, exists := claudeToTmux[cid]; !exists {
					claudeToTmux[cid] = tmuxID
				}
			}
		}

		// Flatten all pane lines for use when no correlation is found.
		var allPane []PaneLine
		for _, lines := range paneBySession {
			allPane = append(allPane, lines...)
		}
		sort.Slice(allPane, func(i, j int) bool {
			return allPane[i].Time.Before(allPane[j].Time)
		})

		for i := range events {
			ev := &events[i]
			if ev.Kind != "api_req" {
				continue
			}
			callTime := ev.Time
			windowStart := callTime.Add(-10 * time.Second)
			dur := time.Duration(ev.DurMs * float64(time.Millisecond))
			if dur < time.Second {
				dur = time.Second
			}
			windowEnd := callTime.Add(dur + 30*time.Second)

			tmuxSess := claudeToTmux[ev.Actor]
			source := allPane
			if tmuxSess != "" {
				source = paneBySession[tmuxSess]
			}

			var chunks []string
			for _, pl := range source {
				if pl.Time.Before(windowStart) || pl.Time.After(windowEnd) {
					continue
				}
				if content := strings.TrimSpace(pl.Content); content != "" {
					chunks = append(chunks, fmt.Sprintf("[%s %s]\n%s",
						pl.Time.Local().Format("15:04:05"), pl.Session, content))
				}
			}
			if len(chunks) > 0 {
				ev.Detail = strings.Join(chunks, "\n\n")
				ev.ShowInline = true
			}
		}
	}
}

// enrichNLMWithAgentEvents fills api_req FlowEvent.Detail using structured
// agent.event logs (preferred over pane output). Correlates by native_session_id
// (Claude Code JSONL UUID stored in the agent event) which matches api_req.session.id
// exactly — no time-window guessing needed.
// enrichNLMWithAgentEvents fills api_req FlowEvent.Detail using structured
// agent.event records. It correlates by native_session_id (Claude Code JSONL UUID)
// which matches api_req.session.id exactly. When the main api_req load (limit=1000)
// missed events for watched sessions, this function fetches them directly.
func enrichNLMWithAgentEvents(cfg *Config, since time.Time, events []FlowEvent) []FlowEvent {
	agentEvs, err := vlQuery(cfg.LogsURL, "agent.event", 50000, since, time.Time{})
	if err != nil || len(agentEvs) == 0 {
		return events
	}

	// Group agent events by native_session_id (Claude Code UUID).
	type agentEv struct {
		Time      time.Time
		EventType string
		Role      string
		Content   string
	}
	byNativeID := map[string][]agentEv{}
	for _, ev := range agentEvs {
		nid := ev.Str("native_session_id")
		if nid == "" {
			continue
		}
		byNativeID[nid] = append(byNativeID[nid], agentEv{
			Time:      ev.Time(),
			EventType: ev.Str("event_type"),
			Role:      ev.Str("role"),
			Content:   ev.Str("content"),
		})
	}
	for nid := range byNativeID {
		sort.Slice(byNativeID[nid], func(i, j int) bool {
			return byNativeID[nid][i].Time.Before(byNativeID[nid][j].Time)
		})
	}

	// Find which native_session_ids already have api_req events loaded.
	coveredIDs := map[string]bool{}
	for i := range events {
		if events[i].Kind == "api_req" {
			coveredIDs[events[i].Actor] = true
		}
	}

	// For sessions with agent events but no api_req loaded, fetch them directly.
	for nid := range byNativeID {
		if coveredIDs[nid] {
			continue
		}
		q := fmt.Sprintf(`claude_code.api_request AND "session.id":%q`, nid)
		apiEvs, err := vlQuery(cfg.LogsURL, q, 500, since, time.Time{})
		if err != nil {
			continue
		}
		for _, ev := range apiEvs {
			in, _ := strconv.ParseInt(ev.Str("input_tokens"), 10, 64)
			out, _ := strconv.ParseInt(ev.Str("output_tokens"), 10, 64)
			cr, _ := strconv.ParseInt(ev.Str("cache_read_tokens"), 10, 64)
			cost, _ := strconv.ParseFloat(ev.Str("cost_usd"), 64)
			dur, _ := strconv.ParseFloat(ev.Str("duration_ms"), 64)
			model := ev.Str("model")
			if idx := strings.LastIndex(model, "-202"); idx > 0 {
				model = model[:idx]
			}
			summary := fmt.Sprintf("↑%dk ↓%dk cache:%dk  %s",
				(in+500)/1000, (out+500)/1000, (cr+500)/1000, model)
			events = append(events, FlowEvent{
				Time:    ev.Time(),
				Kind:    "api_req",
				Actor:   nid,
				Summary: summary,
				DurMs:   dur,
				CostUSD: cost,
			})
		}
	}

	sep := "\n\n" + strings.Repeat("─", 40) + "\n\n"

	for i := range events {
		ev := &events[i]
		if ev.Kind != "api_req" {
			continue
		}
		// ev.Actor = Claude Code session UUID (from session.id field)
		aevs, ok := byNativeID[ev.Actor]
		if !ok || len(aevs) == 0 {
			continue
		}

		var parts []string
		for _, ae := range aevs {
			if ae.EventType == "thinking" {
				continue
			}
			header := ""
			switch ae.EventType {
			case "text":
				header = "◀ response"
			case "tool_use":
				header = "🔧 tool_use"
			case "tool_result":
				header = "↩ tool_result"
			default:
				continue
			}
			content := ae.Content
			if len(content) > 1200 {
				content = content[:1200] + "…"
			}
			parts = append(parts, header+"\n"+content)
		}
		if len(parts) > 0 {
			ev.Detail = strings.Join(parts, sep)
			ev.ShowInline = true
		}
	}
	return events
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// roleFromSession derives a Gas Town role from a tmux session name.
// Conventions: "hq-mayor" → "mayor", "fai-witness" → "witness",
// "gt-wyvern-toast" → "polecat", etc.
func roleFromSession(session string) string {
	known := []string{"mayor", "deacon", "witness", "refinery", "dog", "boot", "crew"}
	lower := strings.ToLower(session)
	for _, role := range known {
		if strings.HasSuffix(lower, "-"+role) || strings.Contains(lower, "-"+role+"-") {
			return role
		}
	}
	// gt-<rig>-<name> pattern → polecat
	if strings.HasPrefix(lower, "gt-") {
		return "polecat"
	}
	return ""
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fmtDur(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
