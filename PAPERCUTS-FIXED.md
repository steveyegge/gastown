# Papercuts Fixed: Golden Path Issues Resolved

## Overview
Fixed two issues encountered during the getting started golden path:

1. **Install Fingerprint Warning** - "Could not verify repo fingerprint"
2. **Rig Add Beads Error** - "database not initialized: issue_prefix config missing"

Both issues are now resolved with clean golden path execution.

## Issue #1: Install Fingerprint Warning

### What Was Happening
```
⚠ Could not verify repo fingerprint: bd migrate --update-repo-id: 
Error: failed to compute repository ID: not a git repository
```

### Root Cause
The `bd migrate --update-repo-id` command requires the directory to already be a git repository. However:
- `gt install` creates a fresh HQ directory
- Git initialization is optional (via `--git` flag or later via `gt git-init`)
- The command fails because it's not in a git repo yet

### Solution
Check if the directory is a git repository before running the migrate command. Skip the fingerprint migration if not in a git repo (it's optional for functionality).

**File**: `internal/cmd/install.go` - `ensureRepoFingerprint()` function

The fix adds a git repository check:
```go
// Check if this is a git repository
cmd := exec.Command("git", "rev-parse", "--git-dir")
cmd.Dir = beadsPath
if err := cmd.Run(); err != nil {
    // Not a git repository yet - skip fingerprint (it's optional)
    return nil
}
```

### Result
✓ No warning during `gt install` (before git initialization)
✓ Fingerprint is set later when git is initialized
✓ All operations remain idempotent

---

## Issue #2: Rig Add Beads Database Error

### What Was Happening
```
Warning: Could not create agent beads: creating gt-pho2vid-witness: 
bd create ... Error: database not initialized: 
issue_prefix config is missing (run 'bd init --prefix <prefix>' first)
```

### Root Cause
Agent beads are stored in the town-level beads database (`.beads/` at HQ root) with the "gt" prefix. When `gt rig add` tries to create agent beads, the database might not be initialized if:
1. User ran `gt install --no-beads` (skipped beads initialization)
2. The `gt install` beads initialization failed earlier
3. Using a legacy HQ without beads

### Solution
Automatically initialize town beads if they don't exist, right before creating agent beads.

**File**: `internal/rig/manager.go` - Added `ensureTownBeadsInitialized()` helper

The fix:
1. Checks if town beads config already exists (quick path)
2. Runs `bd init --prefix gt` if needed
3. Falls back to creating minimal `config.yaml` if bd is unavailable
4. Is idempotent - safe to run multiple times

Called from `initAgentBeads()` before creating any agent beads:
```go
if err := m.ensureTownBeadsInitialized(townBeadsDir); err != nil {
    return fmt.Errorf("ensuring town beads initialized: %w", err)
}
```

### Result
✓ `gt rig add` works even if town beads weren't initialized during install
✓ `gt install --no-beads` now works correctly
✓ Graceful fallback if `bd` command is unavailable
✓ All operations remain idempotent

---

## Clean Golden Path

After these fixes, the full getting started flow is clean:

```bash
# Step 1: Create HQ
❯ gt install ~/gt
🏭 Creating Gas Town HQ at /var/home/matt/gt

   ✓ Created mayor/
   ✓ Created mayor/town.json
   ✓ Created mayor/rigs.json
   ✓ Created rigs/
   ✓ Created mayor/state.json
   ✓ Created CLAUDE.md
   ✓ Initialized .beads/ (town-level beads with gt- prefix)
   ✓ Detected overseer: maphew <maphew@gmail.com> (via git-config)

✓ HQ created successfully!

# Step 2: Initialize git
❯ cd gt
~/gt ❯ gt git-init
✓ Git initialization complete!

# Step 3: Add a rig
~/gt ❯ gt rig add testrig https://github.com/maphew/gastown.git
Creating rig testrig...
   ✓ Created agent bead: gt-testrig-witness
   ✓ Created agent bead: gt-testrig-refinery
   ✓ Created agent bead: gt-deacon
   ✓ Created agent bead: gt-mayor

✓ Rig created in 17.9s
```

**No warnings or errors in the critical path.**

---

## Commits

- `ff556c8` - fix(papercuts): resolve golden path install and rig add warnings
- `76f3ed5` - docs: Add detailed papercuts documentation with fixes applied

## Testing

Both fixes were tested with:
1. Clean `gt install` → `gt git-init` → `gt rig add` flow
2. Edge case: `gt install --no-beads` → `gt git-init` → `gt rig add`
3. Multiple rigs added to same HQ

All scenarios now work without the reported errors or warnings.
