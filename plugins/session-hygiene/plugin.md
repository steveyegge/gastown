+++
name = "session-hygiene"
description = "Clean up zombie tmux sessions and orphaned dog sessions"
version = 2

[gate]
type = "cooldown"
duration = "30m"

[tracking]
labels = ["plugin:session-hygiene", "category:cleanup"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "low"
+++

# Session Hygiene

Deterministic cleanup of zombie tmux sessions and orphaned dog sessions.
Executed via `run.sh` — no AI interpretation of destructive cleanup logic.

## What it does

1. Reads `rigs.json` for valid rig names and beads prefixes
2. Lists all tmux sessions and checks each session's prefix against valid prefixes
3. Kills sessions whose prefix doesn't match any known rig or `hq`
4. Cross-references `hq-dog-*` sessions against the kennel, kills orphans

## Why run.sh (not AI-interpreted plugin.md)

This plugin was converted from AI-interpreted markdown to a deterministic shell
script after two incidents where the AI dog ignored plugin instructions and used
`gt rig list --json` (which returns rig names like "gastown") instead of reading
`rigs.json` (which contains beads prefixes like "gt"). This caused all crew
sessions to be misidentified as zombies and killed.

## Usage

```bash
./run.sh              # Normal execution
./run.sh --dry-run    # Report without killing
```
