// waterfall_v2.go — Gastown Waterfall V2
// Chrome DevTools-style agent orchestration view.
// Handlers: GET /waterfall (HTML), GET /api/waterfall.json (JSON)
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Data model ────────────────────────────────────────────────────────────────

// WV2Attrs holds OTel attributes on an event.
type WV2Attrs map[string]string

// WV2Event is a single OTel log record.
type WV2Event struct {
	ID        string   `json:"id"`
	RunID     string   `json:"run_id"`
	Body      string   `json:"body"`
	Timestamp string   `json:"timestamp"`
	Severity  string   `json:"severity"`
	Attrs     WV2Attrs `json:"attrs"`
}

// WV2Run is a single agent run (one agent.instantiate lifecycle).
type WV2Run struct {
	RunID      string     `json:"run_id"`
	Instance   string     `json:"instance"`
	TownRoot   string     `json:"town_root"`
	AgentType  string     `json:"agent_type"`
	Role       string     `json:"role"`
	AgentName  string     `json:"agent_name"`
	SessionID  string     `json:"session_id"`
	Rig        string     `json:"rig"`
	StartedAt  string     `json:"started_at"`
	EndedAt    string     `json:"ended_at,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	Running    bool       `json:"running"`
	Cost       float64    `json:"cost"`
	Events     []WV2Event `json:"events"`
}

// WV2Comm is an inter-agent communication event.
type WV2Comm struct {
	Time    string `json:"time"`
	Type    string `json:"type"`
	From    string `json:"from"`
	To      string `json:"to"`
	BeadID  string `json:"bead_id,omitempty"`
	Label   string `json:"label"`
	Subject string `json:"subject,omitempty"`
	MsgBody string `json:"body,omitempty"`
}

// WV2Bead is a summarized bead from bd lifecycle.
type WV2Bead struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	BeadType  string `json:"type"`
	State     string `json:"state"`
	CreatedBy string `json:"created_by"`
	Assignee  string `json:"assignee"`
	CreatedAt string `json:"created_at"`
	DoneAt    string `json:"done_at,omitempty"`
}

// WV2Rig groups runs by rig.
type WV2Rig struct {
	Name      string   `json:"name"`
	Collapsed bool     `json:"collapsed"`
	Runs      []WV2Run `json:"runs"`
}

// WV2Summary holds aggregate metrics.
type WV2Summary struct {
	RunCount      int     `json:"run_count"`
	RigCount      int     `json:"rig_count"`
	EventCount    int     `json:"event_count"`
	BeadCount     int     `json:"bead_count"`
	TotalCost     float64 `json:"total_cost"`
	TotalDuration string  `json:"total_duration"`
}

// WV2Window is the queried time range.
type WV2Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// WV2Data is the root JSON payload for /api/waterfall.json.
type WV2Data struct {
	Instance string     `json:"instance"`
	TownRoot string     `json:"town_root"`
	Window   WV2Window  `json:"window"`
	Summary  WV2Summary `json:"summary"`
	Rigs     []WV2Rig   `json:"rigs"`
	Comms    []WV2Comm  `json:"communications"`
	Beads    []WV2Bead  `json:"beads"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// toWV2Event converts a VictoriaLogs LogEvent to a WV2Event.
func toWV2Event(ev LogEvent) WV2Event {
	attrs := WV2Attrs{}
	for k, v := range ev {
		if k != "_time" && k != "_msg" && k != "_stream" {
			attrs[k] = v
		}
	}
	// Also fold _stream fields for legacy events.
	if stream := ev.Str("_stream"); stream != "" {
		for _, f := range []string{"gt.role", "gt.rig", "gt.session", "gt.actor", "gt.agent", "gt.town"} {
			if v := extractStreamField(stream, f); v != "" {
				if _, ok := attrs[f]; !ok {
					attrs[f] = v
				}
			}
		}
	}
	sev := "info"
	if attrs["status"] == "error" {
		sev = "error"
	}
	return WV2Event{
		ID:        ev.Str("_time"),
		RunID:     attrs["run.id"],
		Body:      ev.Msg(),
		Timestamp: ev.Time().UTC().Format(time.RFC3339),
		Severity:  sev,
		Attrs:     attrs,
	}
}

// findRun looks up the run for an event, trying run.id first, then session fallback.
func findRun(runMap map[string]*WV2Run, sessToRun map[string]string, ev LogEvent) *WV2Run {
	if id := ev.Str("run.id"); id != "" {
		if run, ok := runMap[id]; ok {
			return run
		}
	}
	for _, key := range []string{"session_id", "session"} {
		if sid := ev.Str(key); sid != "" {
			if runID := sessToRun[sid]; runID != "" {
				return runMap[runID]
			}
		}
	}
	if stream := ev.Str("_stream"); stream != "" {
		if sid := extractStreamField(stream, "gt.session"); sid != "" {
			if runID := sessToRun[sid]; runID != "" {
				return runMap[runID]
			}
		}
	}
	return nil
}

// buildComm constructs a WV2Comm from a communication event.
func buildComm(ev LogEvent, fromRunID string) WV2Comm {
	c := WV2Comm{
		Time: ev.Time().UTC().Format(time.RFC3339),
		From: fromRunID,
	}
	switch ev.Msg() {
	case "sling":
		c.Type = "sling"
		c.BeadID = ev.Str("bead")
		c.To = ev.Str("target")
		c.Label = fmt.Sprintf("sling %s", ev.Str("bead"))
	case "mail":
		c.Type = "mail"
		c.To = ev.Str("msg.to")
		c.Subject = ev.Str("msg.subject")
		c.MsgBody = ev.Str("msg.body")
		if from := ev.Str("msg.from"); from != "" {
			c.From = from
		}
		if c.Subject != "" {
			c.Label = "mail: " + c.Subject
		} else {
			c.Label = "mail → " + c.To
		}
	case "nudge":
		c.Type = "nudge"
		c.To = ev.Str("target")
		c.Label = "nudge → " + c.To
	case "polecat.spawn":
		c.Type = "spawn"
		c.To = ev.Str("name")
		c.Label = "spawn " + c.To
	case "done":
		c.Type = "done"
		c.Label = "done: " + ev.Str("exit_type")
	}
	return c
}

// roleFromSessionName derives role, rig and agentName from a tmux session name.
// Patterns: "hq-mayor" → (mayor, hq, mayor), "ali-witness" → (witness, ali, witness),
// "ali-furiosa" → (polecat, ali, furiosa).
func roleFromSessionName(session string) (role, rig, agentName string) {
	knownRoles := map[string]bool{
		"mayor": true, "deacon": true, "witness": true, "refinery": true,
		"polecat": true, "dog": true, "boot": true, "crew": true,
	}
	parts := strings.Split(session, "-")
	if len(parts) == 1 {
		return session, "", session
	}
	last := parts[len(parts)-1]
	if knownRoles[last] {
		return last, strings.Join(parts[:len(parts)-1], "-"), last
	}
	// Not a known role suffix → polecat with agent name.
	return "polecat", parts[0], strings.Join(parts[1:], "-")
}

// wv2FmtDur formats a duration for display.
func wv2FmtDur(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// ── Data loading ──────────────────────────────────────────────────────────────

func loadWaterfallV2(cfg *Config, winStart, winEnd time.Time) (*WV2Data, error) {
	const limit = 5000

	runMap := map[string]*WV2Run{}
	var instance, townRoot string

	// 1. Discover runs from agent.instantiate.
	instEvs, _ := vlQuery(cfg.LogsURL, "agent.instantiate", limit, winStart, winEnd)
	for _, ev := range instEvs {
		runID := ev.Str("run.id")
		if runID == "" {
			continue
		}
		stream := ev.Str("_stream")
		role := ev.Str("role")
		if role == "" {
			role = extractStreamField(stream, "gt.role")
		}
		rig := ev.Str("rig")
		if rig == "" {
			rig = extractStreamField(stream, "gt.rig")
		}
		agentName := ev.Str("agent_name")
		if agentName == "" {
			agentName = role
		}
		sessID := ev.Str("session_id")
		if sessID == "" {
			sessID = extractStreamField(stream, "gt.session")
		}
		inst := ev.Str("instance")
		tr := ev.Str("town_root")
		if instance == "" && inst != "" {
			instance = inst
			townRoot = tr
		}
		run := &WV2Run{
			RunID:     runID,
			Instance:  inst,
			TownRoot:  tr,
			AgentType: ev.Str("agent_type"),
			Role:      role,
			AgentName: agentName,
			SessionID: sessID,
			Rig:       rig,
			StartedAt: ev.Time().UTC().Format(time.RFC3339),
			Running:   true,
		}
		run.Events = append(run.Events, toWV2Event(ev))
		runMap[runID] = run
	}

	// Build a set of session_ids already covered by agent.instantiate runs.
	coveredSessions := map[string]bool{}
	for _, run := range runMap {
		if run.SessionID != "" {
			coveredSessions[run.SessionID] = true
		}
	}

	// Always supplement with session.start: picks up agents that don't emit
	// agent.instantiate yet (mixed-instrumentation environments).
	sessEvs, _ := vlQuery(cfg.LogsURL, "session.start", limit, winStart, winEnd)
	for _, ev := range sessEvs {
		stream := ev.Str("_stream")
		sessID := ev.Str("session_id")
		if sessID == "" {
			sessID = ev.Str("session")
		}
		if sessID == "" {
			sessID = extractStreamField(stream, "gt.session")
		}
		if sessID == "" || coveredSessions[sessID] {
			continue
		}
		role := ev.Str("role")
		if role == "" {
			role = extractStreamField(stream, "gt.role")
		}
		rig := ev.Str("rig")
		if rig == "" {
			rig = extractStreamField(stream, "gt.rig")
		}
		run := &WV2Run{
			RunID:     sessID,
			Role:      role,
			AgentName: role,
			SessionID: sessID,
			Rig:       rig,
			StartedAt: ev.Time().UTC().Format(time.RFC3339),
			Running:   true,
		}
		run.Events = append(run.Events, toWV2Event(ev))
		runMap[sessID] = run
		coveredSessions[sessID] = true
	}

	// Supplement: discover runs from agent.event / bd.call / mail events.
	// These agents emit events with a `session` + `run.id` payload but no
	// session.start (e.g. polecat agents logged by mayor's daemon).
	for _, evType := range []string{"agent.event", "bd.call", "mail"} {
		evs, _ := vlQuery(cfg.LogsURL, evType, limit, winStart, winEnd)
		for _, ev := range evs {
			runID := ev.Str("run.id")
			sessID := ev.Str("session")
			if sessID == "" {
				sessID = ev.Str("session_id")
			}
			if runID == "" || sessID == "" || runMap[runID] != nil || coveredSessions[sessID] {
				continue
			}
			role, rig, agentName := roleFromSessionName(sessID)
			run := &WV2Run{
				RunID:     runID,
				Role:      role,
				AgentName: agentName,
				SessionID: sessID,
				Rig:       rig,
				StartedAt: ev.Time().UTC().Format(time.RFC3339),
				Running:   true,
			}
			runMap[runID] = run
			coveredSessions[sessID] = true
			coveredSessions[runID] = true
		}
	}

	if len(runMap) == 0 {
		return &WV2Data{
			Window: WV2Window{
				Start: winStart.UTC().Format(time.RFC3339),
				End:   winEnd.UTC().Format(time.RFC3339),
			},
		}, nil
	}

	// session_id → run_id for fallback matching.
	sessToRun := map[string]string{}
	for runID, run := range runMap {
		if run.SessionID != "" {
			sessToRun[run.SessionID] = runID
		}
	}

	// 2. Session stop → set EndedAt.
	stopEvs, _ := vlQuery(cfg.LogsURL, "session.stop", limit, winStart, winEnd)
	for _, ev := range stopEvs {
		run := findRun(runMap, sessToRun, ev)
		if run == nil {
			continue
		}
		run.EndedAt = ev.Time().UTC().Format(time.RFC3339)
		run.Running = false
		st, _ := time.Parse(time.RFC3339, run.StartedAt)
		run.DurationMs = ev.Time().Sub(st).Milliseconds()
		run.Events = append(run.Events, toWV2Event(ev))
	}

	// 3. Load events by type.
	var comms []WV2Comm
	eventQueries := []struct {
		query  string
		isComm bool
	}{
		{"prime", false},
		{"prompt.send", false},
		{"agent.event", false},
		{"bd.call", false},
		{"sling", true},
		{"mail", true},
		{"nudge", true},
		{"polecat.spawn", true},
		{"polecat.remove", false},
		{"done", true},
		{"formula.instantiate", false},
		{"convoy.create", false},
		{"daemon.restart", false},
	}
	for _, q := range eventQueries {
		evs, _ := vlQuery(cfg.LogsURL, q.query, limit, winStart, winEnd)
		for _, ev := range evs {
			run := findRun(runMap, sessToRun, ev)
			if run == nil {
				continue
			}
			wev := toWV2Event(ev)
			wev.RunID = run.RunID
			run.Events = append(run.Events, wev)
			if q.isComm {
				if comm := buildComm(ev, run.RunID); comm.Type != "" {
					comms = append(comms, comm)
				}
			}
		}
	}

	// 4. Correlate claude_code.api_request / tool_result to runs by time proximity.
	claudeToRun := map[string]string{}
	apiEvs, _ := vlQuery(cfg.LogsURL, "claude_code.api_request", limit, winStart, winEnd)
	for _, aev := range apiEvs {
		csid := aev.Str("session.id")
		if csid == "" || claudeToRun[csid] != "" {
			continue
		}
		apiT := aev.Time()
		var best string
		var bestDiff time.Duration = 5 * time.Minute
		for _, run := range runMap {
			st, _ := time.Parse(time.RFC3339, run.StartedAt)
			var et time.Time
			if run.EndedAt != "" {
				et, _ = time.Parse(time.RFC3339, run.EndedAt)
			} else {
				et = winEnd
			}
			if apiT.Before(st) || apiT.After(et.Add(60*time.Second)) {
				continue
			}
			if diff := apiT.Sub(st); diff >= 0 && diff < bestDiff {
				bestDiff = diff
				best = run.RunID
			}
		}
		if best != "" {
			claudeToRun[csid] = best
		}
	}
	for _, aev := range apiEvs {
		runID := claudeToRun[aev.Str("session.id")]
		run := runMap[runID]
		if run == nil {
			continue
		}
		wev := toWV2Event(aev)
		wev.RunID = runID
		run.Events = append(run.Events, wev)
		if cost, err := strconv.ParseFloat(aev.Str("cost_usd"), 64); err == nil {
			run.Cost += cost
		}
	}
	toolEvs, _ := vlQuery(cfg.LogsURL, "claude_code.tool_result", limit, winStart, winEnd)
	for _, tev := range toolEvs {
		runID := claudeToRun[tev.Str("session.id")]
		run := runMap[runID]
		if run == nil {
			continue
		}
		wev := toWV2Event(tev)
		wev.RunID = runID
		run.Events = append(run.Events, wev)
	}

	// 5. Sort events within each run chronologically.
	for _, run := range runMap {
		sort.Slice(run.Events, func(i, j int) bool {
			return run.Events[i].Timestamp < run.Events[j].Timestamp
		})
	}

	// 6. Load beads.
	beadLifecycles, _ := loadBeadLifecycles(cfg, winStart)
	var beadList []WV2Bead
	for _, b := range beadLifecycles {
		bead := WV2Bead{
			ID:        b.ID,
			Title:     b.Title,
			BeadType:  b.Type,
			State:     b.State(),
			CreatedBy: b.CreatedBy,
			Assignee:  b.Assignee,
			CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339),
		}
		if !b.DoneAt.IsZero() {
			bead.DoneAt = b.DoneAt.UTC().Format(time.RFC3339)
		}
		beadList = append(beadList, bead)
	}

	// 7. Group runs by rig.
	rigMap := map[string]*WV2Rig{}
	var rigOrder []string
	for _, run := range runMap {
		name := run.Rig
		if name == "" {
			name = "town"
		}
		if rigMap[name] == nil {
			rigMap[name] = &WV2Rig{Name: name}
			rigOrder = append(rigOrder, name)
		}
		rigMap[name].Runs = append(rigMap[name].Runs, *run)
	}
	sort.Slice(rigOrder, func(i, j int) bool {
		ai, aj := rigOrder[i], rigOrder[j]
		if ai == "town" {
			return false
		}
		if aj == "town" {
			return true
		}
		return ai < aj
	})
	for _, rig := range rigMap {
		sort.Slice(rig.Runs, func(i, j int) bool {
			return rig.Runs[i].StartedAt < rig.Runs[j].StartedAt
		})
	}
	var rigList []WV2Rig
	for _, name := range rigOrder {
		rigList = append(rigList, *rigMap[name])
	}

	// 8. Compute summary.
	totalCost := 0.0
	totalEvents := 0
	var totalDurMs int64
	for _, run := range runMap {
		totalCost += run.Cost
		totalEvents += len(run.Events)
		totalDurMs += run.DurationMs
	}
	if winEnd.IsZero() {
		winEnd = time.Now()
	}

	return &WV2Data{
		Instance: instance,
		TownRoot: townRoot,
		Window: WV2Window{
			Start: winStart.UTC().Format(time.RFC3339),
			End:   winEnd.UTC().Format(time.RFC3339),
		},
		Summary: WV2Summary{
			RunCount:      len(runMap),
			RigCount:      len(rigList),
			EventCount:    totalEvents,
			BeadCount:     len(beadList),
			TotalCost:     totalCost,
			TotalDuration: wv2FmtDur(time.Duration(totalDurMs) * time.Millisecond),
		},
		Rigs:  rigList,
		Comms: comms,
		Beads: beadList,
	}, nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleWaterfallV2JSON serves GET /api/waterfall.json
func handleWaterfallV2JSON(w http.ResponseWriter, r *http.Request) {
	winStart := since(r)
	winEnd := winEndTime(r)
	if winEnd.IsZero() {
		winEnd = time.Now()
	}
	data, err := loadWaterfallV2(globalCfg, winStart, winEnd)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(data)
}

// handleWaterfallV2 serves GET /waterfall — embeds JSON in the page for the Canvas renderer.
func handleWaterfallV2(w http.ResponseWriter, r *http.Request) {
	winStart := since(r)
	winEnd := winEndTime(r)
	if winEnd.IsZero() {
		winEnd = time.Now()
	}
	data, err := loadWaterfallV2(globalCfg, winStart, winEnd)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type pageData struct {
		JSONData template.JS
		Window   string
		Instance string
		TownRoot string
	}
	render(w, tmplWaterfallV2, pageData{
		JSONData: template.JS(jsonBytes),
		Window:   windowLabel(r),
		Instance: data.Instance,
		TownRoot: data.TownRoot,
	})
}
