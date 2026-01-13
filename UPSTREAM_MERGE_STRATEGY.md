# Upstream Merge Strategy: steveyegge/gastown

**Document Date:** 2026-01-13
**Status:** Investigation Complete

## Executive Summary

This document outlines the relationship between our fork (`groblegark/gastown`) and the upstream repository (`steveyegge/gastown`), including versioning strategies, identified fixes to pull, and recommended workflows for future upstream merges.

## Repository Status

### Current State
- **Our Remote:** `origin` → https://github.com/groblegark/gastown.git
- **Upstream Remote:** `upstream` → https://github.com/steveyegge/gastown.git (newly added)
- **Common Ancestor:** `f79614d76484e3651e6a50171662c8925068cbf2`

### Divergence
- **Commits in upstream NOT in our main:** ~20+ commits
- **Commits in our main NOT in upstream:** ~15+ commits
- **Status:** Both repos have diverged significantly and cannot fast-forward merge

## Branching & Versioning Strategy

### Version Releases
Upstream uses semantic versioning with tags:
- v0.1.1, v0.2.0, v0.2.1, v0.2.2, v0.2.3, v0.2.4, v0.2.5, v0.2.6
- Most recent: **v0.2.6**
- **Our main:** Contains untagged commits ahead of v0.2.6

### Key Branches in Upstream

| Branch | Purpose | Status |
|--------|---------|--------|
| `main` | Primary development branch | Active, contains latest features |
| `beads-sync` | Automated beads issue tracking sync | Maintained |
| `feature/orphans` | Process orphan management commands | Ready for merge |
| `feature/rename-harness-to-hq` | Terminology updates (harness → hq) | Ready for merge |
| `fix/ci-lint-errors` | Resolves CI lint failures | Ready for merge |
| `fix/tmux` (multiple) | TMux session management improvements | Contains critical fixes |
| `polecat/*` | Polecat-specific work branches | Various states |
| `easel/main` | Easel branch | Parallel development |

## Identified Upstream Fixes & Features to Pull

### High Priority (Bug Fixes)

1. **fix(tests): resolve test failures in costs and polecat tests** (73a349e5, a2607b5b)
   - Resolves test suite issues
   - Importance: High (fixes broken tests)
   - Risk: Low (test-only changes)

2. **fix(done): verify commits exist before completing (hq-xthqf)** (e7b0af02)
   - Prevents completion when commits are missing
   - Importance: Medium (prevents workflow errors)
   - Risk: Low (safety check)

3. **fix(done): handle getcwd errors when worktree deleted (hq-3xaxy)** (f9ca7bb8)
   - Gracefully handles deleted worktrees
   - Importance: Medium (prevents crashes)
   - Risk: Low (error handling)

4. **fix(mayor): match handoff priming for gt may at startup (hq-osbot)** (278b2f2d)
   - Improves mayor initialization
   - Importance: Medium
   - Risk: Low

5. **fix(tmux): explicit process kill before session termination** (b1fcb7d3)
   - Better tmux session cleanup
   - Importance: High (prevents zombie sessions)
   - Risk: Medium (affects session lifecycle)

6. **fix(zfc): NamePool.InUse is transient, not persisted (hq-lng09)** (f0192c8b)
   - Fixes name pool persistence logic
   - Importance: Medium
   - Risk: Low (state management fix)

### Medium Priority (Features & Improvements)

7. **feat(orphans): add list and kill subcommands for Claude process orphans** (18893e71, ea12679a, a43c89c0)
   - New `orphans` command: `gt orphans list` and `gt orphans kill`
   - Importance: Medium (operational utility)
   - Risk: Low (new functionality, no changes to existing code)
   - Files: `internal/cmd/orphans.go` (+367 lines)

8. **feat(spawn): migrate to NewSessionWithCommand pattern** (87fde4b4, 38bedc03)
   - Refactors session spawning to use direct command in tmux
   - Replaces: NewSession() + WaitForShellReady() + SendKeys()
   - Importance: High (resolves race conditions)
   - Risk: High (affects boot/spawn mechanism)
   - Files: `internal/boot/boot.go`, `internal/tmux/tmux.go`
   - **Conflict Alert:** Our boot.go diverges from this approach

9. **feat(convoy): add --owner flag for targeted completion notifications** (392ff1d3)
   - Allows filtering convoy completions by owner
   - Importance: Low (nice-to-have feature)
   - Risk: Low

10. **feat(crew): accept rig name as positional arg in crew status** (15cfb76c)
    - Improves crew command usability
    - Importance: Low
    - Risk: Low

11. **feat(polecat): add identity show command with CV summary** (2d8949a3)
    - New `gt polecat identity show` command
    - Importance: Medium (debugging utility)
    - Risk: Low
    - Files: `internal/cmd/polecat_identity.go` (significant expansion)

### Documentation & Design Changes

12. **New Design Documents in Upstream:**
    - `docs/design/convoy-lifecycle.md` (197 lines) - Convoy architecture
    - `docs/formula-resolution.md` (248 lines) - Formula resolution logic
    - `docs/mol-mall-design.md` (476 lines) - Mol mall architecture
    - `docs/design/dog-pool-architecture.md` - Dog pool design
    - **Note:** Our `docs/upstream-pr-protocol.md` (165 lines) was deleted upstream

13. **New Test Coverage:**
    - `internal/cmd/prime_test.go` (+513 lines) - Prime command tests
    - `internal/cmd/costs_workdir_test.go` (+28 lines)

## Merge Strategy Options

### Option 1: Cherry-Pick Safe Fixes (Recommended - Phased Approach)
**Strategy:** Selectively apply low-risk, high-value fixes to our main without full upstream merge.

**Pros:**
- ✅ Full control over changes
- ✅ Low risk of breaking our customizations
- ✅ Can test each fix independently
- ✅ Easy to coordinate with team

**Cons:**
- ❌ Manual effort for each fix
- ❌ May miss interdependencies between fixes
- ❌ Need to track which fixes are applied

**When to use:** For initial integration with upstream

**Fixes to cherry-pick:**
1. Test failure fixes (73a349e5, a2607b5b)
2. Done command safety checks (e7b0af02, f9ca7bb8)
3. Tmux process cleanup (b1fcb7d3)
4. Documentation syncs

### Option 2: Rebase-Based Merge
**Strategy:** Rebase our commits on top of upstream/main to maintain linear history.

**Command:**
```bash
git rebase upstream/main  # will have conflicts to resolve
```

**Pros:**
- ✅ Clean, linear history
- ✅ Gets all upstream fixes
- ✅ Easy to understand commit chain
- ✅ Preserves all our commits

**Cons:**
- ❌ Significant conflicts (boot.go, polecat_identity.go, etc.)
- ❌ Requires resolving each conflict manually
- ❌ Must verify all changes work together
- ❌ Cannot be undone easily if something breaks

**When to use:** When ready to do comprehensive upstream sync with full testing

### Option 3: Three-Way Merge (Merge Commit)
**Strategy:** Merge upstream/main into our main, creating a merge commit.

**Command:**
```bash
git merge --no-ff upstream/main
```

**Pros:**
- ✅ Preserves the history of both branches
- ✅ Clear marker of integration point
- ✅ Easier to bisect if issues arise
- ✅ Both branches remain intact

**Cons:**
- ❌ Still has conflicts to resolve (boot.go minimum)
- ❌ Creates merge commit which makes history busier
- ❌ Downstream merges from us become complex

**When to use:** When doing coordinated team syncs

### Option 4: Squash Merge
**Strategy:** Combine all upstream commits into a single commit on our branch.

**Pros:**
- ✅ Clean history on our main
- ✅ Single point to revert if needed

**Cons:**
- ❌ Loses upstream commit history
- ❌ Hard to track which upstream commits we have
- ❌ Makes upstreaming our fixes harder

**When to use:** NOT recommended for this repo

## Known Conflicts

### boot.go (Critical)
**Location:** `internal/boot/boot.go`

**Our Approach:**
```go
// Create new session
if err := b.tmux.NewSession(SessionName, b.bootDir); err != nil { ... }

// Wait for shell + send keys separately
b.tmux.WaitForShellReady(SessionName, 5*time.Second)
b.tmux.SendKeys(SessionName, startCmd)
```

**Upstream Approach:**
```go
// Pass command directly to session creation
if err := b.tmux.NewSessionWithCommand(SessionName, b.bootDir, startCmd); err != nil { ... }
```

**Analysis:**
- Upstream's approach avoids race conditions in tmux
- Our approach has explicit wait + send-keys which can fail under load
- **Recommendation:** Use upstream's approach, but ensure `NewSessionWithCommand` exists in our tmux package

## Recommended Workflow for Future Upstream Merges

### Long-Term Integration Strategy

#### Phase 1: Set Up Continuous Monitoring (Immediate)
- Keep upstream remote configured: `git remote add upstream https://github.com/steveyegge/gastown.git`
- Run weekly checks: `git log --oneline origin/main..upstream/main`
- Track new fixes via GitHub issue monitoring

#### Phase 2: Cherry-Pick Critical Fixes (Weekly)
1. Identify new fixes in upstream
2. Test fix in isolation on a feature branch
3. Apply via cherry-pick or manual integration
4. Document applied fixes in a tracking issue

#### Phase 3: Periodic Full Sync (Quarterly)
1. Create sync branch: `git checkout -b sync/upstream-2026-Q1`
2. Merge or rebase: `git merge upstream/main` (with conflict resolution)
3. Run full test suite
4. Request code review from team
5. Merge to main if tests pass

#### Phase 4: Upstream Pull Requests (Ad-hoc)
When our fixes are valuable to upstream:
1. Create feature branch from upstream/main
2. Implement fix
3. Push to our fork and create PR against steveyegge/gastown
4. Reference in PR body

### Step-by-Step: How to Cherry-Pick Upstream Fixes

```bash
# View the fix to understand it
git show <upstream-commit-hash>

# Cherry-pick to current branch
git cherry-pick <upstream-commit-hash>

# If conflicts, resolve manually
git status  # see conflicts
# ... fix conflicts in your editor ...
git add <files>
git cherry-pick --continue

# Test the fix
make test  # or appropriate test command

# Commit message will reference the upstream commit
# (already set by cherry-pick)
```

### Step-by-Step: How to Do Full Upstream Merge

```bash
# Create a sync branch
git checkout -b sync/upstream-$(date +%Y-%m-%d)

# Try the merge
git merge upstream/main

# If conflicts, resolve them
git status  # see conflicts
git diff  # review what changed
# ... resolve conflicts in editor ...
git add <resolved-files>

# Test thoroughly
make test
make lint
# Manual testing of critical paths

# Complete merge
git commit -m "Merge upstream/main (YYYY-MM-DD)

- Brings in orphans command
- Updates boot session handling
- Adds new design documentation
- Resolves test failures"

# If good, merge to main
git checkout main
git merge --ff-only sync/upstream-$(date +%Y-%m-%d)
git push origin main
```

## Checklist for Maintaining Upstream Sync

Use this checklist when doing any upstream integration:

### Pre-Merge Checks
- [ ] Fetch latest: `git fetch upstream`
- [ ] Review new commits: `git log --oneline origin/main..upstream/main`
- [ ] Check for breaking changes in upstream
- [ ] Create dedicated sync branch (don't merge directly to main)
- [ ] Understand the scope of changes

### Conflict Resolution
- [ ] Resolve merge conflicts
- [ ] Review each conflicted file carefully
- [ ] Understand both our approach and upstream's
- [ ] Choose the better approach (don't just pick randomly)
- [ ] Look for dependent code changes

### Testing
- [ ] Run unit tests: `make test`
- [ ] Run linters: `make lint`
- [ ] Manual test critical paths:
  - [ ] Session spawning (boot sequence)
  - [ ] Done/handoff mechanism
  - [ ] Polecat commands
  - [ ] TMux session management
- [ ] Test on actual hardware if possible

### Documentation
- [ ] Update changelog/release notes
- [ ] Document applied upstream fixes
- [ ] Update any custom docs that conflict with upstream
- [ ] Note any deviations from upstream in comments

### Review & Merge
- [ ] Request code review from maintainers
- [ ] Verify CI passes (if configured)
- [ ] Merge to main only after approval
- [ ] Create release notes for team
- [ ] Tag new version if applicable

### Post-Merge
- [ ] Monitor for any regressions reported by users
- [ ] Be ready to revert if critical issues arise
- [ ] Update team on sync status
- [ ] Plan next upstream sync (quarterly cadence)

## Key Insights & Recommendations

### 1. Boot Session Handling (Critical)
**Issue:** Upstream refactored boot session creation from synchronous SendKeys to direct command passing.
**Status:** Our version still uses SendKeys pattern
**Action:** We should adopt upstream's pattern (use `NewSessionWithCommand`) as it's more robust
**Risk:** Medium - requires testing but fixes known race conditions

### 2. Process Orphan Management (Nice-to-Have)
**Issue:** No existing orphan cleanup mechanism
**Status:** Upstream has comprehensive orphans command
**Action:** Consider cherry-picking this feature for operational benefit
**Risk:** Low - new functionality, isolated code

### 3. Test Coverage Gaps
**Issue:** Upstream added significant test coverage (prime_test.go, etc.)
**Status:** We lack these tests
**Action:** Cherry-pick test files when syncing
**Risk:** Low - test-only additions

### 4. Documentation Divergence
**Issue:** Upstream has new design docs, we removed one
**Status:** Likely intentional due to customizations
**Action:** Review differences and decide whether to keep our protocol doc or adopt upstream docs
**Risk:** Low - documentation only

### 5. Long-Term Sustainability
**Recommendation:** Establish quarterly sync rhythm to prevent excessive divergence
- Monthly reviews of upstream changes
- Quarterly merge attempts
- Ad-hoc cherry-picks for critical fixes
- This prevents "boiling frog" problem of unbridgeable divergence

## Summary Table: Upstream Fixes Impact

| Fix | Type | Priority | Risk | Effort | Recommendation |
|-----|------|----------|------|--------|-----------------|
| Test failures (73a349e5) | Fix | High | Low | Low | Cherry-pick immediately |
| Done safety checks (e7b0af02, f9ca7bb8) | Fix | Medium | Low | Low | Cherry-pick soon |
| Tmux cleanup (b1fcb7d3) | Fix | High | Medium | Medium | Include in next sync |
| Orphans command (18893e71+) | Feature | Medium | Low | Medium | Cherry-pick if needed |
| NewSessionWithCommand (87fde4b4) | Refactor | High | High | High | Plan for full sync |
| Identity show command (2d8949a3) | Feature | Low | Low | Medium | Optional |
| Design docs (new) | Docs | Medium | Low | Low | Review & adopt if suitable |

## Next Steps

1. **Immediate:** Add upstream remote (already done ✓)
2. **This week:** Cherry-pick test failure fixes
3. **This month:** Evaluate which fixes to apply
4. **Next quarter:** Plan full upstream sync attempt
5. **Ongoing:** Monitor upstream for critical fixes

---

**Document prepared by:** Claude Code (Witness)
**Last updated:** 2026-01-13
**Review status:** Ready for team review
