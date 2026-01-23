-- Classify PRs by various criteria
-- Run after upserting PR data to update classification flags

-- Mark stuck PRs: open, no reviews, >7 days old
UPDATE github_prs
SET is_stuck = CASE
    WHEN state = 'open' AND review_count = 0 AND age_days > 7 THEN 1
    ELSE 0
END
WHERE state = 'open';

-- Mark stale PRs: open, no activity in >30 days
UPDATE github_prs
SET is_stale = CASE
    WHEN state = 'open' AND stale_days > 30 THEN 1
    ELSE 0
END
WHERE state = 'open';

-- Mark large PRs: >500 total lines changed
UPDATE github_prs
SET is_large = CASE
    WHEN total_lines > 500 THEN 1
    ELSE 0
END
WHERE state = 'open';
