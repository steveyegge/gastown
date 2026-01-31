#!/bin/bash
# Sync PRs from GitHub to Dolt database (batch optimized)
# Usage: sync_prs.sh <owner/repo>
#
# Uses REST API (not GraphQL) to avoid separate rate limit exhaustion.
# Makes 1 API call to list PRs, then 1 call per PR for details.

set -euo pipefail

REPO="${1:-steveyegge/gastown}"
DOLT_DB="${GT_ROOT:?GT_ROOT must be set}/.beads/dolt/beads"
TMP_SQL="/tmp/pr_sync_$$.sql"

echo "Syncing open PRs from $REPO..."

# REST API call - fetch open PRs (basic info)
PRS=$(gh api "repos/$REPO/pulls?state=open&per_page=100" --paginate 2>/dev/null) || {
    echo "GitHub REST API call failed (rate limited?)"
    exit 1
}

# Count PRs
PR_COUNT=$(echo "$PRS" | jq 'length')
echo "Found $PR_COUNT open PRs"

if [ "$PR_COUNT" -eq 0 ]; then
    echo "No open PRs to sync"
    exit 0
fi

# Build batch SQL file
echo "-- Auto-generated PR sync $(date -Iseconds)" > "$TMP_SQL"
echo "BEGIN;" >> "$TMP_SQL"

echo "$PRS" | jq -c '.[]' | while read -r pr; do
    NUMBER=$(echo "$pr" | jq -r '.number')

    # Get detailed PR info (includes additions/deletions)
    DETAIL=$(gh api "repos/$REPO/pulls/$NUMBER" 2>/dev/null) || DETAIL="$pr"

    # Escape single quotes and backslashes for SQL
    TITLE=$(echo "$DETAIL" | jq -r '.title' | sed "s/'/''/g; s/\\\\/\\\\\\\\/g")
    AUTHOR=$(echo "$DETAIL" | jq -r '.user.login // "unknown"')
    CREATED=$(echo "$DETAIL" | jq -r '.created_at' | cut -c1-19 | tr 'T' ' ')
    UPDATED=$(echo "$DETAIL" | jq -r '.updated_at' | cut -c1-19 | tr 'T' ' ')
    ADDITIONS=$(echo "$DETAIL" | jq -r '.additions // 0')
    DELETIONS=$(echo "$DETAIL" | jq -r '.deletions // 0')
    FILES=$(echo "$DETAIL" | jq -r '.changed_files // 0')
    DRAFT=$(echo "$DETAIL" | jq -r 'if .draft then 1 else 0 end')

    # Get review count via separate endpoint
    REVIEWS=$(gh api "repos/$REPO/pulls/$NUMBER/reviews" -q 'length' 2>/dev/null) || REVIEWS=0

    echo "  PR #$NUMBER: $AUTHOR - $(echo "$TITLE" | head -c 50)..."

    cat >> "$TMP_SQL" << EOF
INSERT INTO github_prs (
    repo, pr_number, title, author, state, is_draft,
    additions, deletions, changed_files, review_count,
    gh_created_at, gh_updated_at, last_scanned_at
) VALUES (
    '$REPO', $NUMBER, '$TITLE', '$AUTHOR', 'open', $DRAFT,
    $ADDITIONS, $DELETIONS, $FILES, $REVIEWS,
    '$CREATED', '$UPDATED', NOW()
)
ON DUPLICATE KEY UPDATE
    title = '$TITLE',
    state = 'open',
    is_draft = $DRAFT,
    additions = $ADDITIONS,
    deletions = $DELETIONS,
    changed_files = $FILES,
    review_count = $REVIEWS,
    gh_updated_at = '$UPDATED',
    last_scanned_at = NOW();
EOF
done

# Classification updates (batch)
cat >> "$TMP_SQL" << EOF

-- Classify: stuck (no reviews, >7 days old)
UPDATE github_prs
SET is_stuck = CASE
    WHEN state = 'open' AND review_count = 0 AND DATEDIFF(NOW(), gh_created_at) > 7 THEN 1
    ELSE 0
END
WHERE repo = '$REPO' AND state = 'open';

-- Classify: stale (no activity in >30 days)
UPDATE github_prs
SET is_stale = CASE
    WHEN state = 'open' AND DATEDIFF(NOW(), gh_updated_at) > 30 THEN 1
    ELSE 0
END
WHERE repo = '$REPO' AND state = 'open';

-- Classify: large (>500 total lines)
UPDATE github_prs
SET is_large = CASE
    WHEN (additions + deletions) > 500 THEN 1
    ELSE 0
END
WHERE repo = '$REPO' AND state = 'open';

-- Mark PRs not seen this scan as potentially closed
UPDATE github_prs
SET state = 'maybe_closed'
WHERE repo = '$REPO'
  AND state = 'open'
  AND last_scanned_at < DATE_SUB(NOW(), INTERVAL 1 MINUTE);

COMMIT;
EOF

# Single SQL execution
echo "Executing batch SQL..."
cd "$DOLT_DB" && dolt sql < "$TMP_SQL"

# Summary query
echo ""
echo "=== Summary for $REPO ==="
cd "$DOLT_DB" && dolt sql -q "
SELECT
    COUNT(*) AS total_open,
    SUM(CASE WHEN is_stuck = 1 THEN 1 ELSE 0 END) AS stuck,
    SUM(CASE WHEN is_stale = 1 THEN 1 ELSE 0 END) AS stale,
    SUM(CASE WHEN is_large = 1 THEN 1 ELSE 0 END) AS large,
    SUM(CASE WHEN is_draft = 1 THEN 1 ELSE 0 END) AS draft
FROM github_prs
WHERE repo = '$REPO' AND state = 'open';
"

# Show PRs needing attention
STUCK=$(cd "$DOLT_DB" && dolt sql -q "SELECT COUNT(*) FROM github_prs WHERE repo = '$REPO' AND is_stuck = 1;" -r csv | tail -1)
if [ "$STUCK" -gt 0 ]; then
    echo ""
    echo "=== Stuck PRs (no reviews, >7 days) ==="
    cd "$DOLT_DB" && dolt sql -q "
    SELECT pr_number AS '#', title, author,
           DATEDIFF(NOW(), gh_created_at) AS age_days,
           (additions + deletions) AS total_lines
    FROM github_prs
    WHERE repo = '$REPO' AND is_stuck = 1
    ORDER BY gh_created_at ASC
    LIMIT 10;
    "
fi

rm -f "$TMP_SQL"
echo ""
echo "Done! (1 API call, 1 SQL transaction)"
