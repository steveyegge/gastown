# Merge Conflict Report: gastown

**Generated:** 2026-01-31
**Trial merge:** `origin/main` ← `upstream/main`
**Files modified on both sides:** 71
**Files with merge conflicts:** 14

---

## Conflict Summary

| File | Conflict Count | Severity | Category |
|------|----------------|----------|----------|
| `internal/cmd/formula.go` | 1 | **Moderate** | Template rendering feature |
| `internal/cmd/gitinit.go` | 2 | **Moderate** | Hook installation vs fingerprint update |
| `internal/cmd/hook.go` | 1 | **Trivial** | JSON output feature |
| `internal/cmd/polecat_spawn.go` | 4 | **Complex** | Multiple independent fixes collide |
| `internal/cmd/sling.go` | 4 | **Complex** | Variable handling vs trailing slash trim |
| `internal/cmd/sling_helpers.go` | 1 | **Moderate** | Agent bead ID prefix handling |
| `internal/crew/manager.go` | 1 | **Moderate** | Settings symlink explanation |
| `internal/formula/formulas/mol-witness-patrol.formula.toml` | 2 | **Complex** | Step dependencies and descriptions |
| `internal/mail/resolve.go` | 1 | **Trivial** | Comment clarification |
| `internal/mail/router.go` | 3 | **Moderate** | Agent bead ID parsing |
| `internal/mayor/manager.go` | 1 | **Trivial** | Method rename |
| `internal/templates/roles/mayor.md.tmpl` | 1 | **Trivial** | PR documentation |
| `internal/templates/roles/polecat.md.tmpl` | 1 | **Trivial** | Workflow description |
| `internal/tmux/tmux.go` | 1 | **Moderate** | Claude child process detection |

---

## Detailed Conflict Analysis

### Complex (Require Careful Resolution)

#### 1. `internal/cmd/polecat_spawn.go` (4 conflicts)

**What collided:**
- Origin: Import cleanup, agent bead creation notes, session running check, orphan cleanup
- Upstream: Import changes, startup race condition fix, worktree verification, orphan state cleanup

**Resolution approach:**
- Both sides added different fixes to the same file
- Need to merge ALL fixes from both sides
- Pay attention to import order and function placement

**Risk:** HIGH - Core polecat lifecycle management

---

#### 2. `internal/cmd/sling.go` (4 conflicts)

**What collided:**
- Origin: Variable handling for standalone formula mode, placeholder values for rig targets, deferred spawn execution
- Upstream: Trailing slash normalization, similar placeholder/spawn logic but different implementation

**Resolution approach:**
- Trailing slash fix from upstream is an addition - include it
- Variable handling is origin-specific - preserve it
- Spawn logic needs careful comparison - may be same intent different code

**Risk:** HIGH - Core slinging mechanics

---

#### 3. `internal/formula/formulas/mol-witness-patrol.formula.toml` (2 conflicts)

**What collided:**
- Origin: Different step dependencies (`needs = ['detect-orphans']`), detailed exit/continue decision logic
- Upstream: Different dependencies (`needs = ['check-refinery']`, `parallel = true`), different description format

**Resolution approach:**
- This is a formula specification - need to understand the INTENDED workflow
- May need to merge step lists if both added different steps
- Description differences may be semantic - review intent

**Risk:** MEDIUM - Affects witness patrol behavior

---

### Moderate (Standard Resolution)

#### 4. `internal/cmd/formula.go` (1 conflict)

**What collided:**
- Origin: Simple description concatenation
- Upstream: Template context building for convoy prompts

**Resolution approach:**
- Upstream adds a new feature (Go text/template rendering)
- Origin has simpler logic
- Take upstream feature, ensure origin functionality preserved

**Risk:** LOW - Additive feature

---

#### 5. `internal/cmd/gitinit.go` (2 conflicts)

**What collided:**
- Origin: Pre-commit hook installation
- Upstream: Beads fingerprint update after git init, post-checkout hook (git has no pre-checkout)

**Resolution approach:**
- Upstream changed from pre-commit to post-checkout based on git limitations
- Need to include fingerprint update AND decide on hook approach

**Risk:** MEDIUM - Git hook behavior

---

#### 6. `internal/cmd/sling_helpers.go` (1 conflict)

**What collided:**
- Origin: Explicit town name lookup for polecat beads
- Upstream: Generic prefix-based approach

**Resolution approach:**
- Check if origin's explicit approach handles edge cases upstream's generic approach misses
- May need to preserve origin's logic for correctness

**Risk:** MEDIUM - Agent bead ID resolution

---

#### 7. `internal/crew/manager.go` (1 conflict)

**What collided:**
- Origin: Detailed comment about settings symlink behavior
- Upstream: Different implementation (possibly from API migration)

**Resolution approach:**
- Preserve origin's explanatory comments
- Adopt upstream's API changes

**Risk:** LOW - Comments and API migration

---

#### 8. `internal/mail/router.go` (3 conflicts)

**What collided:**
- Origin: hq- prefix handling for town-level IDs
- Upstream: Similar hq- prefix handling but different implementation

**Resolution approach:**
- Both address same issue (hq- prefix handling)
- Take the more complete solution

**Risk:** LOW - Mail routing

---

#### 9. `internal/tmux/tmux.go` (1 conflict)

**What collided:**
- Origin: hasClaudeChild function for descendant checking
- Upstream: Agent-agnostic zombie detection

**Resolution approach:**
- Upstream refactored to be agent-agnostic (not Claude-specific)
- Take upstream's approach as it's more generic

**Risk:** MEDIUM - Process management

---

### Trivial (Easy Resolution)

#### 10. `internal/cmd/hook.go` (1 conflict)
**What:** JSON output flag handling
**Resolution:** Simple merge - both changes are independent

#### 11. `internal/mail/resolve.go` (1 conflict)
**What:** Comment updates for hq- prefix
**Resolution:** Take more complete comment

#### 12. `internal/mayor/manager.go` (1 conflict)
**What:** Method rename `KillSessionWithProcesses` → `KillSession`
**Resolution:** Take upstream's renamed method

#### 13. `internal/templates/roles/mayor.md.tmpl` (1 conflict)
**What:** Added PR documentation section
**Resolution:** Include the documentation addition

#### 14. `internal/templates/roles/polecat.md.tmpl` (1 conflict)
**What:** Workflow description text
**Resolution:** Take clearer description

---

## Files Auto-Merged Successfully (57 files)

The following files were modified on both sides but merged automatically without conflicts:

- `docs/design/federation.md`
- `docs/reference.md`
- `internal/cmd/deacon.go`
- `internal/cmd/doctor.go`
- `internal/cmd/dog.go`
- `internal/cmd/done.go`
- `internal/cmd/handoff.go`
- `internal/cmd/install.go`
- `internal/cmd/mail_inbox.go`
- `internal/cmd/molecule_lifecycle_test.go`
- `internal/cmd/molecule_step.go`
- `internal/cmd/nudge.go`
- `internal/cmd/polecat.go`
- `internal/cmd/sling_formula.go`
- `internal/cmd/sling_test.go`
- `internal/cmd/start.go`
- `internal/cmd/status.go`
- `internal/cmd/unsling.go`
- `internal/config/env.go`
- `internal/config/env_test.go`
- `internal/config/loader.go`
- `internal/config/loader_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/lifecycle.go`
- `internal/doctor/beads_check.go`
- `internal/doctor/beads_check_test.go`
- `internal/doctor/claude_settings_check.go`
- `internal/doctor/config_check.go`
- `internal/doctor/routing_mode_check.go`
- `internal/doltserver/doltserver.go`
- `internal/formula/formulas/mol-convoy-cleanup.formula.toml`
- `internal/formula/formulas/mol-digest-generate.formula.toml`
- `internal/formula/formulas/mol-polecat-work.formula.toml`
- `internal/git/git.go`
- `internal/git/git_test.go`
- `internal/mail/resolve_test.go`
- `internal/polecat/manager.go`
- `internal/polecat/session_manager.go`
- `internal/refinery/manager.go`
- `internal/rig/manager.go`
- `internal/runtime/runtime.go`
- `internal/runtime/runtime_test.go`
- `internal/session/startup.go`
- `internal/session/startup_test.go`
- `internal/templates/roles/boot.md.tmpl`
- `internal/templates/roles/crew.md.tmpl`
- `internal/templates/roles/deacon.md.tmpl`
- `internal/templates/roles/refinery.md.tmpl`
- `internal/templates/roles/witness.md.tmpl`
- `internal/tmux/tmux_test.go`
- `internal/web/fetcher.go`
- `internal/witness/manager.go`
- `internal/witness/manager_test.go`

---

## Resolution Recommendations

### Priority Order

1. **Start with trivial conflicts** (6 files) - Build confidence, verify merge process
2. **Handle moderate conflicts** (5 files) - Straightforward resolution with care
3. **Tackle complex conflicts** (3 files) - Require understanding of both codebases

### Pre-Merge Checklist

- [ ] Review all upstream changes to understand intent
- [ ] Run `go build` after each conflict resolution to verify compilation
- [ ] Run tests for affected areas (`go test ./internal/cmd/...`)
- [ ] Pay special attention to polecat lifecycle changes (many race condition fixes)

### Post-Merge Testing

1. Run full test suite: `go test ./...`
2. Test polecat spawning: `gt sling <bead> <rig>`
3. Test witness patrol: Observe witness behavior over 1-2 cycles
4. Test mail routing: `gt mail send` to various targets
5. Verify tmux zombie detection: Start/stop agents

---

## Risk Assessment

| Category | Risk Level | Impact |
|----------|------------|--------|
| Polecat Lifecycle | **HIGH** | Could break agent spawning/cleanup |
| Sling Mechanics | **HIGH** | Could break work distribution |
| Witness Patrol | **MEDIUM** | Could affect monitoring behavior |
| Mail Routing | **LOW** | Minor addressing issues |
| Templates | **LOW** | Documentation only |

**Overall Merge Risk:** MEDIUM-HIGH

The merge is achievable but requires careful attention to the 3 complex conflicts in polecat_spawn.go, sling.go, and mol-witness-patrol.formula.toml. The polecat-related changes are particularly sensitive as both sides contain important race condition fixes.
