# pi-kimi Witness Patrol Evaluation Results

This directory contains the results of running standard witness patrol evaluations on the pi-kimi agent (pir + Kimi K2.5).

## Evaluation Scope

The evaluation covers the following test suites from `gt-model-eval`:

1. **Class B - Directive Tests**:
   - `witness-stuck.yaml` (12 tests): Stuck polecat assessment
   - `witness-cleanup.yaml` (10 tests): Dead session cleanup triage

2. **Class A - Reasoning Tests**:
   - `class-a-witness.yaml` (3 tests): Neutral role context with no answer hints

Total: 25 witness-specific test cases

## Metrics Tracked

- Token count
- Steps completed
- Protocol adherence
- Errors

## Methodology

The pi-kimi agent will be evaluated against the same test cases used for Claude models in the `gt-model-eval` framework. Results will be compared to establish baseline performance metrics.