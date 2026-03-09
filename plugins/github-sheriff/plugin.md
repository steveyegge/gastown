+++
name = "github-sheriff"
description = "Categorize and report on open PRs across Gas Town rigs"
version = 1

[gate]
type = "cron"
schedule = "0 * * * *"

[tracking]
labels = ["plugin:github-sheriff", "category:code-review", "category:workflow"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "medium"
+++

# GitHub PR Sheriff

Monitors open pull requests across all Gas Town rigs and categorizes them by complexity and readiness for review:

- Fetches open PRs from each rig's repository
- Categorizes as "easy wins" (CI passing, small, no conflicts) vs "needs review" (large, failing CI, conflicts)
- Creates beads for PR status tracking
- Reports summary to town-level dashboard
- Flags complex PRs for human review

## Detection

Discovers rigs dynamically from `rigs.json` in the town root. Requires `GITHUB_TOKEN` env var.

## Action

### 1. Fetch open PRs for each rig

Iterates over rigs discovered from `rigs.json`, extracts GitHub owner/repo from each rig's git remote, and fetches open PRs via the GitHub API.

### 2. Categorize each PR

- **Easy Wins**: CI passing, small (<200 lines), no merge conflicts
- **Needs Review**: CI failing, large, or has conflicts

### 3. Record result

Creates a wisp with categorized summary. On failure, escalates via `gt escalate`.

## Notes

- Runs hourly via cron gate
- Requires `GITHUB_TOKEN` environment variable
- Discovers rigs dynamically from `rigs.json` (no hardcoded rig list)
- Executed via `execute.sh` — not AI-interpreted
- Results stored as wisps for audit trail
