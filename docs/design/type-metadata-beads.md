# Type Metadata Beads Design

> Extending the beads type system with configurable validation rules
> Part of epic: bd-epc-decision_type_templates_subtype

## Summary

Decision types answer: **"Why is the agent asking the human?"**

| Type | Agent Says... | When To Use |
|------|--------------|-------------|
| `confirmation` | "I'm about to do X, is that right?" | High-stakes/irreversible action |
| `ambiguity` | "Requirements could mean A or B" | Multiple valid interpretations |
| `tradeoff` | "Option A vs B, each has pros/cons" | No clear winner, human sets priority |
| `stuck` | "I can't proceed without X" | Agent is blocked |
| `checkpoint` | "Here's where I am, any corrections?" | Mid-work check-in |
| `quality` | "Is this good enough?" | Subjective judgment call |
| `exception` | "Something unexpected happened" | Error or unusual situation |
| `prioritization` | "Multiple things need doing, what first?" | Competing tasks |

These types are derived from analysis of 20+ Gas Town formulas (code-review, design, boot-triage, shutdown-dance, witness-patrol, polecat-work, orphan-scan, etc.).

## Problem Statement

We want decision beads to have structured context requirements based on their type. Rather than hardcoding these rules, we need a flexible system where:

1. Type validation rules are themselves beads (versionable, auditable)
2. Validators can run custom scripts (also beads)
3. The system is extensible to other bead types, not just decisions

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Bead Creation                             │
│  gt decision request --type tradeoff --context "..."             │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Type Metadata Lookup                          │
│  Find: type-meta bead for type=decision, subtype=tradeoff        │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Validation Pipeline                           │
│  For each required_field:                                        │
│    1. Check field present in context                             │
│    2. If validator_bead specified, run it                        │
│    3. Validator loads script_bead, executes                      │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Result                                        │
│  Pass: Create bead with type + subtype                          │
│  Fail: Return errors with helpful messages                       │
└─────────────────────────────────────────────────────────────────┘
```

## Decision Types

### 1. `confirmation` - "I'm about to do X, is that right?"

**High-stakes action needs human sign-off before proceeding.**

Derived from: mol-shutdown-dance (death warrants), mol-town-shutdown (blockers)

**When to use:** Agent is confident about the action but it's irreversible or high-risk.

**Required context:**
```json
{
  "action": "Delete all polecat sandboxes and restart Gas Town",
  "why": "Corrupted state detected, fresh start needed",
  "reversible": false,
  "impact": "All in-progress work will be lost if not committed"
}
```

**Example options:**
- "Proceed with shutdown"
- "Wait, let me check something first"
- "Abort - don't do this"

**Emoji:** ⚠️

---

### 2. `ambiguity` - "The requirements could mean A or B"

**Multiple valid interpretations, need human to clarify intent.**

Derived from: design.formula (Open Questions), feature spec interpretation

**When to use:** Agent found multiple reasonable ways to interpret the task.

**Required context:**
```json
{
  "interpretations": [
    "A: Validate on every keystroke (strict)",
    "B: Validate on blur/submit only (permissive)"
  ],
  "leaning": "B - better UX, less annoying",
  "why_unclear": "Spec says 'validate input' but doesn't specify when"
}
```

**Example options:**
- "Interpretation A: Strict validation"
- "Interpretation B: Permissive validation"
- "Both - strict with debounce"

**Emoji:** ❓

---

### 3. `tradeoff` - "Option A vs B, each has pros/cons"

**No clear winner - depends on human priorities.**

Derived from: code-review synthesis (conflicting findings), architecture decisions

**When to use:** Agent evaluated options but the "right" choice depends on values/priorities the agent can't determine.

**Required context:**
```json
{
  "options": ["Redis", "SQLite", "In-memory"],
  "analysis": {
    "Redis": {"pros": ["distributed", "persistent"], "cons": ["ops overhead"]},
    "SQLite": {"pros": ["simple", "no deps"], "cons": ["single-node"]},
    "In-memory": {"pros": ["fastest"], "cons": ["volatile"]}
  },
  "recommendation": "Redis - we'll need multi-node eventually",
  "deciding_factor": "How important is ops simplicity vs future scalability?"
}
```

**Example options:**
- "Redis: Distributed, future-proof"
- "SQLite: Simple, good enough for now"
- "In-memory: Fastest, accept volatility"

**Emoji:** ⚖️

---

### 4. `stuck` - "I can't proceed without X"

**Agent is blocked and needs something from the human.**

Derived from: polecat escalation patterns, mol-convoy-feed (no capacity)

**When to use:** Agent hit a wall - missing info, external dependency, access needed.

**Required context:**
```json
{
  "blocker": "Need AWS credentials to test S3 integration",
  "tried": [
    "Checked .env files - not present",
    "Checked secrets manager - no access",
    "Asked in #infra channel - no response"
  ],
  "need": "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY for dev account"
}
```

**Example options:**
- "I'll get you the credentials"
- "Skip S3 testing, mock it instead"
- "Let me take over this piece"

**Emoji:** 🚧

---

### 5. `checkpoint` - "Here's where I am, any course corrections?"

**Periodic check-in during long work.**

Derived from: rule-of-five (iterative refinement), shiny workflow stages

**When to use:** Agent wants to confirm direction before investing more effort. Good for expensive or long-running work.

**Required context:**
```json
{
  "progress": "Completed API design and data model. Tests passing.",
  "next_steps": "Implement the CLI commands and integration tests",
  "concerns": "The data model might be over-normalized - 6 tables for what could be 2"
}
```

**Example options:**
- "Looks good, continue"
- "Simplify the data model first"
- "Stop - let's discuss the API design"

**Emoji:** 📍

---

### 6. `quality` - "Is this good enough?"

**Subjective judgment call about completeness or quality.**

Derived from: rule-of-five (excellence pass), code-review (merge readiness)

**When to use:** Agent finished something but "good enough" is subjective.

**Required context:**
```json
{
  "artifact": "PR #123: Add user authentication",
  "assessment": "Functional and tested, but error messages are generic",
  "gaps": [
    "No rate limiting on login endpoint",
    "Error messages don't help users fix issues",
    "No password strength indicator"
  ],
  "bar": "Production-ready MVP"
}
```

**Example options:**
- "Ship it - good enough for MVP"
- "Add rate limiting before merge"
- "Needs more polish - address all gaps"

**Emoji:** ✨

---

### 7. `exception` - "Something unexpected happened"

**Error or unusual situation, need guidance on how to proceed.**

Derived from: mol-orphan-scan (RESET/RECOVER/ESCALATE), mol-refinery-patrol (test failures)

**When to use:** Agent encountered something outside normal flow and isn't sure how to handle it.

**Required context:**
```json
{
  "situation": "Found 3 orphaned polecats with uncommitted work",
  "options": [
    "RESET: Discard work, return issues to open",
    "RECOVER: Try to commit and push their work",
    "ESCALATE: Need human to inspect the code"
  ],
  "recommendation": "RECOVER - the git log shows meaningful commits",
  "risk": "RECOVER might push broken code if tests weren't run"
}
```

**Example options:**
- "RECOVER: Try to save the work"
- "RESET: Discard and restart fresh"
- "ESCALATE: I'll look at it manually"

**Emoji:** ⚡

---

### 8. `prioritization` - "Multiple things need doing, what first?"

**Agent has competing tasks or directions.**

Derived from: mol-convoy-feed (dispatch order), triage patterns

**When to use:** Agent can see multiple valid work items but needs human to set priority.

**Required context:**
```json
{
  "candidates": [
    {"id": "gt-123", "title": "Fix login crash", "severity": "P0"},
    {"id": "gt-456", "title": "Add OAuth support", "severity": "P1"},
    {"id": "gt-789", "title": "Refactor auth module", "severity": "P2"}
  ],
  "analysis": {
    "gt-123": "Blocking users, quick fix (~1h)",
    "gt-456": "Blocking sales demo Friday",
    "gt-789": "Tech debt, no deadline"
  },
  "constraints": "Can only finish 2 before EOD",
  "suggestion": "gt-123 first (users blocked), then gt-456 (demo)"
}
```

**Example options:**
- "123 then 456: Fix crash, then OAuth"
- "456 then 123: Demo is more important"
- "Just 123: Focus on the crash, OAuth can wait"

**Emoji:** 🎯

---

## New Bead Types

### 1. `type-meta` - Type Metadata Bead

Defines validation rules and UI metadata for a decision type.

**Prefix:** `meta-`

**Example:**
```yaml
ID: meta-decision-tradeoff
Type: type-meta

for_type: decision
for_subtype: tradeoff

emoji: "⚖️"
label: "Tradeoff Decision"
description: "No clear winner - depends on human priorities"

required_fields:
  - name: options
    description: "The alternatives being considered"
    validator_bead: vld-has-options

  - name: analysis
    description: "Pros and cons of each option"
    validator_bead: null

  - name: recommendation
    description: "Agent's suggestion if forced to choose"
    validator_bead: vld-not-empty

  - name: deciding_factor
    description: "What would tip the balance"
    validator_bead: null

example: |
  {
    "options": ["Redis", "SQLite"],
    "analysis": {"Redis": {...}, "SQLite": {...}},
    "recommendation": "Redis",
    "deciding_factor": "How important is ops simplicity?"
  }
```

### 2. `validator` - Validator Bead

Defines a validation rule that can be applied to fields.

**Prefix:** `vld-`

**Example:**
```yaml
ID: vld-has-options
Type: validator

name: "has-options"
description: "Verify decision has 2-4 distinct options"

script_bead: scr-check-options-count
extract_mode: json_field
extract_path: "options"

error_template: "Decision must have 2-4 options, found {value}"
timeout_ms: 1000
```

### 3. `script` - Script Bead

Stores executable validation logic.

**Prefix:** `scr-`

**Example:**
```yaml
ID: scr-check-options-count
Type: script

name: "check-options-count"
description: "Verify array has 2-4 items"

interpreter: /bin/bash
script: |
  count=$(echo "$1" | jq 'length')
  if [ "$count" -ge 2 ] && [ "$count" -le 4 ]; then
    exit 0
  else
    echo "Expected 2-4 options, got $count" >&2
    exit 1
  fi

allowed_commands: [jq]
max_runtime_ms: 1000
```

## Validators

Standard validators for decision quality:

| Validator | Purpose |
|-----------|---------|
| `vld-not-empty` | Required field has content |
| `vld-has-options` | Decision has 2-4 actionable options |
| `vld-options-distinct` | Options are meaningfully different |
| `vld-recommendation-present` | Agent provided a recommendation |
| `vld-json-valid` | Valid JSON structure |

## CLI Usage

```bash
# Create a tradeoff decision
gt decision request \
  --type tradeoff \
  --prompt "Which caching strategy?" \
  --context '{"options": ["Redis", "SQLite"], "analysis": {...}, ...}' \
  --option "Redis: Distributed" \
  --option "SQLite: Simple"

# List available decision types
gt decision types

# Show type requirements
gt decision types show tradeoff
```

## Slack Rendering

Decisions display their type with emoji and structured context:

```
⚖️ Tradeoff Decision: Which caching strategy?

**Options under consideration:**
• Redis - Distributed, persistent
• SQLite - Simple, no deps

**Agent's analysis:**
Redis handles our multi-node future but adds ops overhead.
SQLite is simpler but won't scale.

**Recommendation:** Redis
**Deciding factor:** How important is ops simplicity vs scalability?

[Redis] [SQLite] [Neither]
```

## Migration Path

### Phase 1: Schema
- Add `subtype` column to issues table
- Add new bead types: `type-meta`, `validator`, `script`
- Create default type metadata beads for 8 decision types

### Phase 2: Validation
- Implement type metadata lookup
- Implement required field validation
- Implement validator/script execution

### Phase 3: CLI
- Add `--type` flag to `gt decision request`
- Add `gt decision types` command
- Update `bd decision create` to validate

### Phase 4: Integration
- Update Slack rendering for types
- Add type emoji/label to notifications
- Track type usage metrics

## Success Metrics

- Agents use typed decisions >80% of the time
- Humans resolve typed decisions 30% faster (better context)
- Validation catches incomplete decisions before creation
- Type distribution matches formula patterns (tradeoff, stuck, checkpoint most common)
