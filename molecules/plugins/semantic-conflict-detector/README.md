# Semantic Conflict Detector Plugin

Detects semantic conflicts in MR bead modifications and escalates to Mayor for decision.

## What is a Semantic Conflict?

When multiple Polecats modify the same bead field with different values, that's a **semantic conflict** - different professional judgments that need discussion, not arbitrary resolution.

**Example:**
- Polecat A (security): `priority = 0` (critical vulnerability)
- Polecat B (product): `priority = 2` (low user impact)

Auto-resolve (LWW) would silently discard one expert's judgment. This plugin escalates to Mayor instead.

## Installation

This plugin is included in Gas Town. Enable it in your rig's `config.json`:

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

## Usage

### Automatic (via Refinery Patrol)

When enabled, the Refinery patrol molecule bonds this plugin for each MR:

```bash
bd mol bond mol-semantic-conflict-detector $MR_ID --var branch="$BRANCH"
```

### Manual

You can also run the detection manually:

```bash
# Detect conflicts
gt semantic-conflict detect --branch polecat/toast/gt-abc --target main

# Escalate to Mayor
gt semantic-conflict escalate --conflicts conflicts.json --mr gt-abc123

# Wait for resolution
gt semantic-conflict await --mr gt-abc123 --timeout 1h

# Apply resolution
gt semantic-conflict apply --resolution resolution.json
```

## Polecat Integration

For conflicts to be detected, Polecats must include structured metadata in commits:

```bash
git commit -m "Update priority based on security analysis

BEAD_CHANGES:
{
  \"bead_id\": \"gt-abc123\",
  \"polecat\": \"security-agent\",
  \"changes\": [{
    \"field\": \"priority\",
    \"old_value\": \"2\",
    \"new_value\": \"0\",
    \"confidence\": 0.95,
    \"reasoning\": \"CVE-2024-1234 with public exploit\"
  }]
}
"
```

## Mayor Resolution

When Mayor receives an escalation, they reply with a resolution:

```json
{
  "resolutions": {
    "gt-abc123:priority": "0"
  },
  "reasoning": "Security vulnerabilities with public exploits take precedence"
}
```

## Plugin Steps

1. **detect-conflicts** - Analyze commits for bead field changes
2. **escalate-to-mayor** - Send mail to Mayor with conflict details
3. **await-mayor-decision** - Block until Mayor responds
4. **apply-resolution** - Update beads with resolved values

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `false` | Enable/disable the plugin |
| `escalate_fields` | `["priority", "assignee"]` | Fields requiring Mayor decision |
| `timeout` | `"1h"` | Timeout waiting for Mayor |

## Files

- `mol-semantic-conflict-detector.formula.json` - Plugin molecule definition
- `README.md` - This file

## Related

- Issue #88: Original feature request
- `gt escalate` - General escalation command
- `gt mail` - Mail system commands
