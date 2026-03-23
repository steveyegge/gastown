---
name: pr-sheriff
description: >
  PR Sheriff workflow: triage PRs into easy-wins and crew assignments.
  Prints recommendations inline - does NOT post to GitHub.
allowed-tools: "Bash(gh pr *), Bash(git *), Bash(gt *), Bash(bd *), Bash(cat *)"
version: "2.0.0"
author: "Gas Town"
---

# PR Sheriff - Triage and Review Workflow

This skill delegates to the `mol-pr-sheriff-patrol` formula, which lives at
`$GT_ROOT/.beads/formulas/mol-pr-sheriff-patrol.formula.toml` and is shared
across all Gas Town rigs (gastown, beads, etc.).

## Repo Scope

This rig (gastown/crew/max) is responsible for **steveyegge/gastown only**.
The beads repo (steveyegge/beads) is handled by beads/crew/emma.
Do NOT discover or triage PRs from repos outside your scope.

When loading the shared config, filter the repo list to only `steveyegge/gastown`.

## Usage

```
/pr-sheriff [repo]
```

- `repo` - Optional. If provided, overrides the default scope.
  If omitted, scan only `steveyegge/gastown` (this rig's scope).

## How to Execute

**1. Load the config:**
```bash
cat $GT_ROOT/.beads/pr-sheriff-config.json
```

This contains crew mappings per repo and contributor policies
(trust tiers, firewalled contributors, bot auto-merge rules).
**Only scan repos within your scope** (see Repo Scope above).

**2. Follow the formula steps in order:**
```bash
gt formula show mol-pr-sheriff-patrol
```

The formula defines a 7-step workflow:

| Step | What it does |
|------|-------------|
| load-config | Load pr-sheriff-config.json, establish repo list |
| discover-prs | Find open PRs needing review across all repos |
| triage-batch | Categorize all PRs in one pass (preserves cross-PR context) |
| merge-easy-wins | Merge approved easy-wins via `gh pr merge` |
| dispatch-crew-reviews | Nudge crew for NEEDS-CREW PRs |
| dispatch-deep-reviews | Nudge crew for NEEDS-HUMAN PRs (full evaluation framework) |
| collect-results | Gather crew review nudge-backs |
| interactive-review | Walk through remaining NEEDS-HUMAN PRs with overseer |
| summarize | Print patrol summary |

**3. For each step, read the full description:**
```bash
gt formula show mol-pr-sheriff-patrol  # Shows all steps with descriptions
```

## Key References

- **Patrol formula:** `mol-pr-sheriff-patrol` (town-level)
- **Crew review formula:** `mol-pr-crew-review` (dispatched to crew)
- **Deep review formula:** `mol-pr-deep-review` (dispatched for NEEDS-HUMAN)
- **Config:** `$GT_ROOT/.beads/pr-sheriff-config.json`

## Contributor Policy Tiers

Loaded from config. Current tiers:

| Tier | Handling |
|------|----------|
| bot-trusted | Auto-merge if CI passes (e.g., dependabot) |
| community | Normal triage — easy-win / crew / deep review |
| firewalled | Always NEEDS-HUMAN, never auto-merge, deep review required |

## Category Decision Tree

```
Draft? → SKIP
Contributor firewalled? → NEEDS-HUMAN (deep review)
Dependabot patch bump + CI green? → EASY-WIN
<50 lines, obvious bug/doc/test fix? → EASY-WIN
Security/architecture/API change? → NEEDS-HUMAN
Multi-concern PR? → NEEDS-HUMAN
100+ lines new feature? → NEEDS-CREW or NEEDS-HUMAN
Everything else → NEEDS-CREW
```

## Deep Review Evaluation Framework (NEEDS-HUMAN)

The deep review formula (mol-pr-deep-review) applies six lenses:

1. **Plugin/integration fit** — core vs plugin/formula/integration?
2. **Tech-debt weight** — complexity justified by user breadth?
3. **Contributor track record** — first-time, repeat, or firewalled?
4. **ZFC compliance** — structural checks, not string heuristics?
5. **Problem validity + solution fit** — real problem, right solution?
6. **Splitability** — can good parts be cherry-picked from bad?

Final verdicts: MERGE | CHERRY-PICK | REWORK | REIMPLEMENT | CLOSE

## Output Format

For each PR, print a recommendation block:

```
### PR #<num>: <title>
Author: <login> | +<additions>/-<deletions> | <changedFiles> files

**Category**: EASY-WIN | NEEDS-CREW | NEEDS-HUMAN | SKIP

**Analysis**:
<1-3 sentences explaining the change and why it fits this category>

**Recommendation**:
<specific action>
```

## Summary Output

```
## PR Sheriff Patrol Summary — <date>

**Easy-wins merged**: N
**Crew-reviewed and merged**: N
**Sent back for rework**: N
**Closed**: N
**Still pending**: N
```

## Dispatching Work: Use Ephemeral Beads

When creating beads to track fix-merge work for polecats or crew, **use
ephemeral beads (wisps)** rather than persistent beads. PR review/fix-merge
tasks are orchestration scaffolding — they exist to give a polecat something
to hook and track, not to create a permanent record.

Ephemeral beads are the right trade-off: they give polecats and crew trackable
work items without polluting Dolt's permanent ledger with one-off orchestration
noise. If/when beads are exported to permanent ledgers, review-task wisps won't
clutter the history.

```bash
# Ephemeral bead for fix-merge dispatch
bd new -t task "Fix-merge PR #1234: description" -p 2 -l pr-review \
  --wisp-type patrol

# vs persistent (avoid for orchestration work)
bd new -t task "Fix-merge PR #1234: description" -p 2 -l pr-review
```

The `--wisp-type patrol` flag marks it as ephemeral orchestration work that
the reaper will eventually clean up. The polecat/crew member can still hook it,
work it, and close it normally.

## CRITICAL Rules

- All output is printed inline. Do NOT post comments to GitHub.
- The overseer decides what gets posted externally.
- Contributor-friendly: help contributors get to the finish line.
- Use `Co-authored-by` trailer when fixing up contributor work.
