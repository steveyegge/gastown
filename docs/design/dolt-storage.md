# Dolt Storage Architecture

> **Status**: Current reference for Gas Town agents
> **Updated**: 2026-02-28
> **Context**: Dolt is the sole storage backend for Beads and Gas Town

---

## Overview

Gas Town uses [Dolt](https://github.com/dolthub/dolt), an open-source
SQL database with Git-like versioning (Apache 2.0). One Dolt SQL server
per town serves all databases via MySQL protocol on port 3307. There is
no embedded mode and no SQLite. JSONL is used only for disaster-recovery
backups (the JSONL Dog exports scrubbed snapshots every 15 minutes to a
git-backed archive), not as a primary storage format.

The `gt daemon` manages the server lifecycle (auto-start, health checks
every 30s, crash restart with exponential backoff).

## Server Architecture

```
Dolt SQL Server (one per town, port 3307)
├── hq/       town-level beads  (hq-* prefix)
├── gastown/  rig beads         (gt-* prefix)
├── beads/    rig beads         (bd-* prefix)
├── wyvern/   rig beads         (wy-* prefix)
└── sky/      rig beads         (sky-* prefix)
```

**Data directory**: `~/gt/.dolt-data/` — each subdirectory is a database
accessible via `USE <name>` in SQL.

**Connection**: `root@tcp(127.0.0.1:3307)/<database>` (no password for
localhost).

## Commands

```bash
# Daemon manages server lifecycle (preferred)
gt daemon start

# Manual management
gt dolt start          # Start server
gt dolt stop           # Stop server
gt dolt status         # Health check, list databases
gt dolt logs           # View server logs
gt dolt sql            # Open SQL shell
gt dolt init-rig <X>   # Create a new rig database
gt dolt list           # List all databases
```

If the server isn't running, `bd` fails fast with a clear message
pointing to `gt dolt start`.

## Write Concurrency: All-on-Main

All agents — polecats, crew, witness, refinery, deacon — write directly
to `main`. Concurrency is managed through transaction discipline: every
write wraps `BEGIN` / `DOLT_COMMIT` / `COMMIT` atomically.

```
bd update <bead> --status=in_progress
  → BEGIN
  → UPDATE issues SET status='in_progress' ...
  → CALL DOLT_COMMIT('-Am', 'update status')
  → COMMIT
```

This eliminates the former branch-per-worker strategy (BD_BRANCH,
per-polecat Dolt branches, merge-at-done). All writes are immediately
visible to all agents — no cross-agent visibility gaps.

Multi-statement `bd` commands batch their writes inside a single
transaction to maintain atomicity.

## Schema

Schema version 6. The full schema lives in `beads/.../storage/dolt/schema.go`.
Key tables shown below; see source for indexes and full column lists.

```sql
-- Core: every bead is a row in issues (tasks, messages, agents, gates, etc.)
CREATE TABLE issues (
    id VARCHAR(255) PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'open',
    priority INT NOT NULL DEFAULT 2,
    issue_type VARCHAR(32) NOT NULL DEFAULT 'task',
    assignee VARCHAR(255),
    owner VARCHAR(255) DEFAULT '',
    sender VARCHAR(255) DEFAULT '',          -- messaging
    mol_type VARCHAR(32) DEFAULT '',         -- molecule type
    work_type VARCHAR(32) DEFAULT 'mutex',   -- mutex vs open_competition
    hook_bead VARCHAR(255) DEFAULT '',       -- agent hook
    role_bead VARCHAR(255) DEFAULT '',       -- agent role
    agent_state VARCHAR(32) DEFAULT '',      -- agent lifecycle
    wisp_type VARCHAR(32) DEFAULT '',        -- TTL-based compaction class
    metadata JSON DEFAULT (JSON_OBJECT()),   -- extensible metadata
    created_at DATETIME, updated_at DATETIME, closed_at DATETIME
    -- ... plus ~20 more columns (see schema.go)
);

-- Relationships between beads
CREATE TABLE dependencies (
    issue_id VARCHAR(255) NOT NULL,
    depends_on_id VARCHAR(255) NOT NULL,
    type VARCHAR(32) NOT NULL DEFAULT 'blocks',   -- blocks, parent-child, thread
    PRIMARY KEY (issue_id, depends_on_id)
);

-- Labels (many-to-many)
CREATE TABLE labels (
    issue_id VARCHAR(255) NOT NULL,
    label VARCHAR(255) NOT NULL,
    PRIMARY KEY (issue_id, label)
);

-- Audit trail
CREATE TABLE comments (id BIGINT AUTO_INCREMENT PRIMARY KEY, issue_id, author, text, created_at);
CREATE TABLE events   (id BIGINT AUTO_INCREMENT PRIMARY KEY, issue_id, event_type, actor, old_value, new_value, created_at);

-- Agent interaction log
CREATE TABLE interactions (id, kind, actor, issue_id, model, prompt, response, created_at);

-- Infrastructure
CREATE TABLE config          (key PRIMARY KEY, value);       -- runtime config knobs
CREATE TABLE metadata        (key PRIMARY KEY, value);       -- schema version, etc.
CREATE TABLE routes          (prefix PRIMARY KEY, path);     -- prefix→database routing
CREATE TABLE issue_counter   (prefix PRIMARY KEY, last_id);  -- sequential ID generation
CREATE TABLE child_counters  (parent_id PRIMARY KEY, last_child);
CREATE TABLE federation_peers (name PRIMARY KEY, remote_url, sovereignty, last_sync);

-- Compaction
CREATE TABLE issue_snapshots     (id, issue_id, compaction_level, original_content, ...);
CREATE TABLE compaction_snapshots (id, issue_id, compaction_level, snapshot_json, ...);
CREATE TABLE repo_mtimes         (repo_path PRIMARY KEY, mtime_ns, last_checked);
```

**Wisps** (ephemeral patrol data) reuse the same `issues` table with
`wisp_type` set. They are Dolt-ignored (`dolt_ignore` table) so wisp
mutations don't generate Dolt commits — only structural changes to the
ignore config itself are committed.

**Mail** is implemented as beads with `issue_type='message'` in the
issues table — there is no separate mail table. The `sender` field and
`dependencies` (type='thread') provide threading.

## Dolt-Specific Capabilities

These are available to agents via SQL and used throughout Gas Town:

| Feature | Usage |
|---------|-------|
| `dolt_history_*` tables | Full row-level history, queryable via SQL |
| `AS OF` queries | Time-travel: "what did this look like yesterday?" |
| `dolt_diff()` | "What changed between these two points?" |
| `DOLT_COMMIT` | Explicit commit with message (auto-commit is the default) |
| `DOLT_MERGE` | Merge branches (integration branches, federation) |
| `dolt_conflicts` table | Programmatic conflict resolution after merge |
| `DOLT_BRANCH` | Create/delete branches (integration branches) |

**Auto-commit** is on by default: every write gets a Dolt commit. Agents
can batch writes by disabling auto-commit temporarily.

**Conflict resolution** default: `newest` (most recent `updated_at` wins).
Arrays (labels): `union` merge. Counters: `max`.

## Three Data Planes

Beads data falls into three planes with different characteristics:

| Plane | What | Mutation | Durability | Transport | Status |
|-------|------|----------|------------|-----------|--------|
| **Operational** | Work in progress, status, assignments, heartbeats | High (seconds) | Days–weeks | Dolt SQL server (local) | **Live** |
| **Ledger** | Completed work, permanent record | Low (completion boundaries) | Permanent | JSONL export → git push to GitHub | **Live** |
| **Design** | Epics, RFCs, specs — ideas not yet claimed | Conversational | Until crystallized | DoltHub commons (shared) | **Planned** |

The operational plane lives entirely in the local Dolt server. The ledger
plane is currently served by the JSONL Dog, which exports scrubbed snapshots
to a git-backed archive every 15 minutes — this is the durable record that
survives disasters (proven in Clown Show #13). The design plane will
federate via DoltHub as part of the Wasteland commons (in progress).

## Data Lifecycle: Think Git, Not SQL (CRITICAL)

Dolt is git under the hood. **The commit graph IS the storage cost, not the
rows.** Every `bd create`, `bd update`, `bd close` generates a Dolt commit.
DELETE a row and the commit that wrote it still exists in history. `dolt gc`
reclaims unreferenced chunks, but the commit graph itself grows forever.

This is the key insight from Tim Sehn (Dolt founder, 2026-02-27):

> "Your Beads databases are small but your commit history is big."
>
> "If you delete a bead you want to rebase with the commit that wrote it
> so it just isn't there any more in history."

**Rebase** (`CALL DOLT_REBASE()`, available since v1.81.2) rewrites the
commit graph — it's the real cleanup mechanism. DELETE + gc is necessary
but insufficient. DELETE + rebase + gc is the full pipeline.

Reference: https://www.dolthub.com/blog/2026-01-28-everybody-rebase/

### The Six-Stage Lifecycle

```
CREATE → LIVE → CLOSE → DECAY → COMPACT → FLATTEN
  │        │       │        │        │          │
  Dolt   active   done   DELETE   REBASE     SQUASH
  commit  work    bead    rows    commits    all history
                         >7-30d  together   to 1 commit
```

| Stage | Owner | Frequency | Mechanism |
|-------|-------|-----------|-----------|
| CREATE | Any agent | Continuous | `bd create`, `bd mol wisp create` |
| CLOSE | Agent or patrol | Per-task | `bd close`, `gt done` |
| DECAY | Reaper Dog | Daily | `DELETE FROM wisps WHERE status='closed' AND age > 7d` |
| COMPACT | Compactor Dog | Daily | `CALL DOLT_REBASE()` — squash old commits together |
| FLATTEN | Mayor or manual | Monthly | Branch, soft-reset to initial commit, commit, swap main |

All six stages are implemented. DECAY runs in the Reaper Dog (wisp_reaper.go),
COMPACT/FLATTEN run in the Compactor Dog (compactor_dog.go).

### Two Data Streams

```
EPHEMERAL (wisps, patrol data)          PERMANENT (issues, molecules, agents)
  CREATE                                  CREATE
  → work                                  → work
  → CLOSE (>24h)                          → CLOSE
  → DELETE rows (Reaper)                  → JSONL export (scrubbed)
  → REBASE history (Compactor)            → git push to GitHub
  → gc unreferenced chunks (Compactor)    → COMPACT history periodically
                                          → FLATTEN history quarterly
```

**Ephemeral data** (wisps, wisp_events, wisp_labels, wisp_deps) is
high-volume patrol exhaust. Valuable in real-time, worthless after 24h.
The Reaper Dog DELETES the rows. The Compactor Dog REBASES the commits
that wrote them out of history. Without both, storage grows without bound.

**Permanent data** (issues, molecules, agents, dependencies, labels) is
the ledger. Even permanent data benefits from history compaction — a bead
that was created, updated 5 times, and closed generates 7 commits that
can be rebased into 1. The data survives; the intermediate history doesn't.

### History Compaction Operations

**Daily compaction** (Compactor Dog):
```sql
-- Squash recent commits on a feature branch, then merge
CALL DOLT_CHECKOUT('-b', 'compact-temp');
CALL DOLT_REBASE('--interactive', 'main~50', 'HEAD');
-- Agent resolves: squash old commits, keep recent ones
CALL DOLT_CHECKOUT('main');
CALL DOLT_MERGE('compact-temp');
CALL DOLT_BRANCH('-d', 'compact-temp');
```

**Monthly flatten** (nuclear option from Tim Sehn):
```bash
# Squash ALL history to a single commit
cd ~/gt/.dolt-data/<db>
dolt checkout -b fresh-start
dolt reset --soft $(dolt log --oneline | tail -1 | awk '{print $1}')
dolt commit -m "Flatten: squash history to single commit"
dolt branch -D main
dolt branch -m fresh-start main
dolt gc
```

### Dolt GC

`dolt gc` compacts old chunk data AFTER rebase removes commits from the
graph. Run gc after rebase, not instead of it. The Compactor Dog runs gc
automatically after each successful compaction. Order matters: rebase
first, gc second.

```bash
# Manual gc (stop server first for exclusive access)
gt dolt stop
cd ~/gt/.dolt-data/<db> && dolt gc
gt dolt start
```

### Pollution Prevention

Pollution enters Dolt via four vectors:

1. **Commit graph growth**: Every mutation = a commit. Rebase compacts.
2. **Mail pollution**: Agents overuse `gt mail send` for routine comms.
   Use `gt nudge` (ephemeral, zero Dolt cost) instead. See mail-protocol.md.
3. **Test artifacts**: Test code creating issues on production server.
   Firewall in store.go refuses test-prefixed CREATE DATABASE on port 3307.
4. **Zombie processes**: Test dolt-server processes that outlive tests.
   Doctor Dog kills these. 45 zombies (7GB RAM) found and killed 2026-02-27.

Prevention is layered:
- **Prompting**: Agents prefer `gt nudge` over `gt mail send` (zero commits)
- **Firewall** (store.go): refuses test-prefixed CREATE DATABASE on port 3307
- **Reaper Dog**: DELETEs closed wisps, auto-closes stale issues
- **Compactor Dog**: REBASEs old commits to compress history, runs gc after
- **Doctor Dog**: kills zombie servers, detects orphan DBs, monitors health
- **JSONL Dog**: scrubs exports, rejects pollution, spike-detects before commit
- **Janitor Dog**: cleans test server (port 3308)

### Communication Hygiene (Reducing Commit Volume)

Every `gt mail send` creates a bead + Dolt commit. Every `gt nudge`
creates nothing. The rule:

**Default to `gt nudge`. Only use `gt mail send` when the message MUST
survive the recipient's session death.**

| Role | Mail budget | Nudge for everything else |
|------|-------------|--------------------------|
| Polecat | 0-1 per session (HELP only) | Status, questions, updates |
| Witness | Protocol messages only | Health checks, polecat pokes |
| Refinery | Protocol messages only | Status to Witness |
| Deacon | Escalations only | Timer callbacks, health pokes |
| Dogs | Zero (never mail) | DOG_DONE via nudge to Deacon |

## Standalone Beads Note

The `bd` CLI retains an embedded Dolt option for standalone use (outside
Gas Town). Server-only mode applies to Gas Town exclusively — standalone
users may not have a Dolt server running.

## Remote Push (Git Protocol)

Gas Town pushes Dolt databases to GitHub remotes via `gt dolt sync`. These
use git SSH protocol (`git+ssh://git@github.com/...`), not DoltHub's native
protocol.

### Git Remote Cache

Dolt maintains a cache at `~/gt/.dolt-data/<db>/.dolt/git-remote-cache/` that
stores git objects built from Dolt's internal format. Per the Dolt team
(Dustin Brown, 2026-02-26):

- **The cache is necessary** — Dolt uses it to build git objects for push/pull
- **Accumulates garbage** (orphaned refs) and is not cleaned up automatically
- **Safe to delete** between pushes, but causes a full rebuild on next push
  (beads: ~20 min rebuild, gastown: even longer)
- **Orphaned refs** can be pruned without deleting the whole cache — better balance
- **Grows over time** as the database grows — inherent to git-protocol remotes

**Guidance**: Do NOT routinely delete the cache. Prefer pruning orphaned refs.
Full deletion should only be done when disk pressure is critical and a long
rebuild is acceptable.

### Sync Procedure

`gt dolt sync` parks all rigs (stops witnesses/refineries), stops the Dolt
server, runs `dolt push` for each database with a configured remote, then
restarts the server and unparks rigs. The parking prevents witnesses from
detecting the server outage and restarting it mid-push.

### Force Push

After data recovery (e.g., Clown Show #13), local and remote histories
diverge. Use `gt dolt sync --force` for the first push to overwrite the
remote with local state. Subsequent pushes should work without `--force`.

### Known Limitations

- **Slow**: Git-protocol remotes are orders of magnitude slower than DoltHub
  native remotes. A 71MB database takes ~90s; larger ones take 20+ minutes.
- **Cache growth**: No automatic garbage collection. Orphan pruning TBD.
- **Server downtime**: Push requires exclusive access to the data directory,
  so the server must be stopped during push. This creates a maintenance window.

### DoltHub Remotes (In Progress)

DoltHub's native protocol (`https://doltremoteapi.dolthub.com/...`) avoids
the git-remote-cache entirely and is much faster. Julian Knutsen is
implementing DoltHub-based federation as part of the Wasteland commons —
this will replace git-protocol remotes for the design and ledger planes.
Migration requires DoltHub accounts and reconfiguring remotes with
`dolt remote set-url`.

## File Layout

```
~/gt/                            Town root
├── .dolt-data/                  Centralized Dolt data directory
│   ├── hq/                      Town beads (hq-*)
│   ├── gastown/                 Gastown rig (gt-*)
│   ├── beads/                   Beads rig (bd-*)
│   ├── wyvern/                  Wyvern rig (wy-*)
│   └── sky/                     Sky rig (sky-*)
├── daemon/
│   ├── dolt.pid                 Server PID (daemon-managed)
│   ├── dolt.log                 Server log
│   └── dolt-state.json          Server state
└── mayor/
    └── daemon.json              Daemon config (dolt_server section)
```
