# Gas Town Hooks Management

Centralized Claude Code hook management for Gas Town workspaces.

## Overview

Gas Town manages `.claude/settings.json` files across all agent locations
(mayor, deacon, crew, witness, refinery, polecats). The hooks system provides
a single source of truth with a base config and per-role/per-rig overrides.

## Architecture

```
~/.gt/hooks-base.json              ← Shared base config (all agents)
~/.gt/hooks-overrides/
  ├── crew.json                    ← Override for all crew workers
  ├── witness.json                 ← Override for all witnesses
  ├── gastown__crew.json           ← Override for gastown crew specifically
  └── ...
```

**Merge strategy:** `base → role → rig+role` (more specific wins)

For a target like `gastown/crew`:
1. Start with base config
2. Apply `crew` override (if exists)
3. Apply `gastown/crew` override (if exists)

## Generated targets

Each rig generates settings for these locations:

| Target | Path | Override Key |
|--------|------|--------------|
| Rig root | `<rig>/.claude/settings.json` | `<rig>/rig` |
| Crew (shared) | `<rig>/crew/.claude/settings.json` | `<rig>/crew` |
| Witness | `<rig>/witness/.claude/settings.json` | `<rig>/witness` |
| Refinery | `<rig>/refinery/.claude/settings.json` | `<rig>/refinery` |
| Polecats (shared) | `<rig>/polecats/.claude/settings.json` | `<rig>/polecats` |

Town-level targets:
- `mayor/.claude/settings.json` (key: `mayor`)
- `deacon/.claude/settings.json` (key: `deacon`)

## Commands

### `gt hooks sync`

Regenerate all `.claude/settings.json` files from base + overrides.
Preserves non-hooks fields (editorMode, enabledPlugins, etc.).

```bash
gt hooks sync             # Write all settings files
gt hooks sync --dry-run   # Preview changes without writing
```

### `gt hooks diff`

Show what `sync` would change, without writing anything.

```bash
gt hooks diff             # Show differences
gt hooks diff --no-color  # Plain output
```

### `gt hooks base`

Edit the shared base config in `$EDITOR`.

```bash
gt hooks base             # Open in editor
gt hooks base --show      # Print current base config
```

### `gt hooks override <target>`

Edit overrides for a specific role or rig+role.

```bash
gt hooks override crew              # Edit crew override
gt hooks override gastown/witness   # Edit gastown witness override
gt hooks override crew --show       # Print current override
```

### `gt hooks list`

Show all managed settings.json locations and their sync status.

```bash
gt hooks list             # Show all targets
gt hooks list --json      # Machine-readable output
```

### `gt hooks scan`

Scan the workspace for existing hooks (reads current settings files).

```bash
gt hooks scan             # List all hooks
gt hooks scan --verbose   # Show hook commands
gt hooks scan --json      # JSON output
```

### `gt hooks init`

Bootstrap base config from existing settings.json files. Analyzes all
current settings, extracts common hooks as the base, and creates overrides
for per-target differences.

```bash
gt hooks init             # Bootstrap base and overrides
gt hooks init --dry-run   # Preview what would be created
```

Only works when no base config exists yet. Use `gt hooks base` to edit
an existing base config.

### `gt hooks registry` / `gt hooks install`

Browse and install hooks from the registry.

```bash
gt hooks registry                  # List available hooks
gt hooks install <hook-id>         # Install a hook to base config
```

## Integration

### `gt rig add`

When a new rig is created, hooks are automatically synced for all the
new rig's targets (crew, witness, refinery, polecats).

### `gt doctor`

The `hooks-sync` check verifies all settings.json files match what
`gt hooks sync` would generate. Use `gt doctor --fix` to auto-fix
out-of-sync targets.

## Per-matcher merge semantics

When an override has the same matcher as a base entry, the override
**replaces** the base entry entirely. Different matchers are appended.
An override entry with an empty hooks list **removes** that matcher.

Example base:
```json
{
  "SessionStart": [
    { "matcher": "", "hooks": [{ "type": "command", "command": "gt prime" }] }
  ]
}
```

Override for witness:
```json
{
  "SessionStart": [
    { "matcher": "", "hooks": [{ "type": "command", "command": "gt prime --witness" }] }
  ]
}
```

Result: The witness gets `gt prime --witness` instead of `gt prime`
(same matcher = replace).

## Default base config

When no base config exists, the system uses sensible defaults:

- **SessionStart**: PATH setup + `gt prime --hook`
- **PreCompact**: PATH setup + `gt prime --hook`
- **UserPromptSubmit**: PATH setup + `gt mail check --inject`
- **Stop**: PATH setup + `gt costs record`
