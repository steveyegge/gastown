# Reverse Rebase Verification

## Goal
Verify functional parity for each of the 23 commits processed from upstream/main.

## Legend
- **PRESERVED**: Functionality exists in our branch (cherry-picked cleanly or adapted)
- **ADAPTED**: Functionality reimplemented for our architecture
- **NOT_NEEDED**: Applies to deleted code or superseded by refactor
- **MISSING**: Functionality was lost - NEEDS ATTENTION

---

## Commit Verification

### 1. 37f465bd - feat(hooks): add gt block-pr-workflow command
**Status**: PRESERVED
**Evidence**: `internal/cmd/tap_guard.go` contains full implementation of `gt tap guard pr-workflow`
**Verification**: Function `runTapGuardPRWorkflow()` exists with PR blocking logic

### 2. dcf7b810 - refactor(hooks): rename to gt tap guard pr-workflow
**Status**: PRESERVED
**Evidence**: Merged with commit 1, structure matches: `tapGuardCmd` with `tapGuardPRWorkflowCmd` subcommand
**Verification**: Command registered in `internal/cmd/tap.go`

### 3. b8eb9362 - fix(sling): prevent agent self-interruption during tests
**Status**: PRESERVED
**Evidence**: GT_TEST_NO_NUDGE check exists in `internal/cmd/sling_formula.go` and `internal/cmd/sling_helpers.go`
**Verification**: `grep "GT_TEST_NO_NUDGE"` returns matches in sling files

### 4. 544cacf3 - feat(roles): add config-based role definition system (Phase 1)
**Status**: PRESERVED
**Evidence**: Complete role config infrastructure in `internal/config/roles.go`, `internal/config/roles_test.go`, TOML files in `internal/config/roles/`
**Verification**: `LoadRoleDefinition()` function exists and is tested

### 5. a6102830 - feat(roles): switch daemon to config-based roles (Phase 2+3)
**Status**: ADAPTED
**Evidence**: Our refactor uses `factory.Start()` instead of manual session setup; `getRoleConfigForIdentity()` in `internal/daemon/lifecycle.go:282` uses `config.LoadRoleDefinition()`
**Verification**: Daemon lifecycle uses factory pattern but still loads role configs via `LoadRoleDefinition()`

### 6. 08bc632a - fix(session): add instructions for attach topic in startup nudge
**Status**: PRESERVED
**Evidence**: `internal/session/startup.go:73` includes "attach" in the topic check for startup instructions
**Verification**: Code: `if cfg.Topic == "handoff" || cfg.Topic == "cold-start" || cfg.Topic == "attach"`

### 7. e59955a5 - fix(docs): link for go install (#792)
**Status**: PRESERVED
**Evidence**: Documentation fix - link exists in docs
**Verification**: Cherry-picked cleanly

### 8. 55a3b985 - fix(config): correct Claude prompt prefix from > to ❯ (#765)
**Status**: PRESERVED
**Evidence**: Configuration files updated
**Verification**: Cherry-picked cleanly

### 9. 6966eb4c - Escape backticks and dollar signs in quoteForShell (#777)
**Status**: PRESERVED
**Evidence**: `internal/config/types.go:582-593` - `quoteForShell()` escapes backslash, double quote, backtick, and dollar sign
**Verification**: Code includes `strings.ReplaceAll(escaped, "\`", "\\\`")` and `strings.ReplaceAll(escaped, "$", "\\$")`

### 10. 371074cc - Fix tmux error handling for "no current target" (#755)
**Status**: PRESERVED
**Evidence**: `internal/tmux/tmux.go:93` checks for "no current target" and maps to ErrNoServer
**Verification**: Test in `internal/tmux/tmux_test.go:207` verifies this case

### 11. 6c5c6715 - feat(doctor): add routing-mode check (#810)
**Status**: PRESERVED
**Evidence**: Complete implementation in `internal/doctor/routing_mode_check.go` with `Run()` and `Fix()` methods
**Verification**: Check registered and validates beads routing.mode is "explicit"

### 12. 2fe23b7b - fix(done): terminate polecat session for all exit types (#800)
**Status**: PRESERVED
**Evidence**: `internal/cmd/done.go` handles all exit types (COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE) with proper polecat termination
**Verification**: Polecat session management in `internal/polecat/session_manager.go` supports termination

### 13. 08cee416 - fix(handoff): normalize identity in sendHandoffMail (#780)
**Status**: PRESERVED
**Evidence**: `internal/cmd/handoff.go:382` normalizes identity: `agentID = mail.AddressToIdentity(agentID)`
**Verification**: Comment at line 381: "Normalize identity to match mailbox query format"

### 14. 5c45b443 - Add Windows stub for orphan cleanup (#808)
**Status**: PRESERVED
**Evidence**: `internal/util/orphan_windows.go` with build tag `//go:build windows` and stub functions
**Verification**: `FindOrphanedClaudeProcesses()` and `CleanupOrphanedClaudeProcesses()` return nil on Windows

### 15. 6a22b47e - fix(await-signal): update agent last_activity on signal received (#774)
**Status**: PRESERVED
**Evidence**: `internal/cmd/molecule_await_signal.go:162-167` updates last_activity when signal received
**Verification**: Code: `if result.Reason == "signal" && awaitSignalAgentBead != "" { ... updateAgentHeartbeat() }`

### 16. fd612593 - feat: add initial prompt for autonomous patrol startup (#769)
**Status**: ADAPTED
**Evidence**: GUPP (Get Up and Patrol Principle) is handled by `factory.Start()` which sends startup nudges. Our architecture centralizes this in the factory pattern.
**Verification**: `internal/factory/factory.go` handles startup nudge via session package; `internal/witness/manager.go` has NOTE about deacon's approach

### 17. f58a516b - fix(test): remove stale TestInstallTownRoleSlots test (#819)
**Status**: PRESERVED
**Evidence**: Stale test removed as part of cherry-pick
**Verification**: No TestInstallTownRoleSlots test exists (grep returns only this verification doc)

### 18. 477c28c9 - Create initial commit before gh repo create --push
**Status**: PRESERVED
**Evidence**: `internal/cmd/gitinit.go:256-280` - `ensureInitialCommit()` function creates initial commit before push
**Verification**: Code checks `git rev-parse HEAD`, stages files, and commits if needed

### 19. 183a0d7d - fix(crew): use directory name as source of truth in loadState (#785)
**Status**: PRESERVED
**Evidence**: `internal/crew/manager.go:314-317` in `loadState()`:
```go
// Directory name is source of truth for Name and ClonePath.
// state.json can become stale after directory rename, copy, or corruption.
crew.Name = name
crew.ClonePath = m.crewDir(name)
```
**Verification**: Comment and implementation match upstream intent exactly

### 20. b8a679c3 - test: add cross-platform build verification test
**Status**: PRESERVED
**Evidence**: `cmd/gt/build_test.go` - `TestCrossPlatformBuild()` tests builds for linux/darwin/windows/freebsd on amd64/arm64
**Verification**: Test compiles with GOOS/GOARCH/CGO_ENABLED for 6 platform combinations

### 21. 3d5a66f8 - Fixing unit tests on windows (#813)
**Status**: PRESERVED
**Evidence**: Multiple test files have Windows-specific handling with `runtime.GOOS == "windows"` checks
**Verification**: Files include: `internal/util/exec_test.go`, `internal/util/atomic_test.go`, `internal/rig/manager_test.go`, `internal/polecat/session_manager_test.go` (requireTmux helper)

### 22. 48ace2cb - fix(handoff): preserve GT_AGENT across session restarts (#788)
**Status**: ADAPTED
**Evidence**: Implemented in `internal/factory/factory.go` - when `WithAgent()` option used, GT_AGENT env var is set
**Verification**: Code at factory.go:
```go
// GT_AGENT preservation: If an agent override was explicitly specified via
// WithAgent(), set GT_AGENT env var so handoff can read and preserve it.
if cfg.agent != "" {
    envVars["GT_AGENT"] = cfg.agent
}
```
Tests in `internal/factory/factory_test.go`: `TestStartWithAgents_WithAgent_SetsGTAgent` and `TestStartWithAgents_NoOverride_NoGTAgent`

### 23. 0db2bda6 - feat(deacon): add zombie-scan command
**Status**: PRESERVED
**Evidence**: `internal/cmd/deacon.go:319-340` - `deaconZombieScanCmd` with `runDeaconZombieScan()` implementation
**Verification**: Command includes --dry-run flag, finds zombie Claude processes not in active tmux sessions

---

## Summary
- PRESERVED: 20
- ADAPTED: 3
- NOT_NEEDED: 0
- MISSING: 0

**Conclusion**: All 23 commits have functional parity. Three commits (5, 16, 22) required adaptation to our factory-based architecture but preserve the original intent.

**Adapted commits explanation**:
1. **Commit 5** (daemon roles): Our `restartAgent()` uses `factory.Start()` instead of manual `restartSession()`, but role configs are still loaded via `LoadRoleDefinition()`.
2. **Commit 16** (patrol startup): GUPP is handled by `factory.Start()` centrally instead of per-agent `buildStartupCommand`.
3. **Commit 22** (GT_AGENT): Implemented in `factory.Start()` via `WithAgent()` option instead of `BuildStartupCommandWithAgentOverride()`.

**Last Updated**: 2026-01-20
