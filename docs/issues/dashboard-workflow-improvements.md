# Dashboard Workflow Improvements Plan

**Reference**: [Gas Town Emergency User Manual](https://steveyegge.medium.com/gas-town-emergency-user-manual) by Steve Yegge

The current dashboard doesn't fully reflect the key Gas Town workflows described in the user manual. This plan addresses the gaps.

---

## Phase 1: Crew vs Polecat Distinction (High Priority)

### Problem
The dashboard shows a unified "Workers" panel that combines **Crew** (named, long-lived design workers) and **Polecats** (batch workers with persistent identity). The article emphasizes these are fundamentally different:

- **Crew**: "Personal concierges" for thought-intensive work, design, PR review, implementation plans
- **Polecats**: Batch workers for well-defined, spec'd-out bulk work (persistent identity, ephemeral sessions)

### Solution

#### 1.1 Split Workers Panel into Two Panels

**Crew Panel** (`👨‍💼 Crew`)
- Group by rig (blue for gastown, green for beads, etc.)
- Show each crew member's name (jack, joe, max, gus, george, dennis)
- Status indicators:
  - 🟢 **Spinning** - actively working
  - 🟡 **Has Questions** - waiting for human input
  - ✅ **Finished** - completed task, ready for review
  - ⏸️ **Ready** - idle, can accept new work
- Current hook/task if any
- Session status (attached/detached)
- Last activity timestamp

**Polecats Panel** (`🦨 Polecats`)
- Keep current polecat display
- Add convoy association
- Show session lifecycle (session ends after MR, identity persists)

#### 1.2 API Changes
- Add `/api/crew` endpoint returning crew status per rig
- Extend `gt status --json` to include crew state
- Add crew session detection (is Claude active?)

#### 1.3 Data Model
```go
type CrewStatus struct {
    Name        string `json:"name"`
    Rig         string `json:"rig"`
    State       string `json:"state"`       // spinning, questions, finished, ready
    Hook        string `json:"hook,omitempty"`
    HookTitle   string `json:"hook_title,omitempty"`
    Session     string `json:"session"`     // attached, detached, none
    LastActive  time.Time `json:"last_active"`
}
```

---

## Phase 2: Ready Work Panel (High Priority)

### Problem
The article mentions `gt ready` providing a "town-level view of ready work to assign to crew." No dashboard panel shows this.

### Solution

#### 2.1 New "Ready Work" Panel (`📋 Ready Work`)
- Display output of `gt ready --json`
- Group by source (town, each rig)
- Show priority badges (P0-P4)
- Quick-action: Click to sling to a crew member
- Filter by rig

#### 2.2 API Changes
- Add `/api/ready` endpoint wrapping `gt ready --json`
- Include refinery ready MRs (`gt refinery ready --json`)

#### 2.3 Dashboard Layout
Position prominently - this is the "inbox" of work to distribute.

---

## Phase 3: Crew Cycling Workflow (Medium Priority)

### Problem
The article describes a powerful workflow:
1. Give each crew member a task
2. Let them all spin
3. Cycle through to see "where your slot machines landed"
4. Act on each result (merge, send back, repurpose)

The dashboard doesn't support this "results review" workflow.

### Solution

#### 3.1 Crew Results View
New expandable view showing crew members who have finished:
- What they were working on
- Their summary/output (last message if detectable)
- Quick actions: Handoff, Review, Reassign

#### 3.2 Crew Notifications
Visual indicator when crew member finishes:
- Badge on Crew panel count
- Optional: Toast notification
- Sound alert option (configurable)

#### 3.3 Keyboard Navigation (Future)
- `j`/`k` to navigate crew
- `Enter` to attach to session
- Matches tmux `C-b n`/`C-b p` muscle memory

---

## Phase 4: PR Sheriff Workflow (Medium Priority)

### Problem
The article describes "PR Sheriff" - a crew member with standing orders to:
1. Check open PRs on session startup
2. Divide into "easy wins" vs "needs human review"
3. Sling easy wins to other crew
4. Flag complex ones for human

### Solution

#### 4.1 Enhance Merge Queue Panel
Add PR categorization:
- **Easy Wins** (green): CI pass, no conflicts, small diff, bot author
- **Needs Review** (yellow): Large changes, conflicts, failing CI
- Show which crew member is acting as Sheriff
- Show Sheriff's standing orders bead ID

#### 4.2 Sheriff Status Widget
Small indicator showing:
- Sheriff name and rig
- Standing orders bead (e.g., `bd-pr-sheriff`)
- Last patrol time
- PRs processed this session

#### 4.3 API Changes
- Add `/api/pr/categorize` to classify PRs
- Track Sheriff hook in status

---

## Phase 5: Per-Rig Crew Grouping (Low Priority)

### Problem
The article shows color-coded rig groups (blue=gastown, green=beads). Dashboard shows rigs but not their crew.

### Solution

#### 5.1 Expandable Rig Rows
Click rig row to expand and show:
- All crew members for that rig
- Their current status
- Quick-start buttons

#### 5.2 Rig Color Coding
Assign consistent colors to rigs:
- Configure in rig settings
- Apply to crew badges, session borders

---

## Phase 6: Town Cleanup Status (Low Priority)

### Problem
Article mentions "regular town cleanups" for stale beads, workers, processes.

### Solution

#### 6.1 Health Panel Enhancement
Add cleanup indicators:
- Stale beads count
- Orphaned processes
- Untracked files across rigs
- Last cleanup timestamp

#### 6.2 Cleanup Actions
Quick buttons:
- "Run Cleanup" (triggers Deacon plugin or manual command)
- Link to cleanup logs

---

## Implementation Order

1. **Phase 1.1**: Split Workers into Crew + Polecats panels
2. **Phase 2**: Ready Work panel
3. **Phase 1.2-1.3**: Crew API and state detection
4. **Phase 4.1**: PR categorization in Merge Queue
5. **Phase 3**: Crew results/notification system
6. **Phase 5**: Rig expansion with crew
7. **Phase 4.2-4.3**: Sheriff workflow
8. **Phase 6**: Cleanup status

---

## Files to Modify

### Backend (Go)
- `internal/web/api.go` - New endpoints
- `internal/web/handler.go` - Template data
- `internal/crew/manager.go` - Crew status queries
- `internal/cmd/status.go` - Extend JSON output

### Frontend
- `internal/web/templates/convoy.html` - New panels
- `internal/web/static/dashboard.css` - Crew styling
- `internal/web/static/dashboard.js` - Crew interactions

### New Files
- `internal/web/crew.go` - Crew status aggregation
- `internal/web/ready.go` - Ready work aggregation

---

## Success Metrics

1. Dashboard clearly shows crew vs polecat distinction
2. Human can see ready work without running `gt ready`
3. Human can see which crew are finished and need review
4. PR Sheriff workflow is visible
5. Per-rig crew is browsable from dashboard

---

## Related Issues

- Consider adding `gt crew status` CLI command
- Consider crew state persistence for cross-session tracking
- Consider webhook/SSE for real-time crew status updates
