# Epic System Pain Points and Missing Features

> Research document for hq-5881b3.6
> Created: 2026-01-24

## Critical Pain Points

### 1. Wisp Epic Clutter (HIGH)

**Problem:** 83% of epics (43 of 52) are `hq-wisp--*` temporary molecule/patrol epics that clutter epic listings.

```bash
$ bd list -t epic | wc -l
52
$ bd list -t epic | grep "wisp" | wc -l
43
```

**Impact:**
- `bd epic status` output is dominated by wisp noise
- Hard to find real user-created epics
- 288 epics eligible for closure (mostly wisps)

**Desired:** Wisps should use a different type (e.g., `wisp` or `molecule`) rather than `epic`, or have a way to filter them out by default.

### 2. Confusing "BLOCKS" Terminology (MEDIUM)

**Problem:** In `bd show <epic>`, children are displayed under "BLOCKS" header:

```
BLOCKS
  ← ✓ hq-5881b3.5: Document current epic functionality
  ← ◐ hq-5881b3.6: Identify epic pain points
```

**Impact:** Semantically confusing - children don't "block" the epic, they're contained by it.

**Desired:** Use "CHILDREN" or "SUBTASKS" label for parent-child relationships.

### 3. No Cross-Rig `--rig` Flag on `bd list` (MEDIUM)

**Problem:** `bd create --rig` exists but `bd list --rig` doesn't:

```bash
$ bd list -t epic --rig gastown
Error: unknown flag: --rig
```

**Impact:** Can't filter listings by rig, must navigate to each rig to see its issues.

**Desired:** Consistent `--rig` flag across all commands.

### 4. `bd dep list` Doesn't Show Children (LOW)

**Problem:** `bd dep list <epic>` only shows blocking dependencies, not parent-child relationships:

```bash
$ bd dep list hq-5881b3
hq-5881b3 has no dependencies
```

Yet the epic clearly has 4 children.

**Impact:** Need to use `bd show` to see children, inconsistent with `bd list --parent`.

**Desired:** `bd dep list` should include parent-child relationships with a `--children` flag or similar.

---

## Missing Features

### 1. Epic Templates

**Current State:** No template system for epics. Each epic must be created manually with its children.

**Desired:** Ability to define epic templates with standard child structure:
```bash
bd epic create-from-template "Sprint Planning" --template=sprint
# Auto-creates:
#   sprint-xxx
#   ├── sprint-xxx.1 "Design"
#   ├── sprint-xxx.2 "Implementation"
#   ├── sprint-xxx.3 "Testing"
#   └── sprint-xxx.4 "Documentation"
```

### 2. Progress Bar in List Output

**Current State:** Progress only visible in `bd epic status`, not in `bd list -t epic`.

**Desired:** Show progress percentage inline:
```bash
$ bd list -t epic
○ hq-5881b3 [● P2] [epic] [2/5 40%] - Epic system research
```

### 3. Epic Dashboard Command

**Current State:** Must use multiple commands to get epic overview.

**Desired:** Single command showing epic health:
```bash
$ bd epic dashboard hq-5881b3
Epic: hq-5881b3 - Epic system research
Progress: ████████░░ 40% (2/5)
Status: in_progress
Blocked children: 0
Ready children: 3
Assignee distribution: crew/epic_researcher (5)
```

### 4. Orphan Detection

**Current State:** No command to find orphan children (children whose parent was deleted/closed).

**Desired:**
```bash
$ bd epic orphans
Found 3 orphaned issues:
  bd-abc.1 - parent bd-abc is closed
  bd-xyz.2 - parent bd-xyz not found
```

### 5. Priority Inheritance/Override

**Current State:** Epic priority doesn't affect child priorities.

**Desired:** Option to inherit parent priority or explicit override tracking:
```bash
bd create "Urgent task" --parent=hq-epic --inherit-priority
# Child gets P1 if parent is P1
```

### 6. Epic Burndown/Velocity

**Current State:** No historical progress tracking.

**Desired:** Track completion over time:
```bash
$ bd epic burndown hq-5881b3
2026-01-20: 0/5 (0%)
2026-01-22: 1/5 (20%)
2026-01-24: 2/5 (40%)
Estimated completion: 2026-01-28
```

### 7. Auto-Close on Completion

**Current State:** Must run `bd epic close-eligible` manually.

**Desired:** Option for automatic epic closure:
```bash
bd create "Quick epic" -t epic --auto-close
# Automatically closes when all children complete
```

### 8. Epic Move/Reparent

**Current State:** Cannot move a child to a different parent.

**Desired:**
```bash
bd reparent bd-abc.1 --to bd-xyz
# Moves child from bd-abc to bd-xyz
```

---

## Usability Issues

### 1. Cross-Rig Epic Confusion

**Observation:** `bd show gt-u1j` fails but `bd list --parent gt-u1j` works.

The routing system can find children but not always the parent. Confusing behavior.

### 2. Inconsistent Status Filtering

**Observation:** `bd epic status` shows eligible-for-closure epics by default, not in-progress ones.

Users looking for active epic progress must scroll past completed ones.

### 3. No Recursive Child Count

**Current:** `bd list --parent` shows only direct children.

**Desired:** Option to show full descendant count:
```bash
$ bd list --parent hq-epic --recursive
hq-epic.1 (3 descendants)
hq-epic.2 (0 descendants)
```

---

## Summary Table

| Issue | Severity | Category |
|-------|----------|----------|
| Wisp epic clutter | HIGH | Pain Point |
| "BLOCKS" terminology | MEDIUM | Pain Point |
| No `--rig` on `bd list` | MEDIUM | Missing Feature |
| `bd dep list` ignores children | LOW | Pain Point |
| Epic templates | HIGH | Missing Feature |
| Progress in list output | MEDIUM | Missing Feature |
| Epic dashboard | MEDIUM | Missing Feature |
| Orphan detection | LOW | Missing Feature |
| Priority inheritance | LOW | Missing Feature |
| Burndown tracking | LOW | Missing Feature |
| Auto-close option | LOW | Missing Feature |
| Epic move/reparent | MEDIUM | Missing Feature |
