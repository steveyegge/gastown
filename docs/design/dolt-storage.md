# Dolt Storage Architecture

> **Status**: Current reference for Gas Town agents
> **Updated**: 2026-02-16
> **Context**: Dolt is the sole storage backend for Beads and Gas Town

---

## Overview

Gas Town uses [Dolt](https://github.com/dolthub/dolt), an open-source
SQL database with Git-like versioning (Apache 2.0). One Dolt SQL server
per town serves all databases via MySQL protocol on port 3307. There is
no embedded mode, no SQLite, and no JSONL.

The `gt daemon` manages the server lifecycle (auto-start, health checks
every 30s, crash restart with exponential backoff).

## Server Architecture

```
Dolt SQL Server (one per town, port 3307)
├── hq/       town-level beads  (hq-* prefix)
├── gastown/  rig beads         (gt-* prefix)
├── beads/    rig beads         (bd-* prefix)
└── ...       additional rigs
```

**Data directory**: `~/.dolt-data/` — each subdirectory is a database
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

| Plane | What | Mutation | Durability | Transport |
|-------|------|----------|------------|-----------|
| **Operational** | Work in progress, status, assignments, heartbeats | High (seconds) | Days–weeks | Dolt SQL server (local) |
| **Ledger** | Completed work, permanent record, skill vectors | Low (completion boundaries) | Permanent | DoltHub remotes + federation |
| **Design** | Epics, RFCs, specs — ideas not yet claimed | Conversational | Until crystallized | DoltHub commons (shared) |

The operational plane lives entirely in the local Dolt server. The ledger
and design planes federate via DoltHub using the Highway Operations
Protocol — Gas Town's public federation layer built on Dolt's native
push/pull remotes.

## Standalone Beads Note

The `bd` CLI retains an embedded Dolt option for standalone use (outside
Gas Town). Server-only mode applies to Gas Town exclusively — standalone
users may not have a Dolt server running.

## File Layout

```
~/gt/                            Town root
├── .dolt-data/                  Centralized Dolt data directory
│   ├── hq/                      Town beads (hq-*)
│   ├── gastown/                 Gastown rig (gt-*)
│   ├── beads/                   Beads rig (bd-*)
│   └── wyvern/                  Wyvern rig (wy-*)
├── daemon/
│   ├── dolt.pid                 Server PID (daemon-managed)
│   ├── dolt-server.log          Server log
│   └── dolt-state.json          Server state
└── mayor/
    └── daemon.json              Daemon config (dolt_server section)
```
