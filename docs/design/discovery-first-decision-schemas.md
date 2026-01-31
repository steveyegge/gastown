# Discovery-First Decision Schemas

> Design for emergent decision type schemas through LLM discovery and creation.
> Written 2026-01-30, crew/decision_notify session.
> Parent epic: bd-epc-decision_type_templates_subtype

## Problem Statement

Current decision validation uses hard-coded bash scripts in `validators/` directories:
- Fixed set of types: tradeoff, ambiguity, confirmation, checkpoint, etc.
- Adding a new type requires writing a bash script
- No discovery mechanism - agents must know schema names upfront
- No visibility into which schemas are actually useful

We want to **let schemas evolve organically** based on real LLM usage patterns.

## Design Goals

1. **Discovery-first**: LLM searches for matching schema before creating decision
2. **Intentional creation**: Creating a new schema is a deliberate act, not a fallback
3. **Emergent vocabulary**: Useful schemas get reused; bad ones fade away
4. **Bead-native**: Schemas are beads - discoverable, versionable, traceable
5. **Low friction**: Easy to find, easy to create, easy to iterate

---

## Architecture

### Schema as Bead Type

Decision schemas are stored as beads with `type: decision-schema`.

```
┌─────────────────────────────────────────────────────────────┐
│                 Agent creating decision                      │
│                                                             │
│  "I need to ask about cache strategy..."                    │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│               gt decision request                            │
│                                                             │
│  Step 1: DISCOVER - "What schemas match my intent?"         │
│          → bd list -t decision-schema --search "tradeoff"   │
│          → Shows: tradeoff, architecture-choice, tooling    │
│                                                             │
│  Step 2: SELECT or CREATE                                   │
│          → Match found? Use --type=tradeoff                 │
│          → No match? "venture" to create new schema         │
└─────────────────────────────────────────────────────────────┘
```

### Discovery Flow

```
Agent has decision to make
        │
        ▼
┌───────────────────┐
│ gt schema search  │  Search by intent/keywords
│ "cache strategy"  │  Returns matching schemas
└────────┬──────────┘
         │
    ┌────┴────┐
    │ Match?  │
    └────┬────┘
         │
    ┌────┴────────────────┐
    │                     │
    ▼                     ▼
┌─────────┐         ┌─────────────────┐
│  Yes    │         │      No         │
│ Use it  │         │ Create new?     │
└────┬────┘         └────────┬────────┘
     │                       │
     ▼                       ▼
┌───────────────┐    ┌────────────────────┐
│ gt decision   │    │ gt schema create   │
│ request       │    │ "cache-strategy"   │
│ --type=X      │    │ --intent="..."     │
└───────────────┘    │ --requires="..."   │
                     └─────────┬──────────┘
                               │
                               ▼
                     ┌─────────────────────┐
                     │ New schema bead     │
                     │ Use immediately or  │
                     │ save for later      │
                     └─────────────────────┘
```

---

## Data Model

### Schema Bead Format

```yaml
id: gt-schema-tradeoff
type: decision-schema
status: open
title: "tradeoff: Weighing alternatives with recommendation"
labels:
  - schema:decision
  - category:choice
  - usage-count:47
description: |
  ## Intent
  Use when choosing between multiple valid alternatives where each has
  distinct pros and cons.

  ## When to Use
  - Architectural decisions (database, framework, language)
  - Technology choices with tradeoffs
  - Resource allocation decisions

  ## Required Context
  - `options`: Array of alternatives being compared (min 2)
  - `recommendation`: Your suggested choice
  - `deciding_factor`: What tips the balance (optional but encouraged)

  ## Example
  ```json
  {
    "options": ["Redis", "SQLite", "In-memory"],
    "recommendation": "Redis",
    "deciding_factor": "Multi-process architecture requires shared state"
  }
  ```

  ## Validation
  - Must have 2+ options
  - Must include recommendation
  - Options should be distinct alternatives (not yes/no)

created_by: gastown/crew/slack_decisions
created_at: 2026-01-15T10:00:00Z
```

### Schema Registry

Schemas are discovered via standard bead queries:

```bash
# List all schemas
bd list -t decision-schema

# Search by keyword
bd list -t decision-schema --search "tradeoff"

# Search by category
bd list -t decision-schema --label=category:choice

# Show popular schemas
bd list -t decision-schema --sort=usage-count
```

---

## Commands

### gt schema search

Search for schemas matching intent.

```bash
gt schema search "choosing between caching options"
```

**Output:**
```
🔍 Schemas matching "choosing between caching options":

  1. tradeoff (47 uses)
     Weighing alternatives with recommendation
     Required: options[], recommendation
     Match: "choosing between" → alternatives

  2. architecture-choice (12 uses)
     Technical architecture decisions
     Required: options[], constraints[], recommendation
     Match: "caching" → architecture

  3. tooling (8 uses)
     Tool/library selection
     Required: candidates[], criteria[]
     Match: "options" → selection

None of these fit? Create a new schema:
  gt schema create --intent "choosing between caching options"
```

### gt schema show

Show schema details.

```bash
gt schema show tradeoff
```

**Output:**
```
📋 Schema: tradeoff (gt-schema-tradeoff)

Intent: Weighing alternatives with recommendation

When to use:
  - Architectural decisions
  - Technology choices with tradeoffs
  - Resource allocation decisions

Required context:
  - options: Array of alternatives (min 2)
  - recommendation: Your suggested choice
  - deciding_factor: What tips the balance (optional)

Example:
  gt decision request \
    --type=tradeoff \
    --prompt "Which caching strategy?" \
    --context '{"options": ["Redis", "SQLite"], "recommendation": "Redis", ...}'

Usage: 47 decisions have used this schema
Created by: gastown/crew/slack_decisions
```

### gt schema create

Create a new schema (the "venture" action).

```bash
gt schema create \
  --name "cache-strategy" \
  --intent "Choosing between caching approaches for performance" \
  --requires "options,recommendation,latency_target" \
  --example '{"options": ["Redis", "In-memory"], "recommendation": "Redis", "latency_target": "<50ms"}'
```

**Flags:**
- `--name` (required): Schema identifier (kebab-case)
- `--intent`: What this schema is for (searchable)
- `--requires`: Comma-separated required context fields
- `--optional`: Comma-separated optional context fields
- `--example`: Example context JSON
- `--category`: Category label (choice, confirmation, checkpoint, etc.)

**Output:**
```
📋 Created schema: cache-strategy (gt-schema-cache-strategy)

Intent: Choosing between caching approaches for performance
Required: options, recommendation, latency_target

To use:
  gt decision request --type=cache-strategy ...

💡 Tip: Your schema is now discoverable. If others find it useful,
   it will accumulate usage and become part of the shared vocabulary.
```

### gt schema list

List available schemas.

```bash
gt schema list [--category=choice] [--sort=usage-count]
```

**Output:**
```
📋 Decision Schemas (12 total)

Category: choice
  ● tradeoff (47 uses) - Weighing alternatives with recommendation
  ● architecture-choice (12 uses) - Technical architecture decisions
  ● tooling (8 uses) - Tool/library selection

Category: confirmation
  ● destructive-action (23 uses) - Confirming irreversible operations
  ● scope-change (5 uses) - Confirming scope expansion

Category: checkpoint
  ● phase-gate (15 uses) - End of phase review
  ● milestone (3 uses) - Major deliverable checkpoint

Category: custom
  ● cache-strategy (1 use) - Choosing between caching approaches
```

---

## Integration with gt decision request

### Current Flow (Script-Based)

```bash
gt decision request \
  --type=tradeoff \
  --prompt "Which cache?" \
  --context '{"options": [...], "recommendation": "Redis"}'
```

Validation runs `create-decision-type-tradeoff.sh` script.

### New Flow (Schema-Based)

```bash
gt decision request \
  --type=tradeoff \
  --prompt "Which cache?" \
  --context '{"options": [...], "recommendation": "Redis"}'
```

1. Look up `gt-schema-tradeoff` bead
2. Extract `requires` fields from schema
3. Validate context against requirements
4. Create decision with schema reference
5. Increment schema usage count

### Fallback Behavior

If `--type` is not provided:
1. Suggest schemas based on prompt keywords
2. Allow creation without type (untyped decision)
3. Prompt: "Consider using --type=X for structured validation"

---

## Emergent Schema Evolution

### Usage Tracking

Each time a schema is used:
```bash
bd update gt-schema-tradeoff --label=usage-count:48
```

### Schema Popularity

Popular schemas (high usage) are:
- Shown first in search results
- Documented in prime context
- Candidates for "blessed" status

### Schema Retirement

Schemas with 0 usage after 30 days:
- Flagged for review
- Option to archive or delete
- Creator notified

### Schema Iteration

LLMs can propose schema improvements:
```bash
gt schema update tradeoff \
  --add-optional "risk_assessment" \
  --rationale "Many tradeoff decisions benefit from risk analysis"
```

Changes are versioned in the bead history.

---

## LLM Guidance

### In Prime Context

```markdown
## Decision Schemas

Before creating a decision, search for a matching schema:

  gt schema search "<your intent>"

Schemas provide:
- Structured context requirements
- Validation of required fields
- Consistent decision formatting
- Shared vocabulary across agents

If no schema matches your intent, you can venture to create one:

  gt schema create --name="my-schema" --intent="..." --requires="..."

Creating a schema signals: "I think this decision pattern is common enough
to deserve its own structure." Good schemas get reused; unused ones fade.
```

### In Validation Errors

When validation fails:
```
❌ Decision validation failed for type 'tradeoff':
   Missing required field: recommendation

Either:
  1. Add the missing field to your context
  2. Use a different schema that fits better:
     gt schema search "your intent"
  3. Create a new schema if this pattern doesn't exist:
     gt schema create --name="..." --intent="..."
```

---

## Migration Path

### Phase 1: Parallel Systems

1. Keep existing bash scripts for current types
2. Add schema beads for same types
3. Schema beads reference scripts for validation

### Phase 2: Schema Discovery

1. Add `gt schema search` command
2. Update `gt decision request` to suggest schemas
3. Add usage tracking

### Phase 3: LLM Creation

1. Add `gt schema create` command
2. Update prime context with schema guidance
3. Enable schema iteration

### Phase 4: Full Migration

1. Generate schemas from existing scripts
2. Deprecate direct script validation
3. Scripts become implementation detail of schema beads

---

## Open Questions

1. **Schema namespacing?**

   Should schemas be rig-local or town-wide?

   Proposal: Town-wide by default (`hq-schema-*`), with rig-local overrides.

2. **Schema approval workflow?**

   Should new schemas require human approval?

   Proposal: No - let LLMs create freely, let usage/non-usage be the filter.

3. **Schema validation implementation?**

   Keep bash scripts? Move to Go? JSON Schema?

   Proposal: JSON Schema stored in bead, validated in Go.

4. **Backward compatibility?**

   How to handle decisions created with old schema versions?

   Proposal: Store schema version in decision bead, validate against that version.

---

## Success Metrics

1. **Schema reuse rate**: % of decisions using existing schemas vs creating new
2. **Schema survival rate**: % of created schemas still in use after 30 days
3. **Discovery success**: % of schema searches that find useful match
4. **Creation rate**: How often LLMs venture to create new schemas
5. **Vocabulary growth**: Net new useful schemas per month
