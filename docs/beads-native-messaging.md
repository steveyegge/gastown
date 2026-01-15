# Beads-Native Messaging

This document describes the beads-native messaging system for Gas Town, which replaces the file-based messaging configuration with persistent beads stored in the town's `.beads` directory.

## Overview

Beads-native messaging introduces three new bead types for managing communication:

- **Groups** (`gt:group`) - Named collections of addresses for mail distribution
- **Queues** (`gt:queue`) - Work queues where messages can be claimed by workers
- **Channels** (`gt:channel`) - Pub/sub broadcast streams with message retention

All messaging beads use the `hq-` prefix because they are town-level entities that span rigs.

## Bead Types

### Groups (`gt:group`)

Groups are named collections of addresses used for mail distribution. When you send to a group, the message is delivered to all members.

**Bead ID format:** `hq-group-<name>` (e.g., `hq-group-ops-team`)

**Fields:**
- `name` - Unique group name
- `members` - Comma-separated list of addresses, patterns, or nested group names
- `created_by` - Who created the group (from BD_ACTOR)
- `created_at` - ISO 8601 timestamp

**Member types:**
- Direct addresses: `gastown/crew/max`, `mayor/`, `deacon/`
- Wildcard patterns: `*/witness`, `gastown/*`, `gastown/crew/*`
- Special patterns: `@town`, `@crew`, `@witnesses`
- Nested groups: Reference other group names

### Queues (`gt:queue`)

Queues are work queues where messages wait to be claimed by workers. Unlike groups, each message goes to exactly one claimant.

**Bead ID format:** `hq-q-<name>` (town-level) or `gt-q-<name>` (rig-level)

**Fields:**
- `name` - Queue name
- `status` - `active`, `paused`, or `closed`
- `max_concurrency` - Maximum concurrent workers (0 = unlimited)
- `processing_order` - `fifo` or `priority`
- `available_count` - Items ready to process
- `processing_count` - Items currently being processed
- `completed_count` - Items completed
- `failed_count` - Items that failed

### Channels (`gt:channel`)

Channels are pub/sub streams for broadcasting messages. Messages are retained according to the channel's retention policy.

**Bead ID format:** `hq-channel-<name>` (e.g., `hq-channel-alerts`)

**Fields:**
- `name` - Unique channel name
- `subscribers` - Comma-separated list of subscribed addresses
- `status` - `active` or `closed`
- `retention_count` - Number of recent messages to retain (0 = unlimited)
- `retention_hours` - Hours to retain messages (0 = forever)
- `created_by` - Who created the channel
- `created_at` - ISO 8601 timestamp

## CLI Commands

### Group Management

```bash
# List all groups
gt mail group list

# Show group details
gt mail group show <name>

# Create a new group with members
gt mail group create <name> [members...]
gt mail group create ops-team gastown/witness gastown/crew/max

# Add member to group
gt mail group add <name> <member>

# Remove member from group
gt mail group remove <name> <member>

# Delete a group
gt mail group delete <name>
```

### Channel Management

```bash
# List all channels
gt mail channel
gt mail channel list

# View channel messages
gt mail channel <name>
gt mail channel show <name>

# Create a channel with retention policy
gt mail channel create <name> [--retain-count=N] [--retain-hours=N]
gt mail channel create alerts --retain-count=100

# Delete a channel
gt mail channel delete <name>
```

### Sending Messages

The `gt mail send` command now supports groups, queues, and channels:

```bash
# Send to a group (expands to all members)
gt mail send my-group -s "Subject" -m "Body"

# Send to a queue (single message, workers claim)
gt mail send queue:my-queue -s "Work item" -m "Details"

# Send to a channel (broadcast with retention)
gt mail send channel:my-channel -s "Announcement" -m "Content"

# Direct address (unchanged)
gt mail send gastown/crew/max -s "Hello" -m "World"
```

## Address Resolution

When sending mail, addresses are resolved in this order:

1. **Explicit prefix** - If address starts with `group:`, `queue:`, or `channel:`, use that type directly
2. **Contains `/`** - Treat as agent address or pattern (direct delivery)
3. **Starts with `@`** - Special pattern (`@town`, `@crew`, etc.) or beads-native group
4. **Name lookup** - Search for group → queue → channel by name

If a name matches multiple types (e.g., both a group and a channel named "alerts"), the resolver returns an error and requires an explicit prefix.

## Key Implementation Files

| File | Description |
|------|-------------|
| `internal/beads/beads_group.go` | Group bead CRUD operations |
| `internal/beads/beads_queue.go` | Queue bead CRUD operations |
| `internal/beads/beads_channel.go` | Channel bead + retention logic |
| `internal/mail/resolve.go` | Address resolution logic |
| `internal/cmd/mail_group.go` | Group CLI commands |
| `internal/cmd/mail_channel.go` | Channel CLI commands |
| `internal/cmd/mail_send.go` | Updated send with resolver |

## Retention Policy

Channels support two retention mechanisms:

- **Count-based** (`--retain-count=N`): Keep only the last N messages
- **Time-based** (`--retain-hours=N`): Delete messages older than N hours

Retention is enforced:
1. **On-write**: After posting a new message, old messages are pruned
2. **On-patrol**: Deacon patrol runs `PruneAllChannels()` as a backup cleanup

The patrol uses a 10% buffer to avoid thrashing (only prunes if count > retainCount × 1.1).

## Examples

### Create a team distribution group

```bash
# Create a group for the ops team
gt mail group create ops-team gastown/witness gastown/crew/max deacon/

# Send to the group
gt mail send ops-team -s "Team meeting" -m "Tomorrow at 10am"

# Add a new member
gt mail group add ops-team gastown/crew/dennis
```

### Set up an alerts channel

```bash
# Create an alerts channel that keeps last 50 messages
gt mail channel create alerts --retain-count=50

# Send an alert
gt mail send channel:alerts -s "Build failed" -m "See CI for details"

# View recent alerts
gt mail channel alerts
```

### Create nested groups

```bash
# Create role-based groups
gt mail group create witnesses */witness
gt mail group create leads gastown/crew/max gastown/crew/dennis

# Create a group that includes other groups
gt mail group create all-hands witnesses leads mayor/
```
