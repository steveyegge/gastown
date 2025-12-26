# Bootstrapping Gas Town from an HQ

This guide documents how to bootstrap a full Gas Town installation from an HQ repository (e.g., `steveyegge/stevey-gt`).

## Prerequisites

- macOS or Linux
- Git configured with SSH access to GitHub
- Homebrew (for macOS)

## Overview

A Gas Town HQ is a template repository containing:
- Town-level configuration (`mayor/`, `.beads/`)
- Rig configs (`gastown/config.json`, `beads/config.json`)
- CLAUDE.md for Mayor context

The HQ does NOT contain:
- The actual gt binary (must be built)
- Full rig structures (must be populated)
- Agent state files (must be created)

## Step 1: Clone the HQ

```bash
git clone git@github.com:steveyegge/stevey-gt.git ~/gt
cd ~/gt
```

## Step 2: Install Go

Gas Town is written in Go. Install it via Homebrew:

```bash
brew install go
go version  # Verify: should show go1.25+
```

## Step 3: Clone Gastown into Mayor's Rig

The gt binary lives in the gastown repository. Clone it into the mayor's rig directory:

```bash
mkdir -p ~/gt/gastown/mayor
git clone git@github.com:steveyegge/gastown.git ~/gt/gastown/mayor/rig
```

## Step 4: Build the gt Binary

```bash
cd ~/gt/gastown/mayor/rig
go build -o gt ./cmd/gt
```

Optionally install to PATH:

```bash
mkdir -p ~/bin
cp gt ~/bin/gt
# Ensure ~/bin is in your PATH
```

## Step 5: Populate Rig Structures

For each rig in your HQ (gastown, beads, etc.), create the full agent structure:

### Gastown Rig

```bash
cd ~/gt/gastown

# Create directories
mkdir -p refinery witness polecats crew/main

# Clone for refinery (canonical main)
git clone git@github.com:steveyegge/gastown.git refinery/rig

# Clone for crew workspace
git clone git@github.com:steveyegge/gastown.git crew/main
```

### Beads Rig

```bash
cd ~/gt/beads

# Create directories
mkdir -p refinery mayor witness polecats crew/main

# Clone for each agent
git clone git@github.com:steveyegge/beads.git refinery/rig
git clone git@github.com:steveyegge/beads.git mayor/rig
git clone git@github.com:steveyegge/beads.git crew/main
```

## Step 6: Create Agent State Files

Each agent needs a state.json file:

```bash
# Gastown agents
echo '{"role": "refinery", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/gastown/refinery/state.json
echo '{"role": "witness", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/gastown/witness/state.json
echo '{"role": "mayor", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/gastown/mayor/state.json

# Beads agents
echo '{"role": "refinery", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/beads/refinery/state.json
echo '{"role": "witness", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/beads/witness/state.json
echo '{"role": "mayor", "last_active": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > ~/gt/beads/mayor/state.json
```

## Step 7: Initialize Town-Level Beads

```bash
cd ~/gt
bd init --prefix gm
```

If there are existing issues in JSONL that fail to import (e.g., due to invalid issue types), you can:
- Fix the JSONL manually and re-run `bd sync --import-only`
- Or start fresh with the empty database

Run doctor to check for issues:

```bash
bd doctor --fix
```

## Step 8: Verify Installation

```bash
cd ~/gt

# Check gt works
gt status

# Check rig structure
gt rig list

# Expected output:
# gastown - Polecats: 0, Crew: 1, Agents: [refinery mayor]
# beads   - Polecats: 0, Crew: 1, Agents: [refinery mayor]
```

## Troubleshooting

### Go not in PATH
If `go` command fails, ensure Homebrew's bin is in your PATH:
```bash
export PATH="/opt/homebrew/bin:$PATH"
```

### Witnesses Not Detected
Known issue (gm-2ej): The witness detection code checks for `witness/rig` but witnesses don't have a git clone. This is a bug in the detection logic - witnesses should still work.

### Beads Import Fails
Known issue (gm-r6e): If your JSONL contains invalid issue types (e.g., "merge-request"), the import will fail. Either fix the JSONL or start with an empty database.

## Full Bootstrap Script

Here's a condensed script for bootstrapping:

```bash
#!/bin/bash
set -e

# Configuration
HQ_REPO="git@github.com:steveyegge/stevey-gt.git"
GASTOWN_REPO="git@github.com:steveyegge/gastown.git"
BEADS_REPO="git@github.com:steveyegge/beads.git"
TOWN_ROOT="$HOME/gt"

# Clone HQ
git clone "$HQ_REPO" "$TOWN_ROOT"
cd "$TOWN_ROOT"

# Install Go if needed
if ! command -v go &> /dev/null; then
    brew install go
fi

# Clone and build gastown
mkdir -p gastown/mayor
git clone "$GASTOWN_REPO" gastown/mayor/rig
cd gastown/mayor/rig
go build -o gt ./cmd/gt
cp gt ~/bin/gt
cd "$TOWN_ROOT"

# Populate gastown rig
cd gastown
mkdir -p refinery witness polecats crew/main
git clone "$GASTOWN_REPO" refinery/rig
git clone "$GASTOWN_REPO" crew/main
echo '{"role": "refinery"}' > refinery/state.json
echo '{"role": "witness"}' > witness/state.json
echo '{"role": "mayor"}' > mayor/state.json
cd "$TOWN_ROOT"

# Populate beads rig
cd beads
mkdir -p refinery mayor witness polecats crew/main
git clone "$BEADS_REPO" refinery/rig
git clone "$BEADS_REPO" mayor/rig
git clone "$BEADS_REPO" crew/main
echo '{"role": "refinery"}' > refinery/state.json
echo '{"role": "witness"}' > witness/state.json
echo '{"role": "mayor"}' > mayor/state.json
cd "$TOWN_ROOT"

# Initialize beads
bd init --prefix gm

# Verify
gt status
echo "Bootstrap complete!"
```

## Next Steps

After bootstrapping:
1. Start a Mayor session: `gt mayor attach`
2. Check for work: `bd ready`
3. Spawn workers with molecules: `gt spawn --issue <id> --molecule mol-shiny`
