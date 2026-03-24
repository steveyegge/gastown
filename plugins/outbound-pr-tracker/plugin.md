+++
name = "outbound-pr-tracker"
description = "Track outbound fix-merge PRs on upstream repos: CI status, maintainer feedback, merge/close events"
version = 1

[gate]
type = "cooldown"
duration = "2h"

[tracking]
labels = ["plugin:outbound-pr-tracker", "category:pr-tracking"]
digest = true

[execution]
timeout = "3m"
notify_on_failure = true
severity = "medium"
+++

# Outbound PR Tracker

Tracks PRs that we (outdoorsea) have submitted to upstream repos. Closes the
loop on the fix-merge workflow by monitoring CI status, maintainer feedback,
and merge/close events.

This is the outbound counterpart to the `github-sheriff` plugin, which monitors
inbound PRs on repos we own. This plugin monitors PRs we've sent upstream.

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

Detect the upstream repo from the rig's git remote:

```bash
# Get upstream remote (the repo we submit PRs TO)
UPSTREAM=$(git -C "$GT_RIG_ROOT" remote get-url upstream 2>/dev/null \
  | sed -E 's|.*github\.com[:/]||; s|\.git$||')

if [ -z "$UPSTREAM" ]; then
  echo "SKIP: no upstream remote configured"
  exit 0
fi

# Our fork's GitHub org (the author of outbound PRs)
OUR_ORG=$(git -C "$GT_RIG_ROOT" remote get-url origin 2>/dev/null \
  | sed -E 's|.*github\.com[:/]||; s|/.*||')

if [ -z "$OUR_ORG" ]; then
  echo "SKIP: could not detect origin org"
  exit 0
fi

echo "Tracking outbound PRs: author=$OUR_ORG upstream=$UPSTREAM"
```

## Action

### Step 1: Fetch open outbound PRs

Query the upstream repo for PRs authored by our org. This finds all our
fix-merge PRs, feature submissions, etc.

```bash
OPEN_PRS=$(gh pr list --repo "$UPSTREAM" --author "$OUR_ORG" --state open \
  --json number,title,url,state,statusCheckRollup,reviews,comments,updatedAt,createdAt \
  --limit 50 2>/dev/null || echo "[]")

OPEN_COUNT=$(echo "$OPEN_PRS" | jq 'length')
echo "Found $OPEN_COUNT open outbound PR(s) on $UPSTREAM"
```

### Step 2: Fetch recently closed/merged outbound PRs

Check PRs closed in the last 7 days to detect merges and rejections:

```bash
SINCE=$(date -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -v-7d +%Y-%m-%dT%H:%M:%SZ)

CLOSED_PRS=$(gh pr list --repo "$UPSTREAM" --author "$OUR_ORG" --state closed \
  --json number,title,url,state,mergedAt,closedAt,updatedAt \
  --limit 50 2>/dev/null || echo "[]")

# Filter to recently closed only
CLOSED_PRS=$(echo "$CLOSED_PRS" | jq --arg since "$SINCE" \
  '[.[] | select((.closedAt // .updatedAt) >= $since)]')

CLOSED_COUNT=$(echo "$CLOSED_PRS" | jq 'length')
echo "Found $CLOSED_COUNT recently closed outbound PR(s)"
```

### Step 3: Categorize open PRs

Process each open PR to determine its state and any actions needed:

```bash
CI_FAILING=()
REVIEW_COMMENTS=()
CHANGES_REQUESTED=()
APPROVED=()
WAITING=()

# Derive rig name for bead operations
RIG_NAME=$(basename "$(dirname "$(dirname "$GT_RIG_ROOT")")" 2>/dev/null)
RIG_FLAG=""
[ -n "$RIG_NAME" ] && RIG_FLAG="--rig $RIG_NAME"

while IFS= read -r PR_JSON; do
  [ -z "$PR_JSON" ] && continue

  PR_NUM=$(echo "$PR_JSON" | jq -r '.number')
  PR_TITLE=$(echo "$PR_JSON" | jq -r '.title')
  PR_URL=$(echo "$PR_JSON" | jq -r '.url')

  # --- CI Status ---
  TOTAL_CHECKS=$(echo "$PR_JSON" | jq '.statusCheckRollup | length')
  PASSING_CHECKS=$(echo "$PR_JSON" | jq '[.statusCheckRollup[] | select(
    .conclusion == "SUCCESS" or .conclusion == "NEUTRAL" or
    .conclusion == "SKIPPED" or .state == "SUCCESS"
  )] | length')
  PENDING_CHECKS=$(echo "$PR_JSON" | jq '[.statusCheckRollup[] | select(
    .state == "PENDING" or .conclusion == null
  )] | length')

  if [ "$TOTAL_CHECKS" -gt 0 ] && [ "$TOTAL_CHECKS" -eq "$PASSING_CHECKS" ]; then
    CI_STATUS="passing"
  elif [ "$PENDING_CHECKS" -gt 0 ] && [ "$PASSING_CHECKS" -eq 0 ] && [ "$TOTAL_CHECKS" -eq "$PENDING_CHECKS" ]; then
    CI_STATUS="pending"
  else
    CI_STATUS="failing"
  fi

  # Collect individual check failures
  FAILED_CHECKS=""
  while IFS= read -r CHECK; do
    [ -z "$CHECK" ] && continue
    CHECK_NAME=$(echo "$CHECK" | jq -r '.name')
    FAILED_CHECKS="${FAILED_CHECKS}${CHECK_NAME}, "
  done < <(echo "$PR_JSON" | jq -c '.statusCheckRollup[] | select(
    .conclusion == "FAILURE" or .conclusion == "CANCELLED" or
    .conclusion == "TIMED_OUT" or .state == "FAILURE" or .state == "ERROR"
  )')
  FAILED_CHECKS="${FAILED_CHECKS%, }"

  if [ "$CI_STATUS" = "failing" ] && [ -n "$FAILED_CHECKS" ]; then
    CI_FAILING+=("$PR_NUM|$PR_TITLE|$PR_URL|$FAILED_CHECKS")
  fi

  # --- Review Status ---
  # Check for CHANGES_REQUESTED or APPROVED reviews
  HAS_CHANGES_REQUESTED=$(echo "$PR_JSON" | jq '[.reviews[] | select(.state == "CHANGES_REQUESTED")] | length')
  HAS_APPROVED=$(echo "$PR_JSON" | jq '[.reviews[] | select(.state == "APPROVED")] | length')

  # Check for recent comments (review comments from maintainers)
  COMMENT_COUNT=$(echo "$PR_JSON" | jq '.comments | length')

  if [ "$HAS_CHANGES_REQUESTED" -gt 0 ]; then
    REVIEWER=$(echo "$PR_JSON" | jq -r '[.reviews[] | select(.state == "CHANGES_REQUESTED")] | last | .author.login')
    CHANGES_REQUESTED+=("$PR_NUM|$PR_TITLE|$PR_URL|$REVIEWER")
  elif [ "$HAS_APPROVED" -gt 0 ]; then
    APPROVED+=("$PR_NUM|$PR_TITLE|$PR_URL")
  elif [ "$COMMENT_COUNT" -gt 0 ]; then
    LAST_COMMENTER=$(echo "$PR_JSON" | jq -r '.comments | last | .author.login // "unknown"')
    # Only flag if last comment is NOT from us
    if [ "$LAST_COMMENTER" != "$OUR_ORG" ]; then
      REVIEW_COMMENTS+=("$PR_NUM|$PR_TITLE|$PR_URL|$LAST_COMMENTER")
    else
      WAITING+=("$PR_NUM|$PR_TITLE|$PR_URL|$CI_STATUS")
    fi
  else
    WAITING+=("$PR_NUM|$PR_TITLE|$PR_URL|$CI_STATUS")
  fi

done < <(echo "$OPEN_PRS" | jq -c '.[]')
```

### Step 4: Detect merged and rejected PRs

```bash
MERGED=()
REJECTED=()

while IFS= read -r PR_JSON; do
  [ -z "$PR_JSON" ] && continue

  PR_NUM=$(echo "$PR_JSON" | jq -r '.number')
  PR_TITLE=$(echo "$PR_JSON" | jq -r '.title')
  PR_URL=$(echo "$PR_JSON" | jq -r '.url')
  MERGED_AT=$(echo "$PR_JSON" | jq -r '.mergedAt // empty')

  if [ -n "$MERGED_AT" ]; then
    MERGED+=("$PR_NUM|$PR_TITLE|$PR_URL")
  else
    REJECTED+=("$PR_NUM|$PR_TITLE|$PR_URL")
  fi
done < <(echo "$CLOSED_PRS" | jq -c '.[]')
```

### Step 5: Create beads for actionable items

Deduplicate against existing beads before creating new ones:

```bash
EXISTING=$(bd list --label outbound-pr --status open $RIG_FLAG --json 2>/dev/null || echo "[]")
CREATED=0
SKIPPED=0

# --- CI Failures: create beads for crew to diagnose ---
for F in "${CI_FAILING[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE PR_URL CHECKS <<< "$F"
  BEAD_TITLE="Outbound PR #$PR_NUM: CI failing ($CHECKS)"

  if echo "$EXISTING" | jq -e --arg n "$PR_NUM" '.[] | select(.title | contains("#" + $n + ":"))' > /dev/null 2>&1; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  bd create "$BEAD_TITLE" -t task -p 2 \
    -d "CI checks failing on our outbound PR.

PR: $PR_URL
Failed checks: $CHECKS

Action: diagnose failure, push fix to the PR branch, or escalate if upstream CI is broken." \
    -l outbound-pr,ci-failure \
    $RIG_FLAG \
    --silent 2>/dev/null && CREATED=$((CREATED + 1))
done

# --- Changes Requested: create beads for crew to respond ---
for F in "${CHANGES_REQUESTED[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE PR_URL REVIEWER <<< "$F"
  BEAD_TITLE="Outbound PR #$PR_NUM: changes requested by $REVIEWER"

  if echo "$EXISTING" | jq -e --arg n "$PR_NUM" '.[] | select(.title | contains("#" + $n + ":"))' > /dev/null 2>&1; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  bd create "$BEAD_TITLE" -t task -p 2 \
    -d "Maintainer requested changes on our outbound PR.

PR: $PR_URL
Reviewer: $REVIEWER

Action: review feedback, make requested changes, push update." \
    -l outbound-pr,changes-requested \
    $RIG_FLAG \
    --silent 2>/dev/null && CREATED=$((CREATED + 1))
done

# --- Review Comments: surface for crew response ---
for F in "${REVIEW_COMMENTS[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE PR_URL COMMENTER <<< "$F"
  BEAD_TITLE="Outbound PR #$PR_NUM: maintainer comment from $COMMENTER"

  if echo "$EXISTING" | jq -e --arg n "$PR_NUM" '.[] | select(.title | contains("#" + $n + ":"))' > /dev/null 2>&1; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  bd create "$BEAD_TITLE" -t task -p 3 \
    -d "Maintainer commented on our outbound PR.

PR: $PR_URL
Commenter: $COMMENTER

Action: read comment, respond or make changes as needed." \
    -l outbound-pr,maintainer-comment \
    $RIG_FLAG \
    --silent 2>/dev/null && CREATED=$((CREATED + 1))
done

# --- Merged: emit activity event, close any tracking beads ---
for F in "${MERGED[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE PR_URL <<< "$F"
  echo "  MERGED: PR #$PR_NUM — $PR_TITLE ($PR_URL)"

  gt activity emit outbound_pr_merged \
    --message "Outbound PR #$PR_NUM merged upstream: $PR_TITLE ($UPSTREAM)" \
    2>/dev/null || true

  # Close any open tracking beads for this PR
  TRACKING_ID=$(echo "$EXISTING" | jq -r --arg n "$PR_NUM" \
    '.[] | select(.title | contains("#" + $n + ":")) | .id // empty' 2>/dev/null | head -1)
  if [ -n "$TRACKING_ID" ]; then
    bd close "$TRACKING_ID" --reason="merged: PR #$PR_NUM merged upstream" \
      $RIG_FLAG 2>/dev/null || true
    echo "  Closed tracking bead: $TRACKING_ID"
  fi
done

# --- Rejected (closed without merge): flag for review ---
for F in "${REJECTED[@]}"; do
  IFS='|' read -r PR_NUM PR_TITLE PR_URL <<< "$F"
  BEAD_TITLE="Outbound PR #$PR_NUM: closed without merge — review needed"

  if echo "$EXISTING" | jq -e --arg n "$PR_NUM" '.[] | select(.title | contains("#" + $n + ":"))' > /dev/null 2>&1; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  bd create "$BEAD_TITLE" -t task -p 2 \
    -d "Our outbound PR was closed without being merged.

PR: $PR_URL

Action: review why it was closed. Consider: resubmit with changes, open discussion, or accept rejection." \
    -l outbound-pr,rejected \
    $RIG_FLAG \
    --silent 2>/dev/null && CREATED=$((CREATED + 1))

  gt activity emit outbound_pr_rejected \
    --message "Outbound PR #$PR_NUM closed without merge: $PR_TITLE ($UPSTREAM)" \
    2>/dev/null || true
done
```

### Step 6: Print patrol summary

```bash
echo ""
echo "=== Outbound PR Tracker Summary ==="
echo "Upstream: $UPSTREAM (author: $OUR_ORG)"
echo ""

if [ ${#CI_FAILING[@]} -gt 0 ]; then
  echo "CI Failing (${#CI_FAILING[@]}):"
  for F in "${CI_FAILING[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL CHECKS <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE — failing: $CHECKS"
  done
fi

if [ ${#CHANGES_REQUESTED[@]} -gt 0 ]; then
  echo "Changes Requested (${#CHANGES_REQUESTED[@]}):"
  for F in "${CHANGES_REQUESTED[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL REVIEWER <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE — by $REVIEWER"
  done
fi

if [ ${#REVIEW_COMMENTS[@]} -gt 0 ]; then
  echo "Maintainer Comments (${#REVIEW_COMMENTS[@]}):"
  for F in "${REVIEW_COMMENTS[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL COMMENTER <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE — from $COMMENTER"
  done
fi

if [ ${#APPROVED[@]} -gt 0 ]; then
  echo "Approved (${#APPROVED[@]}):"
  for F in "${APPROVED[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE"
  done
fi

if [ ${#MERGED[@]} -gt 0 ]; then
  echo "Recently Merged (${#MERGED[@]}):"
  for F in "${MERGED[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE"
  done
fi

if [ ${#REJECTED[@]} -gt 0 ]; then
  echo "Closed Without Merge (${#REJECTED[@]}):"
  for F in "${REJECTED[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE"
  done
fi

if [ ${#WAITING[@]} -gt 0 ]; then
  echo "Waiting (${#WAITING[@]}):"
  for F in "${WAITING[@]}"; do
    IFS='|' read -r PR_NUM PR_TITLE PR_URL CI_STATUS <<< "$F"
    echo "  PR #$PR_NUM: $PR_TITLE — CI: $CI_STATUS"
  done
fi

echo ""
echo "Beads: $CREATED created, $SKIPPED already tracked"
```

## Record Result

```bash
SUMMARY="$UPSTREAM: $OPEN_COUNT open, ${#CI_FAILING[@]} CI-failing, ${#CHANGES_REQUESTED[@]} changes-requested, ${#REVIEW_COMMENTS[@]} comments, ${#MERGED[@]} merged, ${#REJECTED[@]} rejected, $CREATED bead(s) created"
echo "$SUMMARY"
```

On success:
```bash
bd create "outbound-pr-tracker: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:outbound-pr-tracker,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "outbound-pr-tracker: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:outbound-pr-tracker,result:failure \
  -d "Outbound PR tracker failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: outbound-pr-tracker" \
  --severity medium \
  --reason "$ERROR"
```
