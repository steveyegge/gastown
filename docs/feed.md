# GT Feed

`gt feed` is the operator and agent event viewer for Gas Town.

Use it to answer:
- What is happening right now?
- Which agents are stalled?
- Did a convoy or merge queue step fail?

Goal: operators and agents should not need to inspect tmux panes or raw logs for normal coordination.

## Quick Start

```bash
# Interactive dashboard (default in TTY)
gt feed

# Problem-first view (best default for triage)
gt feed --problems

# Snapshot for scripts/agents (prints and exits)
gt feed --plain --no-follow --limit 100

# Filter by actor and free text
gt feed --actor gastown/crew --contains conflict
```

## Views

### Activity View

Default 3-panel layout:
- Agent tree (top): latest activity by rig/role/agent.
- Convoy panel (middle): in-progress and recently landed convoys.
- Event stream (bottom): chronological event log.

### Problems View

Use `gt feed --problems` (or press `p` in TUI).

This mode prioritizes agents needing attention:
- `🔥` GUPP violation: hooked work and no progress for 30m+.
- `⚠` stalled: hooked work and no progress for 15m+.
- `💀` zombie: dead/crashed session.

Actions:
- `Enter`: attach to selected agent session.
- `n`: nudge selected agent.
- `h`: handoff selected agent.

## Common Agent Recipes

```bash
# "What changed in the last hour?"
gt feed --since 1h

# "Show merge queue failures only"
gt feed --type merge_failed --since 30m

# "Focus on one convoy/molecule"
gt feed --mol hq-cv-abc

# "Show only one actor's events"
gt feed --actor gastown/crew/claude

# "Search all event text for a keyword"
gt feed --contains conflict

# "Watch another rig live"
gt feed --rig greenplace
```

## Filter Reference

Query filters use plain output mode automatically:
- `--since <duration>`: time window (for example `30m`, `2h`).
- `--type <event_type>`: event type match (case-insensitive).
- `--mol <id_fragment>`: molecule/issue ID substring match.
- `--actor <actor_fragment>`: actor substring match (case-insensitive).
- `--contains <text>`: text search across message, target, actor, and raw event line.
- `--limit <n>`: max rows.
- `--follow` / `--no-follow`: streaming behavior.

Examples:

```bash
gt feed --actor refinery --type merge_failed --since 2h
gt feed --contains "gt-abc12" --since 24h --limit 200
```

## Agent SOP

When you need to understand what's happening:
1. Run `gt feed --problems`.
2. Run a scoped query for context (`gt feed --since 1h --contains <id-or-keyword>`).
3. Only after feed evidence should you attach/nudge/handoff.

Prefer feed-based diagnosis over tmux/log inspection for routine triage.

## Do/Don't

- Do: `gt feed --problems` for first-pass health checks.
- Do: `gt feed --actor <agent> --since 1h` before nudging someone.
- Do: `gt feed --contains <issue-or-convoy-id>` when tracing one work item.
- Don't: start by reading tmux panes or raw logs for normal coordination.

## Tmux Workflow

```bash
# Open persistent feed window named "feed"
gt feed --window
```

If the window already exists, Gas Town switches to it.

## Event Legend (Core)

- `+` created/bonded
- `→` in_progress
- `✓` completed/merged
- `✗` failed/merge_failed
- `⊘` deleted/skipped
- `🦉` patrol_started
- `⚡` polecat_nudged
- `🎯` sling
- `🤝` handoff

## Notes

- `--plain` is safer for non-interactive agent workflows.
- `--follow` is default in TTY unless `--no-follow` is set.
- Use `--rig` to scope events to another rig.

## Event Emission Contract

For command contributors:
- If a command changes workflow state (dispatch, handoff, stop/start, escalation, merge), emit a feed-visible event.
- Prefer structured payloads via `internal/events` helper constructors.
- Keep event logging best-effort (never fail command execution solely because logging failed).
- Add/update `gt feed` docs/examples when introducing new high-signal event types.
