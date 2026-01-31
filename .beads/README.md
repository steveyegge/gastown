# Beads - AI-Native Issue Tracking

Welcome to Beads! This repository uses **Beads** for issue tracking - a modern, AI-native tool designed to live directly in your codebase alongside your code.

## What is Beads?

Beads is issue tracking that lives in your repo, making it perfect for AI coding agents and developers who want their issues close to their code. No web UI required - everything works through the CLI and integrates seamlessly with git.

**Learn more:** [github.com/groblegark/beads](https://github.com/groblegark/beads)

## Quick Start

### Essential Commands

```bash
# Create new issues
bd create "Add user authentication"

# View all issues
bd list

# View issue details
bd show <issue-id>

# Update issue status
bd update <issue-id> --status in_progress
bd update <issue-id> --status done

# Sync with git remote
bd sync
```

### Working with Issues

Issues in Beads are:
- **Git-native**: Stored in `.beads/issues.jsonl` and synced like code
- **AI-friendly**: CLI-first design works perfectly with AI coding agents
- **Branch-aware**: Issues can follow your branch workflow
- **Always in sync**: Auto-syncs with your commits

## Why Beads?

✨ **AI-Native Design**
- Built specifically for AI-assisted development workflows
- CLI-first interface works seamlessly with AI coding agents
- No context switching to web UIs

🚀 **Developer Focused**
- Issues live in your repo, right next to your code
- Works offline, syncs when you push
- Fast, lightweight, and stays out of your way

🔧 **Git Integration**
- Automatic sync with git commits
- Branch-aware issue tracking
- Intelligent JSONL merge resolution

## Get Started with Beads

Try Beads in your own projects:

```bash
# Install Beads
curl -sSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash

# Initialize in your repo
bd init

# Create your first issue
bd create "Try out Beads"
```

## Learn More

- **Documentation**: [github.com/groblegark/beads/docs](https://github.com/groblegark/beads/tree/main/docs)
- **Quick Start Guide**: Run `bd quickstart`
- **Examples**: [github.com/groblegark/beads/examples](https://github.com/groblegark/beads/tree/main/examples)

---

## Dolt-Native Sync Mode

This repository uses **Dolt** as the storage backend for version-controlled issue tracking with AWS S3+DynamoDB as the remote.

### Configuration

The following settings in `config.yaml` enable Dolt-native mode:

```yaml
storage-backend: dolt
sync.mode: "dolt-native"
routing.mode: "direct"
```

### Architecture

```
townRoot/.beads/
├── dolt/                    # Dolt database directory
│   ├── beads/              # The actual Dolt repo
│   ├── config.yaml         # Dolt SQL server config
│   └── sql-server.log      # Server logs
├── config.yaml             # Beads config (storage-backend: dolt)
└── beads.db                # SQLite cache (syncs from Dolt)
```

**Remote**: `aws://[pihealth-dolt-beads:pihealth-dolt-beads]/gastown9`

### Daily Workflow

#### How the Daemon Manages Dolt

1. **Connection Management**: The daemon holds a connection to Dolt SQL server
2. **Idle Timeout**: After idle period, daemon releases locks to allow CLI operations
3. **Auto-Restart**: Daemon restarts Dolt SQL server if needed

#### Sync Operations

```bash
# Standard sync (commits to Dolt, pushes to AWS)
bd sync

# Manual Dolt operations (when daemon is idle)
cd $TOWN_ROOT/.beads/dolt/beads
dolt status
dolt commit -m "manual commit"
dolt push origin main
```

### Multi-Prefix Support

Dolt-native mode supports multiple issue prefixes in a single database:

| Prefix | Description |
|--------|-------------|
| `hq-*` | Town-level coordination |
| `bd-*` | Beads tool issues |
| `gt-*` | Gas Town tool issues |

All prefixes coexist in the same Dolt database with:
- Unified listing: `bd list --all`
- Cross-prefix dependencies: `bd dep add hq-123 bd-456`
- Agent bead routing across prefixes

### Troubleshooting

#### "Database is read only" Error

The Dolt LOCK file may be stale:

```bash
# Check for stale LOCK file
ls -la $TOWN_ROOT/.beads/dolt/beads/.dolt/LOCK

# If daemon is not running, safe to remove
rm $TOWN_ROOT/.beads/dolt/beads/.dolt/LOCK

# Restart daemon
bd daemon restart
```

#### Daemon Caching Stale Data

If updates don't appear immediately:

```bash
# Force direct mode (bypass daemon)
bd show <issue-id> --no-daemon

# Or restart daemon
bd daemon restart
```

#### Multi-Agent Lock Contention

When multiple agents access Dolt simultaneously:

1. Daemon uses idle timeout to release locks
2. Each agent should use the daemon (not direct Dolt access)
3. `bd sync` coordinates commits through the daemon

### AWS Remote Setup

The Dolt remote uses AWS S3 for storage and DynamoDB for versioning:

- **S3 Bucket**: `pihealth-dolt-beads`
- **DynamoDB Table**: `pihealth-dolt-beads`
- **IAM User**: `dolt-beads-sync`

Initial setup was done with:

```bash
bd init --backend dolt
# AWS credentials configured via IAM user
dolt remote add origin aws://[bucket:table]/database
```

---

*Beads: Issue tracking that moves at the speed of thought* ⚡
