# Epic System Improvements Proposal

> Research document for hq-5881b3.8
> Created: 2026-01-24
> Based on: epic-system-documentation.md, epic-pain-points.md, epic-simplification-opportunities.md

## Executive Summary

The epic system is functional but suffers from:
1. **Noise** - Wisp epics dominate listings
2. **Confusion** - Molecule vs epic terminology overlap
3. **Missing features** - No templates, inline progress, or dashboard
4. **Complexity** - Duplicate blocking logic, redundant commands

This proposal prioritizes practical improvements with clear implementation paths.

---

## Priority 1: Critical Fixes (Do First)

### 1.1 Wisp Type Separation

**Problem:** 83% of epics are `hq-wisp--*` temporary molecules.

**Solution:** Change wisps to use `issue_type = "wisp"` instead of `"epic"`.

**Implementation:**
```sql
-- Migration
UPDATE issues SET issue_type = 'wisp'
WHERE id LIKE 'hq-wisp--%' AND issue_type = 'epic';
```

**CLI changes:**
- `bd list -t epic` excludes wisps by default
- `bd list -t wisp` shows wisps explicitly
- `bd epic status` filters out wisps

**Effort:** Low (1-2 hours)
**Impact:** Dramatically cleaner epic listings

### 1.2 Fix "BLOCKS" Label Confusion

**Problem:** Children shown under "BLOCKS" header in `bd show`.

**Solution:** Separate display sections:
```
CHILDREN (4)
  ← ✓ hq-5881b3.5: Document current epic functionality
  ← ◐ hq-5881b3.6: Identify pain points

BLOCKED BY
  → ○ hq-prereq: Some prerequisite
```

**Implementation:** Update `renderIssue()` in `cmd/bd/show.go`.

**Effort:** Low (1 hour)
**Impact:** Better UX, clearer semantics

### 1.3 Remove `bd mol stale` Duplicate

**Problem:** `bd mol stale` duplicates `bd epic status --eligible-only`.

**Solution:** Delete `cmd/bd/mol_stale.go`, update docs.

**Effort:** Low (30 minutes)
**Impact:** Less confusion, simpler codebase

---

## Priority 2: High Value Features (Do Next)

### 2.1 Progress Display in `bd list`

**Problem:** Must run separate `bd epic status` to see progress.

**Solution:** Add inline progress to list output:
```bash
$ bd list -t epic
○ hq-5881b3 [● P2] [epic] [3/4] - Epic system research
○ gt-auth   [● P1] [epic] [0/6] - Authentication overhaul
✓ hq-older  [● P2] [epic] [5/5] - Completed epic
```

**Implementation:**
- Add progress lookup in `bd list` handler
- Cache epic stats to avoid N+1 queries
- Optional `--no-progress` flag if slow

**Effort:** Medium (4-6 hours)
**Impact:** High - most requested feature

### 2.2 Add `--rig` Flag to `bd list`

**Problem:** `bd create --rig` exists but `bd list --rig` doesn't.

**Solution:** Add consistent `--rig` flag to list command.

**Implementation:**
- Use existing routing infrastructure
- Allow `bd list --rig beads` to query beads rig

**Effort:** Medium (2-4 hours)
**Impact:** Cross-rig visibility

### 2.3 Auto-Close Option

**Problem:** Must manually run `bd epic close-eligible`.

**Solution:** Add `--auto-close` flag on epic creation:
```bash
bd create "Quick epic" -t epic --auto-close
```

When all children close, epic auto-closes.

**Implementation:**
- Add `auto_close` boolean field to issues table
- Hook into child close to check parent status
- Create notification on auto-close

**Effort:** Medium (4-6 hours)
**Impact:** Less manual bookkeeping

---

## Priority 3: Nice-to-Have Features (Consider Later)

### 3.1 Epic Templates

**Problem:** Repetitive epic structures (sprint, bugfix, feature).

**Solution:** Template system:
```bash
# Define template
bd template create sprint -t epic \
  --child "Design" \
  --child "Implementation" \
  --child "Testing"

# Use template
bd epic create-from-template "Sprint 47" --template sprint
```

**Implementation:**
- Store templates as special beads or config files
- Template expansion on create

**Effort:** High (8-12 hours)
**Impact:** Reduces repetition for standard workflows

### 3.2 Epic Dashboard Command

**Solution:** Single command for epic health:
```bash
$ bd epic dashboard hq-5881b3
╭─────────────────────────────────────────────────╮
│ Epic: hq-5881b3 - Epic system research          │
├─────────────────────────────────────────────────┤
│ Progress: ████████░░ 75% (3/4)                  │
│ Status: in_progress                             │
│ Created: 2026-01-24                             │
│                                                 │
│ Children:                                       │
│  ✓ hq-5881b3.5 Document functionality    closed │
│  ✓ hq-5881b3.6 Pain points               closed │
│  ✓ hq-5881b3.7 Simplification            closed │
│  ◐ hq-5881b3.8 Improvements          in_progress│
│                                                 │
│ Blocked: 0 | Ready: 1 | Complete: 3             │
╰─────────────────────────────────────────────────╯
```

**Effort:** Medium (4-6 hours)
**Impact:** Better at-a-glance status

### 3.3 Orphan Detection

**Solution:** Command to find orphaned children:
```bash
$ bd epic orphans
Found 2 orphaned issues:
  bd-abc.1 - parent bd-abc closed
  bd-xyz.2 - parent bd-xyz not found
```

**Effort:** Low (2-3 hours)
**Impact:** Data hygiene

### 3.4 Epic Reparenting

**Solution:** Move children between epics:
```bash
bd reparent bd-abc.1 --to bd-xyz
# Result: bd-abc.1 → bd-xyz.N (renumbered)
```

**Effort:** Medium (4-6 hours)
**Impact:** Flexibility in organizing work

---

## Priority 4: Simplifications (Clean Up)

### 4.1 Remove `--eligible-only` Flag

`bd epic status --eligible-only` duplicates `bd epic close-eligible` with different output.

**Action:** Remove flag, document `close-eligible` as the way to see eligible epics.

### 4.2 Simplify Epic Status Query

Replace nested CTEs with simpler GROUP BY query. Performance and maintainability win.

### 4.3 Consolidate Blocking Logic

Single source of truth for "is this issue blocked?"

---

## Epic-Convoy Integration

Current state is reasonable. Convoys track epic children without blocking them.

**Proposed enhancement:** When creating convoy, auto-populate with epic children:
```bash
gt convoy create "Sprint work" --from-epic hq-sprint-47
# Auto-adds all open children of the epic to convoy
```

---

## Implementation Roadmap

### Phase 1: Quick Wins (1-2 days)
- [ ] Wisp type separation (1.1)
- [ ] Fix BLOCKS label (1.2)
- [ ] Remove `bd mol stale` (1.3)
- [ ] Remove `--eligible-only` flag (4.1)

### Phase 2: Core Features (3-5 days)
- [ ] Progress in `bd list` (2.1)
- [ ] `--rig` flag on list (2.2)
- [ ] Auto-close option (2.3)

### Phase 3: Polish (1-2 weeks)
- [ ] Epic dashboard (3.2)
- [ ] Orphan detection (3.3)
- [ ] Epic reparenting (3.4)

### Phase 4: Advanced (Later)
- [ ] Epic templates (3.1)
- [ ] Query simplification (4.2)
- [ ] Blocking consolidation (4.3)

---

## Success Metrics

After implementation:
1. `bd list -t epic` shows < 20 real epics (not 52 with wisp noise)
2. Progress visible without extra commands
3. Cross-rig epic visibility via `--rig` flag
4. Zero redundant commands (`mol stale` removed)
5. Children labeled correctly (not "BLOCKS")

---

## Summary Table

| Improvement | Priority | Effort | Impact |
|-------------|----------|--------|--------|
| Wisp type separation | P1 | Low | Critical |
| Fix BLOCKS label | P1 | Low | High |
| Remove `bd mol stale` | P1 | Low | Medium |
| Progress in list | P2 | Medium | High |
| `--rig` on list | P2 | Medium | High |
| Auto-close option | P2 | Medium | Medium |
| Epic dashboard | P3 | Medium | Medium |
| Templates | P3 | High | Medium |
| Orphan detection | P3 | Low | Low |
| Reparenting | P3 | Medium | Low |
