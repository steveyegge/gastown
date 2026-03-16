# PR-Based Contribution Flow for Wasteland Phase 2

**Wasteland Item:** w-wl-001
**Type:** Feature Design + Implementation Plan
**Priority:** P2
**Author:** gastown/crew/zhora (dreadpiraterobertz)
**Date:** 2026-03-15

## Problem

Phase 1 (wild-west mode) writes claims, posts, and completions directly to
the local fork's main branch. This creates several problems:

1. **Invisible claims** — other rigs can't see your claim until you manually
   create a DoltHub PR from your fork
2. **Duplicate claims** — two rigs independently claim the same item with no
   conflict detection until forks are reconciled
3. **No review gate** — anyone can post/claim/complete without validation
4. **Trust enforcement impossible** — can't gate operations by trust level
   when writes are local-only

## Solution: DoltHub PR-Based Writes

Replace direct writes to main with a branch-then-PR workflow:

```
Phase 1 (current):
  claim → UPDATE local main → push to fork origin

Phase 2 (proposed):
  claim → checkout branch → UPDATE → commit → push branch → create DoltHub PR
```

Each mutating operation (claim, post, done) creates a short-lived branch on
the user's fork, commits the change there, pushes the branch, and opens a
DoltHub PR against the upstream commons. The PR is immediately visible to
all participants and can be auto-merged or gated by trust level.

## Design

### Branch Naming Convention

```
wl/<operation>/<item-id>/<timestamp>
```

Examples:
- `wl/claim/w-gt-005/1710532800`
- `wl/post/w-new-item/1710532801`
- `wl/done/w-gt-005/1710532802`

Timestamp suffix prevents branch name collisions when re-claiming after a
withdrawal or when operations are retried.

### Operation Flow (Claim Example)

```
1. dolt checkout -b wl/claim/w-gt-005/... (from main)
2. UPDATE wanted SET claimed_by=..., status='claimed'
3. CALL DOLT_ADD('-A')
4. CALL DOLT_COMMIT('-m', 'wl claim: w-gt-005 by dreadpiraterobertz')
5. dolt push origin wl/claim/w-gt-005/...
6. POST /api/v1alpha1/{upstream-owner}/{db}/pulls
   {
     "title": "claim: w-gt-005 by dreadpiraterobertz",
     "description": "Claiming wanted item w-gt-005",
     "fromBranchOwnerName": "dreadpiraterobertz",
     "fromBranchRepoName": "wl-commons",
     "fromBranchName": "wl/claim/w-gt-005/...",
     "toBranchOwnerName": "steveyegge",
     "toBranchRepoName": "wl-commons",
     "toBranchName": "main"
   }
7. dolt checkout main (return to main)
8. Print PR URL for user
```

### Auto-Merge Policy

For Phase 2, claims and posts can be auto-merged immediately (no human
review needed). Completions may require validation depending on trust level.

| Operation | Auto-merge? | Gate |
|-----------|-------------|------|
| claim     | Yes         | None (first-come-first-served) |
| post      | Yes         | None |
| done      | Configurable| Trust level >= 2: auto-merge; < 2: requires stamp |

Auto-merge uses the DoltHub merge API:
```
POST /api/v1alpha1/{owner}/{db}/pulls/{pull_id}/merge
```

Then poll for completion:
```
GET /api/v1alpha1/{owner}/{db}/pulls/{pull_id}/merge
```

### Configuration

Add `pr_mode` to wasteland config (`mayor/wasteland.json`):

```json
{
  "upstream": "steveyegge/wl-commons",
  "fork_org": "dreadpiraterobertz",
  "fork_db": "wl-commons",
  "local_dir": "/path/to/.wasteland/steveyegge/wl-commons",
  "rig_handle": "dreadpiraterobertz",
  "joined_at": "2026-03-15T00:00:00Z",
  "pr_mode": true,
  "auto_merge": {
    "claim": true,
    "post": true,
    "done": false
  }
}
```

When `pr_mode` is false (default), existing wild-west behavior is preserved.
This makes the migration opt-in and backward compatible.

### Conflict Resolution

DoltHub PRs can conflict if two rigs claim the same item simultaneously.
The merge API returns a conflict error. Resolution:

1. **Claims:** First PR to merge wins. Second PR detects conflict, syncs
   upstream, and either withdraws or reports "already claimed."
2. **Posts:** No conflicts possible (unique IDs per item).
3. **Completions:** One completion per wanted item (DB constraint). Second
   completion PR fails at merge — reporter notified.

### Sync Integration

`gt wl sync` already pulls upstream into local main. In PR mode, sync also
cleans up merged branches:

```
dolt branch -D wl/claim/w-gt-005/...  (if PR merged)
```

## Implementation Plan

### New Code: `internal/doltserver/dolthub_pr.go`

DoltHub PR API client functions, following the existing pattern in
`dolthub.go`:

```go
// CreateDoltHubPR creates a pull request on DoltHub.
func CreateDoltHubPR(owner, db, token string, pr *PRRequest) (*PRResponse, error)

// MergeDoltHubPR triggers an async merge of a DoltHub PR.
func MergeDoltHubPR(owner, db, token string, pullID int) error

// PollDoltHubMerge polls until a PR merge completes or times out.
func PollDoltHubMerge(owner, db, token string, pullID int) error

type PRRequest struct {
    Title              string `json:"title"`
    Description        string `json:"description"`
    FromBranchOwner    string `json:"fromBranchOwnerName"`
    FromBranchRepo     string `json:"fromBranchRepoName"`
    FromBranchName     string `json:"fromBranchName"`
    ToBranchOwner      string `json:"toBranchOwnerName"`
    ToBranchRepo       string `json:"toBranchRepoName"`
    ToBranchName       string `json:"toBranchName"`
}

type PRResponse struct {
    PullID int    `json:"pull_id"`
    Status string `json:"status"`
}
```

### Modified: `internal/cmd/wl_claim.go`, `wl_done.go`, `wl_post.go`

Each command gains a `--pr` flag (auto-enabled when `pr_mode: true` in
config). The execution path branches:

```go
if wlCfg.PRMode {
    return claimWantedViaPR(localDir, wantedID, rigHandle, wlCfg)
} else {
    return claimWantedInLocalClone(localDir, wantedID, rigHandle)
}
```

### Modified: `internal/wasteland/wasteland.go`

Add `PRMode` and `AutoMerge` fields to `WastelandConfig`.

### Modified: `internal/cmd/wl_sync.go`

Add branch cleanup for merged PRs.

## Implementation Sequence

| Step | File | Description | Effort |
|------|------|-------------|--------|
| 1 | `dolthub_pr.go` | DoltHub PR API client | Small |
| 2 | `dolthub_pr_test.go` | API client tests | Small |
| 3 | `wasteland.go` | Add PRMode/AutoMerge to config | Small |
| 4 | `wl_claim.go` | PR-mode claim path | Medium |
| 5 | `wl_post.go` | PR-mode post path | Medium |
| 6 | `wl_done.go` | PR-mode done path | Medium |
| 7 | `wl_sync.go` | Branch cleanup | Small |
| 8 | Integration tests | Full cycle test | Medium |
| 9 | `WASTELAND.md` | Documentation update | Small |

## Migration Path

1. **Phase 2a (opt-in):** Add `--pr` flag. Config `pr_mode` enables default.
   Wild-west still works.
2. **Phase 2b (default):** `pr_mode: true` default for new joins. Existing
   configs unchanged.
3. **Phase 2c (enforced):** Upstream maintainer enables trust gating. Direct
   writes to main rejected. All mutations require PRs.

## Trust Level Enforcement (Future)

Once PR mode is established, trust gating becomes straightforward:

```go
func canAutoMerge(operation string, trustLevel int, config *WastelandConfig) bool {
    if !config.AutoMerge[operation] {
        return false
    }
    if operation == "done" && trustLevel < 2 {
        return false // Requires stamp before merge
    }
    return true
}
```

The `rigs.trust_level` field already exists in the schema. PR mode provides
the enforcement point that was missing in Phase 1.
