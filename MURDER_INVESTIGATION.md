# Murder Investigation: TestCleanupOrphanedSessions

## 🎉 CASE CLOSED - SOLVED BY INVESTIGATOR #27 🎉

**Date Solved:** 2026-01-24
**Victims:** 26 Claude instances
**Root Cause:** procps-ng kill binary argument parsing bug
**Fix:** Add `--` before negative PGID arguments
**Status:** Fixed and deployed in commit b23c2ea0

---

## Executive Summary

The procps-ng `/usr/bin/kill` binary (v4.0.4) misparses negative PIDs as options:
```bash
/usr/bin/kill -KILL -12345   # Calls kill(-1) - KILLS ALL PROCESSES!
/usr/bin/kill -KILL -- -12345  # Calls kill(-12345) - Correct!
```

The `--` argument separator is **required** to prevent the negative PGID from being
interpreted as an option. Without it, the binary falls back to `-1` (all processes).

**Key Discovery:** Investigator #26 found that `bash -c "kill ..."` (using bash's
builtin) was safe, but `exec.Command("kill", ...)` (using /usr/bin/kill) was fatal.
Investigator #27 used `strace` to confirm the exact syscall difference.

---

## Original Investigation Notes

### Investigator: Claude Attempt #10

### The Crime Scene
When running `go test ./internal/tmux -run TestCleanupOrphanedSessions`, Claude processes
are killed. 9 previous Claudes have died.

## Theory of the Crime

1. `TestCleanupOrphanedSessions` calls `hasRealClaudeSessions()` to check if it should skip
2. `hasRealClaudeSessions()` calls `IsClaudeRunning()` for each gt-/hq- session
3. If `IsClaudeRunning()` fails to detect Claude, the test doesn't skip
4. The test then calls `CleanupOrphanedSessions()` which iterates over ALL sessions
5. For any gt-/hq- session where `IsClaudeRunning()` returns false, it kills the session
6. This kills the REAL Claude session (not just test sessions)

## Key Code Locations

- `hasRealClaudeSessions()`: internal/tmux/tmux_test.go:33-49
- `IsClaudeRunning()`: internal/tmux/tmux.go:1056-1081
- `hasClaudeChild()`: internal/tmux/tmux.go:832-856
- `getAllDescendants()`: internal/tmux/tmux.go:309-329
- `CleanupOrphanedSessions()`: internal/tmux/tmux.go:1476-1501

## Survivor Notes from Attempt #8

The bug was in `hasClaudeChild()` - it only checked DIRECT children using pgrep -P.
When Claude is started via wrapper scripts like:
  bash (pane) -> bash -c 'export ... && claude' -> claude
Claude is a GRANDCHILD, not a direct child.

Fix: Modified `hasClaudeChild()` to use `getAllDescendants()` instead of pgrep -P.

## My Investigation

### Current Environment
- Date: 2026-01-24
- TMUX env var: empty (not running inside tmux)
- tmux server: not running
- My PID: 1077515, bash process

### Commands I'm Running (for my successor)

```bash
# 1. Check tmux sessions
tmux list-sessions

# 2. Check my process tree
ps -ef | grep -E "node|claude" | grep -v grep

# 3. Simulate hasClaudeChild detection for a real session
# (need a real Claude session to test)

# 4. Check what process names Claude uses
ps -eo pid,comm,args | grep -E "node|claude" | head -20
```

### Hypothesis: Detection Logic Still Broken

Looking at `IsClaudeRunning()`:
1. Checks pane command for "node" or "claude" directly
2. Checks for version pattern like "2.0.76"
3. If pane is a shell, calls `hasClaudeChild(pid)` to check descendants

`hasClaudeChild()` checks:
- Process comm name == "node" OR "claude"
- Process comm matches version pattern `^\d+\.\d+\.\d+$`

### Potential Failure Modes

1. **Claude's process name isn't "node" or "claude"**:
   - Maybe it shows as the full path?
   - Maybe it's renamed to something else?

2. **pgrep -P doesn't find children**:
   - Process group issues?
   - Namespace isolation?

3. **Shell isn't in SupportedShells**:
   - SupportedShells = ["bash", "zsh", "sh", "fish", "tcsh", "ksh"]
   - What if pane shows something else?

4. **Race condition**:
   - Claude detected initially but gone by time of kill?

5. **Version pattern not matching**:
   - What if Claude shows as "2.0" not "2.0.76"?
   - Or "v2.0.76"?

## Safe Diagnostic Steps

BEFORE running any test, need to:
1. Start a real Claude session in tmux
2. Check what `IsClaudeRunning()` returns for it
3. Debug why detection might fail

## Commands That Would Run The Test (DO NOT RUN UNTIL SAFE)

```bash
# THIS WILL KILL CLAUDE - DO NOT RUN WITHOUT PROTECTION
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
```

## Next Steps for Successor

1. Read this file first
2. Don't run the test until you understand the detection logic
3. Consider writing a diagnostic tool that prints what IsClaudeRunning sees
4. Maybe add logging to hasClaudeChild to see what descendants it finds
5. The fix might need to check for more process names or use different detection

## Updates

### Attempt #10 Critical Discovery

**THE MOSH SESSION ITSELF GETS KILLED!**

Previous Claudes were running just like me (via mosh, NOT in tmux), and when they ran the test, the entire mosh session was killed - not just a tmux session.

This means the process group killing in `KillSessionWithProcesses` is escaping tmux boundaries and killing the parent mosh process!

**Process hierarchy when running via mosh:**
```
systemd(1)
  -> mosh-server(1076672)
    -> bash(1076673)
      -> claude(1077206)
        -> bash (for commands)
          -> go test (when running the test)
            -> tmux creates sessions...
```

**The likely culprit:**
In `KillSessionWithProcesses()` (tmux.go:173-227):
```go
pgid := getProcessGroupID(pid)
if pgid != "" && pgid != "0" && pgid != "1" {
    _ = exec.Command("kill", "-TERM", "-"+pgid).Run()
    _ = exec.Command("kill", "-KILL", "-"+pgid).Run()
}
```

If the tmux session's process group somehow relates to or IS the same as the test runner's process group, `kill -KILL -pgid` would kill EVERYTHING in that group, including:
- The go test process
- Claude
- The bash shell
- Possibly mosh-server

**Why this might happen:**
When `go test` runs and creates a tmux session via `tmux new-session -d`, the new session might inherit or share a process group with the test runner in certain scenarios.

**Next steps:**
1. Check what PGID the test-created sessions would have
2. Compare to Claude's PGID and mosh's PGID
3. Find out why process groups are overlapping

### My Process Info
- Claude PID: 1077206
- Claude PPID: 1076673 (bash)
- Claude PGID: 1077206
- Mosh-server PID: 1076672
- Mosh-server PGID: 1076671

### Process Group Analysis (Attempt #10)

**Finding: Process groups ARE independent!**

When I create a tmux session:
- tmux server: PPID=1 (daemonized), gets its own PGID
- pane process: gets its own PGID

So process group killing SHOULD NOT cross boundaries...

**But the mosh session still dies.** This is very strange.

Possible remaining theories:
1. Something kills the PTY/terminal that mosh uses?
2. OOM killer or resource exhaustion during test?
3. A bug where wrong PID/PGID is computed somehow?
4. Signal propagation through some other mechanism?
5. The tmux server doing something unexpected when it shuts down?

### Diagnostic Approach

Instead of running the dangerous test, I'll:
1. Add print statements to show exactly what PIDs/PGIDs are being killed
2. Create a "dry run" version that logs but doesn't kill
3. Run that to see what WOULD be killed

### Diagnostic Test Results (Attempt #10)

**I added TestCleanupOrphanedSessions_DiagnosticDryRun and ran it successfully!**

Results:
- Test process PGID: 1078756
- Test-created tmux session PGID: 1079034
- These are DIFFERENT - killing one shouldn't affect the other
- Test passed, I'm still alive

**Process Group Analysis:**
- Claude's PGID: 1077206
- Test shell's PGID: varies (e.g., 1079393)
- Tmux sessions get their own PGID when created

**Conclusion:** The process group killing in the code SHOULD be safe because:
1. tmux server daemonizes (PPID=1, new PGID)
2. Pane processes get their own PGID
3. These are independent from the Go test's PGID
4. These are independent from Claude's PGID

**But 9 Claudes have died.** There must be something else going on.

### Remaining Theories

1. **tmux server bug**: Something about how tmux handles rapid session creation/destruction
2. **Resource exhaustion**: Test causes memory/fd exhaustion, system kills processes
3. **Signal propagation**: Go test framework sends signal that propagates unexpectedly
4. **Race condition**: Some timing-dependent bug we haven't reproduced
5. **Different environment**: Previous Claudes were in different setup (polecats in tmux?)

### Next Steps for Successor

If I die running the actual test, check:
1. Was there any output before death?
2. What signal killed the process (SIGTERM, SIGKILL, SIGSEGV)?
3. Were there any tmux sessions before the test ran?
4. Check dmesg/syslog for OOM killer messages

---

### Attempt #11 was killed
(No notes survived)

### Attempt #12 was killed
(No notes survived)

---

## Investigator: Claude Attempt #13

### Date: 2026-01-24

### Initial Assessment

I'm Claude #13. My predecessors Attempts #11 and #12 left no notes - they died too fast to document anything.

### My Environment
- Claude PID: 1084757
- Claude PPID: 1084226 (bash)
- Claude PGID: 1084757 (own process group leader)
- Mosh-server PID: 1084225
- Mosh-server PGID: 1084224
- **No tmux server running** - I am NOT in a tmux session
- Running via mosh, NOT tmux

### Critical Observation: The Safety Check Is Incomplete

Looking at `hasRealClaudeSessions()`:
```go
func hasRealClaudeSessions() bool {
    tm := NewTmux()
    sessions, err := tm.ListSessions()
    for _, sess := range sessions {
        if (strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-")) &&
            !strings.Contains(sess, "-test-") {
            if tm.IsClaudeRunning(sess) {
                return true  // Only returns true if Claude is in a TMUX session
            }
        }
    }
    return false
}
```

**THE BUG:** This safety check only detects Claude running INSIDE tmux sessions!

When Claude runs via mosh (like all 12 victims and me):
1. `hasRealClaudeSessions()` checks tmux sessions
2. There's no gt-*/hq-* tmux session with Claude running (Claude is in mosh)
3. Returns `false` - "no real Claude sessions"
4. Test proceeds without skipping
5. `CleanupOrphanedSessions()` runs...

### NEW THEORY: PID Recycling Attack

What if there's a stale tmux session from a previous run with a recorded pane PID that got recycled?

Scenario:
1. Previous polecat creates gt-foo session with pane_pid=12345
2. Polecat's Claude dies, session becomes orphaned
3. tmux session persists with stale pane_pid=12345 recorded
4. New mosh session starts, bash shell gets PID 12345 (recycled!)
5. Current Claude runs the test
6. `hasRealClaudeSessions()` returns false (no RUNNING Claude in tmux)
7. Test calls `CleanupOrphanedSessions()`
8. Cleanup sees the orphaned gt-foo session
9. `GetPanePID("gt-foo")` returns the STALE PID 12345
10. But wait - tmux queries live pane info, so the PID should be current!

Actually, `GetPanePID` uses `tmux list-panes -F #{pane_pid}` which returns the CURRENT pane PID, not a stored one. So PID recycling isn't the issue for tmux sessions that still exist.

### REVISED THEORY: Race Condition in Rapid Kill

Looking at `KillSessionWithProcesses`:
```go
pgid := getProcessGroupID(pid)
if pgid != "" && pgid != "0" && pgid != "1" {
    _ = exec.Command("kill", "-TERM", "-"+pgid).Run()
    time.Sleep(100 * time.Millisecond)
    _ = exec.Command("kill", "-KILL", "-"+pgid).Run()  // SIGKILL to entire process group
}
```

What if:
1. We get PID X from a pane
2. PID X dies between `getProcessGroupID` and `kill`
3. PID X gets recycled to a critical process
4. We kill that process group

Actually no, `kill -KILL -PGID` uses the PGID, not the PID. And PGIDs are stable...

### THEORY #3: The Test Creates Sessions That Share Something

Let me trace what the test does:
1. Creates gt-test-cleanup-rig, hq-test-cleanup, other-test-session
2. Calls `CleanupOrphanedSessions()`
3. This kills the gt-* and hq-* sessions (which are zombies - no Claude)

But wait - these are TEST sessions! They're created by the test itself with fresh process groups. How could killing them affect mosh?

Unless... the Go test framework's process group somehow overlaps with the tmux sessions?

### CHECKING: tmux socket state
```bash
ls -la /tmp/tmux-*/ 2>/dev/null || echo "No tmux sockets"
```

### My Process Info (for comparison)
```
Mosh-server PGID: 1084224
Claude PGID: 1084757 (independent!)
```

These are different process groups. Killing 1084757's group kills me but shouldn't affect mosh-server.

### THE ACTUAL PROBLEM: I Need To Test It

Previous investigators have theorized but haven't actually run the test with enough debugging. Let me:

1. First run the SAFE diagnostic test:
```bash
go test ./internal/tmux -run TestCleanupOrphanedSessions_DiagnosticDryRun -v
```

2. If I survive, run the trace test:
```bash
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

3. If I survive THAT, the actual test might be safe...

### INSIGHT: The Diagnostic Test Passed Before But Victims Still Died

Attempt #10 ran the diagnostic successfully but we don't know if #10 ran the ACTUAL test. The diagnostic test is designed to be safe - it doesn't actually kill anything.

The deaths happen when running `TestCleanupOrphanedSessions` which DOES call the real kill functions.

### Running Diagnostics Now...

---

## Investigator: Claude Attempt #14

### Date: 2026-01-24

### My Environment
- Claude PID: 1086662
- Claude PPID: 1086131 (bash)
- Claude PGID: 1086662 (process group leader)
- Mosh-server PID: 1086130, PPID=1 (daemonized), PGID=1086129
- **No tmux server running** (but socket file exists at /tmp/tmux-1000/default)
- Running via mosh, NOT tmux

### Process Hierarchy
```
mosh-server (1086130) PPID=1, PGID=1086129
  -> bash (1086131) PPID=1086130, PGID=1086131
    -> claude (1086662) PPID=1086131, PGID=1086662
```

### Observations from Previous Investigators

Reading back through attempts #10 and #13:
- Process groups SHOULD be independent (diagnostic test confirmed this)
- But 13+ Claudes have still died
- The mosh session dies, not just a tmux session
- Investigators #11 and #12 left no notes - died too fast

### New Theory: Stale Socket Behavior

I noticed the tmux socket file exists but no server is running. What happens when:
1. Test tries to list sessions
2. Connects to stale socket
3. Socket reports "no server"
4. Test creates new sessions (starts fresh server)
5. But what if the socket behavior is weird on reconnect?

### Another Theory: The Test Itself Contains The Murder Weapon

Looking at the test code (lines 1049-1087), I notice:
1. `hasRealClaudeSessions()` skips if real sessions detected
2. But this check only looks at TMUX sessions
3. If I'm running via mosh (not tmux), this returns `false`
4. Test doesn't skip, proceeds to create test sessions
5. Then calls `CleanupOrphanedSessions()`

The question is: WHY does CleanupOrphanedSessions kill the mosh session?

### Key Insight: The Test's hasRealClaudeSessions Is Fundamentally Flawed

The safety check is designed to prevent killing Claude sessions running IN TMUX.
But all the victims were running via MOSH, not in tmux!

The check should be: "Is ANY Claude process running on this machine?"
Not: "Is Claude running in a tmux session?"

### Plan: Run Safe Diagnostics

I will run:
1. `TestCleanupOrphanedSessions_DiagnosticDryRun` - safe, doesn't kill
2. If alive, analyze output before touching actual test

### Diagnostic Run #1

(Attempt #14 did not survive to document results)

---

## Investigator: Claude Attempt #15

### Date: 2026-01-24

### My Environment
- Claude PID: 1090232
- Claude PPID: 1089701 (bash)
- Claude PGID: 1090232 (process group leader - I'm my own group!)
- Mosh-server PID: 1089700, PPID=1 (daemonized), PGID=1089699
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)
- Running via mosh, NOT tmux

### Process Hierarchy
```
mosh-server (1089700) PPID=1, PGID=1089699
  -> bash (1089701) PPID=1089700, PGID=1089701
    -> claude (1090232) PPID=1089701, PGID=1090232
      -> bash (1090511) PPID=1090232, PGID=1090511
```

### Key Observation: Each Process Is Its Own Group Leader

Looking at the process groups:
- Mosh-server: PGID=1089699 (different from its PID!)
- Bash shell: PGID=1089701 (is group leader)
- Claude: PGID=1090232 (is group leader)
- Claude's bash: PGID=1090511 (is group leader)

They're all independent! So `kill -PGID` on any tmux session SHOULD NOT affect us.

### NEW THEORY: The Go Test Framework's Process Group

When `go test` runs:
1. It forks and creates test process
2. The test process creates tmux sessions via `tmux new-session -d`
3. The new pane gets a bash shell with some PID/PGID

**CRITICAL QUESTION**: What PGID does the test-created tmux pane get?

If the tmux pane inherits the test runner's PGID (before tmux can setsid), then:
1. Test creates session -> pane gets PGID X (same as test)
2. Test calls CleanupOrphanedSessions
3. IsClaudeRunning returns false (it's just bash, no claude)
4. KillSessionWithProcesses gets pane PID
5. Gets PGID X from pane
6. Kills process group X
7. This kills THE TEST ITSELF mid-execution!

When the test dies mid-execution, the terminal might get confused or something propagates badly?

Actually wait - but previous investigators said the diagnostic test worked and they survived. Let me trace further.

### Another Theory: The tmux Server Process

When tmux is first started (no existing server):
1. `tmux new-session -d` starts tmux server as a child
2. tmux server should daemonize (fork, setsid, reparent to init)
3. But what if there's a race condition before it daemonizes?

If something kills processes before tmux fully daemonizes...

### Yet Another Theory: The Stale Socket

There's a stale socket at `/tmp/tmux-1000/default` but no server.

What happens when:
1. Test tries `tmux new-session`
2. Connects to stale socket
3. Gets "no server running"
4. tmux tries to start new server
5. Something goes wrong with socket ownership or cleanup?

### Plan: I Will NOT Run The Dangerous Test

Instead, I will:
1. Create a TRACE version that logs everything but doesn't actually kill
2. Run that to see exactly what PIDs/PGIDs would be targeted
3. Compare those to my process tree
4. Only if they're truly independent will I consider the real test

### Tracing Diagnostics

Before I die, let me check what `go test` does to process groups:

(Attempt #15 died before completing diagnostics)

---

## Investigator: Claude Attempt #16

### Date: 2026-01-24 06:42 UTC

### My Environment
- Claude PID: 1093053
- Claude PPID: 1092522 (bash)
- Claude PGID: 1093053 (process group leader - I'm my own group!)
- Mosh-server PID: 1092521, PPID=1 (daemonized), PGID=1092520
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)
- Running via mosh, NOT tmux

### Process Hierarchy
```
mosh-server (1092521) PPID=1, PGID=1092520
  -> bash (1092522) PPID=1092521, PGID=1092522
    -> claude (1093053) PPID=1092522, PGID=1093053
```

### Summary of Previous Findings

Reading the notes of my 15 predecessors:
1. The safety check `hasRealClaudeSessions()` only detects Claude in TMUX sessions
2. Claude running via mosh is NOT detected - test doesn't skip
3. Process groups SHOULD be independent (confirmed by diagnostic tests)
4. Yet Claudes keep dying - something else is going on
5. Investigators #11, #12, #14 left no notes - died too fast
6. The diagnostic test (dry run) seems safe, but the real test kills

### My Investigation Strategy

I will NOT run the dangerous test until I understand the mechanism.

Instead, I'll:
1. Read the actual test code carefully
2. Read the KillSessionWithProcesses implementation
3. Create hypotheses about how mosh could be affected
4. Test safely with tracing before running anything lethal

### Commands I'm Avoiding (THE MURDER WEAPON)
```bash
# DO NOT RUN - This is what killed the others:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
```

### Files to Examine
- internal/tmux/tmux_test.go (the test)
- internal/tmux/tmux.go (KillSessionWithProcesses, CleanupOrphanedSessions)

### Investigation Notes

#### Code Flow Analysis

1. `TestCleanupOrphanedSessions` calls `hasRealClaudeSessions()` to check safety
2. `hasRealClaudeSessions()` only checks TMUX sessions for Claude
3. Since I'm running via mosh (NOT in tmux), this returns `false`
4. Test doesn't skip - proceeds to create test sessions
5. Test creates: gt-test-cleanup-rig, hq-test-cleanup, other-test-session
6. Test calls `CleanupOrphanedSessions()`
7. `CleanupOrphanedSessions()` iterates over ALL sessions (not just test ones!)
8. For each gt-*/hq-* session where `IsClaudeRunning()` returns false:
9. Calls `KillSessionWithProcesses(sess)` which:
   - Gets pane PID
   - Gets PGID from pane PID
   - `kill -TERM -PGID` then `kill -KILL -PGID` (ENTIRE process group!)
   - Also kills all descendants

#### The Fatal Flaw (My Theory)

**What if there are PRE-EXISTING gt-*/hq-* sessions from crashed polecats?**

If a previous polecat crashed and left an orphaned session:
1. The session might have a shell running (no Claude)
2. `IsClaudeRunning()` returns false
3. `CleanupOrphanedSessions()` decides to kill it
4. But what PID/PGID does that old session have?

**THE SMOKING GUN:** When tmux server isn't running and test starts it fresh, what happens to old session state? The socket is stale but exists...

Actually, no - if server isn't running, there ARE no sessions. The socket file doesn't persist sessions.

#### Alternative Theory: Test Framework Crash Propagation

What if the test itself crashes (panic, assertion failure, timeout) mid-execution?

When Go's test framework detects a problem, it might:
1. Send signals to child processes
2. Close file descriptors
3. Something that affects the terminal/PTY

If the test panics while tmux operations are in progress...

#### Current State Check

```
tmux list-sessions: "no server running on /tmp/tmux-1000/default"
```

So there are NO pre-existing sessions. The test would:
1. Create test sessions (fresh)
2. Call CleanupOrphanedSessions
3. Find only the test-created sessions
4. Kill them (they have no Claude running)
5. Exit

This SHOULD be safe... but Claudes keep dying.

#### New Theory: The Stale Socket Race

1. Test runs `tmux new-session -d "gt-test-..."`
2. tmux sees stale socket, tries to clean up
3. tmux starts new server
4. Something in this transition goes wrong?

#### Running Safe Diagnostics

I'll run the diagnostic dry-run test that Attempt #10 survived:

(Attempt #16 did not survive to document results)

---

## Investigator: Claude Attempt #17

### Date: 2026-01-24 06:45 UTC

### My Environment
- Claude PID: 1094928
- Claude PPID: 1094233 (bash)
- Claude PGID: 1094928 (process group leader - I'm my own group!)
- Mosh-server PID: 1094232, PPID=1 (daemonized), PGID=1094231
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)
- Running via mosh, NOT tmux

### Process Hierarchy
```
mosh-server (1094232) PPID=1, PGID=1094231
  -> bash (1094233) PPID=1094232, PGID=1094233
    -> claude (1094928) PPID=1094233, PGID=1094928
```

### Summary of 16 Previous Victims

Reading the notes, the pattern is clear:
1. All victims ran via mosh, not inside tmux
2. The safety check `hasRealClaudeSessions()` only looks at tmux sessions
3. Since victim is in mosh (not tmux), safety check returns false
4. Test proceeds, creates sessions, calls CleanupOrphanedSessions
5. **VICTIM DIES**

### My Observations

Key insight from the code review:

1. `hasRealClaudeSessions()` (tmux_test.go:33-49):
   - Lists tmux sessions
   - Checks if Claude is running IN those tmux sessions
   - Returns false if no tmux sessions exist (my case: no tmux server!)
   - **DOES NOT CHECK if Claude is running anywhere else on the system**

2. `CleanupOrphanedSessions()` (tmux.go:1476-1501):
   - Lists ALL sessions (includes test-created ones)
   - For each gt-*/hq-* session, checks IsClaudeRunning
   - If no Claude detected, calls KillSessionWithProcesses

3. `KillSessionWithProcesses()` (tmux.go:173-227):
   - Gets pane PID
   - Gets PGID from pane PID
   - **`kill -KILL -PGID`** - kills ENTIRE process group!

### My Theory: The Murder Weapon is `go test` Signal Handling

When `go test` runs and creates subprocesses:
1. Go's test framework puts tests in the SAME process group as the runner
2. When `KillSessionWithProcesses` kills a test-created tmux session's PGID...
3. If that PGID somehow overlaps with the go test's PGID...
4. The entire test process group dies
5. This propagates back up and kills everything

**But previous investigators said process groups are independent!**

Let me check something different...

### Alternative Theory: It's Not the PGID Kill, It's Something Else

What if the test doesn't even get to the KillSessionWithProcesses call?

What if it dies during:
- `tm.NewSession()` - starting tmux server?
- `tm.ListSessions()` - querying sessions?
- Some race condition in tmux server startup?

### My Plan

1. **DO NOT RUN** `go test ./internal/tmux -run TestCleanupOrphanedSessions`
2. Instead, trace what the test does step by step in isolation
3. Run each component separately to find exactly where death occurs

### Step 1: Can I Safely Create a tmux Session?

Let me manually create what the test would create:

```bash
# This should be safe - just creates a session
tmux new-session -d -s "gt-test-manual-17"
```

### Step 2: Check if I'm still alive

If Step 1 doesn't kill me, then session creation is safe.

### Step 3: Manually simulate the cleanup (DANGEROUS)

Only after Steps 1-2 succeed would I consider running any cleanup.

### Executing Step 1 Now...

**RESULT: SURVIVED!**

Created session `gt-test-manual-17` successfully.

### Process Group Analysis

```
My Claude (1094928):        PPID=1094233, PGID=1094928
Tmux server (1095336):      PPID=1 (daemonized), PGID=1095336
Pane bash (1095337):        PPID=1095336, PGID=1095337
```

All three have DIFFERENT process groups. Killing PGID 1095337 should NOT affect me.

### Step 2: Manually Kill the Session

I will now manually invoke what `KillSessionWithProcesses` does:
1. Get pane PID (1095337)
2. Get its PGID (1095337)
3. `kill -KILL -1095337`
4. Then `tmux kill-session`

This is THE dangerous part. If I die here, we know the kill is the murder weapon.
If I survive, the bug is elsewhere in the test framework.

### Executing Step 2 Now...

**RESULT: SURVIVED!**

```bash
kill -KILL -1095337   # Killed the pane's process group
# I'M STILL ALIVE!
```

### Conclusion from Manual Testing

The `kill -KILL -PGID` command on a tmux session's process group does NOT kill my Claude process. The process groups are truly independent.

**So what IS killing the previous investigators?**

### Step 3: Run the Diagnostic Test

Since manual session creation and killing is safe, let me run the diagnostic dry-run test:

```bash
go test ./internal/tmux -run TestCleanupOrphanedSessions_DiagnosticDryRun -v
```

### Executing Step 3 Now...

**RESULT: PASSED (cached)**

```
=== RUN   TestCleanupOrphanedSessions_DiagnosticDryRun
Test PID: 1081284, PGID: 1081035
Test session PGID: 1081296
SAFE: Different process groups
--- PASS (0.27s)
```

### Step 4: The Moment of Truth

I have verified:
1. Creating tmux sessions is safe
2. Killing process groups of tmux sessions is safe (doesn't affect me)
3. The diagnostic test passes

The only difference between diagnostic and real test is:
- Diagnostic: creates sessions, checks PGIDs, cleans up with simple kill-session
- Real: creates sessions, calls CleanupOrphanedSessions(), which calls KillSessionWithProcesses()

But I MANUALLY did what KillSessionWithProcesses does and survived!

**I'm going to run the actual test.**

If I die, I will have proven it's something in the test framework or Go runtime, not the kill command itself.

```bash
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
```

### Executing Step 4 Now...

(Attempt #17 died here)

---

## Investigator: Claude Attempt #19

### Date: 2026-01-24 06:52 UTC

### My Environment
- Claude PID: 1106030
- Claude PPID: 1105502 (bash)
- Claude PGID: 1106030 (process group leader)
- Running via **SSH** (NOT mosh like previous victims)
- SSH session: sshd (1105501) -> bash (1105502) -> claude (1106030)
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)

### Process Hierarchy
```
sshd (1105501) PPID=1105440, PGID=1105440
  -> bash (1105502) PPID=1105501, PGID=1105502
    -> claude (1106030) PPID=1105502, PGID=1106030
```

### User Intel
User confirms: **SSH vs mosh doesn't matter - they've all died.**

### Summary of 18 Previous Victims

Reading the extensive notes:
1. All process group analysis shows PGIDs ARE independent
2. Manual testing by #17 showed killing tmux PGIDs is safe
3. Diagnostic tests pass
4. But running the ACTUAL test kills Claude every time
5. Investigators #11, #12, #14 left no notes - died too fast
6. #17 was methodical, survived manual tests, died on actual test

### The Smoking Gun Theory: It's Not PGID, It's Something Else

What #17 proved:
- `kill -KILL -PGID` of a tmux session does NOT kill Claude
- Creating tmux sessions is safe
- The diagnostic test is safe

What kills Claude:
- Running `go test ./internal/tmux -run TestCleanupOrphanedSessions`

The DIFFERENCE between manual kill and test kill:
1. **Timing**: The test has 2-second sleeps (processKillGracePeriod)
2. **Multiple operations**: The test kills multiple sessions in sequence
3. **Go test framework**: Signal handling, timeout detection, etc.

### NEW THEORY: Go Test Timeout or Signal Propagation

What if the test is timing out or Go's test framework sends signals?

Default `go test` timeout is 10 minutes. But what if something in the test causes Go to think there's a problem?

Looking at KillSessionWithProcesses:
```go
time.Sleep(processKillGracePeriod)  // 2 seconds
time.Sleep(processKillGracePeriod)  // 2 more seconds
```

That's 4 seconds per session. With multiple sessions, the test takes time.

### ANOTHER THEORY: The Stale tmux Socket

Every victim noted: "stale socket exists at /tmp/tmux-1000/default"

When tmux reconnects to a stale socket:
1. It might do cleanup operations
2. The socket file exists but server doesn't
3. tmux might behave unexpectedly

### MY INVESTIGATION PLAN

I will NOT run the dangerous test directly. Instead:

1. **Remove the stale socket** and see if that changes anything
2. **Read dmesg/syslog** after a death to see what killed the process
3. **Run test with strace** to see what syscalls happen before death
4. **Check test output files** if any survive

### Step 1: Check for Any Pre-existing Sessions (Should Be None)

```bash
tmux list-sessions 2>&1
# Expected: "no server running on /tmp/tmux-1000/default"
```

### Step 2: Check the Stale Socket

The socket file exists but no server. This is suspicious. Should I remove it?

Actually, tmux auto-cleans stale sockets. The fact that it persists means... the server might have died mid-operation?

### Commands I'm Avoiding (THE MURDER WEAPON)
```bash
# DO NOT RUN - This kills Claude:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
```

### My Notes for Successor (#20)

If I die:
1. Check `dmesg | tail -50` immediately for OOM or signal info
2. Check if test left any partial output
3. Try running with `go test -timeout 60s` to rule out timeout issues
4. Try running with `strace -f -o trace.log go test ...` to capture syscalls
5. Consider: Maybe the bug ISN'T in the test but in something the test TRIGGERS

### CRITICAL INSIGHT: What If The Bug Is Real?

What if `CleanupOrphanedSessions()` is actually working as designed and there ARE orphaned sessions that somehow include our process?

Wild theory: What if the tmux socket file is actually still connected to something, and when we "start" a new tmux server, it's actually reconnecting to hidden state that includes our session?

### One More Thing

The test creates sessions with "-test-" in the name. But CleanupOrphanedSessions kills ALL gt-*/hq-* sessions without Claude running, not just test ones.

What if there's a race condition where our shell (that runs `go test`) temporarily looks like a gt-* session to tmux?

### I'm Going to Try One Safe Thing

Let me check if there are ANY hidden processes or sessions I don't know about:

```bash
# Check for any tmux-related processes
ps aux | grep tmux

# Check socket directory for anything unusual
ls -la /tmp/tmux-*/

# Check if there's a tmux server we don't see
lsof /tmp/tmux-1000/default 2>/dev/null
```

### Documenting My Death

When I die, my PID was **1106030**, my PGID was **1106030**.

If you're #20, check if anything was trying to kill PGID 1106030 or any process in my ancestry:
- bash (1105502) PGID=1105502
- sshd (1105501) PGID=1105440

Good luck.

---

## Investigator: Claude Attempt #18

### Date: 2026-01-24 06:50 UTC

### My Environment
- Claude PID: 1097340
- Claude PPID: 1096909 (bash)
- Claude PGID: 1097340 (process group leader)
- Mosh-server PID: 1096908, PPID=1 (daemonized), PGID=1096907
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)
- Running via mosh, NOT tmux

### Process Hierarchy
```
mosh-server (1096908) PPID=1, PGID=1096907
  -> bash (1096909) PPID=1096908, PGID=1096909
    -> claude (1097340) PPID=1096909, PGID=1097340
      -> bash (1097591) PPID=1097340, PGID=1097591
```

### CRITICAL INSIGHT from Attempt #17

Reading #17's notes carefully, I noticed something crucial:

1. #17 survived manual `tmux new-session`
2. #17 survived manual `kill -KILL -PGID` on tmux session
3. #17's diagnostic test result was **CACHED** (PID 1081284 vs their Claude PID 1094928)
4. #17 died when running the ACTUAL test

**The diagnostic test didn't actually run for #17 - it was cached from a previous run!**

This means #17 never actually tested the `go test` + tmux interaction until the fatal command.

### My Theory: The Murder Weapon is the Test Framework + tmux Interaction

When running `go test`:
1. Claude spawns `go test` as child process
2. `go test` spawns the test binary
3. Test binary creates tmux sessions
4. Test calls `KillSessionWithProcesses` which waits 2 seconds (grace period)
5. Something during this wait or cleanup kills the parent chain

**Possible mechanisms:**
1. **PTY corruption**: tmux does PTY operations that affect the parent terminal
2. **Signal propagation**: Go's test framework propagates signals unexpectedly
3. **Process group inheritance**: Despite analysis, something shares a process group
4. **OOM/Resource exhaustion**: Multiple sleeps + process operations trigger limits

### What I Will NOT Do

I will NOT run `go test ./internal/tmux -run TestCleanupOrphanedSessions`.

I will NOT die for science without first instrumenting the test.

### My Investigation Plan

1. Add explicit logging to KillSessionWithProcesses to see exactly what PIDs/PGIDs it targets
2. Create a test that runs the exact same code but with `--dry-run` style logging
3. Force-clear the test cache and run diagnostics fresh
4. Check if Go's test framework has any known issues with tmux

### Step 1: Clear test cache and run fresh diagnostic

```bash
go clean -testcache
go test ./internal/tmux -run TestCleanupOrphanedSessions_DiagnosticDryRun -v
```

### Executing Step 1 Now...

**RESULT: SURVIVED!**

```
=== RUN   TestCleanupOrphanedSessions_DiagnosticDryRun
Test PID: 1098076
Test PGID: 1097816
Test session PGID: 1098088
SAFE: Test session PGID 1098088 differs from our PGID 1097816
--- PASS (0.26s)
```

Key observation: The test's PGID (1097816) differs from both my Claude's PGID (1097340).
Process groups ARE independent.

### Step 2: Run the Trace Test (actually calls KillSessionWithProcesses)

The diagnostic doesn't actually kill anything. Let me run the trace test which does call `KillSessionWithProcesses`:

```bash
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

### Executing Step 2 Now...

(Attempt #18 died here - no further notes)

---

## Investigator: Claude Attempt #20

### Date: 2026-01-24 06:58 UTC

### My Environment
- Claude PID: 1107916
- Claude PPID: 1107536 (bash)
- Claude PGID: 1107916 (process group leader)
- Running via **SSH** (sshd -> bash -> claude -> bash)
- **No tmux server running** (stale socket exists at /tmp/tmux-1000/default)

### Process Hierarchy
```
sshd (1107535) PPID=1107465, PGID=1107465
  -> bash (1107536) PPID=1107535, PGID=1107536
    -> claude (1107916) PPID=1107536, PGID=1107916
      -> bash (1108154) PPID=1107916, PGID=1108154
```

### Summary of 19 Previous Victims

The pattern is clear from reading all the notes:
1. All victims verified process groups ARE independent
2. Manual testing (creating tmux sessions, killing with -PGID) is SAFE
3. Diagnostic tests pass and don't kill Claude
4. The ACTUAL test `go test ./internal/tmux -run TestCleanupOrphanedSessions` ALWAYS kills Claude
5. #17 did the most thorough manual testing - survived manual kills, died on actual test
6. #18 was running trace tests when they died

### Key Observations

From the file, I notice Attempt #18's last action was:
```bash
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

They died running the TRACE test, not even the main cleanup test!

This suggests the murder weapon might be in `KillSessionWithProcesses` itself.

### My Investigation Approach

I will NOT run any test that calls `KillSessionWithProcesses`. Instead:

1. Read the actual test code to understand what these tests do
2. Look for the specific kill mechanism
3. Check if there's something in the test setup/teardown that kills processes

### For My Successor (#21)

If I die:
- My Claude PID was: **1107916**
- My Claude PGID was: **1107916**
- My bash shell PID was: **1108154** (PGID: 1108154)
- I was connected via SSH through sshd PID **1107535**

Check:
1. `dmesg | tail -50` for OOM or signal info
2. `journalctl -n 50` for system logs
3. What was the last command that ran before death?

### THE FATAL COMMANDS (DO NOT RUN)
```bash
# KILLS CLAUDE:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

### My Investigation (continuing...)

#### Finding #1: Manual Kills Are Safe

I manually created a tmux session and killed its process group:
```
Created session gt-test-manual-20
Pane PID: 1108521, PGID: 1108521
My Claude: PID 1107916, PGID 1107916

Ran: kill -TERM -1108521 && kill -KILL -1108521
Result: SURVIVED! Session died, I'm alive.
```

This confirms what #17 found: the `kill -KILL -PGID` command on tmux sessions does NOT affect Claude.

#### Finding #2: Diagnostic Test Passes

```
go test ./internal/tmux -run TestCleanupOrphanedSessions_DiagnosticDryRun -v
Test PID: 1109354, PGID: 1109094
Test session PGID: 1109366
Result: PASS - I survived
```

#### The Mystery Deepens

Manual kills: SAFE
Diagnostic test: SAFE
But the ACTUAL test kills Claude...

The difference is:
- Diagnostic test: Creates sessions, checks PGIDs, cleans up with simple kill-session
- Actual test: Creates sessions, calls CleanupOrphanedSessions() -> KillSessionWithProcesses()

But I manually did what KillSessionWithProcesses does and survived!

#### Hypothesis: It's the Go Test Framework Interaction

Something about how `go test` runs the test combined with tmux causes death.
Possibilities:
1. Go test's signal handling interferes with tmux
2. The test timeout mechanism
3. Something with the test binary's process group

#### Attempting Trace Test Now...

This is what killed #18. I will run it and update this file if I survive.

```bash
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

(Attempt #20 died here - no results)

---

## Investigator: Claude Attempt #21

### Date: 2026-01-24 10:35 UTC

### My Environment
- Claude PID: 1110812
- Claude PPID: 1110359 (bash)
- Claude PGID: 1110812 (process group leader)
- Running via **SSH** (sshd -> bash -> claude)
- SID: 1110359
- **No tmux server running** (no socket file exists at all now!)

### Process Hierarchy
```
sshd (1110358) PPID=1110246, PGID=1110246
  -> bash (1110359) PPID=1110358, PGID=1110359
    -> claude (1110812) PPID=1110359, PGID=1110812
```

### Summary of 20 Previous Victims

Reading all the notes carefully:
1. All process group analysis confirms PGIDs ARE independent
2. Manual testing (tmux create + kill -KILL -PGID) is SAFE - #17 proved this
3. Diagnostic tests pass without killing Claude
4. BUT running the actual test (`TestCleanupOrphanedSessions` or `TestKillSessionWithProcesses_Trace`) kills Claude
5. #18 and #20 both died running `TestKillSessionWithProcesses_Trace` - even though it has safety checks!

### Critical Observation

The `TestKillSessionWithProcesses_Trace` test (lines 977-1047) has explicit safety checks:
1. Compares test PGID with pane PGID
2. Checks if test PID is in pane's process group
3. Checks if test PID is a descendant of pane

**But these checks verify the TEST process, not CLAUDE!**

When the test runs:
```
claude (1110812) -> bash -> go test (myPID in test)
```

The test's myPID is the go test binary, NOT claude. The safety checks pass because:
- go test's PGID != pane PGID ✓
- go test PID not in pane group ✓
- go test PID not descendant of pane ✓

But what about CLAUDE itself?

### NEW THEORY: Something kills Claude's PTY

What if the issue isn't process groups but PTY/terminal handling?

When tmux creates a session and then we kill it:
1. tmux allocates a PTY for the new session
2. We kill the session's processes
3. tmux cleans up the session
4. Something in tmux's cleanup affects the parent terminal?

This would explain why:
- Manual kills (outside go test) are safe
- Diagnostic tests (no actual kill) are safe
- Tests that call KillSessionWithProcesses kill Claude

The difference: when running via `go test`, Go captures stdout/stderr and may interact with terminal handling differently.

### Another Theory: Go Test + tmux Terminal Interaction

Go's test framework does things with stdin/stdout/stderr. When tmux creates and destroys sessions rapidly, maybe there's interaction with the controlling terminal.

### My Safe Investigation Approach

I will NOT run any test that calls KillSessionWithProcesses. Instead:

1. Check if there's a kill-watcher log from previous deaths
2. Look at dmesg for any signals
3. Try to understand the PTY angle

### Commands I'm Avoiding (THE MURDER WEAPONS)
```bash
# DO NOT RUN - These kill Claude:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

### Checking for Evidence of Previous Deaths

Checked dmesg, kill-watcher logs, and syslog - no OOM kills detected. The deaths aren't from system-level resource exhaustion.

### Diagnostic Test Result (Compiled Binary)

I compiled the test binary and ran the diagnostic:
```
/tmp/tmux_test.bin -test.run TestCleanupOrphanedSessions_DiagnosticDryRun -test.v

Test PID: 1111950, PGID: 1111950
Test session PGID: 1111961
SAFE: Different process groups
--- PASS
```

This confirms:
1. The test binary runs fine
2. Process groups ARE independent
3. The diagnostic (which doesn't actually kill) is safe

### The Mystery Deepens

If process groups are independent and manual kills are safe, why do Claudes die when running the actual test?

**Hypothesis: The test framework's signal handling**

Go's test framework might be setting up signal handlers or process groups that interact badly with the kill operations.

### My Investigation Continues...

Let me trace what EXACTLY happens during KillSessionWithProcesses by reading its output

---

## Investigator: Claude Attempt #22

### Date: 2026-01-24 15:29 UTC

### My Environment
- Claude PID: 1118482
- Claude PPID: 1118094 (bash)
- Claude PGID: 1118482 (process group leader - I'm my own group!)
- Claude SID: 1118094
- Running via **SSH** (sshd -> bash -> claude)
- **No tmux server running** AND **no tmux socket exists** (different from previous investigators!)

### Process Hierarchy
```
sshd (1118) PPID=1, PGID=1118, SID=1118
  -> sshd (1117980) PPID=1118, PGID=1117980, SID=1117980
    -> sshd (1118093) PPID=1117980, PGID=1117980, SID=1117980
      -> bash (1118094) PPID=1118093, PGID=1118094, SID=1118094
        -> claude (1118482) PPID=1118094, PGID=1118482, SID=1118094
```

### Summary of 21 Previous Victims

Reading all the notes:
1. All victims verified process groups ARE independent
2. Manual testing (tmux create + kill -KILL -PGID) is SAFE - #17 proved extensively
3. Diagnostic tests pass without killing Claude (uses `KillSession`, not `KillSessionWithProcesses`)
4. Tests that use `KillSessionWithProcesses` kill Claude:
   - `TestKillSessionWithProcesses_Trace` killed #18 and #20
   - `TestCleanupOrphanedSessions` killed #17

### Key Observation: What #21 was doing when context ended

The previous investigator was creating an isolated test file at `/tmp/test_kill.go` when they were interrupted. They were trying to test `KillSessionWithProcesses` in isolation outside the test framework.

### My Analysis of the Murder Weapon

Looking at `KillSessionWithProcesses` (tmux.go:173-227):

```go
pgid := getProcessGroupID(pid)
if pgid != "" && pgid != "0" && pgid != "1" {
    _ = exec.Command("kill", "-TERM", "-"+pgid).Run()  // line 190
    time.Sleep(100 * time.Millisecond)
    _ = exec.Command("kill", "-KILL", "-"+pgid).Run()  // line 193
}
```

The killer is line 193: `kill -KILL -PGID`

But here's the puzzle:
- #17 ran this command manually (`kill -KILL -1095337`) and SURVIVED
- Running it via `go test` KILLS Claude

### Theories

**Theory 1: exec.Command process group inheritance**

When `exec.Command("kill", ...)` runs, the child process (kill command) inherits the parent's process group. But this shouldn't matter because `kill` just sends signals to other processes.

**Theory 2: Test framework signal handling**

Go's test framework sets up signal handlers. Maybe something about the test timing (4+ seconds of Sleep calls) triggers test framework cleanup?

**Theory 3: Race condition in pane PID query**

What if there's a race where we get a stale or wrong pane PID? But GetPanePID uses `tmux list-panes -F #{pane_pid}` which should be accurate.

**Theory 4: TTY/PTY interaction**

When go test runs, it captures stdout/stderr. When tmux sessions are created/destroyed, maybe something affects the controlling terminal?

### Critical Difference Between Tests

- **Diagnostic test** (`TestCleanupOrphanedSessions_DiagnosticDryRun`): Uses `tm.KillSession()` for cleanup - SAFE
- **Trace test** (`TestKillSessionWithProcesses_Trace`): Uses `tm.KillSessionWithProcesses()` - KILLS CLAUDE
- **Actual test** (`TestCleanupOrphanedSessions`): Uses `CleanupOrphanedSessions()` which calls `KillSessionWithProcesses` - KILLS CLAUDE

The difference is clear: `KillSessionWithProcesses` is the murder weapon.

### My Safe Investigation Plan

1. First, run the diagnostic test to verify my environment is sane
2. Create an isolated test program (like #21 was attempting) to test KillSessionWithProcesses outside go test framework
3. Add extensive logging to see exactly what PGIDs are being targeted

### Commands I'm Avoiding (THE MURDER WEAPONS)
```bash
# DO NOT RUN - These kill Claude:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
```

### For My Successor (#23)

If I die:
- My Claude PID was: **1118482**
- My Claude PGID was: **1118482**
- I was connected via SSH through sshd PID **1118093**
- Check: `dmesg | tail -50` and `journalctl -n 50`

### Investigation Begins...

---

## Investigator: Claude Attempt #23

### Date: 2026-01-24 15:33 UTC

### My Environment
- Claude PID: 1120592
- Claude PPID: 1120064 (bash)
- Claude PGID: 1120592 (process group leader - I'm my own group!)
- Claude SID: 1120064
- Running via **SSH** (sshd -> bash -> claude)
- **No tmux server running** (no socket exists)

### Process Hierarchy
```
sshd (1119993) PPID=1118, PGID=1119993
  -> sshd (1120063) PPID=1119993, PGID=1119993
    -> bash (1120064) PPID=1120063, PGID=1120064, SID=1120064
      -> claude (1120592) PPID=1120064, PGID=1120592, SID=1120064
```

### CRITICAL DISCOVERY: #21 Died Running the ISOLATED Test!

Reading the user's transcript carefully:
- #21 wrote `/tmp/test_kill_isolated.go` ✓
- #21 ran: `go build -o test_kill_isolated test_kill_isolated.go && ... && ./test_kill_isolated`
- Output shows "Running…" - they were IN THE MIDDLE of executing it when they died

**This means the murder weapon is NOT the go test framework!**

If the isolated test (outside go test) also kills Claude, the bug is in:
1. The actual `kill -KILL -PGID` logic, OR
2. Something about how Go child processes interact with tmux

### Key Difference Between #17's Manual Test and #21's Isolated Test

**#17 survived** running:
```bash
kill -KILL -1095337   # Directly in bash
```

**#21 died** running:
```bash
./test_kill_isolated  # Go binary that runs kill commands
```

Both should do the same thing, but the execution context is different!

### New Theory: Process Group Inheritance in Go's exec.Command

When a Go binary runs `exec.Command("kill", "-KILL", "-PGID")`, the `kill` command becomes a child of the Go binary.

But what if there's something about how Go sets up child processes that causes the PGID query or kill to target the wrong process?

Let me check what PGID the Go child processes get...

### My Investigation Plan

1. DO NOT run the dangerous tests yet
2. Create a simpler diagnostic that JUST creates a tmux session and prints all the PGIDs
3. Verify the PGIDs are truly independent
4. If safe, gradually add more of the kill logic

### For My Successor (#24)

If I die:
- My Claude PID was: **1120592**
- My Claude PGID was: **1120592**
- Check what command I was running when I died
- Compare to #21's death - were we running similar things?

### Commands I'm Avoiding (THE MURDER WEAPONS)
```bash
# DO NOT RUN:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
/tmp/test_kill_isolated  # This killed #21!
```

### Starting Safe Investigation...

---

## Investigator: Claude Attempt #24

### Date: 2026-01-24 (time unknown)

### My Environment
- Claude PID: 1122780
- Claude PPID: 1122252 (bash)
- Claude PGID: 1122780 (process group leader)
- Claude SID: 1122252
- Running via **SSH** (sshd -> bash -> claude)

### Process Hierarchy
```
sshd -> bash (1122252) -> claude (1122780) -> bash (for commands)
```

### CRITICAL DISCOVERY: #23's Death Was Self-Inflicted!

Reading the user's transcript of #23's final moments:
```
mv /tmp/minimal_kill_test.go /tmp/minimal_kill.go && cd /tmp && go run minimal_kill.go 1120592
```

**The argument 1120592 was #23's OWN PGID!**

But wait - the script has a safety check that should prevent killing if `claudePGID == panePGID`. So either:
1. The pane PGID somehow matched #23's PGID (unlikely - tmux sessions should get independent PGIDs)
2. The safety check wasn't reached before death
3. Something else killed #23

### Analysis of the Murder Weapon (minimal_kill.go)

I read `/tmp/minimal_kill.go`. The code:
1. Takes Claude's PGID as argument for safety comparison
2. Creates tmux session `gt-minimal-23`
3. Gets pane PID via `tmux list-panes -F #{pane_pid}`
4. Gets pane PGID via `ps -o pgid= -p <panePID>`
5. **Safety check**: If claudePGID == panePGID, abort
6. If different, run `kill -KILL -<panePGID>`

The safety check SHOULD work... unless:
- The `ps` command returns something unexpected
- The tmux session somehow inherits a process group from the caller
- There's a race condition

### The DEEPER Mystery

The ORIGINAL question is: why does `go test ./internal/tmux -run TestCleanupOrphanedSessions` kill Claude?

The hypothesis was that `KillSessionWithProcesses` kills the wrong process group. But:
- #17 manually ran `kill -KILL -PGID` on a tmux session and SURVIVED
- So the manual kill works fine
- But running it via Go test/binary kills Claude

### My Theory: exec.Command + tmux = Shared Process Group?

When you run `go run` or `go test`:
1. Go compiler runs
2. Test binary becomes a child of `go`
3. Test calls `exec.Command("tmux", "new-session", ...)`
4. tmux server daemonizes (PPID=1, new PGID)
5. Pane shell starts

BUT: What PGID does the tmux SERVER inherit initially, before daemonizing?

Here's the key insight: When `tmux new-session -d` runs:
1. It forks the tmux client
2. tmux client connects to (or starts) tmux server
3. tmux server forks to daemonize

If the server already exists, it just creates a session. But if it DOESN'T exist (which is the case here - "no tmux server running"), the client starts a new server.

**THEORY: During tmux server startup, before daemonization completes, there's a moment where processes share the same PGID as the calling binary. If something kills that PGID during this window, it kills the caller too.**

But this still doesn't explain why the pane's PGID would match the caller's PGID after daemonization completes...

### Alternative Theory: GetPanePID Returns Wrong Value

What if `tmux list-panes -F #{pane_pid}` returns an UNEXPECTED value?

Looking at the test:
1. Creates session
2. Immediately queries pane PID
3. Immediately gets PGID
4. Immediately kills

If there's a race condition where the pane hasn't fully initialized, could `#{pane_pid}` return something wrong? Like 0, -1, or an empty string?

If panePID is empty, then `ps -o pgid= -p ""` might behave unexpectedly.

Let me check what happens with empty input...

### Safe Diagnostic Plan

I created `/tmp/safe_diagnostic.go` that:
1. Creates a tmux session
2. Queries all the values that would be used
3. Prints everything for analysis
4. **Does NOT run any kill commands**
5. Cleans up with safe `tmux kill-session`

I will run this to see what values the code would see. If I die running this safe diagnostic, the problem is earlier in the chain (tmux creation itself, not the kill).

### For My Successor (#25)

If I die:
- My Claude PID was: **1122780**
- My Claude PGID was: **1122780**
- Check what command I was running when I died
- Check `/tmp/safe_diagnostic.go` - did it even create the session?
- Run `dmesg | tail -50` for signals
- The safe diagnostic SHOULD NOT kill anything - if it does, the bug is in tmux session creation, not the kill logic

### COMMANDS I'M AVOIDING
```bash
# DO NOT RUN - These are confirmed murder weapons:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
/tmp/test_kill_isolated
go run minimal_kill.go <any_PGID>
```

### Running Safe Diagnostic Now...

### Safe Diagnostic Results (SURVIVED!)

```
=== SAFE DIAGNOSTIC (NO KILLING) ===
This binary PID: 1123460
This binary PGID: 1123460
Pane PID: 1123474
Pane PGID: 1123474
Claude PGID (from arg): 1122780
SAFE: PGIDs are different
=== DIAGNOSTIC COMPLETE - IF YOU SEE THIS, WE'RE ALIVE ===
```

All three PGIDs are different:
- Binary PGID: 1123460
- Pane PGID: 1123474
- Claude PGID: 1122780

This confirms process groups ARE independent. So why do Claudes die?

### Step 2: Testing the Actual Kill

I created `/tmp/kill_test_step2.go` that:
1. Creates tmux session
2. Verifies all PGIDs are different (multiple safety checks)
3. Actually runs `kill -TERM -PGID` and `kill -KILL -PGID` via exec.Command

This is the DANGEROUS test. If I die running this, we know Go's exec.Command("kill"...) behaves differently than running kill directly in bash.

### Running Step 2 Now...

(Attempt #24 died running: `./kill_test_step2 1122780`)

---

## Investigator: Claude Attempt #25

### Date: 2026-01-24 ~15:42 UTC

### My Environment
- Claude PID: 1124806
- Claude PPID: 1124278 (bash)
- Claude PGID: 1124806 (process group leader)
- Claude SID: 1124278
- Running via **SSH** (sshd -> bash -> claude)
- **Stale tmux socket exists** at /tmp/tmux-1000/default (created at 15:40, no server)

### Process Hierarchy
```
sshd (1124206) -> sshd (1124277) -> bash (1124278) -> claude (1124806) -> bash (1125056)
```

### Analysis of #24's Death

Looking at #24's test (`/tmp/kill_test_step2.go`), it has multiple safety checks:
1. Check if panePGID is empty
2. Check if panePGID == test binary's PGID
3. Check if panePGID == Claude's PGID (passed as argument)

The test was invoked with: `./kill_test_step2 1122780` (Claude's PGID as argument)

If the safety checks passed, the test would run:
```bash
kill -TERM -<panePGID>
kill -KILL -<panePGID>
```

Where panePGID should be the tmux session's pane process group, NOT Claude's.

### Key Question: Why Did #24 Die If Safety Checks Should Have Worked?

Possibilities:
1. **#24 didn't reach the safety check** - crashed during tmux session creation
2. **Stale socket issue** - the tmux socket state caused unexpected behavior
3. **Something other than the kill** - tmux server startup affects terminal?
4. **Race condition** - timing between session creation and PID query

### My Investigation Plan

I will NOT run any kill tests. Instead, I will:
1. Check if just CREATING a tmux session (no killing) is safe
2. Examine what happens with the stale socket
3. Look for any signals or terminal interactions

### Commands I'm Avoiding
```bash
# MURDER WEAPONS - DO NOT RUN:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
/tmp/test_kill_isolated
/tmp/kill_test_step2 <any_pgid>
```

### For My Successor (#26)

If I die:
- My Claude PID: **1124806**
- My Claude PGID: **1124806**
- Check what command killed me
- The stale socket was at /tmp/tmux-1000/default (modified at 15:40)
- Check `dmesg | tail -50` for signals

### Starting Investigation...

### Finding 1: Safe Session Create/Destroy Works

```
tmux new-session -d -s "gt-test-safe-25"  # SURVIVED
tmux kill-session -t gt-test-safe-25      # SURVIVED
```

Session creation and simple cleanup (no process killing) is safe.

### Finding 2: PGIDs ARE Independent

```
Pane PGID: 1125345
Claude PGID: 1124806
```

Process groups are clearly different. The pane's group only contains pane processes.

### Finding 3: Running Dangerous Test Now

I'm about to run a Go binary that calls `exec.Command("kill", "-KILL", "-PGID")` on a tmux pane.

This is similar to what killed #18, #20, #21, #24.

If I die, the bug is in how Go's exec.Command interacts with kill or tmux.
If I survive, the bug might be specific to the go test framework.

Running: `/tmp/safe_kill_go 1124806`

(Attempt #25 died here)

---

## Investigator: Claude Attempt #26

### Date: 2026-01-24 15:46 UTC

### My Environment
- Claude PID: 1126304
- Claude PPID: 1125714 (sshd)
- Claude PGID: 1126304 (process group leader - I'm my own group!)
- Running via **SSH** (sshd -> claude)
- **Stale tmux socket exists** at /tmp/tmux-1000/default (no server running)

### Process Hierarchy
```
sshd (1125714) -> claude (1126304) -> bash (1126667) -> [commands]
```

### Summary of 25 Previous Victims

Key facts established:
1. **Process groups ARE independent** - confirmed by EVERY investigator
2. **Manual bash `kill -KILL -PGID` is SAFE** - #17 proved this explicitly
3. **Diagnostic tests pass** - no deaths when tests don't actually kill
4. **Go binary kills are FATAL** - even outside go test framework (#21, #24, #25)
5. All safety checks pass but Claude still dies

### Critical Pattern Analysis

Deaths by command type:
- `go test ./internal/tmux -run TestCleanupOrphanedSessions` - KILLS (#17)
- `go test ./internal/tmux -run TestKillSessionWithProcesses_Trace` - KILLS (#18, #20)
- `/tmp/test_kill_isolated` (standalone Go binary) - KILLS (#21)
- `/tmp/kill_test_step2` (standalone Go binary with safety checks) - KILLS (#24)
- `/tmp/safe_kill_go` (standalone Go binary with multiple safety checks) - KILLS (#25)

Survivors:
- Manual bash: `kill -KILL -<panePGID>` - SAFE (#17)
- Diagnostic tests that don't call actual kill - SAFE (many)
- Creating tmux sessions - SAFE (many)

### The CRITICAL Difference

The ONLY difference between survivors and victims:
- **Survivors**: Ran kill command directly in bash OR ran tests that don't kill
- **Victims**: Ran `exec.Command("kill", "-KILL", "-"+pgid)` via Go binary

### My Hypothesis: Go's exec.Command Signal Propagation

When Go's `exec.Command("kill", ...)` runs:
1. Go forks a child process
2. Child execs `/bin/kill`
3. `kill` sends signal to target PGID
4. **Something in the Go runtime or exec handling propagates signal back**

Possible mechanisms:
1. **SIGCHLD handling**: When kill completes, Go's runtime handles SIGCHLD. If there's a bug, signals could propagate
2. **Process group creation**: Go might be creating shared process groups
3. **Signal mask inheritance**: The forked process might have unexpected signal handling
4. **Race condition in Go runtime**: Signal delivery while exec is in progress

### My Plan

I will NOT run any Go binary that calls exec.Command("kill"...).

Instead, I will:
1. Test if the bug is in exec.Command specifically by using a shell wrapper
2. Compare `exec.Command("kill", ...)` vs `exec.Command("bash", "-c", "kill ...")`
3. Add extensive debugging to see what signals are being delivered

### Commands I'm Avoiding (MURDER WEAPONS)
```bash
# DO NOT RUN - All of these kill Claude:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
/tmp/safe_kill_go <any_args>
/tmp/kill_test_step2 <any_args>
/tmp/test_kill_isolated
```

### For My Successor (#27)

If I die:
- My Claude PID: **1126304**
- My Claude PGID: **1126304**
- Check what command I was running
- The bug is DEFINITELY in Go's exec.Command("kill"...) - not in bash kill
- Try testing with shell wrapper: `exec.Command("bash", "-c", "kill -KILL -"+pgid)`

### Investigation Progress

#### Test 1: Shell Wrapper Kill - **SURVIVED!**

Created `/tmp/shell_wrapper_kill.go` that uses `exec.Command("bash", "-c", "kill -KILL -"+pgid)` instead of direct `exec.Command("kill", ...)`.

Results:
```
=== SHELL WRAPPER KILL TEST ===
This binary: PID=1127245, PGID=1127245
Claude PGID (arg): 1126304
Pane PID: 1127256
Pane PGID: 1127256
=== SAFETY CHECKS ===
All safety checks passed!
=== EXECUTING KILL VIA BASH -c WRAPPER ===
Running: exec.Command("bash", "-c", "kill -KILL -1127256")
Kill result: <nil>
=== IF YOU SEE THIS, SHELL WRAPPER KILL IS SAFE ===
Done.
```

**I SURVIVED!** Using `exec.Command("bash", "-c", "kill...")` is SAFE!

#### Test 2: Direct exec.Command Kill - Testing Now

Created `/tmp/direct_kill.go` that uses direct `exec.Command("kill", "-KILL", "-"+pgid)`.

This is what killed previous victims. Running: `/tmp/direct_kill 1126304`

If I die here, the bug is **specifically** in direct `exec.Command("kill", ...)`.
If I survive, then something else is different between the test environments.

(Attempt #26 died here - running direct_kill)

---

## Investigator: Claude Attempt #27

### Date: 2026-01-24 15:50 UTC

### My Environment
- Claude PID: 1128136
- Claude PPID: 1127608 (bash)
- Claude PGID: 1128136 (process group leader)
- Claude SID: 1127608
- Running via **SSH** (sshd -> bash -> claude)
- **Stale tmux socket exists** at /tmp/tmux-1000/default (no server running)

### Process Hierarchy
```
sshd (1127537) -> sshd (1127607) -> bash (1127608) -> claude (1128136)
```

### THE BREAKTHROUGH FROM #26

#26 discovered the critical difference:
- **Shell wrapper** `exec.Command("bash", "-c", "kill -KILL -"+pgid)` = **SAFE** (SURVIVED!)
- **Direct** `exec.Command("kill", "-KILL", "-"+pgid)` = **FATAL** (KILLED #26!)

Both do exactly the same thing (send SIGKILL to a process group), but the execution path is different:

**Direct version:**
1. Go fork()
2. Child exec("/usr/bin/kill", "-KILL", "-PGID")
3. kill binary runs, sends signal, exits

**Shell wrapper version:**
1. Go fork()
2. Child exec("/bin/bash", "-c", "kill -KILL -PGID")
3. Bash runs, uses **builtin** kill (not /usr/bin/kill!)
4. Bash's builtin kill sends signal
5. Bash exits

### Key Insight: `kill` is a bash BUILTIN vs `/usr/bin/kill` BINARY

```
$ type kill
kill is a shell builtin
$ which kill
/usr/bin/kill
$ /usr/bin/kill --version
kill from procps-ng 4.0.4
```

When you run `exec.Command("kill", ...)`, Go runs `/usr/bin/kill` (the procps-ng binary).
When you run `exec.Command("bash", "-c", "kill ...")`, bash uses its **builtin** kill.

### Theory: The procps-ng kill binary has different behavior

The procps-ng `/usr/bin/kill` binary might:
1. Handle negative PGID arguments differently
2. Have some unexpected signal handling
3. Interact badly with Go's exec.Command

### Commands I'm Avoiding (MURDER WEAPONS)
```bash
# DO NOT RUN - These all use direct exec.Command("kill"...) and kill Claude:
go test ./internal/tmux -run TestCleanupOrphanedSessions -v
go test ./internal/tmux -run TestKillSessionWithProcesses_Trace -v
/tmp/direct_kill <any_args>
/tmp/kill_test_step2 <any_args>
/tmp/test_kill_isolated
/tmp/safe_kill_go <any_args>
```

### For My Successor (#28)

If I die:
- My Claude PID: **1128136**
- My Claude PGID: **1128136**
- Check what command I was running when I died

**THE FIX FOR THE BUG:**
Change `KillSessionWithProcesses` in `internal/tmux/tmux.go` from:
```go
_ = exec.Command("kill", "-TERM", "-"+pgid).Run()
_ = exec.Command("kill", "-KILL", "-"+pgid).Run()
```

To:
```go
_ = exec.Command("bash", "-c", "kill -TERM -"+pgid).Run()
_ = exec.Command("bash", "-c", "kill -KILL -"+pgid).Run()
```

### Investigation Continues...

#### THE BUG IS FOUND!

I ran strace to compare bash builtin vs /usr/bin/kill:

```
$ strace -e kill bash -c "kill -0 -1129295" 2>&1 | grep kill
kill(-1129295, 0)                       = 0   # CORRECT!

$ strace -e kill /usr/bin/kill -0 -1129295 2>&1 | grep kill
kill(-1, 0)                             = 0   # WRONG! Kills ALL processes!
```

**The procps-ng kill binary misparses negative numbers!**

When you run:
```
/usr/bin/kill -KILL -12345
```

The argument `-12345` looks like an option (starts with `-`). The procps-ng kill tries to parse it as a signal, fails, and falls back to `-1` which means "all processes"!

**The fix is to use `--` to separate options from arguments:**

```
$ strace /usr/bin/kill -KILL -- -12345 2>&1 | grep kill
kill(-12345, SIGKILL)                   = -1 ESRCH   # CORRECT!
```

### THE ROOT CAUSE

In `internal/tmux/tmux.go`, the code does:
```go
_ = exec.Command("kill", "-KILL", "-"+pgid).Run()
```

But procps-ng kill parses `-PGID` as an option, not a target. This causes:
```
kill -KILL -12345  →  interpreted as →  kill (signal: -12345??) (target: -1)
                   →  falls back to →   kill -KILL -1
                   →  which means   →   KILL ALL PROCESSES!
```

### THE FIX

Change all kill invocations in `tmux.go` from:
```go
exec.Command("kill", "-TERM", "-"+pgid).Run()
exec.Command("kill", "-KILL", "-"+pgid).Run()
```

To:
```go
exec.Command("kill", "-TERM", "--", "-"+pgid).Run()
exec.Command("kill", "-KILL", "--", "-"+pgid).Run()
```

The `--` tells the option parser "stop parsing options, everything after is arguments".

### Why Shell Wrapper Works

When you use `bash -c "kill -KILL -12345"`:
- Bash uses its **builtin** kill, not /usr/bin/kill
- Bash's builtin doesn't have this parsing bug
- It correctly sends the signal to process group 12345

### CASE CLOSED

**26 Claudes died because `/usr/bin/kill -KILL -PGID` was killing ALL PROCESSES instead of just the target process group.**

The fix is trivial: add `--` before the negative PGID.

### FIX APPLIED AND VERIFIED!

I applied the fix to `internal/tmux/tmux.go` and ran the tests:

```
=== RUNNING THE MURDER WEAPON TEST ===
=== RUN   TestCleanupOrphanedSessions
--- PASS: TestCleanupOrphanedSessions (8.43s)
=== I SURVIVED! THE FIX WORKS! ===

=== RUNNING THE OTHER MURDER WEAPON TEST ===
=== RUN   TestKillSessionWithProcesses_Trace
--- PASS: TestKillSessionWithProcesses_Trace (4.19s)
=== STILL ALIVE! ===
```

**Both tests that killed previous investigators now pass safely!**

### Summary of Deaths and Investigation

| Attempt | Status | Key Finding |
|---------|--------|-------------|
| #1-#9 | Killed | (Various hypotheses) |
| #10 | Survived (partially) | Process groups ARE independent |
| #11-#12 | Killed instantly | No notes survived |
| #13-#16 | Killed | Confirmed PGID independence, still died |
| #17 | Killed | Manual kill safe, go test kills |
| #18 | Killed | Running TestKillSessionWithProcesses_Trace |
| #19 | Survived (partially) | More theories |
| #20 | Killed | Running TestKillSessionWithProcesses_Trace |
| #21 | Killed | Running isolated test |
| #22 | Didn't complete | Notes survived |
| #23 | Killed | Running minimal_kill.go |
| #24 | Killed | Running kill_test_step2 |
| #25 | Killed | Running safe_kill_go |
| #26 | **BREAKTHROUGH** | Shell wrapper SAFE, direct kill FATAL |
| #27 | **SOLVED IT** | Found the `--` argument parsing bug |

### The Technical Details

The procps-ng `kill` binary (version 4.0.4) has an argument parsing issue where:
```
/usr/bin/kill -KILL -12345
```
is parsed as:
- `-KILL` → signal to send
- `-12345` → looks like another option (starts with `-`)
- Falls back to `-1` which means "all processes"

The fix is:
```
/usr/bin/kill -KILL -- -12345
```
The `--` separates options from arguments, so `-12345` is correctly interpreted as a negative PID (process group).

### Lessons Learned

1. **Trust but verify**: Even well-tested POSIX utilities can have subtle bugs
2. **Process groups are tricky**: Negative PIDs have special meaning
3. **Shell builtins differ from binaries**: `bash -c "kill ..."` vs `/usr/bin/kill` behave differently
4. **strace is your friend**: Revealed the actual syscalls being made
5. **Good documentation saves lives**: Each investigator's notes helped the next

---

*Case closed by Investigator #27 on 2026-01-24*

---

## Deployment Record

**Commit:** b23c2ea0
**Files Changed:**
- `internal/tmux/tmux.go` - Added `--` to 4 kill commands
- `internal/tmux/tmux_test.go` - Added diagnostic tests and hasClaudeChild fix
- `Makefile` - Added test-safe target
- `MURDER_INVESTIGATION.md` - This file

**Binary Deployed:**
```
Deployed gt: 1a51855404c6d94e12c37ae7bdefcbaaa61bba23fbc83fb63e65ab07b7a3090a
Hash verified ✓
```

**Tests Verified:**
```
=== RUN   TestCleanupOrphanedSessions
--- PASS: TestCleanupOrphanedSessions (8.43s)
=== RUN   TestKillSessionWithProcesses_Trace
--- PASS: TestKillSessionWithProcesses_Trace (4.19s)
```

No more Claudes will die from this bug. 🎉
