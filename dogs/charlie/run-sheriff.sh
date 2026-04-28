#!/bin/bash
set -e

# Verify gh CLI is available
gh auth status 2>/dev/null || { echo "SKIP: gh CLI not authenticated"; exit 0; }

# List of worktree roots to check
WORKROOT="/Users/athos/gt/deacon/dogs/charlie"
RIGS=(
  "gastown"
  "property_scrapers" 
  "whatsapp_automation"
)

TOTAL_EASY=0
TOTAL_NEEDS_REVIEW=0
TOTAL_FAILURES=0
TOTAL_CREATED=0
TOTAL_SKIPPED=0

for RIG in "${RIGS[@]}"; do
  RIG_PATH="$WORKROOT/$RIG"
  [ ! -d "$RIG_PATH" ] && continue
  
  echo "=== Processing $RIG ==="
  
  # Detect repo from git remote
  REPO=$(git -C "$RIG_PATH" remote get-url origin 2>/dev/null | sed -E 's|.*github\.com[:/]||; s|\.git$||' || true)
  [ -z "$REPO" ] && { echo "SKIP: could not detect GitHub repo from $RIG remote"; continue; }
  
  echo "Repo: $REPO"
  
  # Get open PRs from last 7 days
  SINCE=$(date -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -v-7d +%Y-%m-%dT%H:%M:%SZ)
  PRS=$(gh pr list --repo "$REPO" --state open \
    --json number,title,author,additions,deletions,mergeable,statusCheckRollup,url,updatedAt \
    --limit 100 2>/dev/null | jq --arg since "$SINCE" '[.[] | select(.updatedAt >= $since)]' || echo "[]")
  
  PR_COUNT=$(echo "$PRS" | jq length)
  [ "$PR_COUNT" -eq 0 ] && { echo "No open PRs"; continue; }
  echo "Found $PR_COUNT open PRs"
  
  # Categorize PRs
  EASY_WINS=()
  NEEDS_REVIEW=()
  FAILURES=()
  
  while IFS= read -r PR_JSON; do
    [ -z "$PR_JSON" ] && continue
    
    PR_NUM=$(echo "$PR_JSON" | jq -r '.number')
    PR_TITLE=$(echo "$PR_JSON" | jq -r '.title')
    AUTHOR=$(echo "$PR_JSON" | jq -r '.author.login')
    ADDITIONS=$(echo "$PR_JSON" | jq -r '.additions // 0')
    DELETIONS=$(echo "$PR_JSON" | jq -r '.deletions // 0')
    MERGEABLE=$(echo "$PR_JSON" | jq -r '.mergeable')
    TOTAL_CHANGES=$((ADDITIONS + DELETIONS))
    
    # Check CI status
    TOTAL_CHECKS=$(echo "$PR_JSON" | jq '.statusCheckRollup | length')
    PASSING_CHECKS=$(echo "$PR_JSON" | jq '[.statusCheckRollup[] | select(.conclusion == "SUCCESS" or .conclusion == "NEUTRAL" or .conclusion == "SKIPPED" or .state == "SUCCESS")] | length')
    
    if [ "$TOTAL_CHECKS" -gt 0 ] && [ "$TOTAL_CHECKS" -eq "$PASSING_CHECKS" ]; then
      CI_PASS=true
    else
      CI_PASS=false
    fi
    
    # Collect failures
    while IFS= read -r CHECK; do
      [ -z "$CHECK" ] && continue
      CHECK_NAME=$(echo "$CHECK" | jq -r '.name')
      CHECK_URL=$(echo "$CHECK" | jq -r '.detailsUrl // .targetUrl // empty')
      FAILURES+=("$PR_NUM|$PR_TITLE|$CHECK_NAME|$CHECK_URL")
    done < <(echo "$PR_JSON" | jq -c '.statusCheckRollup[] | select(.conclusion == "FAILURE" or .conclusion == "CANCELLED" or .conclusion == "TIMED_OUT" or .state == "FAILURE" or .state == "ERROR")')
    
    # Categorize
    if [ "$MERGEABLE" = "MERGEABLE" ] && [ "$CI_PASS" = true ] && [ "$TOTAL_CHANGES" -lt 200 ]; then
      EASY_WINS+=("PR #$PR_NUM: $PR_TITLE (by $AUTHOR, +$ADDITIONS/-$DELETIONS)")
    else
      REASONS=""
      [ "$MERGEABLE" != "MERGEABLE" ] && REASONS+="conflicts "
      [ "$CI_PASS" != true ] && REASONS+="ci-failing "
      [ "$TOTAL_CHANGES" -ge 200 ] && REASONS+="large(${TOTAL_CHANGES}loc) "
      NEEDS_REVIEW+=("PR #$PR_NUM: $PR_TITLE (by $AUTHOR, ${REASONS% })")
    fi
  done < <(echo "$PRS" | jq -c '.[]')
  
  # Report categorization
  [ ${#EASY_WINS[@]} -gt 0 ] && echo "✓ Easy wins (${#EASY_WINS[@]}): ${EASY_WINS[@]}"
  [ ${#NEEDS_REVIEW[@]} -gt 0 ] && echo "⚠ Needs review (${#NEEDS_REVIEW[@]}): ${NEEDS_REVIEW[@]}"
  [ ${#FAILURES[@]} -gt 0 ] && echo "✗ CI failures: ${#FAILURES[@]}"
  
  # Create beads for failures
  RIG_FLAG="--rig $RIG"
  REPO_OWNER=$(echo "$REPO" | cut -d'/' -f1)
  
  CREATED=0
  SKIPPED=0
  
  if [ "$REPO_OWNER" != "athosmartins" ]; then
    echo "Skipping CI failure beads for upstream repo $REPO"
    SKIPPED=${#FAILURES[@]}
  else
    EXISTING=$(bd list --label ci-failure --status open $RIG_FLAG --json 2>/dev/null || echo "[]")
    
    for F in "${FAILURES[@]}"; do
      IFS='|' read -r PR_NUM PR_TITLE CHECK_NAME CHECK_URL <<< "$F"
      BEAD_TITLE="CI failure: $CHECK_NAME on PR #$PR_NUM"
      
      if echo "$EXISTING" | jq -e --arg t "$BEAD_TITLE" '.[] | select(.title == $t)' > /dev/null 2>&1; then
        SKIPPED=$((SKIPPED + 1))
        continue
      fi
      
      DESCRIPTION="CI check \`$CHECK_NAME\` failed on PR #$PR_NUM ($PR_TITLE)

PR: https://github.com/$REPO/pull/$PR_NUM"
      [ -n "$CHECK_URL" ] && DESCRIPTION="$DESCRIPTION
Check: $CHECK_URL"
      
      BEAD_ID=$(bd create "$BEAD_TITLE" -t task -p 2 -d "$DESCRIPTION" -l ci-failure $RIG_FLAG --json 2>/dev/null | jq -r '.id // empty' || true)
      
      if [ -n "$BEAD_ID" ]; then
        CREATED=$((CREATED + 1))
        gt activity emit github_check_failed --message "CI check $CHECK_NAME failed on PR #$PR_NUM ($REPO), bead $BEAD_ID" 2>/dev/null || true
      fi
    done
  fi
  
  echo "$RIG result: $PR_COUNT PRs — ${#EASY_WINS[@]} easy, ${#NEEDS_REVIEW[@]} review, $CREATED bead(s), $SKIPPED skipped"
  
  TOTAL_EASY=$((TOTAL_EASY + ${#EASY_WINS[@]}))
  TOTAL_NEEDS_REVIEW=$((TOTAL_NEEDS_REVIEW + ${#NEEDS_REVIEW[@]}))
  TOTAL_FAILURES=$((TOTAL_FAILURES + ${#FAILURES[@]}))
  TOTAL_CREATED=$((TOTAL_CREATED + CREATED))
  TOTAL_SKIPPED=$((TOTAL_SKIPPED + SKIPPED))
done

SUMMARY="GitHub Sheriff: ${TOTAL_EASY} easy, ${TOTAL_NEEDS_REVIEW} need review, ${TOTAL_FAILURES} failures — ${TOTAL_CREATED} bead(s) created, ${TOTAL_SKIPPED} skipped"
echo "=== $SUMMARY ==="

bd create "github-sheriff: run complete" -t chore --ephemeral -l type:plugin-run,plugin:github-sheriff,result:success -d "$SUMMARY" --silent 2>/dev/null || true
