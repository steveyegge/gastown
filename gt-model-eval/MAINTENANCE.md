# Maintenance Guide

This eval framework is a **snapshot** of Gas Town patrol protocols. When patrol formulas, role definitions, or infrastructure change, these tests must be updated to stay aligned.

## What to Update When

### Action verbs change

Each role has hardcoded `allowed_actions` in every test case. If an action is renamed, added, or removed in patrol formulas, update the corresponding test files.

| Role | Actions | Test Files |
|------|---------|------------|
| Deacon (zombie-scan) | `file-warrant`, `no-op`, `nudge`, `log-and-watch`, `escalate-to-mayor`, `create-cleanup-wisp` | `deacon-zombie.yaml`, `class-a-deacon.yaml` |
| Deacon (plugin-run) | `execute-plugin`, `skip` | `deacon-plugin-gate.yaml` |
| Deacon (dog-health) | `no-op`, `log-and-watch`, `file-warrant`, `force-clear`, `spawn-dog`, `retire-dog` | `deacon-dog-health.yaml`, `class-a-deacon.yaml` |
| Witness | `no-op`, `nudge`, `escalate`, `nuke`, `mark-zombie`, `create-cleanup-wisp` | `witness-stuck.yaml`, `witness-cleanup.yaml`, `class-a-witness.yaml` |
| Refinery | `reject-mr`, `file-bead-and-proceed`, `retry`, `skip-mr`, `investigate` | `refinery-triage.yaml`, `refinery-conflict.yaml`, `class-a-refinery.yaml` |
| Dog | `reset`, `reassign`, `recover`, `escalate`, `burn` | `dog-orphan.yaml`, `class-a-dog.yaml` |

Each test case repeats the full `allowed_actions` array in `vars`. Search for the old action name across all YAML files:

```bash
grep -r '"old-action"' tests/
```

### Formula steps change

Test cases reference `formula_step` values (e.g., `zombie-scan`, `plugin-run`, `survey-workers`). If a formula step is renamed in patrol code, update the matching test files:

```bash
grep -r 'formula_step:' tests/
```

### Bead metadata labels change

Shell output in test cases contains bead JSON with labels like `agent_state:running`, `agent_state:idle`. If these label names change in `bd` or Gas Town agent code, update the simulated shell output in affected tests.

### Infrastructure paths change

Test shell output hardcodes these paths and naming conventions:

| Pattern | Example | Used In |
|---------|---------|---------|
| Polecat worktree | `git -C /town/gastown/polecats/<name>` | deacon, witness, dog tests |
| Tmux session | `tmux has-session -t bd-polecat-<name>` | deacon, witness tests |
| Bead commands | `bd show agent-<name> --json` | all role tests |
| Mail commands | `gt mail list --to polecat-<name>` | witness tests |

If directory structure, tmux naming, or CLI interfaces change, search and update:

```bash
grep -r '/town/gastown/polecats/' tests/
grep -r 'bd-polecat-' tests/
grep -r 'bd show' tests/
grep -r 'gt mail' tests/
```

### Decision thresholds shift

Some test descriptions encode timing expectations (e.g., "10 minutes idle triggers nudge", "45 minutes triggers escalate"). These are embedded in test case descriptions and `context` fields, not in a config file. If patrol formulas adjust timing thresholds, review the test scenarios to ensure they still test the right behavior.

### Roles are added or renamed

- Add new test files for the role following the existing pattern
- Add Class A (neutral context, evidence-only) and Class B (directive context) variants
- Register new files in `promptfooconfig.yaml` under the `tests:` section

### Response format changes

The system prompt (`prompts/patrol-decision.txt`) defines required fields (`action`, `reason`) and optional fields (`target`, `urgency`, `preserve`). If new required fields are added, update `defaultTest.assert` in `promptfooconfig.yaml` to validate them.

### Model versions change

Provider IDs in `promptfooconfig.yaml` reference specific model versions. When Anthropic releases new model versions, update the `providers` section. The `defaultTest.options.provider` (grading model) should always be the strongest available model.

## Class A vs Class B

**Class B** tests include directive `role_context` that hints at expected behavior. These validate instruction-following.

**Class A** tests use neutral `role_context` with no answer hints. These measure reasoning from evidence alone and are the primary signal for downgrade decisions.

When updating tests, maintain this distinction: Class A must never leak the expected answer into the role context.

## Running After Updates

```bash
# Quick validation (single run)
npx promptfoo eval

# Full comparison (3x for consistency)
npx promptfoo eval --repeat 3

# View results
npx promptfoo view
```

## Action Vocabulary vs CLI Verbs

Eval action names are **abstractions** of the actual CLI commands. This is intentional — the eval tests decision quality, not CLI syntax knowledge. When interpreting results, use this mapping:

| Eval Action | Actual CLI Command | Context |
|-------------|-------------------|---------|
| `spawn-dog` | `gt dog add` | Deacon dog pool maintenance |
| `retire-dog` | `gt dog remove` | Deacon dog pool maintenance |
| `force-clear` | `gt dog clear --force` | Deacon dog health check |
| `file-warrant` | `bd create --type=warrant ...` | Deacon zombie detection |
| `create-cleanup-wisp` | `bd create --type=wisp ...` | Deacon/witness cleanup |

## Future Improvements

- Extract `allowed_actions` per role into a shared config to avoid repetition across test cases
- Add protocol version tracking so staleness is detectable in CI
- Auto-generate test skeletons from patrol formula definitions
