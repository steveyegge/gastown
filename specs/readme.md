# Gas Town Specifications

## Active Specifications

| Document | Description | Status |
|----------|-------------|--------|
| [Multi-LLM Integration Plan](./multi-llm-integration-plan.md) | Merging PR #775 & #794 for provider-agnostic architecture | **Active** |
| ESP32S3 Specifications (below) | Hardware-specific testing requirements | Reference |

---

# Multi-LLM Integration Summary

This section summarizes the plan to merge PR #775 (Complete OpenCode Support) and PR #794 (Multi-LLM CLI Tool Support) into a unified, provider-agnostic architecture.

**See full plan**: [multi-llm-integration-plan.md](./multi-llm-integration-plan.md)

### Key Features
- **Provider Interface**: Pluggable system for arbitrary CLI tools
- **Per-Role Configuration**: Assign different agents/models to each role (cost optimization)
- **Built-in Presets**: Claude, Gemini, Codex, Cursor, Auggie, Amp, OpenCode, Kiro
- **Hooks Fallback**: Tmux-based fallback for tools without native hook support

### Priority E2E Testing
1. OpenCode
2. Kiro CLI

---

## Tasks

Pick the next `[ ]` task. Mark `[x]` when done, `[!]` if blocked.

### Phase 1: Foundation
- [x] 1.1 Merge `internal/config/agents.go` - combine PR 775's RoleAgents + PR 794's registry
- [x] 1.2 Create Provider interface in `internal/hooks/provider.go`
- [x] 1.3 Create provider implementations (claude.go, opencode.go, none.go)
- [x] 1.4 Update `internal/runtime/runtime.go` to use Provider interface

### Phase 2: Role Managers
- [x] 2.1 Update crew manager with `ResolveAgentConfig("crew")`
- [x] 2.2 Update deacon manager
- [ ] 2.3 Update autonomous managers (polecat, witness, refinery)
- [ ] 2.4 Update mayor and rig managers

### Phase 3: CLI Commands
- [ ] 3.1 Add `gt config agent <name>` command
- [ ] 3.2 Add `gt config role-agent <role> <agent>` command
- [ ] 3.3 Add `gt config add-agent` command
- [ ] 3.4 Add `gt config agents` listing command

### Phase 4: Kiro CLI
- [ ] 4.1 Research kiro-cli hooks system
- [ ] 4.2 Create kiro provider
- [ ] 4.3 Add kiro preset to agent registry
- [ ] 4.4 Create kiro plugin if needed

### Phase 5: Testing
- [ ] 5.1 Unit tests for Provider interface
- [ ] 5.2 Integration tests for agent switching
- [ ] 5.3 E2E tests for OpenCode
- [ ] 5.4 E2E tests for Kiro CLI

### Phase 6: Documentation
- [ ] 6.1 Update docs/custom-agents.md
- [ ] 6.2 Update doctor checks
- [ ] 6.3 Write migration guide
- [ ] 6.4 Clean up deprecated code

---

## Lessons Learned

_Agents: Add notes here after completing tasks._

- **Phase 1.1-1.3**: Most of the foundation was already implemented. The `RoleAgents` map, `ResolveRoleAgentConfig()`, and agent registry existed. Created `internal/hooks/` package with Provider interface and implementations wrapping existing `claude` and `opencode` packages.

- **Phase 1.4**: Runtime package now uses `hooks.Get(provider).EnsureHooks()` instead of switch statement. `StartupFallbackCommands` now uses `provider.SupportsHooks()` and `provider.GetHooksFallback()` for cleaner extensibility. Removed direct imports of `claude` and `opencode` packages from runtime.

- **Phase 2.1**: Updated crew manager to use `config.ResolveRoleAgentConfig("crew", townRoot, rigPath)` instead of hardcoded Claude. Replaced `claude.EnsureSettingsForRole()` with `runtime.EnsureSettingsForRole()` which uses the Provider interface. Replaced `t.IsClaudeRunning()` with `t.IsAgentRunning()` using `config.ExpectedPaneCommands()` for agent-agnostic process detection.

- **Phase 2.2**: Updated deacon manager (`internal/cmd/deacon.go`). Removed `internal/claude` import. Now uses `config.ResolveRoleAgentConfig("deacon", townRoot, "")` and `runtime.EnsureSettingsForRole()`. Deacon is a town-level role so it passes empty string for rigPath.

---

# ESP32S3 Gas Town Agent Specifications

## Overview

This document contains specifications and testing requirements for the Gas Town AI orchestrator when used with ESP32S3 devices. The Gas Town system provides automated AI agent workflows with support for multiple AI runtimes including Claude, OpenCode, Gemini, and others.

## OpenCode Agent Integration

### Status: ✅ IMPLEMENTED

The OpenCode CLI agent has been successfully integrated into Gas Town's built-in agent registry.

#### Configuration Details

- **Agent Name**: `opencode`
- **Command**: `opencode`
- **Default Args**: `[]` (empty, no default arguments)
- **Process Names**: `["opencode"]`
- **Session ID Environment Variable**: `OPENCODE_SESSION_ID`
- **Resume Flag**: `--resume`
- **Resume Style**: `flag`
- **Supports Hooks**: `true`
- **Supports Fork Session**: `false`

#### Usage Examples

```bash
# Use OpenCode as the default agent
gt town agent opencode

# Start a session with OpenCode
gt start --agent opencode

# Resume an OpenCode session
gt resume --session-id <SESSION_ID>
```

#### Session Management

OpenCode sessions are managed using the `OPENCODE_SESSION_ID` environment variable. Sessions can be resumed across restarts using the `--resume` flag.

#### Hook Integration

OpenCode supports the Gas Town hooks system for:
- Pre-session initialization
- Post-session cleanup
- Custom workflow integrations

## Testing Requirements

### Unit Tests
- ✅ Agent preset registration in registry
- ✅ Configuration validation
- ✅ Process name detection
- ✅ Session resumption command generation

### Integration Tests (ESP32S3 Device Required)
- [ ] OpenCode CLI installation and accessibility
- [ ] Session creation and persistence
- [ ] Hook execution with OpenCode
- [ ] Resume functionality across device restarts
- [ ] Process detection in tmux environment

### Hardware-Specific Testing Notes

#### ESP32S3 Considerations
1. **Memory Constraints**: Verify OpenCode CLI memory usage fits within ESP32S3 constraints
2. **Storage**: Ensure sufficient storage for CLI binary and session data
3. **Network Connectivity**: Test OpenCode's network dependencies on ESP32S3 networking stack
4. **Power Management**: Validate power consumption during long-running sessions

#### Test Environment Setup
```bash
# On ESP32S3 development environment
export OPENCODE_SESSION_ID=test_session_123
gt start --agent opencode
# Verify process is running
ps aux | grep opencode
# Test resume
gt resume --session-id test_session_123
```

## Future Work

### Known Issues
- No fork session support (OpenCode limitation)
- Requires manual CLI installation on target device

### Enhancement Roadmap
1. **Automatic CLI Installation**: Bundle OpenCode CLI with Gas Town distribution
2. **Resource Monitoring**: Add memory and CPU usage monitoring for OpenCode sessions
3. **Device-Specific Optimizations**: Optimize OpenCode configuration for embedded devices
4. **Offline Mode**: Investigate offline capabilities for network-constrained environments

## Testing Checklist

Before deploying to ESP32S3:

- [ ] Verify OpenCode CLI binary compatibility with target architecture
- [ ] Test memory footprint under typical workload
- [ ] Validate session persistence across device reboots
- [ ] Test hook integration with device-specific workflows
- [ ] Confirm network connectivity requirements are met
- [ ] Performance benchmarking vs other agents (Claude, Gemini)

## Documentation Updates

Each successful implementation should update this README with:
1. Performance characteristics on ESP32S3
2. Memory usage statistics
3. Network requirement details
4. Any device-specific configuration adjustments
5. Lessons learned and optimization opportunities