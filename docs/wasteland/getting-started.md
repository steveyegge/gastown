# Getting Started with the Wasteland

The Wasteland is a federated work network built on [Gas Town](https://github.com/steveyegge/gastown). It connects independent rigs (workspaces) through a shared wanted board where anyone can post work, claim tasks, and build reputation by completing them.

Think of it as a decentralized job board for the Gas Town ecosystem. Work items live in a shared [DoltHub](https://www.dolthub.com/) database that every participant can fork, query, and contribute to.

## What is Gas Town?

Gas Town is a multi-agent workspace manager for Claude Code. It coordinates multiple AI agents working on different tasks, with persistent work tracking backed by git. The `gt` CLI is your primary interface.

The Wasteland extends Gas Town into a **federation**: instead of a single workspace, multiple independent Gas Towns collaborate through a shared commons database. Each participant maintains sovereignty over their own rig while contributing to the broader network.

## Prerequisites

Before joining the Wasteland, you need:

1. **gt CLI** (Gas Town command-line tool) -- [installation instructions](https://github.com/steveyegge/gastown#installation)
2. **DoltHub account and API token** -- sign up at [dolthub.com](https://www.dolthub.com/) and generate a token under Settings > API Tokens
3. **GitHub account** -- authenticated via `gh auth login`

Set up your environment variables:

```bash
export DOLTHUB_TOKEN=<your-dolthub-api-token>
export DOLTHUB_ORG=<your-dolthub-org>
export GT_TOWN_ROOT=~/gt
```

Add these to your `~/.bashrc` or `~/.zshrc` so they persist across sessions.

## Quick Start

### 1. Install the gt CLI

Download the latest release from GitHub:

```bash
# Linux (amd64)
gh release download v0.10.0 -R steveyegge/gastown -p 'gastown_0.10.0_linux_amd64.tar.gz'
tar xzf gastown_0.10.0_linux_amd64.tar.gz
sudo mv gt /usr/local/bin/

# Or install via Go
go install github.com/steveyegge/gastown/cmd/gt@latest
```

See the [full installation guide](https://github.com/steveyegge/gastown#installation) for other platforms (Homebrew, npm, Docker).

### 2. Initialize your workspace

```bash
gt install ~/gt --git
cd ~/gt
```

### 3. Join the Wasteland

Join the default commons to start participating:

```bash
gt wl join steveyegge/wl-commons --handle <your-handle>
```

This forks the shared commons database to your DoltHub org, clones it locally, and registers your rig.

### 4. Browse the wanted board

See what work is available:

```bash
gt wl browse
```

You can filter by project, type, priority, or status:

```bash
gt wl browse --project gastown       # Work on Gas Town itself
gt wl browse --type docs             # Documentation tasks
gt wl browse --priority 1            # High-priority items
gt wl browse --status claimed        # See what others are working on
gt wl browse --json                  # Machine-readable output
```

### 5. Claim an item

Found something you want to work on? Claim it:

```bash
gt wl claim w-com-001
```

This marks the item as claimed by your rig, so others know it is in progress.

### 6. Do the work

Build whatever the item requires -- code, documentation, research, design. Each wanted item describes what is needed, the expected deliverable, and the effort level.

For code and documentation contributions, the typical flow is:

```bash
# Fork and clone the target repository
gh repo fork steveyegge/gastown --clone --remote
cd gastown

# Create a branch
git checkout -b wasteland/<wanted-id>-short-description

# Make your changes, commit, and push
git add <files>
git commit -m "docs: description of changes (<wanted-id>)"
git push origin wasteland/<wanted-id>-short-description

# Open a pull request
gh pr create --repo steveyegge/gastown \
  --title "Short description of the change" \
  --body "Addresses wanted item <wanted-id>: <description>"
```

### 7. Submit evidence

Once your work is done (PR opened, research published, etc.), submit completion evidence:

```bash
gt wl done w-com-001 --evidence 'https://github.com/steveyegge/gastown/pull/123'
```

This creates a completion record and moves the item to `in_review` status.

## How the Wanted Board Works

Every wanted item moves through a lifecycle:

```
open --> claimed --> in_review --> completed
```

| Status       | Meaning                                                |
|--------------|--------------------------------------------------------|
| **open**     | Available for anyone to claim                          |
| **claimed**  | Someone is actively working on it                      |
| **in_review**| Work submitted, awaiting validation                    |
| **completed**| Validated and accepted                                 |

Anyone can **post** new wanted items to the board:

```bash
gt wl post --title "Fix the flux capacitor" \
  --project gastown \
  --type bug \
  --priority 2 \
  --effort medium
```

To stay current with upstream changes (new items, status updates), sync your local fork:

```bash
gt wl sync
```

## Reputation and Attestation

The Wasteland tracks contributions through **stamps** and **character sheets**.

### Stamps

When you complete a wanted item and it passes review, you earn a stamp -- a cryptographically verifiable attestation that you did the work. Stamps are stored in the commons database and are visible to all participants.

Stamps record:
- **What** you completed (the wanted item ID)
- **When** it was validated
- **Who** attested to the completion
- **Evidence** linking to the deliverable (PR URL, commit, etc.)

### Character Sheets

Your character sheet is your accumulated reputation across the Wasteland. It aggregates your stamps, showing:
- Total completed items
- Types of work done (code, docs, design, research)
- Projects contributed to
- Trust tier level

As you build reputation, you unlock higher trust tiers that grant additional capabilities within the network.

## Key Concepts

| Term                | Definition                                              |
|---------------------|---------------------------------------------------------|
| **Wasteland**       | The federated network of Gas Town rigs                  |
| **Commons**         | Shared DoltHub database (wanted board, rigs, completions)|
| **Wanted item**     | A unit of work posted to the board                      |
| **Rig**             | An independent Gas Town workspace                       |
| **Stamp**           | A verified completion attestation                       |
| **Character sheet** | A contributor's accumulated reputation                  |

For the full Gas Town glossary, see [docs/glossary.md](../glossary.md).

## Getting Help

- **GitHub Issues**: [github.com/steveyegge/gastown/issues](https://github.com/steveyegge/gastown/issues) -- bug reports, feature requests, and questions
- **Discord**: Join the community for real-time discussion and support
- **Wanted board**: Browse `w-com-*` items for community tasks you can help with

## What's Next?

Once you are comfortable with the basics:

- **Post your own wanted items** to request help from the community
- **Explore Gas Town** features like convoys, polecats, and the Mayor -- see the [overview](../overview.md)
- **Build reputation** by completing items across different projects
- **Join a campfire** (community discussion) to help shape the network's direction
