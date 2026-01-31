# Current Decision Patterns in Gas Town

Research output for bead `hq-02e8e8.1`

## Executive Summary

Gas Town currently handles decisions through several mechanisms, but lacks a unified "decision point" abstraction. This document catalogs existing patterns to inform the design of a formal decision system.

**Key Finding**: The architecture deliberately avoids blocking waits. Agents are designed to execute autonomously or escalate, not pause for approval.

---

## 1. AskUserQuestion Usage

`AskUserQuestion` is a Claude Code tool for structured user input. In Gas Town context:

**Where it appears**:
- Role templates reference it for "Direction Decisions" (choosing between approaches)
- Not used in gt/bd CLI code directly - it's a Claude Code primitive

**Current usage pattern**:
- Agent needs human input on approach selection
- Agent invokes AskUserQuestion with 2-4 options
- Human selects; agent proceeds
- No record persisted in beads

**Gap identified**: Decisions made via AskUserQuestion are ephemeral - they exist only in the Claude Code session context, not in beads. If session cycles, decision context is lost.

---

## 2. How Agents Pause for Human Input

### The Propulsion Principle (No Pausing)

The Gas Town architecture explicitly avoids decision waits:

> "There is no approval step. When your work is done, you act - you don't wait."

**Design philosophy**: Agents either:
1. Execute autonomously based on hooked work
2. Escalate if truly blocked
3. Complete and handoff

There is no "pause and wait for human decision" primitive.

### Current Pause Mechanisms

| Mechanism | Purpose | How it works |
|-----------|---------|--------------|
| `gt hook` | Assign work for later | Bead marked `hooked`, agent finds on restart |
| `gt handoff` | Context preservation | Mail to self for next session |
| `gt mol await-signal` | Patrol wake | Timeout-based wait with backoff |
| `gt escalate` | Blocking issue | Creates escalation bead, routes by severity |

### Escalation (Primary Decision Request)

When an agent needs human input:

```bash
gt escalate --severity=medium --subject="Need architecture decision" \
  --body="Choosing between X and Y. Current analysis: ..."
```

**Escalation routing** (from `settings/escalation.json`):
- `low`: Creates bead only
- `medium`: Bead + mail to Mayor
- `high`: Bead + Mayor + email to human
- `critical`: Bead + Mayor + email + SMS

**What happens**: Human sees escalation, makes decision, then either:
- Replies with decision (mail or in-session)
- Updates the bead with resolution
- Slings resolved work back to agent

---

## 3. Blocked Work Handling

### Beads Dependency System

```go
// From internal/beads/beads.go
BlockedBy   []string `json:"blocked_by,omitempty"`
Blocks      []string `json:"blocks,omitempty"`
DependsOn   []string `json:"depends_on,omitempty"`
```

**Commands**:
- `bd blocked` - Show what's blocking me
- `bd dep add <A> <B>` - A depends on B (requirement semantics, not temporal)
- `bd ready` - Issues with no blockers

### Status Workflow

```
open → in_progress → done/closed
```

Plus pseudo-statuses in agent context:
- `hooked` - Assigned to agent's hook
- `pinned` - Permanent reference
- `spawning/working/stuck` - Agent lifecycle

### Blocking in Practice

When work is blocked:
1. Agent discovers blocker during execution
2. Agent creates dependency: `bd dep add <my-work> <blocker>`
3. Agent escalates if blocker needs human decision
4. Agent moves to other work or cycles

**Gap identified**: No formal "awaiting decision" status. Blocked work has dependencies but there's no distinction between "blocked by prerequisite work" and "blocked by human decision".

---

## 4. How Decisions Are Recorded

### Current Recording Mechanisms

| Mechanism | What it records | Persistence |
|-----------|-----------------|-------------|
| Bead comments (via description) | Text narrative | In `.beads/issues.jsonl` |
| Labels | Semantic metadata | On bead |
| Status transitions | Lifecycle events | Via git commits |
| Escalation beads | Formal decision requests | Dedicated bead type |
| Mail threads | Conversation history | In town beads |

### Escalation Beads (Most Formal)

```yaml
id: gt-esc-abc123
type: escalation
labels:
  - severity:high
  - source:plugin:rebuild-gt
  - acknowledged:false
  - reescalated:false
  - reescalation_count:0
```

**Lifecycle**:
1. Created: Agent escalates
2. Acknowledged: Human sees it (`gt escalate ack <id>`)
3. Resolved: Human decides (`gt escalate close <id> --reason="Chose X because..."`)

### The Capability Ledger

All work is tracked in beads as permanent audit trail:

> "Every completion is recorded. Every handoff is logged. Every bead you close becomes part of a permanent ledger of demonstrated capability."

**What's tracked**:
- Who did what work (assignee, closedBy)
- When (created, updated, closed timestamps)
- Outcome (status, any linked issues)

---

## 5. Gap Analysis

| Need | Current State | Gap |
|------|---------------|-----|
| Request decision | Escalation beads | Works but heavyweight |
| Track decision status | No "awaiting_decision" status | Can't distinguish from other blockers |
| Record decision | Escalation close reason | Not structured |
| Propagate decision | Manual - update dependents | No automatic propagation |
| Decision audit | In git history | Hard to query |

---

## 6. Observed Patterns (Desire Paths)

Patterns agents naturally try that don't quite work:

1. **Hooked mail as decisions**: Using `gt hook attach <mail-id>` to capture prose decisions. Works but no structure.

2. **Labels as decision markers**: Adding `decision:needed` or `blocked:decision` labels. Ad-hoc, inconsistent.

3. **Parent bead as decision context**: Creating child issues for "research options" then closing parent with decision in description.

4. **Escalation as question**: Using escalation for any decision request, even non-critical.

---

## 7. Current Decision Flow

```
Agent needs decision
        │
        ▼
┌───────────────────┐
│ Escalation Bead   │  (gt escalate)
│ severity: medium+ │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Mail to Mayor     │  (routed by severity)
│ or email to human │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Human sees        │  (checks escalations/mail)
│ escalation        │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Human closes      │  (gt escalate close --reason=)
│ with decision     │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Agent checks      │  (on next patrol cycle)
│ escalation status │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Agent proceeds    │  (reads reason, continues)
└───────────────────┘
```

**Problems with this flow**:
1. Heavyweight for simple decisions
2. No structured options/choices
3. Decision not linked to blocked work automatically
4. Agent must poll to discover resolution

---

## 8. Recommendations for Decision Bead Design

Based on this analysis, a formal decision system should:

1. **Lighter weight than escalation** - For "choose A or B" decisions
2. **Structured options** - Predefined choices, not just prose
3. **Status tracking** - `pending → resolved` with decision recorded
4. **Dependency integration** - Blocked work automatically tracks pending decisions
5. **Propagation mechanism** - Dependents notified when decision made
6. **Audit trail** - Who decided, when, rationale preserved

See `hq-02e8e8.2` (Design decision bead type) for proposed design.

---

## Data Sources

- Role templates: `internal/templates/roles/*.md.tmpl`
- Escalation system: `docs/design/escalation-system.md`
- Hook commands: `internal/cmd/hook.go`
- Await-signal: `internal/cmd/molecule_await_signal.go`
- Beads structure: `internal/beads/beads.go`
- Prior research: `.beads/research/claude-code-decision-catalog.md`
