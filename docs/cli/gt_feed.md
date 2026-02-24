---
title: "DOCS/CLI/GT FEED"
---

## gt feed

Show real-time activity feed of gt events

### Synopsis

Display a real-time feed of issue changes and agent activity.

By default, launches an interactive TUI dashboard with:
  - Agent tree (top): Shows all agents organized by role with latest activity
  - Convoy panel (middle): Shows in-progress and recently landed convoys
  - Event stream (bottom): Chronological feed you can scroll through
  - Vim-style navigation: j/k to scroll, tab to switch panels, 1/2/3 for panels, q to quit

Problems View (--problems/-p):
  A problem-first view that surfaces agents needing attention:
  - Detects stuck agents via structured beads data (hook state, timestamps)
  - Shows GUPP violations (hooked work + 30m no progress)
  - Keyboard actions: Enter=attach, n=nudge, h=handoff
  - Press 'p' to toggle between activity and problems view

The feed combines multiple event sources:
  - GT events: Agent activity like patrol, sling, handoff (from .events.jsonl)
  - Beads activity: Issue creates, updates, completions (from bd activity, when available)
  - Convoy status: In-progress and recently-landed convoys (refreshes every 10s)

Use --plain for simple text output (reads .events.jsonl directly).

Tmux Integration:
  Use --window to open the feed in a dedicated tmux window named 'feed'.
  This creates a persistent window you can cycle to with C-b n/p.

Event symbols:
  +  created/bonded    - New issue or molecule created
  →  in_progress       - Work started on an issue
  ✓  completed         - Issue closed or step completed
  ✗  failed            - Step or issue failed
  ⊘  deleted           - Issue removed
  🦉  patrol_started   - Witness began patrol cycle
  ⚡  polecat_nudged   - Worker was nudged
  🎯  sling            - Work was slung to worker
  🤝  handoff          - Session handed off

Agent state symbols (problems view):
  🔥  GUPP violation   - Hooked work + 30m no progress (critical)
  ⚠   STALLED          - Hooked work + 15m no progress
  ●   Working          - Actively producing output
  ○   Idle             - No hooked work
  💀  Zombie           - Dead/crashed session

MQ (Merge Queue) event symbols:
  ⚙  merge_started   - Refinery began processing an MR
  ✓  merged          - MR successfully merged (green)
  ✗  merge_failed    - Merge failed (conflict, tests, etc.) (red)
  ⊘  merge_skipped   - MR skipped (already merged, etc.)

Examples:
  gt feed                       # Launch TUI dashboard
  gt feed --problems            # Start in problems view
  gt feed -p                    # Short flag for problems view
  gt feed --plain               # Plain text output (bd activity)
  gt feed --window              # Open in dedicated tmux window
  gt feed --since 1h            # Events from last hour
  gt feed --rig greenplace      # Use gastown rig's beads

```
gt feed [flags]
```

### Options

```
  -f, --follow         Stream events in real-time (default when no other flags)
  -h, --help           help for feed
  -n, --limit int      Maximum number of events to show (default 100)
      --mol string     Filter by molecule/issue ID prefix
      --no-follow      Show events once and exit
      --plain          Use plain text output (bd activity) instead of TUI
  -p, --problems       Start in problems view (shows stuck agents)
      --rig string     Filter events by rig name
      --since string   Show events since duration (e.g., 5m, 1h, 30s)
      --type string    Filter by event type (create, update, delete, comment)
  -w, --window         Open in dedicated tmux window (creates 'feed' window)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

