# Crew as Formula Owners

> Design document for the crew/formula ownership model

## Core Principle

**Crew build formulas, polecats run them.**

This creates a clean separation of concerns:
- **Crew** are formula specialists who iterate on workflow design
- **Polecats** are execution engines that run formulas at scale

## The Model

### 1:1 Crew-Formula Mapping

Each crew member owns exactly one formula:

```
gastown/crew/code_review     → owns code-review.formula.toml
gastown/crew/conflict_resolve → owns mol-polecat-conflict-resolve.formula.toml
gastown/crew/e2e_reproduce   → owns e2e-reproduce-fix.formula.toml
```

**Naming convention**: Crew names use underscores, formula names use hyphens.
The mapping is: `crew_name` owns `crew-name.formula.toml` or `mol-polecat-crew-name.formula.toml`.

### Crew Responsibilities

1. **Own the formula** - The canonical source of truth for one workflow
2. **Spawn polecats** - When work arrives, dispatch to workers
3. **Collect feedback** - Aggregate execution results from their polecats
4. **Iterate** - Improve the formula based on execution patterns
5. **Never execute directly** - Crew orchestrate, polecats execute

### Polecat Responsibilities

1. **Execute molecules** - Run the formula steps faithfully
2. **Report results** - Update molecule beads with outcomes
3. **Signal completion** - Run `gt done` when finished
4. **Self-terminate** - Clean up after work completes

## Work Discovery

### How Crew Find Work

Crew discover work that needs their formula through **formula subscriptions**:

```toml
# In crew config or formula metadata
[subscription]
labels = ["needs-code-review", "review-requested"]
issue_types = ["pr-review", "code-audit"]
prefix_filter = "gt-*"  # Only watch gastown issues
```

When issues matching these criteria become available (unblocked, unassigned),
the crew receives notification via:

1. **Beads event** - `bd events --watch --labels=needs-code-review`
2. **Mail notification** - Mayor sends "work available" mail to subscribed crew

### Discovery Flow

```
Issue created with label "needs-code-review"
    ↓
Mayor scans for subscribed crew
    ↓
Mail sent to gastown/crew/code_review
    ↓
Crew wakes, sees mail on hook
    ↓
Crew spawns polecat with the issue
    ↓
Polecat executes code-review formula
    ↓
Polecat completes, reports back
```

## Polecat→Crew Feedback

### Feedback Channels

1. **Molecule completion events** - Standard bead state changes
2. **Execution reports** - Structured feedback on formula performance
3. **Critical failures** - Immediate mail for blocking issues

### Execution Report Schema

When a polecat completes (success or failure), it generates an execution report:

```jsonl
{
  "formula": "code-review",
  "molecule_id": "mol-abc123",
  "polecat": "gastown/polecats/rictus",
  "crew_owner": "gastown/crew/code_review",
  "outcome": "success|failure|partial",
  "steps_completed": 5,
  "steps_total": 5,
  "duration_seconds": 342,
  "errors": [],
  "suggestions": ["Step 3 could be parallelized"],
  "timestamp": "2026-01-17T12:00:00Z"
}
```

### Feedback Flow

```
Polecat completes molecule
    ↓
gt done triggers execution report generation
    ↓
Report written to crew's feedback inbox:
  .beads/feedback/<formula>/<date>/<molecule-id>.jsonl
    ↓
Crew periodically reviews feedback
    ↓
Formula improvements committed
```

### Critical Failure Escalation

When polecats fail critically (crash, stuck, unrecoverable error):

```bash
# Witness detects failure
# Witness sends mail to crew owner
gt mail send gastown/crew/code_review \
  -s "POLECAT FAILURE: mol-abc123" \
  -m "Polecat rictus failed on code-review molecule.
Error: Test timeout in step 3
Molecule: mol-abc123
See: bd show mol-abc123"
```

## Witness vs Crew Monitoring

### Division of Responsibilities

| Aspect | Witness | Crew |
|--------|---------|------|
| Lifecycle management | ✓ | |
| Health checks | ✓ | |
| Stuck detection | ✓ | |
| Recycling failed polecats | ✓ | |
| Formula quality | | ✓ |
| Execution patterns | | ✓ |
| Improvement iterations | | ✓ |

**Rule**: Crew trust Witness for polecat lifecycle. Crew focus on formula quality.

### What Crew Monitor

1. **Success rate** - What % of executions succeed?
2. **Common failure points** - Which steps fail most often?
3. **Duration trends** - Is the formula getting slower?
4. **Error patterns** - Are there systematic issues?

Crew do NOT monitor:
- Individual polecat health (Witness does this)
- Polecat session state (Witness does this)
- When to recycle polecats (Witness does this)

## Formula Dependencies

### When Formulas Depend on Each Other

Example: `deploy` formula depends on `code-review` completing first.

**Solution**: Use existing bead dependency graph.

```
Issue: gt-deploy-123 (needs deploy formula)
  └── depends on: gt-review-456 (needs code-review formula)
```

### Coordination Flow

```
1. gt-review-456 created (blocked by nothing)
2. crew/code_review discovers it, spawns polecat
3. Polecat completes review, closes gt-review-456
4. Dependency resolves → gt-deploy-123 unblocks
5. crew/deploy discovers newly unblocked issue
6. crew/deploy spawns polecat for deploy
```

**Key insight**: Crew don't coordinate directly. The bead dependency graph
handles sequencing. Each crew just watches for unblocked work in their domain.

### Cross-Crew Communication

For exceptional cases requiring direct coordination:

```bash
# Crew A needs input from Crew B
gt mail send gastown/crew/deploy \
  -s "Pre-deploy checklist question" \
  -m "The review for gt-123 flagged a security concern.
Should deploy proceed? See: bd show gt-review-456"
```

## Formula Improvement Cadence

### Triggers for Formula Review

1. **Threshold-based**: Success rate drops below 90%
2. **Pattern-based**: Same error occurs 3+ times in a week
3. **Duration-based**: Average execution time increases 20%
4. **Scheduled**: Weekly review of all execution reports
5. **On-demand**: Human requests formula audit

### Improvement Workflow

```
1. Crew reviews accumulated feedback
2. Identifies improvement opportunity
3. Creates new formula version (bump version number)
4. Tests with limited rollout (optional)
5. Commits updated formula
6. Future polecats use new version
```

### Version Management

```toml
[formula]
name = "code-review"
version = "4.1.0"  # Semver: major.minor.patch
changelog = """
4.1.0 - Added parallel test execution in step 3
4.0.0 - Restructured for new code review guidelines
"""
```

## New Commands Needed

### Crew Management

```bash
# Create crew with formula ownership
gt crew create code_review --formula=code-review

# Show crew's formula and stats
gt crew show code_review
# Output:
#   Crew: gastown/crew/code_review
#   Formula: code-review v4.1.0
#   Subscriptions: labels=[needs-code-review]
#   Stats (last 7 days):
#     Executions: 23
#     Success rate: 95.6%
#     Avg duration: 5m 42s

# List all crew with their formulas
gt crew list --formulas
```

### Work Dispatch

```bash
# Crew spawns polecat for specific work
gt crew dispatch gt-review-456
# → Creates polecat, attaches molecule from crew's formula

# Crew spawns polecat for next available work
gt crew dispatch --next
# → Finds unblocked work matching subscription, dispatches
```

### Feedback Management

```bash
# View execution feedback for crew's formula
gt crew feedback [crew-name]
# Shows recent execution reports

# Aggregate stats
gt crew stats code_review --period=7d
# Shows success rate, common errors, duration trends
```

### Formula Operations

```bash
# Crew updates their formula
gt crew formula edit
# Opens formula in editor

# Crew tests formula change
gt crew formula test --dry-run gt-review-456
# Validates formula without executing

# Crew publishes updated formula
gt crew formula publish
# Commits and pushes formula changes
```

## Crew Startup Behavior

When a crew agent starts:

```
1. Announce: "gastown Crew code_review, checking in"
2. Load formula context: gt crew prime
3. Check hook for work: gt hook
4. If hooked work → dispatch polecat
5. If no hooked work → check subscriptions for available work
6. If available work → dispatch polecat
7. If no work → check feedback inbox
8. If feedback → review and iterate formula
9. If nothing → idle (await mail/nudge)
```

### Crew Prime Output

```bash
gt crew prime
# Output:
# ═══════════════════════════════════════
# Crew: gastown/crew/code_review
# Formula: code-review v4.1.0
# ═══════════════════════════════════════
#
# Subscriptions:
#   - labels: needs-code-review
#   - types: pr-review
#
# Pending work: 3 issues
#   gt-review-456 (unblocked)
#   gt-review-457 (unblocked)
#   gt-review-458 (blocked by gt-build-789)
#
# Active polecats: 1
#   rictus → gt-review-455 (in_progress)
#
# Feedback inbox: 5 new reports
# ═══════════════════════════════════════
```

## Migration Path

### Phase 1: Infrastructure (Current)

- Crew exist as persistent workspaces
- Polecats execute molecules
- Formulas define workflows

### Phase 2: Ownership Model

1. Add `--formula` flag to `gt crew create`
2. Create crew-formula mapping in config
3. Add subscription system for work discovery
4. Add feedback collection in `gt done`

### Phase 3: Crew Dispatch

1. Implement `gt crew dispatch`
2. Add crew-owned polecat tracking
3. Build feedback aggregation

### Phase 4: Formula Iteration

1. Add `gt crew feedback` and `gt crew stats`
2. Implement threshold-based review triggers
3. Add formula testing capabilities

## Open Questions

1. **Multi-formula crew?** Should advanced crew own multiple related formulas?
2. **Formula inheritance?** Can formulas extend base formulas?
3. **Crew collaboration?** How do multiple crew improve a shared formula?
4. **Formula marketplace?** How do crew share formulas across towns?

## Appendix: Complete Command Inventory

### New `gt crew` Commands

| Command | Description |
|---------|-------------|
| `gt crew create <name> --formula=<f>` | Create crew with formula ownership |
| `gt crew show <name>` | Show crew details, formula, and stats |
| `gt crew list [--formulas]` | List all crew, optionally with formulas |
| `gt crew prime` | Load crew context (formula, subscriptions, pending work) |
| `gt crew pending [--count\|--next]` | List/count work matching subscription |
| `gt crew dispatch <bead-id>` | Spawn polecat to execute crew's formula |
| `gt crew dispatch --next` | Dispatch next available work |
| `gt crew active [--count]` | List active polecats owned by crew |
| `gt crew status` | Overview of crew state (pending, active, feedback) |
| `gt crew feedback [--since=<period>]` | View execution feedback reports |
| `gt crew stats <name> [--period=<p>]` | Aggregate execution statistics |
| `gt crew formula edit` | Open crew's formula in editor |
| `gt crew formula validate` | Validate formula syntax and structure |
| `gt crew formula test --dry-run <bead>` | Test formula without execution |
| `gt crew formula publish` | Commit and push formula changes |

### Modified Commands

| Command | Change |
|---------|--------|
| `gt done` | Add execution report generation, `WORK_DONE` mail to dispatcher |
| `gt sling` | Store `dispatched_by` in bead metadata for feedback routing |

### New Mail Message Types

| Type | Route | Purpose |
|------|-------|---------|
| `WORK_DONE` | Polecat → Crew | Notify dispatcher of completion |
| `EXECUTION_REPORT` | Polecat → Crew feedback | Structured execution feedback |
| `POLECAT_FAILURE` | Witness → Crew | Alert on critical polecat failure |
| `FORMULA_ALERT` | System → Crew | Alert on metric threshold breach |

### Configuration Files

| File | Purpose |
|------|---------|
| `~/.config/gt/crew/<name>.toml` | Crew configuration (formula, subscriptions, capacity) |
| `.beads/feedback/<formula>/` | Execution report storage |

## Related Documents

- [Formula Resolution](formula-resolution.md) - Where formulas live
- [Polecat Lifecycle](../concepts/polecat-lifecycle.md) - Worker management
- [Mail Protocol](mail-protocol.md) - Agent communication
- [Crew Startup Behavior](crew-startup-behavior.md) - Detailed startup spec
