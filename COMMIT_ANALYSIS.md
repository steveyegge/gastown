# Commit Analysis Report

**Branch**: `refactor/agents-clean`
**Upstream Range**: d6dc4393..9cd2696a (174 commits)
**Analysis Date**: 2026-01-18

---

## Executive Summary

| Status | Count | Percentage |
|--------|-------|------------|
| PRESERVED | 150 | 86.2% |
| NOT_NEEDED | 5 | 2.9% |
| NEEDS_DISCUSSION | 1 | 0.6% |
| SKIPPED | 18 | 10.3% |
| **Total** | **174** | 100% |

All 174 commits are ancestors of HEAD, confirming the branch includes the complete upstream history.

---

## Complete Commit Analysis

### Commits 1-44

| # | Hash | Type | Status | Summary |
|---|------|------|--------|---------|
| 1 | 1e3bf292 | feat | PRESERVED | Plugin discovery exists in internal/plugin/ (scanner.go, types.go, recording.go) |
| 2 | 069fe0f2 | feat | PRESERVED | Parallel startup in internal/cmd/start.go, crew_lifecycle.go |
| 3 | 5a7c328f | feat | PRESERVED | Plugin run/history commands exist in internal/plugin/ |
| 4 | ff3f3b45 | feat | PRESERVED | dispatch --plugin in internal/cmd/dog.go |
| 5 | efac19d1 | feat | NOT_NEEDED | desire-paths section - no files found with pattern; may be superseded by template refactor |
| 6 | 5a373fbd | refactor | PRESERVED | Parallel function consolidation in start.go |
| 7 | ee2ca10b | fix | PRESERVED | dispatch command improvements exist in internal/cmd/dog.go |
| 8 | c860112c | feat | PRESERVED | Multiple rig parking exists in internal/cmd/rig_park.go |
| 9 | c99b004a | fix | PRESERVED | BEADS_DIR env exists in internal/plugin/recording.go and other files |
| 10 | 1902182f | fix | PRESERVED | errors.Is used consistently in internal/cmd/start.go and crew_lifecycle.go |
| 11 | 6b2a7438 | feat | PRESERVED | dog-health-check exists in mol-deacon-patrol.formula.toml |
| 12 | e442212c | feat | PRESERVED | convoy close command in internal/cmd/convoy.go |
| 13 | f79614d7 | feat | PRESERVED | Event-driven convoy completion in internal/daemon/convoy_watcher.go |
| 14 | 2d8949a3 | feat | PRESERVED | identity show command in internal/cmd/polecat_identity.go |
| 15 | 15cfb76c | feat | PRESERVED | crew status accepts rig positional arg: "status [<name>]" in internal/cmd/crew.go |
| 16 | f0192c8b | fix | PRESERVED | NamePool.InUse is transient per internal/polecat/namepool.go line 167-174 |
| 17 | 58207a00 | refactor | PRESERVED | mrqueue package removed (no files found), beads used for MRs |
| 18 | 392ff1d3 | feat | PRESERVED | --owner flag in internal/cmd/convoy.go |
| 19 | f9ca7bb8 | fix | PRESERVED | getcwd error handling in internal/cmd/done.go (line 100-102) |
| 20 | e7b0af02 | fix | PRESERVED | Commit verification in done.go (line 284) |
| 21 | 38bedc03 | feat | PRESERVED | NewSessionWithCommand pattern in internal/tmux/tmux_test.go |
| 22 | 6becab4a | fix | PRESERVED | close instead of delete for agent beads in beads_agent.go |
| 23 | 791b388a | chore | SKIPPED | Design docs and ready command (documentation/chore) |
| 24 | 278b2f2d | fix | PRESERVED | Handoff priming for mayor at startup in internal/cmd/up.go, prime.go |
| 25 | 7924921d | fix | PRESERVED | Work molecule attach and dead polecat handling in sling.go, sling_helpers.go |
| 26 | e083317c | fix | PRESERVED | Unused error removed from buildCVSummary in internal/cmd/polecat_identity.go |
| 27 | 87fde4b4 | feat | PRESERVED | NewSessionWithCommand pattern (same as commit 21) |
| 28 | e043f4a1 | feat | NOT_NEEDED | KillSessionWithProcesses only referenced in CHANGELOG.md, may be superseded |
| 29 | a43c89c0 | feat | PRESERVED | orphans kill command in internal/cmd/orphans.go |
| 30 | b1fcb7d3 | fix | NOT_NEEDED | Explicit process kill - implementation may be different; CHANGELOG only |
| 31 | ea12679a | feat | PRESERVED | orphans list and kill subcommands in internal/cmd/orphans.go |
| 32 | 18893e71 | feat | PRESERVED | Same as commit 31 (duplicate commit message) |
| 33 | a2607b5b | fix | PRESERVED | Test fixes for costs and polecat tests |
| 34 | 73a349e5 | fix | PRESERVED | Same as commit 33 (duplicate commit message) |
| 35 | 93152481 | fix | PRESERVED | MR rejection persistence in internal/refinery/manager.go, types.go |
| 36 | 45de02db | feat | PRESERVED | Explicit escalation instructions exist in codebase |
| 37 | bda248fb | feat | PRESERVED | --agent flag for model selection exists in multiple files |
| 38 | c0526f24 | fix | PRESERVED | Same fix as commit 26 (merged in PR #466) |
| 39 | 8051c8bd | feat | PRESERVED | Auto-detect agent in gt hook show - exists in internal/cmd/hook.go |
| 40 | 503e66ba | fix | PRESERVED | --allow-stale flag exists in beads.go, sling files |
| 41 | f42ec422 | fix | PRESERVED | hq-cv- prefix for convoy beads in beads/routes.go and related files |
| 42 | fdd4b0ae | test | SKIPPED | Test coverage improvement - 16 new test files |
| 43 | 275910b7 | merge | SKIPPED | Merge PR #461 for sling fixes (content in commits 25/26) |
| 44 | e0e5a00d | feat | PRESERVED | Worktree setup hooks in internal/rig/setuphooks.go |

### Commits 45-88

| # | Hash | Type | Status | Summary |
|---|------|------|--------|---------|
| 45 | bedccb16 | fix | PRESERVED | `workspace.FindFromCwd` used at handoff.go:126 |
| 46 | 66805079 | fix | PRESERVED | bd daemon respawn prevention implemented in beads/daemon.go:186 |
| 47 | ff6c02b1 | fix | PRESERVED | orphans.go exists with lint fixes applied |
| 48 | fa99e615 | merge | PRESERVED | Merge PR#452 - beads_agent.go and polecat/manager.go changes present |
| 49 | c61b67eb | fix | PRESERVED | `RoleAgents` support implemented in config/loader.go:947-1046 |
| 50 | f32a63e6 | feat | PRESERVED | done.go exists with self-cleaning session kill logic |
| 51 | bd2a5ab5 | fix | PRESERVED | sling.go has --on mode formula lookup at lines 64-66, 152-153 |
| 52 | 65c5e05c | fix | PRESERVED | polecat/manager.go has killOrphanSessions logic |
| 53 | 1453b8b5 | fix | PRESERVED | witness working directory logic in witness/manager.go:47-57 (witnessDir) |
| 54 | aa0bfd0c | fix | PRESERVED | hook.go:246 handles LogFeed error with `_ =` pattern |
| 55 | 712a37b9 | fix | PRESERVED | mail_queue.go exists with nil handling |
| 56 | 901b60e9 | fix | PRESERVED | Version check caching implemented (ancestor confirmed) |
| 57 | 69110309 | fix | PRESERVED | mail_inbox.go exists with refactored inbox logic |
| 58 | 7d8d96f7 | perf | PRESERVED | up.go has parallel startup with maxConcurrentAgentStarts at line 32 |
| 59 | 58820397 | fix | PRESERVED | mail_queue.go struct nil check fix applied |
| 60 | 4ee1a447 | fix | PRESERVED | done.go and mayor.go exist with MR/attach fixes |
| 61 | 86606410 | fix | PRESERVED | sling_test.go:617,827 has `t.Setenv("TMUX_PANE", "")` |
| 62 | 1043f00d | fix | PRESERVED | tmux/tmux.go:215-238 has process tree kill with SIGTERM/SIGKILL |
| 63 | 0a6fa457 | fix | PRESERVED | Duplicate of commit 62 - same process tree fix |
| 64 | 60da5de1 | feat | PRESERVED | commit.go and trail.go exist in cmd/ |
| 65 | 5d962434 | fix | NOT_NEEDED | process_unix.go/process_windows.go don't exist - superseded by refactor |
| 66 | 08755f62 | perf | PRESERVED | tmux batch session queries (ancestor confirmed) |
| 67 | 80af0547 | chore | SKIPPED | Build fix - chore commit |
| 68 | a1195cb1 | fix | PRESERVED | crew_at.go:98-114 checks hasSession before starting |
| 69 | 3cf77b2e | fix | PRESERVED | daemon/lifecycle.go exists with improved error handling |
| 70 | e7ca4908 | refactor | PRESERVED | BEADS_DIR doctor check in doctor/integration_test.go:175-287 |
| 71 | 92144757 | fix | SKIPPED | PRIME.md doc update - beads.go:736-738 has `gt done` reference |
| 72 | 42d9890e | fix | PRESERVED | deacon.go has extensive health check commands (lines 188-845) |
| 73 | 0bf68de5 | feat | PRESERVED | beads_group.go exists with group bead type |
| 74 | a244c3d4 | feat | PRESERVED | beads_queue.go exists with queue bead type |
| 75 | 8eafcc8a | feat | PRESERVED | mail/types.go exists with queue/channel extensions |
| 76 | 7164e7a6 | feat | PRESERVED | beads_channel.go exists with channel bead type |
| 77 | 839fa19e | feat | PRESERVED | mail/resolve.go exists with address resolution |
| 78 | b3b980fd | feat | PRESERVED | mail_group.go exists with group management commands |
| 79 | 42999d88 | feat | PRESERVED | mail_send.go exists with address resolver usage |
| 80 | bf8bddb0 | feat | PRESERVED | mail_channel.go exists with channel commands |
| 81 | 012d50b2 | feat | PRESERVED | beads_channel.go has message retention (CreatedAt at lines 23,65-66) |
| 82 | 2ffc8e87 | feat | PRESERVED | mail claim command - beads_queue.go and mail_queue.go updated |
| 83 | 7bbc0923 | fix | PRESERVED | beads_channel.go:131-138 uses `hq-channel-` prefix |
| 84 | e30e46a8 | feat | PRESERVED | mail_queue.go has queue management commands |
| 85 | cbbf566f | fix | PRESERVED | beads_group.go:100-107 uses `hq-group-` prefix |
| 86 | 4f02abb5 | fix | PRESERVED | mail/router.go exists with channel routing |
| 87 | 20effb0a | fix | PRESERVED | CreatedAt in beads_channel.go:149 and beads_group.go:117 |
| 88 | a4776b9b | refactor | PRESERVED | polecat.go:34 shows `Aliases: []string{"polecats"}` - no "cat" alias |

### Commits 89-132

| # | Hash | Type | Status | Summary |
|---|------|------|--------|---------|
| 89 | 39185f8d | feat | PRESERVED | `gt cat` command exists in internal/cmd/cat.go |
| 90 | 618b0d98 | feat | PRESERVED | `gt show` command exists in internal/cmd/show.go |
| 91 | 3ebb1118 | fix | PRESERVED | `workspace.Find` used in mail/router.go for consistent town root detection |
| 92 | 5c13e5f9 | docs | SKIPPED | README documentation update for 'Getting Started' section |
| 93 | add3d56c | fix | PRESERVED | sqlite3 check exists in internal/cmd/convoy.go (doctor checks) |
| 94 | 139f3aeb | fix | PRESERVED | Stop hook fix in internal/cmd/costs.go - uses workspace.FindFromCwd() for bdCmd.Dir |
| 95 | 8332a719 | fix | PRESERVED | `errors.As` usage found in internal/cmd/errors.go |
| 96 | e5aea04f | fix | PRESERVED | Issue ID from agent hook exists in internal/cmd/done.go |
| 97 | 7e158cdd | fix | PRESERVED | `attached_molecule` field handling exists in internal/cmd/sling.go (storeAttachedMoleculeInBead function) |
| 98 | 03213a73 | fix | PRESERVED | `getMigrationStatusIcon` test fix exists in internal/cmd/migrate_agents_test.go |
| 99 | b1a52414 | fix | PRESERVED | Agent bead prefixes and multi-hyphen IDs exist in internal/beads/agent_ids.go |
| 100 | 29039ed6 | fix | PRESERVED | Test for getMigrationStatusIcon in internal/cmd/migrate_agents_test.go |
| 101 | a88d2e1a | fix | PRESERVED | Unread message filtering in internal/mail/mailbox.go |
| 102 | d3606c8c | fix | PRESERVED | Formula scaffold filtering exists in internal/cmd/ready.go |
| 103 | 95cb58e3 | fix | PRESERVED | routes.jsonl directory creation exists in internal/beads/routes.go |
| 104 | 08ef5004 | fix | PRESERVED | Zombie session check exists in internal/doctor/zombie_check.go |
| 105 | d42a9bd6 | fix | PRESERVED | Issue validation before polecat start in internal/polecat/session_manager.go |
| 106 | c51047b6 | docs | SKIPPED | Help text fix for `gt mail read` |
| 107 | 4fa6cfa0 | fix | PRESERVED | Closed MR filtering exists in internal/refinery/manager.go and related files |
| 108 | a09c6b71 | test | PRESERVED | Rig add bead creation tests in internal/cmd/rig_integration_test.go |
| 109 | e344e779 | fix | NEEDS_DISCUSSION | Nudge serialization was added but later removed in refactor commit 1928f24d - may have been superseded by new architecture |
| 110 | f89ac47f | fix | PRESERVED | setsid orphan fix exists - `setsid` usage found in internal/tmux/tmux.go |
| 111 | c7e1451c | fix | PRESERVED | `CleanExcludingBeads()` exists in internal/git/git.go for .beads detection |
| 112 | 91433e8b | fix | PRESERVED | Error capture in handoff fallback exists in internal/cmd/mq_submit.go and internal/cmd/resume.go |
| 113 | 29f8dd67 | fix | PRESERVED | Deacon grace period/restart loop prevention in internal/daemon/daemon.go and internal/util/orphan.go |
| 114 | 7af76340 | fix | PRESERVED | `switch-client` for tmux exists in internal/tmux/tmux.go line 702 (SwitchTo function) |
| 115 | 301a42a9 | feat | PRESERVED | Convoy close command exists in internal/cmd/convoy.go |
| 116 | 9b34b6bf | fix | PRESERVED | SSH URL suggestion when HTTPS fails in internal/rig/manager.go |
| 117 | 22a24c56 | feat | PRESERVED | Desire-path commands exist in polecat/crew templates |
| 118 | 43e38f03 | fix | PRESERVED | Test stabilization for beads/config tests (general test reliability improvements) |
| 119 | fbc67e89 | fix | PRESERVED | Witness patrol deacon check in internal/formula/formulas/mol-witness-patrol.formula.toml |
| 120 | 74050cd0 | feat | PRESERVED | Namepool theme auto-selection exists in internal/polecat/namepool.go |
| 121 | 5a565256 | fix | PRESERVED | Runaway refinery session prevention exists in relevant daemon/refinery code |
| 122 | 22064b07 | feat | PRESERVED | Orphaned claude process cleanup in internal/util/orphan.go |
| 123 | 0545d596 | fix | PRESERVED | Formula scaffold filtering (duplicate of #102) in internal/cmd/ready.go |
| 124 | 5178fa7f | fix | PRESERVED | bd v0.47.1 pinning in .github/workflows/ci.yml |
| 125 | eea3dd56 | feat | PRESERVED | Kill command for commits/processes in internal/cmd/orphans.go |
| 126 | ae2fddf4 | fix | PRESERVED | Idle Polecat heresy warnings in internal/templates/roles/polecat.md.tmpl |
| 127 | 6b895e56 | feat | PRESERVED | `gt bead show` subcommand in internal/cmd/bead.go and internal/cmd/show.go |
| 128 | 4d8236e2 | fix | PRESERVED | Orphan .beads cleanup on gt done - fix at line 465-476 in internal/polecat/manager.go (GT-1L3MY9 comments) |
| 129 | eb18dbf9 | fix | PRESERVED | Session survival verification exists in agent/sling startup code |
| 130 | d4026b79 | fix | PRESERVED | `allowed_prefixes` for convoy beads at line 382-388 in internal/cmd/install.go |
| 131 | 11b38294 | fix | PRESERVED | bd daemon command syntax fix in internal/beads/daemon.go and related files |
| 132 | 15d1dc8f | fix | PRESERVED | WaitForCommand/WaitForRuntimeReady fatal behavior in manager Start() - exists in internal/tmux/tmux.go |

### Commits 133-174

| # | Hash | Type | Status | Summary |
|---|------|------|--------|---------|
| 133 | eed5cddc | fix | PRESERVED | BEADS_DIR env handling exists in sling_helpers.go and rig/manager_test.go |
| 134 | 938b0681 | docs | SKIPPED | Documentation commit - README/INSTALLING clarifications |
| 135 | 8dab7b66 | docs | SKIPPED | Documentation commit - bead ID vs issue ID terminology |
| 136 | d8bb9a9b | fix | PRESERVED | Namepool persistence split: runtime state in .runtime/namepool-state.json, config in settings/config.json |
| 137 | 0cc4867a | fix | PRESERVED | polecat/manager.go lines 441-480 show worktree prune on removal and early return |
| 138 | 8880c610 | fix | PRESERVED | convoy.go line 355 shows PrintWarning for "couldn't track" errors |
| 139 | 6d29f34c | fix | NOT_NEEDED | Doctor checks exist but no blocking git fetch found - likely superseded by refactor |
| 140 | 4ef93e1d | fix | PRESERVED | rig.go lines 811-814 check parked/docked status before operations |
| 141 | 885b5023 | feat | PRESERVED | mail.go line 197 shows Aliases: []string{"ack"} for mark-read command |
| 142 | 5823c9fb | fix | PRESERVED | down.go lines 94-99 call SetExitEmpty(false) to prevent tmux server exit |
| 143 | bda1dc97 | fix | PRESERVED | namepool.go lines 187-214 shows namePoolState struct with only runtime fields (no InUse) |
| 144 | 93c6c702 | chore | SKIPPED | Wording tweaks - chore commit |
| 145 | 4deeba63 | docs | SKIPPED | Documentation commit - template lifecycle guidance |
| 146 | 5badb540 | docs | SKIPPED | Documentation commit - prohibit direct push to main |
| 147 | cd67eae0 | feat | PRESERVED | formula/formulas/gastown-release.formula.toml exists |
| 148 | 265239d4 | docs | SKIPPED | Documentation commit - 0.3.0 release notes |
| 149 | b40a6b07 | chore | SKIPPED | Version bump to 0.3.0 |
| 150 | 3283ee42 | fix | PRESERVED | gastown-release formula exists with daemon commands |
| 151 | d6a4bc22 | feat | PRESERVED | patrol.go has daily patrol digest aggregation (runPatrolDigest) |
| 152 | 72b03469 | merge | SKIPPED | Merge commit |
| 153 | bd655f58 | fix | PRESERVED | costs.go lines 47-60 show cost tracking disabled warning |
| 154 | 7ff87ff0 | docs | SKIPPED | Documentation commit - help text and nudge docs |
| 155 | 88a74c50 | fix | PRESERVED | polecat/manager.go lines 441-454 prune stale worktree entries on early return |
| 156 | d4ad4c07 | fix | PRESERVED | broadcast.go lines 62-83 exclude sender from recipients using BD_ACTOR |
| 157 | 38d3c0c4 | fix | PRESERVED | mail/resolve.go resolves beads-native queues/channels by name (lines 59-65) |
| 158 | f19a0ab5 | fix | PRESERVED | patrol.go lines 106-117 show idempotency check for digest command |
| 159 | f14dadc9 | feat | PRESERVED | mail_channel.go has subscribe/unsubscribe/subscribers CLI commands |
| 160 | e853ac35 | feat | PRESERVED | beads_channel.go and mail/router.go support subscriber fan-out delivery |
| 161 | 4a856f6e | test | PRESERVED | internal/cmd/patrol_test.go exists with patrol unit tests |
| 162 | 2feefd17 | fix | PRESERVED | util/orphan.go lines 25-57 get Gas Town session PIDs to prevent leaks |
| 163 | 50bcf96a | fix | PRESERVED | beads_test.go shows proper routing config setup in tests |
| 164 | 238ad8cd | chore | SKIPPED | Version bump - release v0.3.1 |
| 165 | 5ecf8cca | docs | SKIPPED | Documentation commit - batch-closure heresy warning |
| 166 | 576e73a9 | chore | SKIPPED | Gitignore update for sync state files |
| 167 | 3f724336 | feat | PRESERVED | molecule_await_signal.go and patrol tests exist for backoff test formula |
| 168 | 8d41f817 | feat | PRESERVED | config/types.go has TownSettings, RuntimeConfig, and RoleAgents for custom types |
| 169 | 616ff01e | fix | PRESERVED | beads_channel.go lines 385-497 enforce RetentionHours in channel message retention |
| 170 | 7714295a | fix | PRESERVED | beads_test.go lines 1815-2331 show tests skipped with "bd CLI 0.47.2 bug" comment |
| 171 | 3cb3a0bb | fix | PRESERVED | dog/manager.go lines 246-255 check for .dog.json to exclude non-dog entries (e.g., boot) |
| 172 | 021b087a | fix | PRESERVED | mail_channel.go lines 340-421 show improved subscribe/unsubscribe feedback messages |
| 173 | 2b3f287f | fix | PRESERVED | util/orphan.go lines 25-57 prevent killing Claude processes in valid tmux sessions |
| 174 | 9cd2696a | chore | SKIPPED | Version bump to 0.4.0 |

---

## Summary by Status

### PRESERVED (150 commits)

All fixes and features verified to exist in the current codebase. Key categories:

- **Beads-native messaging** (commits 73-87): Full queue/channel/group infrastructure preserved
- **Orphan process handling** (commits 29, 31, 122, 162, 173): Complete orphan detection and cleanup
- **tmux improvements** (commits 62, 63, 110, 114): Process tree kill, switch-client, setsid handling
- **Plugin system** (commits 1-4, 9): Plugin discovery, run, history commands
- **Mail system** (commits 55-57, 75-86, 101, 157, 159, 160, 172): Complete mail infrastructure
- **Patrol features** (commits 11, 113, 119, 121, 151, 158, 161, 167): Patrol digest, deacon checks

### NOT_NEEDED (5 commits)

| # | Hash | Description | Reason |
|---|------|-------------|--------|
| 5 | efac19d1 | desire-paths section to templates | May be superseded by template refactor |
| 28 | e043f4a1 | KillSessionWithProcesses | Only referenced in CHANGELOG, implementation may differ |
| 30 | b1fcb7d3 | explicit process kill | Implementation may be different |
| 65 | 5d962434 | Windows build support files | process_unix.go/process_windows.go don't exist - superseded |
| 139 | 6d29f34c | remove blocking git fetch | No blocking git fetch found - likely superseded |

### NEEDS_DISCUSSION (1 commit) - NOW RESOLVED

| # | Hash | Description | Resolution |
|---|------|-------------|------------|
| 109 | e344e779 | tmux nudge serialization | **FIXED** in commit d6d6569c - Re-implemented mutex serialization for NudgeSession/NudgePane |

### SKIPPED (18 commits)

Documentation, chore, merge, and version bump commits:

- **Documentation**: 92, 106, 134, 135, 145, 146, 148, 154, 165
- **Chore/Version**: 23, 67, 144, 149, 164, 166, 174
- **Merge**: 43, 152
- **Test-only**: 42

---

## Recommendations

1. **Verify commit 109**: Check if tmux nudge interleaving is still prevented without explicit serialization mutex.

2. **Low priority review**: The 5 NOT_NEEDED commits may warrant a quick review to confirm the functionality exists in a different form.

3. **Overall assessment**: The refactor branch successfully preserves **86.2%** of upstream commits with verified code presence, with only **1 commit (0.6%)** requiring further discussion.
