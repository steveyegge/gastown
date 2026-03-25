# Medici Brief: Emergency Stop (E-stop) for Gas Town

## Core Problem

When critical infrastructure fails (Dolt server crash, disk full, network
outage), all coding agents continue burning tokens on work that will fail or
need to be redone. There is no mechanism to immediately halt agent activity
across the town. The overseer observed a 2.1GB Dolt log causing cascading
failures while agents kept working and failing throughout.

## Who Is Affected

- **Overseer**: Watches agents burn money on doomed work, must manually kill
  sessions one by one.
- **Agents (crew + polecats)**: Waste context windows on operations that fail,
  then need fresh sessions anyway.
- **The system**: Cascading failures compound (e.g., Dolt down -> bd commands
  fail -> agents retry -> more failures -> log grows further).

## Why This Is Hard / Cross-Cutting

1. **Must work when the failure IS the data plane.** Dolt being down is a
   primary trigger, but beads/mail/molecules all depend on Dolt. The E-stop
   mechanism cannot rely on what it's protecting against.
2. **Graceful vs immediate.** Hard-killing agents loses in-flight work. But
   waiting for graceful checkpoint may take minutes while tokens burn.
3. **Multiple coordination layers.** Daemon heartbeat, tmux sessions, rig
   lifecycle (park/dock/stop), molecules, merge queue — the stop signal must
   propagate through all of them.
4. **Resume state.** After E-stop, agents need to know what was interrupted
   and where to pick up. This is a distributed checkpoint problem.
5. **Automatic triggers.** Manual E-stop is necessary but insufficient. Some
   failures (disk 95%, Dolt unreachable) should trigger automatically.

## Known Constraints

- Existing rig lifecycle: `park` (temporary), `dock` (persistent), `stop`
  (immediate) — E-stop needs to relate to these but be distinct (town-wide,
  not per-rig).
- Daemon heartbeat loop runs every 30s — could be the propagation mechanism.
- Agents receive nudges via tmux send-keys — this works even when Dolt is down.
- File-based signals (touch a file) are the most reliable IPC when everything
  else is broken.
- The daemon is the only always-running process. It's the natural E-stop
  controller.

## What Success Looks Like

- `gt estop` halts all agent activity within 60 seconds
- Agents checkpoint before stopping (best-effort, with hard timeout)
- Clear visual indicator (tmux status bar, CLI output) shows E-stop state
- `gt resume` brings the town back online
- Automatic triggers for known failure modes (configurable)
- Zero token waste during known-broken infrastructure

## Is Medici Warranted?

Yes. This touches daemon, tmux, rig lifecycle, agent protocol, CLI UX, and
monitoring. Multiple design tensions exist: graceful vs fast, automatic vs
manual, per-rig vs town-wide, file-based vs process-based signaling. Different
perspectives will produce meaningfully different designs.
