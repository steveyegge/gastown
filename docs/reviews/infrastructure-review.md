# Infrastructure & Utilities Code Review

**Review ID**: gt-a02fj.8
**Date**: 2026-01-04
**Reviewer**: gastown/polecats/interceptor (polecat gus)

## Executive Summary

Reviewed 14 infrastructure packages for dead code, missing abstractions, performance concerns, and error handling consistency. Found significant cleanup opportunities totaling ~44% dead code in constants package and an entire unused package (keepalive).

---

## 1. Dead Code Inventory

### Critical: Entire Package Unused

| Package | Status | Recommendation |
|---------|--------|----------------|
| `internal/keepalive/` | 100% unused | **DELETE ENTIRE PACKAGE** |

The keepalive package (5 functions) was removed from the codebase on Dec 30, 2025 as part of the shift to feed-based activation. No imports exist anywhere.

### High Priority: Functions to Remove

| Package | Function | Location | Notes |
|---------|----------|----------|-------|
| `config` | `NewExampleAgentRegistry()` | agents.go:361-381 | Zero usage in codebase |
| `constants` | `DirMayor`, `DirPolecats`, `DirCrew`, etc. | constants.go:32-59 | 9 unused directory constants |
| `constants` | `FileRigsJSON`, `FileTownJSON`, etc. | constants.go:62-74 | 4 unused file constants |
| `constants` | `BranchMain`, `BranchBeadsSync`, etc. | constants.go:77-89 | 4 unused branch constants |
| `constants` | `RigBeadsPath()`, `RigPolecatsPath()`, etc. | constants.go | 5 unused path helper functions |
| `doctor` | `itoa()` | daemon_check.go:93-111 | Duplicate of `strconv.Itoa()` |
| `lock` | `DetectCollisions()` | lock.go:367-402 | Superseded by doctor checks |
| `events` | `BootPayload()` | events.go:186-191 | Never called |
| `events` | `TypePatrolStarted`, `TypeSessionEnd` | events.go:50,54 | Never emitted |
| `events` | `VisibilityBoth` | events.go:32 | Never set |
| `boot` | `DeaconDir()` | boot.go:235-237 | Exported but never called |
| `dog` | `IdleCount()`, `WorkingCount()` | manager.go:532-562 | Inlined in callers |

### Medium Priority: Duplicate Definitions

| Package | Item | Duplicate Location | Action |
|---------|------|-------------------|--------|
| `constants` | `RigSettingsPath()` | Also in config/loader.go:673 | Remove from constants |
| `util` | Atomic write pattern | Also in mrqueue/, wisp/ | Consolidate to util |
| `doctor` | `findRigs()` | 3 identical implementations | Extract shared helper |

---

## 2. Utility Consolidation Plan

### Pattern: Atomic Write (Priority: HIGH)

**Current state**: Duplicated in 3+ locations
- `util/atomic.go` (canonical)
- `mrqueue/mrqueue.go` (duplicate)
- `wisp/io.go` (duplicate)
- `polecat/pending.go` (NON-ATOMIC - bug!)

**Action**:
1. Fix `polecat/pending.go:SavePending()` to use `util.AtomicWriteJSON`
2. Replace inline atomic writes in mrqueue and wisp with util calls

### Pattern: Rig Discovery (Priority: HIGH)

**Current state**: 7+ implementations scattered across doctor package
- `BranchCheck.findPersistentRoleDirs()`
- `OrphanSessionCheck.getValidRigs()`
- `PatrolMoleculesExistCheck.discoverRigs()`
- `config_check.go.findAllRigs()`
- Multiple `findCrewDirs()` implementations

**Action**: Create `internal/workspace/discovery.go`:
```go
type RigDiscovery struct { ... }
func (d *RigDiscovery) FindAllRigs() []string
func (d *RigDiscovery) FindCrewDirs(rig string) []string
func (d *RigDiscovery) FindPolecatDirs(rig string) []string
```

### Pattern: Clone Validation (Priority: MEDIUM)

**Current state**: Duplicate logic in doctor checks
- `rig_check.go`: Validates .git, runs git status
- `branch_check.go`: Similar traversal logic

**Action**: Create `internal/workspace/clone.go`:
```go
type CloneValidator struct { ... }
func (v *CloneValidator) ValidateClone(path string) error
func (v *CloneValidator) GetCloneInfo(path string) (*CloneInfo, error)
```

### Pattern: Tmux Session Handling (Priority: MEDIUM)

**Current state**: Fragmented across lock, doctor, daemon
- `lock/lock.go`: `getActiveTmuxSessions()`
- `doctor/identity_check.go`: Similar logic
- `cmd/agents.go`: Uses `tmux.NewTmux()`

**Action**: Consolidate into `internal/tmux/sessions.go`

### Pattern: Load/Validate Config Files (Priority: LOW)

**Current state**: 8 near-identical Load* functions in config/loader.go
- `LoadTownConfig`, `LoadRigsConfig`, `LoadRigConfig`, etc.

**Action**: Create generic loader using Go generics:
```go
func loadConfigFile[T Validator](path string) (*T, error)
```

### Pattern: Math Utilities (Priority: LOW)

**Current state**: `min()`, `max()`, `min3()`, `abs()` in suggest/suggest.go

**Action**: If needed elsewhere, move to `internal/util/math.go`

---

## 3. Performance Concerns

### Critical: File I/O Per-Event

| Package | Issue | Impact | Recommendation |
|---------|-------|--------|----------------|
| `events` | Opens/closes file for every event | High on busy systems | Batch writes or buffered logger |
| `townlog` | Opens/closes file per log entry | Medium | Same as events |
| `events` | `workspace.FindFromCwd()` on every Log() | Low-medium | Cache town root |

### Critical: Process Tree Walking

| Package | Issue | Impact | Recommendation |
|---------|-------|--------|----------------|
| `doctor/orphan_check` | `hasCrewAncestor()` calls `ps` in loop | O(n) subprocess calls | Batch gather process info |

### High: Directory Traversal Inefficiencies

| Package | Issue | Impact | Recommendation |
|---------|-------|--------|----------------|
| `doctor/hook_check` | Uses `exec.Command("find")` | Subprocess overhead | Use `filepath.Walk` |
| `lock` | `FindAllLocks()` - unbounded Walk | Scales poorly | Add depth limits |
| `townlog` | `TailEvents()` reads entire file | Memory for large logs | Implement true tail |

### Medium: Redundant Operations

| Package | Issue | Recommendation |
|---------|-------|----------------|
| `dog` | `List()` + iterate = double work | Provide `CountByState()` |
| `dog` | Creates new git.Git per worktree | Cache or batch |
| `doctor/rig_check` | Runs git status twice per polecat | Combine operations |
| `checkpoint/Capture` | 3 separate git commands | Use combined flags |

### Low: JSON Formatting Overhead

| Package | Issue | Recommendation |
|---------|-------|----------------|
| `lock` | `MarshalIndent()` for lock files | Use `Marshal()` (no indentation needed) |
| `townlog` | No compression for old logs | Consider gzip rotation |

---

## 4. Error Handling Issues

### Pattern: Silent Failures

| Package | Location | Issue | Fix |
|---------|----------|-------|-----|
| `events` | All callers | 19 instances of `_ = events.LogFeed()` | Standardize: always ignore or always check |
| `townlog` | `ParseLogLines()` | Silently skips malformed lines | Log warnings |
| `lock` | Lines 91, 180, 194-195 | Silent `_ =` without comments | Document intent |
| `checkpoint` | `Capture()` | Returns nil error but git commands fail | Return actual errors |
| `deps` | `BeadsUnknown` case | Silently passes | Log warning or fail |

### Pattern: Inconsistent State Handling

| Package | Issue | Recommendation |
|---------|-------|----------------|
| `dog/Get()` | Returns minimal Dog if state missing | Document or error |
| `config/GetAccount()` | Returns pointer to loop variable (bug!) | Return by value |
| `boot` | `LoadStatus()` returns empty struct if missing | Document behavior |

### Bug: Missing Role Mapping

| Package | Issue | Impact |
|---------|-------|--------|
| `claude` | `RoleTypeFor()` missing `deacon`, `crew` | Wrong settings applied |

---

## 5. Testing Gaps

| Package | Gap | Priority |
|---------|-----|----------|
| `checkpoint` | No unit tests | HIGH (crash recovery) |
| `dog` | 4 tests, major paths untested | HIGH |
| `deps` | Minimal failure path testing | MEDIUM |
| `claude` | No tests | LOW |

---

## Summary Statistics

| Category | Count | Packages Affected |
|----------|-------|-------------------|
| **Dead Code Items** | 25+ | config, constants, doctor, lock, events, boot, dog, keepalive |
| **Duplicate Patterns** | 6 | util, doctor, config, lock |
| **Performance Issues** | 12 | events, townlog, doctor, dog, lock, checkpoint |
| **Error Handling Issues** | 15 | events, townlog, lock, checkpoint, deps, claude |
| **Testing Gaps** | 4 packages | checkpoint, dog, deps, claude |

## Recommended Priority

1. **Delete keepalive package** (entire package unused)
2. **Fix claude/RoleTypeFor()** (incorrect behavior)
3. **Fix config/GetAccount()** (pointer to stack bug)
4. **Fix polecat/pending.go** (non-atomic writes)
5. **Delete 21 unused constants** (maintenance burden)
6. **Consolidate atomic write pattern** (DRY)
7. **Add checkpoint tests** (crash recovery critical)
