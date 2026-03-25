# Constraints Lens: Gas Town Architecture Realities
## Issue: gt-d3n — Emergency Stop (E-stop)

### 1. Framing the Problem

The E-stop problem is fundamentally a question of what remains trustworthy when
the system is in distress. The primary failure trigger (Dolt down) eliminates the
most capable communication substrate. What's left is: the filesystem, the OS
process table, and tmux session existence.

The daemon is the load-bearing structure here. It has a 30-second heartbeat loop,
it knows about every agent session (via tmux and PID files), and it does not
depend on Dolt for its own operation. Any E-stop design that routes through the
daemon survives Dolt failure by construction — but it must survive daemon failure
too, since the daemon could itself be the hung or runaway process.

The signal path therefore has two regimes that must both be designed for:
- Daemon alive: signal daemon -> daemon propagates to all agents via existing
  session/PID knowledge
- Daemon dead or hung: signal must bypass daemon and reach agents directly

The constraint is not "how do we stop agents" — park/dock/stop already exist.
The constraint is "how do we trigger that stop reliably from a single gesture,
across both regimes, without any coordination infrastructure that might itself
be broken."

### 2. What Other Lenses Will Likely Miss

Other lenses (UX, reliability, protocol) will likely focus on the signal *content*
— what message gets sent, how agents acknowledge it, whether there's a handshake.
They will probably propose a structured protocol: a Dolt record, a mail message,
an event file with a defined schema.

What they'll miss: **the filesystem is not atomic across writers, but it IS
readable when nothing else is**. A single sentinel file — a flag drop — requires
no protocol, no acknowledgment, and no infrastructure. Every agent that polls
its heartbeat directory can check for this file on every cycle. The daemon can
check it too. No writes needed by agents; they only need to read.

The constraint architecture already has a pattern for this: agents check for
park/dock signals in files. The E-stop is just a system-wide version of the same
pattern that every component already knows how to handle. The insight is that
the existing heartbeat/lifecycle machinery is already close to what's needed —
it just lacks a "check this one global file" step in each agent's loop.

### 3. Proposed Solution

A single sentinel file at a well-known, stable path: `~/Documents/gt/ESTOP`.

- **To trigger**: `touch ~/Documents/gt/ESTOP` — works from any shell, any agent,
  any script, even a panicked human at a terminal.
- **Daemon behavior**: on each 30s heartbeat iteration, check for ESTOP file
  existence. If present, invoke stop on all known agent sessions immediately
  (using the same PID/tmux knowledge it already has), then park itself.
- **Agent behavior**: each agent's heartbeat loop (or equivalent poll cycle)
  checks for the ESTOP file. If present, the agent saves state if safe, then
  exits. No Dolt, no tmux send-keys, no network.
- **Human direct path**: if daemon is hung, the human runs
  `touch ~/Documents/gt/ESTOP && pkill -f "gt daemon"` — the daemon is killed,
  and any agents that wake up next will see the file and stop. Agents that never
  wake up can be killed by a follow-up `pkill` or the existing `gt stop` tooling.
- **Recovery**: remove the file. The daemon restart procedure already exists.

This works in the Dolt-down case because it touches nothing in Dolt. It works in
the daemon-down case because agents read the file themselves. It works for the
human because it's a single filesystem operation.

### 4. Failure Mode

**Agents that are not in a polling loop cannot see the file.**

Polecats running a long blocking operation (a shell command, a subprocess, a
network call) will not check the ESTOP file until that operation returns. A
polecat that is hung waiting on a Dolt query will never return from that query,
so it will never see the file.

This is the core tension: the agents most likely to need stopping (those stuck
on Dolt) are precisely the ones least likely to poll the filesystem. The file
signal works for well-behaved agents in their normal cycles; it does not work
for wedged agents.

The fallback for wedged agents is OS-level: the daemon (if alive) or a human
uses the PID files to send SIGTERM or SIGKILL. This is not elegant, but it is
reliable. The ESTOP file approach handles the cooperative case; OS signals handle
the uncooperative case. Any complete E-stop design needs both layers.

### 5. Cheap Experiment

**Test the sentinel file visibility across the existing heartbeat infrastructure
without writing any new code.**

Manually touch `~/Documents/gt/ESTOP-TEST` and then check whether any existing
daemon log or agent output mentions it within two heartbeat cycles (60 seconds).
This tells you whether the current heartbeat loop is reading from the town root
directory at all, and at what frequency.

If nothing notices the file, you've confirmed the gap: the heartbeat loop does
not scan the town root. The cost of the fix is then scoped to "add one
`os.Stat` call to the daemon loop and one to each agent's cycle" — a change
small enough to be safe and reviewable in isolation.

If something does notice it, you've discovered latent polling behavior that the
E-stop design can build on for free.
