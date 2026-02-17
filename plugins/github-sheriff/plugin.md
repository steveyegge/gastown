+++
name = "github-sheriff"
description = "Monitor GitHub CI checks on open PRs and create beads for failures"
version = 1

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:github-sheriff", "category:ci-monitoring"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "low"
+++

# GitHub Sheriff

Polls GitHub for failed CI checks on open pull requests and creates `ci-failure`
beads for each new failure. Implements the PR Sheriff pattern from the
[Gas Town User Manual](https://steve-yegge.medium.com/gas-town-emergency-user-manual-cf0e4556d74b)
as a Deacon plugin.

Requires: `gh` CLI installed and authenticated (`gh auth status`).

## Detection

Verify `gh` is available and authenticated:

```bash
gh auth status 2>/dev/null
if [ $? -ne 0 ]; then
  echo "SKIP: gh CLI not authenticated"
  exit 0
fi
```

Detect the repo from the rig's git remote. Fall back to explicit config if
detection fails:

```bash
REPO=$(git -C "$GT_RIG_ROOT" remote get-url origin 2>/dev/null \
  | sed -E 's|.*github\.com[:/]||; s|\.git$||')

if [ -z "$REPO" ]; then
  echo "SKIP: could not detect GitHub repo from rig remote"
  exit 0
fi
```

## Action

### Step 1: List open PRs

```bash
PRS=$(gh pr list --repo "$REPO" --state open \
  --json number,title,author,headRefName,url --limit 100)

PR_COUNT=$(echo "$PRS" | jq length)
if [ "$PR_COUNT" -eq 0 ]; then
  echo "No open PRs found for $REPO"
  exit 0
fi
```

### Step 2: Check each PR for failures

For each open PR, fetch check runs and identify failures:

```bash
FAILURES=()
for PR_NUM in $(echo "$PRS" | jq -r '.[].number'); do
  PR_TITLE=$(echo "$PRS" | jq -r ".[] | select(.number == $PR_NUM) | .title")

  CHECKS=$(gh pr checks "$PR_NUM" --repo "$REPO" \
    --json name,state,conclusion,link 2>/dev/null || echo "[]")

  for ROW in $(echo "$CHECKS" | jq -c '.[] | select(.conclusion == "failure" or .conclusion == "timed_out" or .conclusion == "cancelled" or .conclusion == "action_required")'); do
    CHECK_NAME=$(echo "$ROW" | jq -r '.name')
    CHECK_URL=$(echo "$ROW" | jq -r '.link')
    CONCLUSION=$(echo "$ROW" | jq -r '.conclusion')
    FAILURES+=("$PR_NUM|$PR_TITLE|$CHECK_NAME|$CHECK_URL|$CONCLUSION")
  done
done
```

### Step 3: Deduplicate against existing beads

For each failure, check if a bead already exists:

```bash
EXISTING=$(bd list --label ci-failure --status open --json 2>/dev/null || echo "[]")

CREATED=0
SKIPPED=0

for F in "${FAILURES[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE CHECK_NAME CHECK_URL CONCLUSION <<< "$F"
  BEAD_TITLE="CI failure: $CHECK_NAME on PR #$PR_NUM"

  # Check for duplicate
  if echo "$EXISTING" | jq -e ".[] | select(.title == \"$BEAD_TITLE\")" > /dev/null 2>&1; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  # Create bead
  DESCRIPTION="CI check \`$CHECK_NAME\` failed on PR #$PR_NUM ($PR_TITLE)

PR: https://github.com/$REPO/pull/$PR_NUM
Check: $CHECK_URL
Conclusion: $CONCLUSION"

  BEAD_ID=$(bd create "$BEAD_TITLE" -t task -p 2 \
    --description "$DESCRIPTION" \
    --label ci-failure \
    --json 2>/dev/null | jq -r '.id // empty')

  if [ -n "$BEAD_ID" ]; then
    CREATED=$((CREATED + 1))

    # Log to activity feed
    gt activity emit github_check_failed \
      --repo "$REPO" \
      --pr "$PR_NUM" \
      --check "$CHECK_NAME" \
      --conclusion "$CONCLUSION" \
      --bead "$BEAD_ID" 2>/dev/null || true
  fi
done
```

## Record Result

```bash
SUMMARY="$REPO: checked $PR_COUNT PRs, ${#FAILURES[@]} failure(s), $CREATED bead(s) created, $SKIPPED already tracked"
echo "$SUMMARY"
```

On success:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:github-sheriff \
  --label result:success \
  --body "$SUMMARY"
```

On failure:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:github-sheriff \
  --label result:failure \
  --body "GitHub sheriff failed: $ERROR"

gt escalate --severity=low \
  --subject="Plugin FAILED: github-sheriff" \
  --body="$ERROR" \
  --source="plugin:github-sheriff"
```
