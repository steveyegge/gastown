# Lens: Industrial Control Systems — Emergency Stop (E-stop)
## Issue: gt-d3n

---

### 1. Framing the Problem

In industrial automation, an E-stop is not merely a "shutdown command" — it is a **hardwired, normally-closed safety circuit** that exists outside the control plane it governs. The defining principle is: the mechanism that halts the system must be architecturally independent from the system it halts. A PLC whose software has crashed can still be stopped because the E-stop circuit bypasses it entirely.

Gas Town's problem is that the daemon is both the orchestrator and the only entity capable of issuing halt signals. When the data plane (Dolt) fails, the daemon may itself be degraded, confused, or hung — exactly when you need a halt to work reliably. All coding agents continue because the signal path runs through the very infrastructure that has failed. This is equivalent to wiring your factory E-stop button through the PLC's I/O card: the one case where you need it most is the one case where it will not work.

The framing: **Gas Town lacks a safety-category halt mechanism.** It has a control-plane stop (rig commands), but not a safety-plane stop. These are different things, and conflating them is the root of the gap.

---

### 2. What Other Lenses Will Likely Miss

Most lenses will approach this as a coordination or protocol problem: "how do agents learn they should stop?" They will propose solutions involving the daemon broadcasting a message, writing to Dolt, or sending a tmux command sequence.

What they will miss is the **de-energize-to-safe principle**. In safety engineering, the default state of a machine must be safe. E-stop circuits are normally-closed: power must be continuously supplied to keep the machine running. Cut the power — for any reason, including wiring failure — and the machine stops. The safe state is achieved by doing nothing.

Software agents in Gas Town run until told to stop. This is normally-open: the default state is active. Any halt mechanism that requires a positive action (write a file, send a message, call an API) can fail silently. A de-energize-to-safe design would invert this: agents check for a "continue running" signal at each work cycle. If the signal is absent or stale, they park. The daemon's 30s heartbeat already approximates this infrastructure — it just isn't being used as a safety interlock.

---

### 3. Proposed Solution

**Implement a heartbeat-as-permit file using a short-lived lease.**

The daemon writes a file (e.g., `~/.gt/run.permit`) containing a timestamp every N seconds (say, 20s, shorter than its current 30s heartbeat). Each agent, before beginning any new work unit (before making an API call, before committing to a task step), reads this file and checks that the timestamp is fresher than 2×N seconds. If the permit is absent, unreadable, or stale, the agent parks itself.

E-stop procedure: the daemon (or a human operator via a single `rm ~/.gt/run.permit` or by writing a `HALT` flag into it) stops renewing the permit. All agents drain their current atomic operation and then park. No agent-to-agent communication required. No Dolt dependency. Works even if the daemon crashes — a daemon crash is itself an implicit E-stop because the permit goes stale.

This maps directly to the industrial watchdog timer / deadman switch pattern: the system stays running only as long as someone is actively asserting it should.

---

### 4. Failure Mode

**Permit check granularity is too coarse relative to API call duration.**

If an agent is in the middle of a long LLM API call (which can run 30–120 seconds for complex tasks), it will not check the permit until the call returns. During a Dolt outage, this means agents may complete one full work unit after the E-stop is issued before actually parking. In factory terms, this is the "coasting" problem — a press die that has already begun its stroke cannot stop mid-cycle; it completes the stroke before the E-stop takes effect.

The mitigation in industry is to design work units to be short and atomic, or to use interruptible actuators. In software, this means the permit check must be placed at the finest-grained loop point possible, and the system design should acknowledge that "stop at next safe boundary" is not the same as "stop immediately." Documentation and operator expectations must reflect this: the E-stop halts new work, not in-flight work.

A secondary failure mode: if `~/.gt/run.permit` is on the same disk that is full, the permit cannot be written, which inadvertently triggers a halt. This is actually fail-safe behavior (consistent with de-energize-to-safe), but operators must be aware it is not a bug.

---

### 5. Cheap Experiment

**Without writing any code, run a manual drill.**

Pick a moment when two or more agents are actively running. Have a human operator manually delete (or rename) the file that would serve as the run permit. Then observe: do the agents have any natural "check in before proceeding" points where this absence would be noticed? Look at existing agent loop structure for any polling behavior, idle checks, or pre-task gate conditions that already exist.

This experiment costs nothing and answers the most important architectural question before any implementation: where are the natural safe-stop boundaries in the current agent execution loop? If agents already poll for new work items (likely), those poll points are the insertion sites for a permit check. If they do not, the experiment reveals the deeper problem — agents have no idle loop at all — which is a prerequisite problem to solve first.

Cost: 15 minutes of observation. Output: a map of existing safe-stop boundaries, or evidence that none exist.
