# Fresh Agent Per Step for Workflow Formulas

## Context

Workflow formulas (like `shiny`: design → implement → review → test → submit) run all steps in a single polecat. By the implement phase, 70%+ of context is consumed. If the agent exhausts context and a new one is re-slung, it re-explores the same codebase from scratch and hits the same wall.

**Goal:** Each sequential step gets a fresh polecat with a clean context window. Artifacts (design docs, implementation logs) travel via git — the next polecat's worktree is branched from the previous polecat's branch, so all committed files are already on disk.

Convoys already solve this for parallel legs. This is only for sequential workflow steps.

## Design Summary

```
                        CURRENT                              PROPOSED (distributed)
                        ─────────                            ────────────────────────
gt mol step done        Close step                           Close step
        │               Find next ready                      Find next ready
        │               Pin to SAME agent                    Record branch in bead
        │               Respawn pane                         gt done --exit STEP_COMPLETE
        │               (same context window)                Polecat exits
        ▼                                                          │
                                                             Witness detects POLECAT_DONE
                                                                   │
                                                             Sends STEP_READY to Mayor
                                                                   │
                                                             Mayor slings next step to
                                                             FRESH polecat with
                                                             --base-branch <prev-branch>
                                                                   │
                                                             New polecat starts with
                                                             clean context + all files
```

Guarded by `execution = "distributed"` on the formula. Existing formulas default to `"local"` (current behavior, zero risk).

## Changes

### 1. Formula Schema (`internal/formula/types.go`)

Add `Execution` field to `Formula` and `Output` field to `Step`:

```go
// On Formula struct:
Execution string `toml:"execution"` // "local" (default) or "distributed"

// On Step struct:
Output string `toml:"output"` // Artifact path this step produces (e.g., "design.md")

// Helper:
func (f *Formula) IsDistributed() bool {
    return f.Type == TypeWorkflow && f.Execution == "distributed"
}

// Derive inputs from the needs chain:
func (f *Formula) GetStepInputs(stepID string) []string { ... }
```

Validate in parser: reject unknown execution modes.

### 2. New Exit Type (`internal/cmd/done.go`)

Add `ExitStepComplete = "STEP_COMPLETE"` alongside COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE.

STEP_COMPLETE behavior:
- Push branch to remote (so next polecat can base on it)
- Do NOT create MR (work is incomplete)
- Send POLECAT_DONE with `Exit: STEP_COMPLETE` to witness
- Kill session (standard done path)

Extract push logic from the COMPLETED path into a reusable `pushBranchToOrigin()` helper, called by both COMPLETED and STEP_COMPLETE.

### 3. Distributed Execution Branch (`internal/cmd/molecule_step.go`)

Core change in `runMoleculeStepDone()` (line 154 switch):

```go
case "continue":
    if isDistributed {
        return handleDistributedStepDone(...)
    }
    return handleStepContinue(...)  // existing path unchanged
```

New `handleDistributedStepDone()`:
1. Get current git branch name
2. Write handoff metadata to molecule bead: `step_handoff_next: <id>` and `step_handoff_branch: <branch>`
3. Call `gt done --status STEP_COMPLETE`

New `isDistributedMolecule()`: reads molecule bead description for `execution: distributed` (follows existing key-value metadata pattern used by AttachmentFields, MRFields, etc.).

### 4. Store Execution Mode at Instantiation (`internal/cmd/sling_helpers.go`)

In `InstantiateFormulaOnBead()`, after creating the wisp, if `formula.IsDistributed()`, append `execution: distributed` to the wisp root bead's description. This is how `isDistributedMolecule()` detects it at runtime.

### 5. Witness Handles STEP_COMPLETE (`internal/witness/handlers.go`)

In `HandlePolecatDone()`, add a branch for `payload.Exit == "STEP_COMPLETE"`:

1. Read `step_handoff_next` and `step_handoff_branch` from the molecule bead
2. Send `STEP_READY <next-step-id>` mail to `mayor/` with:
   - NextStep, Molecule, Branch, Rig, PreviousPolecat
   - Priority: High
3. Auto-nuke the completed polecat if clean (standard cleanup)

Follows the exact pattern of MERGE_READY → refinery.

Add `PatternStepReady` to `protocol.go` for message classification.

### 6. Mayor Dispatches Next Step (template awareness only)

No Go code changes. The Mayor is a Claude agent that reads its inbox. STEP_READY is a new mail type following existing patterns (MERGE_READY, RECOVERED_BEAD). The Mayor processes it by running:

```bash
gt sling <next-step-bead-id> <rig> --base-branch <branch>
```

`--base-branch` already exists on `gt sling`. `AddWithOptions` in polecat manager already supports `BaseBranch`. The new polecat's worktree is created from the previous branch via `git worktree add -b <new> <path> <startPoint>`.

Add STEP_READY to the Mayor's mail types table in `internal/templates/roles/mayor.md.tmpl`.

### 7. Prime Surfaces Input Files (`internal/cmd/prime_molecule.go`)

In `showMoleculeExecutionPrompt()`, after displaying the step description, list input artifacts from previous steps. The step bead's description includes `step_inputs: design.md,...` (written during instantiation from the formula's `output` fields and `needs` chain).

### 8. Update Shiny Formula (`internal/formula/formulas/shiny.formula.toml`)

```toml
execution = "distributed"

[[steps]]
id = "design"
title = "Design {{feature}}"
output = "design.md"
description = "Think carefully about architecture..."

[[steps]]
id = "implement"
title = "Implement {{feature}}"
needs = ["design"]
description = "Read design.md for the design plan. Write the code..."

[[steps]]
id = "review"
needs = ["implement"]
# ...
```

Also update the embedded copy at `.beads/formulas/shiny.formula.toml`.

## Implementation Order

| Phase | Files | Risk | Notes |
|-------|-------|------|-------|
| 1 | `types.go` | Low | Additive schema, backward-compatible |
| 2 | `done.go` | Low | New exit type, additive |
| 3 | `molecule_step.go` | Medium | Core branch, guarded by `isDistributed` |
| 4 | `sling_helpers.go` | Low | Metadata at instantiation |
| 5 | `handlers.go`, `protocol.go` | Medium | Witness → Mayor notification |
| 6 | `mayor.md.tmpl` | Low | Template docs only |
| 7 | `prime_molecule.go` | Low | Surface input file hints |
| 8 | `shiny.formula.toml` (x2) | Low | Formula opt-in |

Phases 1-2 are independently testable. Phase 3 is the behavioral core but guarded. Phases 4-5 complete the roundtrip.

## Verification

1. **Unit**: Add test for `IsDistributed()`, `GetStepInputs()`, `isDistributedMolecule()`
2. **Integration**: Create a test formula with `execution = "distributed"` and 2 steps. Sling to a rig. Verify:
   - First polecat runs design step, commits output, exits with STEP_COMPLETE
   - Witness sends STEP_READY to mayor
   - Mayor slings next step with `--base-branch`
   - Second polecat's worktree contains the design output file
3. **Backward compat**: Existing shiny formula (without `execution` field) still self-advances
4. **`go build ./...`** and **`go test ./...`** pass
