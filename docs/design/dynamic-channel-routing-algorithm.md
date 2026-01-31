# Dynamic Channel Routing Algorithm

> Detailed algorithm specification for agent-controlled channel routing.
> Written 2026-01-30, polecat/slit session.
> Parent epic: gt-epc-dynamic_agent_controlled_channel_routing

## Overview

This document specifies the routing resolution algorithm for dynamic agent-controlled
channel routing. The algorithm determines which Slack channel receives decision
notifications based on a priority hierarchy of preferences.

---

## Resolution Priority Hierarchy

When a decision is created, the routing algorithm resolves the target channel using
this priority order (highest to lowest):

```
┌─────────────────────────────────────────────────────────────────────┐
│ PRIORITY 1: Decision-Level Override                                  │
│   Explicit channel specified in decision metadata                    │
│   Source: DecisionFields.ChannelOverride                            │
├─────────────────────────────────────────────────────────────────────┤
│ PRIORITY 2: Epic/Convoy Channel                                      │
│   Channel derived from parent work context                           │
│   Source: ParentBeadTitle via convoy lookup or epic fallback        │
├─────────────────────────────────────────────────────────────────────┤
│ PRIORITY 3: Agent Preference                                         │
│   Agent's configured default channel routing mode                    │
│   Source: Agent config (channel_mode: general|agent|epic|dm)        │
├─────────────────────────────────────────────────────────────────────┤
│ PRIORITY 4: Static Router Config                                     │
│   Pattern-based routing from slack.json                              │
│   Source: SlackConfig.Channels patterns                             │
├─────────────────────────────────────────────────────────────────────┤
│ PRIORITY 5: Workspace Default                                        │
│   Fallback channel for unmatched decisions                           │
│   Source: SlackConfig.DefaultChannel                                │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Resolution Algorithm

### Pseudocode

```
function resolveChannelForDecision(decision):
    // Priority 1: Decision-level override
    if decision.ChannelOverride is set:
        channel = validateChannel(decision.ChannelOverride)
        if channel is valid:
            return channel
        // Invalid override: log warning, continue to next priority

    // Priority 2: Epic/Convoy channel (work-based routing)
    if decision.ParentBeadID is set:
        // Try convoy first
        convoyTitle = getTrackingConvoyTitle(decision.ParentBeadID)
        if convoyTitle is not empty:
            channel = ensureWorkChannel(convoyTitle, "convoy")
            if channel is valid:
                return channel

        // Fall back to epic
        if decision.ParentBeadTitle is set:
            channel = ensureWorkChannel(decision.ParentBeadTitle, "epic")
            if channel is valid:
                return channel

    // Priority 3: Agent preference
    agentPref = getAgentChannelPreference(decision.RequestedBy)
    if agentPref is not nil:
        channel = resolveAgentPreference(agentPref, decision)
        if channel is valid:
            return channel

    // Priority 4: Static router config
    if staticRouter is enabled:
        channel = staticRouter.ResolveChannel(decision.RequestedBy)
        if channel is not default:  // Found a pattern match
            return channel

    // Priority 5: Workspace default
    return config.DefaultChannel
```

### Go Implementation Signature

```go
// ResolveChannelForDecision determines the Slack channel for a decision
// notification using the priority hierarchy.
func (b *Bot) ResolveChannelForDecision(ctx context.Context, d *DecisionEvent) (string, error) {
    // Returns: channelID, error
}

// ChannelResolution captures the routing decision for logging/debugging
type ChannelResolution struct {
    ChannelID   string         // Resolved channel ID
    ChannelName string         // Human-readable name (if known)
    Source      RoutingSource  // Which priority level resolved this
    Pattern     string         // Pattern that matched (if applicable)
    Fallbacks   []string       // Channels tried before final resolution
}

type RoutingSource int

const (
    SourceDecisionOverride RoutingSource = iota + 1
    SourceConvoyChannel
    SourceEpicChannel
    SourceAgentPreference
    SourceStaticRouter
    SourceWorkspaceDefault
)
```

---

## Priority 1: Decision-Level Override

### Purpose
Allow explicit channel targeting for specific decisions, overriding all other
routing logic.

### Usage
```bash
gt decision request --prompt "..." --option "..." --channel "#urgent-decisions"
```

### Data Model Addition
```go
type DecisionFields struct {
    // ... existing fields ...

    // ChannelOverride explicitly routes this decision to a specific channel.
    // Format: channel ID (C...) or channel name (#channel-name).
    // When set, overrides all other routing logic.
    ChannelOverride string `json:"channel_override,omitempty"`
}
```

### Validation
- If channel ID provided: verify it exists and bot has access
- If channel name provided: resolve to ID via Slack API
- On validation failure: log warning and continue to next priority

### Use Cases
- Urgent decisions that need specific attention
- Cross-team decisions that should go to a shared channel
- Testing/debugging to force routing to a specific channel

---

## Priority 2: Epic/Convoy Channel (Work-Based Routing)

### Purpose
Route decisions to channels based on the work context (parent epic or tracking
convoy), keeping related decisions grouped.

### Resolution Order
1. **Convoy lookup**: If decision has `ParentBeadID`, query beads database for
   any convoy that tracks this bead.
2. **Epic fallback**: If no convoy found, use `ParentBeadTitle` directly.

### Channel Name Derivation

```go
func DeriveChannelSlug(title string) string {
    // 1. Convert to lowercase
    // 2. Replace non-alphanumeric with hyphens
    // 3. Collapse multiple hyphens
    // 4. Trim leading/trailing hyphens
    // 5. Truncate to fit Slack's 80-char limit (with prefix)
    // 6. Prepend channel prefix (e.g., "gt-decisions-")
}
```

Examples:
- "Slack Integration Epic" → `gt-decisions-slack-integration-epic`
- "Fix Authentication Bug" → `gt-decisions-fix-authentication-bug`
- "Phase 2: User Onboarding Improvements" → `gt-decisions-phase-2-user-onboarding-improvements`

### Channel Auto-Creation
When epic/convoy channel doesn't exist and `dynamicChannels` is enabled:
1. Create public channel with derived name
2. Invite configured users (auto-invite list)
3. Set channel topic with epic/convoy description
4. Cache channel ID for future lookups

### Convoy Query
```sql
SELECT i.id, i.title
FROM issues i
JOIN dependencies d ON i.id = d.issue_id
WHERE d.type = 'tracks'
  AND d.depends_on_id = ?  -- parent bead ID
  AND i.status != 'closed'
ORDER BY i.created_at DESC
LIMIT 1
```

---

## Priority 3: Agent Preference

### Purpose
Allow agents to configure their default channel routing behavior.

### Preference Modes

| Mode | Description | Channel Resolution |
|------|-------------|-------------------|
| `general` | Use workspace default channel | `config.DefaultChannel` |
| `agent` | Per-agent dedicated channel | Create/use `gt-decisions-{rig}-{role}` |
| `epic` | Route by parent epic | Use epic channel (if parent set) |
| `dm` | Direct message to overseer | Open DM with configured overseer |

### Storage Location

Agent preferences stored in rig settings (`<rigPath>/settings/config.json`):

```json
{
  "agent_channel_preferences": {
    "gastown/polecats/*": {
      "channel_mode": "epic",
      "fallback_mode": "agent"
    },
    "gastown/crew/*": {
      "channel_mode": "dm",
      "overseer_user": "U123456789",
      "fallback_mode": "general"
    }
  }
}
```

### Go Types

```go
// AgentChannelPreference defines an agent's preferred channel routing.
type AgentChannelPreference struct {
    // ChannelMode is the primary routing mode.
    // Values: "general", "agent", "epic", "dm"
    ChannelMode string `json:"channel_mode"`

    // FallbackMode is used when primary mode fails.
    // Values: "general", "agent" (dm cannot be fallback)
    FallbackMode string `json:"fallback_mode,omitempty"`

    // OverseerUser is the Slack user ID for DM mode.
    // Required when ChannelMode is "dm".
    OverseerUser string `json:"overseer_user,omitempty"`

    // DedicatedChannel is an explicit channel for this agent.
    // Overrides auto-generated channel name in "agent" mode.
    DedicatedChannel string `json:"dedicated_channel,omitempty"`
}
```

### Resolution Logic

```go
func resolveAgentPreference(pref *AgentChannelPreference, d *DecisionEvent) (string, error) {
    switch pref.ChannelMode {
    case "general":
        return "", nil  // Signal to use default

    case "agent":
        if pref.DedicatedChannel != "" {
            return pref.DedicatedChannel, nil
        }
        return ensureAgentChannel(d.RequestedBy)

    case "epic":
        if d.ParentBeadTitle != "" {
            return ensureEpicChannel(d.ParentBeadTitle)
        }
        return resolveFallback(pref.FallbackMode, d)

    case "dm":
        return openDMChannel(pref.OverseerUser)

    default:
        return "", fmt.Errorf("unknown channel mode: %s", pref.ChannelMode)
    }
}
```

### Preference Inheritance

Agent preferences support pattern matching (same as static router):
```
1. Exact match:     preferences["gastown/polecats/furiosa"]
2. Wildcard match:  preferences["gastown/polecats/*"]
3. Role match:      preferences["*/polecats/*"]
4. Rig match:       preferences["gastown/*"]
5. No match:        return nil (continue to next priority)
```

---

## Priority 4: Static Router Config

### Purpose
Pattern-based routing from configuration file, unchanged from existing design.

### Reference
See [slack-channel-routing.md](./slack-channel-routing.md) for full specification.

### Integration
The static router is consulted after agent preferences, providing a fallback
for agents without explicit preferences.

---

## Priority 5: Workspace Default

### Purpose
Final fallback when no other routing applies.

### Configuration
```json
{
  "default_channel": "C0123456789"
}
```

### Behavior
Always returns a valid channel. This is the safety net that ensures every
decision gets routed somewhere.

---

## Edge Cases and Fallback Rules

### Edge Case 1: Agent Has No Preference

**Scenario**: Agent `gastown/polecats/newbie` has no preference configured.

**Resolution**:
1. Skip Priority 3 (no preference)
2. Try Priority 4 (static router)
3. Fall back to Priority 5 (workspace default)

**Result**: Decision goes to default channel.

### Edge Case 2: Epic Channel Doesn't Exist

**Scenario**: Decision has `ParentBeadTitle = "New Feature Epic"` but channel
`#gt-decisions-new-feature-epic` doesn't exist.

**Resolution** (depends on configuration):

| `dynamicChannels` | `createOnDemand` | Behavior |
|-------------------|------------------|----------|
| `true` | `true` | Auto-create channel, invite users, return new channel |
| `true` | `false` | Log warning, fall back to next priority |
| `false` | - | Skip epic routing entirely, fall back |

**Default behavior**: `dynamicChannels=true, createOnDemand=true` (auto-create)

### Edge Case 3: Agent Channel Archived

**Scenario**: Agent's dedicated channel was archived.

**Resolution**:
1. Detect archived status from Slack API response
2. If `autoUnarchive=true`: Unarchive channel, return channel ID
3. If `autoUnarchive=false`: Log warning, use fallback mode

**Configuration**:
```json
{
  "channel_lifecycle": {
    "auto_unarchive": true,
    "archive_inactive_days": 30
  }
}
```

### Edge Case 4: DM Mode but Overseer Unavailable

**Scenario**: Agent preference is `dm` but overseer user ID is invalid or user
is deactivated.

**Resolution**:
1. Attempt to open DM channel with overseer
2. On failure (user not found, deactivated, etc.):
   - Log error with details
   - Use `fallback_mode` from agent preference
   - If no fallback configured, use workspace default

**Error logging**:
```
Slack: DM routing failed for gastown/crew/max (overseer U123 unavailable), falling back to general
```

### Edge Case 5: Decision Override to Non-Existent Channel

**Scenario**: Decision specifies `--channel "#does-not-exist"`

**Resolution**:
1. Attempt to resolve channel name to ID
2. On failure:
   - Log warning: `Decision override channel '#does-not-exist' not found, using standard routing`
   - Continue to Priority 2 (epic/convoy)
   - Do NOT auto-create channels from decision overrides

### Edge Case 6: Circular Fallback

**Scenario**: Agent preference `epic` falls back to `epic`.

**Prevention**:
1. Validate preferences on load
2. Reject circular fallbacks at configuration time
3. Enforce: `fallback_mode` cannot equal `channel_mode`
4. Enforce: `dm` cannot be a fallback mode

---

## Flowchart

```
                            ┌─────────────────────┐
                            │  Decision Created   │
                            └──────────┬──────────┘
                                       │
                            ┌──────────▼──────────┐
                            │ ChannelOverride set?│
                            └──────────┬──────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    │ YES              │                  │ NO
                    ▼                  │                  ▼
           ┌───────────────┐           │        ┌─────────────────┐
           │Validate channel│          │        │ParentBeadID set?│
           └───────┬───────┘           │        └────────┬────────┘
                   │                   │                 │
        ┌──────────┼────────┐          │      ┌──────────┼────────┐
        │ VALID    │INVALID │          │      │ YES      │        │ NO
        ▼          ▼        │          │      ▼          │        ▼
    ┌───────┐  [log warn]   │          │  ┌────────────┐ │  ┌───────────────┐
    │RETURN │      │        │          │  │Query convoy│ │  │Get agent pref │
    │channel│      └────────┼──────────┼──┤   title    │ │  └───────┬───────┘
    └───────┘               │          │  └─────┬──────┘ │          │
                            │          │        │        │          │
                            │          │  ┌─────▼──────┐ │          │
                            │          │  │Convoy found?│ │          │
                            │          │  └─────┬──────┘ │          │
                            │          │        │        │          │
                            │       ┌──┴─────┬──┴────┐   │          │
                            │       │YES     │       │NO │          │
                            │       ▼        │       ▼   │          │
                            │  ┌─────────┐   │ ┌─────────────────┐  │
                            │  │Ensure   │   │ │Use ParentBead   │  │
                            │  │convoy   │   │ │Title for epic   │  │
                            │  │channel  │   │ │channel          │  │
                            │  └────┬────┘   │ └────────┬────────┘  │
                            │       │        │          │           │
                            │       ▼        │          ▼           │
                            │  ┌─────────┐   │   ┌─────────────┐    │
                            │  │Channel  │   │   │Ensure epic  │    │
                            │  │exists?  │   │   │channel      │    │
                            │  └────┬────┘   │   └──────┬──────┘    │
                            │       │        │          │           │
                       ┌────┼───┬───┴────┐   │    ┌─────┼─────┐     │
                       │    │YES│        │NO │    │YES  │     │NO   │
                       │    ▼   │        ▼   │    ▼     │     ▼     │
                       │┌───────┐│  ┌────────┐│┌───────┐│ [fallback]│
                       ││RETURN ││  │Create  │││RETURN ││     │     │
                       ││channel││  │channel?│││channel││     │     │
                       │└───────┘│  └───┬────┘│└───────┘│     │     │
                       │         │      │     │         │     │     │
                       │         │ ┌────┴───┐ │         │     │     │
                       │         │ │YES  NO │ │         │     │     │
                       │         │ ▼     │  │ │         │     │     │
                       │         │create │  │ │         │     │     │
                       │         │  │    │  │ │         │     │     │
                       │         │  ▼    ▼  │ │         │     │     │
                       │         │RETURN │  │ │         │     │     │
                       │         │       │  │ │         │     │     │
                       │         └───────┼──┘ │         │     │     │
                       │                 │    │         │     │     │
                       └─────────────────┴────┴─────────┴─────┴─────┘
                                         │
                            ┌────────────▼────────────┐
                            │   Agent pref resolved?  │
                            └────────────┬────────────┘
                                         │
                    ┌────────────────────┼────────────────────┐
                    │ YES                │                    │ NO
                    ▼                    │                    ▼
           ┌───────────────┐             │          ┌─────────────────┐
           │ RETURN channel│             │          │Static router    │
           └───────────────┘             │          │match?           │
                                         │          └────────┬────────┘
                                         │                   │
                                    ┌────┴────┐              │
                                    │YES      │NO            │
                                    ▼         ▼              │
                            ┌───────────┐ ┌──────────────┐   │
                            │RETURN     │ │RETURN default│   │
                            │matched    │ │channel       │   │
                            │channel    │ └──────────────┘   │
                            └───────────┘                    │
```

---

## Configuration Reference

### Complete Configuration Example

```json
{
  "type": "slack",
  "version": 2,
  "enabled": true,
  "default_channel": "C0000000001",

  "channels": {
    "gastown/polecats/*": "C1111111111",
    "gastown/crew/*": "C2222222222",
    "*/witness": "C3333333333"
  },

  "overrides": {
    "gastown/crew/slack_decisions": "C9999999999"
  },

  "channel_names": {
    "C0000000001": "#decisions-general",
    "C1111111111": "#decisions-polecats"
  },

  "dynamic_channels": {
    "enabled": true,
    "create_on_demand": true,
    "channel_prefix": "gt-decisions",
    "auto_invite": ["U123456789", "U987654321"],
    "auto_unarchive": true
  },

  "agent_channel_preferences": {
    "gastown/polecats/*": {
      "channel_mode": "epic",
      "fallback_mode": "agent"
    },
    "gastown/crew/*": {
      "channel_mode": "agent"
    },
    "mayor": {
      "channel_mode": "dm",
      "overseer_user": "U123456789",
      "fallback_mode": "general"
    }
  },

  "channel_lifecycle": {
    "auto_unarchive": true,
    "archive_inactive_days": 30
  }
}
```

### New Schema Fields (Version 2)

```go
type SlackConfig struct {
    // ... existing v1 fields ...

    // DynamicChannels controls automatic channel creation.
    DynamicChannels *DynamicChannelConfig `json:"dynamic_channels,omitempty"`

    // AgentChannelPreferences maps agent patterns to routing preferences.
    AgentChannelPreferences map[string]*AgentChannelPreference `json:"agent_channel_preferences,omitempty"`

    // ChannelLifecycle controls channel lifecycle behavior.
    ChannelLifecycle *ChannelLifecycleConfig `json:"channel_lifecycle,omitempty"`
}

type DynamicChannelConfig struct {
    Enabled       bool     `json:"enabled"`
    CreateOnDemand bool    `json:"create_on_demand"`
    ChannelPrefix string   `json:"channel_prefix"`
    AutoInvite    []string `json:"auto_invite,omitempty"`
    AutoUnarchive bool     `json:"auto_unarchive"`
}

type ChannelLifecycleConfig struct {
    AutoUnarchive       bool `json:"auto_unarchive"`
    ArchiveInactiveDays int  `json:"archive_inactive_days,omitempty"`
}
```

---

## Logging and Debugging

### Debug Mode Output

With `GTSLACK_DEBUG=1` or `-debug` flag:

```
Slack: Resolving channel for decision dec-abc123
Slack:   Agent: gastown/polecats/furiosa
Slack:   ParentBead: gt-epc-slack-integration
Slack:   ChannelOverride: (none)
Slack:   Priority 1 (override): skipped (not set)
Slack:   Priority 2 (epic): checking convoy...
Slack:   Priority 2 (epic): convoy not found, trying epic
Slack:   Priority 2 (epic): epic channel exists: C_SLACK_INTEGRATION
Slack:   Resolved: C_SLACK_INTEGRATION (#gt-decisions-slack-integration)
Slack:   Source: SourceEpicChannel
```

### Resolution Audit Log

Each routing decision is logged with:
- Decision ID
- Agent identity
- Final channel
- Resolution source
- Any fallbacks attempted
- Timestamp

---

## Testing Strategy

### Unit Tests

1. **Priority ordering**: Verify higher priority wins
2. **Fallback chains**: Test each edge case fallback
3. **Pattern matching**: Agent preference pattern resolution
4. **Channel validation**: Override validation logic
5. **Circular fallback detection**: Configuration validation

### Integration Tests

1. **Full resolution flow**: Decision → Channel
2. **Auto-create channels**: Dynamic channel creation
3. **Auto-unarchive**: Archived channel recovery
4. **DM routing**: Direct message channel opening
5. **Convoy lookup**: Database query integration

### Manual Testing

```bash
# Test resolution for specific agent
gt slack resolve "gastown/polecats/furiosa" --decision-parent="gt-epc-test"

# Test with override
gt slack resolve "gastown/crew/max" --override="#urgent"

# Test preference modes
gt slack test-preference "gastown/polecats/*" --mode=epic
gt slack test-preference "gastown/crew/*" --mode=dm --overseer=U123
```

---

## Migration from v1

### Backward Compatibility

- v1 configs continue to work (no agent preferences, no dynamic channels)
- Version field distinguishes: `"version": 1` vs `"version": 2`
- Missing `agent_channel_preferences` defaults to empty (skip Priority 3)
- Missing `dynamic_channels` defaults to disabled

### Migration Path

1. **Phase 1**: Deploy v2 schema support (backward compatible)
2. **Phase 2**: Add agent preferences incrementally
3. **Phase 3**: Enable dynamic channels for willing teams
4. **Phase 4**: Deprecate v1 configs (with migration tool)

---

## Implementation Checklist

- [ ] Add `ChannelOverride` field to `DecisionFields`
- [ ] Add `AgentChannelPreference` type and storage
- [ ] Add `DynamicChannelConfig` to `SlackConfig`
- [ ] Implement `ResolveChannelForDecision` with full priority chain
- [ ] Implement agent preference resolution with pattern matching
- [ ] Add channel auto-creation logic
- [ ] Add channel auto-unarchive logic
- [ ] Add DM channel opening for overseer
- [ ] Add `--channel` flag to `gt decision request`
- [ ] Add configuration validation (circular fallback detection)
- [ ] Add debug logging for resolution flow
- [ ] Add unit tests for all edge cases
- [ ] Add integration tests for Slack API interactions
- [ ] Update documentation

---

## Dependencies

- **gt-epc-dynamic_agent_controlled_channel_routing.1**: Research storage options (for agent preferences)
- **gt-epc-dynamic_agent_controlled_channel_routing.2**: Research channel lifecycle (for auto-create/unarchive)
- **gt-epc-dynamic_agent_controlled_channel_routing.4**: Implement agent preference storage
- **gt-epc-dynamic_agent_controlled_channel_routing.5**: Implement channel auto-creation
- **gt-epc-dynamic_agent_controlled_channel_routing.6**: Integrate into gtslack

---

## Appendix A: Channel Mode Comparison

| Mode | Pro | Con | Use Case |
|------|-----|-----|----------|
| `general` | Simple, all decisions visible | Noisy for large teams | Small teams, initial setup |
| `agent` | Isolated per-agent | Many channels to monitor | Dedicated agent oversight |
| `epic` | Groups related decisions | Requires parent beads | Project-focused teams |
| `dm` | Private, immediate | Single point of failure | Critical decisions, VIPs |

---

## Appendix B: Slack API Considerations

### Rate Limits
- Channel creation: 20/min
- Channel lookup: 20/min
- Message posting: 1/sec per channel

### Recommendations
- Cache channel lookups aggressively
- Batch channel creation during setup
- Use exponential backoff on rate limit errors

### Required Scopes
- `channels:manage` - Create and archive channels
- `channels:write.invites` - Invite users to channels
- `im:write` - Open DM channels
- `chat:write` - Post messages
