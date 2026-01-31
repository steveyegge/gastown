# TMux Session Namespace Design for Multi-Town

**Task:** hq-arcu0q.8
**Epic:** hq-arcu0q (Colonization)
**Date:** 2026-01-27
**Status:** Design Complete

## Problem Statement

Currently, Gas Town tmux session names assume ONE mayor and ONE deacon per machine:

```
hq-mayor     # Town-level: one per machine
hq-deacon    # Town-level: one per machine
gt-boot      # Boot watchdog
```

With gt12 colonization on the same machine as gt11, these names will collide.

## Current Naming Scheme

### Town-Level (HQ Prefix)

| Agent | Session Name | Notes |
|-------|--------------|-------|
| Mayor | `hq-mayor` | One per machine |
| Deacon | `hq-deacon` | One per machine |
| Boot | `gt-boot` | Uses `gt-` to avoid prefix collision |

### Rig-Level (GT Prefix)

| Agent | Session Name Format | Example |
|-------|---------------------|---------|
| Witness | `gt-{rig}-witness` | `gt-gastown-witness` |
| Refinery | `gt-{rig}-refinery` | `gt-gastown-refinery` |
| Crew | `gt-{rig}-crew-{name}` | `gt-gastown-crew-slack_decisions` |
| Polecat | `gt-{rig}-{name}` | `gt-gastown-alpha` |

**Rig-level sessions already support multi-town** because they include the rig name.
Only HQ-level sessions (mayor, deacon, boot) have collision issues.

## Proposed Solution: Town-Namespaced HQ Sessions

### New Naming Scheme

Include the town name (basename of town root) in HQ session names:

| Agent | Old Format | New Format | Example (gt11) | Example (gt12) |
|-------|------------|------------|----------------|----------------|
| Mayor | `hq-mayor` | `hq-{town}-mayor` | `hq-gt11-mayor` | `hq-gt12-mayor` |
| Deacon | `hq-deacon` | `hq-{town}-deacon` | `hq-gt11-deacon` | `hq-gt12-deacon` |
| Boot | `gt-boot` | `hq-{town}-boot` | `hq-gt11-boot` | `hq-gt12-boot` |

**Boot Changes**: Move boot from `gt-` to `hq-` prefix to be consistent with other
town-level services. The original reason for `gt-` was tmux prefix matching collision
with `hq-deacon-boot` matching `hq-deacon`. With town name inserted, `hq-gt11-boot`
won't match `hq-gt11-deacon`.

### Why This Approach

1. **Consistent hierarchy**: Town → Rig → Agent mirrors session name structure
2. **Backwards compatible parsing**: `hq-` prefix still identifies town-level
3. **Clear ownership**: Session name encodes which town owns it
4. **Minimal disruption**: Rig-level sessions unchanged

### Alternative Considered: Containers/VMs

The current code comments suggest:
> "One mayor per machine - multi-town requires containers/VMs for isolation."

This is overkill for colonization. Session namespacing is simpler and sufficient.

## Implementation Changes

### 1. Update `internal/session/names.go`

```go
// Before
func MayorSessionName() string {
    return HQPrefix + "mayor"
}

// After
func MayorSessionName(town string) string {
    return fmt.Sprintf("%s%s-mayor", HQPrefix, town)
}

func DeaconSessionName(town string) string {
    return fmt.Sprintf("%s%s-deacon", HQPrefix, town)
}

func BootSessionName(town string) string {
    return fmt.Sprintf("%s%s-boot", HQPrefix, town)
}

// For backwards compatibility during migration
func LegacyMayorSessionName() string {
    return HQPrefix + "mayor"
}
```

### 2. Update `internal/session/identity.go`

Add `Town` field to `AgentIdentity`:

```go
type AgentIdentity struct {
    Role Role   // mayor, deacon, witness, refinery, crew, polecat
    Town string // town name (for mayor/deacon/boot)
    Rig  string // rig name (empty for mayor/deacon)
    Name string // crew/polecat name
}
```

Update `ParseSessionName` to handle new format:

```go
// Handles both legacy "hq-mayor" and new "hq-gt11-mayor"
if strings.HasPrefix(session, HQPrefix) {
    suffix := strings.TrimPrefix(session, HQPrefix)
    parts := strings.Split(suffix, "-")

    // Legacy format: hq-mayor, hq-deacon
    if len(parts) == 1 {
        switch parts[0] {
        case "mayor":
            return &AgentIdentity{Role: RoleMayor}, nil
        case "deacon":
            return &AgentIdentity{Role: RoleDeacon}, nil
        }
    }

    // New format: hq-{town}-mayor, hq-{town}-deacon, hq-{town}-boot
    if len(parts) == 2 {
        town := parts[0]
        role := parts[1]
        switch role {
        case "mayor":
            return &AgentIdentity{Role: RoleMayor, Town: town}, nil
        case "deacon":
            return &AgentIdentity{Role: RoleDeacon, Town: town}, nil
        case "boot":
            return &AgentIdentity{Role: RoleBoot, Town: town}, nil
        }
    }
}
```

### 3. Update `internal/session/town.go`

Update `TownSessions()` to accept town parameter:

```go
func TownSessions(town string) []TownSession {
    return []TownSession{
        {"Mayor", MayorSessionName(town)},
        {"Boot", BootSessionName(town)},
        {"Deacon", DeaconSessionName(town)},
    }
}
```

### 4. Update Callers (30 files)

All callers need to pass town name. Town name is typically:
- From `GT_ROOT` environment variable (basename)
- From discovered town root path
- From configuration

Key files to update:

| File | Function | Change |
|------|----------|--------|
| `internal/mayor/manager.go` | startup | Pass town to `MayorSessionName(town)` |
| `internal/deacon/manager.go` | startup | Pass town to `DeaconSessionName(town)` |
| `internal/cmd/start.go` | `gt up` | Pass town from discovered root |
| `internal/cmd/status.go` | `gt status` | Pass town for session lookups |
| `internal/daemon/daemon.go` | daemon | Pass town from config |
| `internal/doctor/*.go` | health checks | Pass town from context |

### 5. Migration Strategy

**Phase 1: Add New Functions (Backwards Compatible)**
- Add `MayorSessionName(town)` alongside legacy `LegacyMayorSessionName()`
- New deployments use town-namespaced sessions
- Old sessions continue to work

**Phase 2: Migrate Running Sessions**
- `gt doctor --fix` can detect and migrate sessions
- Or: `gt down && gt up` naturally creates new-format sessions

**Phase 3: Remove Legacy Support**
- After all towns upgraded, remove legacy functions
- Simplify parsing code

## Impact Analysis

### Affected Components

| Component | Impact | Notes |
|-----------|--------|-------|
| Mayor startup | Medium | Needs town discovery |
| Deacon startup | Medium | Needs town discovery |
| `gt up` / `gt down` | Medium | Pass town to session funcs |
| `gt status` | Low | Display change only |
| `gt doctor` | Medium | Health checks need update |
| `gt nudge` | Low | Uses session parsing |
| Mail routing | Low | Session lookup change |
| Event logging | Low | Session names in logs |

### Backwards Compatibility

- **Parsing**: Both old (`hq-mayor`) and new (`hq-gt11-mayor`) formats work
- **Running sessions**: Old sessions continue until restarted
- **Commands**: `gt` commands work with both formats during transition

## Testing Plan

1. **Unit tests**: `names_test.go` - verify new format generation
2. **Parse tests**: `identity_test.go` - verify bidirectional parsing
3. **Integration**: Start gt11 and gt12 mayors simultaneously
4. **Verify**: `tmux list-sessions` shows distinct sessions

## Session Name Examples

After implementation, running gt11 and gt12 simultaneously:

```
# gt11 sessions
hq-gt11-mayor
hq-gt11-boot
hq-gt11-deacon
gt-gastown-witness
gt-gastown-refinery
gt-gastown-crew-slack_decisions
gt-beads-witness
gt-beads-refinery

# gt12 sessions (hypothetical)
hq-gt12-mayor
hq-gt12-boot
hq-gt12-deacon
gt-newrig-witness
gt-newrig-refinery
```

## Open Questions

1. **Town name source**: Should town name come from:
   - Basename of `GT_ROOT` (e.g., `gt11` from `/home/ubuntu/gt11`)
   - Explicit config in `town.toml`
   - Environment variable `GT_TOWN_NAME`

   **Recommendation**: Use basename of `GT_ROOT` by default, allow override via config.

2. **Boot prefix**: Should boot move to `hq-` or stay at `gt-`?

   **Recommendation**: Move to `hq-` for consistency. The collision issue is resolved
   by town namespacing.

## References

- `internal/session/names.go` - Session name generation
- `internal/session/identity.go` - Session name parsing
- `internal/session/town.go` - Town session management
- `internal/tmux/tmux.go` - Tmux operations
