# Configurable Merge Queue Strategies

> Design document for adding configurable merge strategies to Gas Town

## Overview

Currently, the Gas Town refinery has a single merge strategy: **auto-merge to main**. This works well for repos where we have direct push access (steveyegge/gastown, steveyegge/beads), but doesn't support other common workflows:

- Repos with protected branches requiring PR review
- Repos with develop/staging branch workflows
- External repos where we're contributors, not maintainers
- Repos with specific merge policies (squash-only, no force-push, etc.)

This document proposes four configurable merge strategies and a configuration schema to support them.

## Current State

### How Merges Work Today

1. Polecat completes work, runs `gt done`
2. `gt done` creates ephemeral MR bead with target branch (usually `main`)
3. Refinery polls MQ, picks up MR bead
4. Refinery rebases branch on target, runs tests
5. Refinery merges with `git merge --ff-only` and pushes directly
6. No PR creation, no review, no approval gate

### Where Strategy is Determined

| Component | File | Current Behavior |
|-----------|------|------------------|
| MR creation | `internal/cmd/done.go:313-318` | Auto-detects integration branch or uses default |
| Config | `internal/config/types.go:674-735` | `MergeQueueConfig` with `target_branch` |
| Refinery merge | `internal/refinery/engineer.go` | Always direct merge, never creates PR |
| Template | `internal/templates/roles/refinery.md.tmpl` | Hardcoded `git merge --ff-only` |

### Existing Configuration

```go
type MergeQueueConfig struct {
    Enabled              bool   // Enable/disable merge queue
    TargetBranch         string // Default target ("main")
    IntegrationBranches  bool   // Enable epic integration branches
    OnConflict           string // "assign_back" or "auto_rebase"
    RunTests             bool   // Run tests before merge
    TestCommand          string // Custom test command
    DeleteMergedBranches bool   // Cleanup after merge
    // ... other fields
}
```

## Proposed Strategies

### 1. DIRECT_MERGE (Current Default)

**Description**: Refinery merges directly to target branch. No PR created.

**Use Cases**:
- Repos where we have direct push access
- Internal repos with no branch protection
- High-trust development environments

**Configuration**:
```json
{
  "merge_queue": {
    "strategy": "direct_merge",
    "target_branch": "main"
  }
}
```

**Refinery Behavior**:
1. Rebase polecat branch on target
2. Run tests
3. `git merge --ff-only`
4. `git push origin <target>`
5. Delete polecat branch

**Exit Condition**: Work lands on target branch.

### 2. PR_TO_MAIN

**Description**: Create a GitHub PR targeting main. Requires review/approval.

**Use Cases**:
- External repos where we're contributors
- Repos with protected main branch
- Repos requiring code review

**Configuration**:
```json
{
  "merge_queue": {
    "strategy": "pr_to_main",
    "target_branch": "main",
    "pr_template": "optional-template-path",
    "auto_merge": false
  }
}
```

**Refinery Behavior**:
1. Rebase polecat branch on target
2. Run tests locally
3. Push rebased branch to origin
4. Create PR via `gh pr create --base main --head polecat/<worker>`
5. **STOP** - Work is now awaiting external review
6. Update MR bead with PR URL

**Exit Condition**: PR created. Actual merge happens externally (GitHub merge button or auto-merge).

**New Requirement**: Track PR state in MR bead for lifecycle management.

### 3. PR_TO_BRANCH

**Description**: Create PR targeting a non-main branch (develop, staging, etc.)

**Use Cases**:
- Repos with develop → main promotion workflow
- Feature branch aggregation (release branches)
- Staging environments

**Configuration**:
```json
{
  "merge_queue": {
    "strategy": "pr_to_branch",
    "target_branch": "develop",
    "pr_template": "optional-template-path"
  }
}
```

**Refinery Behavior**: Same as PR_TO_MAIN but targets `develop` instead.

**Exit Condition**: PR created targeting configured branch.

### 4. DIRECT_TO_BRANCH

**Description**: Direct merge to a non-main branch (like DIRECT_MERGE but not targeting main).

**Use Cases**:
- Staging branch workflows without PR requirement
- Integration branches for large features
- Internal develop branch merging

**Configuration**:
```json
{
  "merge_queue": {
    "strategy": "direct_to_branch",
    "target_branch": "develop"
  }
}
```

**Refinery Behavior**: Same as DIRECT_MERGE but targets `develop`.

**Exit Condition**: Work lands on target branch.

## Strategy Matrix

| Strategy | Creates PR | Target | Needs Review | Our Push Access |
|----------|------------|--------|--------------|-----------------|
| `direct_merge` | No | main | No | Yes (maintainer) |
| `pr_to_main` | Yes | main | Yes | No (contributor) |
| `pr_to_branch` | Yes | configurable | Maybe | No (contributor) |
| `direct_to_branch` | No | configurable | No | Yes (maintainer) |

## Configuration Schema

### Per-Rig Configuration

Location: `<rig>/settings/config.json`

```json
{
  "merge_queue": {
    "enabled": true,
    "strategy": "direct_merge",
    "target_branch": "main",
    "integration_branches": true,
    "on_conflict": "assign_back",
    "run_tests": true,
    "test_command": "go test ./...",
    "delete_merged_branches": true,

    "pr_options": {
      "template": ".github/PULL_REQUEST_TEMPLATE.md",
      "auto_merge": false,
      "labels": ["automated", "from-gastown"],
      "reviewers": []
    }
  }
}
```

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `strategy` | enum | One of: `direct_merge`, `pr_to_main`, `pr_to_branch`, `direct_to_branch` |
| `pr_options.template` | string | Path to PR template file |
| `pr_options.auto_merge` | bool | Enable GitHub auto-merge when creating PR |
| `pr_options.labels` | []string | Labels to apply to created PRs |
| `pr_options.reviewers` | []string | GitHub usernames to request review from |

### Default Strategy Detection (Optional)

Could auto-detect strategy based on:

1. **Remote URL analysis**: `steveyegge/*` → `direct_merge`, others → `pr_to_main`
2. **GitHub API**: Check if we have push access, check branch protection
3. **Existing branches**: If `develop` exists, maybe suggest `pr_to_branch`

However, explicit configuration is preferred for predictability.

## Implementation Plan

### Phase 1: Configuration Schema (Low Risk)

1. Add `Strategy` field to `MergeQueueConfig` in `types.go`
2. Add `PROptions` struct for PR-specific settings
3. Add validation for valid strategy values
4. Default to `direct_merge` for backward compatibility

**Files**: `internal/config/types.go`, `internal/config/loader.go`

### Phase 2: Refinery Strategy Dispatch (Medium Risk)

1. Add strategy dispatch in `engineer.go`
2. Implement `ProcessDirectMerge()` (existing behavior)
3. Implement `ProcessPRMerge()` (new: creates PR via gh CLI)
4. Update `ProcessMR()` to dispatch based on strategy

**Files**: `internal/refinery/engineer.go`

### Phase 3: PR Lifecycle Tracking (Medium Risk)

1. Extend MR bead schema for PR URL, PR number, PR state
2. Add methods to update MR bead when PR state changes
3. Consider webhook integration for PR merge notifications

**Files**: `internal/beads/`, `internal/refinery/engineer.go`

### Phase 4: Template Updates (Low Risk)

1. Update refinery template with strategy-aware guidance
2. Add PR creation workflow to patrol cycle
3. Document new behavior for agents

**Files**: `internal/templates/roles/refinery.md.tmpl`

### Phase 5: Polecat/Crew Awareness (Low Risk)

1. Update polecat template with strategy context
2. Polecats don't need to know strategy (they just push, refinery decides)
3. Document for crew members working with PR workflows

**Files**: `internal/templates/roles/polecat.md.tmpl`, docs

## Research Questions Answered

### 1. Configuration location?

**Recommendation**: Per-rig config in `<rig>/settings/config.json`

- Already exists and is loaded by refinery
- Natural place for rig-specific settings
- Town-level default can be in mayor config, with rig overrides

### 2. Auto-detection possibilities?

**Recommendation**: Explicit configuration preferred

Auto-detection is fragile:
- Remote URLs change
- GitHub API requires auth and is slow
- Branch existence isn't reliable indicator

But we could add a `gt mq detect` command that suggests config based on repo analysis.

### 3. Refinery changes needed?

See Implementation Plan above. Key changes:
- Strategy dispatch in `engineer.go`
- PR creation via `gh pr create`
- MR bead tracking for PR state

### 4. Polecat/crew awareness?

**Polecats don't need to know.** They push branches and run `gt done`. The refinery decides how to land based on strategy.

**Crew can know** via config inspection, but it doesn't change their workflow.

### 5. Edge cases?

| Edge Case | Handling |
|-----------|----------|
| Target branch doesn't exist | Error at config validation, fail MR with clear message |
| PR creation fails (permissions) | Log error, block MR, file bug bead |
| PR rejected/closed | Update MR bead state, optionally notify |
| Strategy change mid-queue | Process remaining MRs with old strategy, new MRs use new |
| Integration branches + PR strategy | Integration branches override target, PR still created |

## Migration Path

1. **Deploy config schema changes** (backward compatible)
2. **Default is `direct_merge`** - existing behavior unchanged
3. **Rigs opt-in to new strategies** via config
4. **No flag day** - gradual rollout per rig

## Testing Strategy

1. **Unit tests**: Strategy dispatch, config parsing, validation
2. **Integration tests**: Mock `gh` CLI for PR creation
3. **E2E tests**: Test each strategy with real GitHub repo (test org)
4. **Rollback plan**: Remove strategy field → falls back to `direct_merge`

## Open Questions

1. **PR merge notification**: How do we know when an external PR is merged?
   - Option A: Poll GitHub API
   - Option B: Webhook (requires infrastructure)
   - Option C: Manual `gt mq merged <mr-id>` command

2. **PR update workflow**: If PR needs changes after review, what happens?
   - Option A: Polecat rework task, force-push to same branch
   - Option B: Close PR, create new one after rework

3. **Multiple strategies per rig**: Should we support different strategies for different issue types?
   - Probably not in v1, keep it simple

## Deliverables Checklist

- [x] Research doc with strategy matrix (this document)
- [ ] Configuration schema proposal (included above)
- [ ] Refinery modification plan (included above)
- [ ] Updated templates for strategy-aware guidance
- [ ] Implementation tasks as child issues

## Next Steps

1. Create child issues for each implementation phase
2. Start with Phase 1 (config schema) as lowest risk
3. Test PR workflow manually before automating
4. Document for crew and polecats
