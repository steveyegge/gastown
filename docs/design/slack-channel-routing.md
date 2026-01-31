# Slack Channel Routing Design

> Design for per-agent Slack channel routing in Gas Town decisions.
> Written 2026-01-29, polecat/furiosa session.
> Parent epic: hq-oezv5

## Problem Statement

Current Slack decision notifications go to a single channel configured in the bot.
Issues:
- All agents' decisions appear in one channel (noisy)
- No way to route different teams/projects to different channels
- Hard to track decisions by owner/context
- No fallback when channel not configured

## Design Goals

1. **Per-agent routing**: Decisions routed based on requesting agent identity
2. **Pattern matching**: Support wildcards for groups of agents
3. **Inheritance**: Fallback from specific to general (agent → rig → default)
4. **Config-driven**: No code changes needed for new channels
5. **Backward compatible**: Single-channel mode still works

---

## Architecture

### Resolution Order

When routing a decision from agent `gastown/polecats/furiosa`:

```
1. Exact match:     channels["gastown/polecats/furiosa"]
2. Wildcard match:  channels["gastown/polecats/*"]
3. Role match:      channels["*/polecats/*"]
4. Rig match:       channels["gastown/*"]
5. Default:         default_channel
```

First match wins. This allows specific overrides while providing sensible defaults.

### Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Decision Created                          │
│  RequestedBy: "gastown/polecats/furiosa"                    │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  Channel Router                              │
│  1. Load SlackConfig from settings/slack.json               │
│  2. Match agent against channel patterns                     │
│  3. Resolve channel ID                                       │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  Slack Client                                │
│  Post decision notification to resolved channel             │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration Schema

### File Location

`~/gt/settings/slack.json`

Following the existing pattern where `~/gt/settings/` contains town-level config.

### Go Types

```go
// SlackConfig represents Slack integration configuration.
type SlackConfig struct {
    Type    string `json:"type"`    // "slack"
    Version int    `json:"version"` // schema version

    // Enabled controls whether Slack notifications are active.
    Enabled bool `json:"enabled"`

    // DefaultChannel is the fallback channel when no pattern matches.
    // Format: channel ID (e.g., "C0123456789") or name (e.g., "#decisions")
    DefaultChannel string `json:"default_channel"`

    // Channels maps agent patterns to Slack channel IDs.
    // Patterns support wildcards: "*" matches any single segment.
    // Examples:
    //   "gastown/polecats/*"  → all polecats in gastown
    //   "*/crew/*"            → all crew members across rigs
    //   "beads/*"             → all agents in beads rig
    Channels map[string]string `json:"channels"`

    // ChannelNames maps channel IDs to human-readable names for display.
    // Optional; used for logging and debugging.
    ChannelNames map[string]string `json:"channel_names,omitempty"`

    // BotToken is the Slack bot OAuth token (xoxb-...).
    // Can also be set via SLACK_BOT_TOKEN environment variable.
    BotToken string `json:"bot_token,omitempty"`

    // AppToken is the Slack app-level token for Socket Mode (xapp-...).
    // Can also be set via SLACK_APP_TOKEN environment variable.
    AppToken string `json:"app_token,omitempty"`
}

const CurrentSlackVersion = 1
```

### JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "type": { "const": "slack" },
    "version": { "type": "integer", "minimum": 1 },
    "enabled": { "type": "boolean" },
    "default_channel": { "type": "string" },
    "channels": {
      "type": "object",
      "additionalProperties": { "type": "string" }
    },
    "channel_names": {
      "type": "object",
      "additionalProperties": { "type": "string" }
    },
    "bot_token": { "type": "string" },
    "app_token": { "type": "string" }
  },
  "required": ["type", "version", "enabled", "default_channel"]
}
```

---

## Configuration Examples

### Example 1: Single Channel (Backward Compatible)

All decisions go to one channel. Simplest configuration.

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0123456789",
  "channels": {}
}
```

### Example 2: Per-Rig Channels

Each rig has its own channel for decisions.

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0000000001",
  "channels": {
    "gastown/*": "C1111111111",
    "beads/*": "C2222222222",
    "longeye/*": "C3333333333"
  },
  "channel_names": {
    "C0000000001": "#decisions-general",
    "C1111111111": "#decisions-gastown",
    "C2222222222": "#decisions-beads",
    "C3333333333": "#decisions-longeye"
  }
}
```

### Example 3: Per-Agent Channels

Specific agents have dedicated channels.

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0000000001",
  "channels": {
    "gastown/crew/decision_point": "C9999999999",
    "gastown/polecats/*": "C1111111111",
    "gastown/crew/*": "C2222222222",
    "gastown/*": "C3333333333"
  }
}
```

Resolution for `gastown/crew/decision_point`: `C9999999999` (exact match)
Resolution for `gastown/polecats/furiosa`: `C1111111111` (wildcard match)
Resolution for `gastown/witness`: `C3333333333` (rig fallback)

### Example 4: Role-Based Channels

Different channels for different agent roles across all rigs.

```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0000000001",
  "channels": {
    "*/polecats/*": "C1111111111",
    "*/crew/*": "C2222222222",
    "*/witness": "C3333333333",
    "*/refinery": "C4444444444"
  },
  "channel_names": {
    "C1111111111": "#polecat-decisions",
    "C2222222222": "#crew-decisions",
    "C3333333333": "#witness-decisions",
    "C4444444444": "#refinery-decisions"
  }
}
```

---

## Pattern Matching Rules

### Syntax

- `*` matches exactly one path segment (not including `/`)
- Patterns match from left to right
- Longer/more-specific patterns take precedence

### Precedence

Patterns are sorted by specificity:
1. Exact matches (no wildcards)
2. Patterns with fewer wildcards
3. Patterns that are longer (more segments)

Within same specificity, first match in config order wins.

### Examples

| Agent | Pattern | Match? |
|-------|---------|--------|
| `gastown/polecats/furiosa` | `gastown/polecats/furiosa` | Yes (exact) |
| `gastown/polecats/furiosa` | `gastown/polecats/*` | Yes |
| `gastown/polecats/furiosa` | `*/polecats/*` | Yes |
| `gastown/polecats/furiosa` | `gastown/*` | Yes |
| `gastown/polecats/furiosa` | `beads/*` | No |
| `gastown/crew/max` | `*/crew/*` | Yes |
| `mayor` | `mayor` | Yes (exact) |
| `mayor` | `*` | Yes |

---

## Implementation

### Channel Router

```go
package slack

// Router resolves agent identities to Slack channel IDs.
type Router struct {
    config *SlackConfig
    // Compiled patterns sorted by specificity
    patterns []compiledPattern
}

type compiledPattern struct {
    pattern   string
    channel   string
    segments  []string  // split pattern
    wildcards int       // count of "*" segments
}

// NewRouter creates a router from config.
func NewRouter(cfg *SlackConfig) *Router {
    r := &Router{config: cfg}
    r.compilePatterns()
    return r
}

// ResolveChannel returns the Slack channel for an agent.
func (r *Router) ResolveChannel(agent string) string {
    // Try each pattern in specificity order
    for _, p := range r.patterns {
        if r.matches(agent, p) {
            return p.channel
        }
    }
    return r.config.DefaultChannel
}

func (r *Router) matches(agent string, p compiledPattern) bool {
    agentParts := strings.Split(agent, "/")
    if len(agentParts) != len(p.segments) {
        return false
    }
    for i, seg := range p.segments {
        if seg != "*" && seg != agentParts[i] {
            return false
        }
    }
    return true
}
```

### Integration with Decision Service

```go
// In decision creation/notification flow:

func (s *DecisionService) NotifySlack(ctx context.Context, d *Decision) error {
    if !s.slackConfig.Enabled {
        return nil
    }

    channel := s.slackRouter.ResolveChannel(d.RequestedBy)
    if channel == "" {
        return fmt.Errorf("no channel configured for agent: %s", d.RequestedBy)
    }

    return s.slackClient.PostDecision(ctx, channel, d)
}
```

---

## Migration Path

### Phase 1: Config Schema (this task)
- Add `SlackConfig` to `internal/config/types.go`
- Add config loading in `internal/config/load.go`
- Document schema

### Phase 2: Router Implementation
- Implement pattern matching
- Add unit tests for pattern precedence
- Integrate with existing bot

### Phase 3: Bot Integration
- Update `slackbot/bot.go` to use router
- Add channel resolution to notification flow
- Test with multiple channels

### Phase 4: Documentation
- Update user docs with config examples
- Add troubleshooting for channel routing

---

## Relationship to Existing Config

### EscalationConfig.Contacts.SlackWebhook

This existing field is for **escalation** webhooks, not decision routing.
Decision routing uses the new `SlackConfig.Channels` mapping.

The two are complementary:
- `EscalationConfig`: Routes by **severity** to notification channels
- `SlackConfig`: Routes by **agent** to decision channels

### MessagingConfig.NudgeChannels

This routes **nudges** (real-time tmux messages) by channel name.
Similar pattern but different transport (tmux vs Slack).

Could potentially unify in future, but keeping separate for now:
- `NudgeChannels`: Internal agent-to-agent communication
- `SlackConfig.Channels`: External human-facing notifications

---

## Testing Plan

### Unit Tests

- Pattern parsing and compilation
- Match precedence (specific > wildcard)
- Edge cases: empty agent, malformed patterns
- Config loading and validation

### Integration Tests

- Router resolves channels correctly
- Fallback to default when no match
- Multiple patterns with same agent

### Manual Testing

```bash
# Test channel resolution
gt slack resolve "gastown/polecats/furiosa"
# Output: #decisions-gastown (C1111111111)

gt slack resolve "unknown/agent"
# Output: #decisions-general (default)
```

---

## Security Considerations

### Token Storage

Slack tokens should not be committed to git:
- Use environment variables: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`
- Config file tokens are optional overrides
- Document token scopes needed

### Channel Access

Bot must be invited to all configured channels.
Provide helpful error when posting to channel bot isn't in.

---

## Open Questions

1. **Channel ID vs Name**: Use IDs (stable) or names (readable)?
   → **Decision**: Use IDs in config, names in `channel_names` for display

2. **Validation on startup**: Fail fast or warn on invalid channels?
   → **Decision**: Warn but continue; fail only on actual post

3. **Multi-workspace**: Support multiple Slack workspaces?
   → **Deferred**: Single workspace for v1, can extend later

---

## Dependencies

- `hq-oezv5.2`: Implement Slack HTTP client (uses this config)
- `hq-oezv5.3`: Add channel routing configuration (implements loading)
- `hq-oezv5.4`: Integrate routing into decision workflow

---

## Usage Guide

### Setup

1. **Create config file** at `~/gt/settings/slack.json`:
   ```bash
   cp ~/gt/settings/slack.json.example ~/gt/settings/slack.json
   # Edit with your actual Slack channel IDs
   ```

2. **Configure channel patterns** based on your team structure:
   - Use exact matches for specific agents: `"gastown/crew/decision_point": "C123"`
   - Use wildcards for groups: `"gastown/polecats/*": "C456"`
   - Set a default for unmatched agents: `"default_channel": "C789"`

3. **Restart gtslack** to load the new config:
   ```bash
   systemctl restart gtslack  # or kill and restart manually
   ```

### Verification

The router auto-loads from standard locations on startup. Check logs for:
```
Slack: Channel router auto-loaded
```

With debug mode (`-debug` flag), you'll see routing decisions:
```
Slack: Routing gastown/polecats/furiosa to channel C_GASTOWN_POLECATS (matched by: gastown/polecats/*)
```

### Testing Patterns

To verify pattern matching works as expected, check the unit tests:
```bash
cd gastown/mayor/rig
go test ./internal/slack/... -v -run TestResolve
```

### Troubleshooting

**Decisions not routing to expected channel:**
- Check pattern specificity - more specific patterns match first
- Verify channel IDs are correct (not channel names)
- Enable debug mode to see routing decisions

**Router not loading:**
- Check file exists at `~/gt/settings/slack.json`
- Validate JSON syntax: `jq . ~/gt/settings/slack.json`
- Check file permissions

**Bot can't post to channel:**
- Ensure bot is invited to the channel
- Verify channel ID is correct (not archived)

---

## Appendix: Agent Identity Format

Gas Town agent identities follow the pattern: `<rig>/<role>/<name>`

Examples:
- `gastown/polecats/furiosa` - polecat named furiosa in gastown rig
- `gastown/crew/max` - crew member max in gastown rig
- `gastown/witness` - witness agent in gastown rig
- `mayor` - town-level mayor (no rig)
- `beads/refinery` - refinery agent in beads rig

Special cases:
- `mayor` - single segment, town-level
- `gastown/witness` - two segments, rig-level agent
