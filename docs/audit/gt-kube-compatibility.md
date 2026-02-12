# GT CLI Kubernetes Compatibility Audit

**Date**: 2026-02-12
**Scope**: All 263 files in `internal/cmd/`, 132 cobra command registrations
**Bead**: bd-svwfd

## Summary

| Category | Count | % |
|----------|-------|---|
| K8s-Ready (works as-is) | ~15 | 11% |
| Partially Ready (has K8s paths, needs work) | ~40 | 30% |
| Local-Only (needs complete rewrite for K8s) | ~77 | 59% |

## Critical Blocker Patterns

| Pattern | Occurrences | Files | Severity |
|---------|-------------|-------|----------|
| `workspace.FindFromCwd()` | ~95 | 87 | CRITICAL |
| `tmux.NewTmux()` + session ops | ~75 | 62 | CRITICAL |
| `beads.New()` local store | ~65 | 58 | CRITICAL |
| `filepath.Join()` workspace paths | ~50 | 45 | HIGH |

## K8s-Ready Patterns (Already Working)

| Pattern | Occurrences | Files |
|---------|-------------|-------|
| `os.Getenv()` | ~120 | 95 |
| `bdcmd.Command()` (delegates to bd) | ~28 | 25 |
| `backend.HasSession()` | ~12 | 8 |
| `terminal.ResolveBackend()` | ~8 | 5 |
| `registry.SessionRegistry` | ~3 | 2 |

---

## Command-by-Command Analysis

### K8s-Ready Commands

| Command | Notes |
|---------|-------|
| `gt connect` | K8s-native design (discoverK8sDaemon, extractK8sToken) |
| `gt mail *` | 100% daemon RPC (mail.NewRouter) |
| `gt inject *` | Session-local JSONL queue, fully portable |
| `gt role` | Reads GT_ROLE, GT_RIG env vars |
| `gt version` | Portable |
| `gt toolchain *` | K8s-only (runs in agent pods) |
| `gt bootstrap` | Daemon RPC when connected |

### Partially Ready Commands

| Command | % Ready | Blockers |
|---------|---------|----------|
| `gt nudge` | 70% | `nudgeViaBackend()` not impl in sling.go |
| `gt sling` | 60% | workspace.FindFromCwd(), beads.New() |
| `gt done` | 60% | tmux.KillSessionWithProcesses() |
| `gt prime` | 60% | workspace.FindFromCwd(), state.State() |
| `gt hook` | 60% | workspace.FindFromCwd(), beads.New() |
| `gt status` | 50% | tmux.ListSessions() (registry exists but underused) |
| `gt agents` | 50% | tmux.ListSessions() (K8s merge started) |
| `gt convoy` | 50% | workspace.FindFromCwd(), beads.New() |
| `gt formula *` | 50% | beads.New() (formulas now in DB via bootstrap) |

### Local-Only Commands (Need Redesign)

| Command | Critical? | Notes |
|---------|-----------|-------|
| `gt down` | YES | 0% ready, all shutdown logic is tmux |
| `gt up` | YES | tmux session startup |
| `gt doctor` | No | Diagnostic, filesystem-heavy |
| `gt polecat *` | No | tmux session management |
| `gt crew *` | No | tmux session management |
| `gt witness` | No | tmux manager |
| `gt refinery` | No | tmux manager |
| `gt mayor` | No | tmux session |
| `gt deacon` | No | tmux session |

---

## Architecture Gaps

### 1. Workspace Discovery (95 call sites)

**Current**: `workspace.FindFromCwd()` walks filesystem upward to find town root.

**K8s**: Agent pods know their context from env vars (GT_ROLE, GT_RIG, GT_AGENT, GT_TOWN_ROOT). No filesystem walk needed.

**Fix**: Add `resolveContext()` helper that checks env vars first, falls back to workspace.FindFromCwd() only for local mode. ~87 files need update.

### 2. Session Management (75 tmux calls)

**Current**: Direct `tmux.NewTmux()` calls for session CRUD.

**K8s**: Coop backend via `terminal.Backend` interface (17 methods, 3 impls).

**Status**: Phase 2-3 complete (backend + registry), Phase 5 partial (status/agents/down). ~62 files still call tmux directly.

**Fix**: Route all session operations through `terminal.Backend`. Phase 5 pattern in status.go is the template.

### 3. Local Store Access (65 call sites)

**Current**: `beads.New(cwd)` creates local .beads store for reading/writing.

**K8s**: Daemon RPC (daemonClient) handles all beads operations.

**Status**: Many commands already have `if daemonClient != nil` branches. ~58 files still create local stores.

**Fix**: In K8s mode, always use daemon RPC. The `if daemonClient != nil` pattern is correct but needs consistent application.

### 4. Session Lifecycle (up/down/restart)

**Current**: `gt up` starts tmux sessions, `gt down` kills them.

**K8s**: Session lifecycle is pod lifecycle (bead create → controller → pod create; bead close → pod delete).

**Fix**: `gt up/down` become no-ops or wrappers around bead operations in K8s mode. SessionRegistry already has CreateSession/DestroySession.

---

## Recommended Migration Order

### Phase 1: Quick Wins (unblock daily driver)
1. Add env-var-first `resolveContext()` to bypass workspace.FindFromCwd()
2. Implement `nudgeViaBackend()` stub in sling.go
3. Route `gt down` through Backend.KillSession() instead of tmux

### Phase 2: Critical Path Commands
1. `gt sling` → daemon RPC for beads, backend for session
2. `gt done` → backend for session kill, daemon for bead updates
3. `gt hook` → daemon RPC for beads access
4. `gt prime` → daemon RPC for state/context

### Phase 3: Discovery Commands
1. `gt status` → use SessionRegistry.DiscoverAll() exclusively
2. `gt agents` → use SessionRegistry for all session data

### Phase 4: Lifecycle Commands
1. `gt up` → bead create + controller for K8s
2. `gt down` → bead close for K8s
3. Manager commands (polecat, crew, etc.) → K8s-only via controller

### Phase 5: Cleanup
1. Delete tmux package (3,659 lines)
2. Delete TmuxBackend, SSHBackend
3. Remove all workspace.FindFromCwd() calls (use resolveContext())
