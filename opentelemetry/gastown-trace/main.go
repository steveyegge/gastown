// gastown-trace: observability web UI for Gastown multi-agent system.
// Queries VictoriaLogs and renders session transcripts, bead lifecycles,
// delegation chains, cost breakdowns, and a live pane stream.
//
// Usage:
//
//	gastown-trace --logs http://localhost:9428 --port 7428
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	LogsURL string
	Port    int
}

var globalCfg *Config

// ── Template helpers ──────────────────────────────────────────────────────────

var funcMap = template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("Jan 2 15:04:05")
	},
	"fmtTimeShort": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("15:04:05")
	},
	"fmtDuration": func(ms float64) string {
		if ms == 0 {
			return "—"
		}
		if ms < 1000 {
			return fmt.Sprintf("%.0fms", ms)
		}
		return fmt.Sprintf("%.1fs", ms/1000)
	},
	"fmtDur": func(d time.Duration) string { return fmtDur(d) },
	"fmtCost": func(c float64) string {
		if c == 0 {
			return "$0"
		}
		if c < 0.001 {
			return fmt.Sprintf("$%.5f", c)
		}
		return fmt.Sprintf("$%.4f", c)
	},
	"fmtTokens": func(n int64) string {
		if n >= 1_000_000 {
			return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
		}
		if n >= 1000 {
			return fmt.Sprintf("%.1fk", float64(n)/1000)
		}
		return strconv.FormatInt(n, 10)
	},
	"roleColor": func(role string) string {
		m := map[string]string{
			"mayor":    "#f59e0b",
			"deacon":   "#8b5cf6",
			"witness":  "#3b82f6",
			"refinery": "#10b981",
			"polecat":  "#ef4444",
			"dog":      "#f97316",
			"boot":     "#6b7280",
			"crew":     "#06b6d4",
			"unknown":  "#9ca3af",
		}
		if c, ok := m[role]; ok {
			return c
		}
		return "#9ca3af"
	},
	"truncate": func(s string, n int) string {
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.TrimSpace(s)
		if len(s) > n {
			return s[:n] + "…"
		}
		return s
	},
	"stateColor": func(state string) string {
		switch state {
		case "done", "closed":
			return "#56d364"
		case "in_progress":
			return "#f59e0b"
		default:
			return "#8b949e"
		}
	},
	"add": func(a, b int) int { return a + b },
	// isUUID returns true if s looks like a Claude session UUID (xxxxxxxx-xxxx-...)
	"isUUID": func(s string) bool {
		return len(s) >= 36 && s[8] == '-' && s[13] == '-'
	},
}

func render(w http.ResponseWriter, tmplStr string, data any) {
	t, err := template.New("page").Funcs(funcMap).Parse(tmplBase + tmplStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

// since returns the start of the time window from query params.
// Supports ?window=1h|24h|7d and custom ?start=2006-01-02T15:04
func since(r *http.Request) time.Time {
	q := r.URL.Query()
	if s := q.Get("start"); s != "" {
		t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
		if err == nil {
			return t
		}
	}
	switch q.Get("window") {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Now().Add(-24 * time.Hour)
	}
}

// winEndTime returns an explicit end time if ?end= is set, else zero (= now).
func winEndTime(r *http.Request) time.Time {
	if s := r.URL.Query().Get("end"); s != "" {
		t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// windowLabel returns a display string for the current window params.
func windowLabel(r *http.Request) string {
	q := r.URL.Query()
	if q.Get("start") != "" {
		end := q.Get("end")
		if end == "" {
			end = "now"
		}
		return q.Get("start") + " → " + end
	}
	switch q.Get("window") {
	case "1h":
		return "1h"
	case "7d":
		return "7d"
	case "30d":
		return "30d"
	default:
		return "24h"
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	cfg := globalCfg
	win := since(r)

	sessions, _ := loadSessions(cfg, win)
	apiReqs, _ := loadAPIRequests(cfg, win)
	costs := computeCosts(apiReqs)
	toolCalls, _ := loadToolCalls(cfg, win)
	paneLines, _ := loadPaneOutput(cfg, "", time.Now().Add(-5*time.Minute), 20)

	// Count by role
	roleCounts := map[string]int{}
	running := 0
	for _, s := range sessions {
		roleCounts[s.Role]++
		if s.Running {
			running++
		}
	}

	render(w, tmplDashboard, map[string]any{
		"Sessions":   sessions,
		"Running":    running,
		"RoleCounts": roleCounts,
		"Costs":      costs,
		"ToolCalls":  toolCalls[:minInt(15, len(toolCalls))],
		"PaneLines":  paneLines,
		"Now":        time.Now().Local().Format("15:04:05"),
		"Window":     r.URL.Query().Get("window"),
	})
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, _ := loadSessions(globalCfg, since(r))
	corrMap := correlateClaudeSessions(globalCfg, sessions, since(r))
	for i := range sessions {
		sessions[i].ClaudeSessionIDs = corrMap[sessions[i].ID]
	}
	render(w, tmplSessions, map[string]any{
		"Sessions": sessions,
		"Window":   r.URL.Query().Get("window"),
	})
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/session/")
	if name == "" {
		http.Redirect(w, r, "/sessions", http.StatusFound)
		return
	}
	win := since(r)

	// Find the session metadata (search wider if recent window finds nothing)
	sessions, _ := loadSessions(globalCfg, win)
	var sess *Session
	for i := range sessions {
		if sessions[i].ID == name || sessions[i].Actor == name {
			sess = &sessions[i]
			break
		}
	}
	if sess == nil {
		wider, _ := loadSessions(globalCfg, time.Now().Add(-7*24*time.Hour))
		for i := range wider {
			if wider[i].ID == name || wider[i].Actor == name {
				sess = &wider[i]
				break
			}
		}
	}

	// Beads assigned to this session
	beads, _ := loadBeadLifecycles(globalCfg, time.Now().Add(-7*24*time.Hour))
	var assignedBeads []BeadLifecycle
	for _, b := range beads {
		if b.Assignee == name {
			assignedBeads = append(assignedBeads, b)
		}
	}

	lines, _ := loadPaneOutput(globalCfg, name, win, 1000)

	// If no pane output and we have a time range, load from all sessions during that window.
	// Polecats run inside parent tmux sessions (e.g. fai-witness), not their own session.
	var parentLines []PaneLine
	isSubSession := strings.Contains(name, "/")
	if len(lines) == 0 && sess != nil && !sess.StartedAt.IsZero() {
		pEnd := sess.StoppedAt
		if pEnd.IsZero() {
			pEnd = time.Now()
		}
		// pane.output with no session filter → all sessions in this time range
		parentLines, _ = loadPaneOutput(globalCfg, "", sess.StartedAt, 300)
		// Filter to lines within the session's lifetime
		var filtered []PaneLine
		for _, l := range parentLines {
			if !l.Time.Before(sess.StartedAt) && !l.Time.After(pEnd.Add(time.Minute)) {
				filtered = append(filtered, l)
			}
		}
		parentLines = filtered
	}

	render(w, tmplSessionDetail, map[string]any{
		"Name":          name,
		"Session":       sess,
		"Lines":         lines,
		"ParentLines":   parentLines,
		"IsSubSession":  isSubSession,
		"AssignedBeads": assignedBeads,
		"Window":        r.URL.Query().Get("window"),
	})
}

func handleTools(w http.ResponseWriter, r *http.Request) {
	calls, _ := loadToolCalls(globalCfg, since(r))
	// Group by tool name
	byTool := map[string]int{}
	for _, c := range calls {
		byTool[c.ToolName]++
	}
	render(w, tmplTools, map[string]any{
		"Calls":  calls,
		"ByTool": byTool,
		"Window": r.URL.Query().Get("window"),
	})
}

func handleCosts(w http.ResponseWriter, r *http.Request) {
	reqs, _ := loadAPIRequests(globalCfg, since(r))
	costs := computeCosts(reqs)

	// Group by claude session for per-session cost
	bySess := map[string]float64{}
	for _, req := range reqs {
		bySess[req.SessionID] += req.CostUSD
	}
	type sessEntry struct {
		ID   string
		Cost float64
	}
	var bySession []sessEntry
	for id, cost := range bySess {
		bySession = append(bySession, sessEntry{id, cost})
	}
	sort.Slice(bySession, func(i, j int) bool {
		return bySession[i].Cost > bySession[j].Cost
	})

	render(w, tmplCosts, map[string]any{
		"Costs":     costs,
		"Requests":  reqs[:minInt(50, len(reqs))],
		"BySession": bySession[:minInt(20, len(bySession))],
		"Window":    r.URL.Query().Get("window"),
	})
}

func handleBeads(w http.ResponseWriter, r *http.Request) {
	beads, _ := loadBeadLifecycles(globalCfg, since(r))
	// Stats
	var done, inProg, open int
	var totalTTS, totalTTD time.Duration
	var cntTTS, cntTTD int
	for _, b := range beads {
		switch b.State() {
		case "done":
			done++
			if b.TotalTime > 0 {
				totalTTD += b.TotalTime
				cntTTD++
			}
		case "in_progress":
			inProg++
		default:
			open++
		}
		if b.TimeToStart > 0 {
			totalTTS += b.TimeToStart
			cntTTS++
		}
	}
	avgTTS, avgTTD := time.Duration(0), time.Duration(0)
	if cntTTS > 0 {
		avgTTS = totalTTS / time.Duration(cntTTS)
	}
	if cntTTD > 0 {
		avgTTD = totalTTD / time.Duration(cntTTD)
	}

	render(w, tmplBeads, map[string]any{
		"Beads":  beads[:minInt(100, len(beads))],
		"Total":  len(beads),
		"Done":   done,
		"InProg": inProg,
		"Open":   open,
		"AvgTTS": avgTTS,
		"AvgTTD": avgTTD,
		"Window": r.URL.Query().Get("window"),
	})
}

func handleDelegation(w http.ResponseWriter, r *http.Request) {
	win := since(r)
	beads, _ := loadBeadLifecycles(globalCfg, win)
	roots := buildDelegationTree(beads)

	// Also load raw sling bd.calls (--status=hooked --assignee=X updates)
	slings, _ := loadBDCalls(globalCfg, win, `args:"hooked" AND args:"assignee"`)

	render(w, tmplDelegation, map[string]any{
		"Roots":  roots[:minInt(50, len(roots))],
		"Slings": slings[:minInt(100, len(slings))],
		"Window": r.URL.Query().Get("window"),
	})
}

type timelineSession struct {
	Session
	LeftPct  float64
	WidthPct float64
}

func handleTimeline(w http.ResponseWriter, r *http.Request) {
	win := since(r)
	winEnd := time.Now()
	sessions, _ := loadSessions(globalCfg, win)

	winMs := float64(winEnd.Sub(win).Milliseconds())
	if winMs <= 0 {
		winMs = 1
	}

	var tlSessions []timelineSession
	for _, s := range sessions {
		start := s.StartedAt
		if start.Before(win) {
			start = win
		}
		end := s.StoppedAt
		if s.Running || end.IsZero() {
			end = winEnd
		}
		if end.After(winEnd) {
			end = winEnd
		}

		leftMs := float64(start.Sub(win).Milliseconds())
		if leftMs < 0 {
			leftMs = 0
		}
		widthMs := float64(end.Sub(start).Milliseconds())
		if widthMs < 0 {
			widthMs = 0
		}

		leftPct := leftMs / winMs * 100
		widthPct := widthMs / winMs * 100
		if widthPct < 0.3 {
			widthPct = 0.3
		}

		tlSessions = append(tlSessions, timelineSession{
			Session:  s,
			LeftPct:  leftPct,
			WidthPct: widthPct,
		})
	}

	render(w, tmplTimeline, map[string]any{
		"Sessions": tlSessions,
		"Window":   r.URL.Query().Get("window"),
		"WinStart": win,
		"WinEnd":   winEnd,
	})
}

// GET /live — SSE stream of pane output
func handleLive(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	cursor := time.Now().Add(-30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			lines, err := loadPaneOutput(globalCfg, session, cursor, 100)
			if err != nil || len(lines) == 0 {
				continue
			}
			for _, l := range lines {
				if !l.Time.After(cursor) {
					continue
				}
				if l.Time.After(cursor) {
					cursor = l.Time
				}
				data, _ := json.Marshal(map[string]string{
					"session": l.Session,
					"time":    l.Time.Local().Format("15:04:05"),
					"content": l.Content,
				})
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			flusher.Flush()
		}
	}
}

func handleLiveView(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, tmplLiveHTML, session, session)
}

// GET /api/sessions.json
func handleAPISessions(w http.ResponseWriter, r *http.Request) {
	sessions, _ := loadSessions(globalCfg, since(r))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func handleFlow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	win := since(r)
	roleFilter := q.Get("role")
	kindFilter := q.Get("kind")
	beadFilter := q.Get("bead")

	events, _ := loadFlowEvents(globalCfg, win, roleFilter, kindFilter, beadFilter)

	render(w, tmplFlow, map[string]any{
		"Events":     events,
		"Window":     q.Get("window"),
		"RoleFilter": roleFilter,
		"KindFilter": kindFilter,
		"BeadFilter": beadFilter,
		"Count":      len(events),
	})
}

func handleBead(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/bead/")
	if id == "" {
		http.Redirect(w, r, "/beads", http.StatusFound)
		return
	}
	bf, err := loadBeadFull(globalCfg, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	render(w, tmplBeadFull, map[string]any{
		"Bead":   bf,
		"Window": r.URL.Query().Get("window"),
	})
}

func main() {
	logsURL := flag.String("logs", "http://localhost:9428", "VictoriaLogs base URL")
	port := flag.Int("port", 7428, "HTTP listen port")
	flag.Parse()

	globalCfg = &Config{LogsURL: *logsURL, Port: *port}

	log.Printf("gastown-trace on :%d  logs=%s", *port, *logsURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard)
	mux.HandleFunc("/flow", handleFlow)
	mux.HandleFunc("/waterfall", handleWaterfallV2)
	mux.HandleFunc("/api/waterfall.json", handleWaterfallV2JSON)
	mux.HandleFunc("/bead/", handleBead)
	mux.HandleFunc("/sessions", handleSessions)
	mux.HandleFunc("/session/", handleSession)
	mux.HandleFunc("/tools", handleTools)
	mux.HandleFunc("/costs", handleCosts)
	mux.HandleFunc("/beads", handleBeads)
	mux.HandleFunc("/delegation", handleDelegation)
	mux.HandleFunc("/timeline", handleTimeline)
	mux.HandleFunc("/live", handleLive)
	mux.HandleFunc("/live-view", handleLiveView)
	mux.HandleFunc("/api/sessions.json", handleAPISessions)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		log.Fatal(err)
	}
}
