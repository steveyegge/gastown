# Medici Collision: Domain (run-permit lease) × Constraints (ESTOP sentinel)

## The Tension

Both lenses agree on file-based signaling as the mechanism. They disagree on
polarity:

- **Domain** proposes a **run-permit lease**: agents need a continuously-renewed
  `run.permit` file to keep working. Absence = stop. Fail-safe by default.
  Daemon crash = implicit E-stop because the permit goes stale.

- **Constraints** proposes an **ESTOP sentinel**: agents check for the presence
  of an `ESTOP` file. Presence = stop. Simplest possible signal — `touch ESTOP`.
  But absence of the file is the normal state, so failure of the signaling
  mechanism = continued operation.

## What Each Side Correctly Sees

**Domain is right that** the de-energize-to-safe principle is the gold standard.
A system where failure of the safety mechanism itself causes a safe state is
fundamentally more robust. If the daemon crashes, the permit goes stale, and
agents stop — no human intervention needed. This catches failure modes that a
sentinel file cannot: if no one is around to `touch ESTOP`, the system
self-protects.

**Constraints is right that** simplicity wins in crisis. `touch ~/Documents/gt/ESTOP`
is something a panicked human can type from memory. A permit-lease system adds
complexity: agents must parse timestamps, handle clock skew edge cases, and the
daemon must reliably write on a tight schedule. The lease duration must be
calibrated (too short = false stops during daemon GC pauses; too long = delayed
response). The sentinel file has zero configuration.

## What Each Side Is Missing

**Domain misses** that a lease-based system can cause false E-stops under normal
conditions. If the daemon is busy (long GC, heavy heartbeat processing, stuck on
a Dolt query during its own health check), it may miss a lease renewal window.
Agents would park for no real reason. The adversary lens flagged this: the E-stop
itself causing work stoppage is a serious failure mode.

**Constraints misses** that a sentinel file is exclusively a manual mechanism.
No automated trigger can create it without the daemon being healthy enough to
detect the failure and write the file — which is the same daemon that might be
hung. The sentinel file solves the "human wants to stop everything" case but
does NOT solve the "no human is watching and Dolt crashes at 3am" case.

## Recommendation: Hybrid — Sentinel + Health Gate

Use BOTH mechanisms, at different layers:

1. **ESTOP sentinel file** (`~/Documents/gt/ESTOP`) — the manual E-stop.
   Human touch, instant effect. Daemon checks on heartbeat; agents check on
   poll. This is the factory floor red button.

2. **Health gate in daemon** — the automatic circuit breaker. Daemon monitors
   Dolt health (already does heartbeat checks). After N consecutive failures
   over M seconds, daemon CREATES the ESTOP file itself, with a marker
   indicating it was auto-triggered (`ESTOP` file contains
   `auto:dolt-unreachable:2026-03-24T14:23:07`). This gives you the
   de-energize-to-safe property through the daemon, while keeping the signal
   mechanism dead simple.

3. **Auto-resume probe** — after auto-triggered ESTOP, daemon periodically
   probes (SELECT 1). On recovery, it removes the ESTOP file (but only if
   it was auto-triggered, not manual). Manual ESTOP always requires manual
   `gt resume`.

This preserves:
- Constraints' simplicity: one file, one check, works everywhere
- Domain's fail-safe property: daemon crash = can't remove ESTOP = stays stopped
  (if auto-triggered during the failure that crashed the daemon)
- The human override: `touch ESTOP` always works, even if daemon is dead

The one gap: if the daemon dies WITHOUT having created ESTOP (e.g., killed by
OOM before it could detect Dolt failure), agents continue running. This is
accepted as a residual risk — the alternative (lease-based) has worse false
positive characteristics.
