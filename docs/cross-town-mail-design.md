# Cross-Town Mail Design: gt11 ↔ gt12 Communication

**Task:** hq-arcu0q.6
**Epic:** hq-arcu0q (Colonization)
**Date:** 2026-01-27
**Status:** Design Complete

## Problem Statement

Currently, mail addresses are relative to a single town. The mayor in gt11 cannot
send mail to the mayor in gt12 because there's no way to specify a cross-town
recipient.

## Current Mail System

### Address Format (Within Town)

| Type | Format | Example |
|------|--------|---------|
| Town-level | `{role}/` | `mayor/`, `deacon/` |
| Rig singleton | `{rig}/{role}` | `gastown/witness` |
| Rig agent | `{rig}/{name}` | `gastown/Toast` |
| Rig agent (explicit) | `{rig}/crew/{name}` | `gastown/crew/max` |
| Human | `overseer` | `overseer` |

### Storage

All mail for a town is stored in `{townRoot}/.beads/` using the beads system.
Messages are created with `bd create --type=message`.

### Limitation

No town identifier in addresses means no way to route to another town.

## Proposed Solution: Town-Prefixed Addresses

### New Address Format

```
# Local (same-town) - unchanged
mayor/
gastown/crew/max
@town

# Cross-town - NEW
gt12/mayor                    # gt12's mayor
gt12/deacon                   # gt12's deacon
gt12/newrig/witness           # Witness in gt12's rig
gt12/newrig/crew/alice        # Crew in gt12's rig
@gt12/town                    # All town-level agents in gt12
```

### Routing Table

Configure known towns in `mayor/messaging.toml`:

```toml
[towns]
gt11 = "/home/ubuntu/gt11"
gt12 = "/home/ubuntu/gt12"
# Future: gt13 = "/home/ubuntu/gt13"
```

### Example Usage

```bash
# From gt11 mayor to gt12 mayor
gt mail send gt12/mayor -s "Colonization complete" -m "gt12 is operational"

# From gt12 deacon to gt11 witness
gt mail send gt11/gastown/witness -s "Status check" -m "Cross-town ping"
```

## Implementation Design

### 1. Address Parsing (`internal/mail/types.go`)

Extend `AddressToIdentity()` to extract town prefix:

```go
type MailAddress struct {
    Town     string // Empty for local, "gt12" for cross-town
    Identity string // "mayor/", "gastown/crew/max", etc.
}

func ParseMailAddress(addr string) (*MailAddress, error) {
    // Check for town prefix: {town}/{rest}
    // Known towns from config
    for townName := range knownTowns {
        if strings.HasPrefix(addr, townName+"/") {
            return &MailAddress{
                Town:     townName,
                Identity: strings.TrimPrefix(addr, townName+"/"),
            }, nil
        }
    }
    // No town prefix = local
    return &MailAddress{Town: "", Identity: addr}, nil
}
```

### 2. Beads Directory Resolution (`internal/mail/router.go`)

Update `resolveBeadsDir()` to handle cross-town:

```go
func (r *Router) resolveBeadsDir(address *MailAddress) string {
    if address.Town == "" || address.Town == r.localTown {
        return filepath.Join(r.townRoot, ".beads")
    }
    // Cross-town: look up town root from routing table
    if townRoot, ok := r.townRoots[address.Town]; ok {
        return filepath.Join(townRoot, ".beads")
    }
    return "" // Unknown town - error
}
```

### 3. Configuration (`internal/config/types.go`)

Add town routing to messaging config:

```go
type MessagingConfig struct {
    // ... existing fields ...

    // TownRoutes maps town names to their root directories
    // for cross-town mail delivery
    TownRoutes map[string]string `toml:"towns" json:"towns"`
}
```

### 4. Router Updates (`internal/mail/router.go`)

```go
type Router struct {
    townRoot   string            // Local town root
    localTown  string            // Local town name (e.g., "gt11")
    townRoots  map[string]string // Town routing table
    // ... existing fields ...
}

func (r *Router) Send(msg Message) error {
    addr, err := ParseMailAddress(msg.To)
    if err != nil {
        return err
    }

    beadsDir := r.resolveBeadsDir(addr)
    if beadsDir == "" {
        return fmt.Errorf("unknown town: %s", addr.Town)
    }

    // Create message in target town's beads
    return r.createMessage(beadsDir, addr.Identity, msg)
}
```

### 5. Session Notification

Cross-town recipients can't be notified via tmux (different session namespace):

```go
func (r *Router) notifyRecipient(addr *MailAddress) {
    if addr.Town != "" && addr.Town != r.localTown {
        // Skip notification - recipient in different town
        // They'll see it when they check mail
        return
    }
    // Local notification via tmux...
}
```

## Architectural Notes

### Shared Beads Database

The colonization epic specifies gt11 and gt12 share the same Dolt beads database.
This simplifies cross-town mail:

- Both towns write to the same underlying database
- No need to copy messages between databases
- Routing is just about addressing, not data movement

However, the routing table is still needed to:
1. Validate cross-town addresses
2. Determine which town's `.beads/` directory to use for queries
3. Future: Support towns with separate databases

### Group Addresses

Cross-town group addresses require querying the remote town's agents:

```
@gt12/town           # Query gt12's agent beads for town-level agents
@gt12/crew/newrig    # Query gt12's agent beads for rig crew
```

This adds complexity but follows the same pattern as local group resolution.

## Migration Path

### Phase 1: Add Routing Table
1. Add `[towns]` section to `mayor/messaging.toml`
2. Load routing table in Router initialization
3. No breaking changes - local mail unaffected

### Phase 2: Address Parsing
1. Update `AddressToIdentity()` for town prefix
2. Add validation for known towns
3. Update CLI help text

### Phase 3: Cross-Town Delivery
1. Implement `resolveBeadsDir()` with town routing
2. Skip tmux notifications for cross-town
3. Add error handling for unknown towns

### Phase 4: Group Resolution
1. Extend `@` address resolution for cross-town
2. Query remote town's agent beads
3. Fan out to cross-town recipients

## Testing Plan

1. **Unit tests**: Address parsing with and without town prefix
2. **Integration**: Send mail from gt11 mayor to gt12 mayor
3. **Receive test**: gt12 mayor checks inbox, sees cross-town message
4. **Reply test**: gt12 replies, gt11 receives reply
5. **Group test**: `@gt12/town` resolves to gt12's town agents

## Open Questions

1. **Auto-discovery vs config**: Should towns be auto-discovered from filesystem
   or require explicit configuration?

   **Recommendation**: Explicit config for now. Auto-discovery could miss towns
   in non-standard locations.

2. **Cross-town notifications**: With shared Dolt DB, could we notify via database
   events rather than tmux?

   **Recommendation**: Defer. Polling/pull model is simpler and sufficient.

3. **Address ambiguity**: What if a rig is named "gt12"?

   **Recommendation**: Town names in routing table take precedence. Document
   that rig names should not match town names.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/mail/types.go` | Add `ParseMailAddress()`, `MailAddress` struct |
| `internal/mail/router.go` | Town-aware routing, `resolveBeadsDir()` update |
| `internal/config/types.go` | Add `TownRoutes` to `MessagingConfig` |
| `internal/cmd/mail.go` | Update help, validation |
| `mayor/messaging.toml` | Add `[towns]` section |

## Example Configuration

```toml
# mayor/messaging.toml

[towns]
# Map town names to their root directories for cross-town mail
gt11 = "/home/ubuntu/gt11"
gt12 = "/home/ubuntu/gt12"

# Existing config...
[lists]
oncall = ["mayor/", "deacon/"]
```
