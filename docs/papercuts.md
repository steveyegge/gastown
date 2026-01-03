# Papercuts: Getting Started Golden Path Issues

## Issue 1: "Could not verify repo fingerprint" during `gt install`

### Root Cause
In `internal/cmd/install.go`, the `ensureRepoFingerprint()` function (line 310) runs:
```
bd migrate --update-repo-id
```

This command **requires the directory to already be a git repository**. However, when `gt install` is run, the target directory is NOT yet a git repository - it's just a freshly created directory. The git initialization is optional and happens afterwards (via `gt git-init` or `--git` flag).

### Error Output
```
⚠ Could not verify repo fingerprint: bd migrate --update-repo-id: Error: failed to compute repository ID: not a git repository
```

### Why This Happens
The `bd migrate --update-repo-id` command tries to compute a repository ID but fails when no git repo exists. This is actually fine - the fingerprint is optional and only needed for daemon optimization.

### Fix
The error should be silently ignored or the command should be skipped if not in a git repository. Since the fingerprint is optional for functionality, we can just skip this step during HQ installation (it will be done later when git is initialized, or by the first rig that gets git).

---

## Issue 2: "database not initialized" when creating agent beads during `gt rig add`

### Root Cause
In `internal/rig/manager.go`, the `initAgentBeads()` function (line 501) tries to create agent beads in the **town beads database** (`.beads/` at HQ root).

However, if you run `gt rig add` before the town beads database is fully initialized with the correct prefix, the `bd create` command fails because the beads database doesn't have the `issue_prefix` config set.

### Error Output
```
Warning: Could not create agent beads: creating gt-pho2vid-witness: bd create --json --id=gt-pho2vid-witness ...
Error: database not initialized: issue_prefix config is missing (run 'bd init --prefix <prefix>' first)
```

### Why This Happens
The town beads are supposed to be initialized during `gt install` with `bd init --prefix gt`. However:

1. If `gt install` didn't initialize beads (due to `--no-beads` or earlier error)
2. Or if `.beads/config.yaml` exists but the database wasn't fully initialized
3. Then trying to create beads fails

### Sequence Problem
The actual issue is that the golden path doesn't explicitly initialize the beads database before the user runs `gt rig add`. The instructions say:
```
1. Initialize git: gt git-init
2. Add a rig: gt rig add <name> <git-url>
```

But there's no explicit "Initialize beads" step. The assumption is that `gt install` initialized them, but that's not always clear.

### Fix
There are two parts:
1. Make `initAgentBeads()` more graceful - if town beads aren't initialized, either initialize them or log a clearer error
2. Update documentation to make it clear that beads are initialized during `gt install` (if not using `--no-beads`)
3. If the user runs `gt rig add` and town beads aren't initialized, initialize them automatically as part of the rig setup

---

## Getting Started Walkthrough with Issues

```bash
# Step 1: Create HQ
❯ gt install ~/gt
✓ HQ created successfully!
⚠ Could not verify repo fingerprint: ... (NOT A GIT REPO YET)
# This warning is harmless - git hasn't been initialized yet

# Step 2 (Per instructions): Initialize git
❯ cd gt
~/gt ❯ gt git-init
(git repo initialized)

# Step 3 (Per instructions): Add a rig
~/gt ❯ gt rig add pho2vid https://github.com/maphew/pho-vid
Warning: Could not create agent beads: ...
Error: database not initialized: issue_prefix config is missing
# This happens because town beads initialization might have skipped or failed

✓ Rig created in 5.5s  (but with a warning)
```

---

## Summary of Fixes

### Fix 1: Install Fingerprint Warning
**File**: `internal/cmd/install.go` (function `ensureRepoFingerprint`)

**Change**: Check if directory is a git repository before running `bd migrate --update-repo-id`

```go
// Check if this is a git repository
cmd := exec.Command("git", "rev-parse", "--git-dir")
cmd.Dir = beadsPath
if err := cmd.Run(); err != nil {
    // Not a git repository yet - skip fingerprint (it's optional)
    return nil
}
```

**Why**: The `bd migrate --update-repo-id` command requires a git repository. During `gt install`, the directory is just created - git initialization is optional and happens later. Since the fingerprint is optional for functionality (only a daemon optimization), we skip it and let it be set later when git is initialized.

**Result**: No more warning during `gt install` (before git init)

---

### Fix 2: Rig Add Beads Initialization
**File**: `internal/rig/manager.go` 

**Changes**:
1. Added `ensureTownBeadsInitialized()` helper function that:
   - Checks if town beads config already exists (quick path)
   - Runs `bd init --prefix gt` if needed
   - Falls back to creating minimal `config.yaml` if bd is unavailable
   - Is idempotent (safe to run multiple times)

2. Modified `initAgentBeads()` to call this helper before creating agent beads:
   ```go
   // Ensure town beads database is initialized before creating agent beads.
   if err := m.ensureTownBeadsInitialized(townBeadsDir); err != nil {
       return fmt.Errorf("ensuring town beads initialized: %w", err)
   }
   ```

**Why**: If user ran `gt install --no-beads` or if beads initialization failed earlier, town beads won't exist. Trying to create agent beads would fail. This fix automatically initializes town beads on-demand.

**Result**: `gt rig add` works even if town beads weren't initialized during install

---

## After Fixes: Clean Golden Path

```bash
# Step 1: Create HQ
❯ gt install ~/gt
✓ HQ created successfully!
# ✓ No fingerprint warning (not in git repo yet)

# Step 2: Add a rig (works even without git-init - town beads auto-initialized)
~/gt ❯ gt rig add pho2vid https://github.com/maphew/pho-vid
   ✓ Created agent bead: gt-pho2vid-witness
   ✓ Created agent bead: gt-pho2vid-refinery
   ✓ Created agent bead: gt-deacon
   ✓ Created agent bead: gt-mayor

✓ Rig created in X.Xs
# ✓ All agent beads created successfully
# ✓ No "database not initialized" error

# Step 3: Optional - Initialize git later
~/gt ❯ gt git-init
✓ Git initialization complete!
```

**Notes:**
- Both original issues are fixed
- `git-init` is now optional and can be run after `rig add`
- Remaining "could not set role slot" warnings are a separate issue (role bead definitions not yet created)
