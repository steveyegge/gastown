# Polecat Cleanup Flow

> Complete termination sequence from `gt done` to worktree nuke

This document traces the cleanup flow **backwards** from final worktree deletion
to work completion, revealing the full signal chain.

---

## Overview: Cleanup Signal Chain

```
gt done (polecat)
    │
    ├─► Set done-intent label (early, for crash detection)
    ├─► Create MR bead (type=merge-request)
    ├─► Push branch to remote
    ├─► Notify witness (MERGE_READY)
    └─► Self-kill session
            │
            ▼
Witness receives MERGE_READY
    │
    └─► Nudge refinery
            │
            ▼
Refinery picks up MR
    │
    ├─► Rebase on target branch
    ├─► Run quality gates (test, lint, build)
    ├─► Git merge & push
    ├─► Verify push (compare SHA)
    ├─► Send MERGED signal ◄── CRITICAL: before any cleanup
    └─► Close MR bead
            │
            ▼
Witness receives MERGED
    │
    ├─► Verify commit on main (multi-remote aware)
    ├─► Check cleanup_status
    └─► NukePolecat() ◄── FINAL STEP
```

---

## 1. gt done: Polecat Self-Termination

**File:** `internal/cmd/done.go:82-975`

### Complete Step-by-Step Flow

| Lines | Step | Details |
|-------|------|---------|
| 88-91 | Role verification | Only polecats call `gt done` |
| 93-97 | Exit status validation | COMPLETED, ESCALATED, or DEFERRED |
| 113-125 | Deferred cleanup | Ensures `selfKillSession()` runs on ANY exit |
| 127-131 | SIGTERM handler | Handle context exhaustion gracefully |
| 305-342 | Parse issue ID | From branch name, `--issue` flag, or hook_bead |
| **344-360** | **Set done-intent (EARLY)** | Allows zombie detection if crash occurs |
| 414-463 | Handle zero-commit | Skip MR if no commits, just close |
| 465-503 | Check merge strategy | `mr` (default), `direct`, or `local` |
| 700-749 | Push branch | `git push origin <branch>` |
| **752-816** | **Create MR bead** | Type: `merge-request`, ephemeral |
| 852-938 | Update agent state | working → idle |
| **940-950** | **Notify witness** | MERGE_READY or COMPLETED signal |
| 952-965 | Self-kill session | Kill tmux before exit |

### Done-Intent Label (Crash Detection)

**File:** `internal/cmd/done.go:977-995`

```go
func setDoneIntentLabel(bd, agentBeadID, exitType string) error {
    label := fmt.Sprintf("done-intent:%s:%d", exitType, time.Now().Unix())
    // Set on agent bead for Witness to detect
}
```

**Purpose:** If polecat crashes after setting done-intent but before completing,
Witness can detect: `dead session + done-intent = auto-nuke candidate`.

**Format:** `done-intent:<exitType>:<unix-timestamp>`

### MR Bead Creation

**File:** `internal/cmd/done.go:752-816`

```go
mrFields := &beads.MergeRequestFields{
    Branch:       branch,
    Target:       targetBranch,
    SourceIssue:  issueID,
    Rig:          rigName,
    Worker:       polecatName,
    AgentBead:    agentBeadID,
    // Conflict tracking
    RetryCount:      0,
    LastConflictSHA: "",
    ConflictTaskID:  "",
}

mrBead, err := beads.CreateMergeRequest(title, mrFields)
```

**Verification:** Lines 789-800 fetch and verify MR persistence before continuing.

### Witness Notification

**File:** `internal/cmd/done.go:940-950`

```go
func notifyWitness(exitType, polecatName, issueID, mrBeadID string) {
    if exitType == "COMPLETED" && mrBeadID != "" {
        // Send MERGE_READY to witness
        sendMergeReady(polecatName, branch, issueID, mrBeadID)
    } else if exitType == "COMPLETED" {
        // Send COMPLETED (no MR needed)
        sendCompleted(polecatName, issueID)
    } else {
        // Send ESCALATED or DEFERRED
        sendExitSignal(exitType, polecatName, issueID)
    }
}
```

---

## 2. Refinery Processing

**File:** `internal/refinery/manager.go` + `mol-refinery-patrol.formula.toml`

### MR Pickup

**File:** `internal/refinery/manager.go:240-285`

```go
func (m *Manager) Queue() ([]MergeRequest, error) {
    // Query beads with label gt:merge-request and status open
    mrs, err := m.beads.ListMergeRequests()
    // Sort by priority (higher score first)
    sort.Slice(mrs, func(i, j int) bool {
        return mrs[i].Score > mrs[j].Score
    })
    return mrs, nil
}
```

### Processing Steps (from mol-refinery-patrol.formula.toml)

| Step | Lines | Action |
|------|-------|--------|
| inbox-check | 128-190 | Clean wisps, check MERGE_READY mail |
| queue-scan | 192-218 | `gt mq list`, verify branches exist |
| process-branch | 220-302 | Rebase on target, handle conflicts |
| run-tests | 304-347 | Quality gates: setup, typecheck, lint, build, test |
| handle-failures | 350-405 | On fail: reopen issue, send MERGE_FAILED |
| **merge-push** | 408-512 | **Critical merge sequence** |

### Merge-Push Sequence (Critical)

**Lines 408-512 in mol-refinery-patrol.formula.toml:**

```
1. Git merge & push (lines 418-423)
2. VERIFY PUSH - compare local/remote SHA (lines 426-445) [PATCH-003]
3. ████ SEND MERGED MAIL ████ (lines 448-456) ◄── BEFORE cleanup
4. Close MR bead with merged reason (lines 461-475)
5. Archive MERGE_READY mail (lines 485-487)
```

**Critical:** MERGED signal sent **immediately after push verification**, before
any cleanup. This ensures Witness receives signal even if Refinery crashes.

### MERGED Signal Format

**File:** `internal/protocol/refinery_handlers.go:50-75`

```go
func NewMergedMessage(polecat, branch, issue, targetBranch, mergeCommit string) *mail.Message {
    return &mail.Message{
        Subject: fmt.Sprintf("MERGED %s", polecat),
        Body:    fmt.Sprintf("branch: %s\nissue: %s\ntarget: %s\ncommit: %s",
                             branch, issue, targetBranch, mergeCommit),
    }
}
```

---

## 3. Witness MERGED Handling

**File:** `internal/witness/handlers.go:257-298`

### HandleMerged Function

```go
func HandleMerged(workDir, rigName string, msg *mail.Message) (*MergedResult, error) {
    // Line 265: Parse message
    payload, err := ParseMerged(msg.Subject, msg.Body)

    // Line 284: Verify commit is on main
    if !verifyCommitOnMain(workDir, rigName, payload.PolecatName) {
        return nil, fmt.Errorf("MERGED signal may be stale, DO NOT NUKE")
    }

    // Line 295: Get cleanup status
    status := getCleanupStatus(workDir, rigName, payload.PolecatName)

    // Line 296: Handle based on status
    return handleMergedCleanupStatus(status, payload)
}
```

### Commit Verification (Multi-Remote Aware)

**File:** `internal/witness/handlers.go:835-891`

```go
func verifyCommitOnMain(workDir, rigName, polecatName string) bool {
    // Get merge commit from polecat's agent bead
    mergeCommit := getMergeCommitFromAgentBead(polecatName)

    // Check all remotes to find default branch containing commit
    for _, remote := range listRemotes() {
        defaultBranch := getDefaultBranch(remote)
        if branchContainsCommit(remote, defaultBranch, mergeCommit) {
            return true
        }
    }
    return false
}
```

**Why Multi-Remote?** Code may be pushed to a remote other than "origin" (e.g., fork workflow).

---

## 4. Cleanup Status Handling

**File:** `internal/witness/handlers.go:313-323`

### Status Values and Actions

| Status | Action | Risk |
|--------|--------|------|
| `clean` or `""` | Proceed to nuke | Safe |
| `has_uncommitted` | **BLOCK**, escalate to Mayor | Work may be lost |
| `has_stash` | **BLOCK**, escalate to Mayor | State unclear |
| `has_unpushed` | **BLOCK**, "DO NOT NUKE" | Must push first |
| `unknown` | Proceed if commit on main | Assumed clean |

### getCleanupStatus Implementation

**File:** `internal/witness/handlers.go:517-560`

```go
func getCleanupStatus(workDir, rigName, polecatName string) string {
    // Read agent bead description
    agentBead := getAgentBead(polecatName)

    // Parse cleanup_status from description
    // Format: cleanup_status:<value>
    for _, line := range strings.Split(agentBead.Description, "\n") {
        if strings.HasPrefix(strings.ToLower(line), "cleanup_status:") {
            return strings.TrimPrefix(line, "cleanup_status:")
        }
    }
    return ""  // Empty = assumed clean (RISK: parsing failure)
}
```

**Risk:** If description format changes, parsing silently fails and returns empty,
which is assumed "clean". May nuke polecat with uncommitted work.

---

## 5. NukePolecat: Final Step

**File:** `internal/witness/handlers.go:773-803`

### Implementation

```go
func NukePolecat(workDir, rigName, polecatName string) error {
    // Line 778: Initialize registry
    registry := initRegistry(workDir)

    // Lines 779-792: Kill tmux session
    sessionName := session.PolecatSessionName(rigName, polecatName)

    // Graceful first (Ctrl-C)
    if err := tmux.SendKeys(sessionName, "C-c"); err == nil {
        time.Sleep(500 * time.Millisecond)
    }

    // Force kill
    tmux.KillSession(sessionName)

    // Lines 795-800: Nuke worktree, branch, beads
    cmd := exec.Command("gt", "polecat", "nuke",
                        fmt.Sprintf("%s/%s", rigName, polecatName))
    return cmd.Run()
}
```

### What `gt polecat nuke` Does

1. Remove git worktree
2. Delete remote branch (if exists)
3. Remove agent bead
4. Clean up any local state

### Failure Handling

**Line 799:** Returns error but does NOT escalate - just logged.

**Result:** Polecat stays in zombie state for next patrol cycle to retry.

**Gap:** No retry limit visible. Failed nukes could accumulate.

---

## 6. Signal Timing Diagram

```
TIME ──────────────────────────────────────────────────────────►

POLECAT                     REFINERY                    WITNESS
   │                           │                           │
   ├─ set done-intent          │                           │
   │                           │                           │
   ├─ create MR bead           │                           │
   │                           │                           │
   ├─ git push                 │                           │
   │                           │                           │
   ├─ MERGE_READY ─────────────┼───────────────────────────►
   │                           │                           │
   ├─ self-kill ─────┐         │                           │
   │                 │         │                           │
   X (session dead)  │         ├─ pick up MR               │
                     │         │                           │
                     │         ├─ rebase                   │
                     │         │                           │
                     │         ├─ run tests                │
                     │         │                           │
                     │         ├─ git merge && push        │
                     │         │                           │
                     │         ├─ verify push SHA          │
                     │         │                           │
                     │         ├─ MERGED ──────────────────►
                     │         │                           │
                     │         ├─ close MR bead            ├─ verify commit on main
                     │         │                           │
                     │         │                           ├─ check cleanup_status
                     │         │                           │
                     │         │                           ├─ NukePolecat()
                     │         │                           │
                     │         │                           X (worktree deleted)
```

---

## 7. Safeguards Summary

| Safeguard | Location | Purpose |
|-----------|----------|---------|
| Done-intent label | done.go:344-360 | Crash detection |
| MR verification | done.go:789-800 | Ensure persistence |
| Push verification | formula:426-445 | Compare SHA (PATCH-003) |
| MERGED before cleanup | formula:448-456 | Signal survives crash |
| Commit-on-main check | handlers.go:835-891 | Prevent stale signal nuke |
| Cleanup status check | handlers.go:295-296 | Prevent dirty nuke |
| Graceful kill first | handlers.go:779-792 | Allow cleanup handlers |

---

## 8. Failure Scenarios

### Polecat Crashes After done-intent

**Detection:** Witness patrol finds `done-intent` label + dead session.
**Recovery:** Auto-nuke if session confirmed dead > 60 seconds.

### Refinery Crashes After Merge

**Detection:** MERGED already sent (sent immediately after push).
**Recovery:** Witness receives MERGED, proceeds normally.

### Witness Crashes After MERGED

**Detection:** MERGED mail remains in inbox.
**Recovery:** Next patrol cycle re-processes.

### Nuke Fails

**Detection:** Error logged, polecat stays.
**Recovery:** Next patrol cycle retries nuke.
**Gap:** No retry limit - could accumulate failed nukes.
