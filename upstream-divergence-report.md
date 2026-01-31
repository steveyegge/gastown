# Upstream Divergence Report: gastown

**Generated:** 2026-01-31
**Comparison:** `steveyegge/gastown:main` vs `groblegark/gastown:main`
**Total Divergent Commits:** 93

## Summary

The upstream repository (steveyegge/gastown) has 93 commits not present in our fork (groblegark/gastown). These changes span core functionality, bug fixes, new features, and documentation updates.

---

## High-Risk Changes (Core Systems, Tests, CI)

### Critical Path Modifications

| Commit | Area | Description | Risk |
|--------|------|-------------|------|
| `c6832e4b` | Core | Remove bd sync references, Gas Town is Dolt-native | **HIGH** - Core workflow change |
| `6f71b5a5` | Config | Set GT_ROLE in compound format for bd hooks | **HIGH** - Env variable format change |
| `dfec0701` | Runtime | Migrate from claude.EnsureSettingsForRole to runtime.EnsureSettingsForRole | **HIGH** - API migration |
| `8b621ec6` | Tmux | Agent-agnostic zombie detection | **MEDIUM** - Process management |
| `f43de726` | Startup | Prevent startup race condition and isolate polecat settings | **MEDIUM** - Race condition fix |
| `1f293e57` | Sling | Defer polecat session start to prevent race condition | **MEDIUM** - Race condition fix |

### Polecat Management (7 commits)
- `71c86559` - Verify and force removal of worktree directories during nuke
- `936fd41e` - Clean up orphan state during name pool reconciliation
- `943a8865` - Verify worktree creation in SpawnPolecatForSling
- `f38e5bfb` - Allow polecat to delete its own worktree on gt done
- `0d86358c` - Prevent startup race condition and isolate polecat settings
- `7824dd11` - Prevent polecat respawn from destroying unmerged MR work
- `802353b9` - Prevent polecat respawn from destroying unmerged MR work (duplicate fix)

### Doctor Checks (8 commits)
- `feab0166` - Add beads-sync worktree health check
- `4500f196` - Add timing display for slow checks
- `72d20e2e` - Add database-prefix check for rig config mismatch
- `6a5261e1` - Add stale-agent-beads check for removed crew
- `c7661cba` - Bypass bd daemon for town-root subprocess calls (perf)
- `ff2c5fe8` - Add beads-sync worktree health check (gt-mihct)
- `8f88e309` - Add --slow flag to highlight slow checks
- `eec8b91f` - Add streaming output for real-time check progress

---

## New Features

### Major Features

| Commit | Feature | Files Changed |
|--------|---------|---------------|
| `8b6b6be7` | Dashboard: command palette, mail compose, API performance | 22 files, +6117/-1156 lines |
| `c493c924` | KRC: TTL-based ephemeral data lifecycle | 6 files, +1311 lines |
| `0da8dad7` | Dog: session management and delayed dispatch | 8 files, +511 lines |
| `7eb364f3` | Molecule DAG visualization command (gt mol dag) | 2 files, +457 lines |
| `a543ee53` | Patrol: fan-out/gather pattern for parallel steps | 5 files, +289 lines |
| `12ecab3b` | Formula: Go text/template rendering for convoy prompts | 1 file, +236 lines |
| `5f7c1658` | Rig: --adopt flag to register existing directories | 2 files, +204 lines |
| `c5e82ada` | Implement gt rig settings command with comprehensive tests | multi-file |
| `db02dd81` | Startup: nudge fallback for non-hook agents | 4 files, +242 lines |
| `ebb8e42b` | Dog: gt dog done command for returning to idle | multi-file |

### Minor Features

- `c80eb4e5` - Add town root CLAUDE.md to gt install and gt doctor
- `7d5b145b` - Copy overlay files to refinery during rig creation
- `aa7656e6` - Distinguish crew from polecats in Workers panel

---

## Bug Fixes by Area

### Mail System (4 commits)
- `82904fa6` - Handle hq- prefix in agentBeadIDToAddress
- `fb4e2921` - Validate rig-level agent recipients
- `38224006` - Handle hq- prefix in agentBeadIDToAddress (duplicate)
- `f41de0d3` - Mark messages as read when viewed via gt mail read

### Sling (4 commits)
- `67e148ef` - Trim trailing slashes from target arguments
- `8167085b` - Retry hook with verification on Dolt concurrency errors
- `c897b6f2` - Trim trailing slashes from target arguments (duplicate)
- `1f293e57` - Defer polecat session start to prevent race condition

### Git Operations (3 commits)
- `859a2c31` - Preserve symlinks when moving directories across filesystems
- `489053d8` - Preserve symlinks when moving directories across filesystems (duplicate)
- `1dc660c5` - Use post-checkout for branch protection (git has no pre-checkout)

### Tmux (2 commits)
- `b5ba951e` - Handle deleted working directory on respawn
- `0605db97` - Use syscall.Kill() for process group termination

### Templates (3 commits)
- `fd25b519` - Directory discipline, bd create clarification, command fixes
- `f886dab8` - Add directory discipline, fix gt mail hook command
- `ef425074` - Emphasize bd ready and molecule steps in polecat template

### Version Checks (3 commits)
- `4eab8ba1` - Improve bd version check resilience during multi-agent startup
- `cdcde82c` - Use --no-daemon to avoid bd version check contention
- `f3728379` - Change stale binary advice from 'gt install' to 'make install'

### Other Fixes
- `3aafa0ee` - Run mayor session from mayorDir instead of townRoot
- `d9af5a5d` - Use correct method name IsAgentRunning instead of IsClaudeRunning
- `9ed9d3a7` - Follow .beads/redirect in FindMigratableDatabases
- `cf722195` - Update bead status to open after clearing hook (unsling)
- `ea765f95` - Resolve short addresses by trying crew then polecat (nudge)
- `653583b9` - Persist hook_bead on agent bead when hooking work
- `c9718086` - Prevent witness crash loop when rig is idle
- `d820a7e3` - Refuse to create HQ inside existing workspace
- `621bbd32` - gt rig dock now stops polecat sessions
- `177094a2` - Remove KillPaneProcessesExcluding race condition (handoff)
- `6e9d8bf2` - Validate rig settings key path to prevent false success
- `f868250d` - Allow .mcp.json in worktree sparse checkout
- `4fc8c6dc` - Update beads fingerprint after git initialization

---

## Performance Improvements

| Commit | Improvement | Impact |
|--------|-------------|--------|
| `d2e81b9e` | Optimize --fast mode from ~5s to ~2s | 60% faster status checks |
| `a3eca2af` | Optimize --fast mode from ~5s to ~2s (duplicate) | - |
| `c7661cba` | Bypass bd daemon for town-root subprocess calls | Faster doctor checks |

---

## Breaking Changes / API Modifications

### Environment Variables
- **`6f71b5a5`**: GT_ROLE now uses compound format for bd hooks
  - Files: `internal/config/env.go`, `internal/config/loader.go`
  - **Action Required:** Verify bd hooks handle new format

### Runtime API Migration
- **`dfec0701`**: Migrate from `claude.EnsureSettingsForRole` to `runtime.EnsureSettingsForRole`
  - Affects: deacon, install, crew, mayor, rig, witness managers
  - **Action Required:** Verify no custom code uses old API

### Dolt-Native Operations
- **`c6832e4b`**: Removed bd sync references - Gas Town is now fully Dolt-native
  - Affects: Templates (crew, mayor, polecat)
  - **Action Required:** Remove any custom bd sync calls

---

## Documentation Updates

| Commit | Document | Description |
|--------|----------|-------------|
| `9aff946a` | Mayor template | Add PR instruction to use origin remote |
| `b49f08f7` | Federation guide | Add Dolt remotes configuration guide |
| `9238635c` | Templates | Clarify bd create is not a gt command |

---

## Contributors

| Contributor | Commits |
|-------------|---------|
| Steve Yegge | 23 |
| mayor (agent) | 9 |
| aleiby | 8 |
| gastown/crew/max | 7 |
| malcolm | 4 |
| Artem Bambalov | 3 |
| rictus | 2 |
| nux | 2 |
| mhawthorne | 2 |
| jack | 2 |
| furiosa | 2 |
| dustin | 2 |
| amos | 2 |
| Ryan Garver | 2 |
| Piotr Trzpil | 4 |
| Clay Cantrell | 2 |
| Avyukth | 2 |
| Others | 15 |

---

## Files Most Affected

| File/Area | Change Count | Notes |
|-----------|--------------|-------|
| `internal/cmd/` | 30+ | Command implementations |
| `internal/polecat/` | 10+ | Polecat lifecycle management |
| `internal/doctor/` | 10+ | Health check additions |
| `internal/templates/roles/` | 8+ | Agent role templates |
| `internal/web/` | 5+ | Dashboard improvements |
| `internal/mail/` | 4+ | Mail routing fixes |
| `internal/config/` | 4+ | Configuration changes |
| `internal/tmux/` | 4+ | Process management |

---

## Merge Recommendations

### Immediate Attention (High Priority)
1. **Polecat stability fixes** - 7 commits addressing race conditions and orphan cleanup
2. **Dolt-native migration** - Template changes removing bd sync
3. **GT_ROLE format change** - Environment variable format update

### Can Merge Safely
1. Doctor check additions (all additive)
2. Documentation updates
3. Performance optimizations
4. Dashboard improvements

### Requires Testing
1. KRC (Key Record Chronicle) - New subsystem
2. Formula template rendering - New templating system
3. Parallel step execution - New patrol pattern

---

## Next Steps

1. Review high-risk commits for potential conflicts with local changes
2. Run full test suite after merge
3. Verify polecat lifecycle after merge (many fixes in this area)
4. Update any custom scripts that use bd sync or old GT_ROLE format
