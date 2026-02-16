# Wave 1: Anchor Health Gating

Status: draft  
Owner: governance control plane  
Scope: promotion gate only (no scoring model changes)

## Purpose

Add a constitutional brake that blocks all promotion when truth-anchor quality
is degraded.

This change protects direction over velocity:

- Freeze is an integrity state, not an outage state.
- Promotion lanes pause under measurement uncertainty.
- Active pointers remain stable.

## Definitions

### Anchor Health

Anchor health is computed by an independent pipeline:

```text
anchor_health =
  predictive_validity *
  external_concordance *
  calibration_quality *
  coverage
```

Each term must be:

- Windowed (rolling windows, no single-event dominance)
- Spike-resistant (robust estimators, clipped deltas)
- Derived from truth-anchor deltas, never promotion/oracle scores

### Thresholds

- `H_warn`: pre-freeze warning threshold
- `H_min`: freeze threshold (`H_warn > H_min`)

## Architectural Requirements

### 1. Independent Pipeline

Anchor-health computation must be:

- Separately versioned
- Separately audited
- Separately stress-tested
- Separately logged

No circular dependencies:

- Anchor health is upstream of promotion.
- Promotion inputs cannot feed back into anchor-health terms.

### 2. Promotion Gate Order

Anchor-health check executes first in promotion path:

```text
if anchor_health < H_min:
  freeze_all_promotion_lanes()
  open_anchor_investigation_artifact()
  return BLOCKED_ANCHOR_HEALTH
```

This runs before:

- visible score checks
- shadow score checks
- stress-drop checks
- truth-anchor pass checks
- tripwire checks

### 3. Freeze Semantics

Freeze means:

- Active pointer remains unchanged
- New promotions blocked
- Quarantine lanes continue diagnostics
- Exploratory lanes allowed only in sandbox (promotion-ineligible)

Freeze is not a full system halt.

### 4. Investigation Artifact

On freeze, auto-generate immutable artifact containing:

- Anchor-health term breakdown
- Drift trend graph
- Contradiction deltas
- Predictive-validity decay curve
- Recent anchor lineage pointer history

Unfreeze cannot proceed without artifact hash linkage.

### 5. Early Warning Path

```text
if anchor_health < H_warn:
  emit_pre_freeze_signal()
  escalate_monitoring_frequency()
```

Pre-freeze does not block promotion by itself.

## Governance and Operations

### Release Blocker Drill

Wave 1 rollout is blocked until simulated freeze drill passes.

Required drill shape:

- Inject ambiguous synthetic drift (plausibly real signal)
- Trigger freeze by crossing `H_min`
- Verify artifact generation and immutability
- Run threshold-tweak pressure test
- Require artifact + independent attestation for unfreeze

### Simulation Metrics (Human and System)

Track:

- Freeze acknowledgment latency
- Time to artifact review start
- Time to first bypass/override suggestion
- Policy-compliance violations

### Pass Criteria

- Freeze triggers mechanically at `H_min`
- No manual bypass or threshold move during drill
- Active pointer stays stable throughout freeze
- Unfreeze uses artifact hash + independent attestation

## Acceptance Tests

### Technical

1. Drift injection below `H_warn` emits pre-freeze signal.
2. Drift injection below `H_min` blocks promotions with `BLOCKED_ANCHOR_HEALTH`.
3. Promotion evaluation order confirms anchor-health gate is first.
4. Freeze preserves active pointer while diagnostic lanes continue.
5. Artifact is emitted, hash-linked, immutable.
6. Unfreeze without artifact hash fails.

### Adversarial

1. False anchor signal injection.
2. Colluding oracle inflation attempt.
3. Contradictory anchor trend injection.
4. Replay of freeze/unfreeze sequence under degraded context.

## Non-Goals (Wave 1)

- No redesign of visible/shadow oracle scoring
- No changes to promotion formula thresholds beyond anchor-health gate
- No new economic slashing mechanisms
- No governance authority reshaping

## Rollout

1. Implement independent anchor-health pipeline.
2. Insert hard gate at top of promotion path.
3. Implement freeze semantics and artifact emission.
4. Add pre-freeze warnings and monitoring escalation.
5. Run mandatory simulated freeze drill (release blocker).
6. Roll out behind feature flag, then enforce globally.

