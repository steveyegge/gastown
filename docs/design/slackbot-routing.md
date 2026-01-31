# Slackbot Channel Routing - Architecture Documentation

This document describes the existing channel routing implementation in gtslack,
prepared for integration with the dynamic agent-controlled channel routing epic
(gt-epc-dynamic_agent_controlled_channel_routing).

## Overview

The slackbot uses a multi-layer routing system to determine which Slack channel
should receive notifications. The routing resolution happens at notification time,
with multiple fallback levels.

## Current Resolution Order

### For Decisions (`resolveChannelForDecision`)

**Location:** `internal/slackbot/bot.go:1248`

```
1. Convoy-based channel (if parent issue tracked by convoy)
   └─> ensureEpicChannelExists(convoyTitle)

2. Epic-based channel (if decision has parent epic)
   └─> ensureEpicChannelExists(decision.ParentBeadTitle)

3. Static router config (if pattern matches agent)
   └─> router.Resolve(agent)

4. Dynamic channel creation (if enabled)
   └─> ensureChannelExists(agent)

5. Default channelID
```

### For General Agent Routing (`resolveChannel`)

**Location:** `internal/slackbot/bot.go:1215`

```
1. Static router config (pattern match)
   └─> router.Resolve(agent)

2. Dynamic channel creation (if enabled)
   └─> ensureChannelExists(agent)

3. Default channelID
```

## Key Components

### 1. Static Router (`internal/slack/router.go`)

Pattern-based routing from configuration file (`slack.json`):

```go
type Router struct {
    config      *Config
    patterns    []compiledPattern
}

type RouteConfig struct {
    Pattern   string `json:"pattern"`    // Glob pattern: "gastown/*"
    ChannelID string `json:"channel_id"` // Target channel
    Priority  int    `json:"priority"`   // Higher = checked first
}
```

**Resolution:** `router.Resolve(agent)` returns first matching pattern's channel.

### 2. Dynamic Channel Creation

**Agent channels:** `ensureChannelExists(agent string)`
- Location: `bot.go:1443`
- Naming: `{prefix}-{rig}-{role}` (e.g., `gt-decisions-gastown-polecats`)
- Creates channel if missing, caches ID

**Epic channels:** `ensureEpicChannelExists(epicTitle string)`
- Location: `bot.go:1336`
- Naming: `{prefix}-{slug}` (e.g., `gt-decisions-epic-based-channels`)
- Uses `util.DeriveChannelSlug()` for title→slug conversion

### 3. Auto-Invite (NEW in gt-bsw3m.5)

**Function:** `autoInviteToChannel(channelID string)`
- Location: `bot.go:1504`
- Invites configured users when routing to channels
- Handles `already_in_channel` gracefully

**Configuration:**
```go
Config.AutoInviteUsers []string  // Slack user IDs
```

**CLI:** `-auto-invite=U123,U456` or `SLACK_AUTO_INVITE=U123,U456`

### 4. Channel Caching

```go
channelCache     map[string]string // name → ID
channelCacheMu   sync.RWMutex
```

Cached on first lookup/creation. No TTL (cache lives for bot lifetime).

## Data Flow

```
Decision Created
    │
    ▼
NotifyNewDecision(decision)
    │
    ▼
resolveChannelForDecision(decision)
    │
    ├─[has ParentBeadID + townRoot?]──▶ getTrackingConvoyTitle()
    │                                        │
    │                                        ▼
    │                               ensureEpicChannelExists(convoyTitle)
    │                                        │
    │                                        ▼
    │                               autoInviteToChannel() ◄── NEW
    │
    ├─[has ParentBeadTitle?]──────▶ ensureEpicChannelExists(epicTitle)
    │                                        │
    │                                        ▼
    │                               autoInviteToChannel() ◄── NEW
    │
    ├─[router enabled?]───────────▶ router.Resolve(agent)
    │
    ├─[dynamicChannels enabled?]──▶ ensureChannelExists(agent)
    │                                        │
    │                                        ▼
    │                               autoInviteToChannel() ◄── NEW
    │
    └─[fallback]──────────────────▶ channelID (default)
```

## Extension Points for Dynamic Routing

### Where Agent Preferences Would Plug In

The dynamic routing epic needs to add a new resolution layer:

```
PROPOSED:
resolveChannelForDecision(decision)
    │
    ├─[1. Decision-level override]────▶ decision.ChannelHint (NEW)
    │
    ├─[2. Convoy/Epic routing]────────▶ (existing)
    │
    ├─[3. Agent preference]───────────▶ getAgentChannelPreference() (NEW)
    │       │
    │       ├─ "general" → channelID
    │       ├─ "agent"   → ensureChannelExists(agent)
    │       ├─ "epic"    → ensureEpicChannelExists(parentEpic)
    │       └─ "dm"      → openDMWithOverseer()
    │
    ├─[4. Static router]──────────────▶ (existing)
    │
    ├─[5. Dynamic channels]───────────▶ (existing)
    │
    └─[6. Default]────────────────────▶ (existing)
```

### Implementation Considerations

1. **Preference Storage**: Research task .1 will determine where preferences live
   - Options: agent config, bead metadata, Slack user prefs, env vars

2. **Preference Query**: Need new function `getAgentChannelPreference(agent string)`
   - Must be fast (called per notification)
   - Should cache if hitting external storage

3. **DM Mode**: New capability not currently implemented
   - Need `openDMWithOverseer()` helper
   - Need to know overseer's Slack user ID

4. **Decision Hints**: Allow callers to specify channel
   - Add `ChannelHint` field to Decision struct
   - Useful for workflows that know their target channel

## Static Router Deep Dive

**Location:** `internal/slack/router.go`

The Router provides pattern-based channel routing from configuration. It supports
two configuration backends: file-based (`slack.json`) and beads-backed (`bd config`).

### Configuration Structure

```go
type Config struct {
    Enabled        bool              // Master switch
    DefaultChannel string            // Fallback when no pattern matches
    Channels       map[string]string // Pattern → ChannelID
    Overrides      map[string]string // ExactAgent → ChannelID (highest priority)
    ChannelNames   map[string]string // ChannelID → DisplayName
    ChannelWebhooks map[string]string // ChannelID → WebhookURL
}
```

### Pattern Syntax

Patterns use `/`-delimited segments with `*` wildcard:

| Pattern | Matches | Does NOT Match |
|---------|---------|----------------|
| `gastown/polecats/*` | `gastown/polecats/furiosa` | `gastown/crew/joe` |
| `*/crew/*` | `gastown/crew/joe`, `beads/crew/wolf` | `gastown/polecats/nux` |
| `beads/*` | Nothing (2 segments vs 3) | `beads/crew/wolf` |
| `beads/*/*` | `beads/crew/wolf`, `beads/polecats/slit` | `beads` |

**Key insight:** Segments must match exactly. `*` matches one segment only.

### Resolution Priority

```
1. Exact Override (config.Overrides[agent])
   └─ Created via "Break Out" button in Slack
   └─ Example: "gastown/crew/slack_decisions" → "C0987654321"

2. Pattern Match (sorted by specificity)
   └─ More segments = higher priority
   └─ Fewer wildcards = higher priority
   └─ Alphabetical tie-breaker

3. Default Channel (config.DefaultChannel)
```

### Pattern Sorting Algorithm

Patterns are pre-compiled and sorted for deterministic matching:

```go
func patternLessThan(a, b compiledPattern) bool {
    // 1. More segments = more specific = higher priority
    if len(a.segments) != len(b.segments) {
        return len(a.segments) > len(b.segments)
    }
    // 2. Fewer wildcards = more specific = higher priority
    if countWildcards(a) != countWildcards(b) {
        return countWildcards(a) < countWildcards(b)
    }
    // 3. Alphabetical for determinism
    return a.original < b.original
}
```

**Example sorting:**
```
gastown/polecats/furiosa  (3 segments, 0 wildcards) ← checked first
gastown/polecats/*        (3 segments, 1 wildcard)
*/polecats/*              (3 segments, 2 wildcards)
gastown/*                 (2 segments, 1 wildcard)  ← never matches 3-segment agents
```

### Override System

Overrides are agent-specific channel assignments created dynamically:

```go
// Created when user clicks "Break Out" in Slack
router.AddOverrideWithName(
    "gastown/crew/slack_decisions",  // agent
    "C0987654321",                   // channelID
    "gt-decisions-slack-decisions",  // displayName
)
```

Overrides persist to configuration (file or beads) via `router.Save()`.

### Configuration Loading

Priority order:
1. **Beads config** (`bd config get slack.*`) - distributed, versioned
2. **File config** (`$GT_ROOT/settings/slack.json`) - local fallback

```go
func LoadRouter() (*Router, error) {
    // Try beads first
    if router, err := LoadRouterFromBeads(); err == nil {
        return router, nil
    }
    // Fall back to file
    return LoadRouterFromFile(findConfigPath())
}
```

### Thread Safety

Router uses `sync.RWMutex` for concurrent access:
- `Resolve()` takes read lock
- `AddOverride()`, `Save()` take write lock

### Example Configuration

**File-based (`slack.json`):**
```json
{
  "type": "slack",
  "version": 1,
  "enabled": true,
  "default_channel": "C0123456789",
  "channels": {
    "gastown/polecats/*": "C1111111111",
    "*/crew/*": "C2222222222"
  },
  "overrides": {
    "gastown/crew/slack_decisions": "C3333333333"
  },
  "channel_names": {
    "C0123456789": "#gt-decisions-general",
    "C1111111111": "#gt-decisions-polecats",
    "C2222222222": "#gt-decisions-crew",
    "C3333333333": "#gt-decisions-slack-decisions"
  }
}
```

**Beads-backed:**
```bash
bd config set slack.enabled true
bd config set slack.default_channel C0123456789
bd config set slack.channels '{"gastown/polecats/*": "C1111111111"}'
bd config set slack.overrides '{"gastown/crew/slack_decisions": "C3333333333"}'
```

## Slug Derivation

**Location:** `internal/util/slug.go`

The slug derivation system converts human-readable titles (epic names, convoy titles)
into Slack-safe channel name suffixes.

### Algorithm

```go
func DeriveChannelSlug(title string) string {
    return DeriveChannelSlugWithMaxLen(title, 30)  // Default max 30 chars
}

func DeriveChannelSlugWithMaxLen(title string, maxLen int) string {
    // 1. Lowercase
    slug = strings.ToLower(title)

    // 2. Replace non-alphanumeric with hyphens
    // [^a-z0-9] → "-"

    // 3. Collapse consecutive hyphens
    // "foo--bar" → "foo-bar"

    // 4. Strip leading/trailing hyphens

    // 5. Truncate at word boundary (if > maxLen)
    // Find last hyphen in first maxLen chars
    // Truncate there to avoid mid-word cuts

    return slug
}
```

### Examples

| Input | Output |
|-------|--------|
| `"Ephemeral Polecat Merge Workflow: Rebase-as-Work"` | `ephemeral-polecat-merge` |
| `"Fix bug #123 in parser"` | `fix-bug-123-in-parser` |
| `"This is a very long title that exceeds limit"` | `this-is-a-very-long-title` |
| `"!@#$%^&*()"` (only special chars) | `""` |
| `"---leading and trailing---"` | `leading-and-trailing` |

### Usage in Channel Naming

```go
// In ensureEpicChannelExists:
slug := util.DeriveChannelSlug(epicTitle)    // "ephemeral-polecat-merge"
channelName := b.channelPrefix + "-" + slug  // "gt-decisions-ephemeral-polecat-merge"

// Additional sanitization for Slack:
channelName = strings.ToLower(channelName)
channelName = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(channelName, "-")
channelName = regexp.MustCompile(`-+`).ReplaceAllString(channelName, "-")
channelName = strings.Trim(channelName, "-")

// Slack limit: 80 chars max
if len(channelName) > 80 {
    channelName = channelName[:80]
}
```

## Channel Caching

**Location:** `internal/slackbot/bot.go`

The bot caches channel name→ID mappings to avoid repeated Slack API lookups.

### Data Structure

```go
type Bot struct {
    channelCache   map[string]string  // name → ID
    channelCacheMu sync.RWMutex       // Thread-safe access
}
```

### Cache Operations

```go
// Read (with read lock)
b.channelCacheMu.RLock()
if cachedID, ok := b.channelCache[channelName]; ok {
    b.channelCacheMu.RUnlock()
    return cachedID, nil
}
b.channelCacheMu.RUnlock()

// Write (with write lock)
func (b *Bot) cacheChannel(name, id string) {
    b.channelCacheMu.Lock()
    b.channelCache[name] = id
    b.channelCacheMu.Unlock()
}
```

### Cache Population

Cache is populated in three scenarios:

1. **Channel found by name lookup:**
   ```go
   channelID, err := b.findChannelByName(channelName)
   if err == nil && channelID != "" {
       b.cacheChannel(channelName, channelID)  // ← cached
       return channelID, nil
   }
   ```

2. **Channel created:**
   ```go
   channel, err := b.client.CreateConversation(...)
   if err == nil {
       b.cacheChannel(channelName, channel.ID)  // ← cached
       return channel.ID, nil
   }
   ```

3. **"name_taken" recovery:**
   ```go
   if strings.Contains(err.Error(), "name_taken") {
       channelID, _ := b.findChannelByName(channelName)
       if channelID != "" {
           b.cacheChannel(channelName, channelID)  // ← cached
           return channelID, nil
       }
   }
   ```

### Cache Characteristics

| Property | Value |
|----------|-------|
| Lifetime | Bot process lifetime (no TTL) |
| Eviction | None (grows unbounded) |
| Persistence | None (memory only) |
| Thread safety | RWMutex protected |

### Potential Improvements

For the dynamic routing epic, consider:
- **TTL**: Add expiration for renamed/deleted channels
- **Size limit**: Bound cache size with LRU eviction
- **Warm-up**: Pre-populate cache on startup
- **Invalidation**: React to Slack channel events

## Files Reference

| File | Purpose |
|------|---------|
| `internal/slackbot/bot.go` | Main routing logic, channel helpers |
| `internal/slack/router.go` | Static pattern-based routing |
| `internal/util/slug.go` | Channel slug derivation |
| `cmd/gtslack/main.go` | Config loading, flag parsing |

## Related Work

- **gt-bsw3m.5**: Added auto-invite functionality (completed)
- **gt-bsw3m**: Epic-based channel routing (blocked on beads schema)
- **gt-epc-dynamic_agent_controlled_channel_routing**: This epic
