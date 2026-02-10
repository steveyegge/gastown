# K8s Rig & Workspace Architecture

**Epic**: bd-chv7
**Status**: Design
**Date**: 2026-02-10

## Problem Statement

Classical Gas Town assumes a shared filesystem: all agents (mayor, polecats, crew, witness, refinery) share a single machine with a bare repo (`.repo.git/`) and use git worktrees. In K8s, each agent runs in its own pod with its own filesystem. We need to:

1. Register rigs in the pod workspace so `gt rig list` and `gt prime` work
2. Give each pod access to the rig's source code
3. Enable cross-rig issue routing when pods can't see each other's filesystems
4. Support the full polecat lifecycle: clone → branch → code → commit → push → MR

## Current State (What Works)

- **Mayor pod**: `gt-town-mayor-hq` runs with empty `rigs.json`, daemon-connected, no rigs registered
- **Controller**: Watches agent beads (`gt:agent` + `execution_target:k8s`), creates pods with `GT_RIG`, `GT_ROLE`, `GT_AGENT` env vars; has `PodDefaults` merge hierarchy (town < rig < role < pool) but no rig-level config source yet
- **Daemon**: Single Dolt database, all rig prefixes coexist; `CreateArgs.TargetRig` resolves via route beads (works); `ListArgs.TargetRig` and `MoveArgs.TargetRig` walk filesystem (broken in K8s)
- **Entrypoint**: Creates workspace structure, connects to daemon via `gt connect`, materializes hooks, starts coop+Claude restart loop
- **PVCs**: ReadWriteOnce (gp2) for persistent roles, EmptyDir for polecats

## Classical Gas Town Assumptions That Break in K8s

| Assumption | Classical | K8s Reality |
|-----------|-----------|-------------|
| Shared filesystem | `.repo.git/` bare repo shared by refinery + polecats | Each pod has isolated filesystem |
| Refinery sees polecat branches | Shared bare repo refs are instant | Must push to remote first |
| `routes.jsonl` + `.beads/redirect` | Filesystem walk for routing | Pod can't see other pods' dirs |
| tmux session management | `tmux has-session` for lifecycle | Coop HTTP API (already solved) |
| File-based IPC | Handoff files, mail dirs, lock files | Daemon RPC (already solved for mail) |
| Directory-based role detection | Parse CWD relative path | `GT_ROLE`/`GT_RIG` env vars |
| Single-host orchestration | `gt rig boot/stop` manages all agents | Controller + agent beads |
| `workspace.Find()` walks up | Finds `mayor/town.json` marker | Already works (entrypoint creates it) |

## Design Decisions

### Q1: Does each rig get its own namespace or share gastown-uat?

**Decision: Shared namespace with label-based isolation.**

All rig pods share `gastown-uat`. Reasons:
- The daemon is a single service; separate namespaces add network policy complexity
- Controller already uses `gastown.io/rig` labels for pod selection
- Secrets (Claude credentials, daemon token) are namespace-scoped; sharing avoids duplication
- K8s RBAC is already configured for `gastown-uat`
- Pod naming convention already includes rig: `gt-{rig}-{role}-{name}`

### Q2: How do crew/polecats get their git worktrees in K8s?

**Decision: Clone-on-boot from in-cluster git-mirror Service.**

The key architectural tension: classical Gas Town uses a shared bare repo (`.repo.git/`) so the refinery can see polecat branches instantly without a push. In K8s, we must choose between:

1. **Shared PVC (ReadWriteMany)** — mirrors classical model but requires EFS/NFS, has concurrent git lock issues, and is a SPOF
2. **Per-pod clone** — each pod gets its own isolated clone; refinery can't see polecat branches locally

**We choose per-pod clone** with a workflow change: polecats push branches to origin immediately after creating them (not just at `gt done` time). This lets the refinery `git fetch` to discover branches. The trade-off is ~1-2s latency for branch visibility instead of instant, but eliminates the entire shared PVC complexity.

```
┌─────────────────────────────────────────────────┐
│  git-mirror-{rig} Service (Deployment + PVC)    │
│  ┌────────────────────────────────────────────┐  │
│  │  Bare repo mirror of rig's git remote      │  │
│  │  Periodic: git fetch --all (every 60s)     │  │
│  │  Serves: git://git-mirror-{rig}:9418       │  │
│  └────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
         ▲ fast LAN clone (<5s)
         │
┌────────┴────────────────────────────────────────┐
│  Agent Pod (polecat/crew/refinery)              │
│  ┌──────────────────────────────────────┐       │
│  │ init-clone container:                 │       │
│  │   git clone git://git-mirror-{rig}   │       │
│  │     → /home/agent/gt/{rig}/work/      │       │
│  └──────────────────────────────────────┘       │
│  ┌──────────────────────────────────────┐       │
│  │ agent container:                      │       │
│  │   git checkout -b polecat/{name}      │       │
│  │   git push -u origin polecat/{name}   │  ← early push │
│  │   <do work, commit, push>             │       │
│  └──────────────────────────────────────┘       │
└─────────────────────────────────────────────────┘
```

**Per-role strategy:**

| Role | Clone | Storage | Branch | Push | Notes |
|------|-------|---------|--------|------|-------|
| Polecat | Full from mirror | EmptyDir | `polecat/{name}` | Early push + at done | Ephemeral; push branch immediately after creation |
| Crew | Full from mirror | PVC (persists) | Any | Standard | Long-lived workspace |
| Refinery | Full from mirror | PVC (persists) | `main` | Merge to main | Fetches polecat branches from remote |
| Witness | No clone | PVC (state only) | N/A | N/A | Only monitors, no code needed |
| Mayor | No clone (town-level) | PVC | N/A | N/A | Town-level, no rig code |

**Refinery workflow change** (the critical delta from classical):
- Classical: `BranchExists()` checks local refs in shared bare repo
- K8s: `BranchExists()` must check remote refs → `git ls-remote origin polecat/{name}` or `git fetch origin && git branch -r`
- The refinery `doDirectMerge()` already calls `FetchOrigin()` before merge — just need `BranchExists` to also check remote

**Why not shared PVC?**
- ReadWriteMany requires EFS/NFS — adds cost and latency (~5-10ms per git op)
- Concurrent `git worktree add` modifies `$GIT_DIR/worktrees/` — race conditions
- Git lock files (`.lock`) on shared PVC can block all pods on crash
- Single bare repo is a SPOF — corruption affects all agents
- The refinery branch-visibility benefit can be achieved with early-push + fetch

### Q3: How does .beads/redirect work when agents are in separate pods?

**Decision: Eliminate .beads/redirect in K8s; use single-database model with prefix filtering.**

The routing researcher found a critical insight: the daemon currently operates in two models:

**Model A (classical, multi-DB):** Each rig has its own `.beads` directory and database. Cross-rig operations open separate storage connections via filesystem paths. Used by `ListArgs.TargetRig`, `MoveArgs`, `RefileArgs`.

**Model B (K8s, single-DB):** All rigs share one Dolt database. Prefix-based ID generation distinguishes rigs. Route beads in the same DB provide prefix mapping. Used by `CreateArgs.TargetRig` and `CreateArgs.Prefix`.

**The problem:** List/Move/Refile TargetRig still use Model A (filesystem) even when the rest of the system uses Model B.

**The fix is small:** In single-DB mode, cross-rig operations don't need to open separate storage. They just need prefix-based filtering:
- `bd list --rig beads` → filter by `bd-` prefix in the same database
- `bd move gt-abc beads` → create new `bd-*` issue, close `gt-abc`, same database
- `bd refile gt-abc beads` → same pattern

**What needs to change in beads daemon:**
```go
// server_issues_epics.go — handleList with TargetRig
if listArgs.TargetRig != "" {
    if s.isSingleDBMode() {
        // Single-DB: resolve rig to prefix, filter by prefix
        prefix, err := s.resolveRigToPrefix(listArgs.TargetRig)
        if err != nil { return errorResponse(err) }
        listArgs.PrefixFilter = prefix  // NEW field
    } else {
        // Multi-DB: open separate storage (existing code)
        targetBeadsDir, _, err := resolveTargetRig(req, listArgs.TargetRig)
        // ...
    }
}
```

**What becomes unnecessary in K8s:**
- `routes.jsonl` file (route beads in DB serve the same purpose)
- `.beads/redirect` files (all routing via single daemon)
- `connectToRoutedDaemon()` (Unix socket routing)
- `findTownBeadsDir()` filesystem walk (when `BEADS_DOLT_SERVER_MODE=1`)

### Q4: How does workspace.Find() work when town root is a PVC mount?

**Decision: Town root is always `/home/agent/gt` — already works.**

The entrypoint creates `mayor/town.json` at `/home/agent/gt/mayor/town.json`. `workspace.Find()` walks up from CWD and finds it. No changes needed.

Rig directories live under the workspace:
```
/home/agent/gt/                    ← town root (PVC mount)
├── mayor/town.json                ← workspace marker (exists)
├── mayor/rigs.json                ← rig registry (currently empty!)
├── .beads/config.yaml             ← daemon connection (exists)
├── CLAUDE.md                      ← town identity (exists)
├── {rig}/                         ← NEW: rig directory
│   ├── config.json                ← rig config (prefix, repo URL)
│   └── work/                      ← git clone (init container)
└── .state/                        ← session persistence (exists)
```

### Q5: Should agents clone the rig repo on startup or use a shared volume?

**Decision: Clone on boot from in-cluster git-mirror Service (see Q2).**

### Q6: How do polecats (ephemeral) get code context without a full clone?

**Decision: Full clone from git-mirror (fast enough).**

The git-mirror is an in-cluster bare repo that stays synced with the remote. Cloning from it over cluster networking takes <5s for typical repos. For very large repos (>1GB), the init container can use `--depth=50` for a shallow clone, deepened on demand with `git fetch --unshallow`.

## Rig as a First-Class Beads Type

**Proposal: Make `rig` a proper beads type** to eliminate the last filesystem dependency.

Currently, rig registration lives on the filesystem (`rigs.json` + `config.json` + `routes.jsonl`). Making `rig` a first-class type moves it to the database:

```
type: rig
title: beads
labels:
  - prefix:bd
  - git_url:https://github.com/groblegark/beads
  - git_mirror:git-mirror-beads
  - default_branch:main
  - state:active
```

**What this eliminates:**
- `mayor/rigs.json` → `bd list --type=rig` from daemon
- `{rig}/config.json` → metadata on the rig bead
- `routes.jsonl` → derive from rig beads (prefix label → database mapping)
- `.beads/redirect` → daemon knows all databases

**What this enables:**
- `gt rig list` works on any pod (queries daemon)
- Controller reads rig metadata from beads (git_url, mirror service name)
- `resolveTargetRig()` queries rig beads instead of walking filesystem
- Rig lifecycle (active/parked/docked) is native bead state, not label convention
- `PodDefaults` merge hierarchy can source rig-level config from rig beads

**Migration:** Rig beads already exist informally (rig identity beads, rig registry config beads). Formalizing the type adds validation and a dedicated resolution path in the daemon.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                       gastown-uat namespace                       │
│                                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐   │
│  │ Dolt     │  │ Daemon   │  │ Redis    │  │ Controller    │   │
│  │ (2 rep)  │  │ (2 rep)  │  │          │  │               │   │
│  └──────────┘  └──────────┘  └──────────┘  └───────┬───────┘   │
│       ▲              ▲                              │            │
│       │              │ daemon RPC                   │ reconcile  │
│       │    ┌─────────┴──────────┐                   │            │
│       │    │                    │                   ▼            │
│  ┌────┴────┴──┐  ┌─────────────┴──┐  ┌─────────────────────┐   │
│  │ Mayor Pod  │  │ Polecat Pod    │  │ git-mirror-beads    │   │
│  │ (town)     │  │ (rig: beads)   │  │ (bare repo, 1 rep)  │   │
│  │            │  │ init: clone ←──┼──┤ git://...:9418      │   │
│  │ rig beads  │  │ work/          │  └─────────────────────┘   │
│  │ routes     │  │ .beads/config  │                             │
│  └────────────┘  └────────────────┘  ┌─────────────────────┐   │
│                                       │ git-mirror-gastown  │   │
│  ┌────────────┐  ┌────────────────┐  │ (bare repo, 1 rep)  │   │
│  │ Crew Pod   │  │ Refinery Pod   │  └─────────────────────┘   │
│  │ (rig: bd)  │  │ (rig: bd)     │                             │
│  │ PVC clone  │  │ PVC clone     │                             │
│  └────────────┘  └────────────────┘                             │
└──────────────────────────────────────────────────────────────────┘

Data flow:
  Controller reads rig beads from daemon → knows git_mirror service name
  Controller creates pods with init container → clone from git-mirror
  Agent pods read/write issues via daemon RPC → single DB, prefix routing
  Polecats push branches to origin → refinery fetches from origin
```

## Implementation Plan

### Phase 1: Rig Type in Beads

Add `rig` as a recognized issue type in beads with required metadata fields.

**beads changes:**
- Add `rig` to valid issue types
- `resolveTargetRig()` in daemon: when `BEADS_DOLT_SERVER_MODE=1`, query rig beads by type=rig + label match instead of walking filesystem
- `handleList()` with TargetRig in single-DB mode: prefix-filter instead of opening separate storage
- `handleMove()`/`handleRefile()` in single-DB mode: same-database prefix swap

**gastown changes:**
- `gt rig register` command: creates rig bead in daemon, no filesystem clone
  - Writes minimal `config.json` locally (for `workspace.Find` compatibility)
  - Creates rig bead (type=rig) with labels: prefix, git_url, git_mirror, default_branch, state
  - Route resolution derived from rig bead prefix label (no separate route bead needed)
- `gt rig list` in daemon mode: query rig beads from daemon instead of reading `rigs.json`

### Phase 2: Git Mirror Service

Helm chart for in-cluster git mirrors — one Deployment+Service per rig.

**Components:**
- `git-mirror-{rig}` Deployment: init clone + periodic fetch loop + `git daemon`
- `git-mirror-{rig}` Service: ClusterIP on port 9418
- Small PVC per mirror (1-5Gi bare repo)
- For private repos: Secret mount with SSH key or git-credentials

**Triggered by:** rig bead creation (controller watches for type=rig beads and provisions mirrors)

### Phase 3: Pod Init Container for Code Access

Controller adds init-clone container to pods that need code.

**podmanager changes:**
- Read `git_mirror` label from rig bead metadata
- For roles needing code (polecat/crew/refinery): add `init-clone` init container
- Init container: `git clone git://git-mirror-{rig}:9418/{rig}.git /workspace/{rig}/work`
- Configure git user/email from `GT_AGENT` and `GT_ROLE`

**entrypoint changes:**
- After daemon connect: `gt rig register` for each rig in `GT_RIGS` env var (idempotent)
- For roles with code: `cd` into `{rig}/work/` before starting Claude
- Git remote set to actual origin (GitHub) for push; mirror only used for initial clone

### Phase 4: Refinery Branch Visibility

Adapt refinery to work without shared bare repo.

**git package changes:**
- `BranchExists()`: in K8s mode, check remote refs (`git ls-remote`) when local check fails
- Or: add `RemoteBranchExists()` and use in refinery merge path

**polecat changes:**
- Early branch push: after `git checkout -b polecat/{name}`, immediately `git push -u origin polecat/{name}`
- This makes the branch visible to refinery via `git fetch origin`

**refinery changes:**
- `doDirectMerge()` already calls `FetchOrigin()` — just needs remote-aware branch check
- Merge flow: fetch → check remote branch → checkout → merge/squash → push

### Phase 5: Controller Rig-Aware Pod Provisioning

Controller reads rig beads for per-rig pod customization.

**Changes:**
- Controller watches type=rig beads alongside type=agent beads
- Populates `PodDefaults` rig-level overrides from rig bead metadata
- Per-rig image, resources, secrets, storage class via rig bead labels
- Auto-provision git-mirror when new rig bead appears

## Helm Values Structure

```yaml
# values/gastown-uat.yaml additions
rigs:
  - name: beads
    url: https://github.com/groblegark/beads
    prefix: bd
    mirror:
      enabled: true
      storage: 2Gi
      fetchInterval: 60
    credentials:
      secretName: git-credentials-beads
  - name: gastown
    url: https://github.com/steveyegge/gastown
    prefix: gt
    mirror:
      enabled: true
      storage: 1Gi
      fetchInterval: 60
```

## Open Questions

1. **Git push credentials**: Mount SSH key Secret or use git-credentials-store with PAT? SSH key is simpler but PAT is more portable.
2. **Large repos**: For repos >1GB, use partial clone (`--filter=blob:none`) or shallow (`--depth=50`)?
3. **Mirror HA**: Single-replica git mirror is SPOF. Add readiness probe + PDB? Or rely on K8s restart?
4. **Refinery as pod**: Persistent pod (always running) or event-driven Job (spawned when MR arrives)?
5. **Rig bead schema**: What fields are required vs optional? How does migration work for existing rig identity beads?
