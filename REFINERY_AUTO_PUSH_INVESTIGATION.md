# Investigation: Refinery Auto-Push Behavior

## Issue: sg-0qrt
**Title:** Investigate: Refinery auto-push — why don't merges push to origin?

## Current Behavior Analysis

### Code Path Examination

The refinery merge code **DOES** call `git push` after successful merge. The flow is:

**File:** `/home/ubuntu/gt/sfgastown/mayor/rig/internal/refinery/engineer.go`

**Function:** `doMerge(ctx context.Context, branch, target, sourceIssue string) ProcessResult`

**Execution Flow:**
1. **Step 1-2:** Verify source branch exists, checkout target branch
2. **Step 3:** Check for merge conflicts
3. **Step 3.5:** Push submodule commits (if any) - lines 425-453
4. **Step 4:** Run quality gates/tests - lines 455-474
5. **Step 5:** Perform squash merge - line 489
6. **Step 6:** Get merge commit SHA - line 508
7. **Step 7:** Acquire merge slot for serialization (main branch only) - lines 516-547
8. **Step 8: Push to origin** - **line 551**
   ```go
   if err := e.git.Push("origin", target, false); err != nil {
       // Reset the checked-out target branch to undo the local squash commit.
       if resetErr := e.git.ResetHard("origin/" + target); resetErr != nil {
           _, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to reset %s after push failure: %v\n", target, resetErr)
       }
       return ProcessResult{
           Success: false,
           Error:   fmt.Sprintf("failed to push to origin: %v", err),
       }
   }
   ```

### Key Findings

1. **Push IS called:** Line 551 explicitly calls `e.git.Push("origin", target, false)`
2. **Error handling is correct:** Push failures are caught, logged, and returned as errors
3. **No config option:** There is NO configuration option to enable/disable auto-push
4. **Branch reset on failure:** If push fails, the local branch is reset to origin state (line 554)

## Why Merges Might Not Appear to Push

### Potential Causes

1. **Refinery Not Running**
   - The refinery agent must be running to process the merge queue
   - Check: `gt refinery status`
   - Start: `gt refinery start`

2. **Push Authentication Failure**
   - Git push may fail due to SSH key or credential issues
   - The error would be logged but might not be visible to users
   - Check refinery logs for "failed to push to origin" errors

3. **Pre-push Hook Blocking**
   - A git pre-push hook might be rejecting the push
   - Check: `.git/hooks/pre-push` in the refinery worktree
   - The error would be returned but might not be surfaced clearly

4. **Merge Slot Contention**
   - Step 7 acquires a merge slot before pushing (line 522)
   - If slot acquisition times out, push never happens (line 535)
   - This is intentional serialization to prevent concurrent pushes

5. **MR Not Being Processed**
   - The MR might be blocked by a conflict resolution task
   - Check: `gt refinery blocked`
   - Or the MR might be claimed but not progressing
   - Check: `gt refinery ready --all --json`

6. **Submodule Push Failure**
   - If the branch has submodule changes, Step 3.5 pushes those first (line 445)
   - Submodule push failure prevents the main push from happening

## Recommendations

### Option 1: Add Push Status Logging (RECOMMENDED)

Add explicit logging when push succeeds to make it visible in refinery output:

```go
// After line 561 (successful push)
_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Successfully pushed to origin/%s\n", target)
```

**Pros:** Makes push success/failure visible
**Cons:** Minor code change, requires deployment

### Option 2: Add Configuration for Auto-Push

Add a `merge_queue.auto_push` config option to allow rigs to disable auto-push:

```json
{
  "merge_queue": {
    "auto_push": false
  }
}
```

**Pros:** Gives operators control
**Cons:** Adds complexity, security implications (dangling merge commits)

### Option 3: Add 'Pending Push' Indicator

Add a status indicator to `gt mq list` showing MRs that are merged locally but not yet pushed:

**Pros:** Visibility into push state
**Cons:** Requires state tracking, adds complexity

### Option 4: Add `gt mq push-all` Command

Add a manual command to push all merged branches:

**Pros:** Manual control
**Cons:** Defeats the purpose of automation, adds operational burden

## Conclusion

**The refinery DOES auto-push after merge.** The code is correct and present at line 551 of `engineer.go`.

If merges are not being pushed, the likely causes are:
1. Refinery agent not running
2. Git authentication/SSH issues
3. Pre-push hooks blocking
4. Merge slot contention (temporary)
5. Submodule push failures

**Recommended Fix:** Add explicit success logging (Option 1) to make push operations visible in refinery output. This will help diagnose future issues.

## Files Examined

- `/home/ubuntu/gt/sfgastown/mayor/rig/internal/refinery/engineer.go` (lines 374-568)
- `/home/ubuntu/gt/sfgastown/mayor/rig/internal/refinery/manager.go`
- `/home/ubuntu/gt/sfgastown/mayor/rig/internal/git/git.go` (lines 445-464)
- `/home/ubuntu/gt/sfgastown/mayor/rig/internal/cmd/refinery.go`

## Next Steps

1. Add push success logging to `engineer.go`
2. Consider adding push failure metrics/monitoring
3. Document troubleshooting steps for push failures in runbook
