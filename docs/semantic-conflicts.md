# Semantic Conflict Detection and Mayor Escalation

## Overview

Semantic conflicts occur when multiple Polecats modify the same bead field concurrently with different professional judgments. Unlike git merge conflicts (file-level), semantic conflicts represent disagreements in **decision-making** that require human or Mayor review rather than automatic resolution.

## Problem Statement

Current conflict resolution strategies (Last-Write-Wins, Union, etc.) work well for technical merge conflicts but can silently discard important expert disagreements:

**Example:**
- **Polecat A (security-focused)**: sets `priority = 0` with reasoning "CVE-2024-1234 detected with public exploit"
- **Polecat B (product-focused)**: sets `priority = 2` with reasoning "Edge case only affects <1% of users"

With automatic Last-Write-Wins resolution:
- Whichever polecat's commit is processed last wins
- The other polecat's expertise and reasoning is silently discarded
- Critical security judgment might be overwritten by product priority

**This is a semantic conflict** - different professional perspectives that need discussion, not arbitrary resolution.

## Architecture

### Detection Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                         Refinery                                 │
│                                                                  │
│  ProcessMR(mr)                                                   │
│    ├─> detectSemanticConflicts(mr)                              │
│    │     ├─> Analyze git commits in MR branch                   │
│    │     ├─> Extract bead modifications from commits            │
│    │     ├─> Group changes by bead:field                        │
│    │     └─> Check if field is in escalate_fields config        │
│    │                                                             │
│    ├─> IF conflicts found:                                      │
│    │     ├─> Acquire merge slot (serialize decisions)           │
│    │     ├─> Send escalation mail to Mayor                      │
│    │     ├─> Block MR pending decision                          │
│    │     └─> Return CONFLICT status                             │
│    │                                                             │
│    └─> ELSE: doMerge() as normal                                │
└─────────────────────────────────────────────────────────────────┘
                            │
                            │ Mail: SEMANTIC_CONFLICT_ESCALATED
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                          Mayor                                   │
│                                                                  │
│  Receives escalation mail:                                       │
│    ├─> Review conflicting changes                               │
│    ├─> Review confidence scores & reasoning                     │
│    ├─> Make decision (via Claude or human)                      │
│    └─> Send resolution mail to Witness                          │
└─────────────────────────────────────────────────────────────────┘
                            │
                            │ Mail: SEMANTIC_CONFLICT_RESOLVED
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Witness                                  │
│                                                                  │
│  HandleSemanticConflictResolved(payload):                        │
│    ├─> Apply Mayor's resolution to beads                        │
│    ├─> Release merge slot                                       │
│    ├─> Notify polecats about decision                           │
│    └─> MR unblocks and re-enters queue                          │
└─────────────────────────────────────────────────────────────────┘
                            │
                            │ MR retries merge
                            ▼
                    [Normal merge flow]
```

### Key Components

1. **Refinery** (`internal/refinery/semantic_conflict.go`)
   - Detects semantic conflicts by analyzing git history
   - Acquires merge slot to serialize decision-making
   - Sends escalation mail to Mayor

2. **Protocol Types** (`internal/protocol/types.go`)
   - `TypeSemanticConflictEscalated`: Refinery → Mayor
   - `TypeSemanticConflictResolved`: Mayor → Witness
   - `SemanticConflictEscalatedPayload`: Contains conflict details
   - `SemanticConflictResolvedPayload`: Contains Mayor's decision

3. **Witness Handler** (`internal/protocol/witness_handlers.go`)
   - Receives resolution from Mayor
   - Applies resolved values to beads
   - Releases merge slot
   - Notifies polecats

## Configuration

### Enable Semantic Conflict Detection

Add to your rig's `config.json`:

```json
{
  "merge_queue": {
    "semantic_conflicts": {
      "enabled": true,
      "escalate_fields": ["priority", "assignee", "estimated_minutes"],
      "auto_resolve_fields": ["labels", "title", "description"],
      "escalation_timeout": "1h",
      "require_confidence": false
    }
  }
}
```

### Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable semantic conflict detection (opt-in) |
| `escalate_fields` | []string | `["priority", "assignee"]` | Fields that require Mayor decision on conflict |
| `auto_resolve_fields` | []string | `["labels", "title"]` | Fields that use automatic resolution (LWW, Union) |
| `escalation_timeout` | duration | `1h` | Max time to wait for Mayor before falling back to LWW |
| `require_confidence` | bool | `false` | Require Polecats to report confidence scores |

### Field Classification

Choose which fields escalate vs auto-resolve based on:

**Escalate to Mayor:**
- Fields requiring professional judgment (priority, assignee, estimated_minutes)
- Fields with business impact (status when contradictory)
- Fields where expertise matters (security_review, performance_impact)

**Auto-Resolve (LWW/Union):**
- Metadata fields (labels, tags)
- Descriptive text (title, description)
- Low-impact fields (color, icon)

## Polecat Integration

### Reporting Bead Changes with Confidence

Polecats can include structured metadata in commit messages to help Mayor make informed decisions:

```bash
git commit -m "Update priority to critical based on CVE analysis

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
      \"reasoning\": \"CVE-2024-1234 detected with public exploit available. Affects authentication module. Critical security risk.\"
    }
  ]
}
"
```

### Commit Message Format

The `BEAD_CHANGES:` block must be valid JSON with:

- `bead_id`: The bead being modified (e.g., "gt-abc123")
- `polecat`: Agent name making the change
- `changes`: Array of field modifications
  - `field`: Field name (e.g., "priority")
  - `old_value`: Previous value
  - `new_value`: New value
  - `confidence`: Optional float 0.0-1.0 (how confident in this change)
  - `reasoning`: Optional explanation of why this change was made

## Mayor Escalation Mail Format

When Refinery detects a semantic conflict, it sends a structured mail to Mayor:

```
To: mayor/
Subject: Decision needed: priority conflicts on gt-abc123
Priority: high
Type: task
Thread-ID: semantic-conflict-gt-abc123

Semantic conflicts detected in MR: gt-abc123

Title: Add authentication rate limiting
Branch: polecat/security-agent/gt-abc123

## Conflict 1: gt-abc123.priority

**Change 1** (by security-agent):
- Value: 2 -> 0
- Confidence: 0.95
- Reasoning: CVE-2024-1234 detected with public exploit available
- Commit: a1b2c3d4
- Timestamp: 2025-01-04T10:30:00Z

**Change 2** (by product-agent):
- Value: 0 -> 2
- Confidence: 0.60
- Reasoning: Edge case only affects <1% of users in legacy browser
- Commit: e5f6g7h8
- Timestamp: 2025-01-04T10:45:00Z

---
Please review the conflicting changes and provide a resolution.

To resolve:
1. Review the changes and their reasoning
2. Decide which value to accept (or provide a new value)
3. Reply to this mail with your decision

[Automated escalation from Refinery]
```

## Mayor Resolution Protocol

Mayor responds with a resolution mail:

```
To: witness-rig-1
Subject: SEMANTIC_CONFLICT_RESOLVED gt-abc123
Type: reply
Thread-ID: semantic-conflict-gt-abc123

Resolution: Accept security-agent's priority=0

Reasoning: Security vulnerabilities with public exploits take precedence over
product impact concerns. The 1% user base on legacy browsers is acceptable risk
compared to authentication bypass vulnerability.

Resolutions:
{
  "gt-abc123:priority": "0"
}

Decision made by: mayor (Claude Sonnet 4.5)
```

The Witness handler parses this and:
1. Applies `priority=0` to bead `gt-abc123`
2. Releases merge slot
3. Allows MR to retry merge
4. Notifies both polecats of the decision

## Merge Slot Behavior

Semantic conflict resolution uses the **merge slot** to serialize decision-making:

1. **Refinery detects conflict** → Acquires merge slot
2. **Sends escalation to Mayor** → Holds slot while waiting
3. **Other MRs encounter conflicts** → Join waiters queue (non-blocking)
4. **Mayor decides** → Witness applies resolution
5. **Witness releases slot** → Next waiter acquires slot
6. **Original MR unblocks** → Retries merge with resolved values

**Benefits:**
- Only one semantic conflict resolved at a time (prevents thrashing)
- Other MRs can continue if no conflicts (non-blocking queue)
- FIFO fairness via waiters queue
- Clear audit trail of who holds slot and why

## Use Cases

### 1. Priority Disagreements

**Scenario:** Security and product teams assess risk differently

- **Security agent**: "P0 - Critical CVE"
- **Product agent**: "P2 - Low user impact"

**Resolution:** Mayor reviews CVE severity, exploit availability, and user impact to make final priority call

### 2. Assignee Conflicts

**Scenario:** Multiple agents claim ownership of same task

- **Agent A**: "I have the required security clearance"
- **Agent B**: "I have domain expertise in authentication"

**Resolution:** Mayor considers both factors and assigns based on priority

### 3. Estimate Disputes

**Scenario:** Different complexity assessments

- **Backend agent**: "estimated_minutes: 480" (8 hours - complex migration)
- **Frontend agent**: "estimated_minutes: 120" (2 hours - simple UI change)

**Resolution:** Mayor reviews both perspectives and sets realistic estimate

### 4. Status Contradictions

**Scenario:** One agent closes, another reopens

- **Testing agent**: "status: closed" (all tests pass)
- **Security agent**: "status: open" (found vulnerability during audit)

**Resolution:** Mayor investigates why security audit found issue after tests passed

## Timeout and Fallback

If Mayor doesn't respond within `escalation_timeout`:

1. **Log warning** that Mayor decision timed out
2. **Fall back to Last-Write-Wins** (most recent change wins)
3. **Release merge slot** to unblock queue
4. **Notify polecats** that auto-resolution was used
5. **Log timeout event** for later Mayor review

This prevents the queue from being permanently blocked by unresponsive Mayor.

## Best Practices

### For Polecats

1. **Always include reasoning** when modifying escalate_fields
2. **Report confidence scores** to help Mayor prioritize
3. **Reference evidence** (CVE numbers, user metrics, etc.)
4. **Be specific** in reasoning ("public exploit available" vs "security risk")

### For Teams

1. **Start conservative** - add only critical fields to escalate_fields
2. **Monitor escalation rate** - too many escalations indicate misconfiguration
3. **Review Mayor decisions** - use as training data for Polecat improvements
4. **Tune confidence thresholds** - low confidence changes might auto-defer to Mayor
5. **Set realistic timeouts** - balance responsiveness vs blocking

### For Mayor

1. **Review both perspectives** - don't automatically favor one agent type
2. **Consider confidence scores** - high confidence signals strong evidence
3. **Document reasoning** - helps Polecats learn from decisions
4. **Identify patterns** - recurring conflicts suggest process improvements
5. **Escalate to human** when uncertainty is high or stakes are critical

## Limitations and Future Work

### Current Limitations

1. **Commit message parsing** - Requires Polecats to follow BEAD_CHANGES format
2. **TODO: Bead resolution application** - Witness handler needs beads access
3. **TODO: Polecat notifications** - Not yet implemented in Witness handler
4. **No automatic retry** - MR must be manually resubmitted after resolution
5. **Single-field granularity** - Can't detect conflicts across related fields

### Future Enhancements

1. **Automatic MR retry** after resolution
2. **Confidence-based auto-resolution** (e.g., 0.95 vs 0.60 → accept 0.95)
3. **Related field analysis** (e.g., priority + estimated_minutes correlation)
4. **Historical decision learning** (Mayor learns from past resolutions)
5. **Real-time collaboration** (Polecats negotiate before committing)
6. **Conflict prediction** (warn before committing likely-conflicting change)

## Troubleshooting

### Semantic conflicts not detected

**Check:**
- Is `semantic_conflicts.enabled = true` in config.json?
- Are the fields in `escalate_fields` list?
- Do commits include BEAD_CHANGES metadata?
- Is Refinery processing MRs (not skipping due to errors)?

### Mayor not responding

**Check:**
- Is Mayor agent running? (`gt agents list`)
- Does Mayor have mail access? (`gt mail check` as mayor)
- Is escalation mail in Mayor's inbox?
- Has escalation_timeout expired?

### Merge slot never released

**Check:**
- Did Witness receive SEMANTIC_CONFLICT_RESOLVED?
- Is Witness handler implemented correctly?
- Check logs for slot release errors
- Manual release: `bd slot release <rig>/refinery`

### MR blocked indefinitely

**Workaround:**
1. Check escalation status: `gt mail check` (as Mayor)
2. If timeout expired, manually close escalation mail
3. Release merge slot: `bd slot release <rig>/refinery`
4. Resubmit MR: `gt done` (as Polecat)

## Related Documentation

- [Architecture](architecture.md) - Overall system architecture
- [Merge Queue](merge-queue.md) - MR processing and conflict resolution
- [Mail System](mail-system.md) - Inter-agent messaging
- [Witness](witness.md) - Polecat lifecycle monitoring
- [Beads](beads.md) - Issue storage and state management

## Examples

### Example config.json

```json
{
  "merge_queue": {
    "enabled": true,
    "target_branch": "main",
    "on_conflict": "assign_back",
    "semantic_conflicts": {
      "enabled": true,
      "escalate_fields": [
        "priority",
        "assignee",
        "estimated_minutes",
        "security_review",
        "breaking_change"
      ],
      "auto_resolve_fields": [
        "labels",
        "title",
        "description",
        "tags"
      ],
      "escalation_timeout": "2h",
      "require_confidence": true
    }
  }
}
```

### Example Polecat Commit

```bash
# Security agent sets priority to critical
git commit -m "Escalate priority due to authentication bypass

This issue allows unauthenticated access to user data via
session token reuse vulnerability. Public exploit available.

BEAD_CHANGES:
{
  \"bead_id\": \"gt-sec-001\",
  \"polecat\": \"security-scanner\",
  \"changes\": [
    {
      \"field\": \"priority\",
      \"old_value\": \"2\",
      \"new_value\": \"0\",
      \"confidence\": 0.98,
      \"reasoning\": \"CVE-2024-5678: Authentication bypass via session reuse. Public exploit available on exploit-db. CVSS 9.1 (Critical). Affects all users.\"
    },
    {
      \"field\": \"security_review\",
      \"old_value\": \"pending\",
      \"new_value\": \"required\",
      \"confidence\": 1.0,
      \"reasoning\": \"Critical security vulnerability requires mandatory review before merge.\"
    }
  ]
}
"
```

### Example Mayor Decision

```
To: witness-gastown
Subject: SEMANTIC_CONFLICT_RESOLVED gt-sec-001
Priority: high

Decision: Accept security-scanner's assessment

After reviewing both perspectives:
- Security: priority=0, confidence=0.98, CVE-2024-5678 with public exploit
- Product: priority=2, confidence=0.65, low usage of affected feature

The security assessment is correct. Authentication bypass vulnerabilities
always take precedence regardless of feature usage. The public exploit
availability escalates urgency to P0.

Additional actions:
- Notify security team for immediate patch
- Prepare security advisory for users
- Schedule emergency release

Resolutions:
{
  "gt-sec-001:priority": "0",
  "gt-sec-001:security_review": "required"
}

Decision made by: mayor (Claude Sonnet 4.5)
Reviewed by: human (security lead)
```

## Conclusion

Semantic conflict detection preserves expert judgment in collaborative agent workflows. By escalating decision conflicts to Mayor (or humans), the system maintains both autonomy (agents work independently) and oversight (critical decisions are reviewed).

This feature is **opt-in** and **configurable** to match your team's needs. Start conservatively with high-impact fields, monitor escalation patterns, and tune configuration based on observed behavior.
