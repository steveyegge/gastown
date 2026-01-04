# Semantic Conflict Detection Plugin

## Overview

The semantic conflict detector is a **plugin molecule** that extends Refinery to detect and escalate semantic conflicts - situations where multiple Polecats modify the same bead field with different professional judgments.

## Problem

Current conflict resolution (Last-Write-Wins) works for technical conflicts but silently discards important expert disagreements:

| Agent | Field | Value | Reasoning |
|-------|-------|-------|-----------|
| security-agent | priority | 0 | CVE-2024-1234 with public exploit |
| product-agent | priority | 2 | Low user impact edge case |

With LWW, whichever Polecat's commit is processed last wins, potentially ignoring critical security judgment.

## Solution

This plugin:
1. **Detects** conflicting bead modifications in MR commits
2. **Escalates** to Mayor with confidence scores and reasoning
3. **Awaits** Mayor's decision
4. **Applies** the resolution to affected beads

## Architecture

```
Refinery Patrol
    │
    ├─> Bond mol-semantic-conflict-detector plugin
    │
    ├─> Step: detect-conflicts
    │       └─> gt semantic-conflict detect --branch $BRANCH
    │
    ├─> Step: escalate-to-mayor (if conflicts)
    │       └─> gt semantic-conflict escalate --conflicts conflicts.json
    │
    ├─> Step: await-mayor-decision
    │       └─> gt semantic-conflict await --mr $MR_ID --timeout 1h
    │
    └─> Step: apply-resolution
            └─> gt semantic-conflict apply --resolution resolution.json
```

## Configuration

### Enable in Rig Config

Add to your rig's `config.json`:

```json
{
  "refinery": {
    "plugins": {
      "semantic-conflict-detector": {
        "enabled": true,
        "escalate_fields": ["priority", "assignee", "estimated_minutes"],
        "timeout": "1h"
      }
    }
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the plugin |
| `escalate_fields` | []string | `["priority", "assignee"]` | Fields requiring Mayor decision |
| `timeout` | duration | `"1h"` | Max wait for Mayor decision |

## Commands

### gt semantic-conflict detect

Analyze commits for conflicting bead modifications.

```bash
gt semantic-conflict detect \
  --branch polecat/toast/gt-abc123 \
  --target main \
  --fields priority,assignee \
  --output conflicts.json
```

**Output:**
```json
{
  "mr": "gt-abc123",
  "branch": "polecat/toast/gt-abc123",
  "target": "main",
  "conflicts": [
    {
      "bead_id": "gt-abc123",
      "field": "priority",
      "changes": [
        {"polecat": "security-agent", "new_value": "0", "confidence": 0.95},
        {"polecat": "product-agent", "new_value": "2", "confidence": 0.60}
      ]
    }
  ],
  "detected": "2026-01-04T12:00:00Z"
}
```

### gt semantic-conflict escalate

Send escalation mail to Mayor.

```bash
gt semantic-conflict escalate --conflicts conflicts.json --mr gt-abc123
```

### gt semantic-conflict await

Wait for Mayor's resolution.

```bash
gt semantic-conflict await --mr gt-abc123 --timeout 1h
```

### gt semantic-conflict apply

Apply resolved values to beads.

```bash
gt semantic-conflict apply --resolution resolution.json
```

## Polecat Integration

For detection to work, Polecats must include `BEAD_CHANGES` metadata in commits:

```bash
git commit -m "Update priority due to security analysis

BEAD_CHANGES:
{
  \"bead_id\": \"gt-abc123\",
  \"polecat\": \"security-agent\",
  \"changes\": [
    {
      \"field\": \"priority\",
      \"old_value\": \"2\",
      \"new_value\": \"0\",
      \"confidence\": 0.95,
      \"reasoning\": \"CVE-2024-1234 detected with public exploit available\"
    }
  ]
}
"
```

### BEAD_CHANGES Format

```json
{
  "bead_id": "gt-abc123",
  "polecat": "agent-name",
  "changes": [
    {
      "field": "priority",
      "old_value": "2",
      "new_value": "0",
      "confidence": 0.95,
      "reasoning": "Explanation for this change"
    }
  ]
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `bead_id` | Yes | The bead being modified |
| `polecat` | Yes | Agent making the change |
| `changes[].field` | Yes | Field being modified |
| `changes[].old_value` | Yes | Previous value |
| `changes[].new_value` | Yes | New value |
| `changes[].confidence` | No | 0.0-1.0 confidence score |
| `changes[].reasoning` | No | Why this change was made |

## Mayor Resolution

### Escalation Mail Format

```
Subject: SEMANTIC_CONFLICT_ESCALATED gt-abc123

Semantic conflicts detected in MR: gt-abc123

## Conflict 1: gt-abc123.priority

**Change 1** (by security-agent):
- Value: 2 -> 0
- Confidence: 0.95
- Reasoning: CVE-2024-1234 detected with public exploit available

**Change 2** (by product-agent):
- Value: 0 -> 2
- Confidence: 0.60
- Reasoning: Edge case only affects <1% of users

---
Please review and provide a resolution.
```

### Resolution Format

Mayor replies with:

```json
{
  "resolutions": {
    "gt-abc123:priority": "0"
  },
  "reasoning": "Security vulnerabilities with public exploits take precedence over user impact concerns."
}
```

## Plugin vs Core

This feature is implemented as a **plugin** rather than core code because:

1. **Workflow-specific** - Not all teams need semantic conflict detection
2. **Configurable** - Teams choose which fields to escalate
3. **Optional** - Can be enabled/disabled per rig
4. **Extensible** - Can be modified without touching core

## Files

```
molecules/plugins/semantic-conflict-detector/
├── mol-semantic-conflict-detector.formula.json  # Plugin molecule
└── README.md                                     # Plugin docs

internal/cmd/
└── semantic_conflict.go                          # gt semantic-conflict command

docs/plugins/
└── semantic-conflicts.md                         # This file
```

## Related

- Issue #88: Feature request
- `docs/reference.md`: Plugin molecules documentation
- `gt escalate`: General escalation command
