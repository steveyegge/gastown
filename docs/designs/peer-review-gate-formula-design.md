# Design: Formula-Based Peer Review Gate

**Bead**: gt-05z3
**Context**: Steve's review of PR #3107 — peer review gate belongs in the formula/molecule layer, not core protocol/witness/config.

## Problem Statement

PR #3107 implemented peer review gates by adding REVIEW_PASSED/REVIEW_FAILED protocol types, witness handler methods, PeerReview config fields, and QualityTier structs to core. Steve correctly identified that "review before merge" is a **policy decision** (formula layer) not **plumbing** (protocol layer).

The design challenge: achieve the same peer review gating using only formula variables, formula overlays, event channels, and existing sling infrastructure — zero changes to protocol types, witness handlers, or core config structs.

## Design Decision: Pre-Merge Formula Step in Refinery Patrol

**Chosen approach**: Option A from Steve's feedback — a pre-merge formula step in the refinery patrol.

**Why not Option B** (wrapping molecule around `gt done` → refinery flow): The wrapping molecule approach would add orchestration complexity between `gt done` and refinery, require new coordination points, and is harder to reason about. The refinery already processes MRs sequentially and already has a `quality-review` step (currently measurement-only). Upgrading that step is surgically precise.

## Architecture

```
Polecat         gt done         Witness         Refinery                    Review Polecat
  │               │               │               │                            │
  │ push branch   │               │               │                            │
  │──────────────>│               │               │                            │
  │               │ MERGE_READY   │               │                            │
  │               │──────────────>│               │                            │
  │               │               │ MERGE_READY   │                            │
  │               │               │──────────────>│                            │
  │               │               │               │                            │
  │               │               │         (rebase, tests)                    │
  │               │               │               │                            │
  │               │               │         [if peer_review_enabled]           │
  │               │               │               │ gt sling mol-peer-review   │
  │               │               │               │───────────────────────────>│
  │               │               │               │                            │
  │               │               │               │ await-event review-<mr>    │
  │               │               │               │         ...                │
  │               │               │               │                     (review diff)
  │               │               │               │                            │
  │               │               │               │ REVIEW_VERDICT event       │
  │               │               │               │<───────────────────────────│
  │               │               │               │                     (gt done)
  │               │               │               │                            ×
  │               │               │         [PASS → merge]
  │               │               │         [FAIL → handle-failures]
```

**Key difference from PR #3107**: No new protocol types. No witness involvement in review routing. The refinery both spawns and consumes the review verdict. The witness sees only MERGE_READY and MERGED — same as today.

## Detailed Design

### 1. How does the refinery know to run a review step?

**Formula variable**: Add `peer_review_enabled` to `mol-refinery-patrol.formula.toml`.

```toml
[vars.peer_review_enabled]
description = "Require peer review before merge (true/false)"
default = "false"

[vars.peer_review_formula]
description = "Formula for peer review (default: mol-peer-review-gate)"
default = "mol-peer-review-gate"
```

The existing `quality-review` step checks this variable. When `peer_review_enabled=true`, the step spawns a review polecat and waits for the verdict instead of doing a self-review.

**Rig-level activation**: Set via rig settings (the same `loadRigCommandVars` mechanism that already injects `setup_command`, `test_command`, etc.):

```json
// <rig>/settings/config.json
{
  "merge_queue": {
    "peer_review_enabled": true,
    "peer_review_formula": "mol-peer-review-gate"
  }
}
```

The `loadRigCommandVars()` function in `sling_helpers.go` is extended to read `peer_review_enabled` and `peer_review_formula` from MergeQueueConfig and inject them as formula variables. This requires adding two fields to MergeQueueConfig — but these are **formula variable sources**, not protocol concepts. They sit alongside `setup_command`, `test_command`, etc.

**Alternative (zero config changes)**: Use formula overlays exclusively:

```toml
# <town>/<rig>/formula-overlays/mol-refinery-patrol.toml
[[step-overrides]]
step_id = "quality-review"
mode = "replace"
description = """
... (review step with peer_review_enabled=true baked in) ...
"""
```

This works but is less ergonomic. **Recommendation**: Add `peer_review_enabled` and `peer_review_formula` to `MergeQueueConfig` as simple string fields (not a new struct, not protocol types). This parallels the existing pattern for `setup_command` etc.

### 2. How does the review polecat get spawned?

The refinery spawns it using `gt sling`, the same mechanism used for all polecat spawning. No new spawning infrastructure needed.

```bash
# In the quality-review step, when peer_review_enabled=true:
gt sling {{peer_review_formula}} {{rig}} \
  --var issue=<source-issue-id> \
  --var branch=<polecat-branch> \
  --var polecat=<original-polecat-name> \
  --var mr_bead=<mr-bead-id> \
  --var rig=<rig-name> \
  --args "Review MR for <source-issue-id>"
```

This creates a new polecat with the `mol-peer-review-gate` formula attached. The review polecat:
1. Loads the diff (`git diff origin/main...origin/<branch>`)
2. Reads the original bead requirements (`bd show <issue>`)
3. Reviews completeness and correctness
4. Writes verdict to MR bead notes
5. Emits verdict event
6. Self-cleans via `gt done --cleanup-status clean`

### 3. How does the review verdict get back to the refinery?

**Event channel**: The review polecat emits a verdict event on a per-MR channel. The refinery subscribes to that channel.

**Channel naming**: `review-<mr-bead-id>` (scoped to the specific MR to avoid cross-talk).

**Review polecat emits** (in its verdict step):
```bash
# PASS:
gt mol step emit-event --channel review-<mr-bead-id> --type REVIEW_VERDICT \
  --payload verdict=PASS --payload issue=<issue> --payload branch=<branch>

# FAIL:
gt mol step emit-event --channel review-<mr-bead-id> --type REVIEW_VERDICT \
  --payload verdict=FAIL --payload issue=<issue> --payload branch=<branch> \
  --payload findings="<summary of blocking issues>"
```

**Refinery waits** (in the quality-review step):
```bash
gt mol step await-event --channel review-<mr-bead-id> \
  --timeout 15m --cleanup
```

**Timeout handling**: If the review polecat crashes or takes too long:
- 15-minute timeout → refinery logs a warning and proceeds to merge (fail-open for Phase 1)
- Phase 2 could make this fail-closed (configurable via `peer_review_fail_mode=open|closed`)

**Bead notes as durable record**: In addition to the event, the review polecat writes the full verdict to the MR bead notes. This serves as the permanent audit trail:

```bash
bd update <mr-bead-id> --notes "PEER_REVIEW_VERDICT: PASS
Reviewer: <review-polecat-name>
Issue: <source-issue>
Branch: <branch>
..."
```

The event is the fast path (wakes the refinery immediately). The bead notes are the durable record (survives session death, provides audit trail).

### 4. How does the quality tier concept work without core config?

Quality tiers become **formula variable combinations**, not a new config struct.

| Tier | Variables | Behavior |
|------|-----------|----------|
| Standard | `peer_review_enabled=false` (default) | Tests only, no review |
| Reviewed | `peer_review_enabled=true` | Tests + peer review gate |
| Full | `peer_review_enabled=true`, `design_review_enabled=true` | Tests + peer review + design review |

These are set via rig settings or formula overlays — the operator chooses the quality level per rig.

The `QualityTier` config struct from PR #3107 is **not needed**. The tier is an emergent property of which review variables are enabled. If we later want a single knob:

```json
// Optional convenience — maps to individual variables
{
  "merge_queue": {
    "quality_tier": "reviewed"  // → sets peer_review_enabled=true
  }
}
```

But this is sugar, not a requirement. Individual variables are more flexible and composable.

**Per-bead override**: A bead can request a specific review level via labels:
```bash
bd update <issue> --label review:full
```

The review step reads this label and adjusts behavior. No core config needed — it's formula-level logic reading bead metadata.

### 5. How does the dispute protocol work without witness involvement?

The dispute protocol is entirely within the formula layer, mediated by bead notes.

**Dispute flow**:

1. Review polecat emits `REVIEW_VERDICT: FAIL` with findings
2. Refinery's `handle-failures` step sends `FIX_NEEDED` to the original polecat (existing mechanism — no changes needed)
3. Original polecat receives FIX_NEEDED, reads the review findings from MR bead notes
4. If the polecat disagrees, it writes counter-evidence to the bead:
   ```bash
   bd update <mr-bead-id> --notes "DISPUTE: <counter-evidence>"
   ```
5. Polecat resubmits the MR (existing `gt mq submit` flow)
6. When the refinery processes the resubmission, the quality-review step detects the dispute:
   - Reads MR bead notes for `DISPUTE:` prefix
   - Spawns a **new** review polecat (different from the first — fresh perspective)
   - New reviewer sees both the original findings and the counter-evidence
   - New reviewer's verdict is final (no infinite loops)

**Dispute detection** (in the quality-review step):
```bash
# Check if this is a re-review with dispute
NOTES=$(bd show <mr-bead-id> --field notes)
if echo "$NOTES" | grep -q "DISPUTE:"; then
  # Spawn review with dispute context
  gt sling {{peer_review_formula}} {{rig}} \
    --var issue=<issue> --var branch=<branch> \
    --var dispute=true \
    --args "Re-review with dispute. Original findings and counter-evidence in bead notes."
fi
```

**No witness involvement**: The witness never sees REVIEW_PASSED, REVIEW_FAILED, or dispute signals. It only sees the standard MERGE_READY → MERGED flow. The entire review lifecycle is contained within the refinery's patrol formula.

## Changes Required

### Formula Changes (keep from PR #3107)

1. **`mol-peer-review-gate.formula.toml`** — Keep as-is, with one change: emit verdict to `review-<mr-bead-id>` channel instead of `witness` channel
2. **`mol-peer-review-design.formula.toml`** — Keep as-is (design review formula)
3. **`mol-peer-review-research.formula.toml`** — Keep as-is (research review formula)
4. **`rule-of-design.formula.toml`** — Keep as-is (expansion formula)
5. **`rule-of-research.formula.toml`** — Keep as-is (expansion formula)

### Formula Changes (new/modified)

6. **`mol-refinery-patrol.formula.toml`** — Upgrade `quality-review` step:
   - Add `peer_review_enabled`, `peer_review_formula`, `design_review_enabled` variables
   - Replace quality-review step description with review-spawning logic
   - Add `peer_review_fail_mode` variable (default: "open" for Phase 1)

### Go Changes (minimal)

7. **`internal/config/types.go`** — Add `PeerReviewEnabled` and `PeerReviewFormula` string fields to `MergeQueueConfig` (alongside existing `SetupCommand`, `TestCommand`, etc.)
8. **`internal/cmd/sling_helpers.go`** — Extend `loadRigCommandVars()` to inject `peer_review_enabled` and `peer_review_formula` from rig settings

### Go Changes to REMOVE (from PR #3107)

9. **Remove** `REVIEW_PASSED`/`REVIEW_FAILED` from `internal/protocol/types.go`
10. **Remove** `HandleReviewPassed`/`HandleReviewFailed` from `internal/protocol/witness_handlers.go`
11. **Remove** `isPeerReviewEnabled`/`emitReviewRequested` from `internal/witness/handlers.go`
12. **Remove** `QualityTier` config struct (`internal/config/quality_tier.go`)
13. **Remove** `REVIEW_PASSED`/`REVIEW_FAILED` from `internal/cmd/mail_drain.go` drainableSubjects
14. **Remove** `NewReviewPassedMessage`/`NewReviewFailedMessage` from `internal/protocol/messages.go`

## Updated quality-review Step

Here is the upgraded `quality-review` step for `mol-refinery-patrol.formula.toml`:

```toml
[[steps]]
id = "quality-review"
title = "Quality review merge diff"
needs = ["run-tests"]
description = """
**Config: judgment_enabled = {{judgment_enabled}}**
**Config: review_depth = {{review_depth}}**
**Config: peer_review_enabled = {{peer_review_enabled}}**
**Config: peer_review_formula = {{peer_review_formula}}**
**Config: peer_review_fail_mode = {{peer_review_fail_mode}}**

This step handles two modes:
1. **Self-review** (judgment_enabled=true): Refinery reviews the diff itself (measurement-only)
2. **Peer review** (peer_review_enabled=true): Spawn a review polecat and gate on verdict

Both modes can be active simultaneously (self-review records metrics, peer review gates).

## Peer Review Gate

If peer_review_enabled is not "true", skip peer review entirely.

**Step 1: Spawn review polecat**

```bash
gt sling {{peer_review_formula}} <rig> \
  --var issue=<source-issue-id> \
  --var branch=<polecat-branch> \
  --var polecat=<original-polecat-name> \
  --var mr_bead=<mr-bead-id> \
  --var rig=<rig-name>
```

**Step 2: Wait for verdict**

```bash
gt mol step await-event --channel review-<mr-bead-id> \
  --timeout 15m --cleanup
```

**Step 3: Process verdict**

Parse the REVIEW_VERDICT event payload:
- If verdict=PASS: Log "Peer review passed" and proceed to handle-failures
- If verdict=FAIL: Record failure, proceed to handle-failures (which sends FIX_NEEDED)
- If timeout: Check peer_review_fail_mode
  - "open" (default): Log warning, proceed to merge
  - "closed": Treat as failure, send FIX_NEEDED

**Dispute handling**: If bead notes contain "DISPUTE:", this is a re-review.
Spawn a fresh review polecat (different agent for independent perspective).
Max 2 review rounds (original + dispute). After that, auto-PASS to prevent loops.

## Self-Review (existing Phase 1 measurement)

If judgment_enabled is not "true", skip self-review.

[... existing self-review logic unchanged ...]
"""
```

## Phasing

### Phase 1 (This PR)
- Add formula variables and upgrade quality-review step
- Review polecat spawning via `gt sling`
- Event-based verdict communication
- Fail-open on timeout (log warning, proceed to merge)
- Add `PeerReviewEnabled`/`PeerReviewFormula` to MergeQueueConfig (simple string fields)
- Remove all protocol/witness/config changes from PR #3107
- Keep all 5 review formula TOMLs from PR #3107

### Phase 2 (Future)
- `peer_review_fail_mode=closed` option
- Per-bead review level override via labels
- Design review gate (`design_review_enabled`)
- Dispute protocol with counter-evidence and re-review
- Review metrics and quality score tracking
- Optional `quality_tier` convenience mapping in rig settings

## Validation Against Existing Infrastructure

| Requirement | Existing Mechanism | New Protocol? |
|-------------|-------------------|---------------|
| Refinery knows to review | Formula variable (`peer_review_enabled`) | No |
| Review polecat spawned | `gt sling <formula> <rig>` | No |
| Verdict communicated | `emit-event` / `await-event` channels | No |
| Quality tiers | Formula variable combinations | No |
| Dispute protocol | Bead notes + re-review cycle | No |
| Rig-level config | `loadRigCommandVars()` + MergeQueueConfig | Minimal (2 string fields) |
| Audit trail | MR bead notes | No |
| Failure handling | Existing `handle-failures` → FIX_NEEDED | No |
| Polecat lifecycle | Standard self-cleaning (`gt done`) | No |

**Zero new protocol message types. Zero new witness handler methods. Zero new config structs.**

Two new string fields on an existing config struct (`MergeQueueConfig`), following the identical pattern used for `SetupCommand`, `TestCommand`, `LintCommand`, etc.

## Alternative Considered: Pure Overlay (Zero Go Changes)

It's possible to implement this with **zero Go changes** by using formula overlays exclusively:

```toml
# <town>/<rig>/formula-overlays/mol-refinery-patrol.toml
[[step-overrides]]
step_id = "quality-review"
mode = "replace"
description = "... (full review step with hardcoded peer_review_enabled=true) ..."
```

This works but has drawbacks:
- No `gt info` visibility into review configuration
- Rig settings and formula variables are the standard configuration mechanism
- Operators expect `settings/config.json` to be the tuning knob

**Verdict**: Use formula variables + 2 MergeQueueConfig fields. Minimal Go changes, maximum operator ergonomics.
