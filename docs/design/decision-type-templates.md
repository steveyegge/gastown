# Decision Type Templates Design

> Design for structured context requirements per decision type
> Part of epic: bd-epc-slack_decision_infrastructure

## Overview

Different decisions need different context. An architecture decision needs tradeoffs analysis; a debugging decision needs repro steps. This feature enforces appropriate context structure per decision type.

## Decision Types

### 1. `architecture` - Technical Design Decisions

**When to use:** Choosing between technical approaches, libraries, patterns, or system designs.

**Required sections:**
- `problem` - What problem are we solving?
- `constraints` - What are the technical/business constraints?
- `alternatives_considered` - What options were evaluated?

**Optional sections:**
- `tradeoffs` - Pros/cons of each alternative
- `recommendation` - Which option and why (if you have one)

**Example:**
```bash
gt decision request --type architecture \
  --prompt "Which caching strategy for the API layer?" \
  --option "Redis: Distributed caching with persistence" \
  --option "In-memory: Simple per-process cache" \
  --option "CDN: Edge caching for static responses" \
  --context '
## Problem
API response times averaging 200ms, target is <50ms. Cache hit analysis shows 80% of requests hit same 5 endpoints.

## Constraints
- Must work with our Kubernetes multi-pod deployment
- Budget: No new managed services (use existing Redis cluster OK)
- Timeline: Ship within current sprint

## Alternatives Considered
1. **Redis** - Already have cluster, handles multi-pod, adds ~5ms latency
2. **In-memory** - Zero latency but no sharing between pods, cold start issues
3. **CDN** - Only works for truly static content, our responses are user-specific
'
```

**Slack Display:**
```
📐 Architecture Decision: Which caching strategy for the API layer?

**Problem**
API response times averaging 200ms, target is <50ms...

**Constraints**
• Must work with Kubernetes multi-pod deployment
• Budget: No new managed services
• Timeline: Ship within current sprint

**Alternatives Considered**
1. Redis - Already have cluster, handles multi-pod...
2. In-memory - Zero latency but no sharing...
3. CDN - Only works for truly static content...

[Option buttons: Redis | In-memory | CDN]
```

---

### 2. `debugging` - Investigation Decisions

**When to use:** Deciding how to investigate or fix a bug/issue.

**Required sections:**
- `symptoms` - What's happening? Error messages, behavior
- `attempted` - What was already tried?
- `hypothesis` - Current theory about root cause

**Optional sections:**
- `repro_steps` - How to reproduce
- `affected` - What's impacted (users, systems)

**Example:**
```bash
gt decision request --type debugging \
  --prompt "API returns 500 on user profile updates - how to proceed?" \
  --option "Database investigation: Check for lock contention or deadlocks" \
  --option "Code review: Audit recent changes to profile handler" \
  --option "Add instrumentation: Deploy with detailed logging first" \
  --option "File bug: Create tracking bead for deeper investigation" \
  --context '
## Symptoms
- HTTP 500 errors on PUT /api/users/{id}/profile
- Started ~2 hours ago (after deploy at 14:00)
- Affects ~15% of requests, seemingly random users
- Error log: "database/sql: connection reset by peer"

## Attempted
- Restarted API pods (no change)
- Checked DB metrics (CPU/memory normal)
- Verified DB credentials (valid)

## Hypothesis
Likely connection pool exhaustion or timeout issue. The deploy included a change to increase concurrent request handling, may be exceeding DB connection limits.
'
```

---

### 3. `feature` - Product/Feature Decisions

**When to use:** Deciding on feature scope, requirements, or approach.

**Required sections:**
- `goal` - What user problem are we solving?
- `scope` - What's in/out of scope?
- `acceptance` - How do we know it's done?

**Optional sections:**
- `dependencies` - What needs to happen first?
- `risks` - What could go wrong?

**Example:**
```bash
gt decision request --type feature \
  --prompt "How should we implement the notification preferences feature?" \
  --option "Full settings page: Dedicated page with all notification options" \
  --option "Inline toggles: Add toggles directly in notification dropdown" \
  --option "Minimal MVP: Email-only preferences, expand later" \
  --context '
## Goal
Users want control over notification frequency. Current state: all-or-nothing. Top support request last month.

## Scope
**In scope:**
- Email notification preferences (immediate, daily digest, weekly, off)
- Push notification on/off toggle
- Per-category settings (mentions, replies, system)

**Out of scope:**
- Slack/Discord integrations (future phase)
- Notification scheduling (specific hours)
- Per-project preferences

## Acceptance
- [ ] User can access preferences from profile menu
- [ ] Changes persist and take effect within 1 minute
- [ ] Default: current behavior (all notifications on)
- [ ] Mobile-responsive UI
'
```

---

### 4. `process` - Workflow/Process Decisions

**When to use:** Deciding on team processes, workflows, or operational changes.

**Required sections:**
- `current_state` - How does it work today?
- `proposed_change` - What's the new approach?
- `impact` - Who/what is affected?

**Optional sections:**
- `rollback` - How to revert if needed?
- `timeline` - When would this take effect?

**Example:**
```bash
gt decision request --type process \
  --prompt "Should we require PR reviews for documentation changes?" \
  --option "Require reviews: All doc changes need 1 approval" \
  --option "Auto-merge: Docs auto-merge after CI passes" \
  --option "Hybrid: Require reviews only for user-facing docs" \
  --context '
## Current State
Documentation changes follow same PR process as code: 1 required reviewer, CI must pass. Average review time: 4 hours.

## Proposed Change
Option to allow docs to auto-merge after CI (spell check, link validation) passes, skipping human review.

## Impact
- **Positive:** Faster doc updates, lower reviewer burden
- **Negative:** Risk of incorrect/unclear docs shipping faster
- **Affected:** All contributors, docs team, end users

## Rollback
Can revert branch protection rules in GitHub settings within minutes.
'
```

---

### 5. `prioritization` - What to Work On Decisions

**When to use:** Choosing between multiple possible work items or directions.

**Required sections:**
- `options_analysis` - Brief analysis of each option
- `criteria` - What factors matter for this decision?

**Optional sections:**
- `blockers` - What's blocked waiting on this?
- `deadline` - Any time constraints?

**Example:**
```bash
gt decision request --type prioritization \
  --prompt "What should the team focus on this sprint?" \
  --option "Performance: Address p95 latency regression" \
  --option "Features: Complete OAuth integration" \
  --option "Tech debt: Migrate to new logging framework" \
  --context '
## Options Analysis
1. **Performance** - p95 latency up 40% since last release. Customer complaints starting. ~3 days to fix.
2. **OAuth** - Blocking enterprise sales. 80% complete, ~2 days remaining. Sales demo on Friday.
3. **Tech debt** - Old logging framework EOL in 60 days. ~5 days work. No immediate user impact.

## Criteria
- Customer impact (high weight)
- Revenue impact (high weight)
- Technical risk (medium weight)
- Team morale (low weight)

## Blockers
- Enterprise pilot blocked on OAuth
- SRE on-call load increased due to perf issues
'
```

---

## Implementation Plan

### Phase 1: Type Flag and Validation

1. **Add `--type` flag to `gt decision request`**
   - File: `internal/cmd/decision.go`
   - Valid types: `architecture`, `debugging`, `feature`, `process`, `prioritization`, `general`
   - Default: `general` (no required sections, backward compatible)

2. **Create template definitions**
   - File: `internal/decision/templates.go` (new)
   - Define required/optional sections per type
   - Include help text for each section

3. **Add context validator**
   - File: `internal/validator/builtin/builtin.go`
   - Check for required sections based on type
   - Return helpful error messages with section guidance

### Phase 2: Slack Display Enhancement

4. **Update formatContextForSlack**
   - File: `internal/slackbot/bot.go`
   - Detect markdown sections (`## Heading`)
   - Render as mrkdwn with bold headings
   - Preserve code blocks and lists

5. **Add type indicator to Slack message**
   - Show emoji + type label: `📐 Architecture Decision`
   - Different emoji per type

### Phase 3: Guidance and Discoverability

6. **Interactive template prompts**
   - When `--type` specified without `--context`, show template
   - `gt decision request --type architecture` prints template to fill in

7. **Help text improvements**
   - `gt decision types` - list available types with descriptions
   - `gt decision template <type>` - show template for a type

---

## Type Definitions (for implementation)

```go
type DecisionTemplate struct {
    Type        string
    Emoji       string
    Label       string
    Description string
    Required    []SectionDef
    Optional    []SectionDef
}

type SectionDef struct {
    Name        string  // e.g., "problem"
    Heading     string  // e.g., "## Problem"
    Description string  // Help text for what goes here
}

var Templates = map[string]DecisionTemplate{
    "architecture": {
        Type:        "architecture",
        Emoji:       "📐",
        Label:       "Architecture Decision",
        Description: "Technical design choices between approaches, libraries, or patterns",
        Required: []SectionDef{
            {Name: "problem", Heading: "## Problem", Description: "What problem are we solving?"},
            {Name: "constraints", Heading: "## Constraints", Description: "Technical/business constraints"},
            {Name: "alternatives_considered", Heading: "## Alternatives Considered", Description: "Options evaluated"},
        },
        Optional: []SectionDef{
            {Name: "tradeoffs", Heading: "## Tradeoffs", Description: "Pros/cons of each alternative"},
            {Name: "recommendation", Heading: "## Recommendation", Description: "Preferred option and why"},
        },
    },
    "debugging": {
        Type:        "debugging",
        Emoji:       "🔍",
        Label:       "Debugging Decision",
        Description: "Investigation approach for bugs or issues",
        Required: []SectionDef{
            {Name: "symptoms", Heading: "## Symptoms", Description: "What's happening? Errors, behavior"},
            {Name: "attempted", Heading: "## Attempted", Description: "What was already tried?"},
            {Name: "hypothesis", Heading: "## Hypothesis", Description: "Current theory about root cause"},
        },
        Optional: []SectionDef{
            {Name: "repro_steps", Heading: "## Repro Steps", Description: "How to reproduce"},
            {Name: "affected", Heading: "## Affected", Description: "Users/systems impacted"},
        },
    },
    // ... other types
}
```

---

## Validation Logic

```go
func validateDecisionContext(decisionType, context string) ValidationResult {
    template, ok := Templates[decisionType]
    if !ok || decisionType == "general" {
        return ValidationResult{Valid: true} // No validation for general type
    }

    // Check for required sections
    var missing []string
    for _, section := range template.Required {
        if !strings.Contains(context, section.Heading) {
            missing = append(missing, section.Name)
        }
    }

    if len(missing) > 0 {
        return ValidationResult{
            Valid:    false,
            Blocking: true,
            Errors: []string{
                fmt.Sprintf("%s decision requires sections: %s", template.Label, strings.Join(missing, ", ")),
                fmt.Sprintf("Use 'gt decision template %s' to see the template", decisionType),
            },
        }
    }

    return ValidationResult{Valid: true}
}
```

---

## Migration Path

1. **Phase 1:** Add `--type` flag, default to `general` (no breaking changes)
2. **Phase 2:** Add Slack formatting (visual improvement, no behavior change)
3. **Phase 3:** Add `gt decision template` command for discoverability
4. **Phase 4:** Consider making type required (with deprecation warning first)

---

## Success Metrics

- Decisions with structured context increase from ~20% to >80%
- Human resolution time decreases (faster understanding)
- Fewer back-and-forth clarification requests
- Template usage tracked via decision type field
