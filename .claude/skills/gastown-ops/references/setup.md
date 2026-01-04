# Gastown Setup

## Official Documentation

**Always refer to the source repos for current installation instructions:**

- **Gas Town (gt):** https://github.com/steveyegge/gastown
- **Beads (bd):** https://github.com/steveyegge/beads

## Quick Verification

Check if already installed:

```bash
which gt bd
gt --version
bd --version
```

## Prerequisites Summary

| Requirement | Why |
|-------------|-----|
| Go 1.23+ | For installing gt/bd |
| Git 2.25+ | Worktree functionality |
| tmux 3.0+ | Full automation (optional for manual mode) |
| Claude Code | Agent execution |

## Typical Initialization Flow

```bash
# 1. Install tools (see repos for current commands)
go install github.com/steveyegge/gastown/cmd/gt@latest
go install github.com/steveyegge/beads/cmd/bd@latest

# 2. Create town
gt install ~/gt

# 3. Initialize beads
bd init

# 4. Register a rig
gt rig add <name> --remote=<git-url>

# 5. Verify
gt status
gt rigs
```

## Environment

```bash
export PATH="$HOME/go/bin:$PATH"
export BEADS_DIR="$HOME/gt/<rig>/.beads"
```

## Troubleshooting

See the official repos for current troubleshooting guides:
- Gastown: https://github.com/steveyegge/gastown#troubleshooting
- Beads: https://github.com/steveyegge/beads/blob/main/TROUBLESHOOTING.md
