# Proposal: Semantic Conflict Detection as Plugin Molecule

**Issue:** #88
**Status:** Draft
**Author:** TimpiaAI
**Date:** 2026-01-04

## Summary

Refactor semantic conflict detection from core Refinery code to a **plugin molecule** that can be optionally bonded during Refinery patrol. This follows the Gas Town pattern where workflow-specific logic lives in molecules, not core code.

## Background

The original PR #96 added semantic conflict detection directly to `internal/refinery/engineer.go`. The maintainer feedback was:

> This kind of bespoke application-level/workflow-level routing logic needs to be implemented in the plugin layer rather than in core.

This proposal redesigns the feature as a plugin molecule following Gas Town's extensibility patterns.

## Plugin Architecture

### What is a Plugin Molecule?

From `docs/reference.md`:

```json
{
  "id": "mol-security-scan",
  "labels": ["template", "plugin", "witness", "tier:haiku"]
}
```

Plugins are:
1. **Molecules** - Workflow definitions with steps
2. **Labeled** - With `template`, `plugin`, and target agent
3. **Bonded dynamically** - Patrol molecules attach them when needed
4. **Optional** - Can be enabled/disabled per rig

### Semantic Conflict Plugin Design

```json
{
  "id": "mol-semantic-conflict-detector",
  "title": "Semantic Conflict Detection Plugin",
  "type": "molecule",
  "labels": ["template", "plugin", "refinery", "tier:sonnet"],
  "description": "Detects semantic conflicts in MR bead modifications and escalates to Mayor for decision",
  "steps": [
    {
      "id": "detect",
      "title": "Detect semantic conflicts in MR",
      "type": "task"
    },
    {
      "id": "escalate",
      "title": "Escalate conflicts to Mayor",
      "type": "task",
      "depends_on": ["detect"]
    },
    {
      "id": "await-decision",
      "title": "Wait for Mayor decision",
      "type": "task",
      "depends_on": ["escalate"]
    },
    {
      "id": "apply-resolution",
      "title": "Apply Mayor's resolution to beads",
      "type": "task",
      "depends_on": ["await-decision"]
    }
  ]
}
```

## Integration with Refinery Patrol

### Current Flow (without plugin)

```
Refinery Patrol:
1. Check merge queue
2. For each ready MR:
   a. Fetch branch
   b. Check git conflicts
   c. Run tests
   d. Merge or fail
```

### New Flow (with plugin bonded)

```
Refinery Patrol:
1. Check merge queue
2. For each ready MR:
   a. Fetch branch
   b. **Bond semantic-conflict-detector plugin**
   c. Execute plugin steps:
      - detect: Analyze commits for bead field changes
      - escalate: If conflicts found, mail Mayor
      - await-decision: Block until Mayor responds
      - apply-resolution: Update beads with decision
   d. Check git conflicts
   e. Run tests
   f. Merge or fail
```

### Bonding the Plugin

In the Refinery patrol molecule or via config:

```bash
# Bond plugin for current MR processing
bd mol bond mol-semantic-conflict-detector $MR_ID --var mr_branch="$BRANCH"
```

Or in rig config (`config.json`):

```json
{
  "refinery": {
    "plugins": ["mol-semantic-conflict-detector"],
    "semantic_conflicts": {
      "enabled": true,
      "escalate_fields": ["priority", "assignee"],
      "timeout": "1h"
    }
  }
}
```

## What Stays in Core

### Protocol Types (Minimal Core Addition)

The protocol message types can stay in core since they're part of the inter-agent communication contract:

**File: `internal/protocol/types.go`**

```go
const (
    TypeSemanticConflictEscalated MessageType = "SEMANTIC_CONFLICT_ESCALATED"
    TypeSemanticConflictResolved  MessageType = "SEMANTIC_CONFLICT_RESOLVED"
)

type SemanticConflictEscalatedPayload struct {
    MRID      string
    Conflicts []ConflictData
}

type SemanticConflictResolvedPayload struct {
    MRID        string
    Resolutions map[string]string
}
```

These are just message type definitions - no business logic.

### What Moves to Plugin

Everything else moves out of core:

| Component | Old Location (Core) | New Location (Plugin) |
|-----------|--------------------|-----------------------|
| Detection logic | `internal/refinery/semantic_conflict.go` | Plugin molecule step |
| Config parsing | `internal/refinery/engineer.go` | Rig config + plugin vars |
| Escalation mail | `internal/refinery/semantic_conflict.go` | Plugin molecule step |
| ProcessMR hook | `internal/refinery/engineer.go` | Plugin bonding |

## Plugin Implementation

### Step 1: Detect Conflicts

The plugin molecule's "detect" step runs a command that:

1. Parses commits in the MR branch
2. Looks for `BEAD_CHANGES:` blocks
3. Groups changes by bead:field
4. Identifies conflicts (same field, different values, different polecats)

```bash
# Detection command (could be gt subcommand or script)
gt semantic-conflict detect --branch $MR_BRANCH --target main
```

Output format:
```json
{
  "conflicts": [
    {
      "bead_id": "gt-abc123",
      "field": "priority",
      "changes": [
        {"polecat": "security-agent", "value": "0", "confidence": 0.95},
        {"polecat": "product-agent", "value": "2", "confidence": 0.60}
      ]
    }
  ]
}
```

### Step 2: Escalate to Mayor

If conflicts detected, send escalation mail:

```bash
gt mail send mayor/ \
  -s "SEMANTIC_CONFLICT_ESCALATED $MR_ID" \
  -m "$(gt semantic-conflict format-escalation --input conflicts.json)"
```

### Step 3: Await Decision

Block until Mayor responds:

```bash
# Poll for resolution mail
gt mail wait --subject "SEMANTIC_CONFLICT_RESOLVED $MR_ID" --timeout 1h
```

Or use the molecule's built-in blocking (step stays open until Mayor closes it).

### Step 4: Apply Resolution

Apply Mayor's decision:

```bash
gt semantic-conflict apply --resolution resolution.json
```

## Directory Structure

```
/Users/ovipi/gastown/
├── molecules/
│   └── plugins/
│       └── semantic-conflict-detector/
│           ├── mol-semantic-conflict-detector.json  # Molecule definition
│           ├── detect.sh                            # Detection script
│           ├── escalate.sh                          # Escalation script
│           └── apply.sh                             # Resolution application
├── internal/
│   └── cmd/
│       └── semantic_conflict.go                     # gt semantic-conflict subcommand
└── docs/
    └── plugins/
        └── semantic-conflicts.md                    # Plugin documentation
```

## Configuration

### Enabling the Plugin

Per-rig in `config.json`:

```json
{
  "refinery": {
    "plugins": {
      "semantic-conflict-detector": {
        "enabled": true,
        "escalate_fields": ["priority", "assignee", "estimated_minutes"],
        "auto_resolve_fields": ["labels", "title"],
        "escalation_timeout": "1h"
      }
    }
  }
}
```

### Disabling the Plugin

Simply don't include it in the plugins list, or set `enabled: false`.

## Benefits of Plugin Approach

1. **No core changes** - Refinery code stays clean
2. **Optional** - Teams choose whether to enable
3. **Configurable** - Per-rig settings
4. **Testable** - Plugin can be tested independently
5. **Upgradeable** - Update plugin without touching core
6. **Composable** - Can combine with other plugins

## Migration from PR #96

### Files to Remove from Core

```bash
# Remove from internal/refinery/
rm internal/refinery/semantic_conflict.go

# Revert changes to engineer.go
git checkout origin/main -- internal/refinery/engineer.go
```

### Files to Keep

Protocol types can stay if maintainer approves minimal additions:
- `internal/protocol/types.go` (message type constants only)

### New Files to Create

```bash
# Plugin molecule
molecules/plugins/semantic-conflict-detector/mol-semantic-conflict-detector.json

# Detection command
internal/cmd/semantic_conflict.go

# Documentation
docs/plugins/semantic-conflicts.md
```

## Open Questions for Maintainer

1. **Protocol types in core?** Should `SEMANTIC_CONFLICT_ESCALATED/RESOLVED` message types live in core, or should they be entirely plugin-defined?

2. **Plugin discovery mechanism?** How should Refinery discover and bond available plugins? Currently `bd mol bond` is explicit.

3. **gt subcommand vs scripts?** Should detection logic be a `gt semantic-conflict` subcommand, or shell scripts in the plugin directory?

4. **Plugin molecule location?** Should plugin molecules live in `molecules/plugins/` or elsewhere?

5. **Mayor handling?** Should Mayor's conflict resolution be a separate plugin, or just mail-based responses?

## Next Steps

1. Get maintainer feedback on this proposal
2. Implement plugin molecule structure
3. Create `gt semantic-conflict` subcommand (if approved)
4. Write plugin documentation
5. Submit new PR with plugin-based implementation

## References

- Issue #88: Original feature request
- PR #96: Original implementation (closed)
- `docs/reference.md`: Plugin molecule documentation
- `docs/molecules.md`: Molecule architecture
