# Gas Town Reference

Technical reference for Gas Town internals. Read the README first.

## Directory Structure

```
~/gt/                           Town root
├── .beads/                     Town-level beads (hq-* prefix)
├── mayor/                      Mayor agent home (town coordinator)
│   ├── town.json               Town configuration
│   ├── CLAUDE.md               Mayor context (on disk)
│   └── .claude/settings.json   Mayor Claude settings
├── deacon/                     Deacon agent home (background supervisor)
│   └── .claude/settings.json   Deacon settings (context via gt prime)
└── <rig>/                      Project container (NOT a git clone)
    ├── config.json             Rig identity
    ├── .beads/ → mayor/rig/.beads
    ├── .repo.git/              Bare repo (shared by worktrees)
    ├── mayor/rig/              Mayor's clone (canonical beads)
    │   └── CLAUDE.md           Per-rig mayor context (on disk)
    ├── witness/                Witness agent home (monitors only)
    │   └── .claude/settings.json  (context via gt prime)
    ├── refinery/               Refinery settings parent
    │   ├── .claude/settings.json
    │   └── rig/                Worktree on main
    │       └── CLAUDE.md       Refinery context (on disk)
    ├── crew/                   Crew settings parent (shared)
    │   ├── .claude/settings.json  (context via gt prime)
    │   └── <name>/rig/         Human workspaces
    └── polecats/               Polecat settings parent (shared)
        ├── .claude/settings.json  (context via gt prime)
        └── <name>/rig/         Worker worktrees
```

**Key points:**

- Rig root is a container, not a clone
- `.repo.git/` is bare - refinery and polecats are worktrees
- Per-rig `mayor/rig/` holds canonical `.beads/`, others inherit via redirect
- Settings placed in parent dirs (not git clones) for upward traversal

## Beads Routing

Gas Town routes beads commands based on issue ID prefix. You don't need to think
about which database to use - just use the issue ID.

```bash
bd show gp-xyz    # Routes to greenplace rig's beads
bd show hq-abc    # Routes to town-level beads
bd show wyv-123   # Routes to wyvern rig's beads
```

**How it works**: Routes are defined in `~/gt/.beads/routes.jsonl`. Each rig's
prefix maps to its beads location (the mayor's clone in that rig).

| Prefix | Routes To | Purpose |
|--------|-----------|---------|
| `hq-*` | `~/gt/.beads/` | Mayor mail, cross-rig coordination |
| `gp-*` | `~/gt/greenplace/mayor/rig/.beads/` | Greenplace project issues |
| `wyv-*` | `~/gt/wyvern/mayor/rig/.beads/` | Wyvern project issues |

Debug routing: `BD_DEBUG_ROUTING=1 bd show <id>`

## Configuration

### Rig Config (`config.json`)

```json
{
  "type": "rig",
  "name": "myproject",
  "git_url": "https://github.com/...",
  "beads": { "prefix": "mp" }
}
```

### Settings (`settings/config.json`)

```json
{
  "theme": "desert",
  "max_workers": 5,
  "merge_queue": { "enabled": true }
}
```

### Runtime (`.runtime/` - gitignored)

Process state, PIDs, ephemeral data.

## Formula Format

```toml
formula = "name"
type = "workflow"           # workflow | expansion | aspect
version = 1
description = "..."

[vars.feature]
description = "..."
required = true

[[steps]]
id = "step-id"
title = "{{feature}}"
description = "..."
needs = ["other-step"]      # Dependencies
```

**Composition:**

```toml
extends = ["base-formula"]

[compose]
aspects = ["cross-cutting"]

[[compose.expand]]
target = "step-id"
with = "macro-formula"
```

## Molecule Lifecycle

```
Formula (source TOML) ─── "Ice-9"
    │
    ▼ bd cook
Protomolecule (frozen template) ─── Solid
    │
    ├─▶ bd mol pour ──▶ Mol (persistent) ─── Liquid ──▶ bd squash ──▶ Digest
    │
    └─▶ bd mol wisp ──▶ Wisp (ephemeral) ─── Vapor ──┬▶ bd squash ──▶ Digest
                                                  └▶ bd burn ──▶ (gone)
```

**Note**: Wisps are stored in `.beads/` with an ephemeral flag - they're not
persisted to JSONL. They exist only in memory during execution.

## Molecule Commands

**Principle**: `bd` = beads data operations, `gt` = agent operations.

### Beads Operations (bd)

```bash
# Formulas
bd formula list              # Available formulas
bd formula show <name>       # Formula details
bd cook <formula>            # Formula → Proto

# Molecules (data operations)
bd mol list                  # Available protos
bd mol show <id>             # Proto details
bd mol pour <proto>          # Create mol
bd mol wisp <proto>          # Create wisp
bd mol bond <proto> <parent> # Attach to existing mol
bd mol squash <id>           # Condense to digest (explicit ID)
bd mol burn <id>             # Discard wisp (explicit ID)
```

### Agent Operations (gt)

```bash
# Hook management (operates on current agent's hook)
gt hook                    # What's on MY hook
gt mol current               # What should I work on next
gt mol progress <id>         # Execution progress of molecule
gt mol attach <bead> <mol>   # Pin molecule to bead
gt mol detach <bead>         # Unpin molecule from bead
gt mol attach-from-mail <id> # Attach from mail message

# Agent lifecycle (operates on agent's attached molecule)
gt mol burn                  # Burn attached molecule (no ID needed)
gt mol squash                # Squash attached molecule (no ID needed)
gt mol step done <step>      # Complete a molecule step
```

**Key distinction**: `bd mol burn/squash <id>` take explicit molecule IDs.
`gt mol burn/squash` operate on the current agent's attached molecule
(auto-detected from working directory).

## Agent Lifecycle

### Polecat Shutdown

```
1. Complete work steps
2. bd mol squash (create digest)
3. Submit to merge queue
4. gt handoff (request shutdown)
5. Wait for Witness to kill session
6. Witness removes worktree + branch
```

### Session Cycling

```
1. Agent notices context filling
2. gt handoff (sends mail to self)
3. Manager kills session
4. Manager starts new session
5. New session reads handoff mail
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `BD_ACTOR` | Agent identity for attribution (see [identity.md](identity.md)) |
| `BEADS_DIR` | Point to shared beads database |
| `BEADS_NO_DAEMON` | Required for worktree polecats |
| `GIT_AUTHOR_NAME` | Set to BD_ACTOR for commit attribution |
| `GIT_AUTHOR_EMAIL` | Workspace owner email |
| `GT_TOWN_ROOT` | Override town root detection |
| `GT_ROLE` | Agent role type (mayor, polecat, etc.) |
| `GT_RIG` | Rig name for rig-level agents |
| `GT_POLECAT` | Polecat name (for polecats only) |

## Agent Working Directories and Settings

Each agent runs in a specific working directory and has its own Claude settings.
Understanding this hierarchy is essential for proper configuration.

### Working Directories by Role

| Role | Working Directory | Notes |
|------|-------------------|-------|
| **Mayor** | `~/gt/mayor/` | Town-level coordinator, isolated from rigs |
| **Deacon** | `~/gt/deacon/` | Background supervisor daemon |
| **Witness** | `~/gt/<rig>/witness/` | No git clone, monitors polecats only |
| **Refinery** | `~/gt/<rig>/refinery/rig/` | Worktree on main branch |
| **Crew** | `~/gt/<rig>/crew/<name>/rig/` | Persistent human workspace clone |
| **Polecat** | `~/gt/<rig>/polecats/<name>/rig/` | Ephemeral worker worktree |

Note: The per-rig `<rig>/mayor/rig/` directory is NOT a working directory—it's
a git clone that holds the canonical `.beads/` database for that rig.

### Settings File Locations

Claude Code searches for `.claude/settings.json` starting from the working
directory and traversing upward. Settings are placed in **parent directories**
(not inside git clones) so they're found via directory traversal without
polluting source repositories:

```
~/gt/
├── mayor/.claude/settings.json          # Mayor settings
├── deacon/.claude/settings.json         # Deacon settings
└── <rig>/
    ├── witness/.claude/settings.json    # Witness settings (no rig/ subdir)
    ├── refinery/.claude/settings.json   # Found by refinery/rig/ via traversal
    ├── crew/.claude/settings.json       # Shared by all crew/<name>/rig/
    └── polecats/.claude/settings.json   # Shared by all polecats/<name>/rig/
```

**Why parent directories?** Agents working in git clones (like `refinery/rig/`)
would pollute the source repo if settings were placed there. By putting settings
one level up, Claude finds them via upward traversal, and all workers of the
same type share the same settings.

### CLAUDE.md Locations

Role context is delivered via CLAUDE.md files or ephemeral injection:

| Role | CLAUDE.md Location | Method |
|------|-------------------|--------|
| **Mayor** | `~/gt/mayor/CLAUDE.md` | On disk |
| **Deacon** | (none) | Injected via `gt prime` at SessionStart |
| **Witness** | (none) | Injected via `gt prime` at SessionStart |
| **Refinery** | `<rig>/refinery/rig/CLAUDE.md` | On disk (inside worktree) |
| **Crew** | (none) | Injected via `gt prime` at SessionStart |
| **Polecat** | (none) | Injected via `gt prime` at SessionStart |

Additionally, each rig has `<rig>/mayor/rig/CLAUDE.md` for the per-rig mayor clone
(used for beads operations, not a running agent).

**Why ephemeral injection?** Writing CLAUDE.md into git clones would:
1. Pollute source repos when agents commit/push
2. Leak Gas Town internals into project history
3. Conflict with project-specific CLAUDE.md files

The `gt prime` command runs at SessionStart hook and injects context without
persisting it to disk.

### Sparse Checkout (Source Repo Isolation)

When agents work on source repositories that have their own Claude Code configuration,
Gas Town uses git sparse checkout to exclude all context files:

```bash
# Automatically configured for worktrees - excludes:
# - .claude/       : settings, rules, agents, commands
# - CLAUDE.md      : primary context file
# - CLAUDE.local.md: personal context file
# - .mcp.json      : MCP server configuration
git sparse-checkout set --no-cone '/*' '!/.claude/' '!/CLAUDE.md' '!/CLAUDE.local.md' '!/.mcp.json'
```

This ensures agents use Gas Town's context, not the source repo's instructions.

**Doctor check**: `gt doctor` verifies sparse checkout is configured correctly.
Run `gt doctor --fix` to update legacy configurations missing the newer patterns.

### Settings Inheritance

Claude Code's settings search order (first match wins):

1. `.claude/settings.json` in current working directory
2. `.claude/settings.json` in parent directories (traversing up)
3. `~/.claude/settings.json` (user global settings)

Gas Town places settings at each agent's working directory root, so agents
find their role-specific settings before reaching any parent or global config.

### Settings Templates

Gas Town uses two settings templates based on role type:

| Type | Roles | Key Difference |
|------|-------|----------------|
| **Interactive** | Mayor, Crew | Mail injected on `UserPromptSubmit` hook |
| **Autonomous** | Polecat, Witness, Refinery, Deacon | Mail injected on `SessionStart` hook |

Autonomous agents may start without user input, so they need mail checked
at session start. Interactive agents wait for user prompts.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Agent using wrong settings | Check `gt doctor`, verify sparse checkout |
| Settings not found | Ensure `.claude/settings.json` exists at role home |
| Source repo settings leaking | Run `gt doctor --fix` to configure sparse checkout |
| Mayor settings affecting polecats | Mayor should run in `mayor/`, not town root |

## CLI Reference

### Town Management

```bash
gt install [path]            # Create town
gt install --git             # With git init
gt doctor                    # Health check
gt doctor --fix              # Auto-repair
```

### Configuration

```bash
# Agent management
gt config agent list [--json]     # List all agents (built-in + custom)
gt config agent get <name>        # Show agent configuration
gt config agent set <name> <cmd>  # Create or update custom agent
gt config agent remove <name>     # Remove custom agent (built-ins protected)

# Default agent
gt config default-agent [name]    # Get or set town default agent
```

**Built-in agents**: `claude`, `gemini`, `codex`

**Custom agents**: Define per-town in `mayor/town.json`:
```bash
gt config agent set claude-glm "claude-glm --model glm-4"
gt config agent set claude "claude-opus"  # Override built-in
gt config default-agent claude-glm       # Set default
```

### Rig Management

```bash
gt rig add <name> <url>
gt rig list
gt rig remove <name>
```

### Convoy Management (Primary Dashboard)

```bash
gt convoy list                          # Dashboard of active convoys
gt convoy status [convoy-id]            # Show progress (🚚 hq-cv-*)
gt convoy create "name" [issues...]     # Create convoy tracking issues
gt convoy create "name" gt-a bd-b --notify mayor/  # With notification
gt convoy list --all                    # Include landed convoys
gt convoy list --status=closed          # Only landed convoys
```

Note: "Swarm" is ephemeral (workers on a convoy's issues). See [Convoys](convoy.md).

### Work Assignment

```bash
# Standard workflow: convoy first, then sling
gt convoy create "Feature X" gt-abc gt-def
gt sling gt-abc <rig>                    # Assign to polecat
gt sling gt-abc <rig> --agent codex      # Override runtime for this sling/spawn
gt sling <proto> --on gt-def <rig>       # With workflow template

# Quick sling (auto-creates convoy)
gt sling <bead> <rig>                    # Auto-convoy for dashboard visibility
```

Agent overrides:

- `gt start --agent <alias>` overrides the Mayor/Deacon runtime for this launch.
- `gt mayor start|attach|restart --agent <alias>` and `gt deacon start|attach|restart --agent <alias>` do the same.
- `gt start crew <name> --agent <alias>` and `gt crew at <name> --agent <alias>` override the crew worker runtime.

### Communication

```bash
gt mail inbox
gt mail read <id>
gt mail send <addr> -s "Subject" -m "Body"
gt mail send --human -s "..."    # To overseer
```

### Escalation

```bash
gt escalate "topic"              # Default: MEDIUM severity
gt escalate -s CRITICAL "msg"    # Urgent, immediate attention
gt escalate -s HIGH "msg"        # Important blocker
gt escalate -s MEDIUM "msg" -m "Details..."
```

See [escalation.md](escalation.md) for full protocol.

### Sessions

```bash
gt handoff                   # Request cycle (context-aware)
gt handoff --shutdown        # Terminate (polecats)
gt session stop <rig>/<agent>
gt peek <agent>              # Check health
gt nudge <agent> "message"   # Send message to agent
gt seance                    # List discoverable predecessor sessions
gt seance --talk <id>        # Talk to predecessor (full context)
gt seance --talk <id> -p "Where is X?"  # One-shot question
```

**Session Discovery**: Each session has a startup nudge that becomes searchable
in Claude's `/resume` picker:

```
[GAS TOWN] recipient <- sender • timestamp • topic[:mol-id]
```

Example: `[GAS TOWN] gastown/crew/gus <- human • 2025-12-30T15:42 • restart`

**IMPORTANT**: Always use `gt nudge` to send messages to Claude sessions.
Never use raw `tmux send-keys` - it doesn't handle Claude's input correctly.
`gt nudge` uses literal mode + debounce + separate Enter for reliable delivery.

### Emergency

```bash
gt stop --all                # Kill all sessions
gt stop --rig <name>         # Kill rig sessions
```

## Beads Commands (bd)

```bash
bd ready                     # Work with no blockers
bd list --status=open
bd list --status=in_progress
bd show <id>
bd create --title="..." --type=task
bd update <id> --status=in_progress
bd close <id>
bd dep add <child> <parent>  # child depends on parent
bd sync                      # Push/pull changes
```

## Patrol Agents

Deacon, Witness, and Refinery run continuous patrol loops using wisps:

| Agent | Patrol Molecule | Responsibility |
|-------|-----------------|----------------|
| **Deacon** | `mol-deacon-patrol` | Agent lifecycle, plugin execution, health checks |
| **Witness** | `mol-witness-patrol` | Monitor polecats, nudge stuck workers |
| **Refinery** | `mol-refinery-patrol` | Process merge queue, review MRs |

```
1. bd mol wisp mol-<role>-patrol
2. Execute steps (check workers, process queue, run plugins)
3. bd mol squash (or burn if routine)
4. Loop
```

## Plugin Molecules

Plugins are molecules with specific labels:

```json
{
  "id": "mol-security-scan",
  "labels": ["template", "plugin", "witness", "tier:haiku"]
}
```

Patrol molecules bond plugins dynamically:

```bash
bd mol bond mol-security-scan $PATROL_ID --var scope="$SCOPE"
```

## Common Issues

| Problem | Solution |
|---------|----------|
| Agent in wrong directory | Check cwd, `gt doctor` |
| Beads prefix mismatch | Check `bd show` vs rig config |
| Worktree conflicts | Ensure `BEADS_NO_DAEMON=1` for polecats |
| Stuck worker | `gt nudge`, then `gt peek` |
| Dirty git state | Commit or discard, then `gt handoff` |

## Architecture Notes

**Bare repo pattern**: `.repo.git/` is bare (no working dir). Refinery and polecats are worktrees sharing refs. Polecat branches visible to refinery immediately.

**Beads as control plane**: No separate orchestrator. Molecule steps ARE beads issues. State transitions are git commits.

**Nondeterministic idempotence**: Any worker can continue any molecule. Steps are atomic checkpoints in beads.

**Convoy tracking**: Convoys track batched work across rigs. A "swarm" is ephemeral - just the workers currently on a convoy's issues. See [Convoys](convoy.md) for details.
