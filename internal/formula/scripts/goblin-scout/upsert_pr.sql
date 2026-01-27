-- Upsert a PR into the github_prs table
-- Parameters: $REPO, $NUMBER, $TITLE, $AUTHOR, $STATE, $DRAFT, $ADDITIONS, $DELETIONS, $FILES, $REVIEWS, $CREATED, $UPDATED
INSERT INTO github_prs (
    repo, pr_number, title, author, state, is_draft,
    additions, deletions, changed_files, review_count,
    gh_created_at, gh_updated_at, last_scanned_at
) VALUES (
    '$REPO', $NUMBER, '$TITLE', '$AUTHOR', '$STATE', $DRAFT,
    $ADDITIONS, $DELETIONS, $FILES, $REVIEWS,
    '$CREATED', '$UPDATED', NOW()
)
ON DUPLICATE KEY UPDATE
    title = '$TITLE',
    state = '$STATE',
    is_draft = $DRAFT,
    additions = $ADDITIONS,
    deletions = $DELETIONS,
    changed_files = $FILES,
    review_count = $REVIEWS,
    gh_updated_at = '$UPDATED',
    last_scanned_at = NOW();
