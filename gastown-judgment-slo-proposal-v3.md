# Proposal: The Guardian — Judgment Quality Measurement for the Internal Merge Pipeline

## Summary

Gas Town already has sophisticated code review automation for **external PRs** — the PR Sheriff triages community contributions, the `/adopt-pr` workflow runs dual-model reviews (Claude + Codex) across multiple iterations, and the Rule of Five formula provides multi-pass quality refinement. These work. Julian Knutsen merged my PR #1583 through 5 review passes with zero blockers.

But none of this applies to the **internal merge pipeline** — the continuous flow of polecat-to-Refinery merges that constitute the vast majority of code changes. The Refinery gates on tests and conflicts. Nobody reviews whether the code is actually *correct*.

This proposal introduces the **Guardian** — a new agent persona that brings PR Sheriff-level quality review to internal merges, closing the gap between Gas Town's external review rigor and its internal quality blindspot.

## The Gap in Gas Town's Agent Roster

Gas Town has clearly defined roles, each watching for a specific class of failure:

| Agent | What It Judges | What It Misses |
|---|---|---|
| Mayor | What work to do and who does it | Whether the work was done *well* |
| Deacon | Is the agent alive and responsive | Agent is alive but producing bad work |
| Witness | Did the agent finish the work | Finished work that's wrong |
| Refinery | Does the code merge and pass CI | Code merges clean but is logically wrong |
| PR Sheriff | External PR quality (triage + review) | Only runs on GitHub PRs, not internal merges |
| **Guardian** | **Is the merged code actually good** | *(This is what's missing)* |

The PR Sheriff proves the review pattern works. The Guardian extends it inward.

### Why the Internal Pipeline Matters More

When Peter Steinberger describes evaluating 6,600 commits from 10 parallel agents — that's the internal pipeline. At GasTown scale (20-50 polecats), the internal merge volume dwarfs external PRs by orders of magnitude. And unlike external PRs, which sit in a queue until a human looks at them, internal merges flow automatically through the Refinery to main.

The failure mode: a polecat completes 15 beads in a session. The Witness marks them done. The Refinery merges 10, rejects 3 for conflicts, 2 for test failures. Three hours later the human discovers merged code is architecturally wrong. Meanwhile, 5 other polecats have built on top of it. The cost of late detection compounds — every downstream merge is contaminated by the original bad decision.

Gas Town flagged zero quality issues. The agent was "healthy" by every metric we track.

## Prior Art: Gas Town's Existing Review Infrastructure

This proposal doesn't invent a new pattern — it extends one that's already proven effective.

### PR Sheriff (`bd-pr-sheriff`)

Steve Yegge's PR Sheriff is a permanent crew bead that activates on every session startup. It triages open PRs into easy wins vs. human-review-required, then slings easy wins to other crew members. The Sheriff on the GasTown rig — `gastown/crew/max` — posts structured reviews (as seen on PR #1146) with severity ratings, specific issue identification, and actionable recommendations.

What the Sheriff proves: **AI-driven code review at scale is already operational in Gas Town**. It just doesn't run on internal merges.

### `/adopt-pr` Workflow

Julian Knutsen's adoption workflow goes deeper than triage:

1. **Dual-model review** — Claude and Codex both review the PR, cross-validating findings
2. **Iterative passes** — up to 5 review iterations, with fixes applied between passes
3. **Maintainer fixup** — issues identified by review are fixed directly on the contributor's branch
4. **Clean gate** — merge only after "Pass N review clean — no blockers, no major issues"

On PR #1583, this process found 5 issues (session API mismatch, concurrency gaps, attach-to-selected bug, error handling, selection stability) and resolved all of them before merge.

What `/adopt-pr` proves: **multi-pass, multi-model quality assessment produces measurably better merge outcomes**. The Guardian brings this to every internal merge.

### Rule of Five Formula

The `rule-of-five.formula.toml` defines four focused review passes from Jeffrey Emanuel's methodology:

1. **CORRECTNESS** — Fix errors, bugs, mistakes
2. **CLARITY** — Can someone else understand this?
3. **EDGE CASES** — What could go wrong? What's missing?
4. **EXCELLENCE** — Make it shine

What Rule of Five proves: **structured multi-pass review is already a first-class Gas Town concept**, implemented as a formula that can be applied to any work.

## Design: The Guardian

In Mad Max lore, the Guardian of Gas Town was the original protector of the refinery — the one who kept the output clean and the operation running before it fell to chaos. In GasTown the software project, the Guardian protects the codebase from the chaos of unchecked agent merges.

### Where It Sits

```
Polecat completes → gt done → Witness verifies → MERGE_READY
                                                       ↓
                                              Guardian reviews diff
                                              Scores quality/risk
                                              Records judgment telemetry
                                                       ↓
                                              QUALITY_ASSESSED → Refinery merges
                                              (or QUALITY_HOLD → human review queue)
```

The Guardian sits between the Witness and the Refinery. The Witness says "work is done and git state is clean." The Guardian says "the work is good enough to merge." The Refinery handles the mechanical merge.

### What It Does

**1. Reviews every polecat diff before merge.** Reads the proposed changes with full codebase context — the same way the PR Sheriff reviews external PRs. It can evaluate logic errors, architectural violations, security issues, and subtle bugs that pass CI.

**2. Scores each merge with a structured quality assessment.** Not binary accept/reject, but a multi-dimensional evaluation:

```go
type GuardianResult struct {
    PolecatID          string    `json:"polecat_id"`
    BeadID             string    `json:"bead_id"`
    Rig                string    `json:"rig"`

    // Quality dimensions (0.0-1.0)
    CorrectnessScore   float64   `json:"correctness_score"`
    ClarityScore       float64   `json:"clarity_score"`
    RiskLevel          string    `json:"risk_level"` // low, medium, high, critical
    OverallScore       float64   `json:"overall_score"`

    // Findings
    Issues             []GuardianIssue `json:"issues,omitempty"`
    Recommendation     string    `json:"recommendation"` // merge, hold, reject

    // Meta
    ModelUsed          string    `json:"model_used"`
    ReviewDurationMs   int64     `json:"review_duration_ms"`
    Timestamp          time.Time `json:"timestamp"`
}
```

**3. Tracks per-polecat quality patterns over time.** If polecat X's reviews consistently flag test coverage gaps, the Guardian knows X is weak on testing. This feeds directly into judgment SLOs — not as a noisy proxy signal, but as a direct quality measurement.

**4. Gates the merge pipeline when quality drops.** In permissive mode (Phase 1), it scores but doesn't block. In strict mode (Phase 2), low-confidence merges go to a human review queue instead of merging automatically.

**5. Generates the modification ratio natively.** When the Guardian flags "this auth logic is wrong — the token should be validated before the session check," and the human subsequently fixes exactly that, the Guardian already has the context to link the correction to its original assessment. No file overlap heuristics needed.

### Cost Management

Running AI review on every merge adds latency and token spend. Gas Town already supports cost tiers (`standard`, `economy`, `budget`) with per-role model selection. The Guardian is added to `TierManagedRoles` in `internal/config/cost_tier.go`, so model assignment follows the existing tier system:

| Tier | Guardian Model | Polecat Model | Cross-model? |
|------|---------------|---------------|-------------|
| Standard | opus (default) | opus | No — but highest quality |
| Economy | sonnet | opus | Yes — natural cross-model |
| Budget | haiku | sonnet | Yes — natural cross-model |

Economy tier (the proposed default) naturally produces cross-model review since polecats default to opus — giving dual-model validation without special logic.

**Review depth is risk-tiered:**

- **Single pass** (default): One structured review covering correctness, clarity, and risk assessment. Target: <30s latency.
- **Multi-pass** (escalated): 2-pass review — correctness pass then edge-cases pass. Target: <90s. Triggered by risk signals:
  - Diff touches security-sensitive paths (auth, crypto, permissions)
  - Diff > 500 lines changed
  - Diff modifies core infrastructure (refinery, deacon, beads)
  - Diff has high cyclomatic complexity delta
- **Deep review** (manual): Full 4-pass Rule of Five via `gt guardian review --deep`.

Additional optimizations:
- Skip review for trivial changes (config-only, docs, formatting)
- Batch reviews during high-throughput periods

The cost of the Guardian is trivial compared to the cost of a human spending an evening reverting a cascade of bad merges.

## Implementation

### Phase 1: Measurement (Read-Only)

Phase 1 ships the Guardian as a passive observer. It reviews diffs and records quality scores, but doesn't gate merges.

#### A. New OTel instruments (`internal/telemetry/recorder.go`)

Extends the existing `recorderInstruments` struct:

```go
// Judgment quality instruments
mergeOutcomeTotal      metric.Int64Counter     // gastown.merge.outcome.total
guardianScoreHist         metric.Float64Histogram // gastown.guardian.score
guardianDurationHist      metric.Float64Histogram // gastown.guardian.duration_ms
judgmentRejectionRate  metric.Float64Gauge     // gastown.judgment.rejection_rate (per-polecat)
```

New recorder functions following the pattern of `RecordDone()`:

```go
// RecordMergeOutcome records the mechanical merge result.
func RecordMergeOutcome(ctx context.Context, worker, rig, outcome, sourceIssue string, retryCount int) {
    initInstruments()
    inst.mergeOutcomeTotal.Add(ctx, 1,
        metric.WithAttributes(
            attribute.String("worker", worker),
            attribute.String("rig", rig),
            attribute.String("outcome", outcome),
            attribute.String("source_issue", sourceIssue),
            attribute.Int("retry_count", retryCount),
        ),
    )
}

// RecordGuardianResult records the Guardian's quality assessment.
func RecordGuardianResult(ctx context.Context, worker, rig, recommendation string, score float64, durationMs int64) {
    initInstruments()
    inst.guardianScoreHist.Record(ctx, score,
        metric.WithAttributes(
            attribute.String("worker", worker),
            attribute.String("rig", rig),
            attribute.String("recommendation", recommendation),
        ),
    )
    inst.guardianDurationHist.Record(ctx, float64(durationMs))
}
```

#### B. Guardian agent (`internal/guardian/guardian.go`)

New package following Gas Town's existing agent patterns:

```go
package guardian

// Guardian reviews polecat diffs for quality before merge.
type Guardian struct {
    config    *GuardianConfig
    recorder  *GuardianRecorder
    state     *GuardianState
}

// Review evaluates a polecat's diff and returns a quality assessment.
func (a *Guardian) Review(ctx context.Context, diff *MergeDiff) (*GuardianResult, error) {
    // 1. Classify risk level from diff stats
    risk := classifyRisk(diff)

    // 2. Select model via cost-tier (economy default → sonnet)
    model := config.ResolveRoleAgentConfig("guardian", risk)

    // 3. Run structured review (maps to Rule of Five dimensions)
    result, err := a.runReview(ctx, model, diff, risk)
    if err != nil {
        return nil, err
    }

    // 4. Record telemetry
    telemetry.RecordGuardianResult(ctx, diff.Worker, diff.Rig,
        result.Recommendation, result.OverallScore, result.ReviewDurationMs)

    // 5. Update per-polecat state
    a.state.RecordResult(diff.Worker, result)

    return result, nil
}
```

#### C. Merge pipeline integration (`internal/refinery/engineer.go`)

In the existing MR processing flow, add the Guardian review step before merge:

```go
// After Witness verification, before merge
if guardian != nil {
    result, err := guardian.Review(ctx, buildDiff(mr))
    if err != nil {
        log.Warn("guardian review failed, proceeding with merge", "err", err)
        // Fail open — don't block merges if the Guardian is down
    } else {
        telemetry.RecordMergeOutcome(ctx, mr.Worker, mr.Rig,
            classifyMergeOutcome(result, mr), mr.SourceIssue, mr.RetryCount)

        if result.Recommendation == "hold" && config.Judgment.StrictMode {
            // Phase 2: Route to human review queue
            routeToHumanQueue(ctx, mr, result)
            continue
        }
    }
}

// Proceed with normal merge
```

#### D. `gt judgment` command (`internal/cmd/judgment.go`)

```
$ gt judgment status

Polecat Quality Report (window: 24h, source: Guardian)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Polecat          MRs  Score  Reject  1st-Pass  Risk    Status
                               Rate    Rate      (avg)
  ──────────────   ───  ─────  ──────  ────────  ──────  ──────
  ace-mjxwfy7e     12   0.91   0.00    1.00      low     ✅ OK
  bolt-k3x9p2       8   0.72   0.12    0.75      medium  ⚠️  WARN
  coil-r7m2n1      15   0.43   0.33    0.47      high    🔴 BREACH

Breach Detail:
  coil-r7m2n1:  guardian score 0.43 (threshold: 0.60)
                3 issues flagged: auth logic, missing error handling, test gap
                rejection rate 0.33 exceeds threshold (0.20)

$ gt judgment history --polecat coil-r7m2n1

  Bead         Score  Issues  Recommendation  Merge Outcome
  ──────────   ─────   ──────  ──────────────  ─────────────
  gt-abc12     0.88    0       merge           merged
  gt-def34     0.45    2       hold            tests_fail
  gt-ghi56     0.31    3       reject          conflict
  gt-jkl78     0.52    1       merge           merged ← human corrected later
  gt-mno90     0.29    4       reject          conflict

  Calibration: 2/3 "hold" recommendations later confirmed by merge failure.
               1/1 "merge" with low score was corrected by human.
```

Note the calibration line — this is the judgment SLO signal. When the Guardian says "hold" and the merge later fails, the Guardian was right. When the Guardian says "merge" and a human later corrects it, the Guardian was wrong. Over time, this measures the Guardian's own judgment quality.

#### E. `gt feed` integration

Judgment status appears in the problems view alongside stuck/zombie agents:

```
Problems View (press 'p')
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  🔴 coil-r7m2n1  JUDGMENT  guardian score 0.43, rejection rate 0.33
  🟡 bolt-k3x9p2  JUDGMENT  approaching threshold (0.72 / 0.60)
  🔴 dash-p4q8w2  STUCK     idle 8m, prompt waiting
```

### Phase 2: Active Quality Gating

#### F. Judgment config (`internal/config/types.go`)

```go
type JudgmentConfig struct {
    // Enabled activates the Guardian in the merge pipeline.
    Enabled    bool   `json:"enabled"`

    // StrictMode gates merges on Guardian recommendation.
    // false = score and record only. true = hold/reject blocks merge.
    StrictMode bool   `json:"strict_mode"`

    // Window is the rolling measurement window for per-polecat aggregates.
    Window     string `json:"window,omitempty"` // Default: "24h"

    // CostTier controls which model the Guardian uses.
    // Maps to Gas Town's existing standard/economy/budget tiers.
    CostTier   string `json:"cost_tier,omitempty"` // Default: "economy"

    // Thresholds define quality levels.
    Thresholds *JudgmentThresholds `json:"thresholds,omitempty"`

    // BreachPolicy defines automated responses at each severity level.
    BreachPolicy *JudgmentBreachPolicy `json:"breach_policy,omitempty"`
}
```

Town-level `JudgmentConfig` lives on `TownSettings`. Rig-level overrides follow the existing `RoleAgents` pattern — rig settings with a `Judgment` field take precedence over town defaults.

Default config:

```json
{
  "judgment": {
    "enabled": false,
    "strict_mode": false,
    "window": "24h",
    "cost_tier": "economy",
    "thresholds": {
      "warn":     { "guardian_score": 0.60, "rejection_rate": 0.15 },
      "breach":   { "guardian_score": 0.45, "rejection_rate": 0.25 },
      "critical": { "guardian_score": 0.30, "rejection_rate": 0.40 }
    },
    "breach_policy": {
      "warn": [
        { "action": "nudge", "message": "Quality metrics declining — review recent work carefully." }
      ],
      "breach": [
        { "action": "escalate", "message": "Polecat {polecat} judgment quality breach." }
      ],
      "critical": [
        { "action": "park", "message": "Polecat {polecat} parked — judgment quality critical." }
      ]
    }
  }
}
```

#### G. Deacon judgment patrol (`internal/deacon/judgment_patrol.go`)

Following the exact structure of `stuck.go`:

```go
// JudgmentPatrol monitors per-polecat quality aggregates and
// executes breach policy when thresholds are exceeded.
// Runs on each Deacon heartbeat cycle.
func (d *Deacon) JudgmentPatrol(ctx context.Context) []JudgmentCheckResult {
    state := d.loadJudgmentState()
    config := d.config.Judgment

    var results []JudgmentCheckResult
    for _, pj := range state.Polecats {
        pj.Refresh(config.Window)
        severity := pj.Evaluate(config.Thresholds)

        if severity != "ok" {
            results = append(results, JudgmentCheckResult{
                PolecatID:      pj.PolecatID,
                GuardianScoreAvg:  pj.AvgGuardianScore,
                RejectionRate:  pj.RejectionRate,
                Severity:       severity,
            })
            d.executeBreachPolicy(ctx, pj, severity, config.BreachPolicy)
        }
    }
    return results
}
```

### Phase 2b: Guardian Self-Calibration (Judgment SLOs on the Guardian)

This is where it gets interesting — you can run judgment SLOs on the Guardian itself:

```go
type GuardianCalibration struct {
    // When Guardian said "merge" and human later corrected → Guardian missed it
    FalseAccepts     int     `json:"false_accepts"`

    // When Guardian said "hold" and merge later failed → Guardian was right
    TrueHolds        int     `json:"true_holds"`

    // When Guardian said "hold" and merge would have succeeded → Guardian was too strict
    FalseHolds       int     `json:"false_holds"`

    // Overall calibration score
    Precision        float64 `json:"precision"`  // true_holds / (true_holds + false_holds)
    Recall           float64 `json:"recall"`     // true_holds / (true_holds + false_accepts)
}
```

This directly connects to the OpenSRM judgment SLO spec: the Guardian is an AI system making approve/reject decisions on code quality. Its judgment can be measured through the same reversal rate, high-confidence failure, and calibration metrics defined in the spec. It's judgment SLOs all the way down.

## What This Does NOT Do

- **Does not replace the PR Sheriff.** The Sheriff handles external PR triage. The Guardian handles internal merge quality. They're complementary.
- **Does not add new observability systems.** Uses existing OTel SDK, existing events system, existing config patterns.
- **Does not auto-reject in Phase 1.** Phase 1 is read-only. The Guardian scores and records, but merges proceed normally.
- **Does not replace the Witness.** The Witness monitors polecat progress. The Guardian evaluates output quality. Different concerns.
- **Does not require VictoriaMetrics.** State is local (`judgment-state.json`), same as existing Deacon state. VM is optional for dashboarding.

## Migration Path

| Phase | What Ships | Risk | Config |
|---|---|---|---|
| 1a | `RecordMergeOutcome()` + `RecordGuardianResult()` in telemetry | Zero — adds OTel instruments | None needed |
| 1b | Guardian agent (review-only, fail-open) | Low — scores but doesn't gate | `judgment.enabled: true` |
| 1c | `gt judgment status` / `history` commands | Zero — read-only diagnostic | None needed |
| 1d | Judgment column in `gt feed` problems view | Zero — display only | None needed |
| 2a | `JudgmentConfig` with `strict_mode` | Low — opt-in gating | `judgment.strict_mode: true` |
| 2b | Deacon judgment patrol (breach policy) | Medium — changes agent behavior | Explicit policy config |
| 2c | Guardian self-calibration (judgment SLOs on the Guardian) | Zero — measurement only | None needed |
| 2d | Convoy quality gates (aggregate score gating) | Medium — blocks convoy close | `judgment.convoy_gate_score: 0.70` |
| 2e | Calibration bootstrap (observation-only warmup) | Zero — delays threshold activation | Auto on first enable |

Phase 2 items (2a–2e) depend on Phase 1 measurement data. A tracking issue will be opened after Phase 1 lands.

## Files Changed

| File | Change | Lines (est.) |
|---|---|---|
| `internal/telemetry/recorder.go` | Add `RecordMergeOutcome()` + `RecordGuardianResult()` | +60 |
| `internal/refinery/engineer.go` | Call Guardian before merge, record outcome | +30 |
| `internal/guardian/guardian.go` | New: Guardian agent core | +300 |
| `internal/guardian/review.go` | New: diff review logic, model selection | +250 |
| `internal/guardian/state.go` | New: per-polecat quality state | +150 |
| `internal/events/events.go` | Add judgment event types | +10 |
| `internal/deacon/judgment_patrol.go` | New: judgment quality patrol | +250 |
| `internal/config/types.go` | Add `JudgmentConfig` to `TownSettings` | +80 |
| `internal/cmd/judgment.go` | New: `gt judgment` command suite | +350 |
| `internal/tui/problems.go` | Add judgment health to problems view | +50 |
| Tests | Unit tests for all new code | +500 |
| `docs/guardian.md` | Guardian documentation | +250 |
| **Total** | | **~2,280** |

## Design Decisions

These questions were resolved during design review. Rationale documented here for future reference.

### 1. Guardian Model Selection → Cost-tier managed

The Guardian is added to `TierManagedRoles` in `internal/config/cost_tier.go`. Model assignment follows the existing tier system rather than special-case logic. Economy tier (the proposed default) naturally produces cross-model review — the Guardian runs on sonnet while polecats default to opus. This gives dual-model validation (proven effective by the `/adopt-pr` workflow) without any additional configuration.

### 2. Review Depth → Risk-tiered single pass

Default: single structured pass covering correctness + risk assessment (<30s). A risk classifier triggers multi-pass escalation (<90s) when diffs touch security-sensitive paths, exceed 500 lines, modify core infrastructure, or have high cyclomatic complexity delta. Full 4-pass Rule of Five reserved for manual `gt guardian review --deep`. This balances quality with merge pipeline latency.

### 3. Thresholds → Global defaults, rig overrides

Town-level `JudgmentConfig` on `TownSettings` sets defaults. Rigs override via their existing settings pattern (same as `RoleAgents` overrides). This handles the "complex codebase has higher conflict rate" concern without introducing a new override mechanism.

### 4. Convoy Quality Gates → Deferred to Phase 2

Gating convoy completion on aggregate Guardian scores requires baseline data to set meaningful thresholds. Cannot determine appropriate gate scores (e.g., 0.70) without Phase 1 measurement data. Deferred to Phase 2d.

### 5. Calibration Bootstrap → Deferred to Phase 2

An observation-only warmup period before thresholds activate requires Phase 1 data to determine appropriate duration and baseline metrics. Deferred to Phase 2e.

## Prior Art

- **Gas Town PR Sheriff** — Proof that AI-driven code review at scale is operationally viable in Gas Town. The Guardian extends this pattern from external PRs to internal merges.
- **Gas Town `/adopt-pr` workflow** — Proof that dual-model, multi-pass review produces measurably better outcomes. The Guardian's cross-model review follows this precedent.
- **Gas Town Rule of Five** — Structured multi-pass review is already a first-class formula concept. The Guardian's quality dimensions (correctness, clarity, risk, edge cases) map directly.
- **[OpenSRM](https://github.com/rsionnach/opensrm)** — Open specification for service reliability manifests, including judgment SLOs for AI decision quality. The Guardian's self-calibration metrics (false accept rate, precision, recall) implement the spec's reversal rate and high-confidence failure concepts.
- **[OTel GenAI Semantic Conventions](https://github.com/open-telemetry/semantic-conventions)** — Emerging standards for AI decision telemetry. The `gastown.guardian.*` and `gastown.judgment.*` metrics map to the `gen_ai.decision.*` conventions under discussion.
- **Peter Steinberger / OpenClaw** — Steinberger manually evaluates thousands of agent commits. The Guardian automates what he does by hand — measuring output quality at scale rather than tracing internal reasoning.

## Author

Rob — Senior SRE, creator of [OpenSRM](https://github.com/rsionnach/opensrm) and [NthLayer](https://github.com/rsionnach/nthlayer). Building observability and reliability-as-code for AI systems.

- GitHub: [@rsionnach](https://github.com/rsionnach)
- LinkedIn: [Rob's profile]
