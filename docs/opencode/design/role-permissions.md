# Role-Based Permissions Strategy

> **Status**: Planning  
> **Created**: 2026-01-19  
> **Purpose**: Define permission profiles for Gastown roles across agent runtimes

---

## Problem Statement

Gastown has 6 roles with different permission needs:
- **Interactive roles** (Mayor, Crew): May want confirmation before dangerous operations
- **Autonomous roles** (Polecat, Witness, Deacon, Refinery): Need unattended execution

Each runtime handles permissions differently. We need a unified strategy that:
1. Sets appropriate permissions per role
2. Works consistently across runtimes
3. Can be managed centrally (not per-agent hardcoding)

---

## Current State

### Runtime Permission Mechanisms

| Runtime | Full Auto Flag | Granular Control | Custom Agents | Config Location |
|---------|----------------|------------------|---------------|-----------------|
| **Claude Code** | `--dangerously-skip-permissions` | Via `settings.json` | ✅ `--agent` | `.claude/settings.json` |
| **OpenCode** | `OPENCODE_PERMISSION` env | JSON policy | ✅ agents in config | `.opencode/config.jsonc` |
| **Codex** | `--full-auto` | Sandbox modes | ❌ | CLI flags only |
| **Gemini** | `--yolo` | Policy files | ✅ extensions | `.gemini/settings.json` |

### Claude Code: How Gastown Controls Permissions

Gastown manages Claude Code permissions via **template-based settings** initialized during `gt init`:

```
internal/claude/
├── settings.go           # Ensures .claude/settings.json exists
└── config/
    ├── settings-autonomous.json  # Polecat, Witness, Deacon, Refinery
    └── settings-interactive.json # Mayor, Crew
```

**Flow**:
1. `gt init` or role startup calls `claude.EnsureSettingsForRole(workDir, role)`
2. Based on role type, copies appropriate template to `.claude/settings.json`
3. Claude Code reads `.claude/settings.json` and applies permissions
4. User can customize after initialization

**Key insight**: Gastown doesn't rely on sandbox - it uses Claude Code's **native permission system**.

### OpenCode: Current Gap

Currently, OpenCode permissions require manual shell profile setup:
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
```

This applies globally, not per-role, and isn't initialized by `gt init`.

---

## Proposed Strategy

### 1. Permission Profiles by Role Type

| Role Type | Roles | Permission Level | Rationale |
|-----------|-------|------------------|-----------|
| **Autonomous** | Polecat, Witness, Deacon, Refinery | Full auto | Must execute without user input |
| **Interactive** | Mayor, Crew | Conditional | May want confirmation for destructive ops |

### 2. Per-Runtime Implementation

#### Claude Code

Already handled via settings templates:
```go
// internal/claude/settings.go
func EnsureSettings(workDir string, roleType RoleType) error {
    // Autonomous roles get dangerously-skip-permissions
    // Interactive roles get normal settings
}
```

#### OpenCode

**Option A: Environment Variable** (current)
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
```
- ✅ Simple
- ❌ Applies globally, not per-role
- ❌ Not initialized by `gt init`

**Option D: Template-Based Config** (RECOMMENDED - mirrors Claude Code)

Create `internal/opencode/` with embedded config templates:

```
internal/opencode/
├── settings.go           # Ensures .opencode/config.jsonc exists
├── plugin/
│   └── gastown.js        # Gastown plugin (already exists)
└── config/
    ├── config-autonomous.jsonc  # Polecat, Witness, Deacon, Refinery
    └── config-interactive.jsonc # Mayor, Crew
```

**Autonomous template** (`config-autonomous.jsonc`):
```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "*": "allow"  // Full auto for autonomous roles
  }
}
```

**Interactive template** (`config-interactive.jsonc`):
```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "read": "allow",
    "list": "allow",
    "glob": "allow",
    "grep": "allow",
    "edit": "ask",      // Confirm file modifications
    "bash": "ask",      // Confirm shell commands
    "webfetch": "ask"
  }
}
```

**Implementation**:
```go
// internal/opencode/settings.go
func EnsureSettings(workDir string, roleType RoleType) error {
    return EnsureSettingsAt(workDir, roleType, ".opencode", "config.jsonc")
}
```

**Benefits**:
- ✅ Mirrors Claude Code pattern exactly
- ✅ Uses OpenCode's native permission system (not sandbox)
- ✅ Initialized by `gt init` automatically
- ✅ Per-role permissions out of the box
- ✅ Users can customize after initialization

**Option B: Custom Agents in Config** (proposed)
```jsonc
// .opencode/config.jsonc
{
  "agents": {
    "polecat": {
      "permissions": {"*": "allow"},
      "model": "anthropic/claude-3-5-sonnet"
    },
    "crew": {
      "permissions": {
        "file.read": "allow",
        "file.write": "ask",
        "shell.exec": "ask"
      }
    }
  }
}
```
- ✅ Per-role permissions
- ✅ Centralized config
- ❌ Requires OpenCode agent selection at spawn time

**Option C: Plugin-Based Permissions** (advanced)
Plugin could dynamically adjust permissions based on `GT_ROLE` environment variable:
```javascript
export const GastownPlugin = async () => ({
  "permission.ask": async (input, output) => {
    const role = process.env.GT_ROLE;
    if (['polecat', 'witness', 'deacon', 'refinery'].includes(role)) {
      output.allow = true;  // Auto-approve for autonomous roles
    }
  }
})
```
- ✅ Dynamic, no config changes needed
- ✅ Uses existing GT_ROLE env var
- ⚠️ Experimental hook

#### Codex

Limited options - only CLI flags:
```bash
codex --full-auto  # For autonomous roles
codex              # For interactive roles (default)
```
Managed via `internal/config/agents.go` preset args.

#### Gemini

Similar to Claude Code - settings-based:
```json
// .gemini/settings.json (per-role)
{
  "auto_approve": ["file_read", "file_write", "shell_exec"]
}
```

### 3. Recommended Implementation

**Phase 1: Environment-Based** (Current)
- Continue using `OPENCODE_PERMISSION` for OpenCode
- Document in setup scripts
- Works for all roles uniformly

**Phase 2: Plugin-Based Permissions**
- Enhance `gastown.js` plugin with `permission.ask` hook
- Read `GT_ROLE` to determine permission level
- No external config needed

**Phase 3: Custom Agents**
- Define role-specific agents in OpenCode config
- Map Gastown roles to OpenCode agents
- `gt sling` would pass `--agent polecat` to select the right profile

---

## Cross-Runtime Abstraction

### Possible `runtime.EnsurePermissions` Interface

```go
// internal/runtime/permissions.go
type PermissionProfile struct {
    FileRead   PermissionLevel  // allow, ask, deny
    FileWrite  PermissionLevel
    ShellExec  PermissionLevel
    Network    PermissionLevel
}

func EnsurePermissions(cfg *RuntimeConfig, profile PermissionProfile) error {
    switch cfg.Provider {
    case "claude":
        return claude.EnsurePermissions(cfg, profile)
    case "opencode":
        return opencode.EnsurePermissions(cfg, profile)
    // ... etc
    }
}
```

### Role → Profile Mapping

```go
var RolePermissions = map[string]PermissionProfile{
    "polecat":  {FileRead: Allow, FileWrite: Allow, ShellExec: Allow, Network: Allow},
    "witness":  {FileRead: Allow, FileWrite: Deny,  ShellExec: Ask,   Network: Deny},
    "mayor":    {FileRead: Allow, FileWrite: Ask,   ShellExec: Ask,   Network: Ask},
    "crew":     {FileRead: Allow, FileWrite: Ask,   ShellExec: Ask,   Network: Ask},
    // ...
}
```

---

## Open Questions

1. **Should interactive roles (Mayor, Crew) have different permissions per-runtime?**
   - Claude Code: May want confirmation for writes
   - OpenCode: Same or different?

2. **How granular should permissions be?**
   - Simple: all-or-nothing per role
   - Complex: file.read, file.write, shell.exec, network separately

3. **Where should permission config live?**
   - Per-rig settings?
   - Town-level settings?
   - Environment variables?

4. **Should we create OpenCode custom agents matching Gastown roles?**
   - Would enable `opencode --agent polecat` for role-specific behavior
   - Requires maintaining OpenCode config alongside Gastown

---

## Next Steps

1. [ ] Decide on Phase 2 plugin-based approach
2. [ ] Test `permission.ask` hook in OpenCode
3. [ ] Document permission requirements per role
4. [ ] Consider `runtime.EnsurePermissions` abstraction

---

## Related Documents

- [../reference/configuration.md](../reference/configuration.md) - OpenCode config
- [../../agent-features.md](../../agent-features.md) - Runtime comparison
- [../../reference.md](../../reference.md) - Gastown role settings
