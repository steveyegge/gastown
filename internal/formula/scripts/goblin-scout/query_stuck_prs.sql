-- Get stuck PRs (no reviews, >7 days old)
SELECT
    pr_number,
    title,
    author,
    age_days,
    total_lines AS size,
    is_draft
FROM github_prs
WHERE repo = '$REPO'
  AND state = 'open'
  AND is_stuck = 1
ORDER BY age_days DESC;
