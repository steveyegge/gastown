# Decision Bead Type Design

> Design for a lightweight decision tracking system in Gas Town.
> Written 2026-01-24, crew/decision_point session.
> Parent epic: hq-02e8e8

## Problem Statement

Current decision handling is ad-hoc:
- Agents use escalation for all decision requests (heavyweight)
- No structured way to present options
- Decisions made in Claude Code sessions are ephemeral
- No link between blocked work and pending decisions
- No decision audit trail

## Design Goals

1. **Lightweight**: Simpler than escalation for "choose A or B" decisions
2. **Structured options**: Predefined choices with descriptions
3. **Status tracking**: Pending → resolved lifecycle
4. **Dependency integration**: Blocked work automatically tracks pending decisions
5. **Propagation**: Dependents notified when decision made
6. **Audit trail**: Who decided, when, rationale preserved

---

## Architecture

### Decision as Bead Type

Decisions are beads with `type: decision` and structured fields.

```
┌─────────────────────────────────────────────────────────────┐
│                    gt decision request                      │
│  --question --options --context --blocker                   │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                   Decision Manager                          │
│  1. Create decision bead with options                       │
│  2. Link to blocking work                                   │
│  3. Route notification (mail to overseer)                   │
└─────────────────────┬───────────────────────────────────────┘
                      │
          ┌───────────┼───────────┐
          ▼           ▼           ▼
      ┌───────┐  ┌─────────┐  ┌─────────────┐
      │ Bead  │  │  Mail   │  │ Add Blocker │
      │Create │  │ Notify  │  │ Dependency  │
      └───────┘  └─────────┘  └─────────────┘
```

### Decision Flow

```
Agent needs decision
        │
        ▼
┌───────────────────┐
│ gt decision request│  Creates decision bead
│ --question "..."  │  with structured options
│ --options A,B,C   │
│ --blocker <work>  │  Links blocked work
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Decision bead     │  Status: pending
│ + Mail to human   │  Human sees in dashboard/mail
└────────┬──────────┘
         │
    ┌────┴────┐
    │ WAIT    │  Human reviews options
    └────┬────┘
         │
         ▼
┌───────────────────┐
│ gt decision resolve│  Human picks option
│ <id> --choice A   │  adds rationale
│ --rationale "..." │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Decision bead     │  Status: resolved
│ chosen_option: A  │  Blocker removed
│ resolved_at: now  │  Dependents unblocked
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Blocked work      │  Can now proceed
│ continues         │  Decision recorded
└───────────────────┘
```

---

## Data Model

### Decision Bead Schema

```go
// DecisionBead extends Issue with decision-specific fields.
// These are stored as structured data in the description or as
// a separate JSON blob in a well-known label pattern.

type DecisionFields struct {
    Question    string          `json:"question"`          // The decision to be made
    Context     string          `json:"context,omitempty"` // Background/analysis
    Options     []DecisionOption `json:"options"`          // Available choices
    ChosenIndex int             `json:"chosen_index,omitempty"` // Index of selected option (-1 if pending)
    Rationale   string          `json:"rationale,omitempty"`    // Why this choice was made
    ResolvedAt  string          `json:"resolved_at,omitempty"`  // ISO timestamp
    ResolvedBy  string          `json:"resolved_by,omitempty"`  // Who made the decision
    RequestedBy string          `json:"requested_by"`           // Agent that requested decision
    Blockers    []string        `json:"blockers,omitempty"`     // Work IDs blocked by this decision
}

type DecisionOption struct {
    Label       string `json:"label"`                 // Short label (e.g., "JWT tokens")
    Description string `json:"description,omitempty"` // Explanation of this choice
    Recommended bool   `json:"recommended,omitempty"` // Mark as recommended option
}
```

### Bead Format

```yaml
id: hq-dec-abc123
type: decision
status: pending  # or: resolved
priority: 2
title: "Auth approach: JWT vs Sessions?"
labels:
  - gt:decision
  - decision:pending        # or decision:resolved
  - requested_by:gastown/crew/decision_point
  - urgency:medium          # low, medium, high
description: |
  ## Question
  Which authentication approach should we use for the API?

  ## Context
  We need to add user authentication to the REST API. The frontend
  will be a SPA that makes API calls. We have ~1000 expected users.

  ## Options

  ### 1. JWT tokens (Recommended)
  Stateless, scalable, good for SPAs. Requires token refresh logic.

  ### 2. Session cookies
  Simpler, traditional approach. Requires session storage.

  ### 3. API keys
  Best for service-to-service. Not ideal for user auth.

  ---
  _Decision requested by: gastown/crew/decision_point_
  _Blocking: gt-work-xyz_
created_at: 2026-01-24T18:00:00Z
created_by: gastown/crew/decision_point
```

### Resolved Bead Format

```yaml
id: hq-dec-abc123
type: decision
status: resolved
priority: 2
title: "Auth approach: JWT vs Sessions? [RESOLVED: JWT tokens]"
labels:
  - gt:decision
  - decision:resolved
  - chosen:1                # Index of chosen option
  - resolved_by:human
description: |
  ## Question
  Which authentication approach should we use for the API?

  ## Context
  [... same as before ...]

  ## Options

  ### 1. JWT tokens (Recommended) ✓ CHOSEN
  Stateless, scalable, good for SPAs. Requires token refresh logic.

  ### 2. Session cookies
  Simpler, traditional approach. Requires session storage.

  ### 3. API keys
  Best for service-to-service. Not ideal for user auth.

  ---
  ## Resolution
  **Chosen:** JWT tokens
  **Rationale:** Aligns with our scaling goals and frontend architecture.
  We'll use short-lived access tokens with refresh token rotation.
  **Resolved by:** human
  **Resolved at:** 2026-01-24T19:30:00Z

  ---
  _Decision requested by: gastown/crew/decision_point_
created_at: 2026-01-24T18:00:00Z
closed_at: 2026-01-24T19:30:00Z
```

---

## Commands

### gt decision request

Create a new decision request.

```bash
gt decision request \
  --question "Which auth approach?" \
  --context "Building REST API with SPA frontend..." \
  --option "JWT tokens: Stateless, scalable" \
  --option "Sessions: Simpler, traditional" \
  --option "API keys: Service-to-service" \
  --recommend 1 \
  --blocker gt-work-xyz \
  --urgency medium
```

**Flags:**
- `--question` (required): The decision to be made
- `--context`: Background and analysis
- `--option` (repeatable): "Label: Description" format
- `--recommend N`: Mark option N as recommended (1-indexed)
- `--blocker ID`: Link to work blocked by this decision
- `--urgency`: low, medium, high (default: medium)
- `--json`: Output as JSON

**Output:**
```
📋 Decision requested: hq-dec-abc123
   Question: Which auth approach?
   Options: JWT tokens, Sessions, API keys
   Blocking: gt-work-xyz

→ Mailed human (overseer)
```

### gt decision list

List pending decisions.

```bash
gt decision list [--all] [--urgency=high] [--requestor=gastown/crew/x]
```

**Output:**
```
📋 Pending Decisions (2)

  ● hq-dec-abc123 [MEDIUM] Which auth approach?
    Requested by: gastown/crew/decision_point · Age: 2h
    Options: JWT tokens*, Sessions, API keys
    Blocking: gt-work-xyz

  ● hq-dec-def456 [HIGH] Database migration strategy?
    Requested by: gastown/crew/db_lead · Age: 30m
    Options: Blue-green*, Rolling, Big-bang
    Blocking: gt-work-abc, gt-work-def

(* = recommended)
```

### gt decision show

Show decision details.

```bash
gt decision show hq-dec-abc123
```

**Output:**
```
📋 Decision: hq-dec-abc123 [PENDING]

Question: Which auth approach?

Context:
  Building REST API with SPA frontend. ~1000 expected users.
  Need to balance simplicity with scalability.

Options:
  1. JWT tokens (Recommended)
     Stateless, scalable, good for SPAs. Requires token refresh logic.

  2. Session cookies
     Simpler, traditional approach. Requires session storage.

  3. API keys
     Best for service-to-service. Not ideal for user auth.

Requested by: gastown/crew/decision_point
Requested at: 2026-01-24T18:00:00Z
Urgency: medium
Blocking: gt-work-xyz

To resolve: gt decision resolve hq-dec-abc123 --choice 1 --rationale "..."
```

### gt decision resolve

Resolve a pending decision.

```bash
gt decision resolve hq-dec-abc123 \
  --choice 1 \
  --rationale "JWT aligns with our scaling goals and SPA architecture"
```

**Flags:**
- `--choice N` (required): Option number to select (1-indexed)
- `--rationale`: Explanation for the choice
- `--json`: Output as JSON

**Behavior:**
1. Updates decision bead to resolved status
2. Records chosen option, rationale, timestamp, resolver
3. Removes dependency from blocked work
4. Notifies requestor via mail

**Output:**
```
✓ Resolved hq-dec-abc123: JWT tokens
  Rationale: JWT aligns with our scaling goals and SPA architecture

→ Unblocked: gt-work-xyz
→ Mailed gastown/crew/decision_point
```

### gt decision dashboard

Show decision summary (for human review).

```bash
gt decision dashboard
```

**Output:**
```
📋 Decision Dashboard

Pending (3)
  ● [HIGH] hq-dec-def456: Database migration strategy? (30m)
  ● [MEDIUM] hq-dec-abc123: Which auth approach? (2h)
  ● [LOW] hq-dec-ghi789: Logging format preference? (1d)

Recently Resolved (5)
  ✓ hq-dec-jkl012: Error handling style → "Result types" (yesterday)
  ✓ hq-dec-mno345: Test framework → "Go testing" (2 days ago)
  ...

Stale (unresolved > 24h): 1
  ⚠ hq-dec-pqr678: Code style convention? (3d old)

Run 'gt decision list' for details
```

---

## Dependency Integration

### Automatic Blocking

When a decision is requested with `--blocker`, the system:

1. Creates decision bead
2. Adds blocked work to decision's `blocks` field
3. Adds decision to work's `blocked_by` field

```go
// When creating decision with blocker
decision.Blocks = []string{blockerID}
beads.AddDependency(blockerID, decision.ID)  // blockerID depends on decision
```

### Automatic Unblocking

When a decision is resolved:

1. Updates decision status to resolved
2. Removes decision from blocked work's `blocked_by`
3. Blocked work becomes ready (if no other blockers)

```go
// When resolving decision
beads.RemoveDependency(blockedID, decision.ID)
// Work is now unblocked (appears in bd ready)
```

### Query Integration

```bash
# Show decisions blocking my work
bd blocked gt-work-xyz   # Lists hq-dec-abc123 as blocker

# Show work blocked by a decision
bd show hq-dec-abc123    # Shows "Blocks: gt-work-xyz"

# Find all pending decisions for blocked work
gt decision list --blocking  # Shows decisions with active blockers
```

---

## Notification Flow

### On Request

```
Agent → gt decision request → Creates bead → Mail to overseer
                                         → Mail to requestor (confirmation)
```

### On Resolution

```
Human → gt decision resolve → Updates bead → Mail to requestor
                                          → Unblock dependents
                                          → (Optional) Nudge waiting agent
```

### Configuration

Decisions use the same routing as escalations but with `decision` severity:

```json
// settings/escalation.json
{
  "routes": {
    "decision": ["bead", "mail:human"]
  }
}
```

Or add decision-specific config:

```json
// settings/decisions.json
{
  "type": "decision",
  "version": 1,
  "notify_on_request": ["mail:human"],
  "notify_on_resolve": ["mail:requestor"],
  "stale_threshold": "24h",
  "stale_action": "mail:human"  // Remind human of stale decisions
}
```

---

## Audit Trail

All decision data is preserved in the bead:

| Event | Recorded In |
|-------|-------------|
| Request | `created_at`, `created_by` (requestor) |
| Options | Description (structured markdown) |
| Resolution | `closed_at`, labels (`chosen:N`, `resolved_by:X`) |
| Rationale | Description (appended resolution section) |
| Blocking | `blocks` field, dependency graph |

### Queryable History

```bash
# Find all decisions made in the last week
bd list --label=gt:decision --label=decision:resolved --since="7d"

# Find decisions by a specific resolver
bd list --label=resolved_by:human

# Find decisions for a specific area
bd list --label=gt:decision --label=area:auth
```

---

## Comparison: Decision vs Escalation

| Aspect | Decision | Escalation |
|--------|----------|------------|
| Purpose | Choose between options | Report problem/blocker |
| Structure | Question + options | Subject + body |
| Response | Select option | Acknowledge, close |
| Urgency | Low/Medium/High | Low/Medium/High/Critical |
| Routing | Mail to human | Severity-based (mail/email/SMS) |
| Re-escalation | Stale reminder | Severity bump |
| Typical use | "Which approach?" | "This is broken" |

**When to use which:**
- **Decision**: Agent needs to choose between valid options
- **Escalation**: Agent is stuck, blocked, or reporting a problem

---

## Implementation Plan

### Phase 1: Core Command (hq-02e8e8.3)

1. `gt decision request` - Create decision bead
2. `gt decision show` - Display decision details
3. Basic blocking integration

### Phase 2: Resolution Flow (hq-02e8e8.4)

1. `gt decision resolve` - Resolve with choice
2. Automatic unblocking
3. Notification on resolution

### Phase 3: Dashboard & Reports (hq-02e8e8.5)

1. `gt decision list` - List with filters
2. `gt decision dashboard` - Summary view
3. Stale decision detection

---

## File Structure

```
internal/
  cmd/
    decision.go           # gt decision command group
    decision_request.go   # gt decision request
    decision_resolve.go   # gt decision resolve
    decision_list.go      # gt decision list
    decision_show.go      # gt decision show
    decision_dashboard.go # gt decision dashboard
  decision/
    manager.go            # Decision lifecycle management
    types.go              # DecisionFields, DecisionOption
    formatter.go          # Markdown formatting for beads
```

---

## Open Questions

1. **Structured vs Markdown options?**

   Options: Store options as JSON in a label vs markdown in description.

   Recommendation: Markdown in description for human readability, with
   structured labels for machine parsing (`option:1:JWT tokens`).

2. **Decision bead prefix?**

   Options: `hq-dec-*` (dedicated) vs regular prefix with `gt:decision` label.

   Recommendation: Use regular prefix with label. Keeps ID namespace simpler.

3. **Stale threshold default?**

   Options: 24h, 48h, 1 week.

   Recommendation: 24h for high urgency, 48h for medium, 1 week for low.

---

## Future Enhancements

1. **Decision templates**: Pre-defined decision types (auth-approach, db-choice, etc.)
2. **Voting**: Multiple stakeholders vote on options
3. **Time-boxed decisions**: Auto-resolve with recommended if no response
4. **Decision dependencies**: One decision blocks another decision
5. **Integration with AskUserQuestion**: Claude Code tool creates decision bead
