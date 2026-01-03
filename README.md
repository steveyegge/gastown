# Gas Town

Multi-agent orchestrator for Claude Code. Track work with convoys; sling to agents.

## Why Gas Town?

| Without | With Gas Town |
|---------|---------------|
| Agents forget work after restart | Work persists on hooks - survives crashes, compaction, restarts |
| Manual coordination | Agents have mailboxes, identities, and structured handoffs |
| 4-10 agents is chaotic | Comfortably scale to 20-30 agents |
| Work state in agent memory | Work state in Beads (git-backed ledger) |

## Prerequisites

- **Go 1.23+** - [go.dev/dl](https://go.dev/dl/)
- **Git 2.25+** - for worktree support
- **beads (bd)** - [github.com/steveyegge/beads](https://github.com/steveyegge/beads) - required for issue tracking
- **tmux 3.0+** - recommended for the full experience (the Mayor session is the primary interface)
- **Claude Code CLI** - [claude.ai/code](https://claude.ai/code)

## Quick Start

```bash
# Install
go install github.com/steveyegge/gastown/cmd/gt@latest

# Create workspace
gt install ~/gt

# Add a project
gt rig add myproject https://github.com/you/repo.git

# Enter the Mayor's office (recommended)
gt mayor attach
```

Once inside the Mayor session, you're talking to Claude with full town context. Just tell it what you want:

> "Help me fix the authentication bug in myproject"

The Mayor will create convoys, dispatch workers, and coordinate everything. You can also run CLI commands directly:

```bash
# Create a convoy and sling work (CLI workflow)
gt convoy create "Feature X" issue-123 issue-456 --notify --human
gt sling issue-123 myproject

# Track progress
gt convoy list

# Switch between agent sessions
gt agents
```

## Core Concepts

**The Mayor** is your AI coordinator. It's Claude Code with full context about your workspace, projects, and agents. The Mayor session (`gt prime`) is the primary way to interact with Gas Town - just tell it what you want to accomplish.

```
Town (~/gt/)              Your workspace
├── Mayor                 Your AI coordinator (start here)
├── Rig (project)         Container for a git project + its agents
│   ├── Polecats          Workers (ephemeral, spawn → work → disappear)
│   ├── Witness           Monitors workers, handles lifecycle
│   └── Refinery          Merge queue processor
```

**Hook**: Each agent has a hook where work hangs. On wake, run what's on your hook.

**Beads**: Git-backed issue tracker. All work state lives here. [github.com/steveyegge/beads](https://github.com/steveyegge/beads)

## Workflows

### Full Stack (Recommended)

The primary Gas Town experience. Agents run in tmux sessions with the Mayor as your interface.

```bash
gt start                               # Start Gas Town (daemon + Mayor session)
cd ~/gt && gt prime                    # Enter Mayor session

# Inside Mayor session, just ask:
# "Create a convoy for issues 123 and 456 in myproject"
# "What's the status of my work?"
# "Show me what the witness is doing"

# Or use CLI commands:
gt convoy create "Feature X" issue-123 issue-456
gt sling issue-123 myproject           # Spawns polecat automatically
gt convoy list                         # Dashboard view
gt agents                              # Navigate between sessions
```

### Minimal (No Tmux)

Run individual Claude Code instances manually. Gas Town just tracks state.

```bash
gt convoy create "Fix bugs" issue-123  # Create convoy (sling auto-creates if skipped)
gt sling issue-123 myproject           # Assign to worker
claude --resume                        # Agent reads mail, runs work
gt convoy list                         # Check progress
```

### Pick Your Roles

Gas Town is modular. Run what you need:

- **Polecats only**: Manual spawning, no monitoring
- **+ Witness**: Automatic worker lifecycle, stuck detection
- **+ Refinery**: Merge queue, code review
- **+ Mayor**: Cross-project coordination

## Cooking Formulas

Formulas define structured workflows. Cook them, sling them to agents.

### Basic Example

```toml
# .beads/formulas/shiny.formula.toml
formula = "shiny"
description = "Design before code, review before ship"

[[steps]]
id = "design"
description = "Think about architecture"

[[steps]]
id = "implement"
needs = ["design"]

[[steps]]
id = "test"
needs = ["implement"]

[[steps]]
id = "submit"
needs = ["test"]
```

### Using Formulas

```bash
bd formula list                    # See available formulas
bd cook shiny                      # Cook into a protomolecule
bd mol pour shiny --var feature=auth   # Create runnable molecule
gt convoy create "Auth feature" gt-xyz  # Track with convoy
gt sling gt-xyz myproject          # Assign to worker
gt convoy list                     # Monitor progress
```

### What Happens

1. **Cook** expands the formula into a protomolecule (frozen template)
2. **Pour** creates a molecule (live workflow) with steps as beads
3. **Worker executes** each step, closing beads as it goes
4. **Crash recovery**: Worker restarts, reads molecule, continues from last step

### Example: Beads Release Molecule

A real workflow for releasing a new beads version:

```toml
formula = "beads-release"
description = "Version bump and release workflow"

[[steps]]
id = "bump-version"
description = "Update version in version.go and CHANGELOG"

[[steps]]
id = "update-deps"
needs = ["bump-version"]
description = "Run go mod tidy, update go.sum"

[[steps]]
id = "run-tests"
needs = ["update-deps"]
description = "Full test suite, check for regressions"

[[steps]]
id = "build-binaries"
needs = ["run-tests"]
description = "Cross-compile for all platforms"

[[steps]]
id = "create-tag"
needs = ["build-binaries"]
description = "Git tag with version, push to origin"

[[steps]]
id = "publish-release"
needs = ["create-tag"]
description = "Create GitHub release with binaries"
```

Cook it, pour it, sling it. The polecat runs through each step, and if it crashes
after `run-tests`, a new polecat picks up at `build-binaries`.

### Formula Composition

```toml
# Extend an existing formula
formula = "shiny-enterprise"
extends = ["shiny"]

[compose]
aspects = ["security-audit"]  # Add cross-cutting concerns
```

## Key Commands

### For Humans (Overseer)

```bash
gt start                          # Start Gas Town (daemon + agents)
gt shutdown                       # Graceful shutdown
gt status                         # Town overview
gt <role> attach                  # Jump into any agent session
                                  # e.g., gt mayor attach, gt witness attach
```

Most other work happens through agents - just ask them.

### For Agents

```bash
# Convoy (primary dashboard)
gt convoy list                    # Active work across all rigs
gt convoy status <id>             # Detailed convoy progress
gt convoy create "name" <issues>  # Create new convoy

# Work assignment
gt sling <bead> <rig>             # Assign work to polecat
bd ready                          # Show available work
bd list --status=in_progress      # Active work

# Communication
gt mail inbox                     # Check messages
gt mail send <addr> -s "..." -m "..."

# Lifecycle
gt handoff                        # Request session cycle
gt peek <agent>                   # Check agent health

# Diagnostics
gt doctor                         # Health check
gt doctor --fix                   # Auto-repair
```

## Shell Completions

Enable tab completion for `gt` commands:

### Bash

```bash
# Add to ~/.bashrc
source <(gt completion bash)

# Or install permanently
gt completion bash > /usr/local/etc/bash_completion.d/gt
```

### Zsh

```bash
# Add to ~/.zshrc (before compinit)
source <(gt completion zsh)

# Or install to fpath
gt completion zsh > "${fpath[1]}/_gt"
```

### Fish

```bash
gt completion fish > ~/.config/fish/completions/gt.fish
```

## Roles

| Role | Scope | Job |
|------|-------|-----|
| **Overseer** | Human | Sets strategy, reviews output, handles escalations |
| **Mayor** | Town-wide | Cross-rig coordination, work dispatch |
| **Deacon** | Town-wide | Daemon process, agent lifecycle, plugin execution |
| **Witness** | Per-rig | Monitor polecats, nudge stuck workers |
| **Refinery** | Per-rig | Merge queue, PR review, integration |
| **Polecat** | Per-task | Execute work, file discovered issues, request shutdown |

## The Propulsion Principle

> If your hook has work, RUN IT.

Agents wake up, check their hook, execute the molecule. No waiting for commands.
Molecules survive crashes - any agent can continue where another left off.

---

## Optional: MEOW Deep Dive

**M**olecular **E**xpression **O**f **W**ork - the full algebra.

### States of Matter

| Phase | Name | Storage | Behavior |
|-------|------|---------|----------|
| Ice-9 | Formula | `.beads/formulas/` | Source template, composable |
| Solid | Protomolecule | `.beads/` | Frozen template, reusable |
| Liquid | Mol | `.beads/` | Flowing work, persistent |
| Vapor | Wisp | `.beads/` (ephemeral flag) | Transient, for patrols |

*(Protomolecules are an homage to The Expanse. Ice-9 is a nod to Vonnegut.)*

### Operators

| Operator | From → To | Effect |
|----------|-----------|--------|
| `cook` | Formula → Protomolecule | Expand macros, flatten |
| `pour` | Proto → Mol | Instantiate as persistent |
| `wisp` | Proto → Wisp | Instantiate as ephemeral |
| `squash` | Mol/Wisp → Digest | Condense to permanent record |
| `burn` | Wisp → ∅ | Discard without record |

---

## License

MIT
