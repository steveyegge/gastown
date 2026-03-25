# Lens: Adversary — Failure Modes of the E-stop Itself

## Framing

The E-stop is presented as a safety mechanism, but safety mechanisms are themselves systems — and systems fail. The adversary lens asks: what happens when the cure is part of the disease? An E-stop that depends on Dolt to signal, coordinate, or record its own activation is bootstrapped on the very infrastructure it is meant to protect. When Dolt is degraded, the E-stop is degraded. This is not an edge case; it is the most likely scenario in which E-stop would be invoked.

The deeper problem is that the E-stop introduces a new class of state — "halted" — that every agent, process, and human operator must handle correctly. Each new state transition is a new failure mode. Before E-stop exists, agents are either running or crashed. After E-stop, they can be running, crashed, halted-cleanly, halted-mid-operation, halted-and-orphaned, or believed-halted-but-still-running. The combinatorial surface grows fast.

## What Other Lenses Will Likely Miss

Other lenses will analyze whether the E-stop correctly identifies real failures and correctly halts work. They will treat the E-stop's own execution as reliable. The adversary lens focuses instead on the **split-brain window**: the interval between when the E-stop signal is issued and when all agents have acknowledged it.

During this window, some agents are halted and some are not. An agent that is mid-write to a Dolt branch when the stop arrives may commit a partial object. An agent that has not yet received the signal continues operating and may push commits, open beads, or send mail that another agent — now halted — was supposed to respond to. The system is not paused; it is incoherent. The longer the window, the worse the incoherence. If Dolt is degraded (the trigger condition), the acknowledgment mechanism is also degraded, so the window could be very long or never close.

## Proposed Solution

Implement a **local, agent-side timeout circuit** independent of the central E-stop signal. Each agent, on startup, checks a heartbeat file or lightweight local flag — something that does not require a Dolt query — at the start of each major operation. If the agent has not received a heartbeat within a configurable interval (e.g., 60 seconds), it self-suspends and waits before proceeding. This means even if the central E-stop signal never arrives (because Dolt is down), agents degrade gracefully by stalling rather than thrashing. The E-stop becomes a faster path to an outcome that would happen anyway, not the only path.

This also limits blast radius: an agent that self-suspends at an operation boundary is far less likely to be mid-commit than one that receives an asynchronous kill signal.

## Failure Mode

**The E-stop fires on a transient Dolt blip, halts all agents, and no human is present to resume.**

Dolt has documented fragility. A 10-second connection timeout during a routine GC or compaction is not a real infrastructure failure — it is noise. If the E-stop threshold is tuned aggressively, it will fire on this noise. All agents stop. All pending work — including work with real deadlines — sits frozen. The E-stop mechanism may require a human to issue a resume command. If the Mayor agent is itself halted (because it is also an agent subject to the stop), no automated recovery is possible. If the human operator is unavailable (night, weekend, vacation), the system is frozen for an unbounded duration.

This failure mode is particularly bad because it is invisible from the outside. The system looks "stable" — nothing is crashing, no errors are being logged — but no work is being done.

## Cheap Experiment

Before implementing any E-stop logic, instrument the existing system to measure **how often Dolt would have triggered a threshold-based E-stop over the past 30 days** using historical latency and connection-error logs. Pick two candidate thresholds: a conservative one (e.g., "connection refused for 30 seconds") and an aggressive one (e.g., "any query latency > 5s for 10 seconds"). Count how many times each would have fired, and for how many of those events work was actually corrupted versus just slowed. This produces a concrete false-positive rate before any code is written, and will immediately reveal whether the proposed thresholds are calibrated to reality or to an imagined failure distribution.
