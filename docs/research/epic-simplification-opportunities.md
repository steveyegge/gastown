# Epic System Simplification Opportunities

> Research document for hq-5881b3.7
> Created: 2026-01-24

## Summary

The epic system is reasonably clean but shows signs of feature creep and duplication. The biggest wins come from consolidating molecule/epic concepts and reducing duplicate logic.

---

## High Priority Simplifications

### 1. Consolidate `bd epic` and `bd mol stale`

**Issue:** These commands do essentially the same thing:
- `bd epic status` → shows completion status of epics
- `bd mol stale` → shows "stale molecules" (complete but unclosed)

Both use the same underlying `GetEpicsEligibleForClosure()` method.

**Action:**
- Remove `bd mol stale`
- Rename to `bd epic stale` as a flag on `bd epic status --stale`
- One less command to maintain

### 2. Simplify Epic Status Query

**Location:** `internal/storage/sqlite/epics.go`

**Issue:** Uses unnecessarily complex nested CTEs:
```sql
WITH epic_children AS (...)
     epic_stats AS (...)
SELECT ... FROM epic_stats ...
```

**Action:** Replace with simpler GROUP BY + HAVING:
```sql
SELECT epic_id, COUNT(*) as total,
       SUM(CASE WHEN status='closed' THEN 1 ELSE 0 END) as closed
FROM dependencies d JOIN issues i ON ...
WHERE d.type = 'parent-child'
GROUP BY epic_id
HAVING total = closed  -- for eligible-only
```

### 3. Remove Redundant `--eligible-only` Flag

**Issue:** `bd epic status --eligible-only` duplicates `bd epic close-eligible` functionality.

**Action:** Remove the flag. Users wanting only eligible epics use `close-eligible`.

---

## Medium Priority Simplifications

### 4. Unify Blocking Logic

**Issue:** Blocking/readiness computed in multiple places:
- `blocked_transitively` CTE in schema
- `GetBlockedIssues()` method
- `GetReadyWork()` method
- Multiple filtering passes

**Action:**
- Consolidate to single blocking check
- Cache blocking status per request
- Remove duplicate filtering in cmd layer

### 5. Remove Over-Engineered Recursive Depth Limit

**Location:** `schema.go` - `blocked_transitively` CTE

**Issue:** Depth limit of 50 for recursive blocking, but max hierarchy depth is 3.

**Action:**
- Remove depth tracking in recursion (hierarchy depth limit enforced elsewhere)
- Or increase to reasonable number without tracking

### 6. Clarify Epic vs Molecule Terminology

**Issue:** The system uses both "epic" and "molecule" for parent-child hierarchies:
- Epics: User-created work containers
- Molecules: System-created workflow instances

**Action:**
- Document the distinction clearly
- Use different `issue_type` values consistently
- Consider renaming molecule to `workflow-instance` or similar

---

## Low Priority Simplifications

### 7. Remove `EpicStatus` Wrapper Type

**Location:** `internal/types/types.go`

**Issue:** `EpicStatus` struct only adds 3 computed fields to `Issue`.

**Action:** Return `Issue` with computed fields via separate method.

### 8. Reduce Issue Type Proliferation

**Current Types:**
```
bug, feature, task, epic, chore, message, agent, role,
rig, skill, gate, molecule, merge-request
```

**Action:**
- Document which types support parent-child
- Consider type categories (work types, system types, meta types)
- Potentially consolidate rarely-used types

### 9. Remove Unused Fields from Epic Queries

**Potentially Unused:**
- `compaction_level`
- `content_hash`
- `work_type`
- `crystallizes`
- `quality_score`

**Action:** Audit field usage in epic-specific queries and remove if unused.

---

## Features That Can Likely Be Removed

### 1. `bd mol stale` Command

Duplicate of `bd epic status --eligible-only` functionality.

### 2. `OpMolStale` RPC Operation

The RPC operation mirrors CLI, adds overhead, appears under-utilized.

### 3. Deep Transitive Blocking Through Hierarchy

The recursive blocking through parent-child chains may be unused in practice. Simple direct blocking might suffice.

---

## Recommendations

| Change | Effort | Impact | Risk |
|--------|--------|--------|------|
| Remove `bd mol stale` | Low | High | Low |
| Simplify epic status query | Medium | Medium | Low |
| Remove `--eligible-only` | Low | Low | Low |
| Unify blocking logic | High | Medium | Medium |
| Remove depth tracking | Low | Low | Low |

**Start with:** Removing `bd mol stale` and the `--eligible-only` flag - quick wins with no risk.

**Then:** Simplify the epic status query for performance.

**Later:** Consider larger architectural changes to blocking logic.
