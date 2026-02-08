---
name: handoff
description: >
  Hand off to a fresh Claude session. Use when context is full, you've finished
  a logical chunk of work, or need a fresh perspective. Work continues from hook.
allowed-tools: "Bash(gt handoff:*),Bash(gt mail send:*)"
version: "1.0.0"
author: "Gas Town"
---

# Handoff - Session Cycling for Gas Town Agents

Hand off your current session to a fresh Claude instance while preserving work context.

## When to Use

- Context getting full (approaching token limit)
- Finished a logical chunk of work
- Need a fresh perspective on a problem
- Human requests session cycling

## Usage

```
/handoff [optional message]
```

## How It Works

1. State is collected (your message, or auto-collected inbox/beads/hook status)
2. Handoff mail is sent to yourself with context
3. `gt handoff` respawns your session with a fresh Claude
4. New session auto-primes via SessionStart hook
5. Work continues from your hook (pinned molecule persists)

## Examples

```bash
# Simple handoff - auto-collects state (inbox, ready beads, hooked work)
/handoff

# Handoff with context notes
/handoff "Found the bug in token refresh - check line 145 in auth.go first"
```

## What Persists

- **Hooked molecule**: Your work assignment stays on your hook
- **Beads state**: All issues, dependencies, progress
- **Git state**: Commits, branches, staged changes

## What Resets

- **Conversation context**: Fresh Claude instance
- **TodoWrite items**: Ephemeral, session-scoped
- **In-memory state**: Any uncommitted analysis

## Implementation

When invoked, execute:

1. If user provided a message:
   ```bash
   gt handoff -m "<message>"
   ```

2. If NO message provided, use `-c` to auto-collect state:
   ```bash
   gt handoff -c
   ```

**IMPORTANT**: Never run bare `gt handoff` - it sends no mail and the next session loses context. Always use `-c` or `-m`.
