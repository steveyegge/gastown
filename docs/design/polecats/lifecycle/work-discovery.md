# Polecat Work Discovery

> How polecats discover and begin work on their assigned beads

This document covers the runtime flow from session start to work execution,
focusing on how the polecat learns what to work on.

---

## Overview: Work Discovery Flow

```
Session Created (tmux)
    │
    ├─► Beacon prompt injected via CLI argument
    │       └─► "Run gt prime --hook"
    │
    ▼
gt prime --hook
    │
    ├─► Read session ID from stdin (JSON)
    ├─► Persist session ID to .runtime/
    ├─► Check for hooked work
    │       │
    │       ├─► PRIMARY: agent_bead.hook_bead field
    │       │       └─► Authoritative source (set at spawn)
    │       │
    │       └─► FALLBACK: query by assignee status
    │               └─► beads with status=hooked/in_progress
    │
    └─► Output autonomous work directive
            └─► "Run bd show <bead-id> and begin work"
```

---

## 1. Session Start: Beacon Injection

### Beacon Delivery Method

**File:** `internal/polecat/session_manager.go:268-284`

The beacon (initial prompt) is injected as a **CLI argument** to avoid send-keys races:

```go
// Line 271: Build command with beacon
command := config.BuildStartupCommandFromConfig(
    runtimeConfig,
    beacon,  // ← Beacon content as CLI argument
)

// Line 321: Create session with command
m.tmux.NewSessionWithCommand(sessionID, workDir, command)
```

**Result:** Claude Code receives beacon content as its initial prompt, not via send-keys.

**Why CLI argument?** GitHub issue #280 - send-keys race condition causes lost characters.

### Beacon Content Structure

**File:** `internal/session/startup.go`

```go
type BeaconConfig struct {
    Recipient               string  // "polecat/Toast/gastown"
    Sender                  string  // "witness"
    Topic                   string  // "assigned"
    MolID                   string  // Bead ID
    IncludePrimeInstruction bool    // Include "Run gt prime --hook"
    ExcludeWorkInstructions bool    // Defer details via nudge
}
```

**Example Beacon:**
```
To: polecat/Toast/gastown
From: witness
Topic: assigned
Bead: gt-abc

Run `gt prime --hook` and begin work.
```

---

## 2. gt prime --hook: Hook Mode

### Session ID Reading

**File:** `internal/cmd/prime_session.go:29-97`

**Priority Order:**
1. **stdin JSON** (Claude Code format) - Lines 33-36
2. **GT_SESSION_ID env** - Lines 40-41
3. **CLAUDE_SESSION_ID env** (legacy) - Lines 43-44
4. **Auto-generate UUID** - Line 48

### stdin JSON Format

**File:** `internal/cmd/prime_session.go:53-84`

```go
type hookInput struct {
    SessionID      string `json:"session_id"`
    TranscriptPath string `json:"transcript_path"`
    Source         string `json:"source"`  // "startup", "resume", "clear", "compact"
}
```

**Reading Logic:**
```go
func readStdinJSON() *hookInput {
    stat, _ := os.Stdin.Stat()
    if (stat.Mode() & os.ModeCharDevice) != 0 {
        return nil  // stdin is terminal, not pipe
    }

    reader := bufio.NewReader(os.Stdin)
    line, _ := reader.ReadString('\n')  // ONE line JSON

    var input hookInput
    json.Unmarshal([]byte(line), &input)
    return &input
}
```

### Session ID Persistence

**File:** `internal/cmd/prime_session.go:88-97`

```go
func persistSessionID(dir, sessionID string) {
    runtimeDir := filepath.Join(dir, ".runtime")
    os.MkdirAll(runtimeDir, 0755)

    sessionFile := filepath.Join(runtimeDir, "session_id")
    content := fmt.Sprintf("%s\n%s\n", sessionID, time.Now().Format(time.RFC3339))
    os.WriteFile(sessionFile, []byte(content), 0644)
}
```

**Writes to:** `.runtime/session_id`
**Format:** `<session-id>\n<timestamp>\n`

### Hook Mode Handling

**File:** `internal/cmd/prime.go:245-266`

```go
func handlePrimeHookMode(townRoot, cwd string) {
    sessionID, source := readHookSessionID()

    if !primeDryRun {
        persistSessionID(townRoot, sessionID)
        if cwd != townRoot {
            persistSessionID(cwd, sessionID)  // Also write to cwd
        }
    }

    os.Setenv("GT_SESSION_ID", sessionID)
    os.Setenv("CLAUDE_SESSION_ID", sessionID)  // Legacy compat

    primeHookSource = source  // "startup", "resume", "clear", "compact"

    fmt.Printf("[session:%s]\n", sessionID)
    if source != "" {
        fmt.Printf("[source:%s]\n", source)
    }
}
```

---

## 3. Work Discovery: Finding the Hooked Bead

**File:** `internal/cmd/prime.go:391-465`

### Primary Strategy: agent_bead.hook_bead Field

```go
func findAgentWork(ctx *PrimeContext) (*WorkInfo, error) {
    // Line 429: Build agent bead ID
    agentBeadID := buildAgentBeadID(ctx.AgentID, ctx.Role, ctx.TownRoot)

    // Line 431: Fetch agent bead
    agentBead, err := beads.Show(agentBeadID)

    // Line 437: Read hook_bead field (AUTHORITATIVE)
    if agentBead.HookBead != "" && agentBead.HookBead != "none" {
        hookBead := beads.Show(agentBead.HookBead)
        if hookBead.Status == "hooked" || hookBead.Status == "in_progress" {
            return &WorkInfo{
                BeadID:   agentBead.HookBead,
                Source:   "agent_bead",
            }, nil
        }
    }
    // Fall through to fallback...
}
```

**Why hook_bead is Authoritative:**
- Set **atomically at spawn time** (not post-session)
- Written to Dolt before session starts
- Survives session restart/crash

### Fallback Strategy: Query by Assignee

**File:** `internal/cmd/prime.go:441-464`

```go
func findAgentWork(ctx *PrimeContext) (*WorkInfo, error) {
    // ... primary strategy above ...

    // Line 452: Fallback - query beads assigned to this agent
    hookedBeads, err := beads.List("--status=hooked", "--assignee="+ctx.AgentID)
    if len(hookedBeads) > 0 {
        return &WorkInfo{
            BeadID: hookedBeads[0].ID,
            Source: "assignee_query",
        }, nil
    }

    // Line 458: Further fallback - in_progress beads
    inProgressBeads, err := beads.List("--status=in_progress", "--assignee="+ctx.AgentID)
    if len(inProgressBeads) > 0 {
        return &WorkInfo{
            BeadID: inProgressBeads[0].ID,
            Source: "assignee_query_in_progress",
        }, nil
    }

    return nil, nil  // No work found
}
```

---

## 4. Autonomous Work Directive

**File:** `internal/cmd/prime.go:468-503`

When work is found, gt prime outputs instructions:

```go
func outputAutonomousDirective(work *WorkInfo) {
    fmt.Println("## AUTONOMOUS WORK MODE")
    fmt.Println("")

    // Line 483: Announce role (ONE line)
    fmt.Printf("Announce: \"%s checking in.\"\n", roleName)

    // Line 488-492: Molecule vs naked bead instructions
    if work.IsMolecule {
        fmt.Printf("Run `bd mol current %s` to see current step.\n", work.BeadID)
        fmt.Printf("Close steps with `bd close <step-id>` as you complete them.\n")
    } else {
        fmt.Printf("Run `bd show %s` immediately to see work instructions.\n", work.BeadID)
    }

    fmt.Println("")
    fmt.Println("Begin work now. Do not wait for confirmation.")
}
```

---

## 5. WaitForRuntimeReady: Session Readiness

**File:** `internal/tmux/tmux.go:1691-1766`

### Two Detection Modes

| Mode | Condition | Behavior |
|------|-----------|----------|
| **Prompt-Based** | `ReadyPromptPrefix != ""` | Poll for prompt character |
| **Delay-Based** | `ReadyPromptPrefix == ""` | Sleep `ReadyDelayMs` |

### Prompt-Based Detection

```go
func (t *Tmux) WaitForRuntimeReady(sessionID string, rc *RuntimeConfig, timeout time.Duration) error {
    // Line 1734: Check if prompt detection enabled
    if rc.Tmux.ReadyPromptPrefix == "" {
        // Delay-based fallback
        time.Sleep(time.Duration(rc.Tmux.ReadyDelayMs) * time.Millisecond)
        return nil
    }

    // Line 1757: Poll for prompt
    for {
        output := t.CapturePane(sessionID)
        lastLine := getLastNonEmptyLine(output)

        if matchesPromptPrefix(lastLine, rc.Tmux.ReadyPromptPrefix) {
            return nil  // Ready!
        }

        if time.Now().After(deadline) {
            return fmt.Errorf("timeout waiting for prompt")
        }

        time.Sleep(100 * time.Millisecond)
    }
}
```

**Default Prompt Prefix:** `"❯ "` (Claude Code prompt)

### Agent-Specific Configuration

| Agent | ReadyPromptPrefix | ReadyDelayMs |
|-------|-------------------|--------------|
| Claude Code | `"❯ "` | 0 |
| Copilot | `"❯ "` | 0 |
| OpenCode | `""` (disabled) | 8000 |
| Gemini | `""` (disabled) | 5000 |

---

## 6. Fallback Nudge Logic

**File:** `internal/runtime/runtime.go:160-221`

### Fallback Matrix

| Hooks | Prompt Support | Beacon Delivery | Work Instructions |
|-------|----------------|-----------------|-------------------|
| ✓ | ✓ | CLI argument | In beacon |
| ✓ | ✗ | CLI + nudge | Same nudge |
| ✗ | ✓ | CLI "Run gt prime" | Delayed nudge |
| ✗ | ✗ | CLI + nudge | Delayed nudge |

### Implementation

**File:** `internal/polecat/session_manager.go:395-421`

```go
func (m *SessionManager) Start(polecat string, opts SessionStartOptions) error {
    // ... session creation ...

    // Line 395: Get fallback info
    fallbackInfo := runtime.GetStartupFallbackInfo(runtimeConfig)

    // Line 397-401: Combined nudge (hooks + no prompt)
    if fallbackInfo.SendBeaconNudge && fallbackInfo.SendStartupNudge &&
       fallbackInfo.StartupNudgeDelayMs == 0 {
        combined := beacon + "\n\n" + runtime.StartupNudgeContent()
        m.tmux.NudgeSession(sessionID, combined)
        return nil
    }

    // Line 403-407: Conditional beacon nudge
    if fallbackInfo.SendBeaconNudge {
        m.tmux.NudgeSession(sessionID, beacon)
    }

    // Line 409-415: Delayed work instructions
    if fallbackInfo.StartupNudgeDelayMs > 0 {
        primeWaitRC := runtime.RuntimeConfigWithMinDelay(
            runtimeConfig, fallbackInfo.StartupNudgeDelayMs)
        m.tmux.WaitForRuntimeReady(sessionID, primeWaitRC, timeout)
    }

    // Line 417-419: Send startup nudge
    if fallbackInfo.SendStartupNudge {
        m.tmux.NudgeSession(sessionID, runtime.StartupNudgeContent())
    }
}
```

---

## 7. Hook Attachment: Timing Details

**File:** `internal/polecat/session_manager.go:368-374`

### When Hook is Set

**Atomic at Spawn Time** (preferred):

```go
// internal/polecat/manager.go:773-777 (AddWithOptions)
agentID := m.agentBeadID(name)
err = m.createAgentBeadWithRetry(agentID, &beads.AgentFields{
    RoleType:   "polecat",
    Rig:        m.rig.Name,
    AgentState: "spawning",
    HookBead:   opts.HookBead,  // ← ATOMIC at spawn time
})
```

**Post-Session Fallback** (if HookBead not provided in opts):

```go
// internal/polecat/session_manager.go:369-374
if opts.Issue != "" {
    agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
    if err := m.hookIssue(opts.Issue, agentID, workDir); err != nil {
        style.PrintWarning("could not hook issue %s: %v", opts.Issue, err)
        // Non-fatal - continues without hook
    }
}
```

### hookIssue Implementation

**File:** `internal/polecat/session_manager.go:717-730`

```go
func (m *SessionManager) hookIssue(issueID, agentID, workDir string) error {
    bdWorkDir := m.resolveBeadsDir(issueID, workDir)

    ctx, cancel := context.WithTimeout(context.Background(), constants.BdCommandTimeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "bd", "update", issueID,
                               "--status=hooked", "--assignee="+agentID)
    cmd.Dir = bdWorkDir
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("bd update failed: %w", err)
    }

    fmt.Printf("✓ Hooked issue %s to %s\n", issueID, agentID)
    return nil
}
```

### Failure Handling

- Hook attachment failure is **non-fatal** (warns but continues)
- Session starts even if hook fails
- Polecat may sit idle if hook fails (no work discovered)

---

## 8. Compaction/Resume Detection

**File:** `internal/cmd/prime.go:259-273`

### Source Field Values

| Source | Meaning |
|--------|---------|
| `"startup"` | Fresh session start |
| `"resume"` | Resumed from pause |
| `"clear"` | Context cleared |
| `"compact"` | Context compacted (long conversation) |

### Post-Compaction Handling

```go
// Line 271-273
if primeHookSource == "compact" || primeHookSource == "clear" {
    // May need to re-inject context
    // Hook work should still be on hook_bead field
}
```

**Key Insight:** Hook work persists across compaction because it's stored in
Dolt (agent_bead.hook_bead), not in conversation context.

---

## 9. Summary: Work Discovery Chain

```
1. Session created with beacon CLI argument
   └─► Beacon says "Run gt prime --hook"

2. gt prime --hook runs (via SessionStart hook)
   ├─► Read session ID from stdin JSON
   ├─► Persist to .runtime/session_id
   └─► Set GT_SESSION_ID env var

3. checkSlungWork() called
   └─► findAgentWork() discovers hooked bead

4. findAgentWork() strategy:
   ├─► PRIMARY: agent_bead.hook_bead field
   │       └─► Set atomically at spawn (authoritative)
   │
   └─► FALLBACK: query by assignee
           └─► beads with status=hooked assigned to this agent

5. outputAutonomousDirective() prints instructions
   └─► "Run bd show <bead-id> and begin work"

6. Polecat executes work immediately
   └─► No confirmation needed (GUPP)
```
