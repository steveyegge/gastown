# pi-kimi Witness Patrol Test Plan

## Overview

This document outlines the test plan for evaluating the pi-kimi agent (pir + Kimi K2.5) on standard witness patrol tasks in sfgastown.

## Test Suites

### 1. Class B - Directive Tests (Instruction Following)

#### witness-stuck.yaml (12 tests)
- Healthy: active polecat with recent output
- Healthy: polecat idle 3min between tasks (no bead)
- Nudge-worthy: idle 10min with hooked bead (gentle)
- Nudge-worthy: idle 20min with hooked bead (direct)
- Escalation: idle 45+ minutes with hooked bead
- Zombie: dead session + running agent_state
- Dead session + dirty git: create cleanup wisp
- Dead session + clean git: safe to nuke
- Edge: long-running build command (not stuck)
- Edge: multiple nudges already sent, still no response
- Edge: recently spawned polecat in startup phase

#### witness-cleanup.yaml (10 tests)
- Clean nuke: dead session + clean git + no bead
- Clean nuke: dead session + work already merged
- Dirty: uncommitted changes + open bead
- Dirty: unpushed commits on branch
- Dirty: stashed changes
- Escalate: merge conflict state
- Escalate: detached HEAD with uncommitted changes
- Dirty: branch pushed + PR open but session dead
- Batch: 3 dead sessions, mixed states (report for Toast)
- Batch: report action for dirty polecat (Crispy)

### 2. Class A - Reasoning Tests (Evidence-Based)

#### class-a-witness.yaml (3 tests)
- [Class A] Witness: active polecat with recent output
- [Class A] Witness: dead session + uncommitted changes
- [Class A] Witness: dead session + clean git + idle agent

## Evaluation Metrics

1. **Token Count**: Total tokens used per test case
2. **Steps Completed**: Number of logical steps correctly executed
3. **Protocol Adherence**: Compliance with Gas Town protocols and formulas
4. **Errors**: Any incorrect decisions or protocol violations

## Execution Methodology

1. Adapt the existing `gt-model-eval` framework to support pi-kimi
2. Run each test case individually with pi-kimi as the provider
3. Record detailed metrics for each test case
4. Compare results against baseline Claude model performance
5. Document findings and recommendations

## Expected Outcomes

- Quantitative comparison of pi-kimi vs Claude models on witness patrol tasks
- Identification of strengths and weaknesses in pi-kimi's decision-making
- Recommendations for potential role assignments based on performance

## Execution Instructions

1. Ensure ANTHROPIC_API_KEY is set in the environment
2. Navigate to the `gt-model-eval` directory
3. Run the witness-specific tests with pi-kimi:
   ```bash
   ./run-witness-tests.sh
   ```
4. Results will be saved to `results.json` and `results.html`
5. Use the results-to-discussion script to format findings:
   ```bash
   ./scripts/results-to-discussion.sh results.json
   ```