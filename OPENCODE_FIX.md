# OpenCode Feature Parity Plan for Gastown

## Executive Summary

This document enumerates all features implemented for Claude Code in the gastown project and identifies what needs to be implemented for OpenCode to achieve full feature parity. The goal is to ensure opencode agents behave identically to Claude agents in the gastown multi-agent system.

## Current State Analysis

### Claude Features (Fully Implemented)

1. **Settings Management** (`internal/claude/settings.go`)
   - JSON-based settings.json configuration
   - Role-based templates (autonomous vs interactive)
   - Hooks configuration (SessionStart, UserPromptSubmit, PreToolUse, PreCompact, Stop)
   - Embedded config templates via go:embed

2. **Hooks System** (`internal/claude/config/settings-*.json`)
   - SessionStart: Runs `gt prime --hook && gt mail check --inject && gt nudge deacon session-started`
   - UserPromptSubmit: Runs `gt mail check --inject`
   - PreToolUse: Guards for PR workflow, git branch creation
   - PreCompact: Runs `gt prime --hook`
   - Stop: Runs `gt costs record`

3. **Doctor Checks** (Multiple files in `internal/doctor/`)
   - `commands_check.go`: Validates `.claude/commands/` is provisioned
   - `settings_check.go`: Validates settings.json files match templates
   - `priming_check.go`: Validates priming subsystem configuration
   - `hooks_sync_check.go`: Validates hooks are synchronized

4. **Wrapper Scripts** (`internal/wrappers/`)
   - `gt-codex`: Wrapper that runs `gt prime` before launching codex

5. **Template System** (`internal/templates/`)
   - Role templates (mayor, witness, refinery, polecat, crew, deacon)
   - Message templates (spawn, nudge, escalation, handoff)
   - CLAUDE.md generation for each role

6. **Sparse Checkout** (`internal/git/git.go`)
   - Excludes `.claude/`, `CLAUDE.md`, `CLAUDE.local.md` from git worktrees

7. **Session Management** (`internal/daemon/lifecycle.go`)
   - Special handling for Claude processes
   - Process detection and cleanup

---

## OpenCode Implementation Status

### ✅ Fully Implemented

1. **Plugin System** (`internal/opencode/plugin.go`, `plugin/gastown.js`)
   - JavaScript plugin that transforms messages before LLM processing
   - Handles `[GT_AGENT_INIT]` trigger message
   - Injects `gt prime` context for autonomous roles
   - Mail injection for polecat, witness, refinery, deacon
   - Model fallback on credit exhaustion
   - Context preservation during session compaction
   - Session cost recording on deletion
   - Package.json with `@opencode-ai/plugin` dependency

2. **Agent Template** (`templates/agents/opencode.json.tmpl`)
   - JSON template for OpenCode agent configuration
   - Provider/model configuration
   - Runtime settings with delay configuration
   - Hooks configuration

3. **Model Presets** (`templates/agents/opencode-models.json`)
   - Model-specific delay settings
   - Provider configurations (openrouter, openai, google, xai, github-copilot)

4. **Wrapper Script** (`internal/wrappers/scripts/gt-opencode`)
   - Wrapper that runs `gt prime` before launching opencode

5. **Doctor Check** (`internal/doctor/opencode_commands_check.go`)
   - Validates `.opencode/commands/` is provisioned at town level
   - Auto-fixes by provisioning missing commands

6. **Command Provisioning** (`internal/templates/commands/provision.go`)
   - Agent-agnostic command provisioning
   - Supports both "claude" and "opencode" agents
   - Builds commands with provider-specific frontmatter

7. **Settings Check** (`internal/doctor/settings_check.go`)
   - Validates both `.claude` and `.opencode` directories
   - Detects stale settings files in wrong locations

8. **Priming Check** (`internal/doctor/priming_check.go`)
   - Validates AGENTS.md exists alongside CLAUDE.md for OpenCode compatibility
   - Creates AGENTS.md with same content as CLAUDE.md

9. **Sparse Checkout** (`internal/git/git.go`)
   - Excludes `.opencode/` alongside `.claude/`
   - Excludes `AGENTS.md` alongside `CLAUDE.md`

10. **Runtime Configuration** (`internal/runtime/runtime.go`)
    - Provider-specific paths for OpenCode (`.opencode/plugins`, `gastown.js`)
    - Plugin provisioning via `opencode.EnsurePluginAt()`
    - Distinguishes OpenCode's message-transforming hooks from Claude's shell-command hooks

11. **Session Lifecycle** (`internal/daemon/lifecycle.go`)
    - Special initialization sequence for OpenCode agents
    - Sends `[GT_AGENT_INIT]` trigger message before startup beacon
    - Sets `GT_AUTO_INIT=1` environment variable for OpenCode plugin

12. **Sling Helpers** (`internal/cmd/sling_helpers.go`)
    - Provider-specific nudge handling for OpenCode
    - Sends `[GT_AGENT_INIT]` trigger instead of text prompt when slinging to OpenCode agents

13. **Mayor Attach** (`internal/cmd/mayor.go`)
    - Detects OpenCode provider and includes `gt prime` instruction in beacon
    - Handles runtime restart with proper context for OpenCode

14. **Hooks Detection** (`internal/doctor/types.go`)
    - `DetectHooksDir()` function checks for both `.claude` and `.opencode` directories
    - Returns appropriate directory based on what exists

15. **Install Command** (`internal/cmd/install.go`)
    - Installs both `gt-codex` and `gt-opencode` wrapper scripts
    - Provisions both Claude and OpenCode configurations

16. **Uninstall Command** (`internal/cmd/uninstall.go`)
    - Removes both `gt-codex` and `gt-opencode` wrapper scripts

---

## Gap Analysis: Missing OpenCode Features

### 🔴 Critical Gaps (Must Fix)

1. **Hooks Sync Check** (`internal/doctor/hooks_sync_check.go`)
   - **Current**: Only checks `.claude/settings.json` files
   - **Gap**: Does NOT check `.opencode/plugins/gastown.js` files
   - **Fix Needed**: Extend to validate OpenCode plugin is synchronized
   - **Lines**: 118-119 hardcode `.claude` directory

2. **Hooks Install Command** (`internal/cmd/hooks_install.go`)
   - **Current**: Only installs to `.claude/settings.json`
   - **Gap**: Does NOT install to `.opencode/plugins/gastown.js`
   - **Fix Needed**: Add OpenCode plugin installation path
   - **Lines**: 194, 236 hardcode `.claude` directory

3. **Hooks Scan Command** (`internal/cmd/hooks_scan.go`)
   - **Current**: Only scans `.claude/settings.json` files
   - **Gap**: Does NOT scan `.opencode/plugins/gastown.js` files
   - **Fix Needed**: Add OpenCode plugin scanning
   - **Lines**: 67, 132 reference `.claude/settings.json`

4. **Hooks Sync Command** (`internal/cmd/hooks_sync.go`)
   - **Current**: Only syncs `.claude/settings.json` files
   - **Gap**: Does NOT sync `.opencode/plugins/gastown.js` files
   - **Fix Needed**: Add OpenCode plugin synchronization
   - **Lines**: 18-19 description mentions only `.claude`

5. **Config Check** (`internal/doctor/config_check.go`)
   - **Current**: Only checks `.claude/settings.json` for agents
   - **Gap**: Does NOT check `.opencode` directories
   - **Fix Needed**: Extend to check both providers
   - **Lines**: 511, 525 reference `.claude` only

### 🟡 Medium Priority Gaps (Should Fix)

6. **Deacon Cleanup** (`internal/cmd/deacon.go`)
   - **Current**: Only cleans up "claude" and "claude-code" processes
   - **Gap**: Does NOT clean up "opencode" or "node" processes
   - **Fix Needed**: Add opencode process cleanup
   - **Lines**: 247-254 mention only claude processes

7. **Crash Report Check** (`internal/doctor/crash_report_check.go`)
   - **Current**: Only checks for "claude" and "claude-code" processes
   - **Gap**: Does NOT check for "opencode" processes
   - **Fix Needed**: Add opencode to process list
   - **Lines**: 73-74 list only claude processes

8. **Orphan Check** (`internal/doctor/orphan_check.go`)
   - **Current**: Only matches "claude", "claude-code", and "codex" processes
   - **Gap**: Does NOT match "opencode" processes
   - **Fix Needed**: Add opencode to process matching
   - **Lines**: 421-423 filter only claude processes

9. **Down Command** (`internal/cmd/down.go`)
   - **Current**: Only finds "claude/node" processes
   - **Gap**: Does NOT find "opencode" processes
   - **Fix Needed**: Add opencode process detection
   - **Lines**: 438 mentions only claude/node

10. **Web API** (`internal/web/api.go`)
    - **Current**: Checks if "opencode" is running (line 1340)
    - **Status**: Already implemented ✓
    - **Note**: Already has opencode detection

### 🟢 Low Priority / Documentation Gaps

11. **Command Body Differences** (`internal/templates/commands/bodies/handoff.md` vs `internal/templates/commands-opencode/handoff.md`)
    - **Current**: Two different handoff.md files with different content
    - **Issue**: The opencode version has different instructions (sends mail to self first)
    - **Fix Needed**: Ensure both versions work correctly for their respective agents

12. **Hooks Sync Check** (`internal/doctor/hooks_sync_check.go`)
    - **Line 118-119**: Hardcoded `.claude` directory reference
    - **Needs**: Support for `.opencode/plugins/gastown.js`

13. **Hooks Install** (`internal/cmd/hooks_install.go`)
    - **Line 194**: Hardcoded `.claude/settings.json`
    - **Line 236**: Hardcoded `.claude` directory
    - **Needs**: Support for `.opencode/plugins/gastown.js`

14. **Hooks Scan** (`internal/cmd/hooks_scan.go`)
    - **Line 67**: References `.claude/settings.json`
    - **Line 132**: Function parses `.claude/settings.json`
    - **Needs**: Support for `.opencode/plugins/gastown.js`

15. **Hooks Sync** (`internal/cmd/hooks_sync.go`)
    - **Line 18-19**: Description mentions only `.claude/settings.json`
    - **Line 122**: Hardcoded `.claude` directory
    - **Line 163**: Hardcoded `.claude` directory
    - **Needs**: Support for `.opencode/plugins/gastown.js`

16. **Config Check** (`internal/doctor/config_check.go`)
    - **Line 511**: Comment mentions both directories
    - **Line 525**: Only checks `.claude/settings.json`
    - **Needs**: Check `.opencode/plugins/gastown.js` as well

---

## Implementation Priority Matrix

### Phase 1: Critical (Must Have for Parity)

| # | File | Issue | Effort | Impact |
|---|------|-------|--------|--------|
| 1 | `hooks_sync_check.go` | Only checks .claude, not .opencode | Medium | High |
| 2 | `hooks_install.go` | Only installs to .claude | Medium | High |
| 3 | `hooks_scan.go` | Only scans .claude files | Medium | High |
| 4 | `hooks_sync.go` | Only syncs .claude files | Medium | High |
| 5 | `config_check.go` | Only checks .claude config | Low | Medium |

### Phase 2: Important (Should Have)

| # | File | Issue | Effort | Impact |
|---|------|-------|--------|--------|
| 6 | `deacon.go` | Cleanup only claude processes | Low | Medium |
| 7 | `crash_report_check.go` | Only checks claude processes | Low | Low |
| 8 | `orphan_check.go` | Only matches claude processes | Low | Medium |
| 9 | `down.go` | Only finds claude/node | Low | Medium |

### Phase 3: Nice to Have

| # | File | Issue | Effort | Impact |
|---|------|-------|--------|--------|
| 10 | Command bodies | Different handoff.md content | Low | Low |
| 11 | Documentation | Update comments | Low | Low |

---

## Detailed Implementation Notes

### 1. Hooks Sync Check (`internal/doctor/hooks_sync_check.go`)

**Current Behavior:**
- Only discovers and validates `.claude/settings.json` files
- Compares against embedded templates
- Reports drift between actual and expected hooks

**Required Changes:**
- Add discovery of `.opencode/plugins/gastown.js` files
- Add comparison logic for JavaScript plugin content
- Report drift for OpenCode plugins
- Support both providers in the same check

**Key Code Locations:**
- Line 118: `claudeDir := target.Path[:len(target.Path)-len("/settings.json")]`
- Line 119: Hardcoded `.claude` directory creation

### 2. Hooks Install Command (`internal/cmd/hooks_install.go`)

**Current Behavior:**
- Installs hooks to `.claude/settings.json` only
- Creates directory structure for Claude

**Required Changes:**
- Add `--provider` flag to specify claude/opencode
- Add installation path for `.opencode/plugins/gastown.js`
- Create `.opencode/plugins/` directory structure
- Run `npm install` or `bun install` for OpenCode plugin dependencies

**Key Code Locations:**
- Line 194: `settingsPath := filepath.Join(worktreePath, ".claude", "settings.json")`
- Line 236: `claudeDir := filepath.Dir(target.Path)`

### 3. Hooks Scan Command (`internal/cmd/hooks_scan.go`)

**Current Behavior:**
- Scans for `.claude/settings.json` files
- Displays hooks by type (SessionStart, UserPromptSubmit, etc.)
- Parses JSON hook definitions

**Required Changes:**
- Add scanning for `.opencode/plugins/gastown.js` files
- Parse JavaScript plugin hooks
- Display OpenCode-specific hook information
- Support both providers in output

**Key Code Locations:**
- Line 67: `// Find all .claude/settings.json files via DiscoverTargets`
- Line 132: `func parseHooksFile(path string) ([]HookEntry, error)` - only parses JSON

### 4. Hooks Sync Command (`internal/cmd/hooks_sync.go`)

**Current Behavior:**
- Syncs `.claude/settings.json` files from templates
- Regenerates hooks based on role type
- Only supports Claude

**Required Changes:**
- Add `--provider` flag
- Sync `.opencode/plugins/gastown.js` from embedded template
- Support both Claude and OpenCode in the same command
- Update description to mention both providers

**Key Code Locations:**
- Line 18-19: Description only mentions `.claude/settings.json`
- Line 122: `// syncTarget syncs a single target's .claude/settings.json.`
- Line 163: `claudeDir := filepath.Dir(target.Path)`

### 5. Config Check (`internal/doctor/config_check.go`)

**Current Behavior:**
- Checks `.claude/settings.json` for configuration issues
- Validates agent settings structure

**Required Changes:**
- Extend to check `.opencode/plugins/gastown.js` as well
- Validate OpenCode plugin configuration
- Support both providers

**Key Code Locations:**
- Line 511: Comment mentions both directories but code only checks `.claude`
- Line 525: `agentSettings := filepath.Join(townRoot, agent, ".claude", "settings.json")`

---

## Phase 2: Process Management Gaps

### 6. Deacon Cleanup (`internal/cmd/deacon.go`)

**Current Behavior:**
- Cleans up orphaned "claude" and "claude-code" processes
- Identifies processes without TTY

**Required Changes:**
- Add "opencode" and "node" (for OpenCode plugin) to process cleanup
- Update descriptions to mention OpenCode

**Key Code Locations:**
- Line 247-254: Only mentions claude processes
- Line 1169-1233: Process cleanup logic

### 7. Crash Report Check (`internal/doctor/crash_report_check.go`)

**Current Behavior:**
- Checks for "claude" and "claude-code" processes in crash reports

**Required Changes:**
- Add "opencode" to the list of processes to check

**Key Code Locations:**
- Line 73-74: `processes: []string{"tmux", "claude", "claude-code"}`

### 8. Orphan Check (`internal/doctor/orphan_check.go`)

**Current Behavior:**
- Matches "claude", "claude-code", and "codex" processes
- Identifies orphaned agent processes

**Required Changes:**
- Add "opencode" to process matching
- Update filtering logic

**Key Code Locations:**
- Line 421-423: `if cmd != "claude" && cmd != "claude-code" && cmd != "codex"`

### 9. Down Command (`internal/cmd/down.go`)

**Current Behavior:**
- Finds "claude/node" processes for shutdown

**Required Changes:**
- Add "opencode" process detection
- Update process finding logic

**Key Code Locations:**
- Line 438: `// Use pgrep to find all claude/node processes`

---

## Implementation Strategy

### Phase 1: Critical Path (Week 1)

Priority: Enable OpenCode to work with all hook-related commands

1. **Update hooks_sync_check.go**
   - Add OpenCode plugin discovery
   - Add JavaScript content comparison
   - Test with both providers

2. **Update hooks_install.go**
   - Add `--provider` flag
   - Add OpenCode installation path
   - Add npm/bun install for dependencies

3. **Update hooks_scan.go**
   - Add OpenCode plugin scanning
   - Add JavaScript hook parsing
   - Update output formatting

4. **Update hooks_sync.go**
   - Add `--provider` flag
   - Add OpenCode plugin sync
   - Update command description

5. **Update config_check.go**
   - Add OpenCode plugin validation
   - Check both providers

### Phase 2: Process Management (Week 2)

Priority: Ensure proper cleanup and monitoring of OpenCode processes

1. **Update deacon.go**
   - Add "opencode" and "node" to cleanup
   - Update descriptions

2. **Update crash_report_check.go**
   - Add "opencode" to process list

3. **Update orphan_check.go**
   - Add "opencode" to process matching

4. **Update down.go**
   - Add "opencode" process detection

### Phase 3: Testing & Validation (Week 3)

1. Create integration tests for OpenCode paths
2. Test all hook commands with OpenCode
3. Test process cleanup with OpenCode
4. Test doctor checks with OpenCode
5. Verify feature parity with Claude

---

## Code Changes Required

### File: `internal/doctor/hooks_sync_check.go`

```go
// Add to DiscoverTargets or create DiscoverOpenCodeTargets
// Add comparison logic for gastown.js content
// Currently line 118-119 hardcodes .claude
```

### File: `internal/cmd/hooks_install.go`

```go
// Add flag: --provider string (default "claude")
// Add installation path for .opencode/plugins/gastown.js
// Currently line 194: settingsPath := filepath.Join(worktreePath, ".claude", "settings.json")
```

### File: `internal/cmd/hooks_scan.go`

```go
// Add scanning for .opencode/plugins/gastown.js
// Add parsing of JavaScript plugin hooks
// Currently line 67: // Find all .claude/settings.json files
```

### File: `internal/cmd/hooks_sync.go`

```go
// Add flag: --provider string (default "claude")
// Add sync for .opencode/plugins/gastown.js
// Currently line 18-19: description only mentions .claude/settings.json
```

### File: `internal/doctor/config_check.go`

```go
// Add check for .opencode/plugins/gastown.js
// Currently line 525: agentSettings := filepath.Join(townRoot, agent, ".claude", "settings.json")
```

### File: `internal/cmd/deacon.go`

```go
// Add "opencode" and "node" to process cleanup
// Currently line 421-423: if cmd != "claude" && cmd != "claude-code" && cmd != "codex"
```

### File: `internal/doctor/crash_report_check.go`

```go
// Add "opencode" to processes list
// Currently line 73-74: processes: []string{"tmux", "claude", "claude-code"}
```

### File: `internal/doctor/orphan_check.go`

```go
// Add "opencode" to process matching
// Currently line 421-423: filters for claude/codex only
```

### File: `internal/cmd/down.go`

```go
// Add "opencode" process detection
// Currently line 438: mentions only claude/node processes
```

---

## Testing Strategy

### Unit Tests

1. **Test OpenCode plugin installation**
   - Verify `gastown.js` is created
   - Verify `package.json` is created
   - Verify `npm install` runs successfully

2. **Test OpenCode command provisioning**
   - Verify commands are created in `.opencode/commands/`
   - Verify frontmatter is correct for OpenCode

3. **Test doctor checks with OpenCode**
   - Verify `opencode-commands-provisioned` check works
   - Verify `settings` check works for `.opencode`
   - Verify `priming` check works for OpenCode

### Integration Tests

1. **Test hook commands with OpenCode**
   - `gt hooks install --provider opencode`
   - `gt hooks scan --provider opencode`
   - `gt hooks sync --provider opencode`

2. **Test process cleanup with OpenCode**
   - Start opencode session
   - Run `gt deacon cleanup-orphans`
   - Verify opencode processes are cleaned up

3. **Test doctor with OpenCode**
   - Run `gt doctor` with OpenCode provisioned
   - Verify all checks pass
   - Run `gt doctor --fix` and verify fixes work

### End-to-End Tests

1. **Full OpenCode workflow**
   - Install gastown with OpenCode
   - Create rig with OpenCode agents
   - Run agents and verify they work
   - Test all doctor checks
   - Test process cleanup

---

## Success Criteria

OpenCode will have achieved feature parity with Claude when:

1. ✅ All hook commands work with `--provider opencode` flag
2. ✅ All doctor checks pass for OpenCode configurations
3. ✅ Process cleanup works for opencode processes
4. ✅ Settings synchronization works for both providers
5. ✅ All integration tests pass for OpenCode
6. ✅ Documentation is updated to reflect dual provider support

---

## Appendix: File Inventory

### Claude-Only Files (Need OpenCode Support)

| File | Lines | Priority | Issue |
|------|-------|----------|-------|
| `internal/doctor/hooks_sync_check.go` | ~200 | Critical | Only checks .claude |
| `internal/cmd/hooks_install.go` | ~250 | Critical | Only installs to .claude |
| `internal/cmd/hooks_scan.go` | ~150 | Critical | Only scans .claude |
| `internal/cmd/hooks_sync.go` | ~200 | Critical | Only syncs .claude |
| `internal/doctor/config_check.go` | ~550 | High | Only checks .claude |
| `internal/cmd/deacon.go` | ~1300 | High | Only cleans claude |
| `internal/doctor/crash_report_check.go` | ~100 | Medium | Only checks claude |
| `internal/doctor/orphan_check.go` | ~450 | Medium | Only matches claude |
| `internal/cmd/down.go` | ~500 | Medium | Only finds claude |

### Already Dual-Provider Files

| File | Status | Notes |
|------|--------|-------|
| `internal/opencode/plugin.go` | ✅ | Full plugin management |
| `internal/opencode/plugin/gastown.js` | ✅ | Complete plugin implementation |
| `internal/templates/commands/provision.go` | ✅ | Supports both providers |
| `internal/doctor/opencode_commands_check.go` | ✅ | OpenCode commands check |
| `internal/doctor/settings_check.go` | ✅ | Checks both providers |
| `internal/doctor/priming_check.go` | ✅ | Creates AGENTS.md for OpenCode |
| `internal/runtime/runtime.go` | ✅ | Provider-specific paths |
| `internal/daemon/lifecycle.go` | ✅ | OpenCode init sequence |
| `internal/cmd/sling_helpers.go` | ✅ | OpenCode nudge handling |
| `internal/cmd/mayor.go` | ✅ | OpenCode beacon handling |
| `internal/wrappers/wrappers.go` | ✅ | Both wrappers installed |
| `internal/git/git.go` | ✅ | Excludes both directories |

---

## Next Steps

1. **Start with Phase 1 (Critical)**
   - Focus on hook-related commands first
   - These are user-facing and most visible

2. **Add `--provider` flag consistently**
   - All hook commands should support `--provider` flag
   - Default to "claude" for backward compatibility

3. **Update doctor checks**
   - Ensure all checks work with both providers
   - Add specific OpenCode checks where needed

4. **Test thoroughly**
   - Create comprehensive test suite
   - Test with real OpenCode installations

5. **Update documentation**
   - Update all command help text
   - Update README and docs
   - Add OpenCode-specific examples

---

## Conclusion

OpenCode has substantial support in gastown already, but there are critical gaps in the hook management commands and process cleanup. The most important fixes are:

1. Extend hook commands to support `--provider opencode`
2. Update doctor checks to validate OpenCode configurations
3. Add opencode process cleanup to deacon and other commands

With these changes, OpenCode will achieve full feature parity with Claude in the gastown multi-agent system.
