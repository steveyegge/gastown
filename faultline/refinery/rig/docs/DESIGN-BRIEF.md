# Faultline Design Brief

> Visual identity, UI/UX direction, and terminology for the faultline dashboard.

## Design Philosophy

**Agent-first, human-approachable.**

Faultline is a coding agent's error tracker that humans can also use. The REST API is the primary product — polecats consume it to diagnose and fix bugs autonomously. The dashboard is a thin HTML layer on the same API, designed so a product manager can understand what's happening without touching a terminal.

Two modes in one product:
- **Overview mode (PM-friendly):** Clean, scannable, visual hierarchy. What's breaking, how bad, is it getting better?
- **Detail mode (developer tool):** High-density, code-oriented, keyboard-navigable. Stack traces, raw JSON, bead status, polecat progress.

## Brand Identity

### Name & Theme

**faultline** — geological. Tectonic energy under a calm surface. Errors are seismic events; the system absorbs and responds to them.

The geology theme extends into the product vocabulary, severity system, and visual language without being cartoonish. It should feel like a natural metaphor, not a gimmick.

### Color Palette

Geological strata — layers of earth with sharp signal colors for seismic activity.

| Role | Color | Hex | Usage |
|------|-------|-----|-------|
| **Surface** | Warm white / limestone | `#FAF8F5` | Page background |
| **Bedrock** | Dark slate | `#1E293B` | Headers, nav, code blocks |
| **Sediment** | Warm gray | `#64748B` | Secondary text, borders |
| **Sandstone** | Tan | `#D4A574` | Accent, hover states |
| **Tremor** | Amber | `#F59E0B` | Warnings, elevated activity |
| **Rupture** | Seismic red | `#DC2626` | Errors, critical severity |
| **Dormant** | Sage green | `#059669` | Resolved, stable |
| **Magma** | Deep orange | `#EA580C` | Fatal, system-critical |
| **Obsidian** | Near-black | `#0F172A` | Terminal/code backgrounds |

### Typography

- **Headings / UI:** Inter or system sans-serif — clean, approachable, professional
- **Code / Data:** JetBrains Mono or system monospace — technical, readable at small sizes
- **Rule:** Sans-serif for navigation and prose, monospace for anything a developer would copy-paste

### Logo Direction

The word "faultline" in lowercase, with a subtle fracture/offset in the letterforms — as if the text itself has shifted along a fault. Monochrome primary, seismic red accent mark at the fracture point. Should work at favicon size.

## Seismic Severity Scale

Instead of Sentry's generic "error/warning/info", faultline uses geological severity terms. These map to the same underlying levels but give the product character.

| Severity | Sentry Level | Icon | Description | Bead trigger? |
|----------|-------------|------|-------------|---------------|
| **Tremor** | `warning` | 〰️ | Felt but no damage. Something's off but not breaking. | No |
| **Quake** | `error` | ⚡ | Real shaking. Users are affected, things are breaking. | Yes (3+ in 5min) |
| **Rupture** | `fatal` | 🔴 | Ground split open. Process crashed, service down. | Yes (immediate) |
| **Aftershock** | (regressed) | 🔁 | It came back. Same fault reactivated within 24h. | Reopens bead |
| **Dormant** | (resolved) | ✅ | Quiet. No activity on this fault since resolution. | — |

### Issue Statuses (geological lifecycle)

| Status | Internal | Meaning | Dashboard label |
|--------|----------|---------|----------------|
| **Active** | `unresolved` | Fault is moving. Events accumulating. | "Active fault" |
| **Fixing** | `in_progress` | Polecat dispatched, working on it. | "Repair crew deployed" |
| **Stabilized** | `resolved` | Fix merged, quiet period confirmed. | "Stabilized" |
| **Aftershock** | `regressed` | Same fingerprint returned within 24h. | "Aftershock detected" |
| **Escalated** | `needs_human` | Polecat couldn't fix it. Needs human. | "Manual inspection needed" |
| **Dormant** | `archived` | No activity for extended period. | "Dormant" |

### Event Count Language

Instead of raw numbers, use seismic magnitude language for at-a-glance severity:

| Events | Magnitude | Display |
|--------|-----------|---------|
| 1-2 | Micro | "Micro (2 events)" |
| 3-10 | Minor | "Minor (7 events)" |
| 11-50 | Moderate | "Moderate (34 events)" |
| 51-200 | Strong | "Strong (128 events)" |
| 201-1000 | Major | "Major (567 events)" |
| 1000+ | Great | "Great (2,341 events)" |

## Dashboard Screens

### 1. Seismograph (Issue List)

The main view. A PM opens this and immediately knows the state of things.

**Layout:**
- Top bar: project selector, time range filter, status filter tabs (Active / Fixing / All)
- Issue cards in a list, sorted by last seen (default) or event count
- Each card shows: severity icon, title (exception type: message), culprit, magnitude, first/last seen, status badge
- "Fixing" cards show a subtle pulsing indicator — the unique faultline moment

**Interactions:**
- Click card → issue detail
- Keyboard: `j/k` navigate, `enter` opens, `r` resolve, `i` ignore
- Filter bar supports typed queries: `level:fatal project:myapp status:active`

### 2. Fault Report (Issue Detail)

The drill-down. PM sees the summary, developer sees the stack trace.

**Layout — two zones:**

**Upper zone (approachable):**
- Title: "RuntimeError: connection refused"
- Severity badge + magnitude + trend arrow (↑ increasing, → stable, ↓ decreasing)
- Timeline sparkline: events over last 24h/7d
- Status badge with context: "Fixing — polecat obsidian dispatched 3 min ago"
- Bead link if exists: "fl-abc → view in Gas Town"

**Lower zone (technical):**
- Stack trace with syntax-highlighted code context
- Tags/contexts (OS, browser, release, environment)
- Breadcrumbs (if present in event)
- "Sample event" expandable raw JSON
- Event list: recent events for this group, click to view individual

### 3. Core Sample (Event Detail)

Full event data. Named after geological core sampling — extracting a cross-section for analysis.

**Layout:**
- Exception info: type, value, mechanism
- Full stack trace with expandable frames
- Each frame: filename, line, function, code context (if available)
- Context panels: device, OS, browser, runtime, user (if present)
- Raw JSON toggle: full event payload for agents/developers to copy

### 4. Seismic Monitor (Project Overview — P5)

Deferred to P5. High-level view with charts:
- Error rate over time
- New vs resolved groups
- Polecat resolution rate
- Top active faults

## Component Patterns

### Issue Card
```
┌──────────────────────────────────────────────────┐
│ ⚡ RuntimeError: connection refused    Moderate │
│   app.db.connect                    34 events   │
│   First: 2h ago · Last: 3 min ago               │
│   ████████░░ Active fault                        │
└──────────────────────────────────────────────────┘
```

### Issue Card (with polecat)
```
┌──────────────────────────────────────────────────┐
│ ⚡ ValueError: invalid token          Strong    │
│   auth.middleware.verify            128 events   │
│   First: 1h ago · Last: 30s ago                  │
│   ◉ Repair crew deployed (polecat: obsidian)     │
└──────────────────────────────────────────────────┘
```

### Stack Frame
```
  app/db/connection.go:47 in connect
  ─────────────────────────────────────
  45 │   pool, err := sql.Open("mysql", dsn)
  46 │   if err != nil {
  47 →│       return nil, fmt.Errorf("connect: %w", err)
  48 │   }
  49 │   pool.SetMaxOpenConns(maxConns)
```

## Tech Stack

- **templ** — Go HTML templating with type safety
- **htmx** — interactivity without JS framework (filtering, pagination, live updates)
- **Embedded assets** — `//go:embed` for CSS/JS, single binary deployment
- **Content negotiation** — same handlers serve JSON (agents) and HTML (humans)
- No Node.js, no npm, no build step beyond `go build`

## Design Principles

1. **Data density over whitespace.** Error triage is about scanning fast. Don't waste vertical space.
2. **Progressive disclosure.** PM sees the summary. Engineer clicks for depth. Agent gets JSON.
3. **The agentic loop is visible.** "Fixing" status with polecat name is the hero feature. Make it prominent.
4. **Geological metaphor is flavor, not friction.** Terms should be intuitive. "Quake" is obviously worse than "Tremor." Don't make users learn a dictionary.
5. **Keyboard-first for developers.** Every action has a shortcut. Tab navigation works.
6. **Auth from the start.** Account logins, project creation, role-based access. Not an afterthought.

## Account & Access Model

### Accounts
- Email/password login (local accounts)
- OAuth/OIDC support planned (GitHub, Google) for team onboarding
- API tokens for agent/CI access (scoped per project or org)

### Organizations & Projects
- **Organization** — top-level tenant. Contains projects, members, settings.
- **Project** — maps to a Sentry DSN. Has its own event stream, issue groups, and settings.
- Users create projects via dashboard. Each project gets a DSN (public key auto-generated).
- Project settings: target rig (for Gas Town integration), severity thresholds, retention policy.

### Roles

| Role | Scope | Permissions |
|------|-------|-------------|
| **Owner** | Org | Full access. Manage members, billing, delete org. |
| **Admin** | Org | Manage projects, members, settings. Cannot delete org. |
| **Member** | Org | View all projects, resolve/ignore issues, view events. |
| **Viewer** | Project | Read-only access to a specific project. |
| **Agent** | Project | API-only. Used by polecats and CI. Can read issues, resolve, create beads. |

### Implementation phases
- **P4 (Dashboard):** Local accounts, project CRUD, member invite, role assignment
- **P5 (Hardening):** OAuth/OIDC, API tokens, audit log
- **P6 (Advanced):** SSO/SAML (enterprise), fine-grained permissions

## Integrations

### Slack Plugin (planned)

Channel-level notifications for error activity:
- New issue group detected (with severity + magnitude)
- Issue resolved / regressed
- Polecat dispatched / completed / escalated
- Daily/weekly digest of error activity

Slash commands:
- `/faultline status` — top active faults across projects
- `/faultline resolve <issue>` — mark resolved from Slack
- `/faultline mute <issue> <duration>` — suppress notifications

Bot presence:
- Threads per issue group — conversation + polecat updates in one place
- React with 👀 to claim, ✅ to resolve

**Implementation:** P5 or P6. Slack app with webhook + slash command endpoints in the faultline server.
