#!/bin/bash
#
# GitHub PR Sheriff - Categorize and report on open PRs
#
# This plugin monitors open pull requests across Gas Town rigs,
# categorizes them by complexity/readiness, and flags for review.
#

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly GITHUB_TOKEN="${GITHUB_TOKEN:-}"

# Discover town root — walk up from script dir until we find .gt or rigs.json
find_town_root() {
    local dir="$SCRIPT_DIR"
    while [ "$dir" != "/" ]; do
        if [ -f "$dir/rigs.json" ] || [ -d "$dir/.gt" ]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    # Fallback: GT_TOWN_ROOT env var
    if [ -n "${GT_TOWN_ROOT:-}" ]; then
        echo "$GT_TOWN_ROOT"
        return 0
    fi
    return 1
}

TOWN_ROOT="$(find_town_root)" || { echo "ERROR: cannot find town root" >&2; exit 1; }

# Discover rigs dynamically from rigs.json
if [ -f "$TOWN_ROOT/rigs.json" ]; then
    mapfile -t RIGS < <(jq -r 'keys[]' "$TOWN_ROOT/rigs.json" 2>/dev/null)
else
    echo "ERROR: rigs.json not found at $TOWN_ROOT/rigs.json" >&2
    exit 1
fi

# Color codes for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m' # No Color

log_info() {
    echo "[INFO] $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR] $*${NC}" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS] $*${NC}" >&2
}

# Check prerequisites
if [ -z "$GITHUB_TOKEN" ]; then
    log_info "GitHub token not configured - skipping PR categorization"
    bd wisp create \
        --label type:plugin-run \
        --label plugin:github-sheriff \
        --label result:skipped \
        --body "GitHub token not configured (GITHUB_TOKEN env var) - skipping PR categorization"
    exit 0
fi

# Main plugin logic
main() {
    local easy_wins=()
    local needs_review=()
    local errors=()
    local processed_prs=0

    log_info "Starting GitHub PR Sheriff patrol..."

    # Iterate through all rigs
    for rig in "${RIGS[@]}"; do
        local repo_path="$TOWN_ROOT/$rig"

        # Verify repository exists (rig root should be a git clone)
        if [ ! -d "$repo_path/.git" ]; then
            log_info "Repository not found or not a git repo: $repo_path"
            continue
        fi

        # Extract GitHub owner/repo from git remote
        local remote_url
        remote_url=$(cd "$repo_path" && git config --get remote.origin.url 2>/dev/null || echo "")

        if [ -z "$remote_url" ]; then
            log_info "No remote URL for rig: $rig"
            continue
        fi

        # Parse owner and repo from various git URL formats
        local owner repo
        if [[ $remote_url =~ github.com[:/]([^/]+)/(.+?)(.git)?$ ]]; then
            owner="${BASH_REMATCH[1]}"
            repo="${BASH_REMATCH[2]}"
            repo="${repo%.git}"
        else
            log_info "Could not parse GitHub URL for $rig: $remote_url"
            continue
        fi

        log_info "Processing $owner/$repo from rig $rig..."

        # Fetch open PRs
        local prs_response
        prs_response=$(curl -s \
            -H "Authorization: token $GITHUB_TOKEN" \
            -H "Accept: application/vnd.github.v3+json" \
            "https://api.github.com/repos/$owner/$repo/pulls?state=open&sort=updated&direction=desc&per_page=100" 2>/dev/null || echo "")

        if [ -z "$prs_response" ]; then
            log_error "Failed to fetch PRs for $owner/$repo"
            errors+=("Failed to fetch PRs for $owner/$repo")
            continue
        fi

        # Check if response is an error
        if echo "$prs_response" | grep -q '"message"'; then
            local error_msg
            error_msg=$(echo "$prs_response" | jq -r '.message // "Unknown error"')
            log_error "GitHub API error for $owner/$repo: $error_msg"
            errors+=("$owner/$repo: $error_msg")
            continue
        fi

        # Process each PR
        local pr_count
        pr_count=$(echo "$prs_response" | jq -r 'length')
        if [ "$pr_count" -eq 0 ]; then
            log_info "No open PRs in $rig ($owner/$repo)"
            continue
        fi

        log_info "Found $pr_count open PRs in $rig"

        echo "$prs_response" | jq -r '.[] | @json' | while read -r pr_json; do
            local pr_number pr_title author additions deletions
            pr_number=$(echo "$pr_json" | jq -r '.number')
            pr_title=$(echo "$pr_json" | jq -r '.title')
            author=$(echo "$pr_json" | jq -r '.user.login')
            additions=$(echo "$pr_json" | jq -r '.additions // 0')
            deletions=$(echo "$pr_json" | jq -r '.deletions // 0')

            local total_changes=$((additions + deletions))
            local pr_key="$rig#$pr_number"

            # Fetch detailed PR info and check CI status
            local details
            details=$(curl -s \
                -H "Authorization: token $GITHUB_TOKEN" \
                -H "Accept: application/vnd.github.v3+json" \
                "https://api.github.com/repos/$owner/$repo/pulls/$pr_number" 2>/dev/null || echo "")

            local mergeable ci_status
            mergeable=$(echo "$details" | jq -r '.mergeable // false')
            ci_status=$(echo "$details" | jq -r '.status // "unknown"')

            # Fetch commit status
            local commit_sha
            commit_sha=$(echo "$details" | jq -r '.head.sha // ""')

            local checks_status="unknown"
            if [ -n "$commit_sha" ]; then
                local check_response
                check_response=$(curl -s \
                    -H "Authorization: token $GITHUB_TOKEN" \
                    -H "Accept: application/vnd.github.v3+json" \
                    "https://api.github.com/repos/$owner/$repo/commits/$commit_sha/status" 2>/dev/null || echo "")
                checks_status=$(echo "$check_response" | jq -r '.state // "unknown"')
            fi

            # Categorize PR
            local category=""
            if [ "$mergeable" = "true" ] && [ "$checks_status" = "success" ] && [ $total_changes -lt 200 ]; then
                category="easy_win"
                easy_wins+=("[$rig #$pr_number] $pr_title (by $author, +$additions-$deletions)")
            else
                category="needs_review"
                local status_reason="mergeable=$mergeable, ci=$checks_status, changes=$total_changes"
                needs_review+=("[$rig #$pr_number] $pr_title (by $author, $status_reason)")
            fi

            ((processed_prs++))
        done
    done

    # Generate report
    local easy_count=${#easy_wins[@]}
    local needs_count=${#needs_review[@]}
    local error_count=${#errors[@]}

    # Build report body
    local report_body="PR Sheriff patrol completed

**Statistics:**
- Easy Wins (ready to merge): $easy_count
- Need Review (waiting on humans): $needs_count
- Errors: $error_count
- Total PRs processed: $processed_prs"

    if [ $easy_count -gt 0 ]; then
        report_body+="

**Easy Wins (CI passing, small, mergeable):**"
        for pr in "${easy_wins[@]}"; do
            report_body+="
  ✅ $pr"
        done
    fi

    if [ $needs_count -gt 0 ]; then
        report_body+="

**Needs Review (CI failing, large, or conflicts):**"
        for pr in "${needs_review[@]}"; do
            report_body+="
  ⚠️ $pr"
        done
    fi

    if [ $error_count -gt 0 ]; then
        report_body+="

**Errors during patrol:**"
        for err in "${errors[@]}"; do
            report_body+="
  ❌ $err"
        done
    fi

    # Record result
    if [ $error_count -eq 0 ] || [ $processed_prs -gt 0 ]; then
        log_success "PR Sheriff patrol complete: $easy_count easy wins, $needs_count need review"
        bd wisp create \
            --label type:plugin-run \
            --label plugin:github-sheriff \
            --label result:success \
            --body "$report_body" || log_error "Failed to create success wisp"
        exit 0
    else
        log_error "PR Sheriff patrol failed with errors"
        bd wisp create \
            --label type:plugin-run \
            --label plugin:github-sheriff \
            --label result:failure \
            --body "$report_body" || log_error "Failed to create failure wisp"

        gt escalate \
            --severity=medium \
            --subject="Plugin FAILED: github-sheriff" \
            --body="$report_body" \
            --source="plugin:github-sheriff" || log_error "Failed to escalate"
        exit 1
    fi
}

# Execute main function
main "$@"
