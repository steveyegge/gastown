# Plugin System Design

> Design document for the Gas Town plugin system.
> Written 2026-01-11, crew/george session.

## Problem Statement

Gas Town needs extensible, project-specific automation that runs during Deacon patrol cycles. The immediate use case is rebuilding stale binaries (gt, bd, wv), but the pattern generalizes to any periodic maintenance task.

Current state:
- Plugin infrastructure exists conceptually (patrol step mentions it)
- `~/gt/plugins/` directory exists with README
- No actual plugins in production use
- No formalized execution model

## Design Principles Applied

### Discover, Don't Track
> Reality is truth. State is derived.

Plugin state (last run, run count, results) lives on the ledger as wisps, not in shadow state files. Gate evaluation queries the ledger directly.

### ZFC: Zero Framework Cognition
> Agent decides. Go transports.

The Deacon (agent) evaluates gates and decides whether to dispatch. Go code provides transport (`gt dog dispatch`) but doesn't make decisions.

### MEOW Stack Integration

| Layer | Plugin Analog |
|-------|---------------|
| **M**olecule | `plugin.md` - work template with TOML frontmatter |
| **E**phemeral | Plugin-run wisps - high-volume, digestible |
| **O**bservable | Plugin runs appear in `bd activity` feed |
| **W**orkflow | Gate → Dispatch → Execute → Record → Digest |

---

## Architecture

### Plugin Locations

```
~/gt/
├── plugins/                      # Town-level plugins (universal)
│   └── README.md
├── gastown/
│   └── plugins/                  # Rig-level plugins
│       └── rebuild-gt/
│           └── plugin.md
├── beads/
│   └── plugins/
│       └── rebuild-bd/
│           └── plugin.md
└── wyvern/
    └── plugins/
        └── rebuild-wv/
            └── plugin.md
```

**Town-level** (`~/gt/plugins/`): Universal plugins that apply everywhere.
**Rig-level** (`<rig>/plugins/`): Project-specific plugins.

The Deacon scans both locations during patrol.

### Execution Model: Dog Dispatch

**Key insight**: Plugin execution should not block Deacon patrol.

Dogs are reusable workers designed for infrastructure tasks. Plugin execution is dispatched to dogs:

```
Deacon Patrol                    Dog Worker
─────────────────               ─────────────────
1. Scan plugins
2. Evaluate gates
3. For open gates:
   └─ gt dog dispatch plugin     ──→ 4. Execute plugin
      (non-blocking)                  5. Create result wisp
                                      6. Send DOG_DONE
4. Continue patrol
   ...
5. Process DOG_DONE              ←── (next cycle)
```

Benefits:
- Deacon stays responsive
- Multiple plugins can run concurrently (different dogs)
- Plugin failures don't stall patrol
- Consistent with Dogs' purpose (infrastructure work)

### State Tracking: Wisps on the Ledger

Each plugin run creates a wisp:

```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:rebuild-gt \
  --label rig:gastown \
  --label result:success \
  --body "Rebuilt gt: abc123 → def456 (5 commits)"
```

**Gate evaluation** queries wisps instead of state files:

```bash
# Cooldown check: any runs in last hour?
bd list --type=wisp --label=plugin:rebuild-gt --since=1h --limit=1
```

**Derived state** (no state.json needed):

| Query | Command |
|-------|---------|
| Last run time | `bd list --label=plugin:X --limit=1 --json` |
| Run count | `bd list --label=plugin:X --json \| jq length` |
| Last result | Parse `result:` label from latest wisp |
| Failure rate | Count `result:failure` vs total |

### Digest Pattern

Like cost digests, plugin wisps accumulate and get squashed daily:

```bash
gt plugin digest --yesterday
```

Creates: `Plugin Digest 2026-01-10` bead with summary
Deletes: Individual plugin-run wisps from that day

This keeps the ledger clean while preserving audit history.

---

## Plugin Format Specification

### File Structure

```
rebuild-gt/
└── plugin.md      # Definition with TOML frontmatter
```

### plugin.md Format

```markdown
+++
name = "rebuild-gt"
description = "Rebuild stale gt binary from source"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-gt", "rig:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
+++

# Rebuild gt Binary

Instructions for the dog worker to execute...
```

### TOML Frontmatter Schema

```toml
# Required
name = "string"           # Unique plugin identifier
description = "string"    # Human-readable description
version = 1               # Schema version (for future evolution)

[gate]
type = "cooldown|cron|condition|event|manual"
# Type-specific fields:
duration = "1h"           # For cooldown
schedule = "0 9 * * *"    # For cron
check = "gt stale -q"     # For condition (exit 0 = run)
on = "startup"            # For event

[tracking]
labels = ["label:value", ...]  # Labels for execution wisps
digest = true|false            # Include in daily digest

[execution]
timeout = "5m"            # Max execution time
notify_on_failure = true  # Escalate on failure
severity = "low"          # Escalation severity if failed
```

### Gate Types

| Type | Config | Behavior |
|------|--------|----------|
| `cooldown` | `duration = "1h"` | Query wisps, run if none in window |
| `cron` | `schedule = "0 9 * * *"` | Run on cron schedule |
| `condition` | `check = "cmd"` | Run check command, run if exit 0 |
| `event` | `on = "startup"` | Run on Deacon startup |
| `manual` | (no gate section) | Never auto-run, dispatch explicitly |

### Instructions Section

The markdown body after the frontmatter contains agent-executable instructions. The dog worker reads and executes these steps.

Standard sections:
- **Detection**: Check if action is needed
- **Action**: The actual work
- **Record Result**: Create the execution wisp
- **Notification**: On success/failure

---

## Escalation System

### Problem

Current escalation is ad-hoc "mail Mayor". Issues:
- Mayor gets backlogged easily
- No severity differentiation
- No alternative channels (email, SMS, etc.)
- No tracking of stale escalations

### Solution: Unified Escalation API

New command:

```bash
gt escalate \
  --severity=<low|medium|high|critical> \
  --subject="Plugin FAILED: rebuild-gt" \
  --body="Build failed: make returned exit code 2" \
  --source="plugin:rebuild-gt"
```

### Escalation Routing

The command reads town config (`~/gt/config.json` or similar) for routing rules:

```json
{
  "escalation": {
    "routes": {
      "low": ["bead"],
      "medium": ["bead", "mail:mayor"],
      "high": ["bead", "mail:mayor", "email:human"],
      "critical": ["bead", "mail:mayor", "email:human", "sms:human"]
    },
    "contacts": {
      "human_email": "steve@example.com",
      "human_sms": "+1234567890"
    },
    "stale_threshold": "4h"
  }
}
```

### Escalation Actions

| Action | Behavior |
|--------|----------|
| `bead` | Create escalation bead with severity label |
| `mail:mayor` | Send mail to mayor/ |
| `email:human` | Send email via configured service |
| `sms:human` | Send SMS via configured service |

### Escalation Beads

Every escalation creates a bead:

```yaml
type: escalation
status: open
labels:
  - severity:high
  - source:plugin:rebuild-gt
  - acknowledged:false
```

### Stale Escalation Patrol

A patrol step (or plugin!) checks for unacknowledged escalations:

```bash
bd list --type=escalation --label=acknowledged:false --older-than=4h
```

Stale escalations get re-escalated at higher severity.

### Acknowledging Escalations

```bash
gt escalate ack <bead-id>
# Sets label acknowledged:true
```

---

## New Commands Required

### gt stale

Expose binary staleness check:

```bash
gt stale              # Human-readable output
gt stale --json       # Machine-readable
gt stale --quiet      # Exit code only (0=stale, 1=fresh)
```

### gt dog dispatch

Formalized plugin dispatch to dogs:

```bash
gt dog dispatch --plugin <name> [--rig <rig>]
```

This:
1. Finds the plugin definition
2. Slinga a standardized work unit to an idle dog
3. Returns immediately (non-blocking)

### gt escalate

Unified escalation API:

```bash
gt escalate \
  --severity=<level> \
  --subject="..." \
  --body="..." \
  [--source="..."]

gt escalate ack <bead-id>
gt escalate list [--severity=...] [--stale]
```

### gt plugin

Plugin management:

```bash
gt plugin list                    # List all plugins
gt plugin show <name>             # Show plugin details
gt plugin run <name> [--force]    # Manual trigger
gt plugin digest [--yesterday]    # Squash wisps to digest
gt plugin history <name>          # Show execution history
```

---

## Implementation Plan

### Phase 1: Foundation

1. **`gt stale` command** - Expose CheckStaleBinary() via CLI
2. **Plugin format spec** - Finalize TOML schema
3. **Plugin scanning** - Deacon scans town + rig plugin dirs

### Phase 2: Execution

4. **`gt dog dispatch --plugin`** - Formalized dog dispatch
5. **Plugin execution in dogs** - Dog reads plugin.md, executes
6. **Wisp creation** - Record results on ledger

### Phase 3: Gates & State

7. **Gate evaluation** - Cooldown via wisp query
8. **Other gate types** - Cron, condition, event
9. **Plugin digest** - Daily squash of plugin wisps

### Phase 4: Escalation

10. **`gt escalate` command** - Unified escalation API
11. **Escalation routing** - Config-driven multi-channel
12. **Stale escalation patrol** - Check unacknowledged

### Phase 5: First Plugin

13. **`rebuild-gt` plugin** - The actual gastown plugin
14. **Documentation** - So Beads/Wyvern can create theirs

---

## Example: rebuild-gt Plugin

```markdown
+++
name = "rebuild-gt"
description = "Rebuild stale gt binary from gastown source"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-gt", "rig:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "medium"
+++

# Rebuild gt Binary

Checks if the gt binary is stale (built from older commit than HEAD) and rebuilds.

## Gate Check

The Deacon evaluates this before dispatch. If gate closed, skip.

## Detection

Check binary staleness:

```bash
gt stale --json
```

If `"stale": false`, record success wisp and exit early.

## Action

Rebuild from source:

```bash
cd ~/gt/gastown/crew/george && make build && make install
```

## Record Result

On success:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:rebuild-gt \
  --label rig:gastown \
  --label result:success \
  --body "Rebuilt gt: $OLD → $NEW ($N commits)"
```

On failure:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:rebuild-gt \
  --label rig:gastown \
  --label result:failure \
  --body "Build failed: $ERROR"

gt escalate --severity=medium \
  --subject="Plugin FAILED: rebuild-gt" \
  --body="$ERROR" \
  --source="plugin:rebuild-gt"
```
```

---

## Open Questions

1. **Plugin discovery in multiple clones**: If gastown has crew/george, crew/max, crew/joe - which clone's plugins/ dir is canonical? Probably: scan all, dedupe by name, prefer rig-root if exists.

2. **Dog assignment**: Should specific plugins prefer specific dogs? Or any idle dog?

3. **Plugin dependencies**: Can plugins depend on other plugins? Probably not in v1.

4. **Plugin disable/enable**: How to temporarily disable a plugin without deleting it? Label on a plugin bead? `enabled = false` in frontmatter?

---

## References

- PRIMING.md - Core design principles
- mol-deacon-patrol.formula.toml - Patrol step plugin-run
- ~/gt/plugins/README.md - Current plugin stub
