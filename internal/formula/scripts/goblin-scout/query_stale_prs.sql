-- Get stale PRs (no activity in >30 days)
SELECT
    pr_number,
    title,
    author,
    age_days,
    stale_days,
    total_lines AS size
FROM github_prs
WHERE repo = '$REPO'
  AND state = 'open'
  AND is_stale = 1
ORDER BY stale_days DESC;
