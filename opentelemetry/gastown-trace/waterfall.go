// Waterfall view for Claude Code sessions
// Similar to Chrome DevTools Network Waterfall
package main

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// WaterfallEvent represents a single interaction within a Claude session
type WaterfallEvent struct {
	Time      time.Time
	OffsetSec float64
	DurSec    float64
	Kind      string // prompt, tool_use, tool_result, thinking, answer
	Content   string
	Tokens    int
	CostUSD   float64
}

// WaterfallSession represents a Claude Code session in the waterfall
type WaterfallSession struct {
	SessionID    string
	SessionTime  time.Time
	Model        string
	StartOffset  float64 // seconds from waterfall start
	DurationSec  float64
	TotalCost    float64
	APICount     int
	Status       string
	Role         string  // mayor, witness, polecat, etc.
	Rig          string  // rig name if part of a rig
	Launcher     string // who launched this session
	Events       []WaterfallEvent // individual interactions
}

// WaterfallViewData holds data for waterfall template
type WaterfallViewData struct {
	Sessions      []WaterfallSession
	Window        string
	TotalCost     float64
	TotalAPICalls int
	WindowStart   time.Time
	WindowEnd     time.Time
	MaxDuration   float64
	GTRoot        string // GT directory path
	DoltPort      string // Dolt server port
}

// rigFromSession derives a rig name from a tmux session name
func rigFromSession(session string) string {
	// Patterns: "fai-witness" -> "fai", "gt-wyvern-toast" -> "gt-wyvern", etc.
	parts := strings.Split(session, "-")
	if len(parts) >= 2 {
		// Check for known rig prefixes
		rigPrefixes := []string{"fai", "gt"}
		for _, prefix := range rigPrefixes {
			if parts[0] == prefix {
				if len(parts) >= 3 && parts[1] != "wyvern" {
					return parts[0] + "-" + parts[1]
				}
				return parts[0]
			}
		}
	}
	return ""
}

// Load Claude Code sessions with detailed interactions
func loadWaterfallData(cfg *Config, since time.Time) ([]WaterfallSession, float64, int, string, string) {
	// Load sessions to get tmux correlation
	sessions, err := loadSessions(cfg, since)
	if err != nil {
		return nil, 0, 0, "", ""
	}

	// Extract GT directory and launcher info from most recent session
	var gtRoot, doltPort, launcher string
	if len(sessions) > 0 {
		// Use the most recent session to get GT context
		mostRecent := sessions[len(sessions)-1]
		actor := mostRecent.Actor
		if actor != "" {
			launcher = actor + " (" + mostRecent.Role + ")"
		}
		// Try to extract directory from Actor or use ID context
		// For now, we'll use a placeholder or try to detect from session ID
		if mostRecent.Topic != "" {
			gtRoot = mostRecent.Topic
		}
		// Default values if we can't extract
		if gtRoot == "" {
			gtRoot = "Unknown GT Directory"
		}
		// Try to extract Dolt port from environment or common ports
		// We'll use the most common GT test port as default
		doltPort = "33406" // Default GT test port
		_ = doltPort // Use doltPort to avoid "declared and not used" error
	}

	// Load API requests from Claude Code
	reqs, err := loadAPIRequests(cfg, since)
	if err != nil || len(reqs) == 0 {
		return nil, 0, 0, gtRoot, doltPort
	}

	// Load agent events for detailed interaction data
	agentQ := "agent.event"
	agentEvs, err := vlQuery(cfg.LogsURL, agentQ, 5000, since, time.Time{})
	if err != nil {
		agentEvs = []LogEvent{}
	}

	// Group agent events by session ID
	eventsBySession := make(map[string][]LogEvent)
	for _, ev := range agentEvs {
		sessID := ev["native_session_id"]
		if sessID == "" {
			sessID = ev["session"]
		}
		eventsBySession[sessID] = append(eventsBySession[sessID], ev)
	}

	// Build correlation map: Claude UUID -> tmux session info
	corrMap := correlateClaudeSessions(cfg, sessions, since)

	// Group API requests by session ID and build waterfall sessions
	sessionMap := make(map[string]*WaterfallSession)

	for _, req := range reqs {
		sessionID := req.SessionID
		if sessionID == "" {
			continue
		}

		// Extract model from request metadata
		model := req.Model
		if model == "" {
			model = "unknown"
		}
		if idx := strings.LastIndex(model, "-202"); idx > 0 {
			model = model[:idx]
		}

		sess, exists := sessionMap[sessionID]
		if !exists {
			// Find corresponding tmux session for role/rig info
			var role, rig string
			for _, tmuxID := range corrMap[sessionID] {
				for _, s := range sessions {
					if s.ID == tmuxID {
						role = s.Role
						rig = rigFromSession(tmuxID)
						break
					}
				}
				if role != "" {
					break
				}
			}
			if role == "" {
				role = "unknown"
			}

			sess = &WaterfallSession{
				SessionID:   sessionID,
				SessionTime: req.Time,
				Model:       model,
				Role:        role,
				Rig:         rig,
				Launcher:     launcher,
				TotalCost:   req.CostUSD,
				APICount:    1,
				Events:      []WaterfallEvent{},
			}
			sessionMap[sessionID] = sess
		} else {
			sess.TotalCost += req.CostUSD
			sess.APICount++
			// Update session time to earliest request
			if req.Time.Before(sess.SessionTime) {
				sess.SessionTime = req.Time
			}
		}
	}

	// Build detailed interaction events for each session
	for sessID, sess := range sessionMap {
		sessionStartTime := sess.SessionTime

		// Process agent events for this session
		for _, ev := range eventsBySession[sessID] {
			eventTime := ev.Time()
			if eventTime.IsZero() {
				continue
			}

			offset := eventTime.Sub(sessionStartTime).Seconds()
			if offset < 0 {
				offset = 0
			}

			kind := ev["event_type"]
			content := ev["content"]

			// Map agent event types to waterfall event kinds
			waterfallKind := kind
			switch kind {
			case "text":
				waterfallKind = "prompt"
			case "tool_use":
				waterfallKind = "tool_use"
			case "tool_result":
				waterfallKind = "tool_result"
			case "thinking":
				waterfallKind = "thinking"
			case "answer":
				waterfallKind = "answer"
			}

			tokens := parseInt(ev["tokens"])
			cost := parseFloat(ev["cost_usd"])

			sess.Events = append(sess.Events, WaterfallEvent{
				Time:      eventTime,
				OffsetSec: offset,
				Kind:      waterfallKind,
				Content:   content,
				Tokens:    tokens,
				CostUSD:   cost,
			})
		}

		// Sort events chronologically
		sort.Slice(sess.Events, func(i, j int) bool {
			return sess.Events[i].Time.Before(sess.Events[j].Time)
		})

		// Calculate total duration
		if len(sess.Events) > 0 {
			lastEvent := sess.Events[len(sess.Events)-1]
			sess.DurationSec = lastEvent.OffsetSec + 10 // Add 10s buffer
		} else {
			sess.DurationSec = 60 // Default duration
		}
	}

	// Convert map to slice
	var waterfallSessions []WaterfallSession
	for _, sess := range sessionMap {
		waterfallSessions = append(waterfallSessions, *sess)
	}

	// Sort by session time
	sort.Slice(waterfallSessions, func(i, j int) bool {
		return waterfallSessions[i].SessionTime.Before(waterfallSessions[j].SessionTime)
	})

	// Calculate time offsets from first session
	if len(waterfallSessions) > 0 {
		startTime := waterfallSessions[0].SessionTime
		for i := range waterfallSessions {
			waterfallSessions[i].StartOffset = waterfallSessions[i].SessionTime.Sub(startTime).Seconds()
			waterfallSessions[i].Status = "completed"
		}
	}

	// Calculate totals
	totalCost := 0.0
	totalAPICalls := 0
	for _, sess := range waterfallSessions {
		totalCost += sess.TotalCost
		totalAPICalls += sess.APICount
	}

	return waterfallSessions, totalCost, totalAPICalls, gtRoot, doltPort
}

// Helper functions
func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func handleWaterfall(w http.ResponseWriter, r *http.Request) {
	win := since(r)

	sessions, totalCost, totalAPICalls, gtRoot, doltPort := loadWaterfallData(globalCfg, win)

	// Calculate max duration for timeline scaling
	maxDuration := 60.0 // Default 60s
	if len(sessions) > 0 {
		lastSession := sessions[len(sessions)-1]
		sessionEnd := lastSession.StartOffset + lastSession.DurationSec
		if sessionEnd > maxDuration {
			maxDuration = sessionEnd
		}
	}

	data := WaterfallViewData{
		Sessions:      sessions,
		Window:        r.URL.Query().Get("window"),
		TotalCost:     totalCost,
		TotalAPICalls: totalAPICalls,
		WindowStart:   win,
		WindowEnd:     time.Now(),
		MaxDuration:   maxDuration,
		GTRoot:        gtRoot,
		DoltPort:      doltPort,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := getWaterfallTemplate().Execute(w, data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// Helper functions for template
func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func mul(a, b float64) float64 {
	return a * b
}

// Waterfall HTML template
const waterfallTemplateStr = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Gastown Trace - Waterfall</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'JetBrains Mono',monospace,sans-serif;background:#0d1117;color:#c9d1d9;font-size:13px;line-height:1.5}
a{color:#58a6ff;text-decoration:none}
a:hover{text-decoration:underline}
nav{background:#161b22;border-bottom:1px solid #30363d;padding:8px 16px;display:flex;gap:16px;align-items:center;flex-wrap:wrap}
nav .brand{color:#f0f6fc;font-weight:700;font-size:15px;margin-right:8px}
nav a{color:#8b949e;padding:4px 8px;border-radius:4px}
nav a:hover{color:#c9d1d9;background:#21262d;text-decoration:none}
main{padding:16px}
h1{font-size:16px;font-weight:700;color:#f0f6fc;margin-bottom:12px}
h2{font-size:13px;font-weight:600;color:#8b949e;text-transform:uppercase;letter-spacing:.06em;margin:16px 0 8px}
.cards{display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px}
.card{background:#161b22;border:1px solid #30363d;border-radius:6px;padding:12px 16px;min-width:120px}
.card .val{font-size:22px;font-weight:700;color:#f0f6fc}
.card .lbl{font-size:11px;color:#8b949e;margin-top:2px}
table{width:100%;border-collapse:collapse;font-size:12px}
th{text-align:left;padding:6px 10px;border-bottom:1px solid #30363d;color:#8b949e;font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.05em}
td{padding:5px 10px;border-bottom:1px solid #21262d;vertical-align:top}
tr:hover td{background:#161b22}
.badge{display:inline-block;padding:1px 6px;border-radius:10px;font-size:10px;font-weight:600;color:#0d1117}
.mono{font-family:monospace;font-size:11px;color:#79c0ff}
.dim{color:#8b949e}
.ok{color:#56d364}
.section{background:#161b22;border:1px solid #30363d;border-radius:6px;margin-bottom:16px;overflow:hidden}
.waterfall-container{overflow-x:auto;margin-top:16px}
.waterfall-table{min-width:1600px}
.waterfall-header{display:grid;grid-template-columns:240px 100px 70px 70px 70px 1fr 60px;padding:8px 12px;background:#0d1117;border-bottom:1px solid #30363d;font-size:11px;font-weight:600;color:#8b949e;text-transform:uppercase;letter-spacing:.05em}
.waterfall-row{display:grid;grid-template-columns:240px 100px 70px 70px 70px 1fr 60px;padding:8px 12px;border-bottom:1px solid #21262d;align-items:center}
.waterfall-row:hover{background:#161b22}
.waterfall-timeline{position:relative;height:30px;background:#21262d;border-radius:4px;overflow:hidden}
.waterfall-event{position:absolute;height:100%;border-radius:4px;min-width:4px;display:inline-block;cursor:pointer;opacity:0.9}
.waterfall-event:hover{opacity:1;z-index:10}
.waterfall-event-prompt{background:#1f6feb}
.waterfall-event-thinking{background:#f59e0b}
.waterfall-event-tool_use{background:#10b981}
.waterfall-event-tool_result{background:#8b5cf6}
.waterfall-event-answer{background:#ec4899}
.waterfall-ruler{display:flex;justify-content:space-between;padding:8px 0;font-size:10px;color:#8b949e;border-bottom:1px solid #30363d;margin-bottom:8px}
.waterfall-ruler span{position:relative}
.waterfall-ruler span::before{content:'';position:absolute;left:0;top:12px;width:1px;height:6px;background:#8b949e}
.win-sel{display:flex;gap:4px;margin-left:auto}
.win-sel a{font-size:11px;padding:2px 8px;border:1px solid #30363d;border-radius:4px;color:#8b949e}
.win-sel a.active{background:#1f6feb;border-color:#1f6feb;color:#fff}
.tooltip{position:absolute;background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:12px;font-size:11px;color:#c9d1d9;max-width:400px;max-height:300px;overflow:auto;z-index:1000;box-shadow:0 8px 24px rgba(0,0,0,0.4);display:none;white-space:pre-wrap;word-break:break-word}
.tooltip.show{display:block}
.tooltip-header{font-weight:600;color:#f0f6fc;margin-bottom:8px;padding-bottom:8px;border-bottom:1px solid #30363d}
.tooltip-kind{display:inline-block;padding:2px 6px;border-radius:4px;font-size:10px;font-weight:600;margin-right:8px}
.tooltip-kind.prompt{background:#1f6eb222;color:#58a6ff}
.tooltip-kind.thinking{background:#f59e0b22;color:#f59e0b}
.tooltip-kind.tool_use{background:#10b98122;color:#34d399}
.tooltip-kind.tool_result{background:#8b5cf622;color:#a78bfa}
.tooltip-kind.answer{background:#ec489922;color:#f472b6}
.tooltip-content{margin-top:8px;max-height:200px;overflow:auto}
.tooltip-meta{margin-top:8px;font-size:10px;color:#8b949e}
</style>
</head>
<body>
<nav>
  <a href="/" class="brand">gastown-trace</a>
  <a href="/">Dashboard</a>
  <a href="/flow">Flow</a>
  <a href="/waterfall">Waterfall</a>
  <a href="/sessions">Sessions</a>
  <a href="/tools">Tools</a>
  <a href="/costs">Costs</a>
  <a href="/beads">Beads</a>
  <a href="/delegation">Delegation</a>
  <div class="win-sel">
    <a href="?window=1h"{{if eq $.Window "1h"}} class="active"{{end}}>1h</a>
    <a href="?window=24h"{{if eq $.Window "24h"}} class="active"{{end}}>24h</a>
    <a href="?window=7d"{{if eq $.Window "7d"}} class="active"{{end}}>7d</a>
  </div>
</nav>
<main>
<h1>Claude Code Waterfall</h1>
{{if .GTRoot}}
<div class="section" style="padding:8px 12px;margin-bottom:12px;">
  <div style="display:flex;gap:16px;align-items:center;flex-wrap:wrap;">
    <div style="color:#8b949e;font-size:11px;">GT Directory:</div>
    <div class="mono" style="color:#f0f6fc;">{{.GTRoot}}</div>
    <div style="color:#8b949e;font-size:11px;">Dolt Port:</div>
    <div class="mono" style="color:#f0f6fc;">{{.DoltPort}}</div>
  </div>
</div>
{{end}}
<div class="cards">
  <div class="card">
    <div class="val">{{len .Sessions}}</div>
    <div class="lbl">Sessions</div>
  </div>
  <div class="card">
    <div class="val">{{.TotalAPICalls}}</div>
    <div class="lbl">API Calls</div>
  </div>
  <div class="card">
    <div class="val">${{printf "%.4f" .TotalCost}}</div>
    <div class="lbl">Total Cost</div>
  </div>
</div>
<h2>Session Timeline</h2>
<div class="section">
  {{if gt (len .Sessions) 0}}
  <div class="waterfall-container">
    <div class="waterfall-ruler">
      <span>0s</span>
      <span>{{printf "%.0f" (div .MaxDuration 4)}}s</span>
      <span>{{printf "%.0f" (div .MaxDuration 2)}}s</span>
      <span>{{printf "%.0f" (mul .MaxDuration 0.75)}}s</span>
      <span>{{printf "%.0f" .MaxDuration}}s</span>
    </div>
    <div class="waterfall-table">
      <div class="waterfall-header">
        <div>Session ID</div>
        <div>Launcher</div>
        <div>Agent</div>
        <div>Rig</div>
        <div>Model</div>
        <div>Timeline</div>
        <div>Calls</div>
      </div>
      {{range .Sessions}}
      <div class="waterfall-row">
        <div class="mono" style="word-break:break-all" title="{{.SessionID}}">{{truncate .SessionID 24}}</div>
        <div class="dim" style="font-size:10px">{{if .Launcher}}{{.Launcher}}{{else}}—{{end}}</div>
        <div class="badge" style="background:{{roleColor .Role}}">{{.Role}}</div>
        <div class="dim" style="font-size:10px">{{if .Rig}}{{.Rig}}{{else}}—{{end}}</div>
        <div class="badge" style="background:#1f6eb;font-size:9px">{{.Model}}</div>
        <div class="waterfall-timeline" data-session="{{.SessionID}}">
          {{range .Events}}
          <div class="waterfall-event waterfall-event-{{.Kind}}"
               style="left:{{printf "%.3f" (mul (add .OffsetSec .StartOffset) (div 100 $.MaxDuration))}}%;width:{{printf "%.3f" (mul (max .DurSec 0.3) (div 100 $.MaxDuration))}}%"
               data-kind="{{.Kind}}"
               data-content="{{.Content}}"
               data-tokens="{{.Tokens}}"
               data-time="{{printf "%.2f" .OffsetSec}}s">
          </div>
          {{end}}
        </div>
        <div>{{.APICount}}</div>
      </div>
      {{end}}
    </div>
  {{else}}
  <div style="padding:20px;text-align:center;color:#8b949e">
    No Claude Code sessions found in this time window.
  </div>
  {{end}}
</div>
<h2>Detailed Sessions</h2>
<div class="section">
  <table>
    <tr>
      <th>Session ID</th>
      <th>Launcher</th>
      <th>Agent</th>
      <th>Rig</th>
      <th>Model</th>
      <th>Time</th>
      <th>Offset</th>
      <th>Duration</th>
      <th>Events</th>
      <th>Cost</th>
    </tr>
    {{range .Sessions}}
    <tr>
      <td class="mono" style="font-size:11px"><a href="/flow?kind=api&window={{$.Window}}">{{.SessionID}}</a></td>
      <td class="dim" style="font-size:10px">{{if .Launcher}}{{.Launcher}}{{else}}—{{end}}</td>
      <td><span class="badge" style="background:{{roleColor .Role}}">{{.Role}}</span></td>
      <td>{{if .Rig}}{{.Rig}}{{else}}—{{end}}</td>
      <td>{{.Model}}</td>
      <td class="dim">{{.SessionTime.Format "15:04:05"}}</td>
      <td>{{printf "%.2f" .StartOffset}}s</td>
      <td>{{printf "%.2f" .DurationSec}}s</td>
      <td>{{len .Events}}</td>
      <td>${{printf "%.4f" .TotalCost}}</td>
    </tr>
    {{end}}
  </table>
</div>
<h2>Timeline Legend</h2>
<div class="section" style="padding:12px 16px;display:flex;gap:20px;flex-wrap:wrap">
  <div><span class="tooltip-kind prompt">●</span> Prompt</div>
  <div><span class="tooltip-kind thinking">●</span> Thinking</div>
  <div><span class="tooltip-kind tool_use">●</span> Tool Use</div>
  <div><span class="tooltip-kind tool_result">●</span> Tool Result</div>
  <div><span class="tooltip-kind answer">●</span> Answer</div>
</div>
</main>
<div id="tooltip" class="tooltip">
  <div class="tooltip-header">
    <span class="tooltip-kind" id="tooltip-kind"></span>
    <span id="tooltip-time"></span>
  </div>
  <div class="tooltip-content" id="tooltip-content"></div>
  <div class="tooltip-meta" id="tooltip-meta"></div>
</div>
<script>
(function() {
  const tooltip = document.getElementById('tooltip');
  const tooltipKind = document.getElementById('tooltip-kind');
  const tooltipContent = document.getElementById('tooltip-content');
  const tooltipMeta = document.getElementById('tooltip-meta');
  const tooltipTime = document.getElementById('tooltip-time');

  document.querySelectorAll('.waterfall-event').forEach(el => {
    el.addEventListener('mouseenter', function(e) {
      const kind = this.dataset.kind;
      const content = this.dataset.content;
      const tokens = this.dataset.tokens;
      const time = this.dataset.time;

      tooltipKind.textContent = kind.toUpperCase();
      tooltipKind.className = 'tooltip-kind ' + kind;
      tooltipTime.textContent = time;
      tooltipContent.textContent = content || '(no content)';
      tooltipMeta.textContent = tokens ? 'Tokens: ' + tokens : '';

      const rect = this.getBoundingClientRect();
      const containerRect = this.parentElement.getBoundingClientRect();

      let left = rect.left - containerRect.left;
      if (left + 400 > containerRect.width) {
        left = containerRect.width - 410;
      }

      tooltip.style.left = left + 'px';
      tooltip.style.top = (rect.bottom + 8) + 'px';
      tooltip.classList.add('show');
    });

    el.addEventListener('mouseleave', function() {
      tooltip.classList.remove('show');
    });
  });
})();
</script>
</body>
</html>`

// Register waterfall template - extends the global funcMap
var waterfallFuncMap = template.FuncMap{
	"div":  div,
	"mul":  mul,
	"add":  func(a, b float64) float64 { return a + b },
	"max":  func(a, b float64) float64 { if a > b { return a }; return b },
	"truncate": func(s string, n int) string {
		if len(s) > n {
			return s[:n] + "..."
		}
		return s
	},
}

// Lazy template initialization
var (
	tmplWaterfall     *template.Template
	tmplWaterfallOnce sync.Once
)

func getWaterfallTemplate() *template.Template {
	tmplWaterfallOnce.Do(func() {
		// Combine global funcMap with waterfall-specific functions
		result := make(template.FuncMap)
		for k, v := range funcMap {
			result[k] = v
		}
		for k, v := range waterfallFuncMap {
			result[k] = v
		}
		tmplWaterfall = template.Must(template.New("waterfall").
			Funcs(result).
			Parse(waterfallTemplateStr))
	})
	return tmplWaterfall
}
