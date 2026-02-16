# Hooks Registry and Management Design

> **Status**: Design Phase
> **Issue**: gt-gow8b
> **Author**: gastown/polecats/dag
> **Date**: 2026-02-16

## Vision

A comprehensive hooks registry and management system for Claude Code in Gas Town that provides:

1. **Discovery**: Browse and search available hooks
2. **Installation**: One-command install of verified hooks
3. **Management**: Enable/disable hooks per-worktree or per-role
4. **Ordering**: Control execution order when multiple hooks match
5. **Integration**: Seamless integration with both `gt` commands and shell scripts
6. **Consistency**: Single source of truth synchronized across all worktrees

This elevates hooks from "manually copy config" to "install from registry with one command."

## Current State

### What Exists

Gas Town has a centralized hooks management system (see [HOOKS.md](HOOKS.md)):

- **Base config**: `~/.gt/hooks-base.json` - shared across all agents
- **Overrides**: `~/.gt/hooks-overrides/{role}.json` - per-role customization
- **Merge strategy**: `base → role → rig+role` (more specific wins)
- **Commands**: `gt hooks sync`, `gt hooks diff`, `gt hooks base`, `gt hooks override`, etc.
- **Integration**: Settings passed via `--settings` flag to Claude Code

### What's Missing

1. **Hook Registry**: No central catalog of available hooks with metadata
2. **gt tap Integration**: `gt tap guard/audit` commands exist but not in registry
3. **Shell Script Hooks**: Joe's hooks not integrated (raw scripts vs `gt` commands)
4. **Hook Ordering**: No control over execution order when multiple hooks match same pattern
5. **Per-worktree Control**: Can't disable a hook for specific worktree (only role-level)
6. **Discovery Tools**: No `gt hooks search` or `gt hooks browse`

### Critical Constraint

**Claude Code does NOT traverse parent directories.** It only looks in CWD for `.claude/settings.json`.

This means:
- Each worktree needs its own `.claude/settings.json` OR
- Gas Town passes `--settings <path>` to load from shared parent directory

Current approach uses shared parent directories (`crew/.claude/settings.json`, `polecats/.claude/settings.json`) with `--settings` flag.

## Architecture

### Registry Structure

```yaml
# ~/.gt/hooks-registry.yaml
version: 1

hooks:
  # Guard hooks (PreToolUse - block dangerous operations)
  guard-force-push:
    type: guard
    hook_type: PreToolUse
    command: gt tap guard force-push
    description: "Block force-push in agent context"
    matchers:
      - pattern: "Bash(*push*--force*)"
        scope: [all]
    default_enabled: true
    tags: [safety, git]

  guard-pr-workflow:
    type: guard
    hook_type: PreToolUse
    command: gt tap guard pr-workflow
    description: "Block PR creation and feature branches in agent context"
    matchers:
      - pattern: "Bash(gh pr create*)"
        scope: [crew, polecats]
      - pattern: "Bash(git checkout -b*)"
        scope: [crew, polecats]
      - pattern: "Bash(git switch -c*)"
        scope: [crew, polecats]
    default_enabled: true
    tags: [safety, git, workflow]

  guard-direct-main-push:
    type: guard
    hook_type: PreToolUse
    command: gt tap guard direct-main-push
    description: "Block direct push to main/master branches"
    matchers:
      - pattern: "Bash(git push *main)"
        scope: [all]
      - pattern: "Bash(git push *master)"
        scope: [all]
    default_enabled: true
    tags: [safety, git]

  # Audit hooks (PostToolUse - track operations)
  audit-git-push:
    type: audit
    hook_type: PostToolUse
    command: gt tap audit git-push
    description: "Track all git push operations"
    matchers:
      - pattern: "Bash(git push*)"
        scope: [all]
    default_enabled: false
    tags: [audit, git]

  audit-file-writes:
    type: audit
    hook_type: PostToolUse
    command: gt tap audit file-writes
    description: "Track all file write operations"
    matchers:
      - pattern: "Write(*)"
        scope: [all]
      - pattern: "Edit(*)"
        scope: [all]
    default_enabled: false
    tags: [audit, filesystem]

  # Session lifecycle hooks
  session-prime:
    type: lifecycle
    hook_type: SessionStart
    command: gt prime --hook
    description: "Load role context on session start"
    matchers:
      - pattern: ""
        scope: [all]
    default_enabled: true
    tags: [lifecycle, context]

  session-costs:
    type: lifecycle
    hook_type: Stop
    command: gt costs record
    description: "Record session costs on stop"
    matchers:
      - pattern: ""
        scope: [all]
    default_enabled: true
    tags: [lifecycle, costs]

  mail-check:
    type: lifecycle
    hook_type: UserPromptSubmit
    command: gt mail check --inject
    description: "Check mail on user prompt submit"
    matchers:
      - pattern: ""
        scope: [all]
    default_enabled: true
    tags: [lifecycle, mail]

  # Shell script hooks (external)
  joe-hook:
    type: shell
    hook_type: SessionStart
    command: /path/to/joe-hook.sh
    description: "Joe's custom session initialization"
    matchers:
      - pattern: ""
        scope: [crew]
    default_enabled: false
    tags: [custom, external]
    shell: true  # Indicates this is a raw shell script

metadata:
  # Hook type definitions
  types:
    guard:
      description: "PreToolUse hooks that can block operations"
      hook_type: PreToolUse
      can_block: true
    audit:
      description: "PostToolUse hooks that track operations"
      hook_type: PostToolUse
      can_block: false
    lifecycle:
      description: "Session lifecycle hooks"
      hook_types: [SessionStart, PreCompact, UserPromptSubmit, Stop]
      can_block: false
    shell:
      description: "External shell script hooks"
      hook_types: [SessionStart, PreToolUse, PostToolUse, Stop]
      can_block: depends  # Shell script determines

  # Scope definitions
  scopes:
    all: "All agents (crew, polecats, witness, refinery, deacon, mayor)"
    crew: "Crew workers only"
    polecats: "Polecat workers only"
    witness: "Witness agents only"
    refinery: "Refinery agents only"
    mayor: "Mayor agent only"
    deacon: "Deacon agents only"
```

### Hook Ordering

When multiple hooks match the same pattern, they execute in this order:

1. **Explicit order** in registry (if specified)
2. **Type order**: guards → audits → lifecycle
3. **Registration order**: first registered runs first

Override ordering per-role or per-worktree:

```yaml
# ~/.gt/hooks-overrides/crew.json
{
  "hook_order": {
    "Bash(git push*)": [
      "guard-direct-main-push",  # Runs first
      "guard-force-push",        # Runs second
      "audit-git-push"           # Runs last (after push succeeds)
    ]
  }
}
```

### Shell Script Integration

Two approaches for shell script hooks:

#### Option A: Wrapper Command (Recommended)

```bash
# Shell scripts registered in registry
gt tap run joe-hook
```

Benefits:
- Unified command interface
- Registry tracks all hooks
- Can add metadata (description, tags)
- `gt hooks list` shows both types

Implementation:
```go
// internal/tap/run.go
func RunShellHook(scriptName string) error {
    // Look up script path in registry
    // Execute with proper environment
    // Return exit code
}
```

#### Option B: Direct Registration (Current)

Shell scripts directly registered with full path:

```yaml
joe-hook:
  type: shell
  command: /path/to/joe-hook.sh
  shell: true
```

Benefits:
- No wrapper needed
- Direct execution
- Simpler for one-off scripts

Drawbacks:
- Absolute paths break portability
- No standardized interface
- Hard to version or share

**Recommendation**: Use Option A with `gt tap run <script-name>` and symlink/install scripts to `~/.gt/hooks-scripts/`.

### Per-Worktree Control

Enable per-worktree hook control via settings.local.json (gitignored):

```json
// gastown/polecats/alice/.claude/settings.local.json
{
  "hooks_disabled": [
    "guard-pr-workflow",  // Alice needs to test PR creation
    "audit-file-writes"   // Too noisy for her workflow
  ],
  "hooks_enabled": [
    "audit-git-push"      // Override role default (disabled)
  ]
}
```

Merge precedence:
1. `settings.local.json` hooks_disabled/enabled (highest)
2. Registry override for rig+role
3. Registry override for role
4. Registry default_enabled (lowest)

## Commands

### Registry Management

#### `gt hooks registry`

List all available hooks in the registry:

```bash
$ gt hooks registry

Available Hooks:

GUARDS (PreToolUse - can block operations):
  ✓ guard-force-push         Block force-push in agent context
  ✓ guard-pr-workflow        Block PR creation and feature branches
  ✓ guard-direct-main-push   Block direct push to main/master

AUDITS (PostToolUse - track operations):
    audit-git-push           Track all git push operations
    audit-file-writes        Track all file write operations

LIFECYCLE (Session hooks):
  ✓ session-prime            Load role context on session start
  ✓ session-costs            Record session costs on stop
  ✓ mail-check               Check mail on user prompt submit

SHELL (External scripts):
    joe-hook                 Joe's custom session initialization

Legend: ✓ = enabled by default

$ gt hooks registry --verbose
# Shows full details: command, matchers, scopes, tags

$ gt hooks registry --json
# Machine-readable output
```

#### `gt hooks search <query>`

Search hooks by name, description, or tags:

```bash
$ gt hooks search git

Found 4 hooks matching 'git':

  guard-force-push        [guard, git, safety]
    Block force-push in agent context

  guard-pr-workflow       [guard, git, workflow, safety]
    Block PR creation and feature branches

  guard-direct-main-push  [guard, git, safety]
    Block direct push to main/master

  audit-git-push          [audit, git]
    Track all git push operations
```

#### `gt hooks install <hook-id>`

Install a hook from registry to base config:

```bash
$ gt hooks install guard-pr-workflow

Installing hook: guard-pr-workflow
  Type: guard (PreToolUse)
  Command: gt tap guard pr-workflow
  Matchers: 3 patterns
  Scope: crew, polecats
  Default: enabled

Add to base config? [Y/n] y

✓ Added to ~/.gt/hooks-base.json
  Run 'gt hooks sync' to apply to all worktrees.

$ gt hooks sync
Syncing hooks to 12 worktrees... ✓
```

#### `gt hooks enable/disable <hook-id>`

Enable or disable a hook for current worktree:

```bash
$ gt hooks disable guard-pr-workflow

Disabling guard-pr-workflow for this worktree...
  Writing to .claude/settings.local.json

✓ Hook disabled in this worktree only
  Other worktrees are unaffected.

$ gt hooks enable guard-pr-workflow

Enabling guard-pr-workflow for this worktree...
  Removing from disabled list in settings.local.json

✓ Hook enabled (inherits from base config)
```

#### `gt hooks status`

Show hook status for current worktree:

```bash
$ gt hooks status

Hook Status for gastown/polecats/alice:

ENABLED (8):
  ✓ guard-force-push          [base]
  ✓ guard-direct-main-push    [base]
  ✓ session-prime             [base]
  ✓ session-costs             [base]
  ✓ mail-check                [base]
  ✓ audit-git-push            [local]  ← enabled in settings.local.json

DISABLED (3):
  ✗ guard-pr-workflow         [local]  ← disabled in settings.local.json
  ✗ audit-file-writes         [base]   ← disabled by default
  ✗ joe-hook                  [base]   ← disabled by default

Legend:
  [base]  = from base config
  [role]  = from role override
  [rig]   = from rig+role override
  [local] = from settings.local.json (this worktree only)
```

### Integration with Existing Commands

#### `gt hooks sync`

Enhanced to use registry:

```bash
$ gt hooks sync

Syncing hooks from registry...
  Base config: ~/.gt/hooks-base.json (5 hooks enabled)
  Overrides:
    - crew.json (1 hook added)
    - gastown/polecats.json (0 hooks)

Generating settings.json for 12 targets:
  gastown/crew/.claude/settings.json           [changed]
  gastown/polecats/.claude/settings.json       [unchanged]
  gastown/witness/.claude/settings.json        [unchanged]
  ...

Write changes? [Y/n] y

✓ Synced 12 files
```

#### `gt hooks init`

Enhanced to discover and register hooks:

```bash
$ gt hooks init

Bootstrapping hooks registry from existing settings...

Found hooks in current settings files:
  - SessionStart with 'gt prime' (3 locations)
  - SessionStart with 'gt prime --hook' (4 locations)
  - PreToolUse with 'gt tap guard pr-workflow' (2 locations)
  - Stop with 'gt costs record' (7 locations)

Creating registry entries:
  ✓ session-prime
  ✓ guard-pr-workflow
  ✓ session-costs

Created ~/.gt/hooks-registry.yaml with 3 hooks.
Created ~/.gt/hooks-base.json from common hooks.
Created overrides for differences.

Run 'gt hooks sync' to verify.
```

#### `gt doctor hooks`

Enhanced to check registry consistency:

```bash
$ gt doctor hooks

Checking hooks consistency...

✓ Registry exists: ~/.gt/hooks-registry.yaml
✓ Base config exists: ~/.gt/hooks-base.json
✓ All registry hooks have valid commands

⚠ Settings files out of sync:
  - gastown/crew/.claude/settings.json (2 hooks missing)
  - beads/polecats/.claude/settings.json (1 hook extra)

Run 'gt doctor hooks --fix' to sync.
Run 'gt hooks sync' manually if you want to preview changes.
```

## Hook Types and Semantics

### Guard Hooks (PreToolUse)

**Purpose**: Block dangerous operations in agent context

**Behavior**:
- Exit code 0: Allow operation
- Exit code 2: Block operation (Claude shows error)
- Exit code 1: Hook error (operation allowed by default)

**Example**: `gt tap guard force-push`

```go
// internal/tap/guards/force_push.go
func GuardForcePush(cmd string) int {
    // Check if in Gas Town agent context
    if inGasTownContext() {
        fmt.Fprintln(os.Stderr, "⛔ Force-push blocked in agent context")
        return 2  // Block
    }
    return 0  // Allow
}
```

### Audit Hooks (PostToolUse)

**Purpose**: Track operations for debugging/compliance

**Behavior**:
- Exit code ignored (never blocks)
- Logs to Gas Town audit trail
- Can send notifications

**Example**: `gt tap audit git-push`

```go
// internal/tap/audits/git_push.go
func AuditGitPush(cmd string) int {
    // Extract git push details
    branch := parseGitBranch(cmd)
    remote := parseGitRemote(cmd)

    // Log to audit trail
    auditLog.Record("git-push", map[string]string{
        "branch": branch,
        "remote": remote,
        "agent":  getCurrentAgent(),
        "time":   time.Now().String(),
    })

    return 0  // Always allow
}
```

### Lifecycle Hooks

**Purpose**: Session management and context loading

**Behavior**:
- Run at specific lifecycle points
- Exit code ignored
- Can modify environment

**Example**: `gt prime --hook`

Loads role-specific context from hook:

```bash
# SessionStart hook
gt prime --hook
# Reads hooked bead/molecule
# Injects context into prompt
```

### Shell Hooks

**Purpose**: Custom user scripts

**Behavior**:
- Executed via `gt tap run <script-name>`
- Registry tracks metadata
- Scripts installed to `~/.gt/hooks-scripts/`
- Full shell environment available

**Example**: Joe's custom initialization

```bash
#!/bin/bash
# ~/.gt/hooks-scripts/joe-init.sh

# Set custom environment
export JOE_MODE=1

# Log session start
echo "$(date): Session started by $USER" >> ~/.joe-sessions.log

# Custom prompt injection
echo "Remember to check the dashboard!"
```

Registry entry:

```yaml
joe-init:
  type: shell
  hook_type: SessionStart
  command: joe-init.sh  # Resolved to ~/.gt/hooks-scripts/joe-init.sh
  description: "Joe's custom session initialization"
  default_enabled: false
  shell: true
```

## Implementation Phases

### Phase 1: Registry Foundation (This PR)

**Goal**: Establish registry format and basic commands

**Deliverables**:
- Registry schema in `~/.gt/hooks-registry.yaml`
- `gt hooks registry` command (list hooks)
- `gt hooks search` command
- Registry validation on `gt hooks sync`
- Documentation in this file

**No breaking changes**: Existing `gt hooks` commands continue to work

### Phase 2: Installation and Management

**Goal**: Enable installing hooks from registry

**Deliverables**:
- `gt hooks install <hook-id>` command
- `gt hooks enable/disable <hook-id>` for per-worktree control
- `gt hooks status` command
- Settings.local.json support (hooks_disabled, hooks_enabled)
- `gt hooks init` enhancement to discover existing hooks

### Phase 3: gt tap Integration

**Goal**: Integrate existing `gt tap guard/audit` with registry

**Deliverables**:
- Register all existing `gt tap guard/*` commands
- Register all existing `gt tap audit/*` commands
- `gt tap list` shows hooks from registry
- `gt tap install <hook-id>` alias for `gt hooks install`
- Automatic registration of new guards/audits

### Phase 4: Shell Script Hooks

**Goal**: Support custom shell scripts in registry

**Deliverables**:
- `gt tap run <script-name>` command
- Scripts directory: `~/.gt/hooks-scripts/`
- Shell hook registration in registry
- Hook ordering configuration
- Examples and documentation

### Phase 5: Advanced Features

**Goal**: Hook ordering, versioning, sharing

**Deliverables**:
- Hook execution order control
- Registry versioning (migrate format changes)
- Export/import hooks (share across installations)
- Hook bundles (multiple related hooks)
- Hook templates (parameterized hooks)

## Migration Path

### For Existing Installations

1. **No immediate changes required**: Current `gt hooks sync` continues to work
2. **Run `gt hooks init`**: Discovers existing hooks, creates registry
3. **Review registry**: `gt hooks registry --verbose`
4. **Sync settings**: `gt hooks sync` (uses registry)
5. **Enable new hooks**: `gt hooks install <hook-id>` for new registry hooks

### For New Installations

1. **Registry bootstrapped automatically** on `gt install`
2. **Default hooks enabled**: guards, lifecycle hooks
3. **Run `gt hooks sync`** after customizing
4. **Browse registry**: `gt hooks registry`

## Open Questions and Decisions

### Q1: Where does the registry live?

**Options**:
- A: `~/.gt/hooks-registry.yaml` (user-level)
- B: `~/gt/.beads/hooks-registry.yaml` (town-level)
- C: Both (town-level with user overrides)

**Decision**: Start with A (user-level) for simplicity. Phase 5 can add town-level registry.

### Q2: How to version/migrate registry format?

**Options**:
- A: Version field in registry, `gt hooks migrate` command
- B: Auto-migration on first run
- C: Breaking change warning, manual migration

**Decision**: A - version field + migration command for safety.

### Q3: Per-worktree disable vs global disable?

**Decision**: Support both:
- **settings.local.json** (gitignored): Per-worktree disable
- **Registry override**: Role-level or rig-level disable

Precedence: local > rig > role > base

### Q4: Should gt tap commands require registry?

**Options**:
- A: Yes - all hooks must be registered
- B: No - allow ad-hoc guards/audits
- C: Registry-first, but allow unregistered

**Decision**: C - Registry is source of truth, but allow unregistered hooks for testing. Warn if unregistered hook found during `gt doctor hooks`.

### Q5: Hook ordering - explicit or implicit?

**Decision**: Implicit by default (type order: guards → audits → lifecycle), explicit override in registry or per-role override file.

### Q6: Shell script hooks - wrapper or direct?

**Decision**: Wrapper (`gt tap run <script-name>`) for consistency. Scripts stored in `~/.gt/hooks-scripts/`. Registry tracks metadata.

## Related Documents

- [HOOKS.md](HOOKS.md) - Current hooks management system
- [mol-mall-design.md](mol-mall-design.md) - Registry pattern reference
- Claude Code hooks documentation

## Success Metrics

- **Discoverability**: New users can find and install hooks with `gt hooks registry`
- **Consistency**: `gt doctor hooks` verifies all worktrees have correct hooks
- **Flexibility**: Users can disable hooks per-worktree without touching base config
- **Extensibility**: New hooks added to registry are automatically available
- **Maintainability**: Single source of truth eliminates manual copying

## Appendix: Example Workflows

### Adding a New Guard Hook

```bash
# 1. Implement the guard
# internal/tap/guards/my_guard.go

# 2. Register in registry
vim ~/.gt/hooks-registry.yaml
# Add entry:
#   guard-my-guard:
#     type: guard
#     command: gt tap guard my-guard
#     ...

# 3. Install to base config
gt hooks install guard-my-guard

# 4. Sync to all worktrees
gt hooks sync

# 5. Verify
gt hooks status
```

### Disabling a Hook for One Worktree

```bash
# In worktree where you need to disable
cd ~/gt/gastown/polecats/alice

# Disable the hook locally
gt hooks disable guard-pr-workflow

# Verify it's disabled
gt hooks status
# Shows: guard-pr-workflow [local] (disabled)

# Other worktrees unaffected
cd ~/gt/gastown/polecats/bob
gt hooks status
# Shows: guard-pr-workflow [base] (enabled)
```

### Adding a Shell Script Hook

```bash
# 1. Write your script
vim ~/.gt/hooks-scripts/my-init.sh
chmod +x ~/.gt/hooks-scripts/my-init.sh

# 2. Register in registry
vim ~/.gt/hooks-registry.yaml
# Add:
#   my-init:
#     type: shell
#     hook_type: SessionStart
#     command: my-init.sh
#     shell: true

# 3. Install (creates hook entry referencing gt tap run my-init)
gt hooks install my-init

# 4. Sync
gt hooks sync

# 5. Test
claude
# Your script runs on SessionStart
```
