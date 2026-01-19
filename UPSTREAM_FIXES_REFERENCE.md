# Upstream Fixes Quick Reference

**How to use this file:** Use the commit hashes to cherry-pick fixes with `git cherry-pick <hash>`

## Critical Fixes (Apply ASAP)

### 1. Test Failure Resolution
```
Commits: 73a349e5, a2607b5b
Title: fix(tests): resolve test failures in costs and polecat tests
Branch: upstream/main
Command: git cherry-pick 73a349e5
Status: Duplicate commits (appears twice in history)
Risk: LOW - test fixes only
```

**What it fixes:**
- Failures in `internal/cmd/costs_workdir_test.go`
- Polecat test failures
- Allows full test suite to pass

---

### 2. Boot Session Creation Refactor (Race Condition Fix)
```
Commits: 87fde4b4, 38bedc03
Title: feat(spawn): migrate to NewSessionWithCommand pattern
Branch: upstream/main
Status: COMPLEX - requires understanding of tmux package
Risk: HIGH - affects critical boot sequence
Conflicts: YES - boot.go changes significantly
```

**What it fixes:**
- Race condition in tmux session creation
- Old: `NewSession() → WaitForShellReady() → SendKeys()`
- New: `NewSessionWithCommand()` - single atomic operation
- Prevents "can't find pane" errors under load
- Related: commit b1fcb7d3 for explicit process kill

**Prerequisites:**
- Ensure `NewSessionWithCommand` method exists in `internal/tmux/tmux.go`
- Backup current `internal/boot/boot.go` before applying
- Run comprehensive tests after applying

**Application:**
```bash
# Do NOT cherry-pick directly - must understand changes
git show 87fde4b4  # Review changes first
git show 38bedc03  # Review changes first

# Manual merge recommended:
# 1. Edit internal/boot/boot.go
# 2. Replace NewSession() + WaitForShellReady() + SendKeys()
# 3. Use NewSessionWithCommand() instead
# 4. Test thoroughly
```

---

## High-Priority Fixes (Apply This Month)

### 3. Done Command Safety Improvements
```
Commits: e7b0af02, f9ca7bb8
Title: fix(done): verify commits exist before completing
Title: fix(done): handle getcwd errors when worktree deleted
Branch: upstream/main
Command: git cherry-pick e7b0af02 && git cherry-pick f9ca7bb8
Risk: LOW - safety checks
Conflicts: Likely in internal/cmd/done.go
```

**What it fixes:**
- Prevents `gt done` from completing if commits don't exist
- Gracefully handles deleted worktrees (no crash)
- Better error messages to users

**Why important:**
- Prevents broken workflow state
- Improves user experience when worktree is deleted
- No functional changes to working paths

---

### 4. Tmux Process Management
```
Commits: b1fcb7d3
Title: fix(tmux): explicit process kill before session termination
Branch: upstream/main
Command: git cherry-pick b1fcb7d3
Risk: MEDIUM - affects session cleanup
Conflicts: Likely in internal/tmux/tmux.go
```

**What it fixes:**
- Explicit process termination before tmux session kill
- Prevents zombie sessions and orphan processes
- Related to orphans command feature

**Why important:**
- Keeps system clean of orphaned processes
- Fixes "Claude sessions as zombies" problem (referenced in our commit 75defe09)
- Complements our fix(daemon): prevent daemon from killing Claude sessions

---

### 5. Mayor Initialization
```
Commits: 278b2f2d
Title: fix(mayor): match handoff priming for gt may at startup (hq-osbot)
Branch: upstream/main
Command: git cherry-pick 278b2f2d
Risk: LOW - initialization logic
```

**What it fixes:**
- Mayor process properly primes on startup
- Matches intended handoff behavior
- Fixes hq-osbot issue

---

### 6. Name Pool Persistence
```
Commits: f0192c8b
Title: fix(zfc): NamePool.InUse is transient, not persisted (hq-lng09)
Branch: upstream/main
Command: git cherry-pick f0192c8b
Risk: LOW - state management
```

**What it fixes:**
- Corrects name pool persistence assumption
- Prevents stale name reservations
- Fixes hq-lng09

---

## Medium-Priority Features (Consider for Next Sync)

### 7. Process Orphans Management Command
```
Commits: 18893e71, ea12679a, a43c89c0
Title: feat(orphans): add list and kill subcommands for Claude process orphans
Branch: upstream/main
Command: git cherry-pick 18893e71 (if you want it)
Files Added: internal/cmd/orphans.go (+367 lines)
Risk: LOW - new functionality, isolated code
Conflicts: NONE expected
```

**What it adds:**
- `gt orphans list` - list orphaned Claude processes
- `gt orphans kill` - kill orphaned Claude processes
- Operational utility for system cleanup

**When to apply:**
- When you need process cleanup functionality
- Safe to cherry-pick independently
- No dependencies on other changes

**Test it:**
```bash
git cherry-pick 18893e71
gt orphans list  # should work if Claude is running
```

---

### 8. Crew Command Enhancement
```
Commits: 15cfb76c
Title: feat(crew): accept rig name as positional arg in crew status
Branch: upstream/main
Command: git cherry-pick 15cfb76c
Risk: LOW - argument parsing
Conflicts: Low risk
```

**What it improves:**
- `gt crew status <rig-name>` instead of flags
- Better usability

---

### 9. Polecat Identity Command
```
Commits: 2d8949a3
Title: feat(polecat): add identity show command with CV summary
Branch: upstream/main
Files: internal/cmd/polecat_identity.go (significant changes)
Risk: MEDIUM - extensive code
Conflicts: YES - we have our own polecat_identity.go version
```

**What it adds:**
- `gt polecat identity show` command with CV summary
- Enhanced identity functionality

**Note:** Our version likely has different content. Review carefully before applying.

---

## Optional: Documentation & Tests

### 10. Design Documentation
```
Files Added:
  - docs/design/convoy-lifecycle.md (+197 lines)
  - docs/formula-resolution.md (+248 lines)
  - docs/mol-mall-design.md (+476 lines)
  - docs/design/dog-pool-architecture.md
Risk: LOW - documentation only
Action: Review and adopt if useful
```

**Our situation:**
- We have `docs/upstream-pr-protocol.md` (which upstream deleted)
- Upstream has new architecture docs
- Both sets have value - don't need to choose one

---

### 11. Test Coverage Additions
```
Files Added:
  - internal/cmd/prime_test.go (+513 lines)
  - internal/cmd/costs_workdir_test.go (+28 lines)
Risk: LOW - test additions only
Action: Cherry-pick for better coverage
```

**Why useful:**
- Comprehensive prime command tests
- Better costs command testing
- Safe to add to our codebase

---

## Application Strategy

### Phase 1: Low-Risk Fixes (This Week)
1. Test failure fixes: `git cherry-pick 73a349e5`
2. Done safety checks: `git cherry-pick e7b0af02 f9ca7bb8`
3. Tmux cleanup: `git cherry-pick b1fcb7d3`
4. Test coverage: Cherry-pick test files if needed

**Expected conflicts:** Minimal - mostly in cmd/ files

### Phase 2: Feature Additions (Next Week)
1. Orphans command: `git cherry-pick 18893e71`
2. Crew enhancement: `git cherry-pick 15cfb76c`
3. Documentation: Review and manually integrate

**Expected conflicts:** None for orphans, maybe minor for crew

### Phase 3: Complex Refactors (Next Month - Plan Carefully)
1. Boot session refactor (87fde4b4) - requires careful testing
2. Polecat identity command - needs review of both versions

**Expected conflicts:** MAJOR - requires manual resolution

---

## How to Apply Fixes Safely

```bash
# 1. Create a feature branch
git checkout -b cherry-pick/upstream-fixes

# 2. Apply one fix at a time
git cherry-pick <commit-hash>

# 3. If there's a conflict
git status  # see what's in conflict
# ... edit the files ...
git add <resolved-files>
git cherry-pick --continue

# 4. Test after each fix
make test  # or your test command

# 5. If everything works, merge to main
git checkout main
git merge cherry-pick/upstream-fixes
git push origin main

# 6. If something breaks, revert
git revert <commit-hash>
```

---

## Commit Hashes Reference

For quick lookup, here are all the mentioned commits:

| Hash | Title |
|------|-------|
| 73a349e5 | fix(tests): resolve test failures |
| a2607b5b | fix(tests): resolve test failures (duplicate) |
| 18893e71 | feat(orphans): add list and kill subcommands |
| ea12679a | feat(orphans): add list and kill subcommands (dup) |
| b1fcb7d3 | fix(tmux): explicit process kill |
| a43c89c0 | feat(orphans): add kill command |
| e043f4a1 | feat(tmux): add KillSessionWithProcesses |
| 87fde4b4 | feat(spawn): migrate to NewSessionWithCommand |
| 278b2f2d | fix(mayor): match handoff priming |
| 38bedc03 | feat(spawn): migrate to NewSessionWithCommand (dup) |
| e7b0af02 | fix(done): verify commits exist |
| f9ca7bb8 | fix(done): handle getcwd errors |
| 392ff1d3 | feat(convoy): add --owner flag |
| f0192c8b | fix(zfc): NamePool.InUse is transient |
| 15cfb76c | feat(crew): accept rig name as positional arg |
| 2d8949a3 | feat(polecat): add identity show command |

---

**Last updated:** 2026-01-13
**Upstream status:** Checked on 2026-01-13
**Next review:** 2026-02-13 (one month)
