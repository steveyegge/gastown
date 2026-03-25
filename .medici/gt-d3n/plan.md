# Implementation Plan: E-stop (gt-d3n)

## Chosen Approach: Option B — Sentinel + Auto-Trigger + Recovery Probe

## Implementation Beads

### Phase 1: Manual E-stop (ships first, unblocks everything else)

1. **gt-d3n.1**: `gt estop` and `gt resume` CLI commands
   - `gt estop` creates `<townRoot>/ESTOP` with metadata (manual, timestamp)
   - `gt resume` removes it (only if exists, with confirmation)
   - `gt status` shows E-stop state
   - Estimated: 1-2 hours

2. **gt-d3n.2**: Daemon heartbeat checks ESTOP file
   - On each 30s heartbeat loop, `os.Stat` the ESTOP file
   - If present: stop all agent sessions (tmux kill + PID cleanup)
   - Park all rigs (prevent daemon from restarting agents)
   - Log the E-stop event
   - Estimated: 1-2 hours

3. **gt-d3n.3**: Visual indicator — red tmux status bar
   - When ESTOP file exists, tmux status bar turns red with timestamp
   - Wired into existing status bar rendering
   - Clear visual: "ESTOP 14:23" in red
   - Estimated: 1 hour

### Phase 2: Auto-trigger (daemon-driven)

4. **gt-d3n.4**: Dolt health monitoring in daemon
   - Track consecutive Dolt connection failures in daemon heartbeat
   - Configurable threshold: N failures over M seconds (default: 5/90s)
   - When threshold hit: create ESTOP file with `auto:<reason>:<timestamp>`
   - Estimated: 2 hours

5. **gt-d3n.5**: Recovery probe
   - When ESTOP is auto-triggered, daemon probes every 60s (`SELECT 1`)
   - On success: remove ESTOP file, unpark rigs, log recovery
   - On failure: keep ESTOP, reset probe timer
   - Manual ESTOP never auto-resumes (metadata distinguishes)
   - Estimated: 2 hours

### Phase 3: Polish

6. **gt-d3n.6**: Agent-side ESTOP check (defense-in-depth)
   - Agents check for ESTOP file in their UserPromptSubmit hook
   - If ESTOP exists: agent logs checkpoint and exits gracefully
   - Catches agents that survive daemon's tmux kill
   - Estimated: 1 hour

7. **gt-d3n.7**: Configuration
   - `daemon.json` settings for auto-trigger thresholds
   - `gt estop --dry-run` to test without actually stopping
   - `gt estop --status` for current E-stop state details
   - Estimated: 1 hour

## Dependencies

```
gt-d3n.1 (CLI commands) ← no deps, start here
gt-d3n.2 (daemon check) ← depends on gt-d3n.1 (needs ESTOP file format)
gt-d3n.3 (visual)       ← depends on gt-d3n.1 (needs ESTOP file)
gt-d3n.4 (auto-trigger) ← depends on gt-d3n.2 (daemon already checking)
gt-d3n.5 (probe)        ← depends on gt-d3n.4 (auto-trigger creates the file)
gt-d3n.6 (agent check)  ← depends on gt-d3n.1 (needs ESTOP file format)
gt-d3n.7 (config)       ← depends on gt-d3n.4 (configures thresholds)
```

## Parallel Execution

- Phase 1: gt-d3n.1 first, then gt-d3n.2 and gt-d3n.3 in parallel
- Phase 2: gt-d3n.4 then gt-d3n.5 (sequential)
- Phase 3: gt-d3n.6 and gt-d3n.7 in parallel (after Phase 1)

## What to Do First

Start with gt-d3n.1 (CLI commands). It's the foundation and delivers immediate
value — the human can `gt estop` right now even before daemon integration.
