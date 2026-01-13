# Upstream Sync Maintainer Checklist

**Purpose:** Step-by-step guide for maintaining synchronization with steveyegge/gastown

**Document Type:** Actionable checklist for maintainers
**Last Updated:** 2026-01-13

---

## Overview

This repository (`groblegark/gastown`) is a fork of `steveyegge/gastown` with custom modifications. Periodically, we need to sync with upstream to get bug fixes and new features while preserving our customizations.

**Quick Facts:**
- Upstream URL: `https://github.com/steveyegge/gastown.git` (already configured as `upstream` remote)
- Last sync check: 2026-01-13
- Current divergence: ~15+ commits ahead, ~20+ commits behind
- Recommended sync frequency: Quarterly (or sooner for critical fixes)

---

## Monthly Check (First Monday of Month)

**Duration:** 15 minutes
**Owner:** Any maintainer
**Trigger:** Calendar reminder

### Checklist

- [ ] **Fetch upstream changes**
  ```bash
  git fetch upstream main
  ```

- [ ] **Review new commits**
  ```bash
  git log --oneline origin/main..upstream/main --first-parent
  ```

- [ ] **Check for critical fixes**
  - [ ] Look for `fix:` prefixed commits
  - [ ] Scan titles for:
    - "race condition"
    - "crash"
    - "zombie"
    - "orphan"
    - "deadlock"
    - "corruption"

- [ ] **Document findings**
  - [ ] If critical fixes found → escalate to weekly sync
  - [ ] If minor fixes → note for next quarterly sync
  - [ ] Update team in #gastown Slack channel

- [ ] **Create or update tracking issue** (if using GitHub issues)
  - Title: "Upstream sync check: YYYY-MM-DD"
  - Include: Commit count, critical fixes, recommendation

### Red Flags That Require Immediate Attention

If you see any of these, proceed to **Weekly Emergency Sync** section:
- Fixes for zombie processes
- Boot/spawn system changes
- Session management fixes
- Critical test failures fixed upstream
- Security-related fixes

---

## Weekly Emergency Sync (When Critical Fix Found)

**Duration:** 2-4 hours
**Owner:** Experienced maintainer
**Trigger:** Critical fix identified in monthly check

### Pre-Sync Assessment

- [ ] **Understand the fix**
  ```bash
  git show <commit-hash>
  git log -p <commit-hash>~1..<commit-hash>
  ```

- [ ] **Check impact scope**
  - [ ] How many files modified?
  - [ ] Does it touch core functionality (boot, done, spawn)?
  - [ ] What tests exist for this fix?

- [ ] **Assess risk**
  - [ ] Impact: How critical is this fix?
  - [ ] Complexity: How complex is the change?
  - [ ] Testing: Can we adequately test it?
  - [ ] **Decision:** Cherry-pick vs full merge?

### Cherry-Pick Approach (Low-Risk Fixes)

Use for: Individual bug fixes, minor features
Skip to: [Cherry-Pick Steps](#cherry-pick-steps)

### Full Merge Approach (Complex Changes)

Use for: Session handling, major refactors
Skip to: [Full Merge Steps](#full-merge-steps)

---

## Cherry-Pick Steps

**Best for:** Individual, isolated fixes
**Difficulty:** Easy
**Testing:** Moderate
**Risk:** Low

### 1. Preparation

- [ ] **Create feature branch**
  ```bash
  git checkout -b cherry-pick/upstream-<issue-id>
  # Example: cherry-pick/upstream-hq-xyz12
  ```

- [ ] **Verify clean working directory**
  ```bash
  git status
  ```
  - Must be clean (no uncommitted changes)

### 2. Apply Fix

- [ ] **Cherry-pick the commit**
  ```bash
  git cherry-pick <upstream-commit-hash>
  ```

- [ ] **If no conflicts:**
  - [ ] Skip to Testing section

- [ ] **If conflicts occur:**
  - [ ] Review conflicts: `git diff --cached`
  - [ ] Edit conflicted files
  - [ ] Understand both versions
  - [ ] Choose the correct approach
  - [ ] Stage resolved files: `git add <files>`
  - [ ] Continue cherry-pick: `git cherry-pick --continue`

### 3. Testing

- [ ] **Unit tests**
  ```bash
  make test
  # or: go test ./...
  ```

- [ ] **Linting**
  ```bash
  make lint
  # or: golangci-lint run
  ```

- [ ] **Manual testing** (for critical areas)
  - [ ] If fix touches boot: Test session startup
  - [ ] If fix touches done: Test `gt done`
  - [ ] If fix touches polecat: Test `gt polecat` commands
  - [ ] If fix touches tmux: Test session creation/cleanup

- [ ] **Regression testing**
  - [ ] Verify main features still work
  - [ ] Run any relevant integration tests

### 4. Code Review

- [ ] **Request review from:**
  - [ ] Primary maintainer
  - [ ] Someone familiar with affected code
  - [ ] Include note: "Cherry-picked from upstream: <hash>"

- [ ] **Review checklist for reviewer:**
  - [ ] Does the fix solve the described problem?
  - [ ] No unintended side effects?
  - [ ] Tests pass?
  - [ ] Conflicts resolved correctly?
  - [ ] Commit message preserved from upstream?

### 5. Merge & Deploy

- [ ] **Merge to main**
  ```bash
  git checkout main
  git pull origin main  # sync local main
  git merge --ff-only cherry-pick/upstream-<issue-id>
  git push origin main
  ```

- [ ] **Monitor for issues**
  - [ ] Watch for 30 minutes
  - [ ] Check error logs
  - [ ] Ask team if any problems

- [ ] **Document**
  - [ ] Close any related tracking issues
  - [ ] Update CHANGELOG if maintained
  - [ ] Post summary to team

- [ ] **Cleanup**
  ```bash
  git branch -d cherry-pick/upstream-<issue-id>
  ```

---

## Quarterly Full Sync

**Duration:** 4-8 hours
**Owner:** Lead maintainer + team review
**Frequency:** Every 3 months or when 10+ non-critical commits accumulate
**Timing:** Friday afternoon (allows time to fix issues over weekend)

### 1. Planning Phase

- [ ] **Announce to team**
  - Post in Slack: "Syncing upstream this Friday, may have instability"
  - Give 24 hours notice minimum

- [ ] **Check divergence**
  ```bash
  git fetch upstream
  git log --oneline origin/main..upstream/main | wc -l
  git log --oneline upstream/main..origin/main | wc -l
  ```

- [ ] **Identify conflicts** (test without committing)
  ```bash
  git checkout -b test-merge upstream/main
  git merge origin/main --no-commit
  git merge --abort  # Just testing
  git checkout main
  git branch -D test-merge
  ```

- [ ] **Review all changes**
  ```bash
  git diff origin/main upstream/main --stat | head -50
  ```

- [ ] **Get team buy-in**
  - [ ] Share list of changes
  - [ ] Discuss any concerns
  - [ ] Set expected completion time

### 2. Create Sync Branch

- [ ] **Branch creation**
  ```bash
  git checkout main
  git pull origin main
  git checkout -b sync/upstream-$(date +%Y-%m-%d)
  ```

- [ ] **Verify branch exists and is clean**
  ```bash
  git status  # Should be clean
  git log --oneline -1  # Should be on main
  ```

### 3. Merge Upstream

- [ ] **Attempt merge**
  ```bash
  git merge upstream/main --no-ff
  ```

- [ ] **If no conflicts:**
  - [ ] Skip to Testing Phase

- [ ] **If conflicts:**
  - [ ] Review status: `git status`
  - [ ] Identify conflicted files
  - [ ] For each conflict:
    - [ ] Understand our version
    - [ ] Understand upstream version
    - [ ] Decide which is correct (don't just pick one randomly)
    - [ ] Edit file to resolve
  - [ ] Check specific conflicts:
    - [ ] `internal/boot/boot.go` - verify NewSessionWithCommand is correct
    - [ ] `internal/cmd/polecat_identity.go` - review identity features
    - [ ] `internal/cmd/done.go` - understand safety improvements
  - [ ] Stage resolved files:
    ```bash
    git add <resolved-files>
    ```
  - [ ] Complete merge:
    ```bash
    git commit -m "Merge upstream/main ($(date +%Y-%m-%d))

    Brings in:
    - Bug fixes for test failures
    - TMux session handling improvements
    - Process orphan management (gt orphans command)
    - Safety improvements to done command
    - And more (see commit log)"
    ```

### 4. Testing Phase (Critical)

**This phase takes the most time and is most important.**

- [ ] **Unit test suite**
  ```bash
  make test
  ```
  - [ ] All tests pass
  - [ ] No flaky tests
  - [ ] Report any failures

- [ ] **Lint checks**
  ```bash
  make lint
  ```
  - [ ] Zero lint errors
  - [ ] Fix any issues found

- [ ] **Build verification**
  ```bash
  make build
  # or: go build -o gastown ./cmd/...
  ```

- [ ] **Critical path testing** (manual, by feature)

  - [ ] **Boot/Daemon tests:**
    - [ ] `gt boot` starts cleanly
    - [ ] Sessions spawn correctly
    - [ ] Boot completes handoff
    - [ ] No zombie processes left

  - [ ] **Done/Handoff tests:**
    - [ ] `gt done` works normally
    - [ ] Verify commits before completing
    - [ ] Handles deleted worktrees gracefully

  - [ ] **Polecat tests:**
    - [ ] `gt polecat status` works
    - [ ] `gt polecat identity` shows correctly
    - [ ] All polecat commands function

  - [ ] **TMux integration tests:**
    - [ ] Sessions create/destroy cleanly
    - [ ] No orphaned processes
    - [ ] `gt orphans list` works (if merged)
    - [ ] Process cleanup complete

  - [ ] **Workflow tests:**
    - [ ] Create a new work item
    - [ ] Assign to agent
    - [ ] Complete workflow
    - [ ] Handoff works

- [ ] **Regression testing**
  - [ ] All previously working features still work
  - [ ] No new errors in logs
  - [ ] Performance acceptable

- [ ] **Integration testing** (if applicable)
  - [ ] Test with real rigs if available
  - [ ] Verify with actual Claude sessions

### 5. Code Review

- [ ] **Prepare review materials**
  - [ ] List of changes: `git log --oneline main..HEAD`
  - [ ] Conflict resolutions: Which files had conflicts?
  - [ ] Test results: All passing?

- [ ] **Code review**
  - [ ] Lead developer reviews all changes
  - [ ] Request secondary review from:
    - [ ] Boot/daemon expert
    - [ ] Tmux/session expert
    - [ ] Polecat expert

- [ ] **Checklist for reviewers:**
  - [ ] Conflicts resolved correctly?
  - [ ] Our customizations preserved?
  - [ ] Tests all pass?
  - [ ] No logic inversions or mistakes?
  - [ ] Commit message clear?
  - [ ] Ready to deploy?

### 6. Merge to Main

- [ ] **Final verification**
  ```bash
  git status  # Should be clean
  git log --oneline main..HEAD  # See what will merge
  ```

- [ ] **Merge to main**
  ```bash
  git checkout main
  git pull origin main
  git merge --ff-only sync/upstream-$(date +%Y-%m-%d)
  ```

- [ ] **Push to remote**
  ```bash
  git push origin main
  ```

- [ ] **Verify push**
  ```bash
  git log --oneline -5 origin/main  # Verify remote is updated
  ```

### 7. Post-Merge

- [ ] **Announce completion**
  - Post to Slack: "Upstream sync complete, all tests passing"

- [ ] **Monitor for issues**
  - [ ] Watch for 1 hour
  - [ ] Check error logs
  - [ ] Be ready to revert if critical issues

- [ ] **If issues found:**
  - [ ] For minor issues: Create PR to fix
  - [ ] For critical issues: Be ready to revert
    ```bash
    git revert -m 1 <merge-commit-hash>
    git push origin main
    ```

- [ ] **Document**
  - [ ] Update CHANGELOG
  - [ ] Update version tag if appropriate
  - [ ] Document any deviations from upstream
  - [ ] Note any known issues

- [ ] **Create release notes**
  - Example:
    ```
    ## Upstream Sync: 2026-01-13

    Merged upstream/main with the following highlights:
    - Fixed test failures affecting CI
    - Improved tmux session lifecycle handling
    - Added orphan process management (gt orphans command)
    - Enhanced done command safety checks

    Breaking changes: None
    ```

- [ ] **Cleanup**
  ```bash
  git branch -d sync/upstream-$(date +%Y-%m-%d)
  git push origin :sync/upstream-$(date +%Y-%m-%d)  # delete remote
  ```

### 8. Follow-up

- [ ] **Schedule next sync**
  - [ ] Mark calendar for 3 months out
  - [ ] Earlier if critical issues found

- [ ] **Debrief with team**
  - [ ] What went well?
  - [ ] What was difficult?
  - [ ] How can we improve process?
  - [ ] Document lessons learned

---

## Handling Merge Conflicts

### Decision Tree

```
Found conflict in: <file>

├─ Is this a file we customized?
│  ├─ YES → Review both versions carefully
│  │        Choose the better approach
│  │        Document why
│  └─ NO → Probably accept upstream version
│
├─ Does upstream fix a bug?
│  ├─ YES → Use upstream version (we want the fix)
│  └─ NO → Keep our version (our customization is intentional)
│
└─ Can't decide?
   └─ Ask on Slack: "Conflict in <file>, opinions?"
      Let team vote if unsure
```

### Specific Known Conflicts

#### 1. internal/boot/boot.go

**Our version:**
```go
b.tmux.NewSession(SessionName, b.bootDir)
b.tmux.WaitForShellReady(SessionName, 5*time.Second)
b.tmux.SendKeys(SessionName, startCmd)
```

**Upstream version:**
```go
b.tmux.NewSessionWithCommand(SessionName, b.bootDir, startCmd)
```

**Recommendation:** **ACCEPT UPSTREAM**
- Upstream approach is better (atomic operation, no race)
- Requires `NewSessionWithCommand` to exist in tmux package
- We have commit about preventing zombie sessions - this is related

**Action:**
```bash
# Check if NewSessionWithCommand exists
grep -r "NewSessionWithCommand" internal/tmux/

# If not found, check upstream version
git show upstream/main:internal/tmux/tmux.go | grep -A 10 "NewSessionWithCommand"

# Then decide: use their full implementation or just the pattern
```

#### 2. internal/cmd/polecat_identity.go

**Reason for conflict:** We and upstream have both extended this
**Recommendation:** **REVIEW CAREFULLY**
- Merge both feature sets if possible
- Ask: What features does each side have?
- Keep all valuable functionality

**Action:**
```bash
git diff --ours internal/cmd/polecat_identity.go  # Our version
git diff --theirs internal/cmd/polecat_identity.go  # Upstream version

# Manually merge into single file with all features
```

#### 3. docs/upstream-pr-protocol.md

**Our version:** Has content
**Upstream version:** Deleted
**Recommendation:** **KEEP OURS**
- We have custom protocol for upstream contributions
- Upstream removed theirs
- Non-conflicting decision

**Action:**
```bash
git checkout --ours docs/upstream-pr-protocol.md
```

---

## Reverting a Sync (If Critical Issues Found)

**Use only if:** Critical bug introduced, cannot be fixed quickly

```bash
# Find the merge commit
git log --oneline | head -20

# Revert it
git revert -m 1 <merge-commit-hash>

# Push
git push origin main

# Announce
# "Upstream sync reverted due to <issue>, will retry next week"
```

**Important:** Don't make this a habit - instead, test more thoroughly before merging.

---

## Preventing Divergence

**Long-term strategy:**
- More frequent syncs prevent large divergences
- Smaller, more focused syncs are easier to test
- Quarterly sync with monthly cherry-picks of critical fixes is ideal

---

## Template: Sync Announcement to Team

Copy/paste this for Slack announcement:

```
📦 Upstream Sync: <date>

Starting merge of steveyegge/gastown upstream changes.

Current state:
- Upstream has: <X> commits we don't have
- We have: <Y> commits upstream doesn't have
- Conflicts expected: <none/minor/major>

High-value changes incoming:
- <fix 1>
- <fix 2>
- <feature 1>

Timeline: <duration estimate>
Status updates in #gastown
```

---

## Quick Command Reference

```bash
# Monthly check
git fetch upstream
git log --oneline origin/main..upstream/main

# Cherry-pick a fix
git cherry-pick <hash>
git cherry-pick --continue  # after resolving conflicts

# Full sync
git checkout -b sync/upstream-$(date +%Y-%m-%d)
git merge upstream/main
# ... resolve conflicts ...
git commit -m "Merge upstream..."

# Test
make test
make lint

# Deploy
git checkout main
git merge sync/upstream-$(date +%Y-%m-%d)
git push origin main

# Revert if needed
git revert -m 1 <merge-commit>
git push origin main
```

---

## Contacts

**Questions about this process?** Consult:
- Steve Yegge (upstream owner): @steveyegge
- Lead maintainer: (configure your team)
- Team channel: #gastown

---

**Document Status:** Ready for use
**Last Updated:** 2026-01-13
**Next Review:** 2026-04-13 (quarterly)
