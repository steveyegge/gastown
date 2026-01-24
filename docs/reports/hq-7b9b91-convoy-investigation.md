# Convoy System Investigation Report

> Report for: hq-7b9b91
> Investigator: gastown/crew/convoy_investigator
> Date: 2026-01-24

## Summary

Investigation into convoy system behavior when multiple tasks are slung to a single crew member. This revealed several issues with convoy creation, tracking, and workload management.

## Investigation Setup

**Scenario**: 18 separate decision-point tasks slung to `beads/crew/decision_point`

```bash
# Each of these commands was run separately
gt sling hq-946577.1 beads/crew/decision_point
gt sling hq-946577.2 beads/crew/decision_point
# ... through hq-946577.18
```

## Findings

### 1. Auto-Convoy Behavior

Each `gt sling` command creates a **separate convoy**:
- 18 slings created 18 convoys (hq-cv-v7plu through hq-cv-l5ggo)
- No batching or grouping of related work
- Results in "convoy spam" in the dashboard (34 open convoys total)

**Storage**: Convoys ARE correctly stored in Dolt with proper `tracks` dependencies to work beads.

### 2. Crew Workload Visibility

Crew member experiences:
- All 18 beads set to `status=hooked`
- `gt hook` shows only ONE task at a time
- No visibility into full queue (other 17 tasks)
- No prioritization mechanism for queued work

### 3. Convoy Metadata Issues

| Issue | Impact | Bug ID |
|-------|--------|--------|
| Convoys have `assignee=NULL` | Can't query "my convoys" | hq-7b9b91.4 |
| No batching mechanism | Dashboard spam, no grouping | hq-7b9b91.5 |
| Status shows 0/0 progress | Can't track convoy completion | hq-7b9b91.6 |

### 4. Database Discovery Issues

Related bugs filed in Dolt epic (hq-3446fc):
- `bd list` doesn't use Dolt by default
- `gt convoy list` doesn't use Dolt by default
- Only `bd show` correctly discovers Dolt database
- Other commands require explicit `BEADS_DB` environment variable

## Bugs Filed

### From This Investigation (hq-7b9b91)

| ID | Description | Priority | Status |
|----|-------------|----------|--------|
| hq-7b9b91.4 | Convoys created by gt sling have no assignee | P2 | Closed |
| hq-7b9b91.5 | No convoy batching or workload management for crew | P1 | Closed |
| hq-7b9b91.6 | gt convoy status shows 0/0 instead of tracked issue count | P2 | Closed |

### Related Dolt Issues (hq-3446fc)

| ID | Description |
|----|-------------|
| hq-3446fc.2 | bd list doesn't use Dolt by default |
| hq-3446fc.3 | gt convoy list doesn't use Dolt by default |

## Recommendations

### Short-term Fixes (Filed as bugs above)

1. **Convoy assignee**: Auto-convoys should inherit assignee from tracked bead
2. **Progress tracking**: `gt convoy status` should query `tracks` dependencies correctly
3. **Database discovery**: All bd/gt commands should discover Dolt consistently

### Medium-term Improvements

1. **Convoy batching option**: `gt sling --convoy=<id>` to add to existing convoy
2. **Auto-batch mode**: Group rapid-fire slings to same target into single convoy
3. **Queue visibility**: New command `gt queue` or `gt hook --all` to show full workload
4. **Warning threshold**: Warn when slinging >5 issues to single agent

### Long-term Architecture

1. **Work queue per agent**: Formal queue data structure beyond just hook
2. **Priority-based dequeue**: Allow priority to influence which hooked work runs first
3. **Convoy dashboard improvements**: Show assignees, filter by agent, progress bars

## Current Workaround

For batched work dispatch:

```bash
# Create convoy first with all issues
gt convoy create "Batch: Decision Points" hq-946577.1 hq-946577.2 hq-946577.3 ...

# Then assign convoy to crew member (mechanism TBD)
```

## Lessons Learned

1. **Auto-convoy is per-sling**: Good for visibility, but creates spam at scale
2. **Hook is singular**: Shows one task, not a queue - surprising for crew members
3. **Dolt discovery inconsistent**: Commands need `BEADS_DB` workaround
4. **Test at scale**: Single-sling worked fine; 18 slings revealed these issues

## References

- Epic: hq-7b9b91 "Convoy system investigation and improvements"
- Related: hq-3446fc "Dolt database integration"
- Docs: [Convoy concept](../concepts/convoy.md)
