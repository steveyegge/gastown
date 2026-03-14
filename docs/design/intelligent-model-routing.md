# Implementation Plan: Intelligent Agent Routing

**RFC:** https://github.com/steveyegge/gastown/discussions/2531
**Status:** Implementation Plan
**Date:** 2026-03-14

---

## Current State Analysis

Gas Town already has significant agent-routing infrastructure:

| Capability | Where | How |
|-----------|-------|-----|
| Cost tiers (`standard`/`economy`/`budget`) | `internal/config/cost_tier.go` | Hardcoded role→agent maps; `ApplyCostTier()` writes to `TownSettings.RoleAgents` |
| Role-based agent resolution | `internal/config/loader.go` | `ResolveRoleAgentConfig()` — checks rig settings → town settings → cost tier → default |
| Per-rig overrides | `<rig>/settings/config.json` | `RigSettings.RoleAgents` / `WorkerAgents` |
| CLI agent override | `gt sling --agent <name>` | Passed through `SlingSpawnOptions.Agent` → `SessionStartOptions.Agent` |
| Agent presets | `internal/config/agents.go` | 10 built-in presets (claude, gemini, codex, cursor, etc.) |
| Custom agent registry | `settings/agents.json` (town + rig) | `RuntimeConfig` with command, args, env |
| `gt config agent` CLI | `internal/cmd/config.go` | `list`, `get`, `set`, `remove` subcommands for custom agents |
| `gt config cost-tier` CLI | `internal/cmd/config.go` | Switch between standard/economy/budget presets |
| Witness zombie detection | `internal/witness/handlers.go` | Detects dead sessions, dead agents, stuck polecats, spawn storms |
| Spawn circuit breaker | `internal/witness/spawn_count.go` | `MaxBeadRespawns` threshold → SPAWN_BLOCKED mail to mayor |
| Mail system | `gt mail send/inbox/read` | Dolt-backed persistent messaging between all agents |

**Key insight:** Gas Town already has agents as a first-class concept — presets that encapsulate command, args, model, and environment. We don't route *models*; we route *agents*. Tiers map to ordered lists of agent names, not provider/model tuples.

**What's missing** (what the RFC adds):
1. **Agent tiers with selection strategies** — abstract `small`/`medium`/`large`/`reasoning` tiers that map to ordered lists of agent presets, with per-tier selection strategies
2. **Agent-driven task-complexity routing** — dynamic tier selection based on task analysis; Go code handles agent selection within tiers
3. **Witness→Router feedback via mail** — witness sends AGENT_FAILURE mail to the router when it detects agent-level failures
4. **Router→Mayor escalation via mail** — router sends ROUTING_FAILED mail to the mayor when no routable agents remain

---

## Open Question Decisions

### Q1: Router opt-in or opt-out?
**Decision: Opt-in (off by default)**

The router agent adds latency to every dispatch. Phase 1 (tier config + role defaults) delivers most of the value without it. Enable via config:
```json
{ "router": { "enabled": true } }
```

### Q2: Router context depth?
**Decision: Minimal — bead title + description + role + tier names/descriptions**

The router performs a classification task, not planning. Bead title + body is sufficient signal for tier selection. We explicitly do NOT try to judge "how well" a task was completed — there is no judge for that.

The router agent sees only tier names and descriptions. It does NOT see agent lists, exclusion state, or AGENT_FAILURE mail. Those are handled by Go code after the router returns a tier (see §3.3 for the full separation of concerns).

**v1 router agent input:**
- Bead title + description (from `bd show`)
- Current role (`polecat`, `crew`, etc.)
- Available tier names and their descriptions

**Handled by Go code (not the router agent):**
- AGENT_FAILURE exclusions (from `ExclusionCache`)
- Agent selection within the chosen tier (priority/round-robin)
- Tier fallback when all agents in a tier are excluded

### Q3: No confidence threshold
**Decision: No confidence field — the router always commits to a tier**

The router should be capable enough for a routing decision. If it routes to an underpowered agent and the polecat gets stuck, we can't easily judge whether that was a bad routing decision vs a genuinely hard task. The failure path is:

1. Router picks a tier → polecat spawns with an agent from that tier
2. Polecat gets stuck / crashes / runs out of credits
3. **Witness** detects the failure (zombie detection, spawn circuit breaker)
4. Witness sends `AGENT_FAILURE` mail to `router/` with agent name + reason
5. On re-dispatch, Go routing code reads the mail, excludes the failed agent, and selects an alternative

The witness is the judge, not the router. The router's structured response is simplified:
```json
{ "tier": "large", "rationale": "cross-cutting refactor touching auth + DB layers" }
```

No `confidence` field. The router always returns a tier. If the router agent fails (timeout, crash), use `fallback_tier` from config.

### Q4: Agent fallback on failure — witness-informed via mail
**Decision: Witness sends AGENT_FAILURE mail → router excludes those agents**

When an agent is unavailable (detected by the witness — credit exhaustion, rate limit, network failure, repeated polecat crashes with a specific agent), the witness sends mail to `router/`:

```
gt mail send router/ -s "AGENT_FAILURE: claude-sonnet" -m "agent: claude-sonnet\nreason: credit_exhaustion\nbead: xyz-123\nrig: gastown"
```

**At dispatch time, the Go routing code:**
1. Reads the `router/` inbox for `AGENT_FAILURE` mail newer than 30 minutes (configurable TTL)
2. Builds a set of excluded agent names from those messages (`ExclusionCache`)
3. Skips excluded agents when selecting from a tier's agent list
4. If all agents in a tier are excluded, moves **up one tier** (e.g., `medium` → `large`)
5. If no routable agents remain at any tier, sends `ROUTING_FAILED` mail to `mayor/` and uses `fallback_tier`

**Note:** This is Go code in the dispatch path, not the router agent. The router (Phase 3) only picks a tier — all agent selection, exclusion, and fallback logic is deterministic Go code.

**Why mail instead of a custom store:**
- Uses Gas Town's existing communication infrastructure — no new persistence layer
- Mail is already Dolt-backed, survives agent restarts, and has built-in read/unread tracking
- Consistent with how all other inter-agent communication works (SPAWN_BLOCKED, HELP, MERGE_FAILED)
- The witness already knows how to send mail; no new integration point needed
- The TTL is just a recency filter on mail timestamps — trivial to implement

**Tier ordering** is defined in config (see §1.1). Moving "up" means toward higher capability. Go code uses the tier ordering for automatic fallback when a tier's agents are all excluded.

### Q5: Cost attribution for router calls?
**Decision: Infrastructure overhead — attributed to the same cost bucket as the Mayor**

Router calls are tiny (small context, structured output). Track as a separate OTEL metric (`gt.router.calls`, `gt.router.latency_ms`) but attribute costs to infrastructure, not individual beads.

### Q6: Multi-agent tier resolution — per-tier selection strategy
**Decision: Configurable per-tier — `priority` (default) or `round-robin`**

Each tier specifies its own selection strategy:

```json
{
  "agent_tiers": {
    "medium": {
      "description": "Standard feature work, multi-file changes, bug fixes",
      "selection": "round-robin",
      "agents": ["claude-sonnet", "gemini"]
    },
    "large": {
      "description": "Cross-cutting refactors, new subsystem integration",
      "selection": "priority",
      "agents": ["claude-opus"]
    },
    "reasoning": {
      "description": "Deep debugging, architecture decisions, tricky algorithms",
      "selection": "priority",
      "agents": ["claude-reasoning"]
    }
  }
}
```

- **`priority`** (default): First available agent in list order wins
- **`round-robin`**: Cycle through agents in list order, skipping unavailable ones

When all agents in a tier are exhausted (all excluded via AGENT_FAILURE mail or all failed), Go code moves **up one tier** per `TierOrder` and tries the next tier's agents.

### Q7: Interaction with `gt config agent`
**Decision: Two command paths — `gt config agent tiers` for configuration, `gt agent tier` for runtime**

The existing CLI structure is:
```
gt config
├── agent (list/get/set/remove)     ← custom agent presets
├── cost-tier [standard|economy|budget]  ← quick presets
├── default-agent [name]
├── get/set <key> <value>
```

New structure adds tier config under `gt config agent` and a runtime command at `gt agent tier`:

```
gt config
├── agent                           ← custom agent presets
│   ├── list/get/set/remove         ← (unchanged)
│   └── tiers                       ← NEW: tier configuration
│       ├── show                    ← display current tier config
│       ├── set <tier> [flags]      ← create/update a tier
│       ├── remove <tier>           ← remove a tier
│       ├── set-role <role> <tier>  ← map role to tier
│       ├── add-agent <tier> <agent> ← append agent to tier
│       ├── remove-agent <tier> <agent> ← remove agent from tier
│       ├── set-order <tier...>     ← set tier ordering
│       └── init                    ← initialize default tier config
├── router                          ← NEW: router agent management (Phase 3)
│   ├── show                        ← display router config + recent AGENT_FAILURE mail
│   ├── set <agent>                 ← set router agent and enable routing
│   ├── disable                     ← disable router
│   └── test <bead-id>              ← dry-run routing decision
├── cost-tier [standard|economy|budget]  ← quick presets (unchanged)
├── default-agent [name]            ← (unchanged)
├── get/set <key> <value>           ← (unchanged)

gt agent                            ← NEW: runtime tier operations
├── tier                            ← runtime view of tiers
│   ├── list                        ← show tiers with real-time availability
│   └── list --available            ← only tiers with live agents
```

**Why two paths:**
- `gt config agent tiers` — **configuration**: defines tiers, assigns agents, sets selection strategies. Writes to `settings/config.json`. Run once, rarely changed.
- `gt agent tier` — **runtime**: shows current tier availability factoring in AGENT_FAILURE exclusions. Reads config + `ExclusionCache`. Run frequently to check routing health.

**Precedence (highest → lowest):**
1. `--agent` flag on `gt sling` (explicit agent override — bypasses tiers entirely)
2. `--tier` flag on `gt sling` (explicit tier override — picks agent from tier's list)
3. Rig-level `role_agents` / `worker_agents` (rig settings)
4. Town-level `role_agents` / `crew_agents` (town settings, written by `gt config agent`)
5. Router agent decision (if enabled, Phase 3)
6. Agent tier `role_defaults` (Phase 1)
7. Cost tier preset (standard/economy/budget) — writes to `role_agents`, so effectively layer 4
8. Town `default_agent` (final fallback)

Note: `gt config cost-tier` writes to `role_agents` (layer 4), so it takes precedence over `agent_tiers.role_defaults` (layer 6). Users who switch to the tier system can stop using cost tiers — both systems coexist but tier config is more flexible.

---

## Architecture: Witness→Router Feedback Loop

This uses Gas Town's existing mail system. No custom stores or persistence layers.

```
                    ┌─────────────┐
                    │   Router    │
                    │  (Phase 3)  │
                    │             │
                    │ reads mail: │◄──── AGENT_FAILURE mail (< 30min)
                    │ router/     │
                    └──────┬──────┘
                           │ no routable agents?
                           │
                    ┌──────▼──────┐
                    │   Mayor     │◄──── ROUTING_FAILED mail
                    │  mayor/     │
                    └─────────────┘

                    ┌─────────────┐
                    │   Witness   │
                    │  (existing) │
                    └──────┬──────┘
                           │ detects agent failure
                           │
                    ┌──────▼──────┐
                    │  gt mail    │
                    │  send       │──── AGENT_FAILURE → router/
                    │  router/    │
                    └─────────────┘
                           ▲
                    ┌──────┴──────┐
                    │   Polecat   │
                    │  (failure)  │
                    └─────────────┘
```

### Mail Protocol: AGENT_FAILURE

**Sender:** Witness (any rig)
**Recipient:** `router/`
**Subject:** `AGENT_FAILURE: <agent-name>`

**Body format** (structured, machine-parseable):
```
agent: claude-sonnet
reason: credit_exhaustion
bead: xyz-123
rig: gastown
polecat: gastown/polecat-abc
timestamp: 2026-03-14T06:30:00Z
```

**Reasons:**
- `credit_exhaustion` — billing/quota/credit errors in exit output
- `rate_limit` — 429, "too many requests", "resource_exhausted"
- `network` — connection refused, timeout, ECONNRESET
- `crash_loop` — same bead respawned ≥2 times with same agent, no progress

### Mail Protocol: ROUTING_FAILED

**Sender:** Router (at dispatch time)
**Recipient:** `mayor/`
**Subject:** `ROUTING_FAILED: no routable agents for <tier>`

**Body format:**
```
requested_tier: medium
bead: xyz-123
excluded_agents:
  - claude-sonnet (credit_exhaustion, 15min ago)
  - gemini (rate_limit, 5min ago)
fallback_used: large
action: dispatched with fallback tier
```

If even the fallback tier has no available agents:
```
requested_tier: medium
bead: xyz-123
excluded_agents: [all]
fallback_tier: large (also unavailable)
action: BLOCKED — no routable agents, awaiting mayor intervention
```

### Router Mail Processing at Dispatch Time

The router is not a persistent agent — it runs as a function call during dispatch. It maintains an **in-memory exclusion cache** that is built once per dispatch cycle from mail, then reused for all tier resolutions in that cycle (avoids re-reading mail for every resolution call within the same dispatch).

**Startup recovery:**
On process startup (e.g., after mayor restart, compaction, or new session), the router re-reads **all** AGENT_FAILURE mail within the TTL window — including previously-read messages. This recovers exclusion state that was lost when the in-memory cache was discarded. Without this, a mayor restart during an agent outage would cause the router to immediately route back to the failing agent.

1. Read `router/` inbox via Go mail API — **both read and unread** AGENT_FAILURE messages
2. Filter by timestamp: only messages within configured TTL (default 30 min)
3. Parse `agent` + `reason` from message bodies
4. Build in-memory `ExclusionCache` (a `map[string]ExclusionEntry` keyed by agent name)
5. Cache is now warm — subsequent dispatch cycles only need to check for **unread** messages and merge them in

**Dispatch cycle (after startup):**
1. Check `router/` inbox for **unread** AGENT_FAILURE messages only
2. Parse and merge new exclusions into the existing in-memory `ExclusionCache`
3. Evict expired entries (older than TTL) from the cache
4. **Mark new messages as read** — prevents re-processing on next cycle
5. Use `ExclusionCache` for all tier resolutions in this dispatch cycle

**Mark-as-read is important:** Without it, the `router/` inbox grows unboundedly with stale AGENT_FAILURE mail. The TTL filter would still work, but the inbox becomes noisy for `gt status` and `gt config router show`. Marking as read keeps the inbox clean — only unread AGENT_FAILURE messages represent failures the router hasn't yet acted on.

**Why read+unread on startup, unread-only after:** The startup pass recovers state from before the restart. Once the cache is warm, only new (unread) messages need processing. This keeps dispatch-time mail reads cheap — typically zero or one message — while ensuring restarts don't cause amnesia.

```go
// ExclusionCache holds parsed AGENT_FAILURE state for one dispatch cycle.
// Built once from mail, reused for all tier resolutions, then discarded.
type ExclusionCache struct {
    Entries map[string]ExclusionEntry // key: agent name
    BuiltAt time.Time
}

// ExclusionEntry is a single excluded agent parsed from AGENT_FAILURE mail.
type ExclusionEntry struct {
    Agent      string
    Reason     string
    ReportedAt time.Time
    MailID     string // for mark-as-read
}

// BuildExclusionCache reads router/ inbox (read+unread AGENT_FAILURE mail within TTL),
// builds the full exclusion cache, and marks new messages as read.
// Used on startup to recover state from before a restart.
func BuildExclusionCache(townRoot string, ttl time.Duration) *ExclusionCache

// RefreshExclusionCache reads only unread AGENT_FAILURE mail from router/ inbox,
// merges new exclusions into the existing cache, evicts expired entries, and
// marks new messages as read. Used on subsequent dispatch cycles after startup.
func (c *ExclusionCache) Refresh(townRoot string, ttl time.Duration)

// IsExcluded checks if an agent is in the cache.
func (c *ExclusionCache) IsExcluded(agent string) bool

// ExcludedSet returns the set of agent name strings for tier resolution.
func (c *ExclusionCache) ExcludedSet() map[string]bool
```

**TTL configuration:**
```json
{
  "router": {
    "failure_ttl_minutes": 30
  }
}
```

### `gt status` Integration

The router's state is visible in `gt status` output, giving operators a single-pane view of routing health.

**`gt status` additions (Phase 2+):**

```
🏭 Gas Town Status
  ...existing sections...

📡 Agent Router
  Status:     enabled (agent: qwen-local)
  Fallback:   medium
  Failure TTL: 30m

  Agent Status:
    ✓ claude-haiku       available
    ✓ claude-sonnet      available
    ✗ claude-opus        excluded (credit_exhaustion, 12m ago, expires in 18m)
    ✓ claude-reasoning   available
    ✓ gemini             available

  Recent AGENT_FAILURE mail (router/ inbox):
    1. claude-opus — credit_exhaustion — 12m ago (read ✓)
    2. codex — rate_limit — 45m ago (expired, read ✓)

  Routing summary (last 1h):
    Dispatches: 8 | Fallbacks: 1 | Failures: 0
```

When router is disabled:
```
📡 Agent Router
  Status: disabled
```

When no tier config exists:
```
📡 Agent Router
  Status: not configured (no agent_tiers in settings)
```

**Implementation:** `gt status` already aggregates data from multiple subsystems. Add a `routerStatus()` function that:
1. Loads `RouterConfig` from town settings
2. Reads `router/` inbox (both read and unread AGENT_FAILURE messages)
3. Applies TTL filter to determine which exclusions are active vs expired
4. Cross-references active exclusions against the tier config's agent lists
5. Formats the agent status table (✓/✗)

**File:** `internal/cmd/status.go` — add router status section

This gives operators immediate visibility into:
- Whether the router is active
- Which agents are currently excluded and why
- How long until exclusions expire
- Whether routing failures have been escalated to the mayor

---

## Implementation Phases

### Phase 1: Agent Tiers + Role Defaults

**Goal:** Implement a user-configurable tier system with multi-agent support, per-tier selection strategies, and role defaults. This formalizes what users in #1642 are doing manually.

**This phase alone closes most of the gap described in the RFC.**

#### 1.1 New Config Schema

**File:** `internal/config/agent_tier.go` (new)

```go
// AgentTier defines a capability tier with an ordered list of agent preset names.
type AgentTier struct {
    Description string   `json:"description"` // human-readable purpose — fed to the router agent for tier selection
    Agents      []string `json:"agents"`      // ordered list of agent preset names
    Selection   string   `json:"selection"`   // "priority" (default) or "round-robin"
    Fallback    bool     `json:"fallback"`    // default: true — fall back to higher tier on failure
}

// AgentTierConfig holds the full tier routing configuration.
type AgentTierConfig struct {
    Tiers        map[string]*AgentTier `json:"tiers"`
    TierOrder    []string              `json:"tier_order"`      // capability ordering: ["small", "medium", "large", "reasoning"]
    RoleDefaults map[string]string     `json:"role_defaults"`   // role → tier name
}
```

**Key design decisions:**
- **JSON, not YAML** — the entire config system uses JSON; no new dependencies
- **Agents are just names** — each entry in `Agents` references an agent preset (built-in or custom from `settings/agents.json`). The preset already encapsulates command, args, model, env — no need to re-specify provider/model at the tier level
- **`Description` is for the router agent** — the router sees tier names + descriptions when classifying tasks. This lets users define intent-based tiers (e.g., `"complex-work": { "description": "Pick this for tasks with significant design work still to do" }`) beyond generic size labels. The description is NOT used for mechanical tier resolution — only for the Phase 3 router prompt
- **Tiers are string-keyed, not an enum** — users can define custom tiers beyond small/medium/large/reasoning (e.g., "complex-work", "local", "vision", "code-review")
- **`TierOrder` is explicit** — defines the capability ordering. "Up one tier" means moving toward the end of this list. Required for the router's fallback behavior
- **`Selection` per tier** — `priority` or `round-robin`, not global. A `reasoning` tier with one agent uses `priority`; a `medium` tier with multiple agents might use `round-robin`

#### 1.2 Default Tier Configuration

**File:** `internal/config/agent_tier_defaults.go` (new)

Provides a sensible out-of-the-box configuration matching the community convention from #1642:

```go
func DefaultAgentTierConfig() *AgentTierConfig {
    return &AgentTierConfig{
        Tiers: map[string]*AgentTier{
            "small": {
                Description: "Mechanical changes, single-file fixes, formatting, simple renames",
                Agents:      []string{"claude-haiku"},
                Selection:   "priority",
                Fallback:    true,
            },
            "medium": {
                Description: "Standard feature work, multi-file changes within one package, bug fixes requiring investigation",
                Agents:      []string{"claude-sonnet"},
                Selection:   "priority",
                Fallback:    true,
            },
            "large": {
                Description: "Cross-cutting changes across multiple packages, significant refactors, new subsystem integration",
                Agents:      []string{"claude-opus"},
                Selection:   "priority",
                Fallback:    true,
            },
            "reasoning": {
                Description: "Complex debugging requiring deep analysis, architecture decisions, tricky algorithms, security-sensitive changes",
                Agents:      []string{"claude-reasoning"},
                Selection:   "priority",
                Fallback:    false, // reasoning is the top — nowhere to fall back to
            },
        },
        TierOrder:    []string{"small", "medium", "large", "reasoning"},
        RoleDefaults: map[string]string{
            "mayor":    "large",
            "crew":     "large",
            "polecat":  "medium",
            "witness":  "small",
            "deacon":   "small",
            "dogs":     "small",
            "refinery": "medium",
        },
    }
}
```

#### 1.3 Tier → RuntimeConfig Resolution

**File:** `internal/config/agent_tier.go`

```go
// ResolveTierToRuntimeConfig resolves a tier name to a RuntimeConfig.
// excludedAgents is a set of agent names to skip (from AGENT_FAILURE mail).
// Uses the tier's selection strategy (priority or round-robin).
// If all agents in the tier are unavailable and fallback is enabled,
// moves up one tier (per TierOrder).
func (tc *AgentTierConfig) ResolveTierToRuntimeConfig(
    tierName string,
    excludedAgents map[string]bool, // nil OK — no exclusions
) (*RuntimeConfig, error)

// ResolveTierForRole returns the tier name assigned to a role via role_defaults.
// Returns empty string if the role has no tier assignment.
func (tc *AgentTierConfig) ResolveTierForRole(role string) string

// UpOneTier returns the next tier up in TierOrder, or "" if already at top.
func (tc *AgentTierConfig) UpOneTier(tierName string) string

// TierNames returns tier names in capability order.
func (tc *AgentTierConfig) TierNames() []string
```

**Resolution logic:**
1. Look up tier by name in `Tiers` map
2. Filter agents: skip any whose name is in `excludedAgents`
3. Select agent based on `Selection` strategy:
   - `priority`: first available in list order
   - `round-robin`: next in cycle, tracked per-tier via atomic counter
4. Resolve agent name to `RuntimeConfig` via `GetAgentPresetByName(agent)` → `RuntimeConfigFromPreset()`
5. If no agents available and `Fallback` is true → call `UpOneTier()` and recurse
6. If at top tier with no agents → return error

#### 1.4 Integration with Existing Config System

**File:** `internal/config/types.go` — add to `TownSettings`:

```go
type TownSettings struct {
    // ... existing fields ...
    AgentTiers *AgentTierConfig `json:"agent_tiers,omitempty"` // Tier-based routing config
}
```

**File:** `internal/config/loader.go` — modify `resolveRoleAgentConfigCore()`:

Insert tier resolution between cost tier lookup and default agent fallback:

```
Current: rig role_agents → town role_agents → cost tier → default_agent
New:     rig role_agents → town role_agents → cost tier → agent tier role_defaults → default_agent
```

The cost tier system (`standard`/`economy`/`budget`) remains functional. Cost tiers write to `RoleAgents`, which has higher precedence than `agent_tiers.role_defaults`.

#### 1.5 Backward Compatibility with Cost Tiers

The existing `CostTier` system is **preserved as-is**. It writes directly to `RoleAgents`, which takes precedence over `agent_tiers.role_defaults`.

**Coexistence:** Users currently on `economy` or `budget` tiers continue working unchanged. Users who want the new tier system configure `agent_tiers` in their settings. Both systems can be active simultaneously — cost tiers effectively override tier defaults for the roles they manage.

**Long-term:** Cost tiers could be reimplemented as named presets that generate an `AgentTierConfig`, but this is not required for Phase 1.

#### 1.6 CLI Commands

**`gt config agent tiers`** — manage tier configuration

```bash
# Initialize default tier config (creates agent_tiers in settings if absent)
gt config agent tiers init

# Show current tier configuration (tiers, agents, role defaults, selection strategies)
gt config agent tiers show

# Create/update a tier (with description for the router agent)
gt config agent tiers set large --agent claude-opus --description "Cross-cutting refactors, new subsystem integration"
gt config agent tiers set medium --agent claude-sonnet --selection round-robin --description "Standard feature work, multi-file changes"
gt config agent tiers set reasoning --agent claude-reasoning --description "Deep debugging, architecture decisions, tricky algorithms"
gt config agent tiers set complex-work --agent claude-opus --description "Pick this when there is significant design work still to do"

# Remove a tier
gt config agent tiers remove local

# Map a role to a tier
gt config agent tiers set-role polecat medium
gt config agent tiers set-role witness small

# Add an agent to an existing tier (appends to list)
gt config agent tiers add-agent medium gemini
gt config agent tiers add-agent medium codex

# Remove an agent from a tier
gt config agent tiers remove-agent medium gemini

# Set tier ordering (defines what "up one tier" means)
gt config agent tiers set-order small medium large reasoning

# Enable/disable fallback for a tier
gt config agent tiers set reasoning --fallback=false
```

**`gt agent tier list`** — show tiers with real-time availability

```bash
# List all tiers with their current availability status
gt agent tier list

# Example output:
# Agent Tiers (4 defined, 3 available)
#
#   small      "Mechanical changes, single-file fixes"
#              Agents: claude-haiku (✓)
#              Selection: priority
#
#   medium     "Standard feature work, multi-file changes"
#              Agents: claude-sonnet (✓), gemini (✓)
#              Selection: round-robin
#
#   large      "Cross-cutting refactors, new subsystem integration"
#              Agents: claude-opus (✗ credit_exhaustion, 12m ago)
#              Selection: priority
#              ⚠ No available agents — will fall back to reasoning
#
#   reasoning  "Deep debugging, architecture decisions, tricky algorithms"
#              Agents: claude-reasoning (✓)
#              Selection: priority

# List only tiers that have at least one available agent
gt agent tier list --available
```

This command reads the `ExclusionCache` (from AGENT_FAILURE mail) and cross-references it with tier config to show real-time availability. Tiers with all agents excluded are flagged with their fallback target.

**Files:**
- `internal/cmd/config_agent_tiers.go` (new) — `gt config agent tiers` subcommands
- `internal/cmd/agent_tier.go` (new) — `gt agent tier` subcommands

**Interaction between the three agent commands:**
- `gt config agent` manages **agent presets** (what command to run, what args, what model)
- `gt config agent tiers` manages **tier definitions** (which agents for each capability level)
- `gt agent tier` shows **runtime availability** (which tiers have live agents right now)
- `gt config cost-tier` is a **quick preset** that writes to `role_agents` (shortcut for common patterns)

A tier references agents by name. First define the agent: `gt config agent set my-ollama "ollama run ..."`, then add it to a tier: `gt config agent tiers add-agent medium my-ollama`.

#### 1.7 Files Changed (Phase 1)

| File | Change |
|------|--------|
| `internal/config/agent_tier.go` | **New** — `AgentTier` (with `Description`), `AgentTierConfig` types, resolution logic, round-robin counter |
| `internal/config/agent_tier_defaults.go` | **New** — `DefaultAgentTierConfig()` with default descriptions |
| `internal/config/agent_tier_test.go` | **New** — unit tests: resolution, fallback chains, round-robin, selection strategies, precedence |
| `internal/config/types.go` | **Modified** — add `AgentTiers *AgentTierConfig` field to `TownSettings` |
| `internal/config/loader.go` | **Modified** — integrate tier resolution into `resolveRoleAgentConfigCore()` |
| `internal/config/loader_test.go` | **Modified** — test tier integration in resolution chain |
| `internal/cmd/config_agent_tiers.go` | **New** — `gt config agent tiers` CLI command tree (including `--description` flag) |
| `internal/cmd/agent_tier.go` | **New** — `gt agent tier list` command with runtime availability |
| `internal/cmd/config.go` | **Modified** — register `tiers` subcommand under `config agent` |
| `internal/cmd/agent.go` | **New** — `gt agent` command with `tier` subcommand |

---

### Phase 2: Mayor Override API + Witness Agent Failure Mail

**Goal:** Add `--tier` flag to `gt sling` for explicit tier override. Add AGENT_FAILURE mail protocol so the witness can inform the router about agent failures.

#### 2.1 Spawn Interface Changes

**File:** `internal/cmd/polecat_spawn.go`

Extend `SlingSpawnOptions`:
```go
type SlingSpawnOptions struct {
    // ... existing fields ...
    Agent      string // existing: agent preset override (e.g., "gemini", "codex")
    AgentTier  string // NEW: tier override (e.g., "large", "small", "reasoning")
}
```

**Semantics:**
- `--agent codex` → use the codex agent preset directly (existing behavior, unchanged)
- `--tier large` → resolve "large" via `AgentTierConfig`, pick an agent from the tier's list
- Both specified → error (mutually exclusive)
- Neither specified → normal resolution chain

#### 2.2 CLI Flag

**File:** `internal/cmd/sling.go`

```go
slingCmd.Flags().StringVar(&slingTier, "tier", "",
    "Override agent tier for this sling (e.g., small, medium, large, reasoning)")
```

#### 2.3 Resolution in Session Start

**File:** `internal/polecat/session_manager.go`

In `SessionManager.Start()`, add tier resolution before agent override handling:

```go
if opts.AgentTier != "" {
    tierConfig := config.LoadAgentTierConfig(townRoot)
    cache := routing.BuildExclusionCache(townRoot, routerFailureTTL) // reads + marks read
    rc, err := tierConfig.ResolveTierToRuntimeConfig(opts.AgentTier, cache.ExcludedSet())
    if err != nil {
        // No routable agents — send ROUTING_FAILED mail to mayor
        routing.SendRoutingFailedMail(townRoot, opts.AgentTier, bead.ID, cache)
        return fmt.Errorf("tier override %q: %w", opts.AgentTier, err)
    }
    runtimeConfig = rc
} else if opts.Agent != "" {
    // existing agent override path
}
```

#### 2.4 Bead Agent Tracking

**The witness needs to know which agent was used when a polecat fails.** The resolved agent name is stored on the bead itself and updated at spawn time.

**File:** `internal/beads/schema.go` — add field to bead record:

```go
type BeadRecord struct {
    // ... existing fields ...
    ResolvedAgent string `json:"resolved_agent,omitempty"` // agent preset used for this bead's polecat
}
```

**At spawn time** (in `SpawnPolecatForSling()` or `SessionManager.Start()`), after resolving the final `RuntimeConfig`:

```go
// Record which agent was used on the bead for witness failure tracking
if err := bd.UpdateBead(bead.ID, map[string]string{
    "resolved_agent": runtimeConfig.ResolvedAgent,
}); err != nil {
    log.Warn("failed to record resolved_agent on bead", "err", err, "bead", bead.ID)
}
```

**The witness reads this** when classifying failures. Instead of needing to infer the agent from polecat metadata files, it reads the bead's `resolved_agent` field directly:

```go
agentName := agentBead.ResolvedAgent
if agentName == "" {
    // Legacy bead without agent tracking — can't classify as agent failure
    return nil
}
```

**Why the bead, not a polecat metadata file:**
- Beads are Dolt-backed and survive polecat cleanup — the data is available even after `gt polecat nuke`
- The witness already loads the bead record for zombie detection — no additional query needed
- `bd show <id>` displays the agent used, giving operators immediate visibility
- Agent history per bead enables future analysis (e.g., "which agent was used for each attempt?")

**File changes:**
| File | Change |
|------|--------|
| Dolt schema (beads DB) | **Modified** — add `resolved_agent` column |
| `internal/beads/schema.go` | **Modified** — add `ResolvedAgent` field to `BeadRecord` |
| `internal/cmd/polecat_spawn.go` | **Modified** — update bead with `resolved_agent` at spawn time |

#### 2.5 Agent Failure Mail — Reading

**File:** `internal/routing/mail.go` (new)

```go
package routing

// BuildExclusionCache reads the router/ inbox for AGENT_FAILURE mail within TTL,
// parses agent names and reasons from message bodies, marks processed messages as read,
// and returns an in-memory ExclusionCache for use during this dispatch cycle.
func BuildExclusionCache(townRoot string, ttl time.Duration) *ExclusionCache

// SendRoutingFailedMail sends a ROUTING_FAILED message to the mayor
// when no routable agents are available for a tier.
func SendRoutingFailedMail(townRoot, tier, beadID string, cache *ExclusionCache) {
    // gt mail send mayor/ -s "ROUTING_FAILED: no routable agents for <tier>" -m "..."
}
```

#### 2.6 Witness Integration — Agent Failure Detection and Mail

The witness detects polecat failures and classifies whether they're agent-level (provider down, credits exhausted) vs task-level (genuinely hard problem). **The witness reads the agent name from the bead's `resolved_agent` field** (written at spawn time — see §2.4).

**File:** `internal/witness/agent_failure.go` (new)

```go
// ClassifyAgentFailure determines if a polecat failure is attributable to the
// agent (its underlying provider/model) rather than the task.
//
// The agent name is read from the bead's ResolvedAgent field, which is written
// at spawn time by the dispatch path. If ResolvedAgent is empty (legacy bead),
// classification is skipped.
//
// Agent failure heuristics:
// - Credit exhaustion: exit output contains "credit", "billing", "quota", "insufficient funds"
// - Rate limit: exit output contains "rate limit", "429", "too many requests", "resource_exhausted"
// - Network: exit output contains "connection refused", "timeout", "ECONNRESET", "ETIMEDOUT"
// - Crash loop: same bead respawned >= 2 times with same agent, no code changes between attempts
//
// Returns the failure report if classified as agent failure, nil otherwise.
func ClassifyAgentFailure(
    zombie ZombieInfo,
    agentBead *BeadRecord, // bead.ResolvedAgent contains the agent name
) *AgentFailureReport

// AgentFailureReport contains the classification result.
type AgentFailureReport struct {
    Agent   string // agent preset name, read from bead.ResolvedAgent
    Reason  string // "credit_exhaustion", "rate_limit", "network", "crash_loop"
    BeadID  string
    Rig     string
    Polecat string
}

// SendMail sends an AGENT_FAILURE message to router/ via gt mail.
func (r *AgentFailureReport) SendMail(townRoot string) error {
    // gt mail send router/ \
    //   -s "AGENT_FAILURE: <agent>" \
    //   -m "agent: <agent>\nreason: <reason>\nbead: <bead>\nrig: <rig>\npolecat: <polecat>\ntimestamp: <now>"
}
```

**File:** `internal/witness/handlers.go` — integrate into zombie handling

In the existing `DetectZombiePolecats()` flow, after zombie classification and before restart:

```go
// After classifying zombie type, check if it's an agent failure.
// The agent name comes from the bead's resolved_agent field (set at spawn time).
if agentBead.ResolvedAgent == "" {
    log.Debug("bead has no resolved_agent — skipping agent failure classification", "bead", agentBead.ID)
} else if report := ClassifyAgentFailure(zombie, agentBead); report != nil {
    log.Warn("agent failure detected, notifying router",
        "agent", report.Agent,
        "reason", report.Reason,
        "bead", report.BeadID,
    )
    if err := report.SendMail(townRoot); err != nil {
        log.Error("failed to send AGENT_FAILURE mail", "err", err)
    }
}
// Continue with existing zombie handling (restart, escalate, etc.)
```

**What the witness does NOT do:**
- Judge whether the router made a good tier selection
- Override the router's decision
- Retry with a different tier (that happens on re-dispatch when the router reads its mail)

The witness reports facts via mail. The router acts on them at dispatch time.

#### 2.7 Observability

All overrides (agent or tier) are logged with the polecat's spawn metadata:

```go
log.Info("polecat spawned",
    "polecat", polecatName,
    "agent_override", opts.Agent,
    "tier_override", opts.AgentTier,
    "resolved_agent", runtimeConfig.ResolvedAgent,
)
```

OTEL events for agent failures:
```
gt.routing.agent_failure_reported{agent, reason, rig}
gt.routing.routing_failed{tier, bead, excluded_count}
gt.routing.fallback_triggered{from_tier, to_tier}
```

#### 2.8 Files Changed (Phase 2)

| File | Change |
|------|--------|
| `internal/cmd/sling.go` | **Modified** — add `--tier` flag |
| `internal/cmd/polecat_spawn.go` | **Modified** — add `AgentTier` to `SlingSpawnOptions`, tier resolution with mail-based exclusion, write `resolved_agent` to bead |
| `internal/cmd/sling_target.go` | **Modified** — pass `AgentTier` through to spawn opts |
| `internal/polecat/session_manager.go` | **Modified** — handle `AgentTier` in `Start()` |
| Dolt schema (beads DB) | **Modified** — add `resolved_agent` column to bead records |
| `internal/beads/schema.go` | **Modified** — add `ResolvedAgent` field to `BeadRecord` |
| `internal/routing/mail.go` | **New** — `BuildExclusionCache()`, `ExclusionCache`, `SendRoutingFailedMail()` |
| `internal/routing/mail_test.go` | **New** — unit tests: mail parsing, TTL filtering, mark-as-read, exclusion cache |
| `internal/witness/agent_failure.go` | **New** — `ClassifyAgentFailure()`, `AgentFailureReport`, `SendMail()` — reads `bead.ResolvedAgent` |
| `internal/witness/agent_failure_test.go` | **New** — unit tests: classification heuristics |
| `internal/witness/handlers.go` | **Modified** — integrate `ClassifyAgentFailure` into zombie detection, read agent from bead |
| `internal/cmd/status.go` | **Modified** — add router status section (agent up/down, exclusions, recent mail) |
| `internal/cmd/agent_tier.go` | **New** — `gt agent tier list` command with runtime availability |

---

### Phase 3: Router Agent (Opt-in)

**Goal:** An optional, dedicated router agent evaluates task complexity at dispatch time and selects the appropriate tier. The router agent is any configured agent preset — it sees only tier names and descriptions, classifying the task, not the agents. Go code then handles agent selection (round-robin/priority), exclusion filtering (from AGENT_FAILURE mail), and tier fallback. The router always commits to a decision (no confidence threshold).

#### 3.1 Router Configuration

**File:** `internal/config/types.go` — add to `TownSettings`:

```go
type RouterConfig struct {
    Enabled          bool   `json:"enabled"`             // default: false
    Agent            string `json:"agent"`               // agent preset name (e.g., "qwen-local", "claude-haiku")
    FallbackTier     string `json:"fallback_tier"`       // tier if router agent fails (default: "medium")
    TimeoutMs        int    `json:"timeout_ms"`          // default: 5000
    FailureTTLMinutes int   `json:"failure_ttl_minutes"` // how long AGENT_FAILURE mail stays relevant (default: 30)
}
```

Added to `TownSettings`:
```go
Router *RouterConfig `json:"router,omitempty"`
```

#### 3.2 Router Interface

**File:** `internal/routing/router.go` (new)

```go
package routing

// RoutingRequest is the input to the router agent.
// Contains only tier names + descriptions — no agent lists or exclusion info.
// Agent selection and exclusion are handled by Go code after the router returns.
type RoutingRequest struct {
    TaskTitle       string        `json:"task_title"`
    TaskDescription string        `json:"task_description"`
    Role            string        `json:"role"`
    AvailableTiers  []TierSummary `json:"available_tiers"`
}

// TierSummary describes a tier for the router's context.
// Only includes the name and description — the router doesn't need to know
// which agents are in each tier or which are excluded.
type TierSummary struct {
    Name        string `json:"name"`
    Description string `json:"description"` // from AgentTier.Description
}

// RoutingDecision is the structured output from the router.
// No confidence field — the router always commits.
type RoutingDecision struct {
    Tier      string `json:"tier"`
    Rationale string `json:"rationale"`
}

// Router evaluates task complexity and returns a tier recommendation.
// After the router returns a tier, Go code handles agent selection
// (priority/round-robin), exclusion filtering, and tier fallback.
type Router interface {
    Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error)
}
```

#### 3.3 Router Agent Implementation

**File:** `internal/routing/agent_router.go` (new)

```go
// AgentRouter uses a configured agent to evaluate task complexity.
// The router agent is any agent preset — local (Ollama), remote (Anthropic), or custom.
type AgentRouter struct {
    config     *config.RouterConfig
    agentCfg   *config.RuntimeConfig // resolved from RouterConfig.Agent
    tierConfig *config.AgentTierConfig
}

func (r *AgentRouter) Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error) {
    // 1. Build prompt from template + request (tier names + descriptions only, NO agent lists)
    // 2. Invoke the router agent with JSON mode / structured output
    // 3. Parse response into RoutingDecision
    // 4. Validate tier name exists in config
    // 5. Record OTEL metrics (latency, tier chosen)
    // 6. On any error (timeout, parse failure, invalid tier) → return fallback_tier
}
```

**Separation of concerns — router agent vs Go code:**

The router agent and the Go tier resolution engine have strictly separated responsibilities:

| Concern | Who handles it | Why |
|---------|---------------|-----|
| Task → tier classification | **Router agent** | Requires understanding task semantics |
| Agent selection within a tier | **Go code** (`ResolveTierToRuntimeConfig`) | Mechanical — priority or round-robin |
| Agent exclusion (from AGENT_FAILURE mail) | **Go code** (`ExclusionCache`) | Deterministic — no router judgment needed |
| Tier fallback on exhaustion | **Go code** (`UpOneTier`) | Mechanical — follow `TierOrder` |

The router agent sees **tier names and descriptions only** — never individual agent names, never exclusion state. This keeps the router prompt simple and focused on the one thing it's good at: understanding what kind of work a task involves.

**Router prompt template** (embedded):
```
You are a task complexity classifier for a software engineering automation system.

Given a task description, select the most appropriate agent tier.

## Available Tiers (ordered by capability, lowest to highest)
{{range .Tiers}}
- **{{.Name}}**: {{.Description}}
{{end}}

## Guidelines
- Analyze the task and match it with the tier whose description best fits the work
- Use the lowest tier that can handle the task well (cost efficiency)
- When in doubt between two tiers, prefer the lower one

## Task
Title: {{.Title}}
Description: {{.Description}}
Role: {{.Role}}

Respond with JSON only: {"tier": "...", "rationale": "one sentence"}
```

**Note:** The prompt does NOT include agent lists, exclusion info, or availability status. After the router agent returns a tier name, Go code handles agent selection (round-robin/priority), exclusion filtering, and tier fallback. If the selected tier has no available agents, Go code moves up one tier automatically — the router agent is not consulted again.

#### 3.4 Router Agent Invocation

The router does NOT spawn a tmux session or start an interactive agent. It invokes the configured agent in **non-interactive mode** — a single prompt in, structured JSON out, process exits. This is a subprocess call, not a session.

**How agents support non-interactive mode:**

Every agent preset already has a `NonInteractiveConfig` in `internal/config/agents.go` that describes how to invoke it for one-shot execution:

| Agent | Non-interactive invocation | Config |
|-------|---------------------------|--------|
| Claude | `claude -p "prompt"` | Native (nil config — `-p` is built-in) |
| Gemini | `gemini -p "prompt" --output-format json` | `PromptFlag: "-p"`, `OutputFlag: "--output-format json"` |
| Codex | `codex exec "prompt" --json` | `Subcommand: "exec"`, `OutputFlag: "--json"` |
| Cursor | `cursor-agent -p "prompt" --output-format json` | `PromptFlag: "-p"`, `OutputFlag: "--output-format json"` |
| Custom | User-defined command + args | Via `settings/agents.json` |

**The router builds and executes a non-interactive command from the agent preset:**

```go
// BuildRouterCommand constructs the non-interactive command for the router agent.
// Uses the agent preset's NonInteractiveConfig to determine flags and structure.
//
// For Claude (native non-interactive):
//   claude -p "<routing prompt>" --output-format json
//
// For agents with NonInteractiveConfig.Subcommand (e.g., Codex):
//   codex exec "<routing prompt>" --json
//
// For agents with NonInteractiveConfig.PromptFlag (e.g., Gemini):
//   gemini -p "<routing prompt>" --output-format json
func BuildRouterCommand(preset *config.AgentPresetInfo, prompt string) *exec.Cmd
```

**Execution flow:**

```go
func (r *AgentRouter) Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error) {
    // 1. Render the routing prompt from template + request
    prompt := renderPrompt(req)

    // 2. Build non-interactive command from agent preset
    cmd := BuildRouterCommand(r.preset, prompt)

    // 3. Execute with timeout (from RouterConfig.TimeoutMs)
    ctx, cancel := context.WithTimeout(ctx, r.config.Timeout())
    defer cancel()
    cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

    // 4. Capture stdout, parse JSON response
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("router agent failed: %w", err)
    }

    // 5. Parse into RoutingDecision, validate tier name
    var decision RoutingDecision
    if err := json.Unmarshal(output, &decision); err != nil {
        return nil, fmt.Errorf("router agent returned invalid JSON: %w", err)
    }
    if !r.tierConfig.HasTier(decision.Tier) {
        return nil, fmt.Errorf("router agent returned unknown tier: %q", decision.Tier)
    }

    return &decision, nil
}
```

**No special agent configuration is needed.** The router uses the same agent preset the user already has configured — it just invokes it differently (non-interactive subprocess instead of interactive tmux session). Any agent that supports non-interactive mode can serve as the router agent.

**Key properties of this approach:**
- **No persistent process** — the router agent starts, classifies, and exits. No session management, no tmux, no zombie detection needed
- **Reuses existing infrastructure** — `NonInteractiveConfig` already describes how each agent handles one-shot invocation
- **Works with any agent** — local Ollama, Claude, Gemini, or a custom script. If it can take a prompt and return JSON, it can route
- **Subprocess isolation** — the router agent runs in its own process with its own timeout. A hung router doesn't affect the dispatch path beyond the configured timeout

**Using local models for routing via OpenCode:**

The natural choice for a cost-free, low-latency router is a local model. OpenCode is Gas Town's multi-model agent — it supports any provider (including local Ollama) and already has non-interactive mode (`opencode run "prompt" --format json`). The `-m` flag selects the provider/model.

To set up a local-model router agent:

```bash
# 1. Register a custom agent preset targeting a local model
gt config agent set router-local \
  --command opencode \
  --args '-m ollama/qwen2.5-coder:14b' \
  --non-interactive-subcommand run \
  --non-interactive-output-flag '--format json'

# 2. Set it as the router agent
gt config router set router-local
```

**Note:** `gt config agent set` does not currently support `--non-interactive-*` flags. These must be added as part of this work (see §3.9 below), or the user can edit `settings/agents.json` directly.

This creates an agent preset that invokes `opencode -m ollama/qwen2.5-coder:14b run "<prompt>" --format json` — a local model classification with no API costs and sub-second latency.

The existing OpenCode agent template (`templates/agents/opencode.json.tmpl`) already captures this pattern:

```json
{
  "name": "opencode-{{.Model}}",
  "command": "opencode",
  "args": ["-m", "{{.Provider}}/{{.Model}}"],
  "non_interactive": {
    "subcommand": "run",
    "output_flag": "--format json"
  }
}
```

**Any multi-model agent that supports non-interactive mode works the same way.** The router doesn't care what's behind the agent — Ollama, vLLM, a remote API, a shell script that calls `curl`. It invokes the agent's non-interactive command, reads JSON from stdout, and moves on.

**What about agents without `NonInteractiveConfig`?**

Agents that lack non-interactive support (e.g., Pi, Amp) cannot be used as router agents. `gt config router set` validates this at configuration time:

```go
func validateRouterAgent(agentName string) error {
    preset := config.GetAgentPresetByName(agentName)
    if preset == nil {
        return fmt.Errorf("unknown agent: %q", agentName)
    }
    if !preset.SupportsNonInteractive() {
        return fmt.Errorf("agent %q does not support non-interactive mode (required for routing)", agentName)
    }
    return nil
}
```

#### 3.5 Integration with Dispatch

**File:** `internal/cmd/polecat_spawn.go`

In `SpawnPolecatForSling()`, after resolving the bead but before session start:

```go
// Router evaluation (opt-in)
if routerConfig := loadRouterConfig(townRoot); routerConfig != nil && routerConfig.Enabled {
    if opts.Agent == "" && opts.AgentTier == "" { // don't route if explicit override
        tierConfig := config.LoadAgentTierConfig(townRoot)

        // Step 1: Router agent picks a TIER (sees only tier names + descriptions)
        decision, err := router.Route(ctx, routing.RoutingRequest{
            TaskTitle:       bead.Title,
            TaskDescription: bead.Body,
            Role:            "polecat",
            AvailableTiers:  tierConfig.BuildTierSummaries(), // names + descriptions only
        })
        if err == nil && decision != nil {
            opts.AgentTier = decision.Tier
            log.Info("router selected tier",
                "tier", decision.Tier,
                "rationale", decision.Rationale,
            )
        } else {
            log.Warn("router failed, using fallback",
                "err", err,
                "fallback_tier", routerConfig.FallbackTier,
            )
            opts.AgentTier = routerConfig.FallbackTier
        }

        // Step 2: Go code picks an AGENT from that tier
        // (round-robin/priority, excludes failed agents, falls back up tiers)
        cache := routing.BuildExclusionCache(townRoot, routerFailureTTL)
        rc, resolveErr := tierConfig.ResolveTierToRuntimeConfig(opts.AgentTier, cache.ExcludedSet())
        if resolveErr != nil {
            routing.SendRoutingFailedMail(townRoot, opts.AgentTier, bead.ID, cache)
            return resolveErr
        }
        runtimeConfig = rc
    }
}
```

**Critical:** Router is bypassed when `--agent` or `--tier` is specified. Explicit overrides always win.

**The two-step flow:**
1. **Router agent** → "this task needs the `large` tier" (semantic understanding)
2. **Go code** → "the `large` tier has `[claude-opus, gemini-pro]`, `claude-opus` is excluded, using `gemini-pro`" (mechanical selection)

#### 3.6 CLI Commands

```bash
# Set the router agent and enable routing
gt config router set claude-haiku
gt config router set qwen-local

# Disable the router
gt config router disable

# Show router config + recent AGENT_FAILURE mail
gt config router show

# Test the router against a bead without dispatching (dry-run)
gt config router test <bead-id>
# Output:
#   Router agent: qwen-local (non-interactive: qwen-local -p "<prompt>" --output-format json)
#   Router decision: tier=large, rationale='Cross-cutting refactor across 3 packages'
#   Agent resolution: claude-opus (✓ available, priority selection)
#   Excluded agents: gemini-pro (credit_exhaustion, 12min ago)

# Set fallback tier (used when router agent fails)
gt config router set-fallback medium

# Set timeout
gt config router set-timeout 3000

# Set how long AGENT_FAILURE mail stays relevant
gt config router set-failure-ttl 45
```

#### 3.7 Observability

**OTEL metrics** (via existing telemetry infrastructure):

| Metric | Type | Labels |
|--------|------|--------|
| `gt.router.calls` | Counter | `agent`, `outcome` (success/fallback/error) |
| `gt.router.latency_ms` | Histogram | `agent` |
| `gt.router.tier_selected` | Counter | `tier`, `role` |
| `gt.routing.agent_failure_reported` | Counter | `agent`, `reason` |
| `gt.routing.routing_failed` | Counter | `tier`, `excluded_count` |
| `gt.routing.fallback_triggered` | Counter | `from_tier`, `to_tier` |

#### 3.8 `gt config agent set` — Non-Interactive Flags

`gt config agent set` currently supports `--command`, `--args`, and `--env` but has no flags for `NonInteractiveConfig`. To support router agent registration from the CLI (without editing `settings/agents.json` by hand), add:

```bash
gt config agent set <name> \
  --command <cmd> \
  --args '<args>' \
  --non-interactive-subcommand <sub>  # e.g., "run" for opencode, "exec" for codex
  --non-interactive-prompt-flag <flag> # e.g., "-p" for gemini/claude
  --non-interactive-output-flag <flag> # e.g., "--format json", "--output-format json"
```

**File:** `internal/cmd/config.go` — add three new `StringVar` flags to the `agent set` subcommand, write them into `NonInteractiveConfig` in `settings/agents.json`.

At least one of `--non-interactive-subcommand` or `--non-interactive-prompt-flag` is required for the agent to support non-interactive mode. If neither is set and the agent is not a built-in preset with native non-interactive support (like Claude), the agent cannot be used as a router.

#### 3.9 Files Changed (Phase 3)

| File | Change |
|------|--------|
| `internal/routing/router.go` | **New** — `Router` interface, request/response types, `TierSummary` |
| `internal/routing/agent_router.go` | **New** — `AgentRouter` implementation, `BuildRouterCommand()`, non-interactive invocation |
| `internal/routing/prompt.go` | **New** — router prompt template |
| `internal/routing/router_test.go` | **New** — unit tests with mock router agent |
| `internal/config/types.go` | **Modified** — add `RouterConfig` to `TownSettings` |
| `internal/config/agents.go` | **Modified** — add `SupportsNonInteractive()` method to `AgentPresetInfo` |
| `internal/cmd/config.go` | **Modified** — add `--non-interactive-*` flags to `agent set`, register `router` subcommand |
| `internal/cmd/polecat_spawn.go` | **Modified** — integrate router into dispatch flow |
| `internal/cmd/config_router.go` | **New** — `gt config router` CLI commands (including `set` validation) |

---

## Precedence Chain (Final)

```
    ┌─────────────────────────────────────────────┐
    │  1. CLI: gt sling --agent codex             │  Explicit agent (bypasses tiers)
    │  2. CLI: gt sling --tier large              │  Explicit tier (picks from tier's agents)
    ├─────────────────────────────────────────────┤
    │  3. Rig settings: role_agents / worker_agents│  Per-rig config
    │  4. Town settings: role_agents / crew_agents │  Town-wide config
    │     (also written by gt config cost-tier)    │
    ├─────────────────────────────────────────────┤
    │  5. Router agent decision (if enabled)       │  Dynamic (Phase 3)
    ├─────────────────────────────────────────────┤
    │  6. agent_tiers.role_defaults               │  Tier-based default (Phase 1)
    ├─────────────────────────────────────────────┤
    │  7. Town default_agent                       │  Final fallback
    └─────────────────────────────────────────────┘

    At ALL levels where tier resolution occurs, AGENT_FAILURE
    mail to router/ is checked. Excluded agents are skipped.
    If a tier is fully excluded, resolution moves UP one tier
    (per TierOrder). If no tier has available agents,
    ROUTING_FAILED mail is sent to mayor/.
```

---

## Data Flow Diagram

```
                         ┌──────────────┐
                         │  gt sling    │
                         │  --tier X    │
                         │  --agent Y   │
                         └──────┬───────┘
                                │
                    ┌───────────▼────────────┐
                    │  Precedence Resolution  │
                    │  (loader.go)            │
                    └───────────┬─────────────┘
                                │
              ┌─────────────────▼──────────────────┐
              │  Router Agent (opt-in, Phase 3)      │
              │  Input: bead, role, tier desc.       │
              │  Output: tier name                   │
              │  (does NOT see agents or exclusions) │
              └─────────────────┬──────────────────┘
                                │
              ┌─────────────────▼──────────────────┐
              │  Go: Tier Resolution (Phase 1)      │
              │  Reads: router/ inbox               │
              │     (AGENT_FAILURE mail < 30min)    │
              │  Builds: ExclusionCache             │
              │  tier name → agent selection         │
              │  strategy: priority | round-robin    │
              │  skips: excluded agents from mail    │
              └─────────┬─────────────┬────────────┘
                        │             │
                  (success)    (no agents)
                        │             │
                        │      ┌──────▼──────┐
                        │      │ ROUTING_    │
                        │      │ FAILED mail │──► mayor/
                        │      └─────────────┘
              ┌─────────▼──────────────────┐
              │  RuntimeConfig              │
              │  (from agent preset)        │
              │  command + args + env       │
              └─────────┬──────────────────┘
                        │
              ┌─────────▼──────────────────┐
              │  Polecat Session Start      │
              └─────────┬──────────────────┘
                        │
                 ┌──────▼──────┐
                 │   Polecat   │
                 │  (running)  │
                 └──────┬──────┘
                        │ failure?
                 ┌──────▼──────┐
                 │   Witness   │
                 │  (detects)  │
                 └──────┬──────┘
                        │ agent failure?
          ┌─────────────▼───────────────┐
          │  ClassifyAgentFailure()     │
          │  (credit, rate limit,       │
          │   network, crash loop)      │
          └─────────────┬───────────────┘
                        │ yes
          ┌─────────────▼───────────────┐
          │  gt mail send router/       │
          │  -s "AGENT_FAILURE: ..."    │───► router/ inbox
          └─────────────────────────────┘
                        ▲
                        │ read at next dispatch
                        │ (closes the feedback loop)
```

---

## Testing Strategy

### Unit Tests
- **Tier resolution:** correct agent selection per strategy, fallback chains, tier ordering
- **Round-robin:** verify cycling, skip excluded, reset on config change
- **Mail parsing:** AGENT_FAILURE subject/body parsing, TTL filtering, exclusion set building
- **Agent failure classification:** heuristic matching for credit/rate-limit/network/crash patterns
- **Precedence:** verify each layer overrides the one below it
- **Config loading:** valid/invalid JSON, missing fields, defaults applied
- **Router:** mock agent responses, fallback on timeout, invalid tier name handling

### Integration Tests
- `gt sling --tier large` spawns polecat with correct agent
- Cost tier + agent tier coexistence (cost tier wins via RoleAgents)
- Round-robin distribution across agents in a tier
- AGENT_FAILURE mail → tier resolution skips excluded agent → falls back up
- ROUTING_FAILED mail sent to mayor when all agents exhausted
- Witness detects agent failure → sends AGENT_FAILURE mail → next dispatch avoids agent
- Router opt-in/opt-out: dispatch with/without router enabled

### Manual Testing
- Configure multi-agent tier with round-robin, observe distribution in logs
- Send an AGENT_FAILURE mail manually, verify dispatch routes to alternative agent
- Enable router with local Ollama, verify routing decisions in logs
- `gt config agent tiers show` + `gt config router show` display correct state
- `gt agent tier list` shows runtime availability with exclusions
- `gt config router test <bead>` shows dry-run decision with current exclusions

---

## Migration & Rollout

1. **Phase 1 ships first** — no breaking changes, purely additive config
2. Existing `gt config agent`, `gt config cost-tier`, and cost tiers continue working unchanged
3. Users adopt `agent_tiers` when they want multi-agent tiers, selection strategies, or custom tier definitions
4. Phase 2 adds `--tier` flag and AGENT_FAILURE mail protocol — no behavior change unless used
5. Phase 3 is fully opt-in — zero impact unless `router.enabled = true`

**No migration required.** Users with no `agent_tiers` config get identical behavior to today. The `DefaultAgentTierConfig()` is only materialized when the tier system is explicitly invoked (via `--tier` flag or `gt config agent tiers` commands).

---

## Out of Scope (Future Work)

- **Latency-based agent selection** — requires health-checking infrastructure
- **Per-bead cost tracking with router attribution** — wait for cost reporting v2
- **Router learning from outcomes** — would require a task quality judge, which we've decided we don't have
- **Vision/code-review/specialized tiers** — custom tier names support this, but no built-in specialization logic
- **Cross-rig tier inheritance** — each rig resolves independently; shared config is at town level
- **Automatic tier suggestion** — "you should upgrade this tier based on failure patterns" (future witness intelligence)
