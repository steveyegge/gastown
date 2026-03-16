# Gas City Formula Engine Design

**Wasteland Item:** w-gc-003
**Type:** Design (Epic)
**Priority:** P2
**Author:** gastown/crew/zhora (dreadpiraterobertz)
**Date:** 2026-03-15
**Related:** w-gc-001 (role format), w-gc-002 (routing), w-gc-004 (framework survey),
crew-specialization-design.md, formula-resolution.md, scheduler.md

## Problem

Gas Town has formulas (TOML workflow definitions) and a dispatch system
(capacity-controlled sling), but they operate in separate universes:

1. **Formulas don't know about capabilities.** A formula like `mol-polecat-work`
   dispatches to "a polecat" — it doesn't know or care what the polecat is good
   at. The `[formula.capabilities]` field in formula-resolution.md is declared
   but never consumed.

2. **Dispatch doesn't know about formulas.** The scheduler's `DispatchCycle`
   selects pending beads by readiness, not by matching task requirements to agent
   capabilities. A security audit bead gets dispatched to whichever polecat has
   capacity, even if a polecat with `semgrep` and OWASP context would be better.

3. **Role definitions are static.** The Gas City role format (w-gc-001) defines
   `[capability]` with handles/does_not_handle/examples, but nothing reads these
   fields for routing decisions. They're documentation, not dispatch signals.

4. **No formula composition for capability-aware workflows.** Formulas can
   `extend` other formulas and compose via `expand` rules, but there's no way to
   say "this step requires a security-specialist polecat" vs "this step needs a
   Go expert."

The formula engine bridges these gaps: it consumes capability profiles from role
definitions, matches them against task requirements declared in formulas, and
produces dispatch decisions that the scheduler executes.

## Non-Goals

- **Central planner.** Per crew-specialization-design.md, Gas Town uses
  distributed dispatch (GUPP), not optimal global assignment. The formula engine
  informs dispatch decisions; it doesn't override the pull model.
- **Real-time re-routing.** Once a polecat starts a molecule, it runs to
  completion or bounce. Mid-execution reassignment is not in scope.
- **Evidence system.** Track record accumulation (completions, bounces, reopened
  tasks) is w-gc-002 territory. The formula engine reads evidence signals but
  doesn't produce them.

## Design

### Architecture: Three Layers

```
┌──────────────────────────────────────────────────┐
│ LAYER 3: FORMULA EXECUTION                        │
│ Molecules, wisps, step progression, gate waits    │
│ (Existing: mol pour, mol step, patrol loops)      │
├──────────────────────────────────────────────────┤
│ LAYER 2: CAPABILITY-AWARE DISPATCH                │
│ Match task requirements → agent capabilities      │
│ (NEW: the formula engine proper)                  │
├──────────────────────────────────────────────────┤
│ LAYER 1: CAPACITY MANAGEMENT                      │
│ Concurrency control, sling contexts, circuit      │
│ breakers                                          │
│ (Existing: DispatchCycle, scheduler)              │
└──────────────────────────────────────────────────┘
```

Layer 2 is the missing piece. It sits between the existing capacity scheduler
(Layer 1) and the existing formula execution engine (Layer 3).

### 1. Task Requirements (Formula Side)

Formulas declare what capabilities a task needs. This extends the existing
formula TOML format:

```toml
formula = "security-audit"
type = "workflow"
version = 2

# NEW: Task requirements — what the executing agent needs
[requirements]
# Hard requirements — agent MUST have these capabilities
needs = ["security review", "code analysis"]

# Soft preferences — prefer agents with these, but don't block without
prefers = ["semgrep", "OWASP knowledge"]

# Cognition floor — minimum model tier for this formula
cognition = "standard"

# Context requirement — formula needs these docs available
context = ["docs/security-policy.md"]

[[steps]]
id = "scan"
title = "Run security scan"
# Per-step requirements (optional — overrides formula-level)
[steps.requirements]
needs = ["dependency scanning"]
cognition = "basic"
```

Per-step requirements allow multi-capability workflows. A security audit formula
might need a basic-tier dependency scanner for step 1 and a standard-tier code
reviewer for step 2. When a formula has `pour = true` (materialized steps), each
step can dispatch to a different polecat matched to its requirements.

### 2. Capability Index (Role Side)

At town startup (or on role definition change), the engine builds a capability
index from all loaded role definitions:

```go
// CapabilityIndex maps capability signals to roles that provide them.
type CapabilityIndex struct {
    // Handles maps natural-language capability descriptions to roles.
    // Key: normalized capability text. Value: list of roles with this handle.
    Handles map[string][]RoleMatch

    // Examples maps example task descriptions to roles.
    // Used for fuzzy matching when handles don't match directly.
    Examples map[string][]RoleMatch

    // Cognition maps tier names to roles at that tier or above.
    Cognition map[string][]RoleMatch

    // Tools maps tool names to roles that declare them.
    Tools map[string][]RoleMatch
}

type RoleMatch struct {
    Role       string  // Role identifier
    Confidence float64 // 0.0-1.0 match confidence
    Source     string  // "handle", "example", "tool", "track_record"
}
```

The index is built from:
- `[capability].handles` — direct capability matches (highest confidence)
- `[capability].example_tasks` — fuzzy task matching (medium confidence)
- `[capability].routing_examples` — proven past performance (highest confidence)
- `[execution].tools` — tool availability (used for hard requirements)
- `[execution].cognition` — model tier (used for floor enforcement)
- `[capability].does_not_handle` — negative matches (veto signal)

### 3. Match Algorithm

When dispatch is needed, the engine scores available agents against task
requirements:

```go
func (idx *CapabilityIndex) Match(req Requirements, available []Agent) []ScoredAgent {
    var scored []ScoredAgent

    for _, agent := range available {
        role := agent.RoleDefinition()
        score := Score{}

        // Hard gate: check negative matches first (veto)
        if idx.isVetoed(role, req) {
            continue
        }

        // Hard gate: cognition floor
        if !meetsMinCognition(role.Execution.Cognition, req.Cognition) {
            continue
        }

        // Hard gate: required tools
        if !hasRequiredTools(role.Execution.Tools, req.Needs) {
            continue
        }

        // Soft scoring: capability overlap
        score.CapabilityMatch = idx.scoreCapabilities(role, req)

        // Soft scoring: preference match
        score.PreferenceMatch = idx.scorePreferences(role, req)

        // Soft scoring: track record evidence (if available)
        score.TrackRecord = idx.scoreTrackRecord(role, req)

        scored = append(scored, ScoredAgent{Agent: agent, Score: score})
    }

    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score.Total() > scored[j].Score.Total()
    })

    return scored
}
```

**Scoring weights** (configurable per town):

| Signal | Weight | Rationale |
|--------|--------|-----------|
| Track record match | 0.40 | Proven performance is strongest signal |
| Capability handle match | 0.25 | Declared capability is strong intent |
| Example task similarity | 0.20 | Natural language proximity matters |
| Tool match | 0.10 | Binary: has it or doesn't |
| Preference match | 0.05 | Nice-to-have, shouldn't dominate |

**Fallback behavior:** If no agent scores above threshold (default 0.3), the
engine falls back to capacity-only dispatch (current behavior). This preserves
GUPP — work always moves forward, even without a perfect capability match.

### 4. Matching Strategy: Embedding vs. Keyword

The example_tasks and handles fields use natural language. How to match a task
description like "Users getting 403 on cross-origin API calls" against a role
that handles "CORS configuration and debugging"?

**Phase 1: Keyword + overlap scoring.** Extract significant tokens from both
sides, compute Jaccard similarity. Fast, deterministic, no external dependencies.
Good enough for the 80% case where task descriptions and capability declarations
share vocabulary.

```go
func tokenSimilarity(task, capability string) float64 {
    taskTokens := extractSignificantTokens(task)
    capTokens := extractSignificantTokens(capability)
    intersection := setIntersection(taskTokens, capTokens)
    union := setUnion(taskTokens, capTokens)
    return float64(len(intersection)) / float64(len(union))
}
```

**Phase 2: Embedding similarity (future).** Use a local embedding model (or
API) to compute semantic similarity. Catches cases where vocabulary doesn't
overlap but meaning does ("auth failures" matches "authentication debugging").
This is an optimization, not a requirement for v1.

**Phase 3: LLM-assisted routing (future, optional).** For truly ambiguous tasks,
ask a cheap model (Haiku) to pick the best match from a shortlist. This is the
"cognition as meta-capability" pattern from crew-specialization-design.md. Only
invoked when Phase 1/2 scores are below the ambiguity threshold.

### 5. Integration with Existing Dispatch

The formula engine integrates at the `QueryPending` and `Execute` stages of
`DispatchCycle`:

```go
// Before (capacity-only):
type DispatchCycle struct {
    QueryPending func() ([]PendingBead, error)
    Execute      func(PendingBead) error
    // ...
}

// After (capability-aware):
type DispatchCycle struct {
    QueryPending func() ([]PendingBead, error)
    ScoreAgents  func(PendingBead, []Agent) []ScoredAgent  // NEW
    Execute      func(PendingBead, *ScoredAgent) error      // Agent hint added
    // ...
}
```

When `ScoreAgents` is nil, the existing behavior is preserved (no capability
matching). When set, dispatch prefers the highest-scored agent for each bead.

The scheduler loop becomes:

```
1. Query pending beads
2. For each bead:
   a. Load its formula's [requirements]
   b. ScoreAgents(bead, availableAgents)
   c. If best score > threshold: dispatch to best agent
   d. If no agent scores above threshold: dispatch to any available (GUPP)
   e. If no agents available: defer (existing behavior)
```

### 6. Bouncing Protocol

When a dispatched agent cannot handle a task (capability mismatch despite
scoring), it bounces the work back:

```go
// Agent-side (in polecat formula execution):
func bounceTask(issue, reason, suggestedTarget string) {
    // 1. Update bead with bounce metadata
    bd.UpdateNotes(issue, fmt.Sprintf("BOUNCE: %s → %s", reason, suggestedTarget))

    // 2. Re-enqueue for dispatch (back to scheduler)
    gt.SlingBounce(issue, reason)
}
```

Bounces are learning signals:
- The formula engine records `(task_type, agent_role, bounce_reason)` triples
- Over time, these refine the capability index (anti-examples from real routing)
- Too many bounces from the same role → automatic `does_not_handle` suggestion

### 7. Formula Capabilities Declaration

The existing `[formula.capabilities]` in formula-resolution.md gets connected:

```toml
[formula.capabilities]
# What capabilities this formula exercises (used for agent routing)
primary = ["go", "testing", "code-review"]
secondary = ["git", "ci-cd"]
```

This serves two purposes:
1. **Dispatch matching** — when a bead references this formula, its capabilities
   become the task requirements for scoring
2. **Mol Mall discovery** — users browsing the formula registry can filter by
   capability

The `primary` list maps to hard requirements (`needs`), and `secondary` maps to
soft preferences (`prefers`).

### 8. Postings as Capability Signal

The postings system (issue #2818, rileywhite's analysis) provides an additional
capability signal. A posting is a behavioral overlay that augments a base role
with specialized context. The formula engine can read posting state:

```go
func agentEffectiveCapabilities(agent Agent) []string {
    caps := agent.RoleDefinition().Capability.Handles

    // If agent has an active posting, merge posting capabilities
    if posting := agent.ActivePosting(); posting != nil {
        caps = append(caps, posting.Capabilities...)
    }

    return caps
}
```

This means a generic crew member with a "security review" posting scores higher
for security tasks than a bare crew member, even without a permanent
security-lead role definition. Postings are temporary capability signals that
the formula engine consumes.

### 9. Configuration

Formula engine settings in `daemon.json`:

```json
{
  "formula_engine": {
    "enabled": true,
    "match_threshold": 0.3,
    "fallback_to_capacity": true,
    "scoring_weights": {
      "track_record": 0.40,
      "capability": 0.25,
      "example": 0.20,
      "tool": 0.10,
      "preference": 0.05
    },
    "bounce_limit": 3,
    "index_refresh_interval": "5m"
  }
}
```

When `enabled` is false, all dispatch uses capacity-only (current behavior).
This makes the migration opt-in and backward compatible.

## Implementation Plan

### Phase 1: Task Requirements in Formulas

| Step | File | Description | Effort |
|------|------|-------------|--------|
| 1 | `internal/formula/types.go` | Add `Requirements` struct to Formula | Small |
| 2 | `internal/formula/parser.go` | Parse `[requirements]` from TOML | Small |
| 3 | `internal/formula/parser_test.go` | Tests for requirements parsing | Small |
| 4 | Example formulas | Add `[requirements]` to 2-3 formulas | Small |

### Phase 2: Capability Index

| Step | File | Description | Effort |
|------|------|-------------|--------|
| 5 | `internal/gascity/capability_index.go` | CapabilityIndex + builder | Medium |
| 6 | `internal/gascity/capability_index_test.go` | Index tests | Medium |
| 7 | `internal/gascity/match.go` | Match algorithm + scoring | Medium |
| 8 | `internal/gascity/match_test.go` | Match tests with scoring scenarios | Medium |

### Phase 3: Dispatch Integration

| Step | File | Description | Effort |
|------|------|-------------|--------|
| 9 | `internal/scheduler/capacity/dispatch.go` | Add `ScoreAgents` to DispatchCycle | Medium |
| 10 | `internal/cmd/capacity_dispatch.go` | Wire scoring into existing dispatch | Medium |
| 11 | `internal/config/roles.go` | Expose capability fields from RoleDefinition | Small |
| 12 | Integration tests | End-to-end dispatch with capability matching | Large |

### Phase 4: Bouncing and Learning

| Step | File | Description | Effort |
|------|------|-------------|--------|
| 13 | `internal/gascity/bounce.go` | Bounce protocol + recording | Medium |
| 14 | `internal/cmd/sling.go` | `gt sling --bounce` for re-enqueue | Small |
| 15 | `internal/gascity/bounce_test.go` | Bounce learning tests | Medium |

### Phase 5: Advanced Matching (Future)

| Step | Description | Effort |
|------|-------------|--------|
| 16 | Embedding-based similarity for example matching | Large |
| 17 | LLM-assisted routing for ambiguous tasks | Medium |
| 18 | Automatic does_not_handle from bounce history | Medium |
| 19 | Posting integration for temporary capability signals | Small |

## Migration Path

1. **Phase 1 (opt-in):** `formula_engine.enabled = false` by default. Formulas
   can declare `[requirements]` but they're not consumed. Zero behavior change.

2. **Phase 2 (shadow mode):** Engine runs in shadow mode — scores agents and
   logs recommendations without affecting actual dispatch. Compare shadow
   dispatch decisions vs. actual dispatch outcomes to validate scoring.

3. **Phase 3 (assisted):** Engine provides ranked agent list. Scheduler prefers
   highest-scored agent when capacity is available, falls back to any available
   agent otherwise.

4. **Phase 4 (default):** Capability-aware dispatch becomes the default for
   formulas with `[requirements]`. Formulas without requirements use
   capacity-only dispatch (backward compatible).

## Relationship to Other Gas City Components

```
                    ┌──────────────────┐
                    │  w-gc-001        │
                    │  Role Format     │
                    │  (capability     │
                    │   declarations)  │
                    └────────┬─────────┘
                             │ reads
                    ┌────────▼─────────┐
                    │  w-gc-003        │
                    │  Formula Engine  │◄──── w-gc-004
                    │  (THIS DOC)      │      Framework Survey
                    │  (matching +     │      (borrowable patterns)
                    │   dispatch)      │
                    └────────┬─────────┘
                             │ informs
                    ┌────────▼─────────┐
                    │  w-gc-002        │
                    │  Role Routing    │
                    │  (evidence +     │
                    │   track records) │
                    └──────────────────┘
```

- **w-gc-001** provides the data (capability profiles in TOML)
- **w-gc-003** provides the logic (matching, scoring, dispatch decisions)
- **w-gc-002** provides the learning (evidence that refines capability profiles)
- **w-gc-004** provides design patterns borrowed from external frameworks

## Borrowed Patterns (from w-gc-004 Framework Survey)

| Pattern | Source | Application |
|---------|--------|-------------|
| Role/goal/backstory triad | CrewAI | Role definitions already have role + goal; backstory maps to context_docs |
| Conditional branching | LangGraph | Formula steps with per-step requirements enable branching to different agent types |
| Plugin model for tools | Microsoft Agent Framework | Tool declarations in role definitions follow the same registry pattern |
| Handoff protocol | OpenAI Agents SDK | Bouncing is a structured handoff with routing metadata |
| Structured observability | LangGraph/OpenAI | Score logging in shadow mode provides dispatch observability |

## Key Design Decision: Why Not LLM-First Routing?

AutoGen and CrewAI use LLM-based routing — an orchestrator LLM reads task
descriptions and picks the best agent. This is expensive and non-deterministic.

Gas Town's approach:
1. **Keyword matching first** (cheap, deterministic, fast)
2. **Track record lookup** (proven performance, no LLM needed)
3. **LLM only for ambiguity** (Phase 5, optional, Haiku-tier)

This matches the beads tier system philosophy: use the cheapest tool that works.
Cognition is the fallback, not the first resort.

## Open Questions

1. **Index refresh frequency.** Role definitions change rarely, but postings
   change per-session. Should the index rebuild on every dispatch cycle or on a
   timer?

2. **Cross-rig capability matching.** Can a formula engine on rig A see
   capabilities of agents on rig B? The scheduler already does cross-rig sling
   contexts — capability matching could follow the same path.

3. **Formula requirements inheritance.** If formula B extends formula A, should
   B inherit A's requirements? Or should requirements be explicit per formula?

4. **Scoring weight tuning.** The default weights (0.40/0.25/0.20/0.10/0.05)
   are educated guesses. Shadow mode data from Phase 2 should inform real values.

5. **Minimum viable slice.** The full engine is an epic. The 80/20 slice is
   probably: parse `[requirements]` + keyword matching against `handles` +
   fallback to capacity dispatch. No scoring weights, no bouncing, no embedding.
   This alone gets the right polecat for most tasks.
