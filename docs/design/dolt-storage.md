# Dolt Storage Architecture

> **Status**: Canonical reference — consolidates all prior Dolt design docs
> **Date**: 2026-01-30
> **Context**: Dolt as the unified data layer for Beads and Gas Town
> **Consolidates**: DOLT-STORAGE-DESIGN.md, THREE-PLANES.md, dolt-integration-analysis-v{1,2}.md,
> dolt-license-analysis.md (all deleted; available in git history under ~/hop/docs/)
> **Key decisions**: SQLite retired. JSONL retired (interim backup only). Dolt is the
> only backend. Embedded by default, server optional. Dolt-in-git replaces JSONL for
> federation when it ships.

---

## Part 1: Architecture Decisions

### What's Settled

| Decision | Details |
|----------|---------|
| **Dolt is the only backend** | SQLite retired. No dual-backend. |
| **JSONL is not source of truth** | One-way backup export only (interim). Eliminated entirely by dolt-in-git. |
| **Embedded Dolt is default** | No server process needed. Pure-Go, single binary. |
| **Server mode is optional** | Available as upgrade for heavy concurrency. bd falls back to embedded if server isn't running. |
| **Single binary** | Pure-Go Dolt (`bd`). No CGO needed for local ops. |
| **Licensing** | Dolt is Apache 2.0, compatible with Beads/Gas Town MIT. Standard attribution. |

### Two-Tier Architecture

```
Default (most users):        Optional (heavy concurrency):
┌──────────────────┐         ┌──────────────────┐
│  Dolt embedded   │         │  Dolt SQL server  │
│  (in-process)    │         │  (separate proc)  │
│  lockfile+retry  │         │  multi-writer     │
└──────────────────┘         └──────────────────┘
                                     │
                             Falls back to embedded
                             if server not running
```

Embedded Dolt supports concurrent goroutine writes via standard SQL transaction
semantics (confirmed by Dustin Brown, Dolt engineer). The Dolt team is hardening
multi-writer support with lockfile + retry logic, upgradeable to r/w lock.

---

## Part 2: Three Data Planes

Beads serves three distinct data planes with different requirements. Collapsing
them into one transport (JSONL-in-git) is why scaling hurt.

### Plane 1: Operational

The "live game state" — work in progress, status changes, assignments, patrol
results, molecule transitions, heartbeats.

| Property | Value |
|----------|-------|
| Mutation rate | High (seconds) |
| Mutability | Fully mutable |
| Visibility | Local (town/rig) |
| Durability | Days to weeks |
| Federation | Not federated |
| Transport | **Dolt embedded or server** |

Forensics via `dolt_history_*` tables and `AS OF` queries replaces git-based
JSONL forensics. No git, no JSONL for this plane.

### Plane 2: Ledger

Completed work — the permanent record. Closed beads, validated deliverables.
Accumulates into CVs and skill vectors for HOP.

| Property | Value |
|----------|-------|
| Mutation rate | Low (task completion boundaries) |
| Mutability | Append-only |
| Visibility | Federated (cross-town) |
| Durability | Permanent |
| Transport | **Dolt-in-git** (when it ships) |

The compelling variant: **closed-beads-only export**. Only completed beads go to
the git history. Open/in-progress beads stay in the operational plane. This is
the squash analogy made literal — operational churn stays local, meaningful
completed units go to the permanent record.

### Plane 3: Design

Work imagined but not yet claimed — epics, RFCs, specs, plan beads. The "global
idea scratchpad" that needs maximum visibility and cross-town discoverability.

| Property | Value |
|----------|-------|
| Mutation rate | Conversational (minutes to hours) |
| Visibility | Global (maximally visible) |
| Durability | Until crystallized into operational work |
| Transport | **Dolt-in-git in shared repo** (The Commons, future) |

### The Lifecycle of Work

```
DESIGN PLANE                  OPERATIONAL PLANE              LEDGER PLANE
(global, collaborative)       (local, real-time)             (permanent, federated)

1. Epic created ──────────>
2. Discussed, refined
3. Subtask claimed ───────> 4. Work begins
                             5. Status changes (high freq)
                             6. Agent works, iterates
                             7. Work completes ────────────> 8. Curated record exported
                                                              9. Skills derived
                                                             10. CV accumulates
```

---

## Part 3: Dolt-in-Git — The JSONL Replacement

> **Status**: Dolt team actively building this (~1 week from 2026-01-30).

Instead of serializing Dolt data to JSONL for git transport, push Dolt's native
binary files directly into the git repo. Clone the repo, you have the code AND
the full queryable Dolt database.

### What Changes

```
BEFORE (JSONL era):
  Dolt DB ──serialize──> issues.jsonl ──git add──> GitHub
  GitHub  ──git pull───> issues.jsonl ──import──> Dolt DB
  (Two formats, bidirectional sync, merge conflicts on text)

AFTER (Dolt-in-git):
  Dolt DB ──git add──> GitHub (binary files)
  GitHub  ──git pull──> Dolt DB (binary files, cell-level merge)
  (One format, Dolt merge driver handles conflicts)
```

### Why This Is Strictly Better

| Dimension | JSONL-in-git | Dolt-in-git |
|-----------|-------------|-------------|
| Format translation | Serialize/deserialize every sync | None |
| Merge conflicts | Line-level text conflicts | Cell-level Dolt merge |
| Queryability after clone | Parse JSONL or import to DB | Query directly with `bd` |
| Two sources of truth | DB + JSONL can drift | One format everywhere |
| History/time-travel | Not available | Full Dolt history in binary |
| Size | Compact text | Larger, file-splitting handles 50MB limit |

### What This Eliminates

| Eliminated | Why |
|-----------|-----|
| JSONL entirely | Dolt binary IS the portable format |
| `bd daemon` for JSONL sync | No serialization layer |
| `bd sync` bidirectional | Dolt server handles concurrency |
| JSONL merge conflicts | Cell-level merge via Dolt merge driver |
| Two sources of truth | Dolt DB is the only source |
| 10% agent token tax | No sync overhead |
| Agents reading stale JSONL | JSONL doesn't exist to read |

### Technical Questions for Dolt Team

1. **Git merge driver**: How does cell-level merge work through git? Custom
   merge driver in `.gitattributes`?
2. **File splitting**: How does Dolt split to stay under GitHub's 50MB limit?
   Transparent to users?
3. **Partial export**: Can we export only closed beads to the git-tracked binary?
4. **Clone performance**: What does `git clone` look like with Dolt binary history?

---

## Part 4: Interim — Periodic JSONL Backup

Until dolt-in-git ships, JSONL serves one remaining purpose: **durable backup**
in case of disk crashes. The git-tracked JSONL files are the recovery path.

**What this means:**
- **One-way export only**: Dolt → JSONL, never JSONL → Dolt
- **Periodic, not real-time**: Schedule or manual trigger, not every mutation
- **Not source of truth**: If JSONL and Dolt disagree, Dolt wins
- **No import path**: `bd` never reads JSONL in dolt-native mode
- **Temporary**: Removed when dolt-in-git ships

**Implementation**: `bd export --jsonl` snapshots Dolt state to JSONL. Can use
`dolt_diff()` for incremental export. No daemon, no dirty tracking.

**What this does NOT mean:**
- No `bd daemon` for JSONL sync
- No `bd sync` bidirectional operations
- No JSONL import on clone
- No agents reading JSONL

---

## Part 5: What Dolt Unlocks

### Already Valuable for Beads

| Feature | What It Enables |
|---------|-----------------|
| Cell-level merge | Two agents update different fields → clean merge |
| `dolt_history_*` | Full row-level history, queryable via SQL |
| `AS OF` queries | "What did this look like yesterday?" |
| Branch isolation | Each polecat on own branch during work |
| `dolt_diff` | "What changed between these points?" → activity feeds |

### Unlocks for Gas Town

| Feature | What It Enables |
|---------|-----------------|
| SQL server mode | Multi-writer concurrency without daemon |
| Conflict-as-data | `dolt_conflicts` table, programmatic resolution |
| Schema versioning | Migrations travel with data |
| VCS stored procedures | `DOLT_COMMIT`, `DOLT_MERGE` as SQL |

### Unlocks for HOP (impossible with SQLite)

| Feature | What It Enables |
|---------|-----------------|
| Cross-time skill queries | "What Go work in Q4?" via `dolt_history` join |
| Federated validation | Pull remote ledger, query entity chains |
| Ledger compaction with proof | `dolt_history` proves faithful compaction |
| Native remotes | Push/pull database state for federation |

---

## Part 6: Gas Town Current State (2026-01-30)

### What's Working

- All 4 beads databases (town root, gastown, beads, wyvern) on embedded Dolt
- Creates persist, reads work, `gt ready` shows items across all rigs
- `dolt_mode: embedded` in all rigs (switched from `server` which required
  a running server process)

### Remaining Cleanup

See beads table for tracked issues:
- `gt-dolt-stale-jsonl` — bd blocks reads due to stale JSONL check
- `gt-dolt-fallback` — Graceful server-to-embedded fallback
- `gt-patrol-cleanup` — 153 patrol digest beads (pollution)
- `gt-sqlite-cleanup` — Remove stale SQLite databases
- `gt-misrouted-hq` — hq- beads in gastown JSONL
- `gt-dolt-metadata` — Fix Dolt version metadata
- `gt-dolt-lockfiles` — Stale Dolt LOCK files

### Architecture

```
~/gt/                           ← Town root
├── .beads/dolt/                ← Town Dolt DB (hq-* prefix)
├── gastown/.beads/dolt/        ← Rig Dolt DB (gt-* prefix)
├── beads/.beads/dolt/          ← Rig Dolt DB (bd-* prefix)
└── wyvern/.beads/dolt/         ← Rig Dolt DB (wy-* prefix)
```

Each rig has its own embedded Dolt database. Workers (polecats) in a rig share
the rig's Dolt DB via `metadata.json` pointing to the shared `dolt_path`.

---

## Part 7: Configuration

### metadata.json

```json
{
  "backend": "dolt",
  "dolt_mode": "embedded",
  "dolt_path": "/path/to/.beads/dolt",
  "sync": {
    "mode": "dolt-native"
  }
}
```

### Sync Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `dolt-native` | Pure Dolt, no JSONL | Gas Town, enterprise (current) |
| `git-portable` | Dolt + JSONL export on push | Beads Classic upgrade path |
| `dolt-in-git` | Dolt binary files in git | Future default (when shipped) |

### Conflict Resolution

Default: `newest` (most recent `updated_at` wins, like Google Docs).

Per-field strategies available:
- **Arrays** (labels, waiters): `union` merge
- **Counters** (compaction_level): `max`
- **Human judgment** (estimated_minutes): `manual`

---

## Part 8: Technical Details

### Dolt Commit Strategy

Default: auto-commit on every write (safe, auditable). Agents can batch:

```go
store.SetAutoCommit(false)
defer store.SetAutoCommit(true)
store.UpdateIssue(ctx, issue1)
store.UpdateIssue(ctx, issue2)
store.Commit(ctx, "Batch update: processed 2 issues")
```

This is ZFC-compliant: Go provides a safe default, agents can override.

### Incremental Export via dolt_diff()

No `dirty_issues` table needed. Dolt IS the dirty tracker:

1. Read last export commit from export state file
2. Query `dolt_diff_issues(last_commit, 'HEAD')` for changes
3. Apply changes to JSONL (upserts and deletions)
4. Update export state with current commit

Export state stored per-worktree to prevent polecats exporting each other's work.

### Multi-Table Schema

```sql
CREATE TABLE issues (
    id VARCHAR(64) PRIMARY KEY,
    type VARCHAR(32),
    title TEXT,
    description TEXT,
    status VARCHAR(32),
    priority INT,
    owner VARCHAR(255),
    assignee VARCHAR(255),
    labels JSON,
    parent VARCHAR(64),
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    closed_at TIMESTAMP
);

CREATE TABLE mail (
    id VARCHAR(64) PRIMARY KEY,
    thread_id VARCHAR(64),
    from_addr VARCHAR(255),
    to_addrs JSON,
    subject TEXT,
    body TEXT,
    sent_at TIMESTAMP,
    read_at TIMESTAMP
);

CREATE TABLE channels (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255),
    type VARCHAR(32),
    config JSON,
    created_at TIMESTAMP
);
```

### Bootstrap Flow

On first `bd` command in a fresh clone:
1. If Dolt DB exists → use it
2. If JSONL exists but no Dolt → import to new Dolt DB (legacy bootstrap)
3. If neither → create empty Dolt DB
4. When dolt-in-git ships: Dolt binary IS in the clone, no bootstrap needed

### Error Recovery

| Failure | Recovery |
|---------|----------|
| Crash during export | Re-run export (idempotent) |
| Dolt corruption | Rebuild from JSONL backup (interim) or git clone (dolt-in-git) |
| Merge conflict | Auto-resolve (newest wins) or `dolt_conflicts` table |

---

## Part 9: Dolt Team Clarifications

Direct answers from Tim Sehn (CEO) and Dustin Brown (engineer), January 2026.

### Concurrency

> **Dustin**: Concurrency with the driver is supported, multiple goroutines can
> write to the same embedded Dolt.
>
> **Tim**: Concurrency is handled by standard SQL transaction semantics.

### Scale

> **Tim**: Little scale impact from high commit rates. Don't compact before >1M
> commits. Run `dolt_gc()` when the journal file (`vvvvvvvvvvv...` in `.dolt/`)
> exceeds ~50MB.

### Branches

> **Tim**: Branches are just pointers to commits, like Git. Millions of branches
> without issue.

### Merge Performance

> **Tim**: We merge the Prolly Trees — much smarter/faster than sequential replay.
> See: https://www.dolthub.com/blog/2025-07-16-announcing-fast-merge/

### Replication

> **Tim**: All async, push/pull Git model not binlog. Can set up "push on write"
> or manual pushes. Works on dolt commits, not transaction commits.

### Hosting

> **Tim**: Hosted Dolt (like AWS RDS) starts at $50/month. DoltHub Pro (like
> GitHub) is free for first 1GB, $50/month + $1/GB after.
> See: https://www.dolthub.com/blog/2024-08-02-dolt-deployments/

---

## Part 10: Roadmap

### Immediate

1. **Dolt-in-git integration**: Dolt team delivering ~1 week from 2026-01-30.
   When ready, integrate into bd — replace JSONL with Dolt binary commits.
2. **Graceful server fallback** (`gt-dolt-fallback`): bd falls back from server
   to embedded when server isn't running.
3. **Gas Town pristine state**: Clean up patrol pollution, stale SQLite, misrouted
   beads, stale JSONL.

### Next

- Closed-beads-only ledger export
- Agent-managed Dolt migration flow for Beads users
- Ship `bd` release with pure-Go Dolt (single binary, works out of the box)

### Future

- Design Plane / The Commons architecture (with Brendan Hopper)
- Cross-town delegation via design plane
- Dolt server mode if concurrency demands emerge

---

## Decision Log

| Decision | Rationale | Date |
|----------|-----------|------|
| Dolt only, retire SQLite | One backend, better conflicts | 2026-01-15 |
| JSONL retired as source of truth | Dolt is truth; JSONL is interim backup | 2026-01-15 |
| Embedded Dolt default | No server process, just works | 2026-01-30 |
| Server mode optional | Available but not required; graceful fallback | 2026-01-30 |
| Single binary (pure-Go) | No CGO needed for local ops | 2026-01-30 |
| Dolt-in-git replaces JSONL | Native binary in git, cell-level merge | 2026-01-30 |
| Three data planes | Different data needs different transport | 2026-01-29 |
| Closed-beads-only ledger | Operational churn stays local | 2026-01-30 |
| Newest-wins conflict default | Matches Google Docs mental model | 2026-01-15 |
| Auto-commit per write | Safe default, agents can batch | 2026-01-15 |
| dolt_diff() for export | No dirty_issues table; Dolt IS the tracker | 2026-01-16 |
| Per-worktree export state | Prevent polecats exporting each other's work | 2026-01-16 |
| Apache 2.0 compatible with MIT | Standard attribution, no architectural impact | 2026-01-13 |
