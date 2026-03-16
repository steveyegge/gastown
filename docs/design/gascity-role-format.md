# Gas City Role Format Specification

**Wasteland:** w-gc-001
**Date:** 2026-03-15
**Author:** gastown/crew/deckard
**Status:** Draft
**Related:** crew-specialization-design.md, PR #2518, PR #2527, beads `agent-cost-optimization`

## Overview

This document specifies the concrete TOML format for Gas City role definitions.
It extends the existing `RoleDefinition` struct with capability advertisements,
tool declarations, context documents, behavioral constraints, and sub-agent
delegation — the building blocks of the cellular model described in
`crew-specialization-design.md`.

### Design Principles

1. **Backward compatible** — existing role TOML files remain valid; new fields
   are additive
2. **Claims first, evidence later** — authored capability profiles are cheap;
   the system populates track records over time
3. **Natural language routing** — examples and anti-examples beat taxonomies
4. **Recursive** — roles can contain sub-roles (the cellular model)
5. **Override chain preserved** — builtin → town → rig layering still works

---

## Format

### Minimal Role (backward-compatible)

Existing roles continue to work unchanged:

```toml
role = "witness"
scope = "rig"
nudge = "Run 'gt prime' to check worker status and begin patrol cycle."
prompt_template = "witness.md.tmpl"

[session]
pattern = "{prefix}-witness"
work_dir = "{town}/{rig}/witness"
needs_pre_sync = false
start_command = "exec claude --dangerously-skip-permissions"

[env]
GT_ROLE = "{rig}/witness"
GT_SCOPE = "rig"

[health]
ping_timeout = "30s"
consecutive_failures = 3
kill_cooldown = "5m"
stuck_threshold = "1h"
```

### Extended Role (Gas City)

New fields are organized under `[capability]`, `[execution]`, `[constraints]`,
and `[[sub_agents]]`. All are optional — a role with none of these sections is
a valid infrastructure-only role.

```toml
role = "security-lead"
scope = "rig"
layer = "crew"                    # crew | polecat | dog | infrastructure
goal = "Handle security-related work for this rig"
nudge = "Check your hook and mail, then act accordingly."
prompt_template = "crew.md.tmpl"

[session]
pattern = "{prefix}-crew-{name}"
work_dir = "{town}/{rig}/crew/{name}"
needs_pre_sync = true
start_command = "exec claude --dangerously-skip-permissions"

[env]
GT_ROLE = "{rig}/crew/{name}"
GT_SCOPE = "rig"

[health]
ping_timeout = "30s"
consecutive_failures = 3
kill_cooldown = "5m"
stuck_threshold = "4h"

# === Gas City extensions below ===

[capability]
# What this role handles — natural language, not taxonomy codes.
# Dispatchers pattern-match incoming tasks against these.
handles = [
  "CORS configuration and debugging",
  "CSP header policy",
  "API key validation and rotation",
  "Rate limiting and abuse prevention",
  "Security audit coordination",
]

# Explicit boundaries — prevents misrouting and wasted bounces.
# Format: "capability description (→ suggested target)"
does_not_handle = [
  "Cryptographic primitives (→ crypto)",
  "User identity/password management (→ identity)",
  "Application-level RBAC (→ owning service)",
  "TLS certificate rotation (→ infra)",
]

# Concrete task descriptions the dispatcher will see.
# Grounded in problem-space language (what the requester says).
example_tasks = [
  "Users getting 403 on cross-origin API calls",
  "Need to add a new allowed origin for partner integration",
  "Security audit of the auth module",
  "Rate limiter is blocking legitimate traffic",
]

# Tasks that sound like they might match but shouldn't route here.
anti_examples = [
  "Need to rotate the TLS certificate",
  "Implement role-based access control for the admin panel",
  "Database encryption at rest",
]

# Paired routing examples (symptom + resolution from real work).
# Initially empty — populated by the system from completed tasks.
# Authored examples are allowed for bootstrapping.
[[capability.routing_examples]]
symptom = "403 errors on cross-origin API calls"
resolution = "CORS allow-origin configuration"
cost_tokens = 8000
complexity = "single-domain"    # single-domain | cross-domain | coordination

[execution]
# Cognition tier — the minimum model capability for this role's own work.
# Sub-agents may use cheaper tiers for delegated subtasks.
# Values: basic (Haiku) | standard (Sonnet) | advanced (Opus) | tool (deterministic)
cognition = "standard"

# Tools this role needs access to. These are resolved at runtime from the
# tool registry (MCP servers, CLI tools, built-in commands).
tools = [
  "cargo-audit",
  "semgrep",
  "CVE-lookup",
  "gt",
  "bd",
]

# Context documents loaded into the agent's prompt at session start.
# Paths are relative to the role's work_dir or the town root.
# Supports glob patterns.
context_docs = [
  "docs/security-policy.md",
  "docs/OWASP-top-10.md",
  "{town}/settings/security-standards.md",
]

# Skills (superpowers plugins) this role should prefer.
# These are suggested, not enforced — the agent can still use other skills.
preferred_skills = [
  "review",
  "superpowers:systematic-debugging",
]

[constraints]
# Behavioral boundaries the agent must respect.
# These are injected into the system prompt.
rules = [
  "Never commit security-sensitive files (.env, credentials, private keys)",
  "Always run semgrep before approving security-related PRs",
  "Escalate any finding rated CRITICAL to the overseer immediately",
]

# Maximum token budget per task (0 = unlimited).
max_tokens_per_task = 100000

# Maximum delegation depth (how many sub-agent layers).
max_delegation_depth = 2

# Whether this role can spawn polecats for subtasks.
can_dispatch = true

# Whether this role can access external services (APIs, web).
allow_external = true

# === Sub-agents: the cellular model ===

[[sub_agents]]
role = "dependency-auditor"
cognition = "basic"
tools = ["cargo-audit"]
goal = "Scan dependencies for known vulnerabilities"
# Inline sub-agents don't need session/health config — they're
# ephemeral (spawned as polecats or subagent tool calls).

[[sub_agents]]
role = "code-reviewer"
cognition = "standard"
tools = ["semgrep"]
goal = "Review code changes for security issues"
context_docs = ["docs/security-review-checklist.md"]
```

---

## Schema Reference

### Top-Level Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `role` | string | yes | — | Role identifier. Infrastructure roles use fixed names; Gas City roles use freeform names. |
| `scope` | string | yes | — | `"town"` or `"rig"` |
| `layer` | string | no | inferred | `"crew"`, `"polecat"`, `"dog"`, or `"infrastructure"`. Inferred from `role` for built-in roles. |
| `goal` | string | no | — | One-line description of what this role does. Used in routing and display. |
| `nudge` | string | no | — | Initial prompt sent when starting the agent. |
| `prompt_template` | string | no | — | Template file for the role's system prompt. |

### `[session]`

Unchanged from current `RoleSessionConfig`. See `internal/config/roles.go`.

### `[env]`

Unchanged. Arbitrary key-value pairs set in the session environment.

### `[health]`

Unchanged from current `RoleHealthConfig`.

### `[capability]`

The capability advertisement. All fields are optional.

| Field | Type | Description |
|-------|------|-------------|
| `handles` | string[] | What this role can do, in natural language |
| `does_not_handle` | string[] | Explicit boundaries with suggested redirects |
| `example_tasks` | string[] | Concrete task descriptions (problem-space language) |
| `anti_examples` | string[] | Tasks that seem to match but shouldn't route here |
| `routing_examples` | array of tables | Paired symptom/resolution from real work (see below) |

#### `[[capability.routing_examples]]`

| Field | Type | Description |
|-------|------|-------------|
| `symptom` | string | What the requester described (problem-space) |
| `resolution` | string | What was actually done (solution-space) |
| `cost_tokens` | int | Approximate token cost (0 if unknown) |
| `complexity` | string | `"single-domain"`, `"cross-domain"`, or `"coordination"` |

### `[execution]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `cognition` | string | `"standard"` | Model tier: `basic`, `standard`, `advanced`, `tool` |
| `tools` | string[] | `[]` | Tool names resolved from tool registry |
| `context_docs` | string[] | `[]` | Paths to docs loaded at session start (supports `{town}`, `{rig}` placeholders and globs) |
| `preferred_skills` | string[] | `[]` | Skills the agent should prefer |

### `[constraints]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rules` | string[] | `[]` | Behavioral rules injected into system prompt |
| `max_tokens_per_task` | int | `0` | Token budget per task (0 = unlimited) |
| `max_delegation_depth` | int | `3` | Maximum sub-agent nesting |
| `can_dispatch` | bool | `true` | Whether this role can spawn polecats |
| `allow_external` | bool | `true` | Whether this role can access external services |

### `[[sub_agents]]`

Inline sub-agent definitions. These are ephemeral — spawned as polecats or
subagent tool calls, not persistent sessions.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `role` | string | yes | Sub-agent role name |
| `cognition` | string | no | Model tier (defaults to `"basic"`) |
| `tools` | string[] | no | Tools available to sub-agent |
| `goal` | string | no | One-line purpose |
| `context_docs` | string[] | no | Docs loaded for sub-agent |

Sub-agents inherit the parent's `scope` and `constraints` unless explicitly
overridden. They do NOT inherit `capability` — that would leak the parent's
routing surface to the child.

---

## Track Record (System-Populated)

The `[track_record]` section is **not authored** — it's populated by the
routing system from observed behavior. Stored alongside the role definition
but in a separate file (`<role>.track.toml`) to keep authored claims separate
from evidence.

```toml
# security-lead.track.toml (system-generated, do not edit)

completions = 12
bounce_rate = 0.15
avg_cost_tokens = 9500
trust_level = "operational"     # speculative | tentative | operational | proven

[[routing_examples]]
symptom = "Partner API returning 403 on preflight"
resolution = "Added partner origin to CORS allowlist, updated CSP connect-src"
cost_tokens = 7200
complexity = "single-domain"
completed_at = 2026-03-10T14:30:00Z

[[proven_boundaries]]
direction = "outbound"          # outbound = "not my problem"
symptom = "JWT token expired but no refresh"
target = "identity"
bounced_at = 2026-03-08T09:15:00Z
```

This separation means:
- Authored role files are safe to edit and commit
- Track records are machine-generated and can be rebuilt from routing history
- The override chain (builtin → town → rig) applies to authored files only

---

## Override and Composition

### Override Chain

The existing three-layer override chain is preserved and extended to Gas City
fields:

1. **Builtin** (`internal/config/roles/<role>.toml`) — compiled into binary
2. **Town** (`<town>/roles/<role>.toml`) — town operator customization
3. **Rig** (`<rig>/roles/<role>.toml`) — rig-specific tuning

Merge semantics for new fields:

| Field | Merge behavior |
|-------|---------------|
| `goal` | Replace |
| `layer` | Immutable (ignored in overrides) |
| `handles` | Append (union) |
| `does_not_handle` | Append (union) |
| `example_tasks` | Append |
| `anti_examples` | Append |
| `routing_examples` | Append |
| `tools` | Append (union) |
| `context_docs` | Append |
| `preferred_skills` | Append |
| `rules` | Append |
| `max_tokens_per_task` | Replace |
| `max_delegation_depth` | Replace (min of base and override) |
| `can_dispatch` | Replace |
| `allow_external` | Replace |
| `sub_agents` | Append |

### Named Roles vs. Built-in Roles

Built-in roles (`mayor`, `witness`, `crew`, etc.) are infrastructure — they
define session lifecycle, health checks, and tmux management. Gas City roles
are **functional specializations** layered on top.

A crew member's actual behavior comes from the composition of:
1. The `crew` infrastructure role (session, health, env)
2. A Gas City role overlay (capability, execution, constraints)

This is expressed by setting `layer = "crew"` in a Gas City role. The runtime
loads the `crew` infrastructure config, then merges the Gas City overlay on top.

```
crew.toml (infrastructure)     security-lead.toml (Gas City)
├── session config              ├── capability
├── health config               ├── execution
├── env vars                    ├── constraints
└── nudge                       └── sub_agents
         ↓                              ↓
         └──────── merged at runtime ───┘
```

### Custom Roles Directory

Gas City roles live in `<town>/gascity/roles/` (town-level) or
`<rig>/gascity/roles/` (rig-level). They're discovered by filename:

```
<town>/gascity/roles/
├── security-lead.toml
├── api-developer.toml
└── docs-writer.toml
```

The runtime maps a crew member to a Gas City role via configuration:

```json
// <town>/settings/config.json
{
  "crew_roles": {
    "deckard": "security-lead",
    "zhora": "api-developer"
  }
}
```

Or via a new field in the role TOML override:

```toml
# <rig>/roles/crew.toml (rig-level override)
# This overrides infrastructure config for all crew in this rig.
# Gas City role assignment is per-worker, not per-role.
```

---

## Examples

### Infrastructure-Only Role (no Gas City extensions)

```toml
# refinery.toml — merge queue processor, no capability routing needed
role = "refinery"
scope = "rig"
nudge = "Check for queued PRs."
prompt_template = "refinery.md.tmpl"

[session]
pattern = "{prefix}-refinery"
work_dir = "{town}/{rig}/refinery"

[health]
ping_timeout = "30s"
consecutive_failures = 3
kill_cooldown = "5m"
stuck_threshold = "1h"
```

### Specialized Crew Member

```toml
# docs-writer.toml — a crew member specialized for documentation work
role = "docs-writer"
scope = "rig"
layer = "crew"
goal = "Write and maintain project documentation"

[capability]
handles = [
  "README and getting-started guides",
  "API documentation from code",
  "Architecture decision records",
  "User-facing changelog entries",
]
does_not_handle = [
  "Code implementation (→ dev crew)",
  "Security audits (→ security-lead)",
  "Infrastructure changes (→ deacon)",
]
example_tasks = [
  "The README is out of date after the auth refactor",
  "We need API docs for the new /v2/users endpoints",
  "Write an ADR for the migration to Dolt",
]
anti_examples = [
  "Fix the auth bug in the login handler",
  "Deploy the new version to staging",
]

[execution]
cognition = "standard"
tools = ["gt", "bd"]
context_docs = [
  "docs/STYLE-GUIDE.md",
  "docs/CONTRIBUTING.md",
]
preferred_skills = ["review"]

[constraints]
rules = [
  "Always check existing docs before creating new files",
  "Use the project's markdown style guide",
  "Include code examples for all API endpoints",
]
max_tokens_per_task = 50000
```

### Coordinator with Sub-Agents

```toml
# release-manager.toml — coordinates release process across multiple concerns
role = "release-manager"
scope = "rig"
layer = "crew"
goal = "Coordinate release preparation, testing, and deployment"

[capability]
handles = [
  "Release branch management",
  "Changelog generation and review",
  "Pre-release testing coordination",
  "Version bumping and tagging",
]
does_not_handle = [
  "Individual bug fixes (→ dev crew)",
  "Infrastructure provisioning (→ infra)",
]
example_tasks = [
  "Prepare the v0.13.0 release",
  "Cherry-pick the hotfix to the release branch",
  "Generate changelog from merged PRs since last release",
]

[execution]
cognition = "standard"
tools = ["gt", "bd", "gh"]
context_docs = ["docs/RELEASING.md"]

[constraints]
rules = [
  "Never force-push to release branches",
  "All releases must pass CI before tagging",
  "Changelog must include all merged PRs",
]
can_dispatch = true

[[sub_agents]]
role = "test-runner"
cognition = "basic"
tools = ["go"]
goal = "Run the full test suite and report results"

[[sub_agents]]
role = "changelog-generator"
cognition = "basic"
tools = ["gh", "git"]
goal = "Generate changelog entries from merged PRs"
```

---

## Implementation Path

### Phase 1: Schema and Validation (immediate)

1. Extend `RoleDefinition` struct with `Capability`, `Execution`, `Constraints`,
   and `SubAgent` fields
2. Update `mergeRoleDefinition` for new field merge semantics
3. Add `gt gascity role validate <file>` command (revive PR #2518 approach)
4. Ship example Gas City roles in `docs/examples/`

### Phase 2: Runtime Loading (next)

1. Add Gas City role discovery (`<town>/gascity/roles/`)
2. Implement crew-to-role mapping in `settings/config.json`
3. Inject `[execution]` into agent session setup (tools, context_docs)
4. Inject `[constraints].rules` into system prompt generation

### Phase 3: Routing (future)

1. Build dispatcher that matches task descriptions against `[capability]`
2. Implement bounce protocol (task rejected → update anti_examples)
3. System-populate track records from completed beads
4. Connect to Wasteland stamps for cross-town reputation

---

## Open Questions

1. **TOML vs YAML?** — The existing roles use TOML and the codebase already
   depends on `BurntSushi/toml`. TOML's typed arrays and inline tables work
   well for this schema. Recommendation: stay with TOML.

2. **How are tools resolved?** — Tool names in `execution.tools` need a
   registry that maps names to MCP servers, CLI commands, or built-in
   functions. This is a separate design concern (tool registry) but the role
   format should just use string identifiers.

3. **Sub-agent lifecycle** — Are `[[sub_agents]]` always ephemeral polecats,
   or can they be persistent crew? For now, assume ephemeral. Persistent
   sub-agents are a composition of two Gas City roles, not nesting.

4. **Delegation cost tracking** — The design doc mentions making delegation
   cost visible. Should the role format declare expected costs, or is this
   purely a runtime concern? Recommendation: runtime only. Declared costs
   would be stale immediately.

5. **Track record storage** — `.track.toml` files alongside role definitions?
   Or in beads/Dolt? Recommendation: Dolt for queryability, with optional
   TOML export for portability.
