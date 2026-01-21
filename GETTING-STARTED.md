# Getting Started with Gastown

Gastown is a multi-agent orchestration system that coordinates AI coding agents across tasks. This guide covers the core concepts and how to use it.

## Core Concepts

### The Hierarchy

```
Town (~/gt/ or your gastown folder)
├── Mayor         # Your primary AI coordinator - start here
├── Deacon        # Background daemon managing agents
└── Rigs/         # Your projects
    └── myproject/
        ├── Witness    # Monitors the rig (no code changes)
        ├── Refinery   # Processes merges to main
        ├── Crew/      # Your personal workspaces
        │   └── james/ # Where you actually work
        └── Polecats/  # Ephemeral worker agents
```

### Key Terms

| Term | What it is |
|------|------------|
| **Mayor** | The AI coordinator you talk to. It breaks down work and assigns agents. |
| **Rig** | A project (git repo) managed by Gastown |
| **Crew** | Your personal workspace within a rig |
| **Polecat** | An ephemeral worker agent that completes a task and exits |
| **Convoy** | A bundle of related tasks tracked together |
| **Hook** | Git-backed persistent storage that survives agent restarts |
| **Beads** | The issue tracking system (`bd` command) |

### The MEOW Workflow

**M**ayor-**E**nhanced **O**rchestration **W**orkflow:

1. Tell the Mayor what you want to build
2. Mayor analyzes and breaks work into tasks
3. Mayor creates a convoy with issues
4. Mayor spawns appropriate agents (Claude, Gemini, etc.)
5. Issues distribute to agents via hooks
6. Progress tracked through convoy status
7. Mayor summarizes results

## Quick Reference

### Essential Commands

```bash
# Status and info
gt status                    # See town overview
gt doctor                    # Check health
gt agents                    # List active agents

# Working with the Mayor
gt mayor attach              # Start talking to the Mayor
gt mayor detach              # Leave Mayor session

# Managing rigs (projects)
gt rig add <name> <git-url>  # Add a project
gt rig list                  # List all rigs
gt rig remove <name>         # Remove a rig

# Managing crew (your workspaces)
gt crew add <name> --rig <rig>  # Create workspace
gt crew list                    # List workspaces

# Assigning work
gt sling <issue> <rig>                    # Assign with default agent (claude)
gt sling <issue> <rig> --agent gemini     # Use specific agent
gt sling <issue> <rig> --agent gemini-flash  # Use cheap/fast model

# Convoys (task bundles)
gt convoy create <name>      # Create new convoy
gt convoy status             # Check convoy progress
gt convoy list               # List all convoys
```

### Available Agents

```bash
gt config agent list         # See all available agents
```

| Agent | Best For |
|-------|----------|
| `claude` | Complex reasoning, nuanced code, default |
| `claude-tracked` | Same as claude, with usage tracking (estimates) |
| `gemini` | Large context (1M tokens), code analysis |
| `gemini-flash` | Fast, cheap, simple tasks |
| `gemini-pro` | Deep analysis, architecture decisions |
| `gpt4o` | Documentation, scaffolding, moderate tasks |
| `gpt4o-mini` | Ultra-cheap, high-volume simple tasks |
| `perplexity` | Research, fact-checking, API docs lookup |
| `codex` | OpenAI tasks |
| `cursor` | Cursor IDE integration |

### Cost Optimization Guide

| Task Type | Recommended Agent | Why |
|-----------|-------------------|-----|
| Research/facts | `perplexity` | Web search + citations |
| Boilerplate | `gpt4o-mini` | 100x cheaper than Claude |
| Simple refactors | `gemini-flash` | Fast and cheap |
| Documentation | `gpt4o` | Good prose, 6x cheaper |
| Complex logic | `claude` | Best reasoning |
| Large codebase | `gemini-pro` | 1M context window |

## Adding Your First Project

### Option 1: From a Git URL

```bash
cd ~/path/to/gastown
gt rig add myproject https://github.com/you/your-repo.git
gt crew add yourname --rig myproject
```

### Option 2: From a Local Directory

```bash
cd ~/path/to/gastown
gt rig add myproject /path/to/existing/repo
gt crew add yourname --rig myproject
```

### Then Start Working

```bash
# Enter your workspace
cd myproject/crew/yourname

# Or use the Mayor to orchestrate
gt mayor attach
```

## Example Session

```bash
# 1. Check status
$ gt status
Town: gastown
👤 Overseer: james
🎩 mayor    ○
🐺 deacon   ●

# 2. Add a project
$ gt rig add webapp https://github.com/me/webapp.git
Creating rig webapp...
✓ Rig created

# 3. Create your workspace
$ gt crew add james --rig webapp
✓ Created crew workspace: webapp/james

# 4. Talk to the Mayor
$ gt mayor attach
# Now you're in a Claude session with full context
# Tell it: "I need to add user authentication to webapp"
# The Mayor will create issues, spawn agents, track progress

# 5. Or assign work directly
$ gt sling "add login form" webapp --agent gemini-flash
```

## How Work Persists

Gastown's key innovation: **work survives agent restarts**.

Traditional problem:
```
Agent starts → Does work → Crashes → Context lost 😢
```

Gastown solution:
```
Agent starts → Work saved to git hooks → Crashes →
New agent reads hooks → Continues where left off 😊
```

This is called the "Propulsion Principle" - git hooks propel work forward.

## Tips

1. **Always start with the Mayor** - It's designed to be your primary interface
2. **Use gemini-flash for quick tasks** - Save money on simple work
3. **Use convoys for related work** - Track multiple issues together
4. **Check `gt doctor` regularly** - Keeps things healthy
5. **Your crew workspace is your sandbox** - Experiment freely

## Usage Tracking

All custom agents automatically track token usage and costs. View reports with:

**Note:** The `claude-tracked` agent provides **estimated** token counts since Claude Code CLI doesn't expose exact usage. Estimates are based on session duration (interactive mode) or content size (non-interactive mode). All other agents report actual token counts from their APIs.

```bash
# Today's usage summary
./bin/gt-usage

# Last 7 days
./bin/gt-usage week

# Usage for a specific convoy
./bin/gt-usage convoy hq-123

# Usage for a specific rig
./bin/gt-usage rig myproject

# One-line summary (for scripts)
./bin/gt-usage summary
```

### Sample Output

```
═══════════════════════════════════════════════════════════
  Gastown Usage Report - Today (2025-01-16)
═══════════════════════════════════════════════════════════

By Agent:
  claude: 5 requests, 12500+8200 tokens, $0.42
  gemini-flash: 12 requests, 8000+6000 tokens, $0.01
  gpt4o-mini: 8 requests, 4000+3500 tokens, $0.00
  perplexity: 3 requests, 2000+4000 tokens, $0.00

Totals:
  Requests: 28
  Input tokens: 26500
  Output tokens: 21700
  Total cost: $0.43
  Total duration: 342s
```

### Cost Estimates (per 1M tokens)

| Agent | Input | Output | Notes |
|-------|-------|--------|-------|
| claude / claude-tracked | $3.00 | $15.00 | Sonnet pricing (default) |
| claude (opus) | $15.00 | $75.00 | When using Opus model |
| gemini-pro | $1.25 | $5.00 | |
| gemini-flash | $0.075 | $0.30 | |
| gpt4o | $2.50 | $10.00 | |
| gpt4o-mini | $0.15 | $0.60 | |
| perplexity | Free | Free | |

Usage data is stored in `~/.gastown/usage/usage.jsonl`.

## Image Generation

Gastown includes an image generation system using Gemini's image models.

### Quick Start

```bash
# List available projects
./agents/imagen-batch --list

# See variants for a project
./agents/imagen-batch weirdchess --variants

# Generate test images in different styles
./agents/imagen-batch weirdchess --test grand

# Generate all items in a chosen style
./agents/imagen-batch weirdchess --style flat grand

# Generate a single image from a prompt
./agents/imagen --style pixel "a cute robot mascot"
```

### Creating Your Own Project

1. Copy the template:
   ```bash
   cp agents/lib/imagen-projects/_template.sh agents/lib/imagen-projects/myproject.sh
   ```

2. Edit to define your variants and items:
   - `PROJECT_NAME` - Display name
   - `VARIANTS` - Array of variant names
   - `get_items_for_variant()` - Function returning items
   - Items format: `"id|Name|Description for AI"`

3. Generate:
   ```bash
   ./agents/imagen-batch myproject --test myvariant
   ./agents/imagen-batch myproject --style flat myvariant
   ```

### Available Styles

`flat`, `3d`, `pixel`, `geometric`, `medieval`, `neon`, `watercolor`, `classic`, `staunton`

## Troubleshooting

```bash
# Something broken?
gt doctor --fix

# Daemon not running?
gt daemon start

# Need to reset?
gt daemon restart

# See what's happening
gt status
gt agents
```

## More Resources

- `gt --help` - All commands
- `gt <command> --help` - Command-specific help
- [GEMINI-SETUP.md](GEMINI-SETUP.md) - Gemini agent configuration
- [AGENTS.md](AGENTS.md) - Agent system details
