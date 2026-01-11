# Wisp Squash Design: Cadences, Rules, Templates

Design specification for how wisps squash to digests in Gas Town.

## Problem Statement

Wisps are ephemeral molecules that need to be condensed into digests for:
- **Audit trail**: What happened, when, by whom
- **Activity feed**: Observable progress in the capability ledger
- **Space efficiency**: Ephemeral data doesn't accumulate indefinitely

Currently under-designed:
- **Cadences**: When should squash happen?
- **Templates**: What should digests contain?
- **Retention**: How long to keep, when to aggregate?

## Squash Cadences

### Patrol Wisps (Deacon, Witness, Refinery)

**Trigger**: End of each patrol cycle

```
patrol-start → steps → loop-or-exit step → squash → new wisp
```

| Decision Point | Action |
|----------------|--------|
| `loop-or-exit` with low context | Squash current wisp, create new wisp |
| `loop-or-exit` with high context | Squash current wisp, handoff |
| Extraordinary action | Squash immediately, handoff |

**Rationale**: Each patrol cycle is a logical unit. Squashing per-cycle keeps
digests meaningful and prevents context-filling sessions from losing history.

### Work Wisps (Polecats)

**Trigger**: Before `gt done` or molecule completion

```
work-assigned → steps → all-complete → squash → gt done → merge queue
```

Polecats typically use regular molecules (not wisps), but when wisps are used
for exploratory work:

| Scenario | Action |
|----------|--------|
| Molecule completes | Squash to digest |
| Molecule abandoned | Burn (no digest) |
| Molecule handed off | Squash, include handoff context |

### Time-Based Cadences (Future)

For long-running molecules that span multiple sessions:

| Duration | Action |
|----------|--------|
| Session ends | Auto-squash if molecule in progress |
| > 24 hours | Create checkpoint digest |
| > 7 days | Warning: stale molecule |

**Not implemented initially** - simplicity first.

## Summary Templates

### Template Structure

Digests have three sections:
1. **Header**: Standard metadata (who, what, when)
2. **Body**: Context-specific content (from template)
3. **Footer**: System metrics (steps, duration, commit refs)

### Patrol Digest Template

```markdown
## Patrol Digest: {{.Agent}}

**Cycle**: {{.CycleNumber}} | **Duration**: {{.Duration}}

### Actions Taken
{{range .Actions}}
- {{.Icon}} {{.Description}}
{{end}}

### Issues Filed
{{range .IssuesFiled}}
- {{.ID}}: {{.Title}}
{{end}}

### Metrics
- Inbox: {{.InboxCount}} messages processed
- Health checks: {{.HealthChecks}}
- Alerts: {{.AlertCount}}
```

### Work Digest Template

```markdown
## Work Digest: {{.IssueTitle}}

**Issue**: {{.IssueID}} | **Agent**: {{.Agent}} | **Duration**: {{.Duration}}

### Summary
{{.Summary}}

### Steps Completed
{{range .Steps}}
- [{{.Status}}] {{.Title}}
{{end}}

### Artifacts
- Commits: {{range .Commits}}{{.Short}}, {{end}}
- Files changed: {{.FilesChanged}}
- Lines: +{{.LinesAdded}} -{{.LinesRemoved}}
```

### Formula-Defined Templates

Formulas can define custom squash templates in `[squash]` section:

```toml
formula = "mol-my-workflow"
version = 1

[squash]
template = """
## {{.Title}} Complete

Duration: {{.Duration}}
Key metrics:
{{range .Steps}}
- {{.ID}}: {{.CustomField}}
{{end}}
"""

# Template variables from step outputs
[squash.vars]
include_metrics = true
summary_length = "short"  # short | medium | detailed
```

**Resolution order**:
1. Formula-defined template (if present)
2. Type-specific default (patrol vs work)
3. Minimal fallback (current behavior)

## Retention Rules

### Digest Lifecycle

```
Wisp → Squash → Digest (active) → Digest (archived) → Rollup
```

| Phase | Duration | Storage |
|-------|----------|---------|
| Active | 30 days | `.beads/issues.jsonl` |
| Archived | 1 year | `.beads/archive/` (compressed) |
| Rollup | Permanent | Weekly/monthly summaries |

### Rollup Strategy

After retention period, digests aggregate into rollups:

**Weekly Patrol Rollup**:
```markdown
## Week of {{.WeekStart}}

| Agent | Cycles | Issues Filed | Merges | Incidents |
|-------|--------|--------------|--------|-----------|
| Deacon | 140 | 3 | - | 0 |
| Witness | 168 | 12 | - | 2 |
| Refinery | 84 | 0 | 47 | 1 |
```

**Monthly Work Rollup**:
```markdown
## {{.Month}} Work Summary

Issues completed: {{.TotalIssues}}
Total duration: {{.TotalDuration}}
Contributors: {{range .Contributors}}{{.Name}}, {{end}}

Top categories:
{{range .Categories}}
- {{.Name}}: {{.Count}} issues
{{end}}
```

### Retention Configuration

Per-rig settings in `config.json`:

```json
{
  "retention": {
    "digest_active_days": 30,
    "digest_archive_days": 365,
    "rollup_weekly": true,
    "rollup_monthly": true,
    "auto_archive": true
  }
}
```

## Implementation Plan

### Phase 1: Template System (MVP)

1. Add `[squash]` section parsing to formula loader
2. Create default templates for patrol and work digests
3. Enhance `bd mol squash` to use templates
4. Add `--template` flag for override

### Phase 2: Cadence Automation

1. Hook squash into `gt done` flow
2. Add patrol cycle completion detection
3. Emit squash events for activity feed

### Phase 3: Retention & Archival

1. Implement digest aging (active → archived)
2. Add `bd archive` command for manual archival
3. Create rollup generator for weekly/monthly summaries
4. Background daemon task for auto-archival

## Commands

### Squash with Template

```bash
# Use formula-defined template
bd mol squash <id>

# Use explicit template
bd mol squash <id> --template=detailed

# Add custom summary
bd mol squash <id> --summary="Patrol complete: 3 issues filed"
```

### View Digests

```bash
# List recent digests
bd list --label=digest

# View rollups
bd rollup list
bd rollup show weekly-2025-01
```

### Archive Management

```bash
# Archive old digests
bd archive --older-than=30d

# Generate rollup
bd rollup generate --week=2025-01

# Restore from archive
bd archive restore <digest-id>
```

## Activity Feed Integration

Digests feed into the activity feed for observability:

```json
{
  "type": "digest",
  "agent": "greenplace/witness",
  "timestamp": "2025-12-30T10:00:00Z",
  "summary": "Patrol cycle 47 complete",
  "metrics": {
    "issues_filed": 2,
    "polecats_nudged": 1,
    "duration_minutes": 12
  }
}
```

The feed curator (daemon) can aggregate these for dashboards.

## Formula Example

Complete formula with squash configuration:

```toml
formula = "mol-witness-patrol"
version = 1
type = "workflow"
description = "Witness patrol cycle"

[squash]
trigger = "on_complete"
template_type = "patrol"
include_metrics = true

[[steps]]
id = "inbox-check"
title = "Check inbox"
description = "Process messages and escalations"

[[steps]]
id = "health-scan"
title = "Scan polecat health"
description = "Check all polecats for stuck/idle"

[[steps]]
id = "nudge-stuck"
title = "Nudge stuck workers"
description = "Send nudges to idle polecats"

[[steps]]
id = "loop-or-exit"
title = "Loop or exit decision"
description = "Decide whether to continue or handoff"
```

## Migration

### Existing Digests

Current minimal digests remain valid. New template system is additive:
- Old digests: Title, basic description
- New digests: Structured content, metrics

### Backward Compatibility

- `bd mol squash` without template uses current behavior
- Formulas without `[squash]` section use type defaults
- No breaking changes to existing workflows

## Design Decisions

### Why Squash Per-Cycle?

**Alternative**: Squash on session end only

**Rejected because**:
- Sessions can crash mid-cycle (lost audit trail)
- High-context sessions may span multiple cycles
- Per-cycle gives finer granularity

### Why Formula-Defined Templates?

**Alternative**: Hard-coded templates per role

**Rejected because**:
- Different workflows have different metrics
- Extensibility for custom formulas
- Separation of concerns (workflow defines its own output)

### Why Retain Forever (as Rollups)?

**Alternative**: Delete after N days

**Rejected because**:
- Capability ledger needs long-term history
- Rollups are small (aggregate stats)
- Audit requirements vary by use case

## Future Considerations

- **Search**: Full-text search over archived digests
- **Analytics**: Metrics aggregation dashboard
- **Export**: Export digests to external systems
- **Compliance**: Configurable retention for regulatory needs
