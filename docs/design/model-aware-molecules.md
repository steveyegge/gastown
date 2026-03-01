# Model-Aware Molecule Constraints

> Plan for adding model-specific constraints to molecule steps with subscription-aware routing.

**Status**: In Progress
**Owner**: Design
**Related**: [molecules.md](../concepts/molecules.md) | [agent-provider-interface.md](agent-provider-interface.md)

---

## Relationship with Consensus

Consensus and model-aware molecules are **complementary layers** that share the same session awareness infrastructure but serve different purposes:

| | Consensus | Molecules |
|---|---|---|
| **Pattern** | Fan-out | DAG routing |
| **Shape** | Same prompt → N agents → compare | N steps → best model per step |
| **Session infra** | `GT_AGENT` + `AgentPresetInfo` readiness | Same — reused, not rebuilt |
| **Routing goal** | Diversity (multiple perspectives) | Optimality (right model for each step) |

The provider resolution pipeline that Consensus v2 established — `GT_AGENT` env lookup → `AgentPresetInfo` → readiness detection (prompt polling or delay fallback) — is exactly the session awareness the molecule router needs for dispatch. See §5.3 (Two-Phase Routing).

---

## 1. Introduction / Overview

Molecules currently support dependency-based DAG execution, but lack the ability to specify **which AI model** should execute each step. With multiple AI providers (Anthropic, OpenAI, DeepSeek, Google, etc.) and access types (API keys and subscriptions like Claude Code), we need:

1. **Per-step model constraints** — Specify required model or capability per step
2. **Subscription support** — Support Claude Code and other subscription-based access (crucial for cost optimization)
3. **Automatic pricing data** — Fetch live pricing from OpenRouter; fall back to cached data
4. **Meta-model routing** — Lightweight heuristic selects model based on cost, quality, and quota
5. **Local usage tracking** — Record invocations to `~/.gt/usage.jsonl` (OTel additive/optional)

---

## 2. Design Goals

| Goal | Description |
|-------|-------------|
| **Molecule-Level Constraints** | Add model/capability constraints to molecule steps |
| **Subscription Support** | Support both API key AND subscription-based access |
| **Live Pricing** | Fetch pricing from OpenRouter with 24h local cache |
| **Static Benchmarks** | Bundle MMLU/SWE scores; override via `~/.gt/models.toml` |
| **Meta-Model Routing** | Heuristic-only scoring: no LLM calls |
| **Local Usage Tracking** | `~/.gt/usage.jsonl` always written; OTel is additive |
| **DAG Compatible** | Works with existing molecule DAG structure |
| **Backward Compatible** | Existing formulas work without modification |

---

## 3. Quality Gates

All implementation stories in this plan must pass these quality gates:

- `go test ./...`
- `golangci-lint run`
- Manual verification of subscription access detection

---

## 4. User Stories

### US-001: Subscription-Based Access Configuration

**Description**: As a Gas Town operator, I want to configure Claude Code subscription so that it is automatically preferred over API keys due to cost reasons.

**Acceptance Criteria**:
- [ ] Environment variable `CLAUDE_CODE_SUBSCRIPTION=active` enables subscription detection
- [ ] Subscription metadata (plan type, account) is read from env vars
- [ ] Subscription access is preferred over API key when both are available
- [ ] `bd ready --json` includes subscription quota information

### US-002: Model Capability Database

**Description**: As a developer, I want a built-in database of model capabilities (MMLU, SWE, costs) that is used by the routing system without requiring manual configuration.

**Acceptance Criteria**:
- [x] `internal/models/database.go` contains static model entries with benchmark scores
- [x] Pricing is fetched from OpenRouter (`https://openrouter.ai/api/v1/models`) with 24h cache
- [x] Cache stored at `~/.gt/models_pricing_cache.json`; fetching fails gracefully (zero pricing used)
- [x] `~/.gt/models.toml` overrides or extends any field including prices, benchmarks, new models
- [x] `GetModel(db, id)` returns model metadata or nil
- [x] `LoadDatabase(gtDir)` = static + OpenRouter pricing + user overrides

### US-003: Meta-Model Routing Logic

**Description**: As a system, I want a lightweight routing algorithm that selects which model to use based on task requirements and cost constraints without calling another LLM.

**Acceptance Criteria**:
- [x] `internal/models/router.go` implements `SelectModel()` with heuristics only
- [x] Routing considers: provider, access_type, min_mmlu, min_swe, requires, max_cost
- [x] Subscription access is preferred when available (cost = $0)
- [x] Decision includes: selected model, reason, cost, MMLU/SWE scores
- [x] Returns error when no model satisfies the constraints

### US-004: Molecule Step Constraint Syntax

**Description**: As a formula author, I want to specify model constraints in molecule steps using a simple TOML syntax.

**Acceptance Criteria**:
- [x] Steps support `model = "claude-sonnet-4-5"` for exact model
- [x] Steps support `provider = "anthropic"` for any model from a provider
- [x] Steps support `model = "auto"` for heuristic routing
- [x] Steps support `min_mmlu = 85` and `min_swe = 70` for quality thresholds
- [x] Steps support `requires = ["vision", "code_execution"]` for capability constraints
- [x] Steps support `access_type = "subscription"` to require subscription access
- [x] Steps support `max_cost = 0.01` for cost constraints (USD per 1K tokens, combined)
- [x] Parser validates all new fields; rejects unknown capabilities and invalid ranges
- [x] `model` and `provider` cannot be set simultaneously (parser error)

### US-005: Usage Tracking

**Description**: As a system, I want to track model usage locally so that operators can monitor costs without depending on OTel.

**Acceptance Criteria**:
- [x] `internal/models/usage.go` records usage to `~/.gt/usage.jsonl` (always)
- [x] Each entry: timestamp, model ID, provider, access_type, tokens in/out, cost, success, latency, reason
- [x] `LoadUsage(gtDir, since)` reads and filters entries
- [x] `MonthlyStats(entries, year, month)` aggregates by model
- [x] `TotalCost(entries)` sums USD cost
- [x] OTel integration is additive — callers emit OTel events separately if `GT_OTEL_LOGS_URL` is set

### US-006: Enhanced `gt prime` with Model Info

**Description**: As an operator, I want `gt prime` to show which models are available for each step and which model will be used.

**Acceptance Criteria**:
- [ ] Each step shows: constraint type, recommended model, access type, estimated cost
- [ ] Fallback models are listed when primary is unavailable
- [ ] `gt step <step-id>` executes a specific step with model routing
- [ ] Visual indicators: `✓ subscription` vs `$0.003/K api_key`

### US-007: Batch DAG Execution with Model Assignment

**Description**: As an operator, I want to execute an entire molecule with automatic model assignment per step.

**Acceptance Criteria**:
- [ ] `gt mol execute --auto-route <mol-id>` reads constraints and routes per step
- [ ] Parallel steps execute simultaneously when available
- [ ] Failed routing shows which constraint could not be satisfied

### US-008: Usage Reporting CLI

**Description**: As an operator, I want `gt usage` to show comprehensive usage statistics.

**Acceptance Criteria**:
- [ ] `gt usage` shows monthly summary: total cost, invocations, subscription uses
- [ ] Table: provider, model, tokens, cost, success rate
- [ ] `gt usage --month 2025-02` filters to a specific month
- [ ] Historical data loaded from `~/.gt/usage.jsonl`

---

## 5. Technical Design

### 5.1 Access Type

```go
// internal/models/database.go

// SubscriptionEligible bool on ModelEntry indicates the model can be accessed
// via a subscription (e.g. Claude Code for Anthropic models).
// The caller detects subscription availability from env vars and passes it
// as StepConstraints.SubscriptionActive.
```

Note: Claude Code is an **access method**, not a model. Do not create a fake `"claude-code"` model entry. The correct modelling is `SubscriptionEligible: true` on Anthropic model entries and `AccessType: "subscription"` on the routing decision when a subscription is active.

### 5.2 Model Capability Database

```go
// internal/models/database.go

type ModelEntry struct {
    ID            string   // "claude-sonnet-4-5"
    Provider      string   // "anthropic"
    Name          string   // "Claude Sonnet 4.5"
    OpenRouterID  string   // "anthropic/claude-sonnet-4-5" (for pricing fetch)

    // Benchmark scores (static, overridable via ~/.gt/models.toml)
    MMLUScore     float64
    SWEScore      float64

    // Capabilities
    Vision        bool
    CodeExecution bool
    ContextWindow int

    // Pricing in USD per 1K tokens (fetched from OpenRouter, cached 24h)
    CostPer1KIn   float64
    CostPer1KOut  float64

    SubscriptionEligible bool
    GoodFor              []string
}

// LoadDatabase merges: static benchmarks → OpenRouter pricing → ~/.gt/models.toml overrides
func LoadDatabase(gtDir string) []ModelEntry
```

**External pricing source**: OpenRouter (`https://openrouter.ai/api/v1/models`)
- No API key required
- Returns per-token pricing for hundreds of models
- Response cached to `~/.gt/models_pricing_cache.json` for 24h
- Fetch timeout: 5s; failures are non-fatal (zero pricing used as fallback)

**Benchmark data**: Bundled statically in `staticDB` (from published evaluations).
Override or extend via `~/.gt/models.toml`:

```toml
# Override a built-in model's benchmark
[models.claude-sonnet-4-5]
mmlu = 84.5
swe = 52.0

# Add a new model not in the static DB
[models.my-local-model]
provider = "custom"
mmlu = 70.0
cost_per_1k_in = 0.0
cost_per_1k_out = 0.0
good_for = ["coding"]
```

### 5.3 Two-Phase Routing

Routing happens in two sequential phases. No LLM calls are made at any point.

#### Phase 1 — Model Selection (`SelectModel`)

Picks the optimal model from the capability database based on step constraints and scoring heuristics.

```go
// internal/models/router.go

type StepConstraints struct {
    Model      string   // exact ID or "auto"
    Provider   string
    AccessType string   // "subscription" | "api_key"
    MinMMLU    float64
    MinSWE     float64
    Requires   []string
    MaxCost    float64  // USD per 1K tokens (combined)
    // Filled by caller from env/config:
    SubscriptionActive bool
}

type RoutingDecision struct {
    // Model selection (Phase 1)
    ModelID      string
    Provider     string
    AccessType   string   // "subscription" | "api_key"
    Reason       string
    CostPer1KIn  float64
    CostPer1KOut float64
    MMLUScore    float64
    SWEScore     float64

    // Session resolution (Phase 2) — nil when no live session found
    SessionID    string   // tmux session name, e.g. "gt-gastown-polecat-Toast"
    AgentPreset  string   // resolved GT_AGENT value, e.g. "claude", "gemini"
}

func SelectModel(constraints StepConstraints, db []ModelEntry) (*RoutingDecision, error)
```

Scoring:

| Factor | Weight |
|--------|--------|
| Subscription active + model eligible | +40 pts |
| MMLU score (normalized 0–100) | up to 30 pts |
| SWE score (normalized 0–100) | up to 20 pts |
| Cost savings (inverse of $0.10/1K ceiling) | up to 10 pts |

#### Phase 2 — Session Resolution (`ResolveSession`)

After a model is selected, find a **live, idle tmux session** running that model. This reuses the existing `GT_AGENT` + `AgentPresetInfo` infrastructure from the provider resolution pipeline — the same logic Consensus v2 uses.

```go
// internal/models/router.go (planned)

// ResolveSession scans running tmux sessions and returns the first one that is
// idle and running the selected model. Returns nil if no matching session is found.
//
// Resolution:
//  1. List active tmux sessions
//  2. Read GT_AGENT env var from each session
//  3. Look up AgentPresetInfo for that agent name
//  4. Check readiness: prompt polling (ReadyPromptPrefix e.g. "❯ ") or delay fallback (ReadyDelayMs)
//  5. Return first session that matches ModelID and is idle
func ResolveSession(decision *RoutingDecision, tmux Tmux) *RoutingDecision
```

**Readiness detection** is taken directly from `AgentPresetInfo` — no new mechanism:

| Agent type | Detection method | Source |
|---|---|---|
| Claude | Prompt prefix polling (`❯ `) | `AgentPresetInfo.ReadyPromptPrefix` |
| OpenCode, Codex | Delay-based fallback | `AgentPresetInfo.ReadyDelayMs` |
| Custom agents | Delay-based fallback | Same |

**Dispatch outcome**:
- Live idle session found → dispatch step directly to that session
- No matching session → spawn a new session with the selected model (`AgentPresetInfo.Command + Args`)

This means molecule steps target **live sessions by model capability**, not just by name. A step specifying `min_mmlu = 85` will route to whichever idle session happens to be running a qualifying model, without the formula author needing to know session names.

### 5.4 Molecule Step Constraints

```toml
# All constraint fields are optional and backward-compatible.
# Existing steps without constraints accept any available agent.

[[steps]]
id = "analyze-requirements"
title = "Analyze requirements"
needs = ["load-context"]
# Option A: exact model
model = "claude-sonnet-4-5"

[[steps]]
id = "code-generation"
title = "Code generation"
needs = ["analyze-requirements"]
# Option B: heuristic routing with quality and cost constraints
model = "auto"
min_mmlu = 85
min_swe = 50
max_cost = 0.01

[[steps]]
id = "quick-scan"
title = "Quick scan"
# Option C: provider + capability filter
provider = "openai"
requires = ["code_execution"]

[[steps]]
id = "security-audit"
title = "Security audit"
# Option D: prefer subscription (zero cost)
access_type = "subscription"
```

`model` and `provider` are mutually exclusive (parser error if both are set).

### 5.5 Usage Tracking

```go
// internal/models/usage.go

type UsageEntry struct {
    Timestamp  time.Time `json:"timestamp"`
    ModelID    string    `json:"model_id"`
    Provider   string    `json:"provider"`
    AccessType string    `json:"access_type"`
    TaskType   string    `json:"task_type"`
    TokensIn   int       `json:"tokens_in"`
    TokensOut  int       `json:"tokens_out"`
    CostUSD    float64   `json:"cost_usd"`
    Success    bool      `json:"success"`
    LatencyMs  int       `json:"latency_ms"`
    Reason     string    `json:"reason,omitempty"`
}

func RecordUsage(gtDir string, entry UsageEntry) error     // appends to usage.jsonl
func LoadUsage(gtDir string, since time.Time) ([]UsageEntry, error)
func MonthlyStats(entries []UsageEntry, year int, month time.Month) map[string]*ModelStats
func EstimateCost(model *ModelEntry, tokensIn, tokensOut int) float64
```

**OTel integration**: callers that want OTel observability emit an `agent.usage` OTel log event separately (see `docs/otel-data-model.md`). `usage.jsonl` is always written and does not depend on OTel being configured.

---

## 6. Environment Variables

```bash
# Subscription detection
export CLAUDE_CODE_SUBSCRIPTION=active    # enables subscription preference
export CLAUDE_CODE_ACCOUNT=user@co.com   # informational
export CLAUDE_CODE_PLAN=pro              # informational

# API Key Access (existing)
export ANTHROPIC_API_KEY=sk-ant-xxx
export OPENAI_API_KEY=sk-openai-xxx
export GOOGLE_API_KEY=xxx
export DEEPSEEK_API_KEY=xxx

# Model Defaults (new)
export GT_DEFAULT_MODEL=claude-sonnet-4-5  # fallback for unconstrained steps
export GT_PREFERRED_PROVIDER=anthropic

# Thresholds (new)
export GT_MIN_MMLU=80
export GT_MIN_SWE=50
export GT_MAX_COST=0.005

# Usage tracking
export GT_TRACK_USAGE=true   # default true; set false to disable
```

Note: `CLAUDE_CODE_QUOTA` is **not** a real env var — Claude Code does not expose token quota programmatically. If quota tracking is needed, derive it from `~/.gt/usage.jsonl` entries with `access_type="subscription"`.

---

## 7. Configuration Files

### `~/.gt/models.toml` — Model Database Override

```toml
# Override built-in benchmark scores
[models.claude-sonnet-4-5]
mmlu = 84.5

# Add a new model
[models.deepseek-v3-local]
provider = "deepseek"
mmlu = 88.0
swe = 48.0
cost_per_1k_in = 0.00014
cost_per_1k_out = 0.00028
context_window = 131072
good_for = ["coding", "reasoning"]
```

### `~/.gt/models_pricing_cache.json` — OpenRouter pricing cache (auto-managed)

Written by `LoadDatabase`; refreshed after 24h. Do not edit manually.

---

## 8. CLI Integration

### Step constraints in `gt prime` output (planned)

```
### Step 2: Analyze requirements
  Constraint: model=auto, min_mmlu=85
  Recommended: claude-opus-4-5 (subscription, $0.00)
  Fallback: claude-sonnet-4-5 (api_key, $0.003/1K)

### Step 3: Code generation
  Constraint: provider=openai, requires=[code_execution]
  Recommended: gpt-4o ($0.0025/1K in)
```

### New commands (planned)

```bash
gt step <step-id>                       # execute step with model routing
gt mol execute --auto-route <mol-id>    # batch DAG execution with routing
gt usage                                # monthly cost summary
gt usage --month 2025-02                # filter to specific month
gt model route --task coding --mmlu 85  # debug: test routing logic
```

---

## 9. Formula Examples

### Example 1: Subscription-Preferred Workflow

```toml
formula = "mol-subscription-aware"
version = 1

[[steps]]
id = "code-review"
title = "Code review"
access_type = "subscription"
model = "auto"
description = "Review code changes"

[[steps]]
id = "implement-fixes"
title = "Implement fixes"
needs = ["code-review"]
model = "auto"
description = "Implement the fixes"
```

### Example 2: Multi-Model Code Review

```toml
formula = "mol-multi-model-review"
version = 1

[[steps]]
id = "claude-review"
title = "Review with Claude"
model = "claude-sonnet-4-5"
description = "Review the code changes"

[[steps]]
id = "gpt-review"
title = "Review with GPT-4o"
model = "gpt-4o"
parallel = true
description = "Review the same code"

[[steps]]
id = "synthesize"
title = "Synthesize findings"
needs = ["claude-review", "gpt-review"]
min_mmlu = 85
description = "Combine both reviews"
```

### Example 3: Cost-Optimized Workflow

```toml
formula = "mol-cost-optimized"
version = 1

[[steps]]
id = "quick-scan"
title = "Quick scan"
model = "auto"
max_cost = 0.001
description = "Fast overview with cheapest capable model"

[[steps]]
id = "deep-work"
title = "Deep work"
needs = ["quick-scan"]
model = "auto"
min_mmlu = 85
max_cost = 0.01
description = "Thorough work with quality model within budget"
```

---

## 10. Implementation Phases

### Phase 1: Model Database + Step Constraints (DONE)

- [x] Create `internal/models/database.go` — static benchmarks + OpenRouter pricing + TOML overrides
- [x] Create `internal/models/router.go` — `SelectModel()` heuristic scoring
- [x] Create `internal/models/usage.go` — local JSONL tracking; `MonthlyStats`, `EstimateCost`
- [x] Add routing fields to `internal/formula/types.go` Step struct
- [x] Validate new fields in `internal/formula/parser.go`

### Phase 2: Subscription Discovery (P0)

- [ ] Detect `CLAUDE_CODE_SUBSCRIPTION` and pass `SubscriptionActive` into `StepConstraints`
- [ ] Detect API key env vars (existing pattern) to determine available providers
- [ ] Unit tests for discovery logic

### Phase 3: Session-Aware Dispatch (P0)

Implement `ResolveSession()` using the existing `GT_AGENT` + `AgentPresetInfo` infrastructure:

- [ ] Scan live tmux sessions; read `GT_AGENT` env var per session (already done in `sling_helpers.go`)
- [ ] Look up `AgentPresetInfo` by agent name to get `ReadyPromptPrefix` / `ReadyDelayMs`
- [ ] Implement idle check: prompt polling for agents with `ReadyPromptPrefix`, delay fallback otherwise
- [ ] Return first idle session whose agent matches `RoutingDecision.ModelID`; set `SessionID` + `AgentPreset`
- [ ] If no match: spawn a new session using `AgentPresetInfo.Command + Args` for the selected model
- [ ] Unit tests for session matching and readiness detection

### Phase 4: CLI Integration (P1)

- [ ] Update `gt prime` to show model constraints, routing recommendation, and live session per step
- [ ] Implement `gt step` for single-step execution with two-phase routing
- [ ] Implement `gt mol execute --auto-route` for batch DAG execution
- [ ] Implement `gt usage` and `gt usage --month`

### Phase 5: Usage Recording at Dispatch (P1)

- [ ] Hook `RecordUsage` into the agent dispatch path
- [ ] Derive `TokensIn`/`TokensOut` from `agent.usage` OTel events when available, or estimate
- [ ] OTel: optionally emit `model.route` event when a routing decision is made

---

## 11. Technical Considerations

### Subscription vs API Key Priority

When both subscription and API key are available for the same provider:

1. **Subscription is preferred** — cost is already paid; zero incremental cost
2. **API key is fallback** — used when subscription is not active
3. Subscription quota is **not** exposed programmatically by Claude Code; track from usage.jsonl

### Lightweight Routing

`SelectModel()` is pure heuristics — no LLM calls:

| Factor | Weight | Notes |
|--------|--------|--------|
| Subscription active + model eligible | +40 pts | Free = always prefer |
| MMLU score | up to 30 pts | General knowledge quality |
| SWE score | up to 20 pts | Code-specific quality |
| Cost savings | up to 10 pts | Inverse of $0.10/1K ceiling |
| Quota availability | Hard filter | Applied before scoring |

### Backward Compatibility

Steps without any routing fields accept any idle agent — unchanged behaviour:

```toml
[[steps]]
id = "simple-step"
title = "Simple step"
needs = ["previous-step"]
# No model constraint → any idle agent
```

---

## 12. Success Metrics

- Formula steps can specify per-step model constraints
- Subscription access is detected and preferred over API keys automatically
- Model pricing is fetched from OpenRouter and cached locally (no API key required)
- Usage is tracked locally to `~/.gt/usage.jsonl` regardless of OTel configuration
- Existing formulas continue to work unchanged

---

## 13. Open Questions

| Question | Discussion |
|----------|-------------|
| **Dispatch mechanism** | Resolved: `ResolveSession()` targets the live tmux session directly. The routing decision (`SessionID`, `AgentPreset`) is the dispatch target — no separate env var injection needed. The step description is sent via the existing `tmux send-keys` / nudge path. |
| **Model ID ↔ GT_AGENT mapping** | `GT_AGENT` values are agent preset names (`"claude"`, `"gemini"`), not model IDs (`"claude-sonnet-4-5"`). Need a mapping: `AgentPresetInfo` could carry a `DefaultModelID` field, or sessions could set an additional `GT_MODEL` env var at spawn time for precise matching. |
| **Multiple sessions for same model** | If two Claude sessions are idle and both qualify, which gets the step? Current proposal: first idle session wins (FIFO). Alternative: round-robin or load-based. |
| **Cost-based auto-switch** | Should the system switch to cheaper models mid-session if budget is nearly exhausted? |
| **Model performance learning** | Should historical success rates (from usage.jsonl) influence routing weights? |
| **Multi-subscription support** | Support for multiple Claude Code team subscriptions simultaneously? |
