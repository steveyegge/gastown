# Epic System Documentation

> Research document for hq-5881b3: Epic system research and improvements
> Created: 2026-01-24

## Overview

Epics in the beads/gastown ecosystem are organizational containers for related work items. They enable hierarchical task management, progress tracking, and coordinated multi-agent workflows.

## Core Concepts

### What is an Epic?

An epic is a bead with `issue_type = "epic"`. It serves as:
- A container for related tasks, bugs, and features
- A unit of progress tracking (children completion)
- An integration boundary (integration branches)
- A scope limiter for agent spawning

### Epic Hierarchy

```
Epic (bd-epic-123)
├── Task (bd-epic-123.1)
├── Task (bd-epic-123.2)
│   └── Subtask (bd-epic-123.2.1)
└── Bug (bd-epic-123.3)
```

- Maximum depth: 3 levels (configurable via `hierarchy.max-depth`)
- Child IDs auto-generated: `parent.N` where N increments
- Parent-child is a first-class dependency type

---

## Beads Implementation

### Database Schema

**Issues Table:**
```sql
CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    issue_type TEXT,  -- 'epic', 'task', 'bug', 'feature', 'chore'
    status TEXT,      -- 'open', 'in_progress', 'closed'
    ...
);
```

**Dependencies Table:**
```sql
CREATE TABLE dependencies (
    issue_id TEXT,      -- child
    depends_on_id TEXT, -- parent
    type TEXT,          -- 'parent-child', 'blocks', etc.
    PRIMARY KEY (issue_id, depends_on_id)
);
```

**Child Counters Table:**
```sql
CREATE TABLE child_counters (
    parent_id TEXT PRIMARY KEY,
    last_child INTEGER
);
```

### Dependency Types

| Type | Purpose |
|------|---------|
| `parent-child` | Hierarchical containment |
| `blocks` | Standard blocking dependency |
| `conditional-blocks` | Runs only if predecessor fails |
| `waits-for` | Fanout gate for dynamic children |

### Key Source Files (beads)

| File | Purpose |
|------|---------|
| `internal/types/types.go` | Type definitions (`TypeEpic`, `EpicStatus`, `DepParentChild`) |
| `internal/storage/sqlite/epics.go` | Progress calculations |
| `internal/storage/sqlite/dependencies.go` | Dependency creation/validation |
| `internal/storage/sqlite/schema.go` | Database schema with views |
| `cmd/bd/epic.go` | CLI epic commands |

---

## BD Commands for Epics

### Creating Epics

```bash
# Create a new epic
bd create "Q1 Roadmap" -t epic

# Create with description
bd create "Auth System Overhaul" -t epic -d "Complete rewrite of authentication"
```

### Creating Children

```bash
# Create child of epic (auto-generates hierarchical ID)
bd create "Design review" --parent=bd-abc-123
# Result: bd-abc-123.1

bd create "Implementation" --parent=bd-abc-123
# Result: bd-abc-123.2

# Create nested child
bd create "Unit tests" --parent=bd-abc-123.2
# Result: bd-abc-123.2.1
```

### Viewing Epics

```bash
# Show epic details with children
bd show bd-abc-123

# List all children of an epic
bd list --parent=bd-abc-123

# Show ready work within an epic
bd ready --parent=bd-abc-123
```

### Progress Tracking

```bash
# Show all epics with completion status
bd epic status

# Show only epics eligible for closure
bd epic status --eligible-only

# Output example:
# bd-abc-123 | Q1 Roadmap
# Progress: 2/3 children closed (67%)
```

### Closing Epics

```bash
# Auto-close epics where all children are complete
bd epic close-eligible

# Preview what would be closed
bd epic close-eligible --dry-run
```

---

## Gastown Integration

### Integration Branches

Epics can have associated integration branches for batching work:

```bash
# Create integration branch for epic
gt mq integration create bd-epic-123
# Creates: integration/bd-epic-123

# Submit PR to epic's integration branch
gt mq submit --epic=bd-epic-123

# Land integration branch to main
gt mq integration land bd-epic-123
```

**Auto-Detection:**
When submitting an MR for a child task, gastown auto-detects the parent epic and targets its integration branch if one exists.

### Convoy System

Convoys track work across epics:

```bash
# Create convoy tracking epic children
gt convoy create "Sprint 1 Work" --track bd-epic-123.1 --track bd-epic-123.2

# Auto-convoy on sling (if no existing convoy)
gt sling bd-epic-123.1 gastown
# Creates: hq-cv-xxx "Work: Design review"
```

**Convoy-Epic Relationship:**
- Convoys are persistent tracking units
- Epics are organizational containers
- Convoys can track issues from multiple epics
- Auto-close when all tracked issues complete

### Witness & Polecats

The Witness can be configured to auto-spawn polecats for epic work:

```go
type WitnessConfig struct {
    MaxWorkers int
    AutoSpawn  bool
    EpicID     string  // Limit spawning to this epic's children
}
```

### Sling with Epics

```bash
# Sling epic child to polecat
gt sling bd-epic-123.1 gastown

# Sling with formula (creates ephemeral wisp)
gt sling mol-bugfix gastown --var epic=bd-epic-123
```

---

## Progress Calculation

### Algorithm

```sql
WITH epic_children AS (
    SELECT d.depends_on_id AS epic_id, i.id AS child_id, i.status
    FROM dependencies d
    JOIN issues i ON i.id = d.issue_id
    WHERE d.type = 'parent-child'
)
SELECT epic_id,
       COUNT(*) AS total_children,
       SUM(CASE WHEN status = 'closed' THEN 1 ELSE 0 END) AS closed_children
FROM epic_children
GROUP BY epic_id
```

### EpicStatus Structure

```go
type EpicStatus struct {
    Epic             *Issue
    TotalChildren    int
    ClosedChildren   int
    EligibleForClose bool  // true when all children closed
}
```

### Ready Work Calculation

An epic (or child) is "ready" when:
1. Status is `open` or `in_progress`
2. Has no open blocking dependencies
3. Parent (if any) is not blocked

Blocking propagates down: if parent is blocked, children are blocked.

---

## Validation Rules

### Parent-Child Constraints

1. **Parent must exist** before creating child
2. **No self-dependencies** (can't be own parent)
3. **No cycles** (A→B→C→A not allowed)
4. **No backwards parent-child** (parent can't depend on child)
5. **Maximum depth** enforced (default: 3)

### Status Transitions

```
open → in_progress → closed
         ↓
       open (reopen)
```

Epics can be closed manually or via `bd epic close-eligible`.

---

## Cross-Rig Epics

### Routing by Prefix

```bash
# Beads rig prefix
bd show bd-epic-123

# Gastown rig prefix
bd show gt-epic-456

# Town-level (HQ) prefix
bd show hq-epic-789
```

Routes defined in `~/gt/.beads/routes.jsonl`.

### External References

For tracking cross-rig issues in convoys:
```
external:gastown:gt-xyz
```

---

## Example Workflow

```bash
# 1. Create epic for new feature
bd create "User Authentication System" -t epic
# → bd-auth-001

# 2. Break down into tasks
bd create "Design auth flow" --parent=bd-auth-001
# → bd-auth-001.1
bd create "Implement login endpoint" --parent=bd-auth-001
# → bd-auth-001.2
bd create "Implement logout endpoint" --parent=bd-auth-001
# → bd-auth-001.3
bd create "Add session management" --parent=bd-auth-001
# → bd-auth-001.4

# 3. Create integration branch
gt mq integration create bd-auth-001

# 4. Work on tasks (polecats or crew)
bd update bd-auth-001.1 --status=in_progress
# ... do design work ...
bd close bd-auth-001.1

# 5. Submit PRs to integration branch
gt mq submit --epic=bd-auth-001

# 6. Check progress
bd epic status
# bd-auth-001 | User Authentication System
# Progress: 1/4 children closed (25%)

# 7. When all children done, close epic
bd epic close-eligible

# 8. Land integration branch
gt mq integration land bd-auth-001
```

---

## Key Design Principles

1. **MEOW (Molecular Expression of Work)**: Epics decompose large goals into trackable atomic units
2. **Hierarchical IDs**: Child IDs encode parentage for easy navigation
3. **Progress Rollup**: Epic completion tracks child completion
4. **Integration Branches**: Batch related work before landing to main
5. **Non-Blocking Tracking**: Convoys track without blocking completion

---

## Related Documentation

- Beads CLI: `bd help epic`
- Gastown CLI: `gt help mq integration`
- Convoy system: `gt help convoy`
