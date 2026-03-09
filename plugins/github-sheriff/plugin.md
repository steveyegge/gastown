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

Check for git remote and PR sources:

```bash
RIGS=("beads" "debt_buying" "gleam" "laser" "payment_portal" "shuffle" "teleport" "gastown")
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

if [ -z "$GITHUB_TOKEN" ]; then
  # No GitHub token, cannot fetch PRs
  bd wisp create \
    --label type:plugin-run \
    --label plugin:github-sheriff \
    --label result:skipped \
    --body "GitHub token not configured - skipping PR categorization"
  exit 0
fi
```

## Action

### 1. Fetch open PRs for each rig

```bash
for RIG in "${RIGS[@]}"; do
  REPO_PATH="/Users/seanbearden/gt/deacon/dogs/charlie/$RIG"

  if [ ! -d "$REPO_PATH/.git" ]; then
    continue
  fi

  # Extract GitHub owner/repo from git remote
  REMOTE_URL=$(cd "$REPO_PATH" && git config --get remote.origin.url)
  OWNER=$(echo "$REMOTE_URL" | sed -E 's|.*[:/]([^/]+)/([^/]+)\.git$|\1|')
  REPO=$(echo "$REMOTE_URL" | sed -E 's|.*[:/]([^/]+)/([^/]+)\.git$|\2|')

  if [ -z "$OWNER" ] || [ -z "$REPO" ]; then
    continue
  fi
done
```

### 2. Categorize each PR

```bash
# Easy Wins: Green
# - CI passing (all checks pass)
# - Small PR (< 200 lines changed)
# - No merge conflicts
# - Author is a bot or trusted integrations

# Needs Review: Yellow
# - CI failing or incomplete
# - Large PR (>= 200 lines changed)
# - Merge conflicts present
# - Sensitive files changed
# - Multiple reviewers requested

EASY_WINS=()
NEEDS_REVIEW=()

# Use GitHub API to fetch PR details and categorize
curl -s -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$OWNER/$REPO/pulls?state=open&sort=updated&direction=desc" \
  | jq -r '.[] | @json' \
  | while read -r PR_JSON; do
    PR=$(echo "$PR_JSON" | jq -r '.')
    PR_NUMBER=$(echo "$PR" | jq -r '.number')
    PR_TITLE=$(echo "$PR" | jq -r '.title')
    AUTHOR=$(echo "$PR" | jq -r '.user.login')
    CI_STATUS=$(echo "$PR" | jq -r '.statuses_url // .commits_url' | head -1)

    # Fetch status
    STATUS=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
      "$CI_STATUS" 2>/dev/null | jq -r '.status // "unknown"')

    if [ "$STATUS" = "success" ]; then
      EASY_WINS+=("$RIG/$PR_NUMBER: $PR_TITLE (author: $AUTHOR)")
    else
      NEEDS_REVIEW+=("$RIG/$PR_NUMBER: $PR_TITLE (status: $STATUS)")
    fi
  done
```

### 3. Record Result

Count and categorize:

```bash
EASY_COUNT=${#EASY_WINS[@]}
NEEDS_COUNT=${#NEEDS_REVIEW[@]}
TOTAL_COUNT=$((EASY_COUNT + NEEDS_COUNT))

if [ $TOTAL_COUNT -eq 0 ]; then
  # No open PRs
  bd wisp create \
    --label type:plugin-run \
    --label plugin:github-sheriff \
    --label result:success \
    --body "PR Sheriff patrol complete: no open PRs found"
  exit 0
fi
```

### On success:

```bash
EASY_LIST=$(printf '%s\n' "${EASY_WINS[@]}")
NEEDS_LIST=$(printf '%s\n' "${NEEDS_REVIEW[@]}")

bd wisp create \
  --label type:plugin-run \
  --label plugin:github-sheriff \
  --label result:success \
  --body "PR Sheriff patrol complete: $EASY_COUNT easy wins, $NEEDS_COUNT need review

**Easy Wins (ready to sling):**
$EASY_LIST

**Needs Review (flag for human):**
$NEEDS_LIST"
```

### On failure:

```bash
ERROR_MSG="${1:-Unknown error}"

bd wisp create \
  --label type:plugin-run \
  --label plugin:github-sheriff \
  --label result:failure \
  --body "PR Sheriff patrol failed: $ERROR_MSG"

gt escalate --severity=medium \
  --subject="Plugin FAILED: github-sheriff" \
  --body="Error during PR categorization: $ERROR_MSG" \
  --source="plugin:github-sheriff"
```

## Notes

- Runs hourly to keep PR status current
- Requires GITHUB_TOKEN environment variable
- Monitors all standard Gas Town rigs
- Easy wins are candidates for auto-merge or quick sling to other crew
- Complex PRs flagged for human review and decision-making
- Results stored as wisps for audit trail
- Categories extensible for custom rules

## Future Enhancements

- Auto-merge simple PRs (with human approval rules)
- Slack notifications for new PRs
- Custom categorization rules per rig
- Lint/test requirement integration
- Historical PR metrics dashboard
