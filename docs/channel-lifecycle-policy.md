# Slack Channel Lifecycle Policy

This document describes the policies for dynamic Slack channel creation, management, and archival in Gas Town's Slack integration.

## Overview

Gas Town's Slack bot (`gtslack`) supports dynamic channel routing to organize decision notifications by agent or epic. This document establishes policies for channel lifecycle management.

## Current Implementation Status

| Feature | Status | Notes |
|---------|--------|-------|
| Dynamic channel creation | Implemented | `-dynamic-channels` flag |
| Auto-invite | Implemented | `-auto-invite` flag |
| Pattern-based routing | Implemented | `slack.json` config |
| Break Out feature | Implemented | Per-agent dedicated channels |
| Epic-based routing | Implemented | Channels per parent epic |
| Channel archiving | **Not implemented** | Proposed in this document |

## 1. Auto-Creation Policy

### When to Auto-Create

Channels are auto-created when **all** of the following are true:

1. `-dynamic-channels` flag is enabled
2. A decision routes to a channel that doesn't exist
3. The bot has `channels:manage` scope

### Auto-Creation Triggers

| Routing Type | Creates Channel | Channel Name Format |
|--------------|-----------------|---------------------|
| Agent-based | Yes (if dynamic) | `{prefix}-{rig}-{role}` |
| Epic-based | Yes (if dynamic) | `{prefix}-{epic-slug}` |
| Convoy-based | Uses epic channel | (inherits from epic) |
| Break Out | Always | `{prefix}-{rig}-{role}-{agent}` |

### Implementation Notes

- Channels are created lazily on first message routing
- `name_taken` errors are handled by finding the existing channel
- Cache prevents redundant API lookups (in-memory)

## 2. Naming Conventions

### Channel Name Format

All channel names follow these rules:
- Lowercase only
- Alphanumeric and hyphens only (no underscores)
- Max 80 characters (Slack limit)
- Truncated at word boundary when exceeding limit

### Per-Agent Channels

**Format:** `{prefix}-{rig}-{role}`

| Agent Identity | Channel Name |
|----------------|--------------|
| `gastown/polecats/furiosa` | `gt-decisions-gastown-polecats` |
| `beads/crew/wolf` | `gt-decisions-beads-crew` |
| `mayor` | `gt-decisions-mayor` |

**Rationale:** Agent names are ephemeral (polecats are single-use), so channels are grouped by `rig/role` rather than individual agent names. This prevents channel proliferation.

### Per-Agent Break Out Channels

**Format:** `{prefix}-{rig}-{role}-{agent}`

When using Break Out for a specific agent:
| Agent Identity | Channel Name |
|----------------|--------------|
| `gastown/crew/wolf` | `gt-decisions-gastown-crew-wolf` |

### Per-Epic Channels

**Format:** `{prefix}-{epic-slug}`

| Epic Title | Channel Name |
|------------|--------------|
| "Ephemeral Polecat Merge Workflow" | `gt-decisions-ephemeral-polecat-merge` |
| "Fix bug #123 in parser" | `gt-decisions-fix-bug-123-in-parser` |

**Slug generation:**
- Max 30 characters before prefix
- Truncated at word boundary
- Stop words removed from longer titles

### Recommended Prefix

Default prefix: `gt-decisions`

Configurable via bot initialization. Keep consistent across workspace.

## 3. Auto-Invite Policy

### Who Gets Auto-Invited

When a channel is created (or first accessed), auto-invite these users:

| Role | Why Auto-Invite |
|------|-----------------|
| Overseer/Admin | Visibility into all agent activity |
| Team leads | Oversight of their team's decisions |
| On-call engineers | Immediate awareness of urgent decisions |

### Configuration

Via `-auto-invite` flag (comma-separated Slack user IDs):
```bash
gtslack -auto-invite U123,U456,U789
```

Or via environment variable:
```bash
export SLACK_AUTO_INVITE=U123,U456,U789
```

### Error Handling

Auto-invite failures are logged but don't block channel creation:
- `already_in_channel` - Silently ignored
- `cant_invite_self` - Silently ignored (bot already in channel)
- Other errors - Logged as warning, operation continues

### Recommendation

**Auto-invite at minimum:**
1. The requesting human (if known from decision context)
2. The configured overseer account(s)

## 4. Channel Archiving Policy

### Current State

**Not implemented.** Channels are never archived automatically.

### Proposed Archiving Triggers

| Trigger | Action | Rationale |
|---------|--------|-----------|
| Epic closed | Archive epic channel after 7 days | Epic work complete |
| Agent terminated | Do not archive | Channel reused by future agents with same rig/role |
| No activity 30 days | Archive with warning | Prevent channel bloat |
| Manual request | Immediate archive | Admin override |

### Why NOT Archive on Agent Termination

Agent channels (`gt-decisions-gastown-polecats`) are shared by role:
- Multiple polecats use the same channel
- Archiving would break routing for the next polecat
- Archive only when the entire role is decommissioned

### Proposed Archive Workflow

1. **Pre-archive notice** (24 hours before):
   ```
   This channel will be archived in 24 hours due to [reason].
   Reply to cancel archiving.
   ```

2. **Archive execution:**
   - Call `conversations.archive` API
   - Log archival in audit trail
   - Update routing config to remove stale entries

3. **Restore capability:**
   - Archived channels can be unarchived if needed
   - Routing automatically finds unarchived channels

### Implementation Recommendations

Add to `gtslack`:
- New flag: `-archive-after-days=30` (default: disabled/0)
- Background goroutine to check channel activity
- Respect Slack API rate limits (Tier 2: 20+/minute)

## 5. Privacy Policy

### Current State

All dynamically created channels are **public** (`IsPrivate: false`).

### Rationale for Public Channels

| Aspect | Public Channel | Private Channel |
|--------|----------------|-----------------|
| Discoverability | Anyone can find and join | Must be invited |
| Transparency | Full audit trail visible | Limited visibility |
| Auto-invite | Works normally | Must explicitly manage |
| Bot scope needed | `channels:manage` | `groups:write` |

### When to Use Private Channels

Consider private channels for:
- Sensitive HR decisions
- Security incident response
- Credential/secret handling

### Proposed Privacy Configuration

Add to `slack.json`:
```json
{
  "channel_privacy": {
    "default": "public",
    "patterns": {
      "*/security/*": "private",
      "hr/*": "private"
    }
  }
}
```

### Recommendation

**Default to public** for:
- Maximum transparency
- Easier onboarding (anyone can join)
- Simpler permission model

**Use private** only when:
- Regulatory requirement
- Security-sensitive content
- HR/personnel matters

## 6. Slack API Considerations

### Rate Limits

| API Method | Tier | Limit |
|------------|------|-------|
| `conversations.create` | ~Tier 2 | 20+/minute |
| `conversations.archive` | ~Tier 2 | 20+/minute |
| `conversations.list` | Tier 2 | 20+/minute |

**Mitigation:**
- Cache channel name→ID mappings
- Batch operations where possible
- Implement exponential backoff

### Workspace Limits

Slack does not publish hard limits on channel counts, but:
- Very large workspaces (10k+ channels) may see performance degradation
- Enterprise Grid has higher limits than standard workspaces

### Recommendations

1. **Use role-based grouping** (not per-agent-instance)
2. **Implement archival** to prevent unbounded growth
3. **Monitor channel count** and alert at thresholds (e.g., 500, 1000)

## 7. Configuration Summary

### Minimal Setup (Break Out only)

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0123456789"
}
```

### Full Dynamic Routing

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0123456789",
  "channels": {
    "gastown/polecats/*": "C_GASTOWN_POLECATS",
    "gastown/crew/*": "C_GASTOWN_CREW",
    "*/polecats/*": "C_ALL_POLECATS"
  }
}
```

### Command Line Flags

| Flag | Description | Example |
|------|-------------|---------|
| `-dynamic-channels` | Enable auto-creation | `-dynamic-channels` |
| `-auto-invite` | Users to auto-invite | `-auto-invite=U123,U456` |
| `-channel-prefix` | Channel name prefix | `-channel-prefix=gt-decisions` |
| `-debug` | Verbose logging | `-debug` |

## 8. Future Work

### Recommended Enhancements

1. **Channel archival** (priority: medium)
   - Implement proposed policy above
   - Add monitoring for stale channels

2. **Private channel support** (priority: low)
   - Add privacy configuration to patterns
   - Requires `groups:write` scope

3. **Channel activity metrics** (priority: low)
   - Track message counts per channel
   - Identify unused/low-activity channels

4. **Multi-workspace support** (priority: low)
   - Enterprise Grid compatibility
   - Cross-workspace routing

## References

- [Slack API Rate Limits](https://docs.slack.dev/apis/web-api/rate-limits)
- [conversations.create](https://docs.slack.dev/reference/methods/conversations.create)
- [conversations.archive](https://docs.slack.dev/reference/methods/conversations.archive)
- [Gas Town Slack Bot Setup](./slack-bot-setup.md)
