#!/bin/bash
# Sync forks from GitHub to Dolt database (batch optimized)
# Usage: sync_forks.sh <owner/repo>
#
# Makes minimal API calls: 1 for fork list, then 1 per interesting fork for divergence.

set -euo pipefail

UPSTREAM="${1:-steveyegge/gastown}"
DOLT_DB="/home/ubuntu/gastown9/gastown/.beads/dolt/beads"
TMP_SQL="/tmp/fork_sync_$$.sql"
MIN_SCORE=5  # Threshold for "interesting"

echo "Scanning forks of $UPSTREAM..."

# Get upstream owner and repo
OWNER=$(echo "$UPSTREAM" | cut -d'/' -f1)
REPO=$(echo "$UPSTREAM" | cut -d'/' -f2)

# Single API call - fetch all forks with activity info
# Filter to forks with recent activity (last 180 days)
FORKS=$(gh api "repos/$UPSTREAM/forks" --paginate \
  -q '.[] | select(.pushed_at > (now - 15552000 | strftime("%Y-%m-%dT%H:%M:%SZ"))) | {full_name, pushed_at, default_branch}' \
  2>/dev/null | jq -s '.') || { echo "GitHub API call failed (rate limited?)"; exit 1; }

FORK_COUNT=$(echo "$FORKS" | jq 'length')
echo "Found $FORK_COUNT active forks (pushed in last 180 days)"

if [ "$FORK_COUNT" -eq 0 ]; then
    echo "No active forks to analyze"
    exit 0
fi

# Build batch SQL
echo "-- Auto-generated fork sync $(date -Iseconds)" > "$TMP_SQL"
echo "BEGIN;" >> "$TMP_SQL"

# Process each fork
echo "$FORKS" | jq -c '.[]' | while read -r fork; do
    FORK_NAME=$(echo "$fork" | jq -r '.full_name')
    FORK_BRANCH=$(echo "$fork" | jq -r '.default_branch')
    PUSHED_AT=$(echo "$fork" | jq -r '.pushed_at' | cut -c1-19 | tr 'T' ' ')

    echo "  Analyzing: $FORK_NAME..."

    # Get divergence info (this is the expensive call per fork)
    # Compare upstream:main to fork:default_branch
    COMPARE=$(gh api "repos/$FORK_NAME/compare/$OWNER:main...$FORK_BRANCH" \
      -q '{ahead_by: .ahead_by, behind_by: .behind_by, total_commits: .total_commits, files: [.files[]? | {additions: .additions, deletions: .deletions}]}' \
      2>/dev/null) || COMPARE='{"ahead_by":0,"behind_by":0,"total_commits":0,"files":[]}'

    AHEAD=$(echo "$COMPARE" | jq -r '.ahead_by // 0')
    BEHIND=$(echo "$COMPARE" | jq -r '.behind_by // 0')
    ADDITIONS=$(echo "$COMPARE" | jq '[.files[].additions] | add // 0')
    DELETIONS=$(echo "$COMPARE" | jq '[.files[].deletions] | add // 0')

    # Get last commit info
    LAST_COMMIT=$(gh api "repos/$FORK_NAME/commits/$FORK_BRANCH" \
      -q '{sha: .sha, date: .commit.author.date}' \
      2>/dev/null) || LAST_COMMIT='{"sha":"unknown","date":"1970-01-01T00:00:00Z"}'

    LAST_SHA=$(echo "$LAST_COMMIT" | jq -r '.sha[:12]')
    LAST_DATE=$(echo "$LAST_COMMIT" | jq -r '.date' | cut -c1-19 | tr 'T' ' ')

    # Calculate score (matches formula heuristics)
    SCORE=0
    [ "$AHEAD" -gt 100 ] && SCORE=$((SCORE + 10))
    [ "$AHEAD" -gt 50 ] && [ "$AHEAD" -le 100 ] && SCORE=$((SCORE + 5))
    [ "$AHEAD" -gt 20 ] && [ "$AHEAD" -le 50 ] && SCORE=$((SCORE + 3))

    # Recent activity bonus (last 7 days = +3, last 30 days = +2)
    DAYS_AGO=$(( ($(date +%s) - $(date -d "$LAST_DATE" +%s)) / 86400 ))
    [ "$DAYS_AGO" -lt 7 ] && SCORE=$((SCORE + 3))
    [ "$DAYS_AGO" -ge 7 ] && [ "$DAYS_AGO" -lt 30 ] && SCORE=$((SCORE + 2))

    IS_INTERESTING=0
    [ "$SCORE" -ge "$MIN_SCORE" ] && IS_INTERESTING=1

    echo "    ahead=$AHEAD, score=$SCORE, interesting=$IS_INTERESTING"

    # Escape fork name for SQL
    FORK_NAME_ESC=$(echo "$FORK_NAME" | sed "s/'/''/g")

    cat >> "$TMP_SQL" << EOF
INSERT INTO github_forks (
    upstream_repo, fork_repo, commits_ahead, commits_behind,
    additions, deletions, last_commit_sha, last_commit_at,
    fork_pushed_at, score, is_interesting, last_scanned_at
) VALUES (
    '$UPSTREAM', '$FORK_NAME_ESC', $AHEAD, $BEHIND,
    $ADDITIONS, $DELETIONS, '$LAST_SHA', '$LAST_DATE',
    '$PUSHED_AT', $SCORE, $IS_INTERESTING, NOW()
)
ON DUPLICATE KEY UPDATE
    commits_ahead = $AHEAD,
    commits_behind = $BEHIND,
    additions = $ADDITIONS,
    deletions = $DELETIONS,
    last_commit_sha = '$LAST_SHA',
    last_commit_at = '$LAST_DATE',
    fork_pushed_at = '$PUSHED_AT',
    score = $SCORE,
    is_interesting = $IS_INTERESTING,
    last_scanned_at = NOW();
EOF
done

echo "COMMIT;" >> "$TMP_SQL"

# Execute batch SQL
echo ""
echo "Executing batch SQL..."
cd "$DOLT_DB" && dolt sql < "$TMP_SQL"

# Summary
echo ""
echo "=== Summary for forks of $UPSTREAM ==="
cd "$DOLT_DB" && dolt sql -q "
SELECT
    COUNT(*) AS total_forks,
    SUM(CASE WHEN is_interesting = 1 THEN 1 ELSE 0 END) AS interesting,
    SUM(commits_ahead) AS total_commits_ahead,
    SUM(additions + deletions) AS total_lines_changed,
    MAX(score) AS max_score
FROM github_forks
WHERE upstream_repo = '$UPSTREAM';
"

# Show interesting forks
INTERESTING=$(cd "$DOLT_DB" && dolt sql -q "SELECT COUNT(*) FROM github_forks WHERE upstream_repo = '$UPSTREAM' AND is_interesting = 1;" -r csv | tail -1)
if [ "$INTERESTING" -gt 0 ]; then
    echo ""
    echo "=== Interesting Forks (score >= $MIN_SCORE) ==="
    cd "$DOLT_DB" && dolt sql -q "
    SELECT fork_repo, commits_ahead, total_lines, score,
           DATEDIFF(NOW(), last_commit_at) AS days_since_commit
    FROM github_forks
    WHERE upstream_repo = '$UPSTREAM' AND is_interesting = 1
    ORDER BY score DESC, commits_ahead DESC
    LIMIT 10;
    "
fi

rm -f "$TMP_SQL"
echo ""
echo "Done!"
