# Channel Routing Storage Options: Research & Recommendation

**Issue**: gt-epc-dynamic_agent_controlled_channel_routing.1
**Author**: gastown/polecats/furiosa
**Date**: 2026-01-30

## Executive Summary

**Recommendation**: Use the **existing Slack router config infrastructure** (`settings/slack.json`)
with agent-specific overrides, plus optional **runtime env var** (`GT_CHANNEL_MODE`) for
session-level overrides.

This hybrid approach leverages existing infrastructure, requires minimal new code, and follows
established patterns in the codebase.

---

## Options Evaluated

### Option 1: Agent Config Files

**Proposed**: `~/.config/gastown/<agent>.yaml` or similar

**Existing Infrastructure**:
- Codebase uses JSON (not YAML) for all persistent configuration
- Multi-layer config resolution exists: rig-level → town-level → builtin → fallback
- Locations already defined:
  - Town: `{town_root}/settings/config.json` (TownSettings)
  - Rig: `{rig_path}/settings/config.json` (RigSettings)
  - Agents: `{town_root}/settings/agents.json` (RuntimeConfig per agent)
- Wisp layer supports transient per-rig config: `.beads-wisp/config/{rig_name}.json`

**Analysis**:
| Aspect | Assessment |
|--------|------------|
| Persistence | ✅ Files persist across sessions |
| Editability | ✅ JSON easy to hand-edit |
| Version control | ✅ Can be tracked in git |
| Cross-session sync | ⚠️ Requires file sync mechanism (worktrees are isolated) |
| Implementation | ⚠️ Would need new config fields and loading logic |
| Discovery | ⚠️ Agent-specific files scattered across worktrees |

**Verdict**: Infrastructure exists but designed for *execution* config (commands, env vars),
not *preferences*. Would require adding new fields to existing structures.

---

### Option 2: Bead Metadata

**Proposed**: Agent profile beads in HQ with routing preferences

**Existing Infrastructure**:
- Beads use **structured field types**, not arbitrary key-value storage
- Predefined field types: AgentFields, DecisionFields, MRFields, etc.
- Agent beads exist for lifecycle tracking: `{prefix}-{rig}-{role}-{name}`
- Labels provide multi-valued metadata with query capabilities
- Role beads deprecated since Phase 2 (moved to config files)

**Analysis**:
| Aspect | Assessment |
|--------|------------|
| Persistence | ✅ Dolt provides distributed, durable storage |
| Query capabilities | ✅ Label-based filtering, status queries |
| Distribution | ✅ Synced across all agents automatically |
| Implementation | ⚠️ Would need new field type (RoutingPrefsFields) |
| Schema changes | ❌ Adds complexity to bead schema |
| Purpose fit | ❌ Beads designed for work tracking, not preferences |

**Verdict**: Could work but would require adding new structured field type. Beads are
purposefully constrained to work tracking, not configuration storage.

---

### Option 3: Slack User Preferences

**Proposed**: Custom Slack App Home fields for user-editable preferences

**Existing Infrastructure**:
- App Home **NOT implemented** - no event handlers for `app_home_opened`
- Current bot handles: slash commands, buttons, modals, channel events
- User preferences are team-level (routing patterns), not per-user
- Break Out feature creates persistent agent overrides via config

**Analysis**:
| Aspect | Assessment |
|--------|------------|
| User editability | ✅ Intuitive Slack UI |
| Implementation | ❌ Requires building App Home from scratch |
| Portability | ❌ Tied to Slack, not usable without Slack |
| Persistence | ⚠️ Stored in Slack, not in our systems |
| Agent access | ⚠️ Agents would need API calls to read preferences |

**Verdict**: Would require significant new development. Not suitable as primary storage
but could be a future enhancement for overseer UX.

---

### Option 4: Runtime Env Vars

**Proposed**: `GT_CHANNEL_MODE=general|agent|epic|dm`

**Existing Infrastructure**:
Strong precedent for preference env vars:
- `GT_THEME` - CLI color scheme (overrides stored config)
- `GT_AGENT_MODE` - Output compaction (runtime-only)
- `GT_NO_EMOJI` - Disable emoji decorations
- `GT_DECISION_MODE` - Decision presentation mode

**Pattern established**:
```
Priority: env var > config file > default
Scope: Single session only
Style: Boolean flags or enum values
```

**Analysis**:
| Aspect | Assessment |
|--------|------------|
| Simplicity | ✅ Trivial to implement |
| Immediate effect | ✅ Takes effect instantly |
| Persistence | ❌ Lost on session restart |
| Cross-session | ❌ Not shared between sessions |
| Documentation | ✅ Follows established conventions |

**Verdict**: Excellent for session-level overrides but not suitable as primary persistent storage.

---

## Hidden Option 5: Existing Slack Router Config (Recommended)

**Discovery**: The codebase already has robust channel routing infrastructure in
`internal/slackbot/router.go` using `settings/slack.json`.

**Existing Features**:
```go
type Config struct {
    Enabled       bool              // Master switch
    DefaultChannel string           // Fallback
    Channels      map[string]string // Pattern → Channel mapping
    Overrides     map[string]string // Agent-specific overrides
}
```

**Pattern matching with wildcards**:
```json
{
  "gastown/crew/decision_point": "C9999",  // Exact match (highest priority)
  "gastown/polecats/*": "C1111",           // Wildcard pattern
  "*/crew/*": "C2222",                     // Cross-rig by role
  "gastown/*": "C3333"                     // Rig fallback
}
```

**Existing resolution order**: Exact override → Pattern match → Role match → Default

**Analysis**:
| Aspect | Assessment |
|--------|------------|
| Already implemented | ✅ Router, persistence, resolution logic exist |
| Persistence | ✅ JSON config file, survives restarts |
| Extensibility | ✅ Can add new fields (e.g., `routing_mode` per agent) |
| Admin control | ✅ Centralized in settings/slack.json |
| Agent-level | ✅ Already supports per-agent overrides |

---

## Recommendation: Hybrid Approach

### Primary Storage: Extended Slack Router Config

Extend the existing `settings/slack.json` with agent routing mode preferences:

```json
{
  "enabled": true,
  "default_channel": "C0000000000",
  "channels": {
    "gastown/polecats/*": "C1111111111",
    "*/crew/*": "C2222222222"
  },
  "overrides": {
    "gastown/polecats/furiosa": "C3333333333"
  },
  "routing_modes": {
    "gastown/polecats/furiosa": "epic",
    "gastown/crew/jane": "dm",
    "*": "general"
  }
}
```

**Routing mode values**:
- `general` - Use shared workspace channel (current default)
- `agent` - Use dedicated per-agent channel
- `epic` - Route to parent epic's channel
- `dm` - Direct message to overseer

### Session Override: GT_CHANNEL_MODE Env Var

For temporary overrides, support `GT_CHANNEL_MODE` env var:

```bash
# Override for this session only
GT_CHANNEL_MODE=dm gt start polecat furiosa

# Or export for full session
export GT_CHANNEL_MODE=epic
```

**Resolution priority**:
1. `GT_CHANNEL_MODE` env var (if set)
2. `routing_modes[agent]` from slack.json
3. `routing_modes["*"]` default
4. Fallback to `general`

### Implementation Path

1. **Add routing_modes field** to `slack.Config` struct
2. **Extend Router.ResolveChannel()** to check routing mode
3. **Add GT_CHANNEL_MODE support** to resolution logic
4. **CLI command** for agents to set preference: `gt config slack routing-mode agent`

---

## Tradeoffs Summary

| Approach | Pros | Cons |
|----------|------|------|
| **Recommended (slack.json + env)** | Existing infra, proven pattern, minimal code | Admin-controlled (agents can't self-edit) |
| Agent config files | Per-agent persistence | Scattered files, sync issues |
| Bead metadata | Distributed, queryable | Schema creep, wrong abstraction |
| Slack App Home | User-friendly | Major new development, Slack-tied |
| Env vars only | Simple | Not persistent |

---

## Next Steps

1. **Approve this recommendation** → Update parent epic
2. **Implement routing_modes** in slack.json schema
3. **Extend router resolution** to use modes
4. **Add GT_CHANNEL_MODE** env var support
5. **Add CLI commands** for preference management
6. **Document** the routing mode options

---

## References

- `internal/slackbot/router.go` - Existing routing implementation
- `internal/config/env.go` - GT_ env var patterns
- `docs/design/slack-channel-routing.md` - Existing routing design
- Parent epic: gt-epc-dynamic_agent_controlled_channel_routing
