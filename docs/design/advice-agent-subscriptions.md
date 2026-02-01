# Advice Agent Subscriptions

> Custom advice subscriptions stored in agent beads

## Status: DESIGN

**Related:** beads/docs/design/advice-subscription-model-v2.md (Phase 3)

## Overview

Agents can customize which advice they receive by storing subscription preferences
in their agent bead. This extends the auto-subscription model (global, role, rig, agent)
with explicit opt-in/opt-out control.

## Problem

The auto-subscription model works well for standard advice, but:

1. **No opt-out**: A crew member can't exclude `role:crew` advice they find unhelpful
2. **No custom topics**: Agents can't subscribe to cross-cutting topics like `testing` or `security`
3. **No personalization**: All agents of the same role see identical advice

## Design: Agent Bead Subscriptions

### Storage Location

Subscriptions are stored in the agent bead's description as structured fields:

```yaml
# Agent bead description (parsed by ParseAgentFields)
role_type: crew
rig: gastown
agent_state: working
hook_bead: gt-abc123

# New subscription fields
advice_subscriptions: testing,security,git-workflow
advice_subscriptions_exclude: role:crew
```

### Field Semantics

| Field | Purpose | Example |
|-------|---------|---------|
| `advice_subscriptions` | Additional labels to subscribe to | `testing,security` |
| `advice_subscriptions_exclude` | Labels to exclude (even if auto-subscribed) | `role:crew` |

### Subscription Resolution

```
1. Start with auto-subscriptions:
   [global, agent:<id>, rig:<rig>, role:<role>]

2. Add custom subscriptions:
   + [testing, security, git-workflow]

3. Remove excluded subscriptions:
   - [role:crew]

4. Final subscription set used for matching
```

### Example Scenarios

**Scenario 1: Developer wants testing advice**
```bash
bd update gastown/crew/alice --advice-subscriptions "testing,ci"
```
Alice now sees advice tagged `testing` or `ci` in addition to her auto-subscriptions.

**Scenario 2: Senior dev excludes basic tips**
```bash
bd update gastown/crew/bob --advice-subscriptions-exclude "basics,beginner"
```
Bob stops seeing advice tagged `basics` or `beginner`.

**Scenario 3: Specialist focuses on security**
```bash
bd update gastown/crew/security_lead \
  --advice-subscriptions "security,compliance,audit" \
  --advice-subscriptions-exclude "role:crew"
```
Security lead sees security-focused advice, not generic crew advice.

## Implementation

### Phase 1: Agent Bead Fields

**In beads (AgentFields struct):**
```go
type AgentFields struct {
    // ... existing fields ...

    // Advice subscription customization
    AdviceSubscriptions        []string // Additional labels to subscribe to
    AdviceSubscriptionsExclude []string // Labels to exclude from auto-subscriptions
}
```

**Parsing/formatting in beads_agent.go:**
```go
case "advice_subscriptions":
    fields.AdviceSubscriptions = strings.Split(value, ",")
case "advice_subscriptions_exclude":
    fields.AdviceSubscriptionsExclude = strings.Split(value, ",")
```

### Phase 2: bd advice list --for Enhancement

**In beads advice list command:**
```go
func buildAgentSubscriptions(agentID string) []string {
    // 1. Start with auto-subscriptions
    subs := []string{"global", "agent:" + agentID}
    if rig := extractRig(agentID); rig != "" {
        subs = append(subs, "rig:"+rig)
    }
    if role := extractRole(agentID); role != "" {
        subs = append(subs, "role:"+role)
    }

    // 2. Look up agent bead for custom subscriptions
    agentBead, fields, err := b.GetAgentBead(agentID)
    if err == nil && fields != nil {
        // Add custom subscriptions
        subs = append(subs, fields.AdviceSubscriptions...)

        // Remove excluded subscriptions
        subs = filterOut(subs, fields.AdviceSubscriptionsExclude)
    }

    return subs
}
```

### Phase 3: CLI Commands

**New flags for bd update:**
```bash
bd update <agent-id> --advice-subscriptions "label1,label2"
bd update <agent-id> --advice-subscriptions-exclude "label1,label2"
```

**Query subscriptions:**
```bash
bd agent subscriptions <agent-id>
# Output:
# Auto-subscriptions: global, agent:gastown/crew/alice, rig:gastown, role:crew
# Custom: testing, security
# Excluded: basics
# Final: global, agent:gastown/crew/alice, rig:gastown, testing, security
```

## Migration

1. **No breaking changes**: Agents without subscription fields use auto-subscriptions only
2. **Gradual adoption**: Agents can add subscriptions as needed
3. **Backward compatible**: `bd advice list --for` continues to work

## Subscribing to Individual Advice

Agents can subscribe to specific advice beads by ID:

```bash
# Subscribe to a specific advice bead
bd update gastown/crew/alice --advice-subscriptions "gt-special-tip"

# The advice needs to have that ID as a label (or we auto-add advice IDs as labels)
bd advice add "Alice's special workflow" -l gt-special-tip
```

This enables:
- **Mentorship**: Senior dev creates advice, mentee subscribes to it
- **Ownership**: Formula owner subscribes to advice about their formula
- **Temporary**: Subscribe to advice during a specific project, unsubscribe later

## Label Taxonomy (Optional)

Labels are freeform. Some suggested categories:

| Category | Examples | Purpose |
|----------|----------|---------|
| **Topic** | `testing`, `security`, `git`, `ci` | Cross-cutting concerns |
| **Language** | `go`, `python`, `typescript` | Language-specific tips |
| **Workflow** | `pr-review`, `deployment`, `debugging` | Process guidance |
| **Formula** | `mol-review`, `mol-deploy` | Formula-specific guidance |

No required taxonomy - labels evolve organically based on what's useful.

**Discovery:**
```bash
bd advice labels           # List all labels in use
bd advice list -l testing  # Preview what "testing" subscription sees
```

## Open Questions

1. **Inheritance**: Should crew members inherit from a "crew template" subscription set?
   - Recommendation: No, keep it simple. Auto-subscriptions + agent overrides.

2. **Validation**: Should we validate labels exist before subscribing?
   - Recommendation: No, allow forward references (subscribe before advice exists).

3. **Bulk operations**: How to update subscriptions for multiple agents?
   - Recommendation: Script it. `bd list -t agent | xargs -I{} bd update {} --advice-subscriptions X`

## Example: Full Workflow

```bash
# 1. Create topic-based advice
bd advice add "Always run go vet" -l go -l testing
bd advice add "Check for SQL injection" -l security -l databases
bd advice add "Use gofmt before commit" -l go -l formatting

# 2. Agent subscribes to relevant topics
bd update gastown/crew/backend_dev --advice-subscriptions "go,security,databases"

# 3. Agent sees personalized advice
gt prime  # Shows: go vet, SQL injection, gofmt (plus auto-subscribed advice)

# 4. Agent excludes noisy advice
bd update gastown/crew/backend_dev --advice-subscriptions-exclude "basics"
```

## Summary

- **Storage**: Agent bead fields (`advice_subscriptions`, `advice_subscriptions_exclude`)
- **Resolution**: Auto-subs + custom - excluded
- **CLI**: `bd update --advice-subscriptions`, `bd agent subscriptions`
- **No rig.yaml**: All config lives in agent beads (portable, explicit, auditable)
