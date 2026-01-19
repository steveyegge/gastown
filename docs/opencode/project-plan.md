---
# OpenCode Integration Project Plan
# This document tracks the phased approach to OpenCode integration.

last_updated: 2026-01-19
status: phase_1_in_progress
---

# OpenCode Integration: Project Plan

This document outlines our phased approach to integrating OpenCode with Gastown, matching Claude Code parity first, then expanding capabilities.

---

## Goals

1. **Parity First**: Match Claude Code integration before adding new capabilities
2. **Deep Testing**: Verify OpenCode actually did the expected work, not just end results
3. **Reduce Duplication**: Consider compatibility layer over maintaining parallel implementations
4. **Extensibility**: Design for future OpenCode-specific enhancements

---

## Phase 1: Claude Code Parity (Current)

**Objective**: OpenCode works identically to Claude Code for all standard Gastown workflows.

### 1.1 Core Integration ✅

- [x] Agent preset with correct ProcessNames, Args, Hooks
- [x] RuntimeConfigFromPreset correctly preserves empty slices
- [x] normalizeRuntimeConfig sets Hooks based on Provider
- [x] GT_ROOT used for reliable config resolution
- [x] Plugin installation via EnsureSettingsForRole

### 1.2 Deep E2E Testing 🔄

Tests must verify OpenCode **actually did the work**, not just check end state:

| Test | What to Verify | How to Verify |
|------|----------------|---------------|
| Mayor startup | OpenCode launched | Pane command is `node` or `opencode` |
| Plugin installed | gastown.js exists | File existence check |
| `gt prime` executed | Context was injected | Check session has context OR log output |
| Mail check works | Mail was checked | Log output or beads state |
| Session persists | OpenCode didn't crash | Session still running after N seconds |
| Hooks fire | Events triggered gt commands | Log parsing or side-effect verification |

**Test Requirements**:
- All assertions must have explicit logging explaining what's being checked
- Failures must indicate which specific step failed
- Tests should capture tmux pane content for debugging
- Use structured logging where possible

**Current Test Status**:
- [x] `TestOpenCodeMayorWorkflow` - Basic startup verification
- [ ] Deep verification of plugin hook execution
- [ ] Mail injection verification
- [ ] Multi-role workflow test

### 1.3 Known Blockers

| Issue | Status | Notes |
|-------|--------|-------|
| OpenCode tmux crash | Upstream bug | `proper-lockfile/onExit` issue in OpenCode |

---

## Phase 2: Advanced Testing

**Objective**: Comprehensive E2E coverage matching user expectations.

### 2.1 User Workflow Tests

Each test simulates a real user workflow with OpenCode as default:

| Workflow | Test Coverage |
|----------|---------------|
| `gt install` → `gt mayor start` | Town creation, Mayor startup |
| `gt rig add` → `gt sling` | Rig setup, Polecat spawn |
| `gt witness start` | Build watching |
| `gt mail send` → mail delivered | Cross-session messaging |
| Session resume | `--session` flag works |
| Session fork | HTTP API fork works |

### 2.2 Verification Requirements

For each workflow, tests must verify:

1. **Command executed**: OpenCode received the expected command
2. **Hooks fired**: Plugin events triggered (check logs or side-effects)
3. **State changed**: Beads/files/mail updated correctly
4. **No errors**: No crashes or unexpected stderr output
5. **Timing**: Operations complete within expected bounds

---

## Phase 3: Compatibility Layer (Future Opportunity)

### Current Approach: Duplicate Implementations

```
Claude Code:  .claude/settings.json  → hooks → shell commands
OpenCode:     .opencode/plugin/gastown.js → events → shell commands
```

Both ultimately run the same `gt` commands. We currently use:
- `internal/claude/` for Claude hook settings
- `internal/opencode/` for OpenCode plugin JS

### Opportunity: Shared Hook Logic

Instead of maintaining two hook implementations:

**Option A: Claude-Compatible Plugin for OpenCode**

Create an OpenCode plugin that reads Claude's hooks configuration:

```javascript
// Read Claude hooks and translate to OpenCode events
const claudeSettings = JSON.parse(fs.readFileSync('.claude/settings.json'))
// Map SessionStart → session.created, etc.
```

**Pros**:
- Single source of truth for hook behavior
- Changes to Claude hooks automatically apply to OpenCode
- Reduces maintenance

**Cons**:
- Adds dependency between implementations
- Claude hook format may not map cleanly to all OpenCode events
- May limit OpenCode-specific optimizations

**Option B: Shared Command Generator (Current Approach)**

```go
// internal/runtime/commands.go
func StartupCommands(role string) []string {
    return []string{"gt prime", "gt mail check --inject", ...}
}
```

Both Claude hooks and OpenCode plugin call the same Go function to get commands.

**Pros**:
- Commands are defined once in Go
- Each runtime adapts to its own event model
- Independent optimization possible

**Cons**:
- Still maintaining two hook/plugin implementations
- Logic split between Go and JS/JSON

### Recommendation

Start with **Option B** (current shared command generator approach) for Phase 1.

Consider **Option A** (compatibility layer) for Phase 3 if:
- Claude hooks become more complex
- Maintaining parity becomes burdensome
- OpenCode plugin complexity grows

---

## Phase 4: OpenCode-Specific Enhancements (Future)

After parity is achieved, consider OpenCode-specific features:

| Enhancement | Benefit | Complexity |
|-------------|---------|------------|
| SDK integration | Programmatic control, no process spawn | High |
| HTTP API for sessions | Remote orchestration | Medium |
| Session export/import | State backup/restore | Low |
| Multi-model routing | Use different models per task | Medium |
| Plugin hot-reload | Faster development | Low |

See [stage2-sdk-orchestration.md](stage2-sdk-orchestration.md) for SDK plans.

---

## Testing Philosophy

### Don't Trust End State Alone

❌ **Bad**: "File exists, so OpenCode must have created it"
✅ **Good**: "Verified OpenCode pane command was 'node', checked plugin was installed, saw 'gt prime' in pane output"

### Verify Each Step

```go
// Example: Deep verification in test
func TestMayorWorkflow(t *testing.T) {
    // 1. Start Mayor
    err := manager.Start("opencode")
    require.NoError(t, err, "Mayor should start without error")
    
    // 2. Verify OpenCode is running (not Claude)
    paneCmd := t.GetPaneCommand(sessionID)
    assert.Contains(t, []string{"node", "opencode"}, paneCmd, 
        "Expected OpenCode process, got: %s", paneCmd)
    t.Logf("✓ Verified OpenCode running (pane command: %s)", paneCmd)
    
    // 3. Verify plugin was installed
    pluginPath := filepath.Join(mayorDir, ".opencode/plugin/gastown.js")
    require.FileExists(t, pluginPath, "Plugin should be installed")
    t.Logf("✓ Plugin installed at: %s", pluginPath)
    
    // 4. Verify hooks are firing (check for gt prime output)
    paneContent := t.CapturePaneContent(sessionID)
    assert.Contains(t, paneContent, "gt prime", 
        "Expected 'gt prime' in pane output")
    t.Logf("✓ Verified 'gt prime' executed")
    
    // 5. Verify session persists
    time.Sleep(3 * time.Second)
    running := t.IsSessionRunning(sessionID)
    assert.True(t, running, "Session should still be running")
    t.Logf("✓ Session persisted after checks")
}
```

---

## Agent Navigation

When working on OpenCode integration in Gastown, follow these links:

### Quick Reference
| Need | Document |
|------|----------|
| External sources & topic reference | [opencode-reference.md](opencode-reference.md) |
| Current integration status | [integration-summary.md](integration-summary.md) |
| This project plan | You're reading it |

### Implementation
| Task | Document |
|------|----------|
| How plugins work | [plugin-guide.md](plugin-guide.md) |
| Plugin source | `internal/opencode/plugin/gastown.js` |
| Agent preset | `internal/config/agents.go` → `AgentOpencode` |
| Runtime config | `internal/config/loader.go` → `BuildStartupCommandWithAgentOverride` |

### Testing
| Task | Location |
|------|----------|
| Integration test | `internal/opencode/integration_test.go` |
| E2E scripts | `scripts/test-opencode-*.sh` |

### External
| Need | Link |
|------|------|
| OpenCode source | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) |
| Plugin events | [OpenCode Plugins](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) |
| Community solutions | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Change Log

| Date | Change |
|------|--------|
| 2026-01-19 | Created project plan, documented phases 1-4 |
| 2026-01-19 | Fixed 5 core Gastown bugs blocking OpenCode integration |
| 2026-01-17 | Initial OpenCode integration and documentation |
