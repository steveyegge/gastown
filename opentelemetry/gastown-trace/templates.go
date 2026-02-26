package main

// ── Base layout ───────────────────────────────────────────────────────────────

const tmplBase = `
{{define "base"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Gastown Trace</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'JetBrains Mono',monospace,sans-serif;background:#0d1117;color:#c9d1d9;font-size:13px;line-height:1.5}
a{color:#58a6ff;text-decoration:none}
a:hover{text-decoration:underline}
nav{background:#161b22;border-bottom:1px solid #30363d;padding:8px 16px;display:flex;gap:16px;align-items:center;flex-wrap:wrap}
nav .brand{color:#f0f6fc;font-weight:700;font-size:15px;margin-right:8px}
nav a{color:#8b949e;padding:4px 8px;border-radius:4px}
nav a:hover{color:#c9d1d9;background:#21262d;text-decoration:none}
.win-sel{display:flex;gap:4px;margin-left:auto}
.win-sel a{font-size:11px;padding:2px 8px;border:1px solid #30363d;border-radius:4px;color:#8b949e}
.win-sel a.active{background:#1f6feb;border-color:#1f6feb;color:#fff}
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
.tag{display:inline-block;padding:1px 6px;border-radius:4px;font-size:11px;background:#21262d;color:#8b949e;border:1px solid #30363d}
.mono{font-family:monospace;font-size:11px;color:#79c0ff}
.dim{color:#8b949e}
.ok{color:#56d364}
.warn{color:#f59e0b}
.err{color:#f87171}
.section{background:#161b22;border:1px solid #30363d;border-radius:6px;margin-bottom:16px;overflow:hidden}
.section-hdr{padding:8px 12px;border-bottom:1px solid #30363d;font-size:11px;font-weight:600;color:#8b949e;text-transform:uppercase;letter-spacing:.05em;background:#0d1117}
pre{white-space:pre-wrap;word-break:break-all;font-family:monospace;font-size:11px;color:#c9d1d9}
.pane-line{padding:2px 12px;border-bottom:1px solid #0d1117;display:flex;gap:12px}
.pane-line:hover{background:#21262d}
.pane-time{color:#8b949e;min-width:70px;flex-shrink:0}
.pane-sess{color:#58a6ff;min-width:100px;flex-shrink:0}
.pane-content{flex:1;white-space:pre-wrap;word-break:break-all}
.tree{padding:8px 12px}
.tree-node{margin:4px 0;padding:4px 8px;border-left:2px solid #30363d;margin-left:16px}
.tree-root{margin-left:0;border-left:2px solid #1f6feb}
.cost-bar-wrap{background:#21262d;border-radius:3px;height:6px;width:100px;display:inline-block;vertical-align:middle}
.cost-bar{background:#1f6feb;border-radius:3px;height:6px;display:block}
.timeline-wrap{overflow-x:auto}
.tl-row{display:flex;align-items:center;gap:8px;padding:3px 0;border-bottom:1px solid #0d1117;font-size:11px}
.tl-label{min-width:130px;flex-shrink:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.tl-bar-area{flex:1;position:relative;height:16px}
.tl-bar{position:absolute;height:14px;border-radius:3px;top:1px;font-size:10px;line-height:14px;padding:0 4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#0d1117;font-weight:600}
/* Flow event kinds */
.kind-create{background:#1f6feb22;color:#58a6ff;border-color:#1f6feb}
.kind-update{background:#8b5cf622;color:#a78bfa;border-color:#8b5cf6}
.kind-session{background:#10b98122;color:#34d399;border-color:#10b981}
.kind-api{background:#f59e0b22;color:#fbbf24;border-color:#f59e0b}
/* Agent event kinds */
.kind-agent-text{background:#10b98133;color:#6ee7b7;border-color:#10b981}
.kind-agent-tool{background:#f59e0b33;color:#fcd34d;border-color:#f59e0b}
.kind-agent-result{background:#3b82f633;color:#93c5fd;border-color:#3b82f6}
.kind-agent-think{background:#8b5cf633;color:#c4b5fd;border-color:#8b5cf6}
/* Inline content block */
.flow-inline{margin-top:4px;background:#0d1117;border-left:2px solid #30363d;padding:4px 8px 4px 10px;font-family:'JetBrains Mono',monospace;font-size:11px;color:#c9d1d9;white-space:pre-wrap;word-break:break-word;max-height:200px;overflow-y:auto;border-radius:0 3px 3px 0}
.flow-inline.tool{border-left-color:#f59e0b}
.flow-inline.result{border-left-color:#3b82f6}
.flow-inline.think{border-left-color:#8b5cf6;color:#a78bfa;font-style:italic}
.flow-inline.prompt{border-left-color:#10b981;color:#6ee7b7}
/* Filter form */
.filters{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:12px;background:#161b22;padding:8px 12px;border-radius:6px;border:1px solid #30363d}
.filters label{font-size:11px;color:#8b949e}
.filters select,.filters input{background:#0d1117;border:1px solid #30363d;color:#c9d1d9;border-radius:4px;padding:3px 6px;font-size:12px;font-family:inherit}
.filters button{background:#1f6feb;border:none;color:#fff;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px}
/* Detail expansion */
details summary{cursor:pointer;color:#8b949e;font-size:11px}
details summary:hover{color:#c9d1d9}
details pre{margin-top:4px;background:#0d1117;padding:8px;border-radius:4px;border:1px solid #30363d;max-height:150px;overflow:auto}
/* Bead detail */
.bead-meta td:first-child{color:#8b949e;font-size:11px;text-transform:uppercase;width:120px;padding-right:8px}
.step-elapsed{color:#3b82f6;font-size:10px;min-width:50px}
.step-step{color:#8b949e;font-size:10px;min-width:50px}
</style>
</head>
<body>
<nav>
  <span class="brand">🏙 Gastown Trace</span>
  <a href="/waterfall">Waterfall</a>
  {{- /*
  <a href="/">Dashboard</a>
  <a href="/flow">Flow</a>
  <a href="/sessions">Sessions</a>
  <a href="/beads">Beads</a>
  <a href="/delegation">Delegation</a>
  <a href="/timeline">Timeline</a>
  <a href="/tools">Tools</a>
  <a href="/costs">Costs</a>
  <a href="/live-view">Live</a>
  */ -}}
  <div class="win-sel">
    <a href="?window=1h" {{if eq .Window "1h"}}class="active"{{end}}>1h</a>
    <a href="?window=" {{if eq .Window ""}}class="active"{{end}}>24h</a>
    <a href="?window=7d" {{if eq .Window "7d"}}class="active"{{end}}>7d</a>
    <a href="?window=30d" {{if eq .Window "30d"}}class="active"{{end}}>30d</a>
    <span style="color:#30363d">|</span>
    <form id="dt-form" style="display:inline-flex;gap:4px;align-items:center" method="GET">
      <input id="dt-start" name="start" type="datetime-local" style="background:#0d1117;border:1px solid #30363d;color:#c9d1d9;border-radius:4px;padding:2px 4px;font-size:10px;font-family:inherit" placeholder="start">
      <input id="dt-end" name="end" type="datetime-local" style="background:#0d1117;border:1px solid #30363d;color:#c9d1d9;border-radius:4px;padding:2px 4px;font-size:10px;font-family:inherit" placeholder="end">
      <button type="submit" style="background:#21262d;border:1px solid #30363d;color:#c9d1d9;border-radius:4px;padding:2px 6px;font-size:10px;cursor:pointer">▶</button>
    </form>
  </div>
</nav>
<main>
{{template "content" .}}
</main>
</body>
</html>
{{end}}
`

// ── Dashboard ─────────────────────────────────────────────────────────────────

const tmplDashboard = `
{{define "content"}}
<h1>Dashboard <span class="dim" style="font-size:12px;font-weight:400">updated {{.Now}}</span></h1>
<div class="cards">
  <div class="card"><div class="val">{{len .Sessions}}</div><div class="lbl">Sessions</div></div>
  <div class="card"><div class="val ok">{{.Running}}</div><div class="lbl">Running</div></div>
  <div class="card"><div class="val">{{fmtCost .Costs.TotalUSD}}</div><div class="lbl">Cost (window)</div></div>
  <div class="card"><div class="val">{{.Costs.RequestCount}}</div><div class="lbl">API Requests</div></div>
  <div class="card"><div class="val">{{fmtTokens .Costs.TotalInput}}</div><div class="lbl">Input tokens</div></div>
  <div class="card"><div class="val">{{fmtTokens .Costs.TotalOutput}}</div><div class="lbl">Output tokens</div></div>
</div>

<div class="section">
  <div class="section-hdr">Active Sessions by Role</div>
  <div style="padding:8px 12px;display:flex;gap:8px;flex-wrap:wrap">
  {{range $role, $cnt := .RoleCounts}}
    <span class="tag" style="color:{{roleColor $role}}">{{$role}} ×{{$cnt}}</span>
  {{end}}
  </div>
</div>

<h2>Recent Tool Calls</h2>
<div class="section">
<table>
<tr><th>Time</th><th>Session</th><th>Tool</th><th>Command</th><th>Dur</th><th>OK</th></tr>
{{range .ToolCalls}}
<tr>
  <td class="dim">{{fmtTimeShort .Time}}</td>
  <td class="mono">{{truncate .SessionID 8}}</td>
  <td><span class="tag">{{.ToolName}}</span></td>
  <td style="max-width:400px">{{truncate .Command 80}}</td>
  <td class="dim">{{fmtDuration .DurationMs}}</td>
  <td>{{if .Success}}<span class="ok">✓</span>{{else}}<span class="err">✗</span>{{end}}</td>
</tr>
{{end}}
</table>
</div>

<h2>Live Pane (last 5 min)</h2>
<div class="section" style="max-height:300px;overflow-y:auto">
{{range .PaneLines}}
<div class="pane-line">
  <span class="pane-time">{{fmtTimeShort .Time}}</span>
  <span class="pane-sess"><a href="/session/{{.Session}}">{{.Session}}</a></span>
  <span class="pane-content">{{.Content}}</span>
</div>
{{end}}
</div>
{{end}}
`

// ── Sessions list ─────────────────────────────────────────────────────────────

const tmplSessions = `
{{define "content"}}
<h1>Sessions</h1>
<div class="section">
<table>
<tr><th>Session ID</th><th>Role</th><th>Actor</th><th>Topic</th><th>Started</th><th>Dur</th><th>Status</th><th>Claude Session IDs</th></tr>
{{range .Sessions}}
<tr>
  <td class="mono"><a href="/session/{{.ID}}">{{.ID}}</a></td>
  <td><span class="badge" style="background:{{roleColor .Role}}">{{.Role}}</span></td>
  <td><a href="/session/{{.Actor}}" class="dim">{{.Actor}}</a></td>
  <td style="max-width:300px">{{truncate .Topic 80}}</td>
  <td class="dim">{{fmtTime .StartedAt}}</td>
  <td class="dim">{{fmtDur .Duration}}</td>
  <td>{{if .Running}}<span class="ok">running</span>{{else}}<span class="dim">stopped</span>{{end}}</td>
  <td class="mono" style="font-size:10px;word-break:break-all">
    {{range .ClaudeSessionIDs}}<a href="/flow?kind=api&bead=&window={{$.Window}}">{{.}}</a><br>{{end}}
  </td>
</tr>
{{end}}
</table>
</div>
{{end}}
`

// ── Session detail ────────────────────────────────────────────────────────────

const tmplSessionDetail = `
{{define "content"}}
<h1>Session <span class="mono">{{.Name}}</span>
  <a href="/live-view?session={{.Name}}" style="font-size:12px;margin-left:12px">▶ Live</a>
  <a href="/flow?role={{if .Session}}{{.Session.Role}}{{end}}&window={{.Window}}" style="font-size:12px;margin-left:8px">▶ Flow</a>
</h1>

{{if .Session}}
<div class="cards">
  <div class="card"><div class="val"><span class="badge" style="background:{{roleColor .Session.Role}}">{{.Session.Role}}</span></div><div class="lbl">Role</div></div>
  <div class="card"><div class="val">{{fmtDur .Session.Duration}}</div><div class="lbl">Duration</div></div>
  <div class="card"><div class="val">{{if .Session.Running}}<span class="ok">running</span>{{else}}<span class="dim">stopped</span>{{end}}</div><div class="lbl">Status</div></div>
  <div class="card"><div class="val">{{fmtTime .Session.StartedAt}}</div><div class="lbl">Started</div></div>
</div>

<div class="section">
  <div class="section-hdr">Session Details</div>
  <table class="bead-meta" style="width:auto">
    <tr><td>Session ID</td><td class="mono">{{.Session.ID}}</td></tr>
    <tr><td>Actor</td><td class="mono">{{.Session.Actor}}</td></tr>
    {{if .Session.Topic}}<tr><td>Topic</td><td>{{.Session.Topic}}</td></tr>{{end}}
    {{if .Session.ClaudeSessionIDs}}
    <tr><td>Claude Sessions</td><td class="mono" style="word-break:break-all">
      {{range .Session.ClaudeSessionIDs}}{{.}}<br>{{end}}
    </td></tr>
    {{end}}
  </table>
</div>

{{if .Session.Prompt}}
<div class="section">
  <div class="section-hdr">Prompt</div>
  <pre style="padding:12px;max-height:250px;overflow:auto">{{.Session.Prompt}}</pre>
</div>
{{end}}
{{end}}

{{if .AssignedBeads}}
<div class="section">
  <div class="section-hdr">Beads assigned to {{.Name}}</div>
  <table>
    <tr><th>Bead ID</th><th>Title</th><th>Type</th><th>State</th><th>Created by</th><th>Time-to-start</th><th>Total time</th></tr>
    {{range .AssignedBeads}}
    <tr>
      <td class="mono"><a href="/bead/{{.ID}}">{{.ID}}</a></td>
      <td><a href="/bead/{{.ID}}">{{.Title}}</a></td>
      <td class="dim">{{.Type}}</td>
      <td><span class="badge" style="background:{{stateColor .State}}">{{.State}}</span></td>
      <td class="dim">{{.CreatedBy}}</td>
      <td class="dim">{{fmtDur .TimeToStart}}</td>
      <td class="dim">{{fmtDur .TotalTime}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{end}}

{{if .Lines}}
<div class="section">
  <div class="section-hdr">Pane transcript ({{len .Lines}} chunks) <a href="/live-view?session={{.Name}}" style="font-size:11px;float:right">▶ Live stream</a></div>
  <div style="max-height:70vh;overflow-y:auto">
  {{range .Lines}}
  <div class="pane-line">
    <span class="pane-time">{{fmtTimeShort .Time}}</span>
    <span class="pane-content">{{.Content}}</span>
  </div>
  {{end}}
  </div>
</div>
{{else if .IsSubSession}}
<div class="section">
  <div class="section-hdr" style="color:#f59e0b">⚠ No direct pane output for this session</div>
  <div style="padding:12px;font-size:12px;color:#8b949e;border-bottom:1px solid #30363d">
    <strong style="color:#c9d1d9">{{.Name}}</strong> is a logical sub-session (polecat/agent) that runs
    inside a parent tmux session. Pane-log captures at the tmux level, not the logical sub-session level.
    Content below comes from <em>all active sessions</em> during this sub-session's lifetime.
  </div>
  {{if .ParentLines}}
  <div style="max-height:70vh;overflow-y:auto">
  {{range .ParentLines}}
  <div class="pane-line">
    <span class="pane-time">{{fmtTimeShort .Time}}</span>
    <span class="pane-sess"><a href="/session/{{.Session}}">{{.Session}}</a></span>
    <span class="pane-content">{{.Content}}</span>
  </div>
  {{end}}
  </div>
  {{else}}
  <p class="dim" style="padding:12px">No pane activity captured during this session's lifetime.</p>
  {{end}}
</div>
{{else}}
<div class="section">
  <div class="section-hdr">Pane transcript</div>
  <p class="dim" style="padding:12px">
    No pane output captured for this session in the current window.
    {{if .Session}}
    Pane-log is active for explicitly named tmux sessions (e.g. hq-mayor, fai-witness).
    If this session has no active pane-log, restart it with <code>GT_LOG_PANE_OUTPUT=true</code>.
    {{end}}
  </p>
</div>
{{end}}
{{end}}
`

// ── Tools ─────────────────────────────────────────────────────────────────────

const tmplTools = `
{{define "content"}}
<h1>Tool Calls</h1>
<div class="cards">
{{range $tool, $cnt := .ByTool}}
  <div class="card"><div class="val">{{$cnt}}</div><div class="lbl">{{$tool}}</div></div>
{{end}}
</div>
<div class="section">
<table>
<tr><th>Time</th><th>Session</th><th>Tool</th><th>Command</th><th>Duration</th><th>OK</th></tr>
{{range .Calls}}
<tr>
  <td class="dim">{{fmtTimeShort .Time}}</td>
  <td class="mono">{{truncate .SessionID 8}}</td>
  <td><span class="tag">{{.ToolName}}</span></td>
  <td style="max-width:500px">{{truncate .Command 120}}</td>
  <td class="dim">{{fmtDuration .DurationMs}}</td>
  <td>{{if .Success}}<span class="ok">✓</span>{{else}}<span class="err">✗</span>{{end}}</td>
</tr>
{{end}}
</table>
</div>
{{end}}
`

// ── Costs ─────────────────────────────────────────────────────────────────────

const tmplCosts = `
{{define "content"}}
<h1>API Costs</h1>
<div class="cards">
  <div class="card"><div class="val">{{fmtCost .Costs.TotalUSD}}</div><div class="lbl">Total cost</div></div>
  <div class="card"><div class="val">{{.Costs.RequestCount}}</div><div class="lbl">Requests</div></div>
  <div class="card"><div class="val">{{fmtTokens .Costs.TotalInput}}</div><div class="lbl">Input tokens</div></div>
  <div class="card"><div class="val">{{fmtTokens .Costs.TotalOutput}}</div><div class="lbl">Output tokens</div></div>
  <div class="card"><div class="val">{{fmtTokens .Costs.TotalCache}}</div><div class="lbl">Cache read</div></div>
</div>

<h2>Cost by Model</h2>
<div class="section">
<table>
<tr><th>Model</th><th>Cost</th><th></th></tr>
{{range $model, $cost := .Costs.ByModel}}
<tr>
  <td>{{$model}}</td>
  <td>{{fmtCost $cost}}</td>
</tr>
{{end}}
</table>
</div>

<h2>Cost by Claude Session (top 20)</h2>
<div class="section">
<table>
<tr><th>Session ID</th><th>Cost</th></tr>
{{range .BySession}}
<tr>
  <td class="mono">{{truncate .ID 36}}</td>
  <td>{{fmtCost .Cost}}</td>
</tr>
{{end}}
</table>
</div>

<h2>Recent Requests (last 50)</h2>
<div class="section">
<table>
<tr><th>Time</th><th>Session</th><th>Model</th><th>In</th><th>Out</th><th>Cache</th><th>Cost</th><th>Dur</th></tr>
{{range .Requests}}
<tr>
  <td class="dim">{{fmtTimeShort .Time}}</td>
  <td class="mono">{{truncate .SessionID 8}}</td>
  <td class="dim">{{truncate .Model 20}}</td>
  <td class="dim">{{fmtTokens .InputTokens}}</td>
  <td class="dim">{{fmtTokens .OutputTokens}}</td>
  <td class="dim">{{fmtTokens .CacheRead}}</td>
  <td>{{fmtCost .CostUSD}}</td>
  <td class="dim">{{fmtDuration .DurationMs}}</td>
</tr>
{{end}}
</table>
</div>
{{end}}
`

// ── Beads ─────────────────────────────────────────────────────────────────────

const tmplBeads = `
{{define "content"}}
<h1>Bead Lifecycle</h1>
<div class="cards">
  <div class="card"><div class="val">{{.Total}}</div><div class="lbl">Total beads</div></div>
  <div class="card"><div class="val ok">{{.Done}}</div><div class="lbl">Done</div></div>
  <div class="card"><div class="val warn">{{.InProg}}</div><div class="lbl">In progress</div></div>
  <div class="card"><div class="val dim">{{.Open}}</div><div class="lbl">Open</div></div>
  <div class="card"><div class="val">{{fmtDur .AvgTTS}}</div><div class="lbl">Avg time-to-start</div></div>
  <div class="card"><div class="val">{{fmtDur .AvgTTD}}</div><div class="lbl">Avg time-to-done</div></div>
</div>
<div class="section">
<table>
<tr><th>ID</th><th>Title</th><th>Type</th><th>State</th><th>Created by</th><th>Assignee</th><th>Created</th><th>TTS</th><th>TTD</th></tr>
{{range .Beads}}
<tr>
  <td class="mono"><a href="/bead/{{.ID}}">{{truncate .ID 10}}</a></td>
  <td style="max-width:300px"><a href="/bead/{{.ID}}">{{truncate .Title 60}}</a></td>
  <td class="dim">{{.Type}}</td>
  <td><span class="badge" style="background:{{stateColor .State}}">{{.State}}</span></td>
  <td class="dim">{{.CreatedBy}}</td>
  <td class="dim">{{if .Assignee}}<a href="/session/{{.Assignee}}">{{.Assignee}}</a>{{end}}</td>
  <td class="dim">{{fmtTimeShort .CreatedAt}}</td>
  <td class="dim">{{fmtDur .TimeToStart}}</td>
  <td class="dim">{{fmtDur .TotalTime}}</td>
</tr>
{{end}}
</table>
</div>
{{end}}
`

// ── Delegation ────────────────────────────────────────────────────────────────

const tmplDelegation = `
{{define "content"}}
<h1>Delegation Tree</h1>
<div class="section">
  <div class="section-hdr">Bead hierarchy (hook_bead links)</div>
  <div class="tree">
  {{range .Roots}}{{template "dtree" .}}{{end}}
  </div>
</div>

<h2>Recent BD Hooked calls</h2>
<div class="section">
<table>
<tr><th>Time</th><th>Actor</th><th>Session</th><th>Args</th><th>Status</th></tr>
{{range .Slings}}
<tr>
  <td class="dim">{{fmtTimeShort .Time}}</td>
  <td class="dim">{{.Actor}}</td>
  <td class="mono">{{truncate .GTSession 12}}</td>
  <td style="max-width:400px">{{truncate .Args 100}}</td>
  <td class="dim">{{.Status}}</td>
</tr>
{{end}}
</table>
</div>
{{end}}

{{define "dtree"}}
<div class="tree-node" style="border-left-color:{{if .DoneAt.IsZero}}#f59e0b{{else}}#56d364{{end}}">
  <a href="/bead/{{.BeadID}}" class="mono">{{truncate .BeadID 10}}</a>
  <span style="margin-left:6px">{{truncate .Title 60}}</span>
  {{if .Assignee}}<span class="tag" style="margin-left:6px">→ <a href="/session/{{.Assignee}}">{{.Assignee}}</a></span>{{end}}
  {{if not .DoneAt.IsZero}}<span class="ok" style="margin-left:6px;font-size:10px">✓</span>{{end}}
  {{range .Children}}{{template "dtree" .}}{{end}}
</div>
{{end}}
`

// ── Timeline ──────────────────────────────────────────────────────────────────

const tmplTimeline = `
{{define "content"}}
<h1>Session Timeline</h1>
<div class="section">
<div class="section-hdr">{{fmtTime .WinStart}} → {{fmtTime .WinEnd}}</div>
<div class="timeline-wrap" style="padding:8px 12px">
{{range .Sessions}}
<div class="tl-row">
  <div class="tl-label">
    <span class="badge" style="background:{{roleColor .Role}}">{{.Role}}</span>
    <a href="/session/{{.ID}}" style="margin-left:4px;font-size:11px">{{truncate .ID 12}}</a>
  </div>
  <div class="tl-bar-area">
    <div class="tl-bar" style="left:{{.LeftPct}}%;width:{{.WidthPct}}%;background:{{roleColor .Role}};min-width:4px">
      {{truncate .Topic 20}}
    </div>
  </div>
</div>
{{end}}
</div>
</div>
{{end}}
`

// ── Live view ─────────────────────────────────────────────────────────────────

const tmplLiveHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Live — %s</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:monospace;background:#0d1117;color:#c9d1d9;font-size:12px}
#log{padding:8px}
.line{padding:1px 0;border-bottom:1px solid #161b22;display:flex;gap:10px}
.t{color:#8b949e;min-width:70px}
.s{color:#58a6ff;min-width:100px}
.c{flex:1;white-space:pre-wrap;word-break:break-all}
</style>
</head>
<body>
<div id="log"></div>
<script>
var sess = %q;
var es = new EventSource('/live' + (sess ? '?session=' + encodeURIComponent(sess) : ''));
var log = document.getElementById('log');
es.onmessage = function(e) {
  var d = JSON.parse(e.data);
  var div = document.createElement('div');
  div.className = 'line';
  div.innerHTML = '<span class="t">'+d.time+'</span><span class="s">'+d.session+'</span><span class="c">'+escHtml(d.content)+'</span>';
  log.appendChild(div);
  window.scrollTo(0, document.body.scrollHeight);
};
function escHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
</script>
</body>
</html>
`

// ── Flow (unified event timeline) ─────────────────────────────────────────────

const tmplFlow = `
{{define "content"}}
<h1>Flow <span class="dim" style="font-size:12px;font-weight:400">{{.Count}} events</span></h1>

<form class="filters" method="GET">
  <input type="hidden" name="window" value="{{.Window}}">
  <label>Role
    <select name="role">
      <option value="">All roles</option>
      <option value="mayor" {{if eq .RoleFilter "mayor"}}selected{{end}}>mayor</option>
      <option value="deacon" {{if eq .RoleFilter "deacon"}}selected{{end}}>deacon</option>
      <option value="witness" {{if eq .RoleFilter "witness"}}selected{{end}}>witness</option>
      <option value="polecat" {{if eq .RoleFilter "polecat"}}selected{{end}}>polecat</option>
      <option value="refinery" {{if eq .RoleFilter "refinery"}}selected{{end}}>refinery</option>
      <option value="dog" {{if eq .RoleFilter "dog"}}selected{{end}}>dog</option>
      <option value="boot" {{if eq .RoleFilter "boot"}}selected{{end}}>boot</option>
    </select>
  </label>
  <label>Type
    <select name="kind">
      <option value="">All types</option>
      <option value="bd" {{if eq .KindFilter "bd"}}selected{{end}}>BD calls (bids)</option>
      <option value="session" {{if eq .KindFilter "session"}}selected{{end}}>Sessions</option>
      <option value="api" {{if eq .KindFilter "api"}}selected{{end}}>NLM/API</option>
      <option value="agent" {{if eq .KindFilter "agent"}}selected{{end}}>Agent (all)</option>
      <option value="agent_text" {{if eq .KindFilter "agent_text"}}selected{{end}}>Agent text</option>
      <option value="agent_tool_use" {{if eq .KindFilter "agent_tool_use"}}selected{{end}}>Agent tool_use</option>
      <option value="agent_tool_result" {{if eq .KindFilter "agent_tool_result"}}selected{{end}}>Agent tool_result</option>
    </select>
  </label>
  <label>Bead ID
    <input type="text" name="bead" value="{{.BeadFilter}}" placeholder="bead ID to trace" style="width:200px">
  </label>
  <button type="submit">Filter</button>
  {{if or .RoleFilter .KindFilter .BeadFilter}}<a href="/flow?window={{.Window}}" style="font-size:11px;color:#8b949e">clear</a>{{end}}
</form>

<div class="section">
<table>
<tr>
  <th style="width:70px">Time</th>
  <th style="width:60px">+Total</th>
  <th style="width:50px">+Step</th>
  <th style="width:110px">Type</th>
  <th style="width:80px">Role</th>
  <th style="width:120px">Actor / Session</th>
  <th style="width:90px">Bead</th>
  <th>Summary</th>
  <th style="width:60px">Dur</th>
</tr>
{{range .Events}}
<tr>
  <td class="dim">{{fmtTimeShort .Time}}</td>
  <td class="step-elapsed">+{{fmtDur .Elapsed}}</td>
  <td class="step-step">{{if .Step}}+{{fmtDur .Step}}{{end}}</td>
  <td><span class="tag {{.KindCSS}}">{{.KindLabel}}</span></td>
  <td>{{if .Role}}<span style="color:{{roleColor .Role}}">{{.Role}}</span>{{end}}</td>
  <td class="mono" style="font-size:10px;word-break:break-all">
    {{if .Actor}}{{if isUUID .Actor}}{{.Actor}}{{else}}<a href="/session/{{.Actor}}">{{.Actor}}</a>{{end}}{{end}}
  </td>
  <td class="mono" style="font-size:10px">{{if .BeadID}}<a href="/bead/{{.BeadID}}">{{.BeadID}}</a>{{end}}</td>
  <td>
    {{if .ShowInline}}
      {{$cls := ""}}
      {{if eq .Kind "agent_tool_use"}}{{$cls = "tool"}}{{end}}
      {{if eq .Kind "agent_tool_result"}}{{$cls = "result"}}{{end}}
      {{if eq .Kind "agent_thinking"}}{{$cls = "think"}}{{end}}
      {{if eq .Kind "sess_start"}}{{$cls = "prompt"}}{{end}}
      <div class="flow-inline{{if $cls}} {{$cls}}{{end}}">{{.Detail}}</div>
    {{else}}
      {{truncate .Summary 120}}
      {{if .CostUSD}}<span class="dim" style="font-size:10px">{{fmtCost .CostUSD}}</span>{{end}}
      {{if .Detail}}
      <details>
        <summary>detail</summary>
        <pre>{{.Detail}}</pre>
      </details>
      {{end}}
    {{end}}
  </td>
  <td class="dim">{{fmtDuration .DurMs}}</td>
</tr>
{{end}}
</table>
</div>
{{end}}
`

// ── Bead full detail ──────────────────────────────────────────────────────────

const tmplBeadFull = `
{{define "content"}}
<h1>Bead <span class="mono">{{.Bead.ID}}</span>
  <a href="/flow?bead={{.Bead.ID}}&window={{.Window}}" style="font-size:12px;margin-left:12px;font-weight:400">▶ Flow trace</a>
</h1>

<div class="cards">
  <div class="card"><div class="val"><span class="badge" style="background:{{stateColor .Bead.State}}">{{.Bead.State}}</span></div><div class="lbl">State</div></div>
  <div class="card"><div class="val">{{fmtDur .Bead.TimeToStart}}</div><div class="lbl">Time to start</div></div>
  <div class="card"><div class="val">{{fmtDur .Bead.TotalTime}}</div><div class="lbl">Total time</div></div>
</div>

<div class="section">
  <div class="section-hdr">Bead Details</div>
  <table class="bead-meta" style="width:auto">
    <tr><td>Title</td><td><strong>{{.Bead.Title}}</strong></td></tr>
    <tr><td>Type</td><td>{{.Bead.Type}}</td></tr>
    <tr><td>Created by</td><td><span style="color:{{roleColor .Bead.CreatedBy}}">{{.Bead.CreatedBy}}</span></td></tr>
    <tr><td>Assignee</td><td class="mono">{{if .Bead.Assignee}}<a href="/session/{{.Bead.Assignee}}">{{.Bead.Assignee}}</a>{{else}}—{{end}}</td></tr>
    {{if .Bead.HookBead}}<tr><td>Hook bead</td><td><a href="/bead/{{.Bead.HookBead}}">{{.Bead.HookBead}}</a></td></tr>{{end}}
    <tr><td>Created</td><td>{{fmtTime .Bead.CreatedAt}}</td></tr>
    {{if not .Bead.DoneAt.IsZero}}<tr><td>Done</td><td>{{fmtTime .Bead.DoneAt}}</td></tr>{{end}}
  </table>
</div>

{{if .Bead.Description}}
<div class="section">
  <div class="section-hdr">Description</div>
  <pre style="padding:12px;max-height:300px;overflow:auto">{{.Bead.Description}}</pre>
</div>
{{end}}

<div class="section">
  <div class="section-hdr">Lifecycle — timing from creation</div>
  <table>
    <tr><th>Time</th><th>+Total</th><th>Event</th><th>→ Status</th><th>→ Assignee</th><th>Actor</th><th>Args</th></tr>
    <tr>
      <td class="dim">{{fmtTimeShort .Bead.CreatedAt}}</td>
      <td class="step-elapsed">+0s</td>
      <td><span class="tag kind-create">bid.create</span></td>
      <td></td>
      <td></td>
      <td class="dim">{{.Bead.CreatedBy}}</td>
      <td>
        {{if .Bead.CreateArgs}}
        <details><summary>args</summary><pre>{{.Bead.CreateArgs}}</pre></details>
        {{end}}
      </td>
    </tr>
    {{range .Bead.Transitions}}
    <tr>
      <td class="dim">{{fmtTimeShort .Time}}</td>
      <td class="step-elapsed">+{{fmtDur .Elapsed}}</td>
      <td><span class="tag kind-update">bid.update</span></td>
      <td>{{if .Status}}<span class="badge" style="background:{{stateColor .Status}}">{{.Status}}</span>{{end}}</td>
      <td class="dim">{{.Assignee}}</td>
      <td class="dim">{{.Actor}}</td>
      <td>
        {{if .Args}}
        <details><summary>args</summary><pre>{{.Args}}</pre></details>
        {{end}}
      </td>
    </tr>
    {{end}}
  </table>
</div>

{{if .Bead.PaneChunks}}
<div class="section">
  <div class="section-hdr">Session activity — {{.Bead.Assignee}} (during bead lifetime)</div>
  <div style="max-height:500px;overflow-y:auto">
  {{range .Bead.PaneChunks}}
  <div class="pane-line">
    <span class="pane-time">{{fmtTimeShort .Time}}</span>
    <span class="pane-content">{{.Content}}</span>
  </div>
  {{end}}
  </div>
</div>
{{else}}
<p class="dim" style="margin-top:8px;font-size:12px">No pane output recorded for assignee "{{.Bead.Assignee}}" during bead lifetime.</p>
{{end}}
{{end}}
`
