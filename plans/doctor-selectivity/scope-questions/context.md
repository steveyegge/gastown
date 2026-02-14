# Codebase Context: doctor-selectivity

## App Structure

**Location**: `/home/krystian/gt/gastown/crew/doctor/`

### Key Directories:
- **`internal/cmd/`** - CLI command implementations (main entry points)
- **`internal/doctor/`** - Doctor framework and all health check implementations (47 check files)
- **`internal/tui/`** - Terminal UI components (convoy, feed)
- **`internal/ui/`** - Shared UI utilities (markdown, pager, styles)
- **`cmd/gt/`** - Binary entry point (`main.go`)

### Entry Point Flow:
1. `cmd/gt/main.go` → calls `cmd.Execute()`
2. `internal/cmd/root.go` → Sets up root Cobra command with persistent pre-run
3. `internal/cmd/doctor.go` → Defines the `doctor` command

### Doctor System:
- **Core Package**: `internal/doctor/`
  - `doctor.go` - Doctor orchestrator (manages check registration and execution)
  - `types.go` - Check interface and result types
  - Individual check files (one per check type, e.g., `orphan_check.go`, `wisp_check.go`)

## Existing UI Patterns

### Doctor Command Output Style:
- **Streaming output**: Checks print in real-time as they run (icon + name, then update with status)
- **Icons**: Pass (✓), Warning (⚠), Fail (✗), Fixed (🔧), Hourglass (⏳ for slow checks)
- **Categories**: Checks grouped by type (Core, Infrastructure, Rig, Patrol, Config, Cleanup, Hooks)
- **Semantic styling**: Uses `style` package for colors and formatting

### Report Structure:
1. Streaming output (each check as it runs)
2. Separator line
3. Summary line (X passed, Y warnings, Z failed, A fixed, B slow)
4. Section-based warnings/failures/fixed items (with fix hints)
5. Optional verbose details

### Command Structure:
- Cobra-based CLI
- Flags: `--fix`, `--verbose/-v`, `--rig`, `--restart-sessions`, `--slow`
- Global flags exempt "doctor" from beads checks and branch warnings

## Related Features

### Doctor Check Framework
**47 existing checks** organized by type:
- **Workspace checks**: town-config, rigs-registry, mayor-exists
- **Infrastructure**: stale-binary, daemon, boot-health
- **Cleanup** (fixable): orphan-sessions, orphan-processes, wisp-gc, stale-beads-redirect
- **Routing**: routes-config, prefix-mismatch, database-prefix
- **Hooks**: hook-attachment, hook-singleton, orphaned-attachments
- **Configuration**: claude-settings, deprecated-keys, session-hooks
- **Rig-specific**: Available with `--rig` flag

### Existing Fixable Checks Pattern
Examples of "specific-doctor-fix" candidates:
- `orphan-sessions` - Kills orphaned tmux sessions (except crew-protected ones)
- `wisp-gc` - Garbage collects wisps older than 1 hour
- `stale-beads-redirect` - Cleans up stale redirect files
- `town-root-branch` - Fixes town root off main branch

## Tech Stack

### Go-Based CLI:
- **Go 1.24.2** - Backend language
- **Cobra 1.10.2** - CLI framework
- **BubbleTea 1.3.10** - TUI framework (charmbracelet)
- **Lipgloss** - Terminal styling
- **TOML** - Configuration files

### UI Libraries:
- **charmbracelet**: bubbletea, bubbles, lipgloss (terminal UI)
- **rod** - Browser automation

### Key Dependencies:
- `github.com/steveyegge/gastown/internal/` - Internal Gas Town packages
- `github.com/gofrs/flock` - File locking
- `github.com/spf13/cobra` - CLI framework

### No Pure Web UI:
- TUI-based (terminal) for convoy dashboard
- No React/Vue frontend
- Desktop-focused (tmux integration)

## Check Interface & Implementation Pattern

### Check Interface (types.go):
```go
type Check interface {
    Name() string
    Description() string
    Run(ctx *CheckContext) *CheckResult
    Fix(ctx *CheckContext) error
    CanFix() bool
}
```

### Base Implementations:
- `BaseCheck` - For read-only checks (CanFix returns false)
- `FixableCheck` - For auto-fixable checks (CanFix returns true)

### CheckResult Structure:
```go
type CheckResult struct {
    Name     string
    Status   CheckStatus
    Message  string
    Details  []string
    FixHint  string
    Category string
    Elapsed  time.Duration
    Fixed    bool
}
```

### Status Constants:
- `StatusOK` - Check passed
- `StatusWarning` - Non-critical issue (usually fixable)
- `StatusError` - Critical issue

### Category Constants:
- `CategoryCore` - Fundamental checks
- `CategoryInfrastructure` - Infrastructure checks
- `CategoryCleanup` - Cleanup operations (most fixable checks)
- `CategoryHooks` - Hook-related checks
- `CategoryRig` - Rig-specific checks
- `CategoryPatrol` - Patrol system checks
- `CategoryConfig` - Configuration checks

## Project Conventions

### Naming:
- Check IDs: `kebab-case` (e.g., `orphan-sessions`, `stale-binary`)
- Struct names: `PascalCase` with "Check" suffix (e.g., `OrphanSessionCheck`)
- Methods: `Run(ctx)`, `Fix(ctx)` for checks

### File Organization:
- One check per file: `<check-name>_check.go`
- Tests alongside: `<check-name>_check_test.go`
- Doctor framework in `doctor.go`, types in `types.go`

### Check Registration:
```go
// In cmd/doctor.go
d.Register(doctor.NewOrphanSessionCheck())
d.Register(doctor.NewWispGCCheck())
```

### Design Principles:
- **Fail-safe defaults**: Auto-fixes are conservative (e.g., don't kill crew sessions)
- **Streaming output**: Real-time feedback as checks run
- **Categorization**: Checks grouped for readability
- **Fixable vs informational**: Only auto-fix safe operations
- **Event logging**: Use `events.LogFeed()` for important actions

### Anti-Patterns to Avoid:
- Don't auto-fix user-created resources without explicit consent
- Don't run heavy operations during check (use caching)
- Don't make assumptions about rig structure without validation

## Key Files to Reference

### Doctor Framework:
- `internal/doctor/doctor.go` - Orchestrator
- `internal/doctor/types.go` - Interfaces & types
- `internal/doctor/errors.go` - Error handling

### Example Checks (Fixable):
- `internal/doctor/orphan_check.go` - Orphan sessions/processes
- `internal/doctor/wisp_check.go` - Wisp garbage collection
- `internal/doctor/stale_beads_redirect_check.go` - Redirect cleanup

### CLI Integration:
- `internal/cmd/doctor.go` - Doctor command definition
- `internal/cmd/root.go` - Root command setup

### UI Styling:
- `internal/ui/styles.go` - UI constants
- `internal/style/` - Style package

## Design Considerations for doctor-selectivity

Based on the current architecture, adding `gt doctor <specific-doctor-fix>` would involve:

1. **CLI Enhancement**: Add subcommand or positional argument parsing to `doctor.go`
2. **Check Selection**: Filter which checks to run based on the provided name(s)
3. **Targeted Execution**: Only run checks matching the pattern
4. **Output Adaptation**: Potentially simplified output for single-check runs
5. **Error Handling**: Clear error messages for invalid check names with suggestions

### Example Flow:
```bash
gt doctor orphaned-sessions          # Run only orphaned-sessions check
gt doctor --fix orphaned-sessions    # Fix only orphaned-sessions
gt doctor cleanup                    # Run all "Cleanup" category checks
gt doctor list                       # Show available checks
```
