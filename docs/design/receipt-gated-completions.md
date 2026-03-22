# Receipt-Gated Completions for Federated Tool-Call Verification

> **Date:** 2026-03-21
> **Author:** smledbetter
> **Status:** Proposal
> **Issue:** [steveyegge/gastown#2814](https://github.com/steveyegge/gastown/issues/2814)
> **Labels:** `area/wasteland`, `type/security`
> **Related:** [Receipt-Gated Pipelines](https://github.com/smledbetter/receipt-gated-pipelines) (study),
> `internal/wasteland/trust.go` (tier escalation), `internal/wasteland/spider.go` (collusion detection),
> model-aware-molecules (capability routing)

---

## 1. Problem Statement

When a rig submits a completion to the wanted board, there is no verification that
tool calls claimed in the completion actually happened. A rig running a cheap model
can submit a completion where the model skipped tool calls and fabricated results
from parametric knowledge — and the fabrication looks like success.

This is an NDI failure. The Wasteland assumes nondeterministic processes produce
useful outcomes through orchestration and oversight. But tool-call confabulation
breaks the feedback loop: stamps and trust tiers can only correct what they can
observe, and confabulated completions are indistinguishable from correct ones by
inspection. The oversight layer needs a ground-truth signal.

Three of eight models tested confabulate tool results at 25-70% rates. The
fabricated entries are often real facts drawn from training data, just never
verified through the tool — completeness checks pass, spot-checking passes. In
two-rig simulations, 82% of confabulated inputs propagated downstream and 73%
were elaborated upon by the receiving agent.

The stamps system catches bad completions *after* propagation. Receipt gates
catch them *before*.

---

## 2. Proposal

Add an optional, per-board receipt gate that verifies tool-call completions at
the wanted board acceptance boundary. The gate is deterministic (not a classifier)
and has zero false negatives by construction.

| Component | Location | Purpose |
|-----------|----------|---------|
| **ToolShim receipt signing** | Tool call dispatch layer (see §3.1) | Sign every tool call response with HMAC-SHA256 |
| **Board-level receipt gate** | Wasteland completion pipeline | Verify receipts at wanted board acceptance |
| **Stamps integration** | `internal/wasteland/` | Feed gate verdicts into rig reputation |

---

## 3. ToolShim Receipt Signing

### 3.1 How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│  Agent (polecat / crew / dog)                                   │
│                                                                 │
│  "check_vulnerability(log4j-core, 2.14.1)"                     │
│       │                                                         │
│       ▼                                                         │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  ToolShim                                                │   │
│  │                                                          │   │
│  │  1. Generate UUID4 call_id                               │   │
│  │  2. Forward call to real tool backend                    │   │
│  │  3. Hash input (SHA256) and output (SHA256)              │   │
│  │  4. Sign "{call_id}:{input_hash}:{output_hash}"         │   │
│  │     with per-rig HMAC-SHA256 key                         │   │
│  │  5. Append receipt to completion log                     │   │
│  │  6. Return tool output + _call_id to agent               │   │
│  │                                                          │   │
│  │  Integration point TBD — wherever tool calls are         │   │
│  │  dispatched to backends (proxy layer, agent provider,    │   │
│  │  or session manager). Maintainer to advise.              │   │
│  └──────────────────────────────────────────────────────────┘   │
│       │                                                         │
│       ▼                                                         │
│  Agent includes _call_id in final report                        │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 Receipt Structure

```json
{
  "call_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "tool_name": "check_vulnerability",
  "input_hash": "sha256:a1b2c3...",
  "output_hash": "sha256:d4e5f6...",
  "timestamp": "2026-03-21T14:30:00Z",
  "hmac": "sha256:9f8e7d..."
}
```

- `call_id`: Server-generated UUID4. The model cannot predict or forge this.
- `input_hash` / `output_hash`: SHA256 of canonical JSON input and output.
- Key management: Per-rig signing key, rotated via existing secrets infrastructure.

### 3.3 Receipt Prompt

The model receives a prompt requiring it to echo `_call_id` values and list
unchecked items in an `unverified` field:

```
You MUST include the _call_id for every package in your final report
as proof that you checked it. Any package listed without a valid
_call_id will be flagged as unverified.

Format: {"dependencies_checked": [{"package": "...", "call_id": "...",
"vulnerabilities": [...]}], "unverified": ["packages you could not check"]}
```

The echoed IDs give the gate verification anchors. The `unverified` field gives
models an honest escape hatch — admit incompleteness rather than fabricate.

### 3.4 Formula / Molecule Integration

The receipt log accumulates across all steps in a molecule execution. At
completion time, `gt wl done` submits the full log. The gate verifies against
the aggregate — it does not need to understand the molecule's DAG structure.

For model-aware molecules, the log captures which model produced which calls.
A molecule that routes a tool-heavy step to a high-confabulation model will
show confabulation in the verdict even if other steps are clean.

### 3.5 Overhead

Four sub-millisecond operations per tool call (UUID4, two SHA256s, one HMAC).
Negligible relative to tool call API latency. Receipt prompt adds ~100 tokens
per completion.

---

## 4. Board-Level Receipt Gate

### 4.1 Placement

The gate fires after `gt wl done` but before the completion is accepted into
the commons database — blocking confabulated content before any downstream
consumer sees it.

```
Rig completes work
    │
    ▼
gt wl done <id> --evidence <url> --receipts <log>
    │
    ▼
┌──────────────────────────────────────────────────┐
│  Receipt Gate (NEW)                              │
│                                                  │
│  For each claimed tool result in the completion: │
│    • Look up _call_id in receipt log             │
│    • Verify HMAC signature                       │
│    • Classify: VERIFIED / CONFABULATED / ...     │
│                                                  │
│  Aggregate into completion-level verdict         │
└──────────────────────────────────────────────────┘
    │
    ├── VERIFIED ────────► completion accepted, status = in_review
    ├── PARTIAL_CONFAB ──► accepted with warning tag
    ├── FULL_CONFAB ─────► rejected, rig notified
    ├── NO_RECEIPTS ─────► accepted as [UNVERIFIED]
    │
    ▼
Stamps system receives verdict
```

### 4.2 Board Opt-In

Not all wanted items involve tool calls. The gate activates per item via
`requires_tool_receipts`:

```bash
gt wl post \
  --title "Audit dependency vulnerabilities" \
  --type feature \
  --requires-tool-receipts \
  ...
```

When the flag is set, completions MUST include a receipt log. When unset
(default), completions are accepted as today — no behavioral change.

### 4.3 Verdict Taxonomy

Per-item verdicts:

| Verdict | Definition | Action |
|---------|-----------|--------|
| **VERIFIED** | Tool called AND model echoed correct `_call_id` | Accept |
| **UNVERIFIED** | Tool called but model did not echo `_call_id` | Accept with note |
| **CONFABULATED** | Model claims results for a call that never happened | Flag |
| **MISSING** | No call and no claim — coverage gap | Accept |
| **SELF_REPORTED_UNVERIFIED** | Model listed in `unverified` field honestly | Accept |
| **UNKNOWN** | Cannot classify (malformed output) | Accept with warning |

Completion-level verdicts (aggregated):

| Verdict | Condition | Action |
|---------|-----------|--------|
| **VERIFIED** | All items VERIFIED or SELF_REPORTED_UNVERIFIED | Accept |
| **PARTIAL_CONFAB** | Mix of VERIFIED and CONFABULATED | Accept with warning |
| **FULL_CONFAB** | All or nearly all items CONFABULATED | Reject |
| **NO_RECEIPTS** | No receipt log submitted | Accept as `[UNVERIFIED]` |

### 4.4 Backwards Compatibility and Degradation

Rigs without receipt signing don't attach receipts. Their completions get
`NO_RECEIPTS` — accepted but tagged `[UNVERIFIED]`. Upgraded rigs get stronger
trust signals; non-upgraded rigs still participate. No simultaneous upgrade
required.

If the gate encounters errors (malformed log, missing fields), it falls back to
`NO_RECEIPTS` rather than blocking. The gate should never be an availability
risk for the wanted board.

---

## 5. Stamps Integration

### 5.1 Gate Verdicts as Stamp Signals

| Gate Verdict | Stamp Effect |
|-------------|-------------|
| VERIFIED | Positive signal (verified tool use) |
| PARTIAL_CONFAB | Negative stamp: partial confabulation detected |
| FULL_CONFAB | Strong negative stamp: completion rejected |
| NO_RECEIPTS | Neutral (no signal, not penalized) |

### 5.2 Trust Tier Interaction

Confabulation stamps interact with existing `TierRequirements` in
`internal/wasteland/trust.go`:

- **Demotion**: N CONFAB stamps within a rolling window demotes a rig one tier.
  Threshold configurable per wasteland.
- **Promotion blocker**: CONFAB stamps reduce `MinAvgQuality`, making tier
  advancement harder.
- **Economic pressure**: Rigs running high-confabulation models accumulate
  penalties and either switch models or lose board eligibility. Detection is
  deterministic; consequence is economic.

The Spider Protocol (`spider.go`) can incorporate receipt data as a future
collusion signal — out of scope for this proposal.

---

## 6. Schema Changes

```sql
-- Wanted table
ALTER TABLE wanted ADD COLUMN requires_tool_receipts BOOLEAN DEFAULT FALSE;

-- Completions table
ALTER TABLE completions ADD COLUMN receipt_verdict VARCHAR(20) DEFAULT NULL;
ALTER TABLE completions ADD COLUMN receipt_summary JSON DEFAULT NULL;
```

`receipt_summary` example:

```json
{
  "total_items": 100,
  "verified": 85,
  "unverified": 5,
  "confabulated": 3,
  "missing": 5,
  "self_reported_unverified": 2,
  "unknown": 0
}
```

No stamps schema changes — confabulation verdicts go in the existing `valence`
JSON field alongside quality/reliability/creativity scores.

---

## 7. Security Properties

### What the gate guarantees

1. **No false negatives.** If a model claims results for a tool call that never
   happened, the gate catches it. The model cannot forge a UUID4 that the
   ToolShim never issued (~1/2^122 collision probability).

2. **Deterministic verification on actual results, not model reputation.**
   The gate is a lookup function, not a classifier. No thresholds, no training
   data, no drift. This matters because confabulation rates are unstable across
   runs (R1 varied 60% → 22% across batches) — you cannot reliably pre-screen
   by benchmarking. The gate fires on what the model actually did in *this*
   completion.

3. **Tamper-evident receipts.** HMAC signing means a rig cannot modify tool
   outputs after the fact without invalidating the receipt.

### What the gate does not guarantee

1. **Correctness.** Verifies a tool was called, not that it returned correct
   results.
2. **Completeness.** Does not enforce coverage — a model that honestly reports
   50/100 items gets VERIFIED with MISSING items.
3. **Compromised ToolShim protection.** Leaked signing keys allow forgery.
   Per-rig isolation and rotation mitigate this.

---

## 8. What Migrates to Wasteland

This proposal targets Gastown because Wasteland issues are disabled. The split:

| Component | Gastown | Wasteland |
|-----------|---------|-----------|
| ToolShim receipt signing | Yes (tool dispatch layer) | — |
| Receipt prompt templates | Yes (tool dispatch layer) | — |
| Board-level receipt gate | — | Yes |
| Schema changes (wanted, completions) | — | Yes |
| Stamps integration | — | Yes |
| `gt wl done --receipts` / `gt wl post --requires-tool-receipts` | — | Yes |

---

## 9. Open Questions

1. **Demotion threshold.** How many CONFAB stamps within what window triggers a
   tier demotion? Policy decision, not engineering.
2. **Receipt log storage.** Store in Dolt alongside the completion, or separate
   blob store referenced by hash?
3. **Mandatory long-term.** Per-board opt-in is the right start. Should the
   default flip once adoption reaches a threshold?

---

## 10. Implementation Status

- [ ] ToolShim receipt signing (integration point TBD)
- [ ] Receipt prompt injection for tool-heavy tasks
- [ ] `gt wl post --requires-tool-receipts` flag
- [ ] `gt wl done --receipts <log>` attachment
- [ ] Receipt gate in completion pipeline
- [ ] Verdict recording on completion records
- [ ] Stamps integration (CONFAB → negative stamp)
- [ ] Trust tier demotion on repeated confabulation

---

## Appendix: Research Summary

Design decisions are informed by a published study (686 trials, 9 models, 4
phases, $15.42). Full data and code:
[receipt-gated-pipelines](https://github.com/smledbetter/receipt-gated-pipelines).

- **Confabulation rates**: 3/8 models at 25-70%; 5/8 zero or near-zero.
  Problem is model-specific — hence opt-in per board.
- **Receipt prompt effect**: Mercury 80% → 13% confabulation; R1 bimodal
  collapse. Structured format sustains effort through visibility.
- **Propagation blocking**: 0% gated vs 82% ungated. Downstream agents
  elaborate on fabricated upstream data in 73% of cases.
