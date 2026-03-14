# Implementation Plan: Intelligent Model Routing

**RFC:** https://github.com/steveyegge/gastown/discussions/2531
**Status:** Implementation Plan
**Date:** 2026-03-14

---

## Current State Analysis

Gas Town already has significant model-routing infrastructure:

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

**What's missing** (what the RFC adds):
1. **Model tiers with provider matrix** — abstract `small`/`medium`/`large` tiers that map to concrete models across providers, with per-tier selection strategies
2. **LLM-driven task-complexity routing** — dynamic model selection based on task analysis
3. **Witness→Router feedback via mail** — witness sends MODEL_FAILURE mail to the router when it detects provider-level failures
4. **Router→Mayor escalation via mail** — router sends ROUTING_FAILED mail to the mayor when no routable models remain

---

## Open Question Decisions

### Q1: Router opt-in or opt-out?
**Decision: Opt-in (off by default)**

The LLM router adds latency to every dispatch and requires a running local model. Phase 1 (tier config + role defaults) delivers most of the value without it. Enable via config:
```json
{ "router": { "enabled": true } }
```

### Q2: Router context depth?
**Decision: Minimal — bead title + description + role + recent MODEL_FAILURE mail**

The router performs a classification task, not planning. Bead title + body is sufficient signal for tier selection. We explicitly do NOT try to judge "how well" a task was completed — there is no judge for that.

**However**, the router checks its mail for recent `MODEL_FAILURE` messages from the witness. When the witness detects a model failure (credit exhaustion, network issue, persistent crashes), it sends mail to the router. The router reads recent mail at dispatch time to understand which providers are currently unavailable.

**v1 router input:**
- Bead title + description (from `bd show`)
- Current role (`polecat`, `crew`, etc.)
- Available tiers and their provider lists
- Recent MODEL_FAILURE mail (providers currently known to be down)

### Q3: No confidence threshold
**Decision: No confidence field — the router always commits to a tier**

The router should be capable enough for a routing decision. If it routes to an underpowered model and the polecat gets stuck, we can't easily judge whether that was a bad routing decision vs a genuinely hard task. The failure path is:

1. Router picks a tier → polecat spawns
2. Polecat gets stuck / crashes / runs out of credits
3. **Witness** detects the failure (zombie detection, spawn circuit breaker)
4. Witness sends `MODEL_FAILURE` mail to `router/` with provider/model/reason
5. On re-dispatch, router reads its mail and routes to a different provider

The witness is the judge, not the router. The router's structured response is simplified:
```json
{ "tier": "large", "rationale": "cross-cutting refactor touching auth + DB layers" }
```

No `confidence` field. The router always returns a tier. If the router LLM itself fails (timeout, crash), use `fallback_tier` from config.

### Q4: Provider fallback on failure — witness-informed via mail
**Decision: Witness sends MODEL_FAILURE mail → router excludes those providers**

When a provider is unavailable (detected by the witness — credit exhaustion, rate limit, network failure, repeated polecat crashes on a specific model), the witness sends mail to `router/`:

```
gt mail send router/ -s "MODEL_FAILURE: anthropic/opus" -m "reason: credit_exhaustion\nprovider: anthropic\nmodel: opus\nbead: xyz-123\nrig: gastown"
```

**At dispatch time, the router:**
1. Reads its inbox for `MODEL_FAILURE` mail newer than 30 minutes (configurable TTL)
2. Builds a set of excluded provider/model pairs from those messages
3. Skips excluded providers when selecting from a tier's provider list
4. If all providers in a tier are excluded, moves **up one tier** (e.g., `medium` → `large`)
5. If no routable models remain at any tier, sends `ROUTING_FAILED` mail to `mayor/` and uses `fallback_tier`

**Why mail instead of a custom store:**
- Uses Gas Town's existing communication infrastructure — no new persistence layer
- Mail is already Dolt-backed, survives agent restarts, and has built-in read/unread tracking
- Consistent with how all other inter-agent communication works (SPAWN_BLOCKED, HELP, MERGE_FAILED)
- The witness already knows how to send mail; no new integration point needed
- The TTL is just a recency filter on mail timestamps — trivial to implement

**Tier ordering** is defined in config (see §1.1). Moving "up" means toward higher capability. The router prompt includes the tier ordering so it can reason about fallbacks.

### Q5: Cost attribution for router calls?
**Decision: Infrastructure overhead — attributed to the same cost bucket as the Mayor**

Router calls are tiny (local model, small context, structured output). Track as a separate OTEL metric (`gt.router.calls`, `gt.router.latency_ms`) but attribute costs to infrastructure, not individual beads.

### Q6: Multi-provider tier resolution — per-tier selection strategy
**Decision: Configurable per-tier — `priority` (default) or `round-robin`**

Each tier specifies its own selection strategy:

```json
{
  "model_tiers": {
    "medium": {
      "selection": "round-robin",
      "models": [
        { "provider": "anthropic", "model": "sonnet" },
        { "provider": "openai", "model": "gpt-4.1" }
      ]
    },
    "large": {
      "selection": "priority",
      "models": [
        { "provider": "anthropic", "model": "opus" }
      ]
    }
  }
}
```

- **`priority`** (default): First available provider in list order wins
- **`round-robin`**: Cycle through providers in list order, skipping unavailable ones

When all providers in a tier are exhausted (all excluded via MODEL_FAILURE mail or all failed), fall back to next provider in the same tier, then move **up one tier**. The router understands tier ordering.

### Q7: Interaction with `gt config agent`
**Decision: Extend the existing `gt config` command tree — add `gt config tier` alongside `gt config agent` and `gt config cost-tier`**

The existing CLI structure is:
```
gt config
├── agent (list/get/set/remove)     ← custom agent presets
├── cost-tier [standard|economy|budget]  ← quick presets
├── default-agent [name]
├── get/set <key> <value>
```

New structure:
```
gt config
├── agent (list/get/set/remove)     ← custom agent presets (unchanged)
├── cost-tier [standard|economy|budget]  ← quick presets (unchanged)
├── tier                             ← NEW: model tier management
│   ├── show                         ← display current tier config
│   ├── set <tier> [flags]           ← create/update a tier
│   ├── remove <tier>                ← remove a tier
│   ├── set-role <role> <tier>       ← map role to tier
│   ├── add-provider <tier> [flags]  ← append provider to tier
│   ├── remove-provider <tier> [flags] ← remove provider from tier
│   └── init                         ← initialize default tier config
├── router                           ← NEW: LLM router management (Phase 3)
│   ├── show                         ← display router config + recent MODEL_FAILURE mail
│   ├── enable [flags]               ← enable with provider/model
│   ├── disable                      ← disable router
│   └── test <bead-id>               ← dry-run routing decision
├── default-agent [name]             ← (unchanged)
├── get/set <key> <value>            ← (unchanged)
```

**Precedence (highest → lowest):**
1. `--agent` flag on `gt sling` (explicit CLI override)
2. `--tier` flag on `gt sling` (explicit tier override)
3. Rig-level `role_agents` / `worker_agents` (rig settings)
4. Town-level `role_agents` / `crew_agents` (town settings, written by `gt config agent`)
5. LLM router decision (if enabled, Phase 3)
6. Model tier `role_defaults` (Phase 1)
7. Cost tier preset (standard/economy/budget) — writes to `role_agents`, so effectively layer 4
8. Town `default_agent` (final fallback)

Note: `gt config cost-tier` writes to `role_agents` (layer 4), so it takes precedence over `model_tiers.role_defaults` (layer 6). Users who switch to the tier system can stop using cost tiers — both systems coexist but tier config is more flexible.

---

## Architecture: Witness→Router Feedback Loop

This uses Gas Town's existing mail system. No custom stores or persistence layers.

```
                    ┌─────────────┐
                    │   Router    │
                    │  (Phase 3)  │
                    │             │
                    │ reads mail: │◄──── MODEL_FAILURE mail (< 30min)
                    │ router/     │
                    └──────┬──────┘
                           │ no routable models?
                           │
                    ┌──────▼──────┐
                    │   Mayor     │◄──── ROUTING_FAILED mail
                    │  mayor/     │
                    └─────────────┘

                    ┌─────────────┐
                    │   Witness   │
                    │  (existing) │
                    └──────┬──────┘
                           │ detects model failure
                           │
                    ┌──────▼──────┐
                    │  gt mail    │
                    │  send       │──── MODEL_FAILURE → router/
                    │  router/    │
                    └─────────────┘
                           ▲
                    ┌──────┴──────┐
                    │   Polecat   │
                    │  (failure)  │
                    └─────────────┘
```

### Mail Protocol: MODEL_FAILURE

**Sender:** Witness (any rig)
**Recipient:** `router/`
**Subject:** `MODEL_FAILURE: <provider>/<model>`

**Body format** (structured, machine-parseable):
```
provider: anthropic
model: opus
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
- `crash_loop` — same bead respawned ≥2 times with same model, no progress

### Mail Protocol: ROUTING_FAILED

**Sender:** Router (at dispatch time)
**Recipient:** `mayor/`
**Subject:** `ROUTING_FAILED: no routable models for <tier>`

**Body format:**
```
requested_tier: medium
bead: xyz-123
excluded_providers:
  - anthropic/sonnet (credit_exhaustion, 15min ago)
  - openai/gpt-4.1 (rate_limit, 5min ago)
fallback_used: large
action: dispatched with fallback tier
```

If even the fallback tier has no available providers:
```
requested_tier: medium
bead: xyz-123
excluded_providers: [all]
fallback_tier: large (also unavailable)
action: BLOCKED — no routable models, awaiting mayor intervention
```

### Router Mail Processing at Dispatch Time

The router is not a persistent agent — it runs as a function call during dispatch. It maintains an **in-memory exclusion cache** that is built once per dispatch cycle from mail, then reused for all tier resolutions in that cycle (avoids re-reading mail for every resolution call within the same dispatch).

**Startup recovery:**
On process startup (e.g., after mayor restart, compaction, or new session), the router re-reads **all** MODEL_FAILURE mail within the TTL window — including previously-read messages. This recovers exclusion state that was lost when the in-memory cache was discarded. Without this, a mayor restart during a provider outage would cause the router to immediately route back to the failing provider.

1. Read `router/` inbox via Go mail API — **both read and unread** MODEL_FAILURE messages
2. Filter by timestamp: only messages within configured TTL (default 30 min)
3. Parse `provider` + `model` + `reason` from message bodies
4. Build in-memory `ExclusionCache` (a `map[string]ExclusionEntry` keyed by `"provider/model"`)
5. Cache is now warm — subsequent dispatch cycles only need to check for **unread** messages and merge them in

**Dispatch cycle (after startup):**
1. Check `router/` inbox for **unread** MODEL_FAILURE messages only
2. Parse and merge new exclusions into the existing in-memory `ExclusionCache`
3. Evict expired entries (older than TTL) from the cache
4. **Mark new messages as read** — prevents re-processing on next cycle
5. Use `ExclusionCache` for all tier resolutions in this dispatch cycle

**Mark-as-read is important:** Without it, the `router/` inbox grows unboundedly with stale MODEL_FAILURE mail. The TTL filter would still work, but the inbox becomes noisy for `gt status` and `gt config router show`. Marking as read keeps the inbox clean — only unread MODEL_FAILURE messages represent failures the router hasn't yet acted on.

**Why read+unread on startup, unread-only after:** The startup pass recovers state from before the restart. Once the cache is warm, only new (unread) messages need processing. This keeps dispatch-time mail reads cheap — typically zero or one message — while ensuring restarts don't cause amnesia.

```go
// ExclusionCache holds parsed MODEL_FAILURE state for one dispatch cycle.
// Built once from mail, reused for all tier resolutions, then discarded.
type ExclusionCache struct {
    Entries map[string]ExclusionEntry // key: "provider/model"
    BuiltAt time.Time
}

// ExclusionEntry is a single excluded provider parsed from MODEL_FAILURE mail.
type ExclusionEntry struct {
    Provider  string
    Model     string
    Reason    string
    ReportedAt time.Time
    MailID    string // for mark-as-read
}

// BuildExclusionCache reads router/ inbox (read+unread MODEL_FAILURE mail within TTL),
// builds the full exclusion cache, and marks new messages as read.
// Used on startup to recover state from before a restart.
func BuildExclusionCache(townRoot string, ttl time.Duration) *ExclusionCache

// RefreshExclusionCache reads only unread MODEL_FAILURE mail from router/ inbox,
// merges new exclusions into the existing cache, evicts expired entries, and
// marks new messages as read. Used on subsequent dispatch cycles after startup.
func (c *ExclusionCache) Refresh(townRoot string, ttl time.Duration)

// IsExcluded checks if a provider/model is in the cache.
func (c *ExclusionCache) IsExcluded(provider, model string) bool

// ExcludedSet returns the set of "provider/model" strings for tier resolution.
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

📡 Model Router
  Status:     enabled (ollama/qwen2.5-coder:14b)
  Fallback:   medium
  Failure TTL: 30m

  Provider Status:
    ✓ anthropic/haiku    available
    ✓ anthropic/sonnet   available
    ✗ anthropic/opus     excluded (credit_exhaustion, 12m ago, expires in 18m)
    ✓ ollama/qwen2.5     available

  Recent MODEL_FAILURE mail (router/ inbox):
    1. anthropic/opus — credit_exhaustion — 12m ago (read ✓)
    2. openai/gpt-4.1 — rate_limit — 45m ago (expired, read ✓)

  Routing summary (last 1h):
    Dispatches: 8 | Fallbacks: 1 | Failures: 0
```

When router is disabled:
```
📡 Model Router
  Status: disabled
```

When no tier config exists:
```
📡 Model Router
  Status: not configured (no model_tiers in settings)
```

**Implementation:** `gt status` already aggregates data from multiple subsystems. Add a `routerStatus()` function that:
1. Loads `RouterConfig` from town settings
2. Reads `router/` inbox (both read and unread MODEL_FAILURE messages)
3. Applies TTL filter to determine which exclusions are active vs expired
4. Cross-references active exclusions against the tier config's provider lists
5. Formats the provider status table (✓/✗)

**File:** `internal/cmd/status.go` — add router status section

This gives operators immediate visibility into:
- Whether the router is active
- Which providers are currently excluded and why
- How long until exclusions expire
- Whether routing failures have been escalated to the mayor

---

## Implementation Phases

### Phase 1: Model Tiers + Role Defaults

**Goal:** Implement a user-configurable tier system with multi-provider support, per-tier selection strategies, and role defaults. This formalizes what users in #1642 are doing manually.

**This phase alone closes most of the gap described in the RFC.**

#### 1.1 New Config Schema

**File:** `internal/config/model_tier.go` (new)

```go
// ModelTier defines a capability tier with an ordered list of provider/model pairs.
type ModelTier struct {
    Models    []ModelProvider `json:"models"`
    Selection string         `json:"selection"` // "priority" (default) or "round-robin"
    Fallback  bool           `json:"fallback"`  // default: true — fall back to higher tier on failure
}

// ModelProvider maps a provider to a specific model identifier.
type ModelProvider struct {
    Provider string `json:"provider"` // "anthropic", "openai", "ollama", "generic"
    Model    string `json:"model"`    // e.g., "claude-opus-4-6", "gpt-4.1"
    Agent    string `json:"agent"`    // optional: agent preset name (e.g., "gemini")
}

// ModelTierConfig holds the full tier routing configuration.
type ModelTierConfig struct {
    Tiers        map[string]*ModelTier `json:"tiers"`
    TierOrder    []string              `json:"tier_order"`      // capability ordering: ["small", "medium", "large"]
    RoleDefaults map[string]string     `json:"role_defaults"`   // role → tier name
}
```

**Key design decisions:**
- **JSON, not YAML** — the entire config system uses JSON; no new dependencies
- **`Agent` field on `ModelProvider`** — for non-Claude providers (gemini, codex), the routing target is an agent preset, not a `--model` flag. Example: `{ "provider": "google", "model": "gemini-2.5-pro", "agent": "gemini" }`
- **Tiers are string-keyed, not an enum** — users can define custom tiers beyond small/medium/large (e.g., "local", "reasoning", "vision")
- **`TierOrder` is explicit** — defines the capability ordering. "Up one tier" means moving toward the end of this list. Required for the router's fallback behavior
- **`Selection` per tier** — `priority` or `round-robin`, not global. A `large` tier with one provider uses `priority`; a `medium` tier with multiple providers might use `round-robin`

#### 1.2 Default Tier Configuration

**File:** `internal/config/model_tier_defaults.go` (new)

Provides a sensible out-of-the-box configuration matching the community convention from #1642:

```go
func DefaultModelTierConfig() *ModelTierConfig {
    return &ModelTierConfig{
        Tiers: map[string]*ModelTier{
            "small": {
                Models:    []ModelProvider{{Provider: "anthropic", Model: "haiku"}},
                Selection: "priority",
                Fallback:  true,
            },
            "medium": {
                Models:    []ModelProvider{{Provider: "anthropic", Model: "sonnet"}},
                Selection: "priority",
                Fallback:  true,
            },
            "large": {
                Models:    []ModelProvider{{Provider: "anthropic", Model: "opus"}},
                Selection: "priority",
                Fallback:  false, // large is the top — nowhere to fall back to
            },
        },
        TierOrder:    []string{"small", "medium", "large"},
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

**File:** `internal/config/model_tier.go`

```go
// ResolveTierToRuntimeConfig resolves a tier name to a RuntimeConfig.
// excludedProviders is a set of "provider/model" strings to skip (from MODEL_FAILURE mail).
// Uses the tier's selection strategy (priority or round-robin).
// If all providers in the tier are unavailable and fallback is enabled,
// moves up one tier (per TierOrder).
func (tc *ModelTierConfig) ResolveTierToRuntimeConfig(
    tierName string,
    excludedProviders map[string]bool, // nil OK — no exclusions
) (*RuntimeConfig, error)

// ResolveTierForRole returns the tier name assigned to a role via role_defaults.
// Returns empty string if the role has no tier assignment.
func (tc *ModelTierConfig) ResolveTierForRole(role string) string

// UpOneTier returns the next tier up in TierOrder, or "" if already at top.
func (tc *ModelTierConfig) UpOneTier(tierName string) string

// TierNames returns tier names in capability order.
func (tc *ModelTierConfig) TierNames() []string
```

**Resolution logic:**
1. Look up tier by name in `Tiers` map
2. Filter providers: skip any where `"provider/model"` is in `excludedProviders`
3. Select provider based on `Selection` strategy:
   - `priority`: first available in list order
   - `round-robin`: next in cycle, tracked per-tier via atomic counter
4. Build `RuntimeConfig` from selected provider:
   - If `Agent` field set → resolve via `GetAgentPresetByName(agent)` → `RuntimeConfigFromPreset()`
   - If provider is `"anthropic"` → build Claude `RuntimeConfig` with `--model <model>` flag
   - If provider is `"ollama"` or `"generic"` → build `RuntimeConfig` from command + model
5. If no providers available and `Fallback` is true → call `UpOneTier()` and recurse
6. If at top tier with no providers → return error

#### 1.4 Integration with Existing Config System

**File:** `internal/config/types.go` — add to `TownSettings`:

```go
type TownSettings struct {
    // ... existing fields ...
    ModelTiers *ModelTierConfig `json:"model_tiers,omitempty"` // Tier-based routing config
}
```

**File:** `internal/config/loader.go` — modify `resolveRoleAgentConfigCore()`:

Insert tier resolution between cost tier lookup and default agent fallback:

```
Current: rig role_agents → town role_agents → cost tier → default_agent
New:     rig role_agents → town role_agents → cost tier → model tier role_defaults → default_agent
```

The cost tier system (`standard`/`economy`/`budget`) remains functional. Cost tiers write to `RoleAgents`, which has higher precedence than `model_tiers.role_defaults`.

#### 1.5 Backward Compatibility with Cost Tiers

The existing `CostTier` system is **preserved as-is**. It writes directly to `RoleAgents`, which takes precedence over `model_tiers.role_defaults`.

**Coexistence:** Users currently on `economy` or `budget` tiers continue working unchanged. Users who want the new tier system configure `model_tiers` in their settings. Both systems can be active simultaneously — cost tiers effectively override tier defaults for the roles they manage.

**Long-term:** Cost tiers could be reimplemented as named presets that generate a `ModelTierConfig`, but this is not required for Phase 1.

#### 1.6 CLI Commands

**`gt config tier`** — manage model tiers

```bash
# Initialize default tier config (creates model_tiers in settings if absent)
gt config tier init

# Show current tier configuration (tiers, providers, role defaults, selection strategies)
gt config tier show

# Create/update a tier
gt config tier set large --provider anthropic --model claude-opus-4-6
gt config tier set medium --provider anthropic --model claude-sonnet-4-6 --selection round-robin

# Remove a tier
gt config tier remove local

# Map a role to a tier
gt config tier set-role polecat medium
gt config tier set-role witness small

# Add a provider to an existing tier (appends to list)
gt config tier add-provider medium --provider ollama --model qwen2.5-coder:32b
gt config tier add-provider medium --provider openai --model gpt-4.1 --agent opencode

# Remove a provider from a tier
gt config tier remove-provider medium --provider ollama

# Set tier ordering (defines what "up one tier" means)
gt config tier set-order small medium large

# Enable/disable fallback for a tier
gt config tier set large --fallback=false
```

**File:** `internal/cmd/config_tier.go` (new)

**Interaction with `gt config agent`:** These are complementary, not competing:
- `gt config agent` manages **agent presets** (what command to run, what args)
- `gt config tier` manages **tier definitions** (which agent/model for each capability level)
- `gt config cost-tier` is a **quick preset** that writes to `role_agents` (shortcut for common patterns)

A tier's provider can reference a custom agent: first `gt config agent set my-ollama "ollama run ..."`, then `gt config tier add-provider medium --agent my-ollama`.

#### 1.7 Files Changed (Phase 1)

| File | Change |
|------|--------|
| `internal/config/model_tier.go` | **New** — `ModelTier`, `ModelProvider`, `ModelTierConfig` types, resolution logic, round-robin counter |
| `internal/config/model_tier_defaults.go` | **New** — `DefaultModelTierConfig()` |
| `internal/config/model_tier_test.go` | **New** — unit tests: resolution, fallback chains, round-robin, selection strategies, precedence |
| `internal/config/types.go` | **Modified** — add `ModelTiers *ModelTierConfig` field to `TownSettings` |
| `internal/config/loader.go` | **Modified** — integrate tier resolution into `resolveRoleAgentConfigCore()` |
| `internal/config/loader_test.go` | **Modified** — test tier integration in resolution chain |
| `internal/cmd/config_tier.go` | **New** — `gt config tier` CLI command tree |
| `internal/cmd/config.go` | **Modified** — register `tier` subcommand under `configCmd` |

---

### Phase 2: Mayor Override API + Witness Model Failure Mail

**Goal:** Add `--tier` flag to `gt sling` for explicit tier override. Add MODEL_FAILURE mail protocol so the witness can inform the router about provider failures.

#### 2.1 Spawn Interface Changes

**File:** `internal/cmd/polecat_spawn.go`

Extend `SlingSpawnOptions`:
```go
type SlingSpawnOptions struct {
    // ... existing fields ...
    Agent      string // existing: agent preset override (e.g., "gemini", "codex")
    ModelTier  string // NEW: tier override (e.g., "large", "small")
}
```

**Semantics:**
- `--agent codex` → use the codex agent preset (existing behavior, unchanged)
- `--tier large` → resolve "large" via `ModelTierConfig`, use resulting `RuntimeConfig`
- Both specified → error (mutually exclusive)
- Neither specified → normal resolution chain

#### 2.2 CLI Flag

**File:** `internal/cmd/sling.go`

```go
slingCmd.Flags().StringVar(&slingTier, "tier", "",
    "Override model tier for this sling (e.g., small, medium, large)")
```

#### 2.3 Resolution in Session Start

**File:** `internal/polecat/session_manager.go`

In `SessionManager.Start()`, add tier resolution before agent override handling:

```go
if opts.ModelTier != "" {
    tierConfig := config.LoadModelTierConfig(townRoot)
    cache := routing.BuildExclusionCache(townRoot, routerFailureTTL) // reads + marks read
    rc, err := tierConfig.ResolveTierToRuntimeConfig(opts.ModelTier, cache.ExcludedSet())
    if err != nil {
        // No routable models — send ROUTING_FAILED mail to mayor
        routing.SendRoutingFailedMail(townRoot, opts.ModelTier, bead.ID, cache)
        return fmt.Errorf("tier override %q: %w", opts.ModelTier, err)
    }
    runtimeConfig = rc
} else if opts.Agent != "" {
    // existing agent override path
}
```

#### 2.4 Model Failure Mail — Reading

**File:** `internal/routing/mail.go` (new)

```go
package routing

// BuildExclusionCache reads the router/ inbox for MODEL_FAILURE mail within TTL,
// parses provider/model/reason from message bodies, marks processed messages as read,
// and returns an in-memory ExclusionCache for use during this dispatch cycle.
func BuildExclusionCache(townRoot string, ttl time.Duration) *ExclusionCache

// SendRoutingFailedMail sends a ROUTING_FAILED message to the mayor
// when no routable models are available for a tier.
func SendRoutingFailedMail(townRoot, tier, beadID string, cache *ExclusionCache) {
    // gt mail send mayor/ -s "ROUTING_FAILED: no routable models for <tier>" -m "..."
}
```

#### 2.5 Witness Integration — Model Failure Detection and Mail

**File:** `internal/witness/model_failure.go` (new)

```go
// ClassifyModelFailure determines if a polecat failure is attributable to the
// model/provider rather than the task.
//
// Model failure heuristics:
// - Credit exhaustion: exit output contains "credit", "billing", "quota", "insufficient funds"
// - Rate limit: exit output contains "rate limit", "429", "too many requests", "resource_exhausted"
// - Network: exit output contains "connection refused", "timeout", "ECONNRESET", "ETIMEDOUT"
// - Crash loop: same bead respawned >= 2 times with same model, no code changes between attempts
//
// Returns the failure report if classified as model failure, nil otherwise.
func ClassifyModelFailure(
    zombie ZombieInfo,
    agentBead *BeadRecord,
    resolvedModel string,
) *ModelFailureReport

// ModelFailureReport contains the classification result.
type ModelFailureReport struct {
    Provider string // e.g., "anthropic"
    Model    string // e.g., "opus"
    Reason   string // "credit_exhaustion", "rate_limit", "network", "crash_loop"
    BeadID   string
    Rig      string
    Polecat  string
}

// SendModelFailureMail sends a MODEL_FAILURE message to router/ via gt mail.
func (r *ModelFailureReport) SendMail(townRoot string) error {
    // gt mail send router/ \
    //   -s "MODEL_FAILURE: <provider>/<model>" \
    //   -m "provider: <provider>\nmodel: <model>\nreason: <reason>\nbead: <bead>\nrig: <rig>\npolecat: <polecat>\ntimestamp: <now>"
}
```

**File:** `internal/witness/handlers.go` — integrate into zombie handling

In the existing `DetectZombiePolecats()` flow, after zombie classification and before restart:

```go
// After classifying zombie type, check if it's a model failure
if report := ClassifyModelFailure(zombie, agentBead, resolvedModel); report != nil {
    log.Warn("model failure detected, notifying router",
        "provider", report.Provider,
        "model", report.Model,
        "reason", report.Reason,
        "bead", report.BeadID,
    )
    if err := report.SendMail(townRoot); err != nil {
        log.Error("failed to send MODEL_FAILURE mail", "err", err)
    }
}
// Continue with existing zombie handling (restart, escalate, etc.)
```

**What the witness does NOT do:**
- Judge whether the router made a good tier selection
- Override the router's decision
- Retry with a different tier (that happens on re-dispatch when the router reads its mail)

The witness reports facts via mail. The router acts on them at dispatch time.

#### 2.6 Observability

All overrides (agent or tier) are logged with the polecat's spawn metadata:

```go
log.Info("polecat spawned",
    "polecat", polecatName,
    "agent", opts.Agent,
    "tier_override", opts.ModelTier,
    "resolved_agent", runtimeConfig.ResolvedAgent,
    "resolved_model", extractModel(runtimeConfig),
)
```

OTEL events for model failures:
```
gt.routing.model_failure_reported{provider, model, reason, rig}
gt.routing.routing_failed{tier, bead, excluded_count}
gt.routing.fallback_triggered{from_tier, to_tier}
```

#### 2.7 Files Changed (Phase 2)

| File | Change |
|------|--------|
| `internal/cmd/sling.go` | **Modified** — add `--tier` flag |
| `internal/cmd/polecat_spawn.go` | **Modified** — add `ModelTier` to `SlingSpawnOptions`, tier resolution with mail-based exclusion |
| `internal/cmd/sling_target.go` | **Modified** — pass `ModelTier` through to spawn opts |
| `internal/polecat/session_manager.go` | **Modified** — handle `ModelTier` in `Start()` |
| `internal/routing/mail.go` | **New** — `BuildExclusionCache()`, `ExclusionCache`, `SendRoutingFailedMail()` |
| `internal/routing/mail_test.go` | **New** — unit tests: mail parsing, TTL filtering, mark-as-read, exclusion cache |
| `internal/witness/model_failure.go` | **New** — `ClassifyModelFailure()`, `ModelFailureReport`, `SendMail()` |
| `internal/witness/model_failure_test.go` | **New** — unit tests: classification heuristics |
| `internal/witness/handlers.go` | **Modified** — integrate `ClassifyModelFailure` into zombie detection |
| `internal/cmd/status.go` | **Modified** — add router status section (provider up/down, exclusions, recent mail) |

---

### Phase 3: LLM Router (Opt-in)

**Goal:** An optional, dedicated LLM evaluates task complexity at dispatch time and selects the appropriate tier. The router always commits to a decision (no confidence threshold). MODEL_FAILURE mail from the witness is included in the router's context.

#### 3.1 Router Configuration

**File:** `internal/config/types.go` — add to `TownSettings`:

```go
type RouterConfig struct {
    Enabled          bool   `json:"enabled"`             // default: false
    Provider         string `json:"provider"`            // "ollama", "anthropic", etc.
    Model            string `json:"model"`               // e.g., "qwen2.5-coder:14b"
    FallbackTier     string `json:"fallback_tier"`       // tier if router LLM fails (default: "medium")
    TimeoutMs        int    `json:"timeout_ms"`          // default: 5000
    FailureTTLMinutes int   `json:"failure_ttl_minutes"` // how long MODEL_FAILURE mail stays relevant (default: 30)
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

// RoutingRequest is the input to the router.
type RoutingRequest struct {
    TaskTitle         string          `json:"task_title"`
    TaskDescription   string          `json:"task_description"`
    Role              string          `json:"role"`
    AvailableTiers    []TierSummary   `json:"available_tiers"`
    ExcludedProviders []ExclusionInfo `json:"excluded_providers"` // from MODEL_FAILURE mail
}

// TierSummary describes a tier for the router's context.
type TierSummary struct {
    Name       string   `json:"name"`
    Providers  []string `json:"providers"`  // available (non-excluded) provider names
    Capability string   `json:"capability"` // "low", "medium", "high" — derived from TierOrder position
}

// ExclusionInfo tells the router what's currently unavailable (from mail).
type ExclusionInfo struct {
    Provider string `json:"provider"`
    Model    string `json:"model"`
    Reason   string `json:"reason"`
    Age      string `json:"age"` // e.g., "5 minutes ago"
}

// RoutingDecision is the structured output from the router.
// No confidence field — the router always commits.
type RoutingDecision struct {
    Tier      string `json:"tier"`
    Rationale string `json:"rationale"`
}

// Router evaluates task complexity and returns a tier recommendation.
type Router interface {
    Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error)
}
```

#### 3.3 LLM Router Implementation

**File:** `internal/routing/llm_router.go` (new)

```go
// LLMRouter calls a local or remote LLM to evaluate task complexity.
type LLMRouter struct {
    config     *config.RouterConfig
    client     LLMClient
    tierConfig *config.ModelTierConfig
}

func (r *LLMRouter) Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error) {
    // 1. Build prompt from template + request (includes tier ordering + exclusions from mail)
    // 2. Call LLM with JSON mode / structured output
    // 3. Parse response into RoutingDecision
    // 4. Validate tier name exists in config
    // 5. Record OTEL metrics (latency, tier chosen)
    // 6. On any error (timeout, parse failure, invalid tier) → return fallback_tier
}
```

**Router prompt template** (embedded):
```
You are a task complexity classifier for a software engineering automation system.

Given a task description, select the most appropriate model tier.

## Available Tiers (ordered by capability, lowest to highest)
{{range .Tiers}}
- **{{.Name}}** ({{.Capability}}): Providers: {{.Providers}}
{{end}}

## Currently Unavailable Providers
{{range .Excluded}}
- {{.Provider}}/{{.Model}}: {{.Reason}} ({{.Age}})
{{end}}

## Guidelines
- Use the lowest tier that can handle the task well (cost efficiency)
- Tasks affecting multiple packages/modules or requiring architectural understanding → higher tier
- Mechanical changes (rename, format, single-function fix) → lower tier
- Security-sensitive changes → higher tier
- If the natural tier's providers are all unavailable, select the next higher tier

## Task
Title: {{.Title}}
Description: {{.Description}}
Role: {{.Role}}

Respond with JSON only: {"tier": "...", "rationale": "one sentence"}
```

#### 3.4 LLM Client Abstraction

**File:** `internal/routing/llm_client.go` (new)

```go
// LLMClient abstracts the LLM provider for the router.
type LLMClient interface {
    Complete(ctx context.Context, prompt string) (string, error)
}

// OllamaClient calls a local Ollama instance.
type OllamaClient struct {
    baseURL string // default: http://localhost:11434
    model   string
}

// AnthropicClient calls the Anthropic Messages API.
type AnthropicClient struct {
    apiKey string
    model  string
}

// NewLLMClient creates the appropriate client based on provider name.
func NewLLMClient(provider, model string) (LLMClient, error)
```

#### 3.5 Integration with Dispatch

**File:** `internal/cmd/polecat_spawn.go`

In `SpawnPolecatForSling()`, after resolving the bead but before session start:

```go
// Router evaluation (opt-in)
if routerConfig := loadRouterConfig(townRoot); routerConfig != nil && routerConfig.Enabled {
    if opts.Agent == "" && opts.ModelTier == "" { // don't route if explicit override
        tierConfig := config.LoadModelTierConfig(townRoot)
        cache := routing.BuildExclusionCache(townRoot, routerFailureTTL) // reads + marks read

        decision, err := router.Route(ctx, routing.RoutingRequest{
            TaskTitle:         bead.Title,
            TaskDescription:   bead.Body,
            Role:              "polecat",
            AvailableTiers:    tierConfig.BuildTierSummaries(cache.ExcludedSet()),
            ExcludedProviders: cache.AsExclusionInfoList(),
        })
        if err == nil && decision != nil {
            opts.ModelTier = decision.Tier
            log.Info("router selected tier",
                "tier", decision.Tier,
                "rationale", decision.Rationale,
            )
        } else {
            log.Warn("router failed, using fallback",
                "err", err,
                "fallback_tier", routerConfig.FallbackTier,
            )
            opts.ModelTier = routerConfig.FallbackTier
        }

        // Resolve tier to RuntimeConfig, excluding failed providers
        rc, resolveErr := tierConfig.ResolveTierToRuntimeConfig(opts.ModelTier, cache.ExcludedSet())
        if resolveErr != nil {
            routing.SendRoutingFailedMail(townRoot, opts.ModelTier, bead.ID, cache)
            return resolveErr
        }
        runtimeConfig = rc
    }
}
```

**Critical:** Router is bypassed when `--agent` or `--tier` is specified. Explicit overrides always win.

#### 3.6 CLI Commands

```bash
# Enable the router
gt config router enable --provider ollama --model qwen2.5-coder:14b

# Disable the router
gt config router disable

# Show router config + recent MODEL_FAILURE mail
gt config router show

# Test the router against a bead without dispatching
gt config router test <bead-id>
# Output: "Router decision: tier=large, rationale='Cross-cutting refactor across 3 packages'"
# Also shows: "Excluded providers (from recent mail): anthropic/opus (credit_exhaustion, 12min ago)"

# Set fallback tier (used when router LLM itself fails)
gt config router set-fallback medium

# Set timeout
gt config router set-timeout 3000

# Set how long MODEL_FAILURE mail stays relevant
gt config router set-failure-ttl 45
```

#### 3.7 Observability

**OTEL metrics** (via existing telemetry infrastructure):

| Metric | Type | Labels |
|--------|------|--------|
| `gt.router.calls` | Counter | `provider`, `model`, `outcome` (success/fallback/error) |
| `gt.router.latency_ms` | Histogram | `provider`, `model` |
| `gt.router.tier_selected` | Counter | `tier`, `role` |
| `gt.routing.model_failure_reported` | Counter | `provider`, `model`, `reason` |
| `gt.routing.routing_failed` | Counter | `tier`, `excluded_count` |
| `gt.routing.fallback_triggered` | Counter | `from_tier`, `to_tier` |

#### 3.8 Files Changed (Phase 3)

| File | Change |
|------|--------|
| `internal/routing/router.go` | **New** — `Router` interface, request/response types, `TierSummary` |
| `internal/routing/llm_router.go` | **New** — LLM-based router implementation |
| `internal/routing/llm_client.go` | **New** — `LLMClient` interface, Ollama + Anthropic implementations |
| `internal/routing/prompt.go` | **New** — router prompt template |
| `internal/routing/router_test.go` | **New** — unit tests with mock LLM client |
| `internal/config/types.go` | **Modified** — add `RouterConfig` to `TownSettings` |
| `internal/cmd/polecat_spawn.go` | **Modified** — integrate router into dispatch flow |
| `internal/cmd/config_router.go` | **New** — `gt config router` CLI commands |
| `internal/cmd/config.go` | **Modified** — register `router` subcommand |

---

## Precedence Chain (Final)

```
    ┌─────────────────────────────────────────────┐
    │  1. CLI: gt sling --agent codex             │  Explicit agent preset
    │  2. CLI: gt sling --tier large              │  Explicit tier override
    ├─────────────────────────────────────────────┤
    │  3. Rig settings: role_agents / worker_agents│  Per-rig config
    │  4. Town settings: role_agents / crew_agents │  Town-wide config
    │     (also written by gt config cost-tier)    │
    ├─────────────────────────────────────────────┤
    │  5. LLM Router decision (if enabled)        │  Dynamic (Phase 3)
    ├─────────────────────────────────────────────┤
    │  6. model_tiers.role_defaults               │  Tier-based default (Phase 1)
    ├─────────────────────────────────────────────┤
    │  7. Town default_agent                       │  Final fallback
    └─────────────────────────────────────────────┘

    At ALL levels where tier resolution occurs, MODEL_FAILURE
    mail to router/ is checked. Excluded providers are skipped.
    If a tier is fully excluded, resolution moves UP one tier
    (per TierOrder). If no tier has available providers,
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
              │  Router (opt-in, Phase 3)           │
              │  Input: bead, role, tiers           │
              │  Reads: router/ inbox               │
              │     (MODEL_FAILURE mail < 30min)    │
              │  Output: tier name                  │
              └─────────────────┬──────────────────┘
                                │
              ┌─────────────────▼──────────────────┐
              │  Tier Resolution (Phase 1)          │
              │  tier name → provider selection      │
              │  strategy: priority | round-robin    │
              │  skips: excluded providers from mail │
              └─────────┬─────────────┬────────────┘
                        │             │
                  (success)    (no providers)
                        │             │
                        │      ┌──────▼──────┐
                        │      │ ROUTING_    │
                        │      │ FAILED mail │──► mayor/
                        │      └─────────────┘
              ┌─────────▼──────────────────┐
              │  RuntimeConfig              │
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
                        │ model failure?
          ┌─────────────▼───────────────┐
          │  ClassifyModelFailure()     │
          │  (credit, rate limit,       │
          │   network, crash loop)      │
          └─────────────┬───────────────┘
                        │ yes
          ┌─────────────▼───────────────┐
          │  gt mail send router/       │
          │  -s "MODEL_FAILURE: ..."    │───► router/ inbox
          └─────────────────────────────┘
                        ▲
                        │ read at next dispatch
                        │ (closes the feedback loop)
```

---

## Testing Strategy

### Unit Tests
- **Tier resolution:** correct provider selection per strategy, fallback chains, tier ordering
- **Round-robin:** verify cycling, skip excluded, reset on config change
- **Mail parsing:** MODEL_FAILURE subject/body parsing, TTL filtering, exclusion set building
- **Model failure classification:** heuristic matching for credit/rate-limit/network/crash patterns
- **Precedence:** verify each layer overrides the one below it
- **Config loading:** valid/invalid JSON, missing fields, defaults applied
- **Router:** mock LLM responses, fallback on timeout, invalid tier name handling

### Integration Tests
- `gt sling --tier large` spawns polecat with correct model
- Cost tier + model tier coexistence (cost tier wins via RoleAgents)
- Round-robin distribution across providers in a tier
- MODEL_FAILURE mail → tier resolution skips excluded provider → falls back up
- ROUTING_FAILED mail sent to mayor when all providers exhausted
- Witness detects model failure → sends MODEL_FAILURE mail → next dispatch avoids provider
- Router opt-in/opt-out: dispatch with/without router enabled

### Manual Testing
- Configure multi-provider tier with round-robin, observe distribution in logs
- Send a MODEL_FAILURE mail manually, verify dispatch routes to alternative
- Enable router with local Ollama, verify routing decisions in logs
- `gt config tier show` + `gt config router show` display correct state
- `gt config router test <bead>` shows dry-run decision with current exclusions

---

## Migration & Rollout

1. **Phase 1 ships first** — no breaking changes, purely additive config
2. Existing `gt config agent`, `gt config cost-tier`, and cost tiers continue working unchanged
3. Users adopt `model_tiers` when they want multi-provider, selection strategies, or custom tier definitions
4. Phase 2 adds `--tier` flag and MODEL_FAILURE mail protocol — no behavior change unless used
5. Phase 3 is fully opt-in — zero impact unless `router.enabled = true`

**No migration required.** Users with no `model_tiers` config get identical behavior to today. The `DefaultModelTierConfig()` is only materialized when the tier system is explicitly invoked (via `--tier` flag or `gt config tier` commands).

---

## Out of Scope (Future Work)

- **Latency-based provider selection** — requires health-checking infrastructure
- **Per-bead cost tracking with router attribution** — wait for cost reporting v2
- **Router learning from outcomes** — would require a task quality judge, which we've decided we don't have
- **Vision/reasoning/code-specialized tiers** — custom tier names support this, but no built-in specialization logic
- **Cross-rig tier inheritance** — each rig resolves independently; shared config is at town level
- **Automatic tier suggestion** — "you should upgrade this tier based on failure patterns" (future witness intelligence)
