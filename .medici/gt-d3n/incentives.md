# Lens: Incentives — Agent Economics and Token Waste

**Issue:** gt-d3n — Emergency Stop (E-stop)
**Lens:** Agent economics and token waste

---

## 1. Framing the Problem

The core economic tension is that agents are paid-by-the-token workers who have no native awareness of their own cost. When infrastructure fails, an agent's rational local behavior — retry, diagnose, attempt recovery, ask for help — is irrational at the system level. The agent has no skin in the game: retrying costs the operator money but costs the agent nothing. This creates a principal-agent problem embedded directly in the failure loop.

Concrete cost model: a 100K-context agent session running against a hung Dolt server will spend roughly 1,000–3,000 tokens per failed tool call (context overhead + error message + continuation). At a 30-second retry interval with a 2-minute timeout per call, a single stuck agent burns ~6,000–18,000 tokens per minute. With 10 agents in that state for 2 hours (the 2.1GB log incident), the token waste is on the order of 7–22 million tokens — or roughly $21–$66 at Sonnet pricing, before context window growth effects compound the cost.

But the worse cost is opportunity cost: those agent sessions were holding context about live work. Killing them wastes that context. The question is not "stop or don't stop" — it is "when does the burn rate of doing nothing exceed the replacement cost of a clean restart?"

---

## 2. What Other Lenses Will Miss

Other lenses (reliability, UX, architecture) will frame this as a detection problem: how do we know things are broken? The incentives lens reveals it is actually a **cost-accounting problem**: the system has no way to compare the marginal cost of continued operation against the marginal cost of a halt.

Specifically, other analyses will likely propose thresholds based on error rates or latency — "if Dolt is down for 60 seconds, stop agents." But that framing ignores that the right threshold is not time-based, it is cost-based. A brief outage that hits 3 idle agents has near-zero cost. The same outage hitting 15 agents in the middle of large-context reasoning tasks has very high cost. The threshold must be a function of active agent count, estimated context sizes, and failure duration — not just duration alone.

The incentives lens also surfaces a second missed point: the E-stop itself has a cost. Interrupting an agent mid-task destroys in-flight context. If the outage lasts 35 seconds, killing 10 agents saved maybe 35,000 tokens but destroyed $X of context that must be reconstructed. The decision function must account for restart cost, not just burn rate.

---

## 3. Proposed Solution

Implement a **token-burn rate monitor** as a lightweight daemon that maintains a running estimate of system-wide token expenditure under failure conditions. The monitor tracks:

- Number of active agent sessions (approximated from process table or a registration file)
- Time since first infrastructure error signal
- A configurable per-agent burn rate estimate (tokens/minute, set per model tier)

When `(active_agents * burn_rate_per_agent * failure_duration_minutes) > restart_cost_threshold`, the system emits a halt signal. The `restart_cost_threshold` is a single operator-configured value in tokens (e.g., 500,000 tokens = roughly $1.50) that represents the acceptable waste before a hard stop is cheaper than continued operation.

This gives the E-stop system an explicit economic trigger that sits alongside time-based and error-rate triggers. The operator sets one number that expresses their actual risk tolerance in the currency that matters: money.

---

## 4. Failure Mode

The failure mode for this approach is **miscalibrated burn rate estimates leading to premature halts during brief recoverable hiccups**.

If the per-agent burn rate constant is set too high, or if the restart cost threshold is set too low, the system will trigger E-stops during 30-second Dolt hiccups that agents would have survived gracefully. This is the inverse of the original problem: instead of letting agents burn tokens on a dead system, the operator now kills healthy agents on every transient glitch.

The insidious version: the system learns to fire E-stops on Monday mornings when Dolt is slow at startup, interrupting all agents in the middle of receiving their work queue — the worst possible moment to destroy context. If operators observe this pattern, they will disable the economic trigger entirely, removing the protection that matters most.

Mitigation: the economic trigger must require both a cost threshold AND a minimum failure duration floor (e.g., "at least 90 seconds of confirmed failure"). Neither alone is sufficient.

---

## 5. Cheap Experiment

Before building any monitoring infrastructure, run a **post-mortem cost audit on the 2.1GB log incident** using existing data.

From the Dolt server logs and agent session logs, reconstruct:
1. How many agents were active during the failure window
2. How long each agent was in a failed-retry state
3. Approximate context sizes at failure time (from log timestamps and task complexity)

Then compute the actual token cost of the incident, compare it to the estimated cost of a clean restart at the 5-minute mark, the 15-minute mark, and the 30-minute mark.

This costs nothing except one hour of log analysis. It produces a concrete number ("the 2.1GB incident wasted approximately $X; a halt at T+15 minutes would have cost $Y to restart and saved $Z"). That single data point either validates the economic trigger hypothesis or kills it cheaply, before any code is written.
