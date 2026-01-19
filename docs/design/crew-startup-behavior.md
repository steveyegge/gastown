# Crew Startup Behavior Specification

> How crew agents initialize, discover work, and begin orchestration

## Overview

Crew agents are formula owners. When a crew wakes up, they don't execute work
directly - they discover work needing their formula and spawn polecats to run it.

## Startup Sequence

### Phase 1: Identity and Context

```bash
# 1. Announce identity
"gastown Crew code_review, checking in."

# 2. Load crew context
gt crew prime
```

**`gt crew prime` Output:**

```
═══════════════════════════════════════════════════════════
Crew: gastown/crew/code_review
Formula: code-review v4.1.0
═══════════════════════════════════════════════════════════

Subscriptions:
  - labels: needs-code-review, review-requested
  - types: pr-review, code-audit

Pending work: 3 issues
  gt-review-456 (unblocked) ← READY
  gt-review-457 (unblocked) ← READY
  gt-review-458 (blocked by gt-build-789)

Active polecats: 1
  rictus → gt-review-455 (in_progress, 5m elapsed)

Feedback inbox: 5 new reports since last review
  Success rate (7d): 94.2% (49/52)
  Avg duration: 5m 42s
  Top failure: step 3 timeout (3 occurrences)
═══════════════════════════════════════════════════════════
```

### Phase 2: Check Hook

```bash
# 3. Check for hooked work
gt hook
```

**If work is hooked:**
- Crew was spawned with specific instructions
- Could be: dispatch request, formula review trigger, admin task
- Execute the hooked work (GUPP applies to crew too)

**If hook is empty:**
- Proceed to work discovery (Phase 3)

### Phase 3: Work Discovery

```bash
# 4. Check for pending work matching formula subscription
gt crew pending
```

Crew discover work through their **subscription** - a set of criteria that
identifies issues needing their formula:

```toml
# ~/.config/gt/crew/code_review.toml
[subscription]
labels = ["needs-code-review", "review-requested"]
issue_types = ["pr-review", "code-audit"]
prefix_filter = "gt-*"           # Only gastown issues
exclude_labels = ["wip", "draft"] # Skip work-in-progress
```

### Phase 4: Dispatch Decision

Based on pending work and active polecats:

**If pending work exists and capacity available:**
```bash
# 5a. Dispatch polecat for next available work
gt crew dispatch gt-review-456
```

**If at capacity (too many active polecats):**
```bash
# 5b. Wait for polecats to complete
# Monitor active polecats, check feedback
gt crew status
```

**If no pending work:**
```bash
# 5c. Check feedback inbox for formula improvements
gt crew feedback
```

### Phase 5: Feedback Review

If no work to dispatch, crew review execution feedback:

```bash
# 6. Review recent execution reports
gt crew feedback --since=7d

# Output:
# ═══════════════════════════════════════════════════════════
# Execution Feedback: code-review (last 7 days)
# ═══════════════════════════════════════════════════════════
#
# Summary:
#   Total executions: 52
#   Success rate: 94.2%
#   Avg duration: 5m 42s
#
# Failure Patterns:
#   Step 3 (run tests): 3 timeouts
#   Step 5 (submit review): 1 API error
#
# Suggestions:
#   - Consider increasing step 3 timeout
#   - Add retry logic for API calls in step 5
# ═══════════════════════════════════════════════════════════
```

**If patterns suggest improvement:**
```bash
# Edit formula
gt crew formula edit

# Test changes
gt crew formula validate

# Commit
git add .beads/formulas/code-review.formula.toml
git commit -m "formula(code-review): increase test timeout in step 3"
```

### Phase 6: Idle State

If no work and no feedback needing attention:

```bash
# 7. Enter idle state
# - Subscribe to work notifications
# - Await mail/nudge
```

Crew in idle state should:
1. Keep session alive (unlike polecats who terminate)
2. Watch for incoming mail about new work
3. Periodically re-check pending work (`gt crew pending`)

## Complete Startup Script

```bash
#!/bin/bash
# Crew startup protocol

# Phase 1: Identity
echo "gastown Crew ${CREW_NAME}, checking in."
gt crew prime

# Phase 2: Check hook
HOOKED=$(gt hook --quiet)
if [ -n "$HOOKED" ]; then
    echo "Work hooked: $HOOKED"
    # Execute hooked work (GUPP)
    # This might be a dispatch request, formula review, etc.
    exit 0
fi

# Phase 3-4: Work discovery and dispatch
PENDING=$(gt crew pending --count)
ACTIVE=$(gt crew active --count)
MAX_POLECATS=3  # Configurable per crew

if [ "$PENDING" -gt 0 ] && [ "$ACTIVE" -lt "$MAX_POLECATS" ]; then
    NEXT=$(gt crew pending --next)
    echo "Dispatching polecat for: $NEXT"
    gt crew dispatch "$NEXT"
fi

# Phase 5: Feedback review (if capacity reached or no work)
if [ "$ACTIVE" -ge "$MAX_POLECATS" ] || [ "$PENDING" -eq 0 ]; then
    FEEDBACK_COUNT=$(gt crew feedback --count)
    if [ "$FEEDBACK_COUNT" -gt 0 ]; then
        echo "Reviewing $FEEDBACK_COUNT feedback reports"
        gt crew feedback
        # Human judgment: decide if formula needs iteration
    fi
fi

# Phase 6: Idle
echo "Entering idle state. Watching for work..."
```

## Crew vs Polecat Startup Comparison

| Aspect | Crew Startup | Polecat Startup |
|--------|--------------|-----------------|
| **Identity** | `gt crew prime` | `gt prime` |
| **Hook check** | May or may not have work | MUST have work (error otherwise) |
| **Work source** | Subscriptions + feedback | Molecule on hook |
| **Execution** | Dispatch to polecats | Execute molecule directly |
| **Idle behavior** | Stay alive, watch for work | Never idle - terminate |
| **Termination** | Manual or scheduled | Automatic after `gt done` |

## Configuration

### Crew Config Location

```
~/.config/gt/crew/<crew-name>.toml
```

### Config Schema

```toml
[crew]
name = "code_review"
formula = "code-review"
rig = "gastown"

[subscription]
labels = ["needs-code-review"]
issue_types = ["pr-review"]
prefix_filter = "gt-*"
exclude_labels = ["wip"]

[capacity]
max_polecats = 3
dispatch_cooldown = "30s"  # Min time between dispatches

[feedback]
review_threshold = 10       # Review after N executions
alert_failure_rate = 0.15   # Alert if failure rate exceeds 15%

[schedule]
# Optional: crew can have scheduled review cycles
feedback_review = "0 9 * * 1"  # Monday 9am
```

## Error Handling

### Crew Can't Find Formula

```
ERROR: Formula 'code-review' not found
Resolution path checked:
  1. [project] ~/gt/gastown/.beads/formulas/ - NOT FOUND
  2. [town] ~/gt/.beads/formulas/ - NOT FOUND
  3. [system] <embedded> - NOT FOUND

Action: Create formula or update crew config
```

### Subscription Matches No Work

Normal state - crew enters idle and waits.

### Polecat Dispatch Fails

```
ERROR: Failed to spawn polecat for gt-review-456
Reason: No available polecat slots in rig

Action: Wait for existing polecats to complete
        Or request Witness to increase polecat capacity
```

## Integration with Other Components

### Witness Interaction

- Witness monitors polecats spawned by crew
- Witness notifies crew of polecat failures (critical issues only)
- Crew trusts Witness for lifecycle management

### Mayor Interaction

- Mayor may send work notifications to subscribed crew
- Mayor tracks crew activity for town-level dashboards
- Mayor may request crew to pause/resume operations

### Refinery Interaction

- No direct interaction
- Polecats handle merge queue submission
- Crew sees results via feedback reports

## Related Documents

- [Crew Formula Ownership](crew-formula-ownership.md) - The full ownership model
- [Polecat Lifecycle](../concepts/polecat-lifecycle.md) - How polecats work
- [Formula Resolution](../formula-resolution.md) - Where formulas come from
