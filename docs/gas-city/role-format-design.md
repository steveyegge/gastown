# Gas City Declarative Role Format Design

*Design document for the Gas City agent role definition format.*

## Overview

Gas City needs a declarative format for defining agent roles: what they can do, what tools they have, how they interact with other roles, and how they map to Gas Town's existing role taxonomy. This document surveys existing approaches, proposes a YAML-based format with a formal JSON Schema, and provides worked examples.

## Background: Gas Town's Current Role System

Gas Town defines roles implicitly through a combination of:

1. **Agent Taxonomy** — Eight role types (Mayor, Deacon, Witness, Refinery, Polecat, Crew, Dog, Boot) with hardcoded lifecycle behaviors
2. **Agent Presets** (`agents.json`) — JSON config defining how to launch, detect, resume, and communicate with a specific agent CLI
3. **Role Beads** (`hq-*-role`) — Global templates stored in town beads, referenced by agent beads via `role_bead`
4. **Environment Variables** — `GT_ROLE`, `BD_ACTOR`, `GT_RIG` injected at session start
5. **Hook Templates** — Role-specific settings files (e.g., `settings-autonomous.json`) that configure context injection and tool guards

This system works but has limitations: roles are defined across multiple files and code paths, capabilities are implicit (determined by what commands are available, not declared), and adding a new role type requires code changes. Gas City should make roles first-class, declarative, and composable.

## Survey of Existing Role/Agent Definition Formats

### Framework Approaches (from w-gc-004 survey)

| Framework | Role Definition | Format | Composability |
|-----------|----------------|--------|---------------|
| **CrewAI** | Agent(role, goal, backstory, tools) | Python objects | Crews compose agents; no inheritance |
| **AutoGen/MS Agent Framework** | AssistantAgent(system_message, functions) | Python/JSON config | Graph-based composition |
| **Agency Swarm** | Agent(instructions, tools, communication_flows) | Python + markdown | `>` operator for communication flows |
| **MetaGPT** | Role(profile, goal, constraints, actions) | Python classes | SOP-based chaining |
| **LangGraph** | Nodes in a state graph | Python functions | Graph edges define composition |
| **Claude Agent SDK** | Agent(tools, instructions, subagents) | Python/TypeScript | Subagent trees |
| **OpenAI Agents SDK** | Agent(instructions, tools, handoffs) | Python objects | Handoff-based delegation |

### Relevant Declarative Formats

| System | Format | Key Idea |
|--------|--------|----------|
| **Kubernetes RBAC** | YAML (Role, ClusterRole, RoleBinding) | Verbs on resources; bindings separate identity from permissions |
| **GitHub Actions** | YAML (jobs, steps, permissions) | Declarative workflows with permission scoping |
| **Terraform Providers** | HCL (resource, data, provider) | Declarative infrastructure with typed schemas |
| **Docker Compose** | YAML (services, networks, volumes) | Service definitions with capability constraints |
| **Gas Town Presets** | JSON (AgentPresetInfo) | Agent CLI capabilities; no role semantics |

### Key Insight

No existing framework separates **role definition** (what a role can do) from **agent binding** (which agent CLI fills the role). Gas Town's preset system partially does this via `role_agents` in settings, but the role's capabilities are still implicit. Gas City should make this separation explicit and declarative.

## Proposed Format: Gas City Role Definition (YAML)

### Design Principles

1. **Roles are abstract** — A role defines capabilities, constraints, and communication patterns, not which agent fills it
2. **Agents are concrete** — An agent preset (existing `agents.json`) defines how to run a specific CLI
3. **Bindings connect them** — A binding maps roles to agents, with optional overrides
4. **Composition through inheritance** — Roles can extend other roles, adding or restricting capabilities
5. **YAML for readability** — The primary authoring format is YAML; JSON Schema provides validation

### File Structure

```
~/gt/
├── roles/                      # Gas City role definitions
│   ├── base.role.yaml          # Base roles (built-in)
│   ├── worker.role.yaml        # Worker role family
│   ├── infrastructure.role.yaml # Infrastructure role family
│   └── custom/                 # User-defined roles
│       └── reviewer.role.yaml
├── bindings/                   # Role-to-agent bindings
│   ├── default.binding.yaml    # Default bindings
│   └── cost-optimized.binding.yaml
└── settings/
    └── agents.json             # Existing agent presets (unchanged)
```

### Role Definition Schema

```yaml
# Metadata
apiVersion: gascity/v1
kind: Role
metadata:
  name: builder
  description: "Worker role that implements features and fixes bugs"
  labels:
    family: worker
    scope: rig

# What this role extends
extends: worker-base

# Identity configuration
identity:
  # BD_ACTOR format pattern ({rig} and {name} are substituted at bind time)
  actor_format: "{rig}/polecats/{name}"
  # Git attribution
  git_author_format: "{rig}/polecats/{name}"

# Lifecycle configuration
lifecycle:
  # How this role is managed
  persistence: persistent-identity  # persistent | persistent-identity | ephemeral
  # Who manages this role's lifecycle
  supervisor: witness
  # Operating states (subset of: idle, working, stalled, zombie)
  states: [idle, working, stalled, zombie]
  # Whether sessions can be resumed
  resumable: true
  # Whether completed work triggers automatic recycling
  auto_recycle: true

# Capabilities: what this role is allowed to do
capabilities:
  # Tool/command allowlist
  tools:
    - name: git
      actions: [commit, push, branch, checkout, rebase]
      constraints:
        - "cannot push to main directly"
        - "must push to feature branch"
    - name: gt
      actions: [prime, mail, done, sling, costs]
    - name: bd
      actions: [show, update, close, sync]
    - name: shell
      actions: [read, write, execute]
      constraints:
        - "sandbox_scope applies"

  # File system access
  filesystem:
    read: ["**/*"]
    write: ["**/*"]
    exclude: [".git/config", "*.secret", ".env"]

  # Network access
  network:
    allowed_hosts: ["github.com", "api.github.com"]
    deny_by_default: true

# Constraints: what this role must NOT do
constraints:
  # Hard limits
  - "must not merge to main (Refinery responsibility)"
  - "must not modify .beads/ directly (use bd CLI)"
  - "must not create PRs (submit MR via gt done)"
  # Resource limits
  max_session_duration: 4h
  max_cost_per_task: 5.00  # USD
  max_concurrent_sessions: 1

# Communication: how this role interacts with others
communication:
  # Roles this role can initiate communication with
  can_message:
    - witness    # Report status, request help
    - refinery   # Submit work to merge queue
  # Roles that can message this role
  receives_from:
    - witness    # Nudges, work assignments
    - crew       # Ad-hoc instructions
    - mayor      # Cross-rig directives
  # Communication channels
  channels:
    - type: nudge     # tmux send-keys
    - type: mail      # gt mail system
    - type: hook      # gt hook events

# Context: what information this role receives at session start
context:
  # Injected via gt prime
  prime_sections:
    - role_instructions   # This role's purpose and constraints
    - current_assignment  # Active bead/issue
    - project_context     # CLAUDE.md, AGENTS.md
    - recent_mail         # Unread messages
  # Environment variables
  env:
    GT_ROLE: "{identity.actor_format}"
    GT_RIG: "{rig}"
    BD_ACTOR: "{identity.actor_format}"

# Work assignment: how this role receives and completes work
work:
  # How work is assigned
  assignment:
    method: sling          # sling | self-assign | directed
    source: witness        # Who assigns work
  # How work is completed
  completion:
    method: gt_done        # gt_done | manual | auto
    requires_mr: true      # Must submit merge request
    requires_tests: false  # Tests run by Refinery, not builder
```

### Worked Examples

#### Example 1: Builder Role (Polecat Worker)

```yaml
apiVersion: gascity/v1
kind: Role
metadata:
  name: builder
  description: "Implements features and fixes bugs on assigned issues"
  labels:
    family: worker
    scope: rig

extends: worker-base

identity:
  actor_format: "{rig}/polecats/{name}"
  git_author_format: "{rig}/polecats/{name}"

lifecycle:
  persistence: persistent-identity
  supervisor: witness
  states: [idle, working, stalled, zombie]
  resumable: true
  auto_recycle: true

capabilities:
  tools:
    - name: git
      actions: [commit, push, branch, checkout, rebase, diff, log, status]
      constraints:
        - "push only to feature branches"
    - name: gt
      actions: [prime, mail, done, costs]
    - name: bd
      actions: [show, update, close, sync]
    - name: shell
      actions: [read, write, execute]

  filesystem:
    read: ["**/*"]
    write: ["src/**", "tests/**", "docs/**", "*.md", "*.json", "*.yaml"]
    exclude: [".env", "*.secret", ".git/config"]

constraints:
  - "must not merge to main"
  - "must not create GitHub PRs directly"
  max_session_duration: 4h
  max_cost_per_task: 5.00

communication:
  can_message: [witness, refinery]
  receives_from: [witness, crew, mayor]
  channels: [nudge, mail, hook]

work:
  assignment:
    method: sling
    source: witness
  completion:
    method: gt_done
    requires_mr: true
```

#### Example 2: Reviewer Role (New Gas City Role)

```yaml
apiVersion: gascity/v1
kind: Role
metadata:
  name: reviewer
  description: "Reviews code submitted by builders, provides feedback or approval"
  labels:
    family: worker
    scope: rig

extends: worker-base

identity:
  actor_format: "{rig}/reviewers/{name}"
  git_author_format: "{rig}/reviewers/{name}"

lifecycle:
  persistence: persistent-identity
  supervisor: witness
  states: [idle, working]
  resumable: true
  auto_recycle: true

capabilities:
  tools:
    - name: git
      actions: [diff, log, show, status, checkout]
      constraints:
        - "read-only git operations (no commit, push, rebase)"
    - name: gt
      actions: [prime, mail, costs]
    - name: bd
      actions: [show, update]
      constraints:
        - "can add review comments but not close issues"
    - name: gh
      actions: [pr-review, pr-comment]
    - name: shell
      actions: [read, execute]
      constraints:
        - "no write access to source files"

  filesystem:
    read: ["**/*"]
    write: []  # Reviewers don't write code
    exclude: []

  network:
    allowed_hosts: ["github.com", "api.github.com"]

constraints:
  - "must not modify source code"
  - "must not merge or approve own work"
  - "reviews must include specific, actionable feedback"
  max_session_duration: 2h
  max_cost_per_task: 2.00

communication:
  can_message: [witness, refinery, builder]
  receives_from: [witness, refinery, mayor]
  channels: [nudge, mail]

work:
  assignment:
    method: directed
    source: refinery    # Refinery assigns reviews from merge queue
  completion:
    method: gt_done
    requires_mr: false  # Review produces comments, not code
```

#### Example 3: Coordinator Role (Cross-Rig)

```yaml
apiVersion: gascity/v1
kind: Role
metadata:
  name: coordinator
  description: "Manages convoys and cross-rig work distribution"
  labels:
    family: infrastructure
    scope: town

extends: infrastructure-base

identity:
  actor_format: "mayor/coordinators/{name}"
  git_author_format: "mayor/coordinators/{name}"

lifecycle:
  persistence: persistent
  supervisor: deacon
  states: [idle, working]
  resumable: true
  auto_recycle: false  # Long-lived, not recycled

capabilities:
  tools:
    - name: gt
      actions: [convoy, sling, mail, prime, nudge, status]
    - name: bd
      actions: [ready, list, show, create, update, dep]
    - name: bv
      actions: [robot-triage, robot-next, robot-plan, robot-alerts]

  filesystem:
    read: ["~/gt/**"]
    write: ["~/gt/mayor/**", "~/gt/.beads/**"]

  network:
    allowed_hosts: ["github.com", "api.github.com", "dolthub.com"]

constraints:
  - "must not do implementation work"
  - "must not modify project source code"
  - "convoy decisions require at least robot-triage analysis"
  max_concurrent_sessions: 1

communication:
  can_message: [witness, builder, reviewer, refinery, mayor, deacon]
  receives_from: [mayor, deacon, witness]
  channels: [nudge, mail]

work:
  assignment:
    method: self-assign
    source: null  # Coordinators find their own work
  completion:
    method: manual
    requires_mr: false
```

### Base Roles (Inheritance)

```yaml
# base.role.yaml — abstract base roles

---
apiVersion: gascity/v1
kind: Role
metadata:
  name: worker-base
  description: "Abstract base for all worker roles"
  labels:
    family: worker
    abstract: true

lifecycle:
  persistence: persistent-identity
  states: [idle, working, stalled, zombie]
  resumable: true

capabilities:
  tools:
    - name: gt
      actions: [prime, mail, costs]
    - name: bd
      actions: [show]

communication:
  receives_from: [witness, mayor]
  channels: [nudge, mail, hook]

context:
  prime_sections:
    - role_instructions
    - current_assignment
    - project_context
    - recent_mail
  env:
    GT_ROLE: "{identity.actor_format}"
    GT_RIG: "{rig}"
    BD_ACTOR: "{identity.actor_format}"

---
apiVersion: gascity/v1
kind: Role
metadata:
  name: infrastructure-base
  description: "Abstract base for infrastructure roles"
  labels:
    family: infrastructure
    abstract: true

lifecycle:
  persistence: persistent
  states: [idle, working]
  resumable: true
  auto_recycle: false

capabilities:
  tools:
    - name: gt
      actions: [prime, mail]

communication:
  receives_from: [deacon, mayor]
  channels: [nudge, mail]

context:
  prime_sections:
    - role_instructions
    - system_status
    - recent_mail
  env:
    GT_ROLE: "{identity.actor_format}"
    GT_RIG: "{rig}"
    BD_ACTOR: "{identity.actor_format}"
```

### Role Bindings

Bindings map abstract roles to concrete agent presets:

```yaml
# default.binding.yaml
apiVersion: gascity/v1
kind: RoleBinding
metadata:
  name: default
  description: "Default role-to-agent bindings"

bindings:
  - role: builder
    agent: claude           # Agent preset name from agents.json
    priority: default

  - role: reviewer
    agent: claude
    priority: default
    overrides:
      capabilities:
        # Reviewer gets additional tool access for PR comments
        tools:
          - name: gh
            actions: [pr-review, pr-comment, pr-view]

  - role: coordinator
    agent: claude
    priority: default

  - role: witness
    agent: gemini           # Cost optimization: use cheaper model
    priority: default

  - role: refinery
    agent: claude
    priority: default
```

```yaml
# cost-optimized.binding.yaml
apiVersion: gascity/v1
kind: RoleBinding
metadata:
  name: cost-optimized
  description: "Use cheaper models where possible"

bindings:
  - role: builder
    agent: gemini
    priority: default

  - role: reviewer
    agent: opencode         # Cheapest option for read-only review
    priority: default

  - role: coordinator
    agent: claude           # Keep Claude for complex coordination
    priority: default
```

## Mapping Gas Town Roles to Gas City Roles

| Gas Town Role | Gas City Role | Migration Notes |
|---------------|---------------|-----------------|
| Polecat | `builder` (or custom worker roles) | Polecats become instances of worker roles; lifecycle unchanged |
| Crew | `crew` (special: human-managed) | Crew remains user-managed; role definition adds capability declarations |
| Witness | `witness` | Promoted from implicit behavior to explicit role with declared capabilities |
| Refinery | `refinery` | Merge queue behavior codified in role constraints |
| Mayor | `mayor` | Cross-rig coordination capabilities made explicit |
| Deacon | `deacon` | Monitoring/supervision capabilities declared |
| Dog | `dog` or custom infrastructure roles | Dogs become instances of infrastructure roles |
| Boot | `boot` | Ephemeral recovery role with minimal capabilities |

Key changes:
- **Polecats generalize**: Instead of one "polecat" role, Gas City supports multiple worker role types (builder, reviewer, tester, etc.), each with different capabilities. A polecat becomes an instance of a specific role.
- **New roles are possible**: The reviewer and coordinator examples above show roles that don't exist in Gas Town today. Gas City's declarative format makes adding roles a config change, not a code change.
- **Capabilities become enforceable**: Gas Town's constraints are advisory (agents can ignore them). Gas City roles should feed into tool guards (`gt tap guard`) to make constraints enforceable.

## Composition and Inheritance

### Inheritance Rules

1. A role that specifies `extends` inherits all fields from the parent
2. Scalar fields are overridden (child wins)
3. List fields are merged (child adds to parent's list)
4. Nested objects are deep-merged
5. To remove an inherited capability, use explicit `deny` entries:

```yaml
capabilities:
  tools:
    - name: git
      actions: [diff, log, show]
      deny_actions: [commit, push, rebase]  # Remove inherited actions
```

### Composition Patterns

**Additive**: Reviewer extends worker-base, adds review-specific tools:
```yaml
extends: worker-base
capabilities:
  tools:
    - name: gh
      actions: [pr-review, pr-comment]
```

**Restrictive**: Read-only worker extends worker-base, removes write access:
```yaml
extends: worker-base
capabilities:
  filesystem:
    write: []  # Override: no write access
  tools:
    - name: git
      deny_actions: [commit, push, rebase]
```

**Multi-level**: Senior builder extends builder, adds deploy capability:
```yaml
extends: builder
capabilities:
  tools:
    - name: deploy
      actions: [staging]
constraints:
  max_cost_per_task: 10.00  # Higher limit
```

## JSON Schema

The following JSON Schema validates Gas City role definitions:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://gastown.dev/schemas/gascity/v1/role.schema.json",
  "title": "Gas City Role Definition",
  "description": "Declarative definition of an agent role in Gas City",
  "type": "object",
  "required": ["apiVersion", "kind", "metadata"],
  "properties": {
    "apiVersion": {
      "type": "string",
      "const": "gascity/v1"
    },
    "kind": {
      "type": "string",
      "enum": ["Role", "RoleBinding"]
    },
    "metadata": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {
          "type": "string",
          "pattern": "^[a-z][a-z0-9-]*$",
          "description": "Unique role identifier (lowercase, hyphens allowed)"
        },
        "description": {
          "type": "string"
        },
        "labels": {
          "type": "object",
          "additionalProperties": { "type": "string" }
        }
      }
    },
    "extends": {
      "type": "string",
      "description": "Parent role name to inherit from"
    },
    "identity": {
      "type": "object",
      "properties": {
        "actor_format": {
          "type": "string",
          "description": "BD_ACTOR format with {rig} and {name} placeholders"
        },
        "git_author_format": {
          "type": "string",
          "description": "GIT_AUTHOR_NAME format"
        }
      }
    },
    "lifecycle": {
      "type": "object",
      "properties": {
        "persistence": {
          "type": "string",
          "enum": ["persistent", "persistent-identity", "ephemeral"]
        },
        "supervisor": {
          "type": "string",
          "description": "Role name of the supervisor"
        },
        "states": {
          "type": "array",
          "items": {
            "type": "string",
            "enum": ["idle", "working", "stalled", "zombie"]
          }
        },
        "resumable": { "type": "boolean" },
        "auto_recycle": { "type": "boolean" }
      }
    },
    "capabilities": {
      "type": "object",
      "properties": {
        "tools": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name"],
            "properties": {
              "name": { "type": "string" },
              "actions": {
                "type": "array",
                "items": { "type": "string" }
              },
              "deny_actions": {
                "type": "array",
                "items": { "type": "string" }
              },
              "constraints": {
                "type": "array",
                "items": { "type": "string" }
              }
            }
          }
        },
        "filesystem": {
          "type": "object",
          "properties": {
            "read": {
              "type": "array",
              "items": { "type": "string" }
            },
            "write": {
              "type": "array",
              "items": { "type": "string" }
            },
            "exclude": {
              "type": "array",
              "items": { "type": "string" }
            }
          }
        },
        "network": {
          "type": "object",
          "properties": {
            "allowed_hosts": {
              "type": "array",
              "items": { "type": "string" }
            },
            "deny_by_default": { "type": "boolean" }
          }
        }
      }
    },
    "constraints": {
      "type": "object",
      "properties": {
        "max_session_duration": { "type": "string" },
        "max_cost_per_task": { "type": "number" },
        "max_concurrent_sessions": { "type": "integer" }
      },
      "additionalProperties": true
    },
    "communication": {
      "type": "object",
      "properties": {
        "can_message": {
          "type": "array",
          "items": { "type": "string" }
        },
        "receives_from": {
          "type": "array",
          "items": { "type": "string" }
        },
        "channels": {
          "type": "array",
          "items": {
            "oneOf": [
              { "type": "string" },
              {
                "type": "object",
                "required": ["type"],
                "properties": {
                  "type": { "type": "string" }
                }
              }
            ]
          }
        }
      }
    },
    "context": {
      "type": "object",
      "properties": {
        "prime_sections": {
          "type": "array",
          "items": { "type": "string" }
        },
        "env": {
          "type": "object",
          "additionalProperties": { "type": "string" }
        }
      }
    },
    "work": {
      "type": "object",
      "properties": {
        "assignment": {
          "type": "object",
          "properties": {
            "method": {
              "type": "string",
              "enum": ["sling", "self-assign", "directed"]
            },
            "source": { "type": ["string", "null"] }
          }
        },
        "completion": {
          "type": "object",
          "properties": {
            "method": {
              "type": "string",
              "enum": ["gt_done", "manual", "auto"]
            },
            "requires_mr": { "type": "boolean" },
            "requires_tests": { "type": "boolean" }
          }
        }
      }
    }
  }
}
```

## Open Questions

1. **Enforcement granularity**: Should capability constraints be soft (advisory, logged) or hard (tool call blocked)? Gas Town's current `gt tap guard` mechanism supports hard guards for specific patterns. Gas City could extend this to enforce the full capability model, but this adds complexity and may break agents that expect unrestricted access.

2. **Role versioning**: When a role definition changes, what happens to running agents bound to the old version? Options: (a) hot-reload on next `gt prime`, (b) complete current task then reload, (c) explicit role migration command.

3. **Cross-town roles**: The coordinator role operates across rigs. How do role definitions compose when rigs have different trust levels? Should there be a federation model where roles carry credentials?

4. **Custom role discovery**: If users define custom roles in `~/gt/roles/custom/`, how does Gas City discover and validate them? A `gt role list` / `gt role validate` command set would be needed.

5. **Relationship to Gas City Provider Contract**: The provider contract (in `agent-provider-integration.md`) defines what an agent CLI must implement. Roles define what an agent instance is allowed to do. The binding connects them. Should the binding also validate that the agent preset supports the role's requirements (e.g., a role requiring `resumable: true` should only bind to agents with `resume_flag` set)?
