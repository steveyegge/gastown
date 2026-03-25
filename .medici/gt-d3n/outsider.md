# Lens: Outsider — Circuit Breakers in Distributed Systems

**Issue:** gt-d3n — Emergency Stop (E-stop)
**Date:** 2026-03-24

---

## 1. Framing

Distributed systems solved this problem in 2012 (Netflix/Hystrix) and the pattern is now table stakes: when a downstream dependency becomes unavailable, callers must stop calling it. Not slow down. Stop. The circuit opens. This isn't about protecting the caller — it's about protecting the dependency. A thrashing service receiving 10,000 retry requests cannot recover. The circuit breaker's job is to give the failed component silence in which to heal.

Gas Town's situation maps cleanly: Dolt is the dependency. Every agent session is a caller. When Dolt fails, agents don't know to stop — they retry, they pile up, they generate more Dolt load via escalations that themselves write to Dolt, which makes recovery harder. The coordination plane becomes the blast radius amplifier.

The local-machine framing makes this *more* tractable than cloud, not less. In cloud, circuit breakers are distributed state machines with consensus problems. Here, a single file on disk (`~/.gt/circuit.json`) can hold the state. There are no network partitions between the circuit breaker and its readers. The hardest part is already solved.

---

## 2. What Other Lenses Will Likely Miss

**The half-open state is the most important state, and it's almost always omitted from initial designs.**

A naive E-stop is binary: on or off. Chaos engineering and Erlang supervisors both know that the hard problem isn't stopping — it's *probing for recovery*. Hystrix's half-open state allows exactly one request through to test whether the dependency has recovered before allowing full traffic. Without this, you get one of two failure modes: the circuit never re-closes automatically (human must intervene every time), or it re-closes too eagerly and immediately re-triggers the failure.

Gas Town's equivalent: after Dolt is stopped or fails, some agent needs to be the designated "probe" — it sends a single lightweight query (e.g., `SELECT 1`), waits for a response, and only if that succeeds does it broadcast an all-clear. Every other agent stays silent during this probe window.

Other lenses will focus on the stop. This lens insists that the recovery path is harder to design correctly and must be specified upfront.

---

## 3. Proposed Solution

**A file-based circuit breaker with three states, read by all agents before any Dolt operation.**

State file: `~/.gt/circuit.state` (plaintext: `CLOSED`, `OPEN`, or `HALF_OPEN`)

- **CLOSED** (normal): agents proceed with Dolt operations
- **OPEN** (Dolt down): agents skip all Dolt writes; they buffer or drop non-critical operations; escalations go to a flat file (`~/.gt/escalation-buffer.log`) instead
- **HALF_OPEN** (recovery probe): one designated agent (the Mayor, or whoever holds a probe lock file) attempts a single `SELECT 1`; all others treat this as OPEN

Transition rules:
- CLOSED → OPEN: any agent that sees 3 consecutive Dolt timeouts within 30 seconds writes OPEN and nudges all others
- OPEN → HALF_OPEN: after a configured quiet period (default: 60 seconds), a cron-like watchdog (simple `gt dolt probe` command) flips to HALF_OPEN
- HALF_OPEN → CLOSED: probe succeeds; watchdog writes CLOSED, flushes escalation buffer in order
- HALF_OPEN → OPEN: probe fails; watchdog writes OPEN, resets the quiet-period timer

This requires no new infrastructure. It requires only that agents check the file before each Dolt call. The entire state machine is ~50 lines of Go or shell.

---

## 4. Failure Mode

**The circuit breaker itself becomes a coordination failure point.**

If two agents simultaneously detect Dolt failure and both try to write `OPEN` to the state file, the last writer wins — this is fine, they agree. But if one agent writes `HALF_OPEN` (probe starting) and another agent simultaneously writes `OPEN` (it just saw another failure), the probe is aborted and the timer resets. Under heavy agent load during a Dolt failure, this race can cause indefinite oscillation: the circuit never stays in HALF_OPEN long enough for a probe to complete.

The fix is a probe lock file (`~/.gt/circuit.probe.lock`) with a PID and timestamp — agents only flip to OPEN if the lock is stale (holder process is dead). But now you have a locking protocol layered on top of a state machine, and locking protocols have their own failure modes (stale locks from crashed agents, etc.).

The deeper failure mode: **the E-stop mechanism requires inter-agent coordination, and the coordination plane (Dolt) is what's broken.** This is the fundamental circularity. A file-based approach avoids Dolt, but files are not atomic across processes on macOS without advisory locks (`flock`), which require the locking process to be alive. This is solvable but must be designed explicitly — it cannot be an afterthought.

---

## 5. Cheap Experiment

**Chaos injection: manually flip the circuit to OPEN for 5 minutes during a normal work session and observe agent behavior without fixing anything.**

Procedure:
1. Write `OPEN` to the state file
2. Do not touch Dolt — it's still running
3. Watch what agents do for 5 minutes
4. Restore `CLOSED`

This experiment costs nothing and reveals everything:
- Do agents read the file at all? (If not, the circuit breaker is ignored)
- Do agents buffer, drop, or crash when they can't write to Dolt?
- Does the Mayor's inbox fill up with noise, or do agents go quiet?
- Does any agent try to probe and flip the state on its own?

The result will be one of three findings: (a) agents are already resilient and this is mostly handled, (b) agents crash or thrash and need explicit circuit-breaker awareness, or (c) agents go silent but critical state is silently lost. Each finding points to a different design priority. This experiment costs 5 minutes and produces more signal than any amount of design discussion.
