# Molecules: Composable Workflow Templates

This document covers the molecule system in depth.

For an overview, see [architecture.md](architecture.md#molecules-composable-workflow-templates).
For the full lifecycle (Rig → Cook → Run), see [molecular-chemistry.md](molecular-chemistry.md).

## Core Concepts

A **molecule** is a workflow template stored as a beads issue with `labels: ["template"]`.
When bonded, it creates child issues forming a DAG of steps.

```
Proto (template)
     │
     ▼ bd mol bond
┌─────────────────┐
│ Mol (durable)   │  ← .beads/ (synced, auditable)
│ or              │
│ Wisp (ephemeral)│  ← .beads-wisp/ (gitignored)
└────────┬────────┘
         │
   ┌─────┴─────┐
   ▼           ▼
bd mol burn  bd mol squash
(no record)  (creates digest)
```

| Concept | Description |
|---------|-------------|
| Proto | Template molecule (is_template: true in beads) |
| Mol | Durable instance in .beads/ |
| Wisp | Ephemeral instance in .beads-wisp/ |
| Digest | Condensed completion record |
| Bond | Instantiate proto → mol/wisp |

## Molecule Storage

Molecules are standard beads issues stored in `molecules.jsonl`:

```json
{
  "id": "mol-shiny",
  "title": "Shiny: {{feature_name}}",
  "description": "Full workflow from design to merge.\n\nVars:\n- {{feature_name}} - What to build",
  "labels": ["template"],
  "issue_type": "epic"
}
```

**Loaded from multiple sources** (later overrides earlier):
1. Built-in (embedded in bd binary)
2. Town-level: `~/gt/.beads/molecules.jsonl`
3. User-level: `~/.beads/molecules.jsonl`
4. Project-level: `.beads/molecules.jsonl`

## Variable Substitution

Molecules support `{{var}}` placeholders resolved at bond time:

```bash
bd mol bond mol-shiny --var feature_name="user auth"
# Creates: "Shiny: user auth"
```

Variables work in:
- Title
- Description
- Child step titles/descriptions

## Steps as Children

Steps are hierarchical children of the molecule:

```
mol-shiny (epic)
├── mol-shiny.1  "Design"
├── mol-shiny.2  "Implement"       Needs: .1
├── mol-shiny.3  "Review"          Needs: .2
├── mol-shiny.4  "Test"            Needs: .3
└── mol-shiny.5  "Submit"          Needs: .4
```

Dependencies are encoded in beads edges, not in step descriptions.

## Molecule CLI Reference

### bd mol bond

Instantiate a proto molecule into a runnable mol or wisp.

```bash
bd mol bond <proto-id> [--wisp] [--var key=value...]
```

| Flag | Description |
|------|-------------|
| `--wisp` | Create ephemeral wisp instead of durable mol |
| `--var` | Variable substitution (repeatable) |
| `--ref` | Custom reference ID for the instance |

**Examples:**

```bash
# Durable mol
bd mol bond mol-shiny --var feature_name="auth"

# Ephemeral wisp for patrol
bd mol bond mol-witness-patrol --wisp

# Dynamic child with custom ref (Christmas Ornament pattern)
bd mol bond mol-polecat-arm $PATROL_ID --ref arm-ace --var polecat=ace
```

### bd mol squash

Complete a molecule and generate a permanent digest.

```bash
bd mol squash <mol-id> --summary='...'
```

The summary is agent-generated - the intelligence comes from the agent, not beads.

**For Mol**: Creates digest in .beads/, original steps remain.
**For Wisp**: Evaporates wisp, creates digest. Execution trace gone, outcome preserved.

### bd mol burn

Abandon a molecule without record.

```bash
bd mol burn <mol-id> [--reason='...']
```

Use burn for:
- Routine patrol cycles (no audit needed)
- Failed experiments
- Cancelled work

Use squash for:
- Completed work
- Investigations with findings
- Anything worth recording

### bd mol list

List available molecules:

```bash
bd mol list              # All templates
bd mol list --label plugin  # Plugin molecules only
```

## Plugins ARE Molecules

Patrol plugins are molecules with specific labels:

```json
{
  "id": "mol-security-scan",
  "title": "Security scan for {{polecat_name}}",
  "description": "Check for vulnerabilities.\n\nVars: {{polecat_name}}, {{captured_output}}",
  "labels": ["template", "plugin", "witness", "tier:haiku"],
  "issue_type": "task"
}
```

Label conventions:
- `plugin` - marks as bondable at hook points
- `witness` / `deacon` / `refinery` - which patrol uses it
- `tier:haiku` / `tier:sonnet` - model hint

**Execution in patrol:**

```bash
# plugin-run step bonds registered plugins:
bd mol bond mol-security-scan $PATROL_WISP \
  --ref security-{{polecat_name}} \
  --var polecat_name=ace \
  --var captured_output="$OUTPUT"
```

## The Christmas Ornament Pattern

Dynamic bonding creates tree structures at runtime:

```
                    ★ mol-witness-patrol
                   /|\
            ┌─────┘ │ └─────┐
         PREFLIGHT  │    CLEANUP
            │       │        │
        ┌───┴───┐   │    ┌───┴───┐
        │inbox  │   │    │aggreg │
        │load   │   │    │summary│
        └───────┘   │    └───────┘
                    │
          ┌─────────┼─────────┐
          │         │         │
          ●         ●         ●  mol-polecat-arm (dynamic)
         ace       nux      toast
          │         │         │
       ┌──┴──┐   ┌──┴──┐   ┌──┴──┐
       │steps│   │steps│   │steps│
       └──┬──┘   └──┬──┘   └──┬──┘
          │         │         │
          └─────────┴─────────┘
                    │
                 ⬣ base
```

**Key primitives:**
- `bd mol bond ... --ref` - creates named children
- `WaitsFor: all-children` - fanout gate
- Arms execute in parallel

## Mol Mall

Distribution through molecule marketplace:

```bash
# Install from registry
bd mol install mol-security-scan

# Updates ~/.beads/molecules.jsonl
```

Mol Mall serves `molecules.jsonl` fragments. Installation appends to your catalog.

## Code Review Molecule

The code-review molecule is a pluggable workflow:

```
mol-code-review
├── discovery (parallel)
│   ├── file-census
│   ├── dep-graph
│   └── coverage-map
│
├── structural (sequential)
│   ├── architecture-review
│   └── abstraction-analysis
│
├── tactical (parallel per component)
│   ├── security-scan
│   ├── performance-review
│   └── complexity-analysis
│
└── synthesis (single)
    └── aggregate
```

Each dimension is a molecule that can be:
- Swapped for alternatives from Mol Mall
- Customized by forking and installing your version
- Disabled by not bonding it

**Usage:**

```bash
bd mol bond mol-code-review --var scope="src/"
```

## Lifecycle Summary

Gas Town work follows the **Rig → Cook → Run** lifecycle:

```
           RIG (source)              COOK (artifact)           RUN (execution)
      ┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
      │    Formula      │──────►│     Proto       │──────►│   Mol/Wisp      │
      │   (.yaml)       │ cook  │  (cooked/frozen)│ pour/ │   (flowing)     │
      │                 │       │                 │ wisp  │                 │
      └─────────────────┘       └─────────────────┘       └────────┬────────┘
                                                                   │
                                                              ┌────┴────┐
                                                              ▼         ▼
                                                            burn      squash
                                                              │         │
                                                              ▼         ▼
                                                           (gone)   Digest
                                                                   (permanent)
```

**Full lifecycle:**
- **Formula** (.formula.yaml) = Recipe (source code for workflows)
- **Proto** = Fuel (cooked template, ready to instantiate)
- **Mol/Wisp** = Steam (active execution)
- **Digest** = Distillate (crystallized work)

See [molecular-chemistry.md](molecular-chemistry.md) for the complete specification.
