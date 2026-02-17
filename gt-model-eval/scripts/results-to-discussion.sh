#!/usr/bin/env bash
# Format promptfoo eval results as markdown and optionally post to GitHub Discussions.
#
# Usage:
#   ./scripts/results-to-discussion.sh results.json                         # dry-run (stdout)
#   ./scripts/results-to-discussion.sh results.json --post                  # create new Discussion
#   ./scripts/results-to-discussion.sh results.json --comment 1542          # comment on existing Discussion
#   ./scripts/results-to-discussion.sh results.json --post --repo owner/repo
#
# Requires: jq, gh (if --post or --comment)

set -euo pipefail

RESULTS_FILE="${1:?Usage: $0 results.json [--post] [--repo owner/repo]}"
POST=false
COMMENT_NUM=""
REPO="steveyegge/gastown"
CATEGORY="Ideas"

shift || true
while [[ $# -gt 0 ]]; do
  case "$1" in
    --post) POST=true; shift ;;
    --comment) COMMENT_NUM="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --category) CATEGORY="$2"; shift 2 ;;
    *) echo "Unknown flag: $1" >&2; exit 1 ;;
  esac
done

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required" >&2
  exit 1
fi

if ! [ -f "$RESULTS_FILE" ]; then
  echo "Error: $RESULTS_FILE not found" >&2
  exit 1
fi

# --- Extract metadata ---

GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
PROMPTFOO_VER=$(npx promptfoo --version 2>/dev/null || echo "unknown")
DATE=$(date -u +%Y-%m-%d)
USER=$(whoami)

# --- Extract data ---

# Build aggregate summary per provider
generate_summary() {
  local file="$1"
  jq -r '
    [.results.results[] | {provider: .provider.label, success: .success, cost: (.cost // 0), latency: (.latencyMs // 0)}]
    | group_by(.provider)
    | map({
        provider: .[0].provider,
        total: length,
        passed: [.[] | select(.success == true)] | length,
        failed: [.[] | select(.success == false)] | length,
        avg_cost: ([.[].cost] | add / length * 1000 | round / 1000),
        avg_latency: ([.[].latency] | add / length | round)
      })
    | sort_by(.provider)
    | .[] | "\(.provider)|\(.passed)/\(.total)|\(.passed * 100 / .total | round)%|$\(.avg_cost)|\(.avg_latency)ms"
  ' "$file" 2>/dev/null || echo "PARSE_ERROR"
}

# Build per-suite breakdown per provider
# Groups test descriptions by prefix: "[Class A] Deacon..." -> "class-a-deacon", "Clear zombie..." -> "deacon-zombie" etc.
generate_suite_breakdown() {
  local file="$1"
  jq -r '
    # Extract suite name from testCase.description or vars
    [.results.results[] | {
      provider: .provider.label,
      success: .success,
      suite: (
        if ((.testCase.description // "") | test("^\\[Class A\\]")) then
          "class-a-" + .vars.role
        elif (.vars.role // "" | length) > 0 then
          .vars.role + "/" + (.vars.formula_step // "unknown")
        else
          "unknown"
        end
      )
    }]
    | group_by(.suite)
    | map({
        suite: .[0].suite,
        results: (
          group_by(.provider)
          | map({
              provider: .[0].provider,
              passed: ([.[] | select(.success == true)] | length),
              total: length
            })
          | sort_by(.provider)
        )
      })
    | sort_by(.suite)
    | .[] | "\(.suite)|\(.results | map("\(.provider):\(.passed)/\(.total)") | join("|"))"
  ' "$file" 2>/dev/null || echo "PARSE_ERROR"
}

# --- Generate markdown ---

MARKDOWN="$(cat <<HEADER
## Model Comparison Results: ${DATE}

**Reporter**: ${USER}
**Framework commit**: \`${GIT_SHA}\`
**Promptfoo version**: \`${PROMPTFOO_VER}\`
**Related**: [Discussion #1542](https://github.com/${REPO}/discussions/1542) | [Issue #1545](https://github.com/${REPO}/issues/1545)

> **Note**: Cost and latency values may reflect cached results if the eval was re-run. Re-run with \`--no-cache\` for accurate timing data.

### Aggregate Summary

| Model | Tests Passed | Pass Rate | Avg Cost/Test | Avg Latency |
|-------|-------------|-----------|---------------|-------------|
HEADER
)"

# Append aggregate summary rows
SUMMARY=$(generate_summary "$RESULTS_FILE")
if [ -n "$SUMMARY" ] && [ "$SUMMARY" != "PARSE_ERROR" ]; then
  while IFS='|' read -r provider passed rate cost latency; do
    MARKDOWN="${MARKDOWN}
| ${provider} | ${passed} | ${rate} | ${cost} | ${latency} |"
  done <<< "$SUMMARY"
else
  MARKDOWN="${MARKDOWN}
| (could not parse results — check promptfoo version) | | | | |"
fi

# Append per-suite breakdown
MARKDOWN="${MARKDOWN}

### Per-Suite Breakdown

| Suite | $(jq -r '[.results.results[].provider.label] | unique | sort | join(" | ")' "$RESULTS_FILE" 2>/dev/null || echo "Models") |
|-------|$(jq -r '[.results.results[].provider.label] | unique | sort | map("---") | join("|")' "$RESULTS_FILE" 2>/dev/null || echo "---")|"

SUITE_DATA=$(generate_suite_breakdown "$RESULTS_FILE")
if [ -n "$SUITE_DATA" ] && [ "$SUITE_DATA" != "PARSE_ERROR" ]; then
  while IFS='|' read -r suite rest; do
    # Format each provider's pass/total
    formatted=""
    IFS='|' read -ra parts <<< "$rest"
    for part in "${parts[@]}"; do
      # part is "Provider:passed/total"
      score="${part#*:}"
      formatted="${formatted} | ${score}"
    done
    MARKDOWN="${MARKDOWN}
| ${suite} ${formatted} |"
  done <<< "$SUITE_DATA"
else
  MARKDOWN="${MARKDOWN}
| (per-suite breakdown unavailable) | |"
fi

# Append failed test details
FAILED_TESTS=$(jq -r '
  [.results.results[] | select(.success == false) | {
    provider: .provider.label,
    desc: (.testCase.description // (.vars.role + "/" + .vars.formula_step) // "unknown"),
    action: ((.response.output // "") | try (fromjson | .action) catch "PARSE_ERROR")
  }]
  | group_by(.desc)
  | map({
      desc: .[0].desc,
      failures: [.[] | "\(.provider) → \(.action)"] | join(", ")
    })
  | .[:20]
  | .[] | "| \(.desc) | \(.failures) |"
' "$RESULTS_FILE" 2>/dev/null || echo "")

if [ -n "$FAILED_TESTS" ]; then
  MARKDOWN="${MARKDOWN}

### Failed Tests (up to 20)

| Test Description | Failed On |
|-----------------|-----------|
${FAILED_TESTS}"
fi

MARKDOWN="${MARKDOWN}

### How to Reproduce

\`\`\`bash
git clone https://github.com/${REPO}.git
cd \${REPO##*/}/gt-model-eval
git checkout ${GIT_SHA}  # exact framework version
export ANTHROPIC_API_KEY=sk-...
npm ci
npx promptfoo eval --repeat 3
npx promptfoo view
\`\`\`

### How to Share Your Results

\`\`\`bash
npx promptfoo eval --output results.json
./scripts/results-to-discussion.sh results.json --post
\`\`\`

---
*Generated by gt-model-eval results pipeline (commit ${GIT_SHA})*"

# --- Output or post ---

if [ -n "$COMMENT_NUM" ]; then
  # Post as comment on existing discussion
  if ! command -v gh &>/dev/null; then
    echo "Error: gh CLI required for --comment" >&2
    exit 1
  fi

  # Get discussion node ID from number
  DISCUSSION_ID=$(gh api graphql -f query='
    query($owner: String!, $repo: String!, $num: Int!) {
      repository(owner: $owner, name: $repo) {
        discussion(number: $num) { id }
      }
    }
  ' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" -F num="$COMMENT_NUM" \
    --jq '.data.repository.discussion.id' 2>/dev/null)

  if [ -z "$DISCUSSION_ID" ]; then
    echo "Error: could not find Discussion #${COMMENT_NUM}" >&2
    exit 1
  fi

  RESULT=$(gh api graphql -f query='
    mutation($discussionId: ID!, $body: String!) {
      addDiscussionComment(input: {discussionId: $discussionId, body: $body}) {
        comment { url }
      }
    }
  ' -f discussionId="$DISCUSSION_ID" -f body="$MARKDOWN")

  URL=$(echo "$RESULT" | jq -r '.data.addDiscussionComment.comment.url')
  echo "Commented on Discussion #${COMMENT_NUM}: $URL"

elif [ "$POST" = true ]; then
  # Create new discussion
  if ! command -v gh &>/dev/null; then
    echo "Error: gh CLI required for --post" >&2
    exit 1
  fi

  # Get category ID via GraphQL
  CATEGORY_ID=$(gh api graphql -f query='
    query($owner: String!, $repo: String!) {
      repository(owner: $owner, name: $repo) {
        discussionCategories(first: 20) {
          nodes { id name }
        }
      }
    }
  ' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" 2>/dev/null \
    | jq -r --arg cat "$CATEGORY" '.data.repository.discussionCategories.nodes[] | select(.name == $cat) | .id')

  if [ -z "$CATEGORY_ID" ]; then
    echo "Error: could not find Discussion category '${CATEGORY}'" >&2
    exit 1
  fi

  REPO_ID=$(gh api graphql -f query='
    query($owner: String!, $repo: String!) {
      repository(owner: $owner, name: $repo) { id }
    }
  ' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" \
    --jq '.data.repository.id')

  TITLE="Model Comparison Results: ${DATE} (${USER})"

  RESULT=$(gh api graphql -f query='
    mutation($repoId: ID!, $categoryId: ID!, $title: String!, $body: String!) {
      createDiscussion(input: {repositoryId: $repoId, categoryId: $categoryId, title: $title, body: $body}) {
        discussion { url }
      }
    }
  ' -f repoId="$REPO_ID" -f categoryId="$CATEGORY_ID" -f title="$TITLE" -f body="$MARKDOWN")

  URL=$(echo "$RESULT" | jq -r '.data.createDiscussion.discussion.url')
  echo "Posted to: $URL"
else
  echo "$MARKDOWN"
fi
