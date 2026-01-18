# Daedalus Crew Context

> **Recovery**: Run `gt prime` after compaction, clear, or new session.

**Rig:** Daedalus (formerly gastown)
**Product:** Gas Town - Multi-agent orchestration system for Claude Code

---

## Quick Start

```bash
# Essential commands
make build                # Build gt binary
make install              # Install to ~/.local/bin/gt
make test                 # Run all tests
go generate ./...         # Generate code

# Development
go build -o gt ./cmd/gt   # Quick build
go test ./internal/...    # Test specific package

# Session end (MANDATORY)
bd sync && git push       # ALWAYS push before stopping
```

---

## JIT Context Loading

**Load documentation based on what you need:**

### Finding Code

| I Want To... | Load This |
|--------------|-----------|
| Understand GT CLI commands | [cmd/gt/](cmd/gt/) |
| Find agent management code | [internal/agent/](internal/agent/) |
| Find polecat logic | [internal/polecat/](internal/polecat/) |
| Find convoy tracking | [internal/cmd/convoy*.go](internal/cmd/) |
| Find mail system | [internal/mail/](internal/mail/) |
| Find mayor orchestration | [internal/mayor/](internal/mayor/) |
| Find witness patrol | [internal/witness/](internal/witness/) |
| Find refinery (merge queue) | [internal/refinery/](internal/refinery/) |
| Find beads integration | [internal/beads/](internal/beads/) |
| Find formula/molecule code | [internal/formula/](internal/formula/), [internal/wisp/](internal/wisp/) |
| Find tmux session management | [internal/tmux/](internal/tmux/) |
| Find TUI components | [internal/tui/](internal/tui/) |
| Find rig management | [internal/rig/](internal/rig/) |
| Find crew workspace code | [internal/crew/](internal/crew/) |
| Find git operations | [internal/git/](internal/git/) |
| Find config handling | [internal/config/](internal/config/) |
| Find runtime abstractions | [internal/runtime/](internal/runtime/) |
| Find daemon/deacon code | [internal/daemon/](internal/daemon/), [internal/deacon/](internal/deacon/) |

### Understanding Architecture

| I Want To... | Load This |
|--------------|-----------|
| Full architecture overview | [README.md](README.md) |
| Core concepts glossary | [docs/glossary.md](docs/glossary.md) |
| Architecture details | [docs/architecture.md](docs/architecture.md) |
| Convoy workflow | [docs/convoy.md](docs/convoy.md) |
| Mail protocol | [docs/mail-protocol.md](docs/mail-protocol.md) |
| Molecule/formula system | [docs/molecules.md](docs/molecules.md) |
| Polecat lifecycle | [docs/polecat-lifecycle.md](docs/polecat-lifecycle.md) |
| Propulsion principle (GUPP) | [docs/propulsion-principle.md](docs/propulsion-principle.md) |
| Swarm coordination | [docs/swarm.md](docs/swarm.md) |
| Identity model | [docs/identity.md](docs/identity.md) |
| Escalation patterns | [docs/escalation.md](docs/escalation.md) |

### Writing Code

| I Want To... | Load This |
|--------------|-----------|
| Go module dependencies | [go.mod](go.mod) |
| Build configuration | [Makefile](Makefile) |
| Release process | [RELEASING.md](RELEASING.md) |
| Contributing guidelines | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Agent instructions | [AGENTS.md](AGENTS.md) |

### Templates

| I Want To... | Load This |
|--------------|-----------|
| Polecat agent context | [templates/polecat-CLAUDE.md](templates/polecat-CLAUDE.md) |
| Witness agent context | [templates/witness-CLAUDE.md](templates/witness-CLAUDE.md) |

---

## Repository Structure

```
daedalus/crew/boden/
├── cmd/gt/                 # CLI entry point
├── internal/
│   ├── agent/              # Agent lifecycle management
│   ├── beads/              # Beads issue tracking integration
│   ├── claude/             # Claude Code specific handling
│   ├── cmd/                # Cobra command implementations
│   ├── config/             # Configuration management
│   ├── convoy/             # Convoy (batch work) tracking
│   ├── crew/               # Crew workspace management
│   ├── daemon/             # Background daemon
│   ├── deacon/             # Watchdog patrol service
│   ├── dog/                # Maintenance agents
│   ├── formula/            # Formula TOML handling
│   ├── git/                # Git operations
│   ├── mail/               # Inter-agent messaging
│   ├── mayor/              # Mayor orchestration
│   ├── polecat/            # Ephemeral worker agents
│   ├── refinery/           # Merge queue management
│   ├── rig/                # Project container management
│   ├── runtime/            # Runtime abstractions (claude/codex/etc)
│   ├── session/            # Session state management
│   ├── swarm/              # Multi-agent coordination
│   ├── templates/          # Go templates for agent context
│   ├── tmux/               # Tmux session management
│   ├── tui/                # Terminal UI (bubbletea)
│   ├── wisp/               # Molecule instance tracking
│   ├── witness/            # Polecat oversight patrol
│   └── workspace/          # Workspace structure
├── docs/                   # Documentation
├── mail/                   # Mail templates
├── templates/              # Agent CLAUDE.md templates
└── scripts/                # Build/release scripts
```

---

## Key Concepts

### Gas Town Roles

| Role | Level | Purpose |
|------|-------|---------|
| **Mayor** | Town | Orchestrator - dispatch work, don't implement |
| **Deacon** | Town | Watchdog daemon - monitors system health |
| **Dogs** | Town | Maintenance agents under Deacon |
| **Crew** | Rig | Human-guided persistent workspaces |
| **Polecat** | Rig | Ephemeral workers in isolated worktrees |
| **Witness** | Rig | Monitors polecats and refinery |
| **Refinery** | Rig | Manages merge queue |

### Core Principles

| Principle | Meaning |
|-----------|---------|
| **GUPP** | Gas Town Universal Propulsion Principle - if work is on your hook, run it |
| **MEOW** | Molecular Expression of Work - break goals into atomic trackable units |
| **NDI** | Nondeterministic Idempotence - eventual completion despite failures |

### Work Units

| Unit | Description |
|------|-------------|
| **Convoy** | Batch of issues for parallel execution |
| **Hook** | Git worktree for persistent agent state |
| **Molecule** | Instance of a formula being executed |
| **Formula** | Reusable TOML workflow template |

---

## Common Commands

### Workspace

```bash
gt install <path>           # Initialize town
gt rig add <name> <repo>    # Add project rig
gt rig list                 # List rigs
gt crew add <name> --rig <rig>  # Create crew workspace
```

### Agent Operations

```bash
gt sling <issue> <rig>      # Assign work to polecat
gt hook                     # Check your hooked work
gt prime                    # Load role context
gt agents                   # List active agents
gt polecat list <rig>       # List polecats in rig
gt polecat status <rig>/<name>  # Polecat details
gt polecat nuke <rig>/<name> --force  # Destroy polecat
gt polecat gc <rig>         # Clean merged branches
```

### Convoy (Work Tracking)

```bash
gt convoy create <name> [issues...]  # Create convoy
gt convoy list              # List convoys
gt convoy show [id]         # Convoy details
gt convoy add-issue <issue> # Add to convoy
```

### Mail

```bash
gt mail inbox               # Check messages
gt mail send <recipient> <msg>  # Send message
gt mail broadcast <msg>     # Message all workers
```

### Molecules

```bash
gt mol status               # Check molecule state
gt mol pour <formula>       # Create molecule instance
gt mol list                 # List active molecules
```

---

## Development Workflow

### Adding a New Command

1. Create command file in `internal/cmd/`
2. Register in parent command's `init()`
3. Follow existing patterns (see `convoy*.go` for examples)

### Adding a New Internal Package

1. Create directory in `internal/`
2. Add package-level doc comment
3. Export only necessary types/functions

### Testing

```bash
go test ./...                       # All tests
go test ./internal/convoy/...       # Specific package
go test -v -run TestConvoyCreate    # Specific test
```

---

## Anti-Patterns

| DON'T | DO INSTEAD |
|-------|------------|
| Import from `cmd/` in `internal/` | Keep cmd as thin wrapper |
| Global state in packages | Pass dependencies explicitly |
| Direct tmux calls outside `internal/tmux` | Use tmux package abstractions |
| Hardcode paths | Use config package |
| Skip error wrapping | Wrap with context: `fmt.Errorf("doing X: %w", err)` |

---

## Beads Integration

Gas Town uses Beads for work tracking:

```bash
bd ready                    # Unblocked issues
bd show <id>               # Issue details
bd update <id> --status in_progress
bd close <id> --reason "Done"
bd sync                    # Sync with git
```

Beads data lives in `.beads/` and syncs via git commits.

---

## References

| Document | When to Load |
|----------|--------------|
| [README.md](README.md) | Full product overview |
| [docs/glossary.md](docs/glossary.md) | Term definitions |
| [docs/architecture.md](docs/architecture.md) | System design |
| [CHANGELOG.md](CHANGELOG.md) | Recent changes |
| [RELEASING.md](RELEASING.md) | Release process |
