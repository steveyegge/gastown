# Gas Town Reference

Technical reference for Gas Town internals. Read the README first.

## Directory Structure

```
~/gt/                           Town root
├── .beads/                     Town-level beads (hq-* prefix)
├── mayor/                      Mayor agent home (town coordinator)
│   ├── town.json               Town configuration
│   ├── CLAUDE.md               Mayor context (bootstrap pointer)
│   ├── AGENTS.md               Mayor context for Codex (same content)
│   └── .claude/settings.json   Mayor Claude settings
├── deacon/                     Deacon agent home (background supervisor)
│   └── .claude/settings.json   Deacon settings (context via gt prime)
└── <rig>/                      Project container (NOT a git clone)
    ├── config.json             Rig identity
    ├── .beads/ → mayor/rig/.beads
    ├── .repo.git/              Bare repo (shared by worktrees)
    ├── mayor/rig/              Mayor's clone (canonical beads, NOT an agent)
    ├── witness/                Witness agent home (monitors only)
    │   ├── CLAUDE.md           Witness context (bootstrap pointer)
    │   ├── AGENTS.md           Witness context for Codex
    │   └── .claude/settings.json
    ├── refinery/               Refinery settings parent
    │   ├── CLAUDE.md           Refinery context (bootstrap pointer)
    │   ├── AGENTS.md           Refinery context for Codex
    │   ├── .claude/settings.json
    │   └── rig/                Worktree on main
    ├── crew/                   Crew settings parent (shared)
    │   ├── CLAUDE.md           Crew context (bootstrap pointer)
    │   ├── AGENTS.md           Crew context for Codex
    │   ├── .claude/settings.json
    │   └── <name>/             Human workspaces (worktrees)
    └── polecats/               Polecat settings parent (shared)
        ├── CLAUDE.md           Polecat context (bootstrap pointer)
        ├── AGENTS.md           Polecat context for Codex
        ├── .claude/settings.json
        └── <name>/<rigname>/   Worker worktrees
```

**Key points:**

- Rig root is a container, not a clone
- `.repo.git/` is bare - refinery and polecats are worktrees
- Per-rig `mayor/rig/` holds canonical `.beads/`, others inherit via redirect
- Per-rig `mayor/rig/` is just a source clone, NOT an agent directory (no CLAUDE.md)
- Settings and bootstrap files placed in parent dirs (not git clones) for upward traversal
- CLAUDE.md and AGENTS.md are "bootstrap pointers" - minimal files that tell agents to run `gt prime`
- AGENTS.md provides multi-provider support (Codex reads AGENTS.md, Claude reads CLAUDE.md)

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

### Rig-Level Configuration

Rigs support layered configuration through:
1. **Wisp layer** (`.beads-wisp/config/`) - transient, local overrides
2. **Rig identity bead labels** - persistent rig settings
3. **Town defaults** (`~/gt/settings/config.json`)
4. **System defaults** - compiled-in fallbacks

#### Polecat Branch Naming

Configure custom branch name templates for polecats:

```bash
# Set via wisp (transient - for testing)
echo '{"polecat_branch_template": "adam/{year}/{month}/{description}"}' > \
  ~/gt/.beads-wisp/config/myrig.json

# Or set via rig identity bead labels (persistent)
bd update gt-rig-myrig --labels="polecat_branch_template:adam/{year}/{month}/{description}"
```

**Template Variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{user}` | From `git config user.name` | `adam` |
| `{year}` | Current year (YY format) | `26` |
| `{month}` | Current month (MM format) | `01` |
| `{name}` | Polecat name | `alpha` |
| `{issue}` | Issue ID without prefix | `123` (from `gt-123`) |
| `{description}` | Sanitized issue title | `fix-auth-bug` |
| `{timestamp}` | Unique timestamp | `1ks7f9a` |

**Default Behavior (backward compatible):**

When `polecat_branch_template` is empty or not set:
- With issue: `polecat/{name}/{issue}@{timestamp}`
- Without issue: `polecat/{name}-{timestamp}`

**Example Configurations:**

```bash
# GitHub enterprise format
"adam/{year}/{month}/{description}"

# Simple feature branches
"feature/{issue}"

# Include polecat name for clarity
"work/{name}/{issue}"
```

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

Gas Town sets environment variables for each agent session via `config.AgentEnv()`.
These are set in tmux session environment when agents are spawned.

### Core Variables (All Agents)

| Variable | Purpose | Example |
|----------|---------|---------|
| `GT_ROLE` | Agent role type | `mayor`, `witness`, `polecat`, `crew` |
| `GT_ROOT` | Town root directory | `/home/user/gt` |
| `BD_ACTOR` | Agent identity for attribution | `gastown/polecats/toast` |
| `GIT_AUTHOR_NAME` | Commit attribution (same as BD_ACTOR) | `gastown/polecats/toast` |
| `BEADS_DIR` | Beads database location | `/home/user/gt/gastown/.beads` |

### Rig-Level Variables

| Variable | Purpose | Roles |
|----------|---------|-------|
| `GT_RIG` | Rig name | witness, refinery, polecat, crew |
| `GT_POLECAT` | Polecat worker name | polecat only |
| `GT_CREW` | Crew worker name | crew only |
| `BEADS_AGENT_NAME` | Agent name for beads operations | polecat, crew |
| `BEADS_NO_DAEMON` | Disable beads daemon (isolated context) | polecat, crew |

### Other Variables

| Variable | Purpose |
|----------|---------|
| `GIT_AUTHOR_EMAIL` | Workspace owner email (from git config) |
| `GT_TOWN_ROOT` | Override town root detection (manual use) |
| `CLAUDE_RUNTIME_CONFIG_DIR` | Custom Claude settings directory |

### Environment by Role

| Role | Key Variables |
|------|---------------|
| **Mayor** | `GT_ROLE=mayor`, `BD_ACTOR=mayor` |
| **Deacon** | `GT_ROLE=deacon`, `BD_ACTOR=deacon` |
| **Boot** | `GT_ROLE=boot`, `BD_ACTOR=deacon-boot` |
| **Witness** | `GT_ROLE=witness`, `GT_RIG=<rig>`, `BD_ACTOR=<rig>/witness` |
| **Refinery** | `GT_ROLE=refinery`, `GT_RIG=<rig>`, `BD_ACTOR=<rig>/refinery` |
| **Polecat** | `GT_ROLE=polecat`, `GT_RIG=<rig>`, `GT_POLECAT=<name>`, `BD_ACTOR=<rig>/polecats/<name>` |
| **Crew** | `GT_ROLE=crew`, `GT_RIG=<rig>`, `GT_CREW=<name>`, `BD_ACTOR=<rig>/crew/<name>` |

### Doctor Check

The `gt doctor` command verifies that running tmux sessions have correct
environment variables. Mismatches are reported as warnings:

```
⚠ env-vars: Found 3 env var mismatch(es) across 1 session(s)
    hq-mayor: missing GT_ROOT (expected "/home/user/gt")
```

Fix by restarting sessions: `gt shutdown && gt up`

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

### CLAUDE.md and AGENTS.md Locations

Role context is delivered via bootstrap files (CLAUDE.md/AGENTS.md). These are
minimal "pointer" files that tell agents to run `gt prime` for full context.
AGENTS.md provides multi-provider support (Codex reads AGENTS.md, Claude reads CLAUDE.md).

| Role | Location | Notes |
|------|----------|-------|
| **Mayor** | `~/gt/mayor/CLAUDE.md` | Town-level mayor |
| **Deacon** | (none) | Context injected via `gt prime` at SessionStart |
| **Witness** | `<rig>/witness/CLAUDE.md` | At agent directory level |
| **Refinery** | `<rig>/refinery/CLAUDE.md` | At agent directory level (not inside rig/) |
| **Crew** | `<rig>/crew/CLAUDE.md` | Shared by all crew worktrees via upward traversal |
| **Polecat** | `<rig>/polecats/CLAUDE.md` | Shared by all polecat worktrees via upward traversal |

**Important**: Per-rig `<rig>/mayor/rig/` does NOT have CLAUDE.md - it's just a
source clone that holds the canonical `.beads/` database, not an agent directory.

**Bootstrap content**: Each file contains a minimal pointer like:
```markdown
# Refinery Context (myrig)

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.
```

**Why parent directories?** Placing files at `<rig>/refinery/CLAUDE.md` instead
of `<rig>/refinery/rig/CLAUDE.md` means:
1. Files are outside git worktrees - no pollution of source repos
2. Claude Code finds them via upward directory traversal
3. All workers of the same type share the same bootstrap file

### Sparse Checkout (Source Repo Isolation)

When agents work on source repositories that have their own Claude Code configuration,
Gas Town uses git sparse checkout to exclude Claude Code context files:

```bash
# Automatically configured for worktrees - excludes:
# - .claude/       : settings, rules, agents, commands
# - CLAUDE.md      : primary context file
# - CLAUDE.local.md: personal context file
# Note: .mcp.json is NOT excluded so worktrees inherit MCP server config
git sparse-checkout set --no-cone '/*' '!/.claude/' '!/CLAUDE.md' '!/CLAUDE.local.md'
```

This ensures agents use Gas Town's context, not the source repo's instructions.
MCP servers defined in `.mcp.json` are inherited by all worktrees for tool access.

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

**Built-in agents**: `claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`

**Custom agents**: Define per-town via CLI or JSON:
```bash
gt config agent set claude-glm "claude-glm --model glm-4"
gt config agent set claude "claude-opus"  # Override built-in
gt config default-agent claude-glm       # Set default
```

**Advanced agent config** (`settings/agents.json`):
```json
{
  "version": 1,
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": [],
      "resume_flag": "--session",
      "resume_style": "flag",
      "non_interactive": {
        "subcommand": "run",
        "output_flag": "--format json"
      }
    }
  }
}
```

**Rig-level agents** (`<rig>/settings/config.json`):
```json
{
  "type": "rig-settings",
  "version": 1,
  "agent": "opencode",
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": ["--session"]
    }
  }
}
```

**Agent resolution order**: rig-level → town-level → built-in presets.

For OpenCode autonomous mode, set env var in your shell profile:
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
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

Note: "Swarm" is ephemeral (workers on a convoy's issues). See [Convoys](concepts/convoy.md).

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

See [escalation.md](design/escalation.md) for full protocol.

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

### Health Check

```bash
gt deacon health-check <agent>   # Send health check ping, track response
gt deacon health-state           # Show health check state for all agents
```

### Merge Queue (MQ)

```bash
gt mq list [rig]             # Show the merge queue
gt mq next [rig]             # Show highest-priority merge request
gt mq submit                 # Submit current branch to merge queue
gt mq status <id>            # Show detailed merge request status
gt mq retry <id>             # Retry a failed merge request
gt mq reject <id>            # Reject a merge request
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

**Convoy tracking**: Convoys track batched work across rigs. A "swarm" is ephemeral - just the workers currently on a convoy's issues. See [Convoys](concepts/convoy.md) for details.
