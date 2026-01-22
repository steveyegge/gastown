# Multi-LLM CLI Tool Integration Plan

## Overview

This document outlines the implementation plan for merging PR #794 (Multi-LLM CLI Tool Support) and PR #775 (Complete OpenCode Support) into a unified, provider-agnostic architecture that enables Gas Town to work with arbitrary LLM CLI tools.

**Goal**: Create a flexible system where users can configure different CLI tools and models on a per-role basis, optimizing for cost, performance, or specific use cases without changing the core user experience.

---

## PR Analysis

### PR #794 (cwolfe007) - Multi-LLM CLI Tool Support
**Status**: Draft | **Changes**: +1,200 lines

Key contributions:
- `Provider` interface in `internal/hooks/provider.go` for pluggable hook systems
- Registry-based provider selection (replaces switch statements)
- 7 built-in agent presets: Claude, Gemini, Codex, Cursor, Auggie, Amp, OpenCode
- 508-line integration test suite (`agents_switching_integration_test.go`)
- Documentation: `docs/custom-agents.md`

### PR #775 (arttttt) - Complete OpenCode Support
**Status**: Open | **Changes**: +1,901 −559 lines

Key contributions:
- Provider-agnostic refactor removing hardcoded Claude references
- Per-role agent configuration via `role_agents` in `town_settings.json`
- `ResolveAgentConfig()` for role-specific configuration resolution
- OpenCode plugin (`internal/opencode/plugin/gastown.js`)
- Doctor checks for multi-provider validation
- Updates to ALL role managers (crew, deacon, mayor, polecat, refinery, rig, witness)

### Conflicts & Overlaps
| File | PR 775 | PR 794 |
|------|--------|--------|
| `internal/config/agents.go` | `RoleAgents` map, loader changes | Provider interface, registry |
| `internal/runtime/runtime.go` | `ResolveAgentConfig()` | Provider-based `EnsureSettingsForRole()` |
| `internal/opencode/plugin.go` | Full plugin implementation | Plugin stub |
| `internal/cmd/*.go` | Role-specific config resolution | Minimal changes |

---

## Merged Architecture Design

### Core Principles

1. **Provider Interface**: All CLI tools implement a common `Provider` interface
2. **Per-Role Configuration**: Different agents/models assignable to each role
3. **Registry Pattern**: Providers registered at init, resolved by name
4. **Hooks Fallback**: Tools without native hooks use tmux-based fallback
5. **Configuration Flexibility**: Both file-based (JSON) and CLI-based configuration

### Provider Interface

```go
// internal/hooks/provider.go
type Provider interface {
    // Name returns the provider identifier (e.g., "claude", "opencode", "kiro")
    Name() string

    // EnsureHooks installs/configures hooks for this provider
    EnsureHooks(workDir, role string, config *RuntimeHooksConfig) error

    // SupportsHooks returns true if provider has native hook support
    SupportsHooks() bool

    // GetHooksFallback returns tmux commands to emulate hooks (if !SupportsHooks)
    GetHooksFallback(role string) []string
}

// Registry for provider lookup
var providers = make(map[string]Provider)

func Register(p Provider) {
    providers[p.Name()] = p
}

func Get(name string) Provider {
    return providers[name]
}
```

### Configuration Schema

```json
// town_settings.json
{
  "default_agent": "claude",
  "role_agents": {
    "mayor": { "agent": "claude", "model": "opus" },
    "crew": { "agent": "claude", "model": "sonnet" },
    "deacon": { "agent": "opencode", "model": "glm-4" },
    "polecat": { "agent": "kiro", "model": "default" },
    "witness": { "agent": "claude", "model": "haiku" },
    "refinery": { "agent": "claude", "model": "haiku" }
  },
  "agents": {
    "kiro": {
      "command": "kiro-cli",
      "args": ["--autonomous"],
      "process_names": ["kiro-cli"],
      "session_id_env": "KIRO_SESSION_ID",
      "resume_flag": "--resume",
      "resume_style": "flag",
      "supports_hooks": true,
      "hooks": {
        "provider": "kiro",
        "dir": ".kiro",
        "settings_file": "settings.json"
      }
    }
  }
}
```

### CLI Configuration

```bash
# Set default agent
gt config agent claude

# Set per-role agent
gt config role-agent deacon opencode
gt config role-agent witness claude:haiku

# Add custom agent
gt config add-agent kiro --command kiro-cli --hooks-provider kiro

# List configured agents
gt config agents
```

---

## Implementation Tasks

### Phase 1: Foundation (Merge Core Architecture)
> Combine PR 775's provider-agnostic refactor with PR 794's Provider interface

- [ ] **1.1** Merge `internal/config/agents.go` changes from both PRs
  - Keep PR 794's `AgentPresetInfo` and `AgentRegistry`
  - Add PR 775's `RoleAgents` map to `TownSettings`
  - Implement `ResolveAgentConfig(role string)` that checks role_agents first

- [ ] **1.2** Implement Provider interface (`internal/hooks/provider.go`)
  - Define interface with `Name()`, `EnsureHooks()`, `SupportsHooks()`, `GetHooksFallback()`
  - Create provider registry with `Register()` and `Get()` functions
  - Add auto-registration via `init()` functions

- [ ] **1.3** Create provider implementations
  - `internal/hooks/claude.go` - Claude provider (uses existing claude package)
  - `internal/hooks/opencode.go` - OpenCode provider (uses PR 775's plugin)
  - `internal/hooks/none.go` - No-op provider for tools without hooks
  - `internal/hooks/fallback.go` - Tmux-based fallback for hookless tools

- [ ] **1.4** Update `internal/runtime/runtime.go`
  - Replace switch statement with `provider.Get(name).EnsureHooks()`
  - Add fallback command injection for providers without hook support

### Phase 2: Role Manager Updates
> Propagate per-role configuration to all managers

- [ ] **2.1** Update crew manager (`internal/crew/manager.go`)
  - Use `ResolveAgentConfig("crew")` instead of global config
  - Pass role context to `EnsureSettingsForRole()`

- [ ] **2.2** Update deacon manager (`internal/deacon/manager.go`)
  - Resolve agent config per-role
  - Support different models for deacon vs other roles

- [ ] **2.3** Update autonomous role managers
  - `internal/polecat/session_manager.go`
  - `internal/witness/manager.go`
  - `internal/refinery/manager.go`

- [ ] **2.4** Update mayor and rig managers
  - `internal/mayor/manager.go`
  - `internal/rig/manager.go`

### Phase 3: CLI Configuration Commands
> Implement user-facing configuration options

- [ ] **3.1** Add `gt config agent <name>` command
  - Set default agent in town_settings.json
  - Validate agent exists in registry

- [ ] **3.2** Add `gt config role-agent <role> <agent>` command
  - Set per-role agent configuration
  - Support `<agent>:<model>` syntax (e.g., `claude:haiku`)

- [x] **3.3** Add `gt config add-agent` command
  - Register custom agents via CLI
  - Generate template configuration in settings/agents.json
  - *Completed: Implemented with comprehensive flags for command, args, hooks, session management, and process detection*

- [x] **3.4** Add `gt config agents` listing command
  - Show all registered agents (built-in + custom)
  - Display current role assignments
  - *Completed: Shows agents with text/JSON output, includes role assignments with default indicators*

### Phase 4: Kiro CLI Integration
> Add support for kiro-cli as a third provider

- [ ] **4.1** Research kiro-cli hooks system
  - Document hooks directory structure
  - Identify settings file format
  - Map to Provider interface requirements

- [ ] **4.2** Create kiro provider (`internal/hooks/kiro.go`)
  - Implement `EnsureHooks()` for kiro-cli
  - Add process detection names

- [ ] **4.3** Add kiro preset to agent registry
  - Define default args for autonomous mode
  - Configure session management

- [ ] **4.4** Create kiro plugin if needed
  - Similar to `internal/opencode/plugin/gastown.js`
  - Handle Gas Town context injection

### Phase 5: Testing & Validation
> Ensure quality and compatibility

- [ ] **5.1** Unit tests for Provider interface
  - Test provider registration
  - Test config resolution with role_agents
  - Test fallback command generation

- [ ] **5.2** Integration tests for agent switching
  - Merge PR 794's test suite
  - Add tests for per-role configuration
  - Test Claude -> OpenCode -> Kiro switching

- [x] **5.3** E2E tests for OpenCode
  - Session creation and resume
  - Hook execution verification
  - Process detection in tmux

- [ ] **5.4** E2E tests for Kiro CLI
  - Similar coverage to OpenCode
  - Validate autonomous operation

### Phase 6: Documentation & Cleanup
> Polish and document the new system

- [ ] **6.1** Update `docs/custom-agents.md`
  - Add per-role configuration examples
  - Document Provider interface for contributors
  - Include kiro-cli setup guide

- [ ] **6.2** Update doctor checks
  - Validate settings exist for configured agents
  - Check CLI availability for all role_agents

- [ ] **6.3** Migration guide
  - Document upgrade path from single-agent setup
  - Provide example configurations for common scenarios

- [ ] **6.4** Clean up deprecated code
  - Remove hardcoded Claude references
  - Deprecate `IsClaudeRunning` in favor of `IsAgentRunning`

---

## File Change Summary

### New Files
| Path | Purpose |
|------|---------|
| `internal/hooks/provider.go` | Provider interface definition |
| `internal/hooks/claude.go` | Claude provider implementation |
| `internal/hooks/opencode.go` | OpenCode provider implementation |
| `internal/hooks/kiro.go` | Kiro CLI provider implementation |
| `internal/hooks/none.go` | No-op provider |
| `internal/hooks/fallback.go` | Tmux-based fallback |
| `internal/kiro/plugin/gastown.js` | Kiro plugin (if needed) |

### Modified Files
| Path | Changes |
|------|---------|
| `internal/config/agents.go` | Add `RoleAgents`, merge registry |
| `internal/config/types.go` | Add `RoleAgents` to TownSettings |
| `internal/config/loader.go` | Add `ResolveAgentConfig()` |
| `internal/runtime/runtime.go` | Use Provider interface |
| `internal/cmd/config.go` | Add agent/role-agent commands |
| `internal/crew/manager.go` | Per-role config resolution |
| `internal/deacon/manager.go` | Per-role config resolution |
| `internal/polecat/session_manager.go` | Per-role config resolution |
| `internal/witness/manager.go` | Per-role config resolution |
| `internal/refinery/manager.go` | Per-role config resolution |
| `internal/mayor/manager.go` | Per-role config resolution |

---

## Success Criteria

1. **Backwards Compatible**: Existing Claude-only setups work without configuration changes
2. **Per-Role Flexibility**: Users can assign different agents to different roles
3. **Extensible**: Adding new providers requires only implementing the Provider interface
4. **Tested**: E2E tests pass for Claude, OpenCode, and Kiro CLI
5. **Documented**: Clear guides for custom agent configuration

---

## Timeline Estimate

| Phase | Dependencies | Complexity |
|-------|--------------|------------|
| Phase 1 | None | High (merge conflicts) |
| Phase 2 | Phase 1 | Medium |
| Phase 3 | Phase 1 | Low |
| Phase 4 | Phase 1 | Medium |
| Phase 5 | Phase 1-4 | Medium |
| Phase 6 | Phase 1-5 | Low |

---

## Open Questions

1. **Model selection**: Should model selection (opus/sonnet/haiku) be part of agent config or separate?
2. **Cost tracking**: How to track costs across different providers?
3. **Rate limiting**: Different providers have different rate limits - how to handle?
4. **Error messages**: Provider-specific vs generic error handling?
