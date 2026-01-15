# Beads-Native Messaging

This document describes the beads-native messaging system, which extends Gas Town's mail system with first-class support for groups, queues, and channels backed by beads (Git-native storage).

## Overview

Beads-native messaging introduces three new bead types that integrate with the existing mail system:

- **Groups** (`gt:group`) - Named distribution lists for multi-recipient delivery
- **Queues** (`gt:queue`) - Work queues where workers claim items
- **Channels** (`gt:channel`) - Pub/sub broadcast streams with retention policies

All three are stored as beads, providing Git-native storage, audit trails, and replication.

## Bead Types

### Groups (`gt:group`)

Groups are named collections of addresses used for mail distribution. Members can be:
- Direct agent addresses (`gastown/crew/max`)
- Wildcard patterns (`*/witness`, `gastown/*`)
- Nested group names (groups can contain other groups)

**ID Format:** `hq-group-<name>` (e.g., `hq-group-ops-team`)

**Fields:**
- `name` - Unique group name
- `members` - Comma-separated list of addresses/patterns
- `created_by` - Who created the group
- `created_at` - ISO 8601 timestamp

**Source:** `internal/beads/beads_group.go`

### Queues (`gt:queue`)

Queues are work queues where multiple workers can claim items. Messages sent to a queue are delivered once to a claiming worker.

**ID Format:** `gt-q-<name>` or `hq-q-<name>` (town-level)

**Fields:**
- `name` - Queue name
- `status` - `active`, `paused`, or `closed`
- `max_concurrency` - Maximum concurrent workers (0 = unlimited)
- `processing_order` - `fifo` or `priority`
- `available_count`, `processing_count`, `completed_count`, `failed_count` - Queue statistics

**Source:** `internal/beads/beads_queue.go`

### Channels (`gt:channel`)

Channels are pub/sub broadcast streams. Messages sent to a channel are retained according to the channel's retention policy and can be viewed by any subscriber.

**ID Format:** `hq-channel-<name>` (e.g., `hq-channel-alerts`)

**Fields:**
- `name` - Unique channel name
- `subscribers` - Comma-separated list of subscribed addresses
- `status` - `active` or `closed`
- `retention_count` - Number of messages to retain (0 = unlimited)
- `retention_hours` - Hours to retain messages (0 = forever)
- `created_by` - Who created the channel
- `created_at` - ISO 8601 timestamp

**Retention Enforcement:**
- On-write cleanup: When a message is posted, old messages are pruned if over the limit
- Patrol cleanup: Deacon patrol runs periodic cleanup with 10% buffer to avoid thrashing

**Source:** `internal/beads/beads_channel.go`

## CLI Commands

### Group Commands

```bash
# List all groups
gt mail group list [--json]

# Show group details
gt mail group show <name> [--json]

# Create a group with members
gt mail group create <name> [members...]
gt mail group create ops-team gastown/witness gastown/crew/max
gt mail group create ops-team --member gastown/witness --member gastown/crew/max

# Add member to existing group
gt mail group add <name> <member>

# Remove member from group
gt mail group remove <name> <member>

# Delete a group
gt mail group delete <name>
```

**Source:** `internal/cmd/mail_group.go`

### Channel Commands

```bash
# List all channels
gt mail channel [--json]
gt mail channel list [--json]

# View channel messages
gt mail channel <name> [--json]
gt mail channel show <name> [--json]

# Create a channel with retention policy
gt mail channel create <name> [--retain-count=N] [--retain-hours=N]
gt mail channel create alerts --retain-count=100

# Delete a channel
gt mail channel delete <name>
```

**Source:** `internal/cmd/mail_channel.go`

### Sending Messages

The `gt mail send` command supports all address types through the address resolver:

```bash
# Send to agent (direct)
gt mail send gastown/crew/max -s "Hello" -m "Message body"

# Send to group (expands to all members)
gt mail send ops-team -s "Alert" -m "Important message"
gt mail send group:ops-team -s "Alert" -m "Explicit group syntax"

# Send to queue (delivered to one claiming worker)
gt mail send queue:build-queue -s "Job" -m "Build request"

# Send to channel (broadcast, retained)
gt mail send channel:alerts -s "Alert" -m "System alert"

# Send to pattern (wildcards)
gt mail send "*/witness" -s "Witness alert" -m "All witnesses"
```

**Source:** `internal/cmd/mail_send.go`

## Address Resolution

The address resolver (`internal/mail/resolve.go`) determines how to route messages based on the address format:

### Resolution Order

1. **Explicit prefix** - If address starts with `group:`, `queue:`, or `channel:`, use that type directly
2. **Contains `/`** - Treat as agent address or pattern (direct delivery)
3. **Starts with `@`** - Check for beads group, then fall back to built-in patterns
4. **Name lookup** - Search in order: group → queue → channel

### Address Formats

| Format | Type | Example |
|--------|------|---------|
| `group:<name>` | Group | `group:ops-team` |
| `queue:<name>` | Queue | `queue:build-queue` |
| `channel:<name>` | Channel | `channel:alerts` |
| `<town>/<role>/<name>` | Agent | `gastown/crew/max` |
| `<town>/<role>` | Agent | `gastown/witness` |
| `*/<role>` | Pattern | `*/witness` (all witnesses) |
| `@<name>` | Group/Pattern | `@ops-team` |

### Conflict Handling

If a name matches multiple types (e.g., both a group and a channel named "alerts"), the resolver returns an error requiring an explicit prefix:

```
ambiguous address "alerts": matches multiple types. Use explicit prefix: group:alerts, channel:alerts
```

## Key Files

| File | Description |
|------|-------------|
| `internal/beads/beads_group.go` | Group bead CRUD operations |
| `internal/beads/beads_queue.go` | Queue bead CRUD operations |
| `internal/beads/beads_channel.go` | Channel bead CRUD + retention |
| `internal/mail/resolve.go` | Address resolution logic |
| `internal/cmd/mail_group.go` | Group CLI commands |
| `internal/cmd/mail_channel.go` | Channel CLI commands |
| `internal/cmd/mail_send.go` | Send command with resolver |

## Examples

### Create a Team Distribution Group

```bash
# Create group
gt mail group create dev-team gastown/crew/max gastown/crew/dennis

# Add another member
gt mail group add dev-team gastown/crew/george

# Send to entire team
gt mail send dev-team -s "Standup" -m "Daily standup in 5 minutes"
```

### Set Up a Build Alert Channel

```bash
# Create channel with retention
gt mail channel create build-alerts --retain-count=50

# Send build notifications
gt mail send channel:build-alerts -s "Build #123 passed" -m "All tests green"

# View channel history
gt mail channel build-alerts
```

### Nested Groups

```bash
# Create base groups
gt mail group create witnesses gastown/witness ranchero/witness
gt mail group create crew gastown/crew/max gastown/crew/dennis

# Create umbrella group that includes other groups
gt mail group create all-agents witnesses crew deacon/

# Send to everyone
gt mail send all-agents -s "Town meeting" -m "All hands meeting at noon"
```
