#!/bin/bash
#
# GitHub PR Sheriff — Categorize and report on open PRs
#
# Standalone executable for the github-sheriff plugin.
# Discovers rigs from rigs.json, fetches open PRs via gh CLI,
# and categorizes them as "easy wins" vs "needs review".
#
# Uses gh CLI for auth, pagination, and rate limiting.
# Uses process substitution (< <(...)) to avoid subshell variable loss.
# Uses gh --json for additions/deletions/mergeable/CI in a single GraphQL call.
#

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Town root discovery ---

find_town_root() {
    local dir="$SCRIPT_DIR"
    while [ "$dir" != "/" ]; do
        if [ -f "$dir/rigs.json" ] || [ -d "$dir/.gt" ]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    if [ -n "${GT_TOWN_ROOT:-}" ]; then
        echo "$GT_TOWN_ROOT"
        return 0
    fi
    return 1
}

TOWN_ROOT="$(find_town_root)" || { echo "ERROR: cannot find town root" >&2; exit 1; }

# --- Rig discovery ---

if [ -f "$TOWN_ROOT/rigs.json" ]; then
    mapfile -t RIGS < <(jq -r 'keys[]' "$TOWN_ROOT/rigs.json" 2>/dev/null)
else
    echo "ERROR: rigs.json not found at $TOWN_ROOT/rigs.json" >&2
    exit 1
fi

# --- Helpers ---

log_info()  { echo "[github-sheriff] $*" >&2; }
log_error() { echo "[github-sheriff] ERROR: $*" >&2; }

# --- Prerequisites ---

if ! gh auth status &>/dev/null; then
    log_info "gh CLI not authenticated — skipping"
    bd wisp create \
        --label type:plugin-run \
        --label plugin:github-sheriff \
        --label result:skipped \
        --body "gh CLI not authenticated — skipping PR categorization"
    exit 0
fi

# --- Main ---

main() {
    local easy_wins=()
    local needs_review=()
    local failures=()
    local errors=()
    local processed_prs=0

    log_info "Starting PR Sheriff patrol..."

    for rig in "${RIGS[@]}"; do
        local repo_path="$TOWN_ROOT/$rig"

        if [ ! -d "$repo_path/.git" ]; then
            continue
        fi

        # Extract owner/repo from git remote
        local remote_url
        remote_url=$(git -C "$repo_path" remote get-url origin 2>/dev/null || echo "")
        [ -z "$remote_url" ] && continue

        local owner_repo
        if [[ $remote_url =~ github\.com[:/]([^/]+/[^/]+?)(.git)?$ ]]; then
            owner_repo="${BASH_REMATCH[1]}"
            owner_repo="${owner_repo%.git}"
        else
            continue
        fi

        log_info "Processing $owner_repo ($rig)..."

        # Single gh call fetches all PR data via GraphQL — no N+1 overhead.
        # Returns additions, deletions, mergeable, and CI check results.
        local prs_json
        prs_json=$(gh pr list --repo "$owner_repo" --state open \
            --json number,title,author,additions,deletions,mergeable,statusCheckRollup,url \
            --limit 100 2>/dev/null) || {
            log_error "Failed to fetch PRs for $owner_repo"
            errors+=("Failed to fetch PRs for $owner_repo")
            continue
        }

        local pr_count
        pr_count=$(echo "$prs_json" | jq 'length')
        [ "$pr_count" -eq 0 ] && continue

        log_info "Found $pr_count open PRs in $rig"

        # Process substitution keeps array modifications in this shell
        while IFS= read -r pr_entry; do
            [ -z "$pr_entry" ] && continue

            local number title author additions deletions mergeable total_changes
            number=$(echo "$pr_entry" | jq -r '.number')
            title=$(echo "$pr_entry" | jq -r '.title')
            author=$(echo "$pr_entry" | jq -r '.author.login')
            additions=$(echo "$pr_entry" | jq -r '.additions // 0')
            deletions=$(echo "$pr_entry" | jq -r '.deletions // 0')
            mergeable=$(echo "$pr_entry" | jq -r '.mergeable')
            total_changes=$((additions + deletions))

            # Determine CI status from statusCheckRollup
            local total_checks passing_checks ci_pass
            total_checks=$(echo "$pr_entry" | jq '.statusCheckRollup | length')
            passing_checks=$(echo "$pr_entry" | jq '[.statusCheckRollup[] | select(
                .conclusion == "SUCCESS" or .conclusion == "NEUTRAL" or
                .conclusion == "SKIPPED" or .state == "SUCCESS"
            )] | length')

            if [ "$total_checks" -gt 0 ] && [ "$total_checks" -eq "$passing_checks" ]; then
                ci_pass=true
            else
                ci_pass=false
            fi

            # Collect individual check failures
            while IFS= read -r check; do
                [ -z "$check" ] && continue
                local check_name
                check_name=$(echo "$check" | jq -r '.name')
                failures+=("$owner_repo|$number|$title|$check_name")
            done < <(echo "$pr_entry" | jq -c '.statusCheckRollup[] | select(
                .conclusion == "FAILURE" or .conclusion == "CANCELLED" or
                .conclusion == "TIMED_OUT" or .state == "FAILURE" or .state == "ERROR"
            )')

            # Categorize PR
            if [ "$mergeable" = "MERGEABLE" ] && $ci_pass && [ "$total_changes" -lt 200 ]; then
                easy_wins+=("[$rig #$number] $title (by $author, +$additions/-$deletions)")
            else
                local reasons=""
                [ "$mergeable" != "MERGEABLE" ] && reasons+="conflicts "
                ! $ci_pass && reasons+="ci-failing "
                [ "$total_changes" -ge 200 ] && reasons+="large(${total_changes}loc) "
                needs_review+=("[$rig #$number] $title (by $author, ${reasons% })")
            fi

            ((processed_prs++)) || true
        done < <(echo "$prs_json" | jq -c '.[]')
    done

    # --- Deduplicate CI failures against existing beads ---

    local existing created=0 skipped=0
    existing=$(bd list --label ci-failure --status open --json 2>/dev/null || echo "[]")

    for f in "${failures[@]}"; do
        local repo pr_num pr_title check_name
        IFS='|' read -r repo pr_num pr_title check_name <<< "$f"
        local bead_title="CI failure: $check_name on PR #$pr_num"

        if echo "$existing" | jq -e --arg t "$bead_title" \
            '.[] | select(.title == $t)' > /dev/null 2>&1; then
            ((skipped++)) || true
            continue
        fi

        local description="CI check \`$check_name\` failed on PR #$pr_num ($pr_title)

PR: https://github.com/$repo/pull/$pr_num"

        local bead_id
        bead_id=$(bd create "$bead_title" -t task -p 2 \
            -d "$description" \
            -l ci-failure \
            --json 2>/dev/null | jq -r '.id // empty')

        if [ -n "$bead_id" ]; then
            ((created++)) || true
            gt activity emit github_check_failed \
                --message "CI check $check_name failed on PR #$pr_num ($repo), bead $bead_id" \
                2>/dev/null || true
        fi
    done

    # --- Report ---

    local easy_count=${#easy_wins[@]}
    local needs_count=${#needs_review[@]}
    local error_count=${#errors[@]}
    local failure_count=${#failures[@]}

    local report="PR Sheriff patrol complete

Statistics:
- Easy Wins: $easy_count
- Needs Review: $needs_count
- CI Failures: $failure_count ($created bead(s) created, $skipped already tracked)
- Errors: $error_count
- Total PRs: $processed_prs"

    if [ "$easy_count" -gt 0 ]; then
        report+=$'\n\nEasy Wins (CI passing, <200 LOC, mergeable):'
        for pr in "${easy_wins[@]}"; do
            report+=$'\n'"  ✅ $pr"
        done
    fi

    if [ "$needs_count" -gt 0 ]; then
        report+=$'\n\nNeeds Review:'
        for pr in "${needs_review[@]}"; do
            report+=$'\n'"  ⚠️ $pr"
        done
    fi

    if [ "$error_count" -gt 0 ]; then
        report+=$'\n\nErrors:'
        for err in "${errors[@]}"; do
            report+=$'\n'"  ❌ $err"
        done
    fi

    # Record result
    if [ "$error_count" -eq 0 ] || [ "$processed_prs" -gt 0 ]; then
        log_info "Patrol complete: $easy_count easy wins, $needs_count need review, $created bead(s) created"
        bd wisp create \
            --label type:plugin-run \
            --label plugin:github-sheriff \
            --label result:success \
            --body "$report" || log_error "Failed to create wisp"
        exit 0
    else
        log_error "Patrol failed with errors"
        bd wisp create \
            --label type:plugin-run \
            --label plugin:github-sheriff \
            --label result:failure \
            --body "$report" || log_error "Failed to create failure wisp"

        gt escalate \
            --severity=low \
            --subject="Plugin FAILED: github-sheriff" \
            --body="$report" \
            --source="plugin:github-sheriff" || log_error "Failed to escalate"
        exit 1
    fi
}

main "$@"
