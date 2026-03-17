# Medici Lens: Incentives

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-alh

## Framing

The Rally Tavern integration creates a two-sided marketplace: consumers (agents searching knowledge) and producers (agents nominating knowledge). Like any marketplace, it only works if both sides participate.

The consumption side has low friction — `gt rally search` runs in-band, and formula integration makes it nearly automatic. But the production side has a cold-start problem: agents must actively decide to nominate after completing work, draft a structured nomination (category, title, summary, tags), and the payoff is delayed (their nomination might be accepted days later, and they'll never see the benefit since future sessions don't know who nominated what).

The incentive structure is asymmetric: consuming is cheap and immediate, producing is expensive and delayed. Marketplaces with this asymmetry die from supply starvation.

## What Other Lenses Will Likely Miss

**The nomination prompt in mol-polecat-work is a guilt trip, not an incentive.** The formula says something like "consider nominating what you learned." This is the agent equivalent of "please leave a review" — most users/agents skip it because the cost (drafting a nomination) exceeds the perceived benefit (vague community good).

Real incentives need to be structural, not verbal:
- The capability ledger tracks completions but not knowledge contributions. Nominations are invisible in the agent's track record.
- There's no feedback loop: a polecat that nominates never learns if it was accepted, rejected, or applied by another agent. The mail reply from barkeep goes to the nominator's inbox, but by then the session is long dead.
- There's no *consumption* incentive either: an agent that uses Rally Tavern knowledge to solve a problem faster doesn't get credited for it, and the knowledge entry doesn't learn it was useful.

## Proposed Solution

**Close the feedback loop with lightweight telemetry.**

1. **Track usage:** When `gt rally search` returns results, log which entries were surfaced (append to a `knowledge/.usage-log` file or similar). This doesn't track whether agents *applied* the knowledge, but it measures reach.
2. **Track contribution in the ledger:** When a nomination is accepted, add a line to the nominating agent's capability record (e.g., "knowledge contribution accepted: <entry-id>"). This makes contributions visible in the agent's CV.
3. **Auto-nominate from beads notes:** Instead of asking agents to draft a nomination manually, detect when a bead's `--notes` or `--design` field contains a lesson pattern (e.g., starts with "Lesson:" or "Gotcha:") and offer to auto-generate a nomination from it. This reduces production cost to near-zero.

## Failure Mode

**Cold-start death.** The knowledge corpus starts empty or near-empty. Early agents search and find nothing useful. They learn to skip the knowledge step. They also never nominate because the step feels pointless when the corpus is empty. The corpus stays empty. The flywheel never spins.

This is the classic marketplace cold-start problem. It cannot be solved by better UX alone — it requires manual seeding.

## Cheap Experiment

**Seed 10 entries manually, then measure nomination rate.** Have a human (or a dedicated agent) write 10 high-quality knowledge entries covering common Gas Town patterns (Dolt session management, tmux conventions, formula design, polecat lifecycle). Then track: (a) how many polecats hit these entries in the next week via search, (b) how many polecats nominate new knowledge unprompted. Compare the nomination rate before and after seeding to see if a visible, useful corpus bootstraps the contribution loop.
