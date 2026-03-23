# GitHub PR Merge Strategy

## Why this exists

Gas Town's core thesis is that the Refinery *is* the quality gate and `merge_strategy: "direct"` is the correct end state. Direct merge is the default and the intended mode of operation. This feature does not change that.

The problem is adoption. Organisations in more regulated environments with a higher compliance burden often have requirements where **all changes to the default branch must go through a human-approved pull request**. These organisations cannot adopt Gas Town today — the Refinery either merges directly (violating compliance) or is bypassed altogether (missing a key benefit of Gas Town).

PR mode provides **transitional training wheels**. The Refinery runs its full gate validation (build, lint, test, e2e), then produces a PR as proof of its work instead of merging directly. The PR's CI pipeline is technically redundant as the Refinery duplicates CI gates. That redundancy is the point: it lets the organisation **measure** how often the PR catches something the Refinery missed (ideally: never), building the evidence base to eventually remove the compliance gate.

This is not an alternative end state. The defaults make this clear:
- `merge_strategy` defaults to `"direct"` — no change for existing rigs
- `pr_auto_merge` defaults to `true` — even in PR mode, the default is fully automated
- Stale PR escalation treats open PRs as a problem to resolve, not a normal state
- PR rejection indicates an upstream failure in Gas Town config, not a per-PR issue to fix

**The goal is that organisations using this mode actively want to stop using it.**

## When to enable this

Enable `merge_strategy: "pr"` when:
- Your org requires human-approved PRs on the default branch for compliance
- You want to adopt Gas Town incrementally while satisfying existing branch protection rules
- You want to run the Refinery's gates in parallel with existing CI to prove equivalence

Do **not** enable this if you can merge directly — it adds latency (human review), holds polecat capacity while PRs are open, and provides no quality benefit over the Refinery's own gates.

## Configuration

Add to your rig's `settings/config.json` under `merge_queue`:

```json
{
  "merge_queue": {
    "merge_strategy": "pr",
    "pr_auto_merge": false,
    "pr_stale_warn_hours": 8,
    "pr_stale_escalate_hours": 24
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `merge_strategy` | `string` | `"direct"` | `"direct"` (ff-only merge+push) or `"pr"` (GitHub PR) |
| `pr_auto_merge` | `bool` | `true` | When `"pr"`, auto-merge after CI passes. `false` = leave open for human approval |
| `pr_stale_warn_hours` | `int` | `8` | Hours before an open PR is flagged as stale in patrol summary |
| `pr_stale_escalate_hours` | `int` | `24` | Hours before STALE_PR escalation is sent to Mayor (sent once) |

### How it works

**`pr_auto_merge: true` (default)** — The Refinery creates a GitHub PR, waits for CI, then merges via `gh pr merge`. Same lifecycle as direct merge, just routed through a PR.

**`pr_auto_merge: false`** — The Refinery creates the PR but does **not** merge it. Each patrol cycle, the `poll-open-prs` step checks PR status on GitHub:

- **Merged** — PR approved and merged by a human; sends MERGED to Witness, runs post-merge cleanup
- **Closed without merge** — sends PR_REJECTED to Witness, closes MR bead
- **Still open** — checks staleness against `pr_stale_warn_hours` / `pr_stale_escalate_hours`

While a PR is open the polecat that produced the work remains allocated (MR bead stays open, Witness won't recycle it). Escalation to the Mayor is sent once per PR.

### Design boundary

PR integration is one-way: Gas Town pushes PRs out and polls for terminal status. It does not pull PR comments or review feedback back in. If a reviewer rejects the PR, that feedback must flow back through the operator — not through the PR itself.

PR rejection should be rare and indicates a systemic upstream issue (Refinery gates, AGENTS.md, rig config are not catching what the human reviewer catches). The correct response is to fix the upstream cause, not patch the individual PR.

### Adoption path

1. Start with `merge_strategy: "pr"` + `pr_auto_merge: false` — compliance-safe, human approves every PR
2. Build confidence that Refinery gates catch everything CI catches
3. Move to `pr_auto_merge: true` — Refinery creates and merges PRs after CI passes
4. Eventually move to `merge_strategy: "direct"` — full Gas Town model

### External issue tracker linking

Some orgs also require PRs to link to an external tracker (ClickUp, Jira, Linear). This is solved outside Gas Town via a wrapper script on the container's PATH (assuming gastown is being run within a container) that intercepts `gh pr create` and enriches the PR title/body. No Gas Town code changes needed.
