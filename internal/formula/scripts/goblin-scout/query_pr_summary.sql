-- Get summary stats for a repo's open PRs
SELECT
    COUNT(*) AS total_open,
    SUM(CASE WHEN is_stuck = 1 THEN 1 ELSE 0 END) AS stuck_count,
    SUM(CASE WHEN is_stale = 1 THEN 1 ELSE 0 END) AS stale_count,
    SUM(CASE WHEN is_large = 1 THEN 1 ELSE 0 END) AS large_count,
    SUM(CASE WHEN is_draft = 1 THEN 1 ELSE 0 END) AS draft_count,
    SUM(CASE WHEN is_draft = 0 AND age_days <= 7 THEN 1 ELSE 0 END) AS ready_count
FROM github_prs
WHERE repo = '$REPO'
  AND state = 'open';
