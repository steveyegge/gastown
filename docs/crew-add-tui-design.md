# Design Doc: TUI Integration for `gt crew add`

## Task: gt-y84

This document summarizes findings from investigating the `gt crew add` implementation and proposes an approach for TUI integration.

## Current Implementation Summary

### 1. Parameters Required for Crew Creation

**Required:**
- `name` - The crew worker name (validated: no path separators, hyphens, dots, or spaces)

**Optional:**
- `--rig` - Target rig (inferred from cwd if not specified)
- `--branch` - Create a feature branch `crew/<name>` instead of using main

**Validation rules (`validateCrewName`):**
- Cannot be empty
- Cannot be `.` or `..`
- No path separators (`/` or `\`)
- No `..` sequences
- No hyphens, dots, or spaces (these break agent ID parsing)
- If invalid chars detected, suggests sanitized alternative (e.g., `my-name` → `my_name`)

### 2. How Workspaces Are Set Up

The `Manager.Add()` function in `internal/crew/manager.go:115` performs these steps:

1. **Validate name** - Check for path traversal and invalid characters
2. **Create crew base directory** - `<rig>/crew/` if it doesn't exist
3. **Clone the rig repo** - Full git clone (with `--reference` optimization if LocalRepo is set)
4. **Optionally create branch** - `crew/<name>` branch if `--branch` flag passed
5. **Create mail directory** - `<rig>/crew/<name>/mail/`
6. **Set up shared beads redirect** - Points to rig's `.beads/` database
7. **Set up .claude symlink** - Links to `../..claude` for shared hook config
8. **Provision PRIME.md** - Gas Town context for the worker
9. **Copy overlay files** - From `.runtime/overlay/` for service configs
10. **Ensure .gitignore patterns** - Required Gas Town patterns
11. **Save state.json** - Crew worker metadata

The `runCrewAdd()` function in `internal/cmd/crew_add.go` additionally:
- Creates an **agent bead** for the crew worker with `bd.CreateAgentBead()`
- Supports multiple names in one call (batch creation)

### 3. How Sessions Are Started

The `Manager.Start()` function in `internal/crew/manager.go:496` handles session startup:

1. **Validate name**
2. **Get or create worker** - Calls `Add()` if worker doesn't exist
3. **Check existing session** - Kill zombie sessions or return error if running
4. **Ensure Claude settings** - `claude.EnsureSettingsForRole()` in `<rig>/crew/`
5. **Build startup beacon** - For predecessor discovery via `/resume`
6. **Build startup command** - Via `config.BuildCrewStartupCommandWithAgentOverride()`
7. **Create tmux session** - `t.NewSessionWithCommand()`
8. **Set environment variables** - `GT_ROLE`, `GT_RIG`, `GT_CREW`, `BD_ACTOR`, etc.
9. **Apply theming** - Rig-based tmux session theming
10. **Set keybindings** - C-b n/p for crew session cycling

**StartOptions struct:**
```go
type StartOptions struct {
    Account         string  // Override account handle
    ClaudeConfigDir string  // CLAUDE_CONFIG_DIR override
    KillExisting    bool    // Kill existing session before starting
    Topic           string  // Startup nudge topic (default: "start")
    Interactive     bool    // Remove --dangerously-skip-permissions
    AgentOverride   string  // Alternate agent alias
}
```

### 4. What Hooks/Configs Are Copied

**Shared (via symlink):**
- `.claude/` → `../..claude` - Hook configuration shared by all crew members

**Provisioned:**
- `PRIME.md` - Gas Town context (fallback if hook fails)

**Copied:**
- `.runtime/overlay/*` - Service-specific configs (e.g., `.env`)

**Ensured:**
- `.gitignore` patterns for Gas Town files

**Not copied (inherited via traversal):**
- Slash commands (`.claude/commands/`) - provisioned at town level by `gt install`

### 5. Agent Bead Creation

Created in `runCrewAdd()` with:
```go
fields := &beads.AgentFields{
    RoleType:   "crew",
    Rig:        rigName,
    AgentState: "idle",
}
desc := "Crew worker %s in %s - human-managed persistent workspace."
bd.CreateAgentBead(crewID, desc, fields)
```

## Proposed TUI Integration

### Approach: Bubbletea Wizard

A multi-step wizard using bubbletea that collects parameters progressively:

```
┌─────────────────────────────────────────┐
│        Create Crew Member               │
├─────────────────────────────────────────┤
│                                         │
│  Name: [                    ]           │
│                                         │
│  Rig:  ○ gastown (current)              │
│        ○ beads                          │
│        ○ ...                            │
│                                         │
│  □ Create feature branch (crew/<name>)  │
│                                         │
│  [Create]  [Cancel]                     │
│                                         │
└─────────────────────────────────────────┘
```

### Model Structure

```go
type CrewAddModel struct {
    // Input fields
    nameInput    textinput.Model
    selectedRig  int
    rigs         []string
    createBranch bool

    // State
    step         int  // 0=name, 1=rig, 2=options, 3=confirm
    validating   bool
    creating     bool
    err          error
    result       *crew.CrewWorker

    // UI dimensions
    width, height int
}
```

### Key Implementation Points

1. **Name Validation** - Use existing `validateCrewName()` logic, show real-time feedback:
   ```
   Name: [my-test]
   ⚠ Hyphens not allowed. Suggested: my_test
   ```

2. **Rig Selection** - Load available rigs from `mayor/rigs.json`, show current rig as default

3. **Progress Feedback** - The workspace creation involves multiple steps (clone, symlinks, etc.):
   ```
   Creating crew workspace...
   ✓ Cloning repository
   ✓ Setting up mail directory
   ✓ Configuring shared beads
   ✓ Creating agent bead

   ✓ Crew member 'alice' created successfully!

   Path: /home/ubuntu/gt/gastown/crew/alice

   [Start Session]  [Close]
   ```

4. **Error Handling** - Handle common errors gracefully:
   - `ErrCrewExists` - Offer to start existing session instead
   - Clone failures - Show git error with retry option
   - Network errors - Offer to retry

5. **Optional: Start Session** - After creation, offer to start the session (calls `Manager.Start()`)

### Integration with Main TUI

The crew add wizard could be accessible from:
- A top-level "Crew" menu in a dashboard
- A `/crew add` command-style interaction
- A keybinding (e.g., `c` for create crew)

### Code Reuse

The TUI should call the existing `crew.Manager` methods directly rather than duplicating logic:

```go
// Create workspace
crewMgr := crew.NewManager(rig, git)
worker, err := crewMgr.Add(name, createBranch)

// Create agent bead
bd := beads.New(beadsDir)
bd.CreateAgentBead(crewID, desc, fields)

// Optionally start session
err = crewMgr.Start(name, crew.StartOptions{})
```

### Files to Create/Modify

New files:
- `internal/tui/crew/add.go` - Main wizard model
- `internal/tui/crew/styles.go` - Shared styles for crew TUI components

Modifications:
- `internal/tui/dashboard.go` (or similar) - Add crew menu item

## Dependencies

The TUI will need access to:
- `internal/crew.Manager` - For workspace creation/session management
- `internal/beads.Beads` - For agent bead creation
- `internal/config.LoadRigsConfig` - For rig list
- `internal/rig.Manager` - For rig lookup

## Open Questions

1. **Batch creation** - Should the TUI support creating multiple crew members at once (like CLI does)?
   - Recommendation: Start with single creation, add batch later if needed

2. **Session startup** - Should the TUI offer to attach to the created session?
   - Recommendation: Offer button, but don't auto-attach (keeps TUI context)

3. **Advanced options** - Should we expose `StartOptions` like `AgentOverride`?
   - Recommendation: Hide in "Advanced" collapsible section

4. **Error recovery** - What to do if creation partially fails (e.g., clone succeeds but symlink fails)?
   - The CLI continues on non-fatal errors; TUI should show warnings but allow proceeding

## Summary

The `gt crew add` command is well-factored with clear separation between:
- Command parsing (`cmd/crew_add.go`)
- Workspace management (`crew/manager.go`)
- Bead operations (`beads/`)

A TUI integration can reuse the `crew.Manager` directly. The wizard pattern fits well since crew creation involves:
1. A single required input (name)
2. A few optional choices (rig, branch)
3. A multi-step creation process with progress feedback

The main complexity is providing good UX for the async creation steps and handling partial failures gracefully.
