# Gas Town Specifications

## Active Specifications

| Document | Description | Status |
|----------|-------------|--------|
| [Multi-LLM Integration Plan](./multi-llm-integration-plan.md) | Merging PR #775 & #794 for provider-agnostic architecture | **Active** |

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
- [x] 2.3 Update autonomous managers (polecat, witness, refinery)
- [x] 2.4 Update mayor and rig managers

### Phase 3: CLI Commands
- [x] 3.1 Add `gt config agent <name>` command
- [x] 3.2 Add `gt config role-agent <role> <agent>` command
- [x] 3.3 Add `gt config add-agent` command
- [x] 3.4 Add `gt config agents` listing command

### Phase 4: Kiro CLI
- [ ] 4.1 Research kiro-cli hooks system
- [ ] 4.2 Create kiro provider
- [ ] 4.3 Add kiro preset to agent registry
- [ ] 4.4 Create kiro plugin if needed

### Phase 5: Testing
- [ ] 5.1 Unit tests for Provider interface
- [ ] 5.2 Integration tests for agent switching
- [x] 5.3 E2E tests for OpenCode
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

- **Phase 2.3**: Updated witness and refinery managers. Polecat manager was already updated. For witness (`internal/witness/manager.go`): removed `internal/claude` import, added `runtime` import, replaced `claude.EnsureSettingsForRole()` with `runtime.EnsureSettingsForRole()` using runtimeConfig. For refinery (`internal/refinery/manager.go`): replaced `t.IsClaudeRunning(sessionID)` calls with agent-agnostic detection using `config.ResolveRoleAgentConfig()`, `config.ExpectedPaneCommands()`, and `t.IsRuntimeRunning()`. Moved `townRoot` computation earlier in `Start()` function to share it across both agent detection checks and startup command building.

- **Phase 2.4**: Updated mayor manager (`internal/mayor/manager.go`): removed `internal/claude` import, added `runtime` import, replaced `claude.EnsureSettingsForRole()` with `runtime.EnsureSettingsForRole()` using runtimeConfig. Agent detection was already updated. The rig.go CLI file delegates to already-updated managers, no changes needed.

- **Phase 3.2**: Added `gt config role-agent <role> <agent>` command to `internal/cmd/config.go`. Supports viewing and setting per-role agents with `<agent>:<model>` syntax (e.g., `claude:haiku`). When model syntax is used, automatically creates a custom agent entry with the model flag. Valid roles: mayor, deacon, witness, refinery, polecat, crew. Updates RoleAgents map in TownSettings.

