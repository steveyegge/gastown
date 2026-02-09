# Dolt Storage Architecture

> **Status**: Canonical reference — consolidates all prior Dolt design docs
> **Date**: 2026-02-05
> **Context**: Dolt as the unified data layer for Beads and Gas Town
> **Consolidates**: DOLT-STORAGE-DESIGN.md, THREE-PLANES.md, dolt-integration-analysis-v{1,2}.md,
> dolt-license-analysis.md (all deleted; available in git history under ~/hop/docs/)
> **Key decisions**: SQLite retired. JSONL retired (interim backup only). Dolt is the
> only backend. Server mode is **required** (embedded mode fully removed — no fallback).
> Dolt-in-git replaces JSONL for federation when it ships.

---

## Part 1: Architecture Decisions

### What's Settled

| Decision | Details |
|----------|---------|
| **Dolt is the only backend** | SQLite retired. No dual-backend. |
| **JSONL is not source of truth** | One-way backup export only (interim). Eliminated entirely by dolt-in-git. |
| **Dolt Server is required** | One server per town, serving all rig databases. No embedded fallback. |
| **Embedded mode removed** | File-level locking causes hangs under concurrent load. Removed entirely — not kept as fallback. |
| **Single binary** | Pure-Go Dolt (`bd`). No CGO needed for local ops. |
| **Licensing** | Dolt is Apache 2.0, compatible with Beads/Gas Town MIT. Standard attribution. |

### Server Mode Architecture

```
┌─────────────────────────────────┐
│  Dolt SQL Server (per town)     │
│  - Port 3307                    │
│  - Serves all rig databases     │
│  - Multi-client concurrency     │
│  - Managed by gt daemon         │
│  - Auto-start, auto-restart     │
└─────────────────────────────────┘
           │
           ├── hq/       (town-level beads, hq-* prefix)
           ├── gastown/  (gt-* prefix)
           ├── beads/    (bd-* prefix)
           └── ...       (other rigs)
```

All `bd` commands connect via MySQL protocol. There is no embedded fallback.
If the server is not running, `bd` fails fast with a clear error message
pointing the user to `gt dolt start`.

### Why Embedded Mode Was Removed

Embedded Dolt uses file-level locking (noms LOCK). In multi-agent environments,
this causes severe problems:

- `gt status` spawns 40+ `bd` processes to check all rigs
- Each process contends for the same lock file
- Processes hang indefinitely waiting for locks
- A semaphore hack (MaxConcurrentBd=3) serializes access but kills parallelism
- Even read-only operations acquire exclusive locks in the embedded driver

Embedded was initially kept as a fallback, but this created complexity for no
benefit: if the server is down, the data lives on the server's data directory
(`~/.dolt-data/`), so embedded can't access it anyway. Removing embedded entirely
enables significant code simplification (see Part 11).

### Server Topology Options

| Topology | Use Case |
|----------|----------|
| **One server per town** | Default. Single server at `~/gt/.dolt-data/` serves hq + all rigs. Simple operations. |
| **One server per rig** | Isolation between rigs. Useful if rigs have vastly different load patterns or need independent lifecycle. |

Gas Town currently uses one server per town. Per-rig servers are available if
isolation requirements emerge.

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
| Transport | **Dolt SQL Server** |

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

### Unlocks for Gas Town (Now Active)

| Feature | What It Enables |
|---------|-----------------|
| **SQL server mode** | Multi-writer concurrency — the solution to embedded mode's lock contention |
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

## Part 6: Gas Town Current State (2026-02-05)

### What's Working

- Dolt SQL Server as the **only** access method — embedded mode removed
- Centralized data directory at `~/gt/.dolt-data/` with per-rig subdirectories
- `gt daemon` auto-starts, monitors, and auto-restarts the Dolt server
- Server commands: `gt dolt start`, `gt dolt stop`, `gt dolt status`, `gt dolt logs`
- 5 concurrent `bd` processes tested with zero contention
- Creates persist, reads work, `gt ready` shows items across all rigs

### Server Management

```bash
# Daemon manages server lifecycle automatically (preferred)
gt daemon start     # Daemon auto-starts Dolt server

# Manual management (for debugging or one-off use)
gt dolt start       # Start the Dolt SQL server (port 3307)
gt dolt stop        # Stop the server
gt dolt status      # Check server status, list databases
gt dolt logs        # View server logs
gt dolt sql         # Open SQL shell
gt dolt init-rig X  # Initialize a new rig database
gt dolt list        # List all rig databases
gt dolt migrate     # Migrate from old .beads/dolt/ layout
```

### Architecture

```
~/gt/                           ← Town root
├── .dolt-data/                 ← Centralized Dolt data directory
│   ├── hq/                     ← Town beads (hq-* prefix)
│   ├── gastown/                ← Gastown rig (gt-* prefix)
│   ├── beads/                  ← Beads rig (bd-* prefix)
│   └── wyvern/                 ← Wyvern rig (wy-* prefix)
├── daemon/
│   ├── dolt.pid                ← Server PID file (daemon-managed)
│   ├── dolt-server.log         ← Server log
│   └── dolt-state.json         ← Server state
├── mayor/
│   └── daemon.json             ← Daemon config (dolt_server section)
└── [rigs]/                     ← Rig directories (code, not data)
```

The Dolt server runs with `--data-dir ~/.dolt-data`, making each subdirectory
a separate database accessible via `USE <rigname>` in SQL. The daemon ensures
the server is running on every heartbeat (3-minute interval) and auto-restarts
on crash.

---

## Part 7: Configuration

### Server Configuration

The Dolt server is configured via `gt dolt` commands. Key settings:

| Setting | Default | Description |
|---------|---------|-------------|
| Port | 3307 | MySQL protocol port (avoids conflict with MySQL on 3306) |
| User | root | Default Dolt user (no password for localhost) |
| Data dir | `~/.dolt-data/` | Contains all rig databases |
| Log file | `~/gt/daemon/dolt.log` | Server log output |
| PID file | `~/gt/daemon/dolt.pid` | Process ID for management |

### Connection String

```
root@tcp(127.0.0.1:3307)/        # Server root
root@tcp(127.0.0.1:3307)/gastown # Specific rig database
```

### Sync Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `dolt-native` | Pure Dolt server, no JSONL | Gas Town (current default) |
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

**Gas Town (existing install — migration from embedded):**
1. Run `gt dolt migrate` to move town-level `.beads/dolt/` to `~/.dolt-data/hq/`
2. Manually move rig-level databases: `mv <rig>/mayor/rig/.beads/dolt/beads ~/.dolt-data/<rigname>`
3. Update all `metadata.json` files: `dolt_mode: "server"`, `dolt_database: "<rigname>"`
4. Enable `dolt_server` in `mayor/daemon.json`, restart daemon

**Fresh Gas Town install:**
1. `gt dolt init-rig hq` — initialize town-level database
2. `gt dolt init-rig <rigname>` — initialize per-rig databases
3. Enable `dolt_server` in `mayor/daemon.json`
4. `gt daemon start` — daemon auto-starts the Dolt server

**Fresh Beads install (standalone):**
1. `dolt sql-server --port 3307 --data-dir <path>` — start server
2. `bd` connects via MySQL protocol, creates database and schema automatically

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

### Completed

- **Dolt Server mode**: Required for all access. Commands: `gt dolt start/stop/status`
- **Centralized data directory**: `~/.dolt-data/` with per-rig subdirectories
- **Migration tooling**: `gt dolt migrate` + manual moves for rig-level databases
- **Daemon integration**: Dolt server auto-starts/stops/restarts via `gt daemon`
- **All 4 databases migrated**: hq (4197), beads (2468), gastown (1053), wyvern

### Immediate

1. **Branch-per-polecat for write concurrency** (gt-twqgs): Each polecat gets a
   Dolt branch at sling time. Zero contention at 50 concurrent writers (tested).
   Merge to main at completion. See Part 12.
2. **Remove embedded mode from bd** (see Part 11): Major code simplification.
3. **Dolt-in-git integration**: Dolt team delivering soon.
   When ready, integrate into bd — replace JSONL with Dolt binary commits.
4. **Gas Town pristine state**: Clean up old `.beads/dolt/` directories, stale
   SQLite, misrouted beads, stale JSONL.

### Next

- Closed-beads-only ledger export
- Agent-managed Dolt migration flow for Beads users
- Ship `bd` release (server-only, no embedded driver → smaller binary)
- Per-rig server option for isolation (if demand emerges)

### Future

- Design Plane / The Commons architecture (with Brendan Hopper)
- Cross-town delegation via design plane

---

## Decision Log

| Decision | Rationale | Date |
|----------|-----------|------|
| Dolt only, retire SQLite | One backend, better conflicts | 2026-01-15 |
| JSONL retired as source of truth | Dolt is truth; JSONL is interim backup | 2026-01-15 |
| ~~Embedded Dolt default~~ | ~~No server process, just works~~ | ~~2026-01-30~~ |
| **Server mode is default** | Embedded file locking causes hangs under multi-agent concurrency | 2026-02-05 |
| **Embedded mode removed entirely** | No fallback — data lives on server, embedded can't access it. Enables major code simplification. | 2026-02-05 |
| **Daemon manages Dolt server** | Auto-start on heartbeat, auto-restart on crash, graceful shutdown | 2026-02-05 |
| **One server per town** | Centralized `.dolt-data/` serves all rigs; simple ops, single process | 2026-02-05 |
| Single binary (pure-Go) | No CGO needed for local ops | 2026-01-30 |
| Dolt-in-git replaces JSONL | Native binary in git, cell-level merge | 2026-01-30 |
| Three data planes | Different data needs different transport | 2026-01-29 |
| Closed-beads-only ledger | Operational churn stays local | 2026-01-30 |
| Newest-wins conflict default | Matches Google Docs mental model | 2026-01-15 |
| Auto-commit per write | Safe default, agents can batch | 2026-01-15 |
| dolt_diff() for export | No dirty_issues table; Dolt IS the tracker | 2026-01-16 |
| Per-worktree export state | Prevent polecats exporting each other's work | 2026-01-16 |
| Apache 2.0 compatible with MIT | Standard attribution, no architectural impact | 2026-01-13 |
| **Branch-per-polecat** | Per-worker Dolt branches eliminate optimistic lock contention at 50+ concurrent writers. Tested 2026-02-08. | 2026-02-08 |

---

## Part 12: Branch-Per-Polecat (Write Concurrency Fix)

> Added 2026-02-08 by Mayor. Stress test evidence in `~/gt/mayor/dolt-branch-test.go`.

### The Problem

Dolt's optimistic locking causes `Error 1105: optimistic lock failed on database Root
update` when multiple agents commit to the same branch concurrently. At 20 concurrent
writers on `main`, 50% fail. The Phase 0 band-aid (10 retries with exponential backoff)
helps but doesn't solve the architectural ceiling.

### The Fix

Each polecat gets its own Dolt branch. Branches are independent Root pointers — no
contention between branches. Merges are sequential (refinery or gt done).

```
gt sling <bead> <rig>
  └─ CALL DOLT_BRANCH('polecat-furiosa-1707350000')
     └─ Polecat env: BD_BRANCH=polecat-furiosa-1707350000
        └─ bd connects, runs: CALL DOLT_CHECKOUT('polecat-furiosa-1707350000')
           └─ All bd creates/updates/closes write to polecat branch
              └─ Zero contention with other polecats

gt done
  └─ CALL DOLT_CHECKOUT('main')
     └─ CALL DOLT_MERGE('polecat-furiosa-1707350000')
        └─ CALL DOLT_BRANCH('-D', 'polecat-furiosa-1707350000')
```

### Stress Test Results

| Concurrency | Single Branch (main) | Per-Worker Branches | Sequential Merge |
|-------------|---------------------|--------------------|-----------------|
| 10 | 10/10 (100%) | 10/10 (100%) | 10/10 (100%) |
| 20 | 10/20 (50%) | **20/20 (100%)** | 20/20 (100%) |
| 50 | 25/50 (50%) | **50/50 (100%)** | 50/50 (100%) |

Each worker performed 5 insert+commit cycles. All workers launched simultaneously
via barrier. 50 workers = 250 total Dolt commits, all successful, in 2 seconds.
Sequential merge of all 50 branches completed in 312ms.

### Why This Works

Tim Sehn (Dolt CEO): "Branches are just pointers to commits, like Git. Millions of
branches without issue." And: "We merge the Prolly Trees — much smarter/faster than
sequential replay."

Each branch has its own Root. DOLT_COMMIT on branch A doesn't touch branch B's Root.
The optimistic lock only fires when two writers try to update the SAME Root. With
per-polecat branches, this never happens.

### Implementation (Gas Town side)

1. `gt sling` (internal/polecat/spawn.go): After worktree creation, create Dolt branch
   via SQL: `CALL DOLT_BRANCH('polecat-<name>-<timestamp>')`
2. Set `BD_BRANCH` env var in the polecat's tmux session
3. `gt done` flow: merge branch to main, delete branch
4. `gt polecat nuke`: delete branch as part of cleanup (idempotent)

### Implementation (Beads side)

1. `store.go`: On connection open, check `BD_BRANCH` env var
2. If set, run `CALL DOLT_CHECKOUT('<branch>')` on the connection
3. All subsequent operations happen on that branch transparently
4. No other bd code changes needed — SQL operations are branch-agnostic

### Merge Conflicts

Conflicts should be rare: each polecat works on different issues (different rows).
If conflicts occur (e.g., two polecats update the same parent epic's child count):
- Dolt's `dolt_conflicts` table captures them
- `newest-wins` resolution applies (our default)
- Worst case: retry the merge after resolving

### Relationship to AT War Rigs

Dolt branches and AT War Rigs are orthogonal solutions to different problems:
- **Branches**: Solve write contention at the storage layer (launch-track)
- **AT War Rigs**: Solve coordination overhead at the session layer (post-launch)

Both could coexist. With branches, AT War Rigs become less urgent — the Dolt
contention ceiling is removed regardless of how sessions are managed.

---

## Part 11: Code Simplification (Embedded Removal)

Removing embedded mode entirely enables significant cleanup across the `bd` codebase.
This is not incremental — it's a wholesale removal of a code path that no longer executes.

### What Gets Removed

| Component | File | What |
|-----------|------|------|
| **Embedded Dolt driver** | `go.mod` | `go-dolt` dependency — largest single dep in the binary |
| **Advisory lock layer** | `access_lock.go` | Entire file: shared/exclusive flock, `AcquireAccessLock()`, `dolt-access.lock` |
| **Embedded connection** | `store.go` | `openEmbeddedConnection()`, `withEmbeddedDolt()`, embedded backoff/retry |
| **UOW1/UOW2 init path** | `store.go` | Embedded-only `CREATE DATABASE` + schema init via embedded driver |
| **Server fallback** | `factory_dolt.go` | `isServerConnectionError()` fallback to embedded (lines 39-55) |
| **JSONL bootstrap** | `factory_dolt.go` | `bootstrapEmbeddedDolt()`, `hasDoltSubdir()` |
| **Read-only distinction** | `main.go` | `isReadOnlyCommand()` map — server handles concurrency natively |
| **Semaphore hacks** | gt hooks, `main.go` | `MaxConcurrentBd=3` (G1/G5) — no contention with server |
| **Lock timeout config** | `main.go` | 5s/15s read/write timeouts — no advisory locks |
| **`BD_SKIP_ACCESS_LOCK`** | `store.go` | Debug env var for bypassing flock |
| **Embedded build tags** | Various | `//go:build cgo` guards |

### Impact

| Metric | Before | After (estimated) |
|--------|--------|-------------------|
| Binary size | ~120MB (embedded Dolt engine) | ~20MB (MySQL client only) |
| Build time | ~90s (CGO, Dolt compilation) | ~15s (pure Go, no CGO) |
| `store.go` complexity | Two code paths (embedded + server) | One code path (server only) |
| Lock-related code | ~300 lines across 4 files | 0 |
| External deps | go-dolt + go-sql-driver/mysql | go-sql-driver/mysql only |

### What Stays

- `openServerConnection()` in `store.go` — the MySQL connection path
- `initSchemaOnDB()` — schema creation (runs via MySQL now)
- `dolt.Config` struct — simplified (remove `Path`, `OpenTimeout`, embedded fields)
- `metadata.json` config — `dolt_mode` field becomes vestigial (always server)
- JSONL export (`bd sync --flush-only`) — interim backup until dolt-in-git ships

### Migration Path

1. Remove embedded code paths from `store.go` and `factory_dolt.go`
2. Remove `access_lock.go` entirely
3. Remove `go-dolt` from `go.mod`
4. Remove CGO build tags
5. Simplify `main.go` — remove `isReadOnlyCommand()`, lock timeout logic
6. Remove semaphore infrastructure from gt hooks
7. Update `metadata.json` handling — `dolt_mode: "server"` becomes the only valid value
8. Clean up old `.beads/dolt/` directories and `dolt-access.lock` files
