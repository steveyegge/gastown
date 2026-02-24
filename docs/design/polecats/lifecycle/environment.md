# Polecat Environment & Configuration

> Complete reference for environment variables, settings, and configuration hierarchy

## Environment Variables Injected

### Command-Line Injection (via PrependEnv)

These variables are prepended to the startup command as `export VAR=value && ...`:

| Variable | Value | Purpose |
|----------|-------|---------|
| `GT_RIG` | `<rig-name>` | Identifies the rig (e.g., `gastown`) |
| `GT_POLECAT` | `<polecat-name>` | Identifies the polecat (e.g., `Toast`) |
| `GT_ROLE` | `<rig>/polecats/<name>` | Full role path (e.g., `gastown/polecats/Toast`) |
| `GT_POLECAT_PATH` | `/path/to/worktree` | Polecat's git worktree path |
| `GT_TOWN_ROOT` | `/path/to/town` | Town workspace root |
| `GT_BRANCH` | `feature-branch` | Current git branch |
| `BD_DOLT_AUTO_COMMIT` | `off` | Disable auto-commit for batch mode |
| `BD_ACTOR` | `<rig>/polecats/<name>` | Beads actor for attribution |
| `GIT_AUTHOR_NAME` | `<polecat-name>` | Git commit author name |

### tmux Session Environment (via SetEnvironment)

Set after session creation via `tmux set-environment`. Non-fatal if fails:

| Variable | Purpose |
|----------|---------|
| `GT_AGENT` | Agent type (e.g., `claude`) |
| `GT_PROCESS_NAMES` | Process names to monitor (e.g., `node,claude`) |
| `GT_BRANCH` | Git branch (redundant but for respawned processes) |
| `GT_POLECAT_PATH` | Worktree path (redundant) |
| `GT_TOWN_ROOT` | Town root (redundant) |
| `BD_DOLT_AUTO_COMMIT` | `off` (redundant) |

### Source Locations

| Variables | File | Function |
|-----------|------|----------|
| Core GT_* vars | `internal/config/env.go:65` | `AgentEnv()` |
| Additional injection | `internal/polecat/session_manager.go:307-317` | `Start()` |
| PrependEnv transform | `internal/config/loader.go:1958` | `PrependEnv()` |
| tmux SetEnvironment | `internal/polecat/session_manager.go:325-361` | `Start()` |

---

## Configuration Hierarchy

### Priority Order (First Match Wins)

```
1. Town-level settings (highest)
   └── <townRoot>/settings/config.json

2. Wisp layer (ephemeral, local)
   └── <townRoot>/.beads-wisp/config/<rigName>/settings.json

3. Rig identity bead labels
   └── gt-<rig> bead with max_polecats:N label

4. System defaults (lowest)
   └── Compiled-in values
```

### Town Settings

**Path:** `<townRoot>/settings/config.json`

**Structure:**
```json
{
  "type": "town-settings",
  "version": 1,
  "scheduler": {
    "max_polecats": 5,
    "batch_size": 2,
    "spawn_delay": "100ms"
  }
}
```

**Loading:** `internal/config/loader.go:964-982`
```go
func LoadOrCreateTownSettings(path string) (*TownSettings, error)
func SaveTownSettings(path string, settings *TownSettings) error
```

### Scheduler Configuration

**File:** `internal/scheduler/capacity/config.go`

```go
type SchedulerConfig struct {
    MaxPolecats *int   `json:"max_polecats,omitempty"`
    BatchSize   *int   `json:"batch_size,omitempty"`
    SpawnDelay  string `json:"spawn_delay,omitempty"`
}
```

**Defaults:**
```go
func DefaultSchedulerConfig() *SchedulerConfig {
    return &SchedulerConfig{
        MaxPolecats: intPtr(-1),   // Direct dispatch
        BatchSize:   intPtr(1),    // 1 bead per heartbeat
        SpawnDelay:  "0s",         // No delay
    }
}
```

**Getter with Default:**
```go
func (c *SchedulerConfig) GetMaxPolecats() int {
    if c == nil || c.MaxPolecats == nil {
        return -1  // Default: direct dispatch
    }
    return *c.MaxPolecats
}
```

### Rig-Level System Defaults

**File:** `internal/rig/config.go:31-40`

```go
var SystemDefaults = map[string]interface{}{
    "status":                  "operational",
    "auto_restart":            true,
    "max_polecats":            10,
    "priority_adjustment":     0,
    "dnd":                     false,
    "polecat_branch_template": "",
}
```

### Wisp Layer (Ephemeral)

**Path:** `<townRoot>/.beads-wisp/config/<rigName>/settings.json`

**Purpose:**
- Local/ephemeral overrides
- Disappears on wisp cleanup
- Used for parked rig status

**Example Usage:**
```go
wispCfg := wisp.NewConfig(townRoot, rigName)
status := wispCfg.GetString("status")  // "operational" or "parked"
```

---

## max_polecats Semantics

| Value | Mode | Behavior |
|-------|------|----------|
| `-1` | Direct | Spawn immediately, no capacity control (default) |
| `0` | Direct | Same as -1 (backward compatible) |
| `N > 0` | Deferred | Capacity control, max N concurrent polecats |

### Direct Dispatch (max_polecats <= 0)

```
gt sling <bead> <rig>
    │
    └─► executeSling() → SpawnPolecatForSling() → Session starts immediately
```

### Deferred Dispatch (max_polecats > 0)

```
gt sling <bead> <rig>
    │
    └─► createSlingContextBead() → Bead queued in HQ
                                        │
                                        ▼
                           [Scheduler heartbeat - 3 min]
                                        │
                                        ▼
                           dispatchScheduledWork()
                                        │
                           ┌────────────┴────────────┐
                           │  Check available slots  │
                           │  = max_polecats - active│
                           └────────────┬────────────┘
                                        │
                           ┌────────────┴────────────┐
                           │  slots > 0?             │
                           └─────┬───────────┬───────┘
                                 │           │
                              [yes]        [no]
                                 │           │
                                 ▼           ▼
                           executeSling()  Wait for
                                          next cycle
```

---

## Scheduler State

**Path:** `<townRoot>/.runtime/scheduler-state.json`

**Structure:**
```go
type SchedulerState struct {
    Paused            bool   `json:"paused"`
    PausedBy          string `json:"paused_by,omitempty"`
    PausedAt          string `json:"paused_at,omitempty"`
    LastDispatchAt    string `json:"last_dispatch_at,omitempty"`
    LastDispatchCount int    `json:"last_dispatch_count,omitempty"`
}
```

**Atomic Save:**
```go
func SaveState(townRoot string, state *SchedulerState) error {
    // Write to temp file + rename for atomicity
    tmp, _ := os.CreateTemp(dir, ".scheduler-state-*.tmp")
    tmp.Write(data)
    tmp.Close()
    os.Rename(tmpPath, path)  // Atomic
}
```

---

## Beacon Configuration

The beacon is the initial prompt injected into the polecat session:

**File:** `internal/session/startup.go`

**Config Structure:**
```go
type BeaconConfig struct {
    Recipient               string  // e.g., "polecat/Toast/gastown"
    Sender                  string  // e.g., "witness"
    Topic                   string  // e.g., "assigned"
    MolID                   string  // Bead ID being worked
    IncludePrimeInstruction bool    // Include "Run gt prime --hook"
    ExcludeWorkInstructions bool    // Defer details via nudge
}
```

**Example Beacon Output:**
```
To: polecat/Toast/gastown
From: witness
Topic: assigned
Bead: gt-abc

Run `gt prime --hook` and begin work on gt-abc.

## Instructions

Fix the authentication bug in login.go. The issue is...
```

---

## Session Naming

### Polecat Sessions

**Format:** `gt-<rig-name>-<polecat-name>`

**Examples:**
- `gt-gastown-Toast`
- `gt-bcc-p-001`
- `gt-greenplace-Nitro`

**Implementation:**
```go
session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
```

### Witness Sessions

**Format:** `gt-<rig-name>-witness`

**Examples:**
- `gt-gastown-witness`
- `gt-bcc-witness`

**Implementation:**
```go
session.WitnessSessionName(session.PrefixFor(rigName))
```

### Refinery Sessions

**Format:** `gt-<rig-name>-refinery`

---

## Polecat Worktree Paths

**Pattern:** `<rigRoot>/polecats/<polecatName>/`

**Example:**
```
/home/user/gt/gastown/polecats/Toast/
```

**Contains:**
- Full git worktree of the repo
- `.beads/` symlink to rig's beads
- Runtime settings in `.claude/`

---

## Constants Reference

| Constant | Value | Location |
|----------|-------|----------|
| `maxDispatchFailures` | 3 | `capacity_dispatch.go:22` |
| `HungSessionThresholdMinutes` | 30 | `handlers.go:28` |
| `doneIntentTimeout` | 60s | `handlers.go` (implicit) |
| `doltMaxRetries` | 10 | `manager.go:62` |
| `doltBaseBackoff` | 500ms | `manager.go:63` |
| `doltMaxBackoff` | 30s | `manager.go:64` |
| `schedulerHeartbeat` | 3min | daemon config |

---

## Debugging Environment Issues

### Check Active Environment

From within a polecat session:
```bash
env | grep -E '^(GT_|BD_|GIT_)'
```

### Check tmux Session Environment

```bash
tmux show-environment -t gt-gastown-Toast
```

### Verify Session Name

```bash
tmux list-sessions -F "#{session_name}"
```

### Check Scheduler State

```bash
cat ~/gt/.runtime/scheduler-state.json
```

### Check Town Settings

```bash
cat ~/gt/settings/config.json
```

### Check Rig Wisp Layer

```bash
cat ~/gt/.beads-wisp/config/gastown/settings.json
```
