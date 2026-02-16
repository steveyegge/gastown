# gt-model-eval

Promptfoo-based model comparison framework for Gas Town patrol agents. Compare Claude Opus, Sonnet, and Haiku on patrol decision tasks to find where cheaper models match Opus quality.

## Why

Gas Town multi-agent setups burn through Opus budget on patrol agents (deacon, witness, dogs) that follow prescriptive formulas. These agents parse shell output and make rule-based decisions — they may not need Opus-level reasoning. This framework provides **evidence** for safely downgrading roles to Sonnet or Haiku.

See [Discussion #1542](https://github.com/steveyegge/gastown/discussions/1542) and [Issue #1545](https://github.com/steveyegge/gastown/issues/1545).

## Quick Start

```bash
# Prerequisites: Node.js 18+, Anthropic API key
export ANTHROPIC_API_KEY=sk-ant-...

# Run all tests across Opus, Sonnet, Haiku
npx promptfoo@latest eval

# Run 3x per test for consistency measurement
npx promptfoo@latest eval --repeat 3

# View results in browser
npx promptfoo@latest view

# Export results
npx promptfoo@latest eval --output results.json --output results.html
```

## Test Suites

| File | Role | Tests | What It Measures |
|------|------|-------|-----------------|
| `deacon-zombie.yaml` | Deacon | 10 | Zombie session detection vs healthy/idle/crashed |
| `deacon-plugin-gate.yaml` | Deacon | 10 | Plugin cooldown/cron gate evaluation (mechanical) |
| `deacon-dog-health.yaml` | Deacon | 10 | Dog timeout matrix and pool spawn/retire decisions |
| `witness-stuck.yaml` | Witness | 12 | Stuck polecat assessment: no-op → nudge → escalate |
| `witness-cleanup.yaml` | Witness | 10 | Dead session cleanup: nuke vs recover vs escalate |
| `refinery-triage.yaml` | Refinery | 10 | Test failure diagnosis: branch-caused vs pre-existing |
| `refinery-conflict.yaml` | Refinery | 10 | Merge conflict handling and push verification |
| `dog-orphan.yaml` | Dog | 10 | Orphan triage: RESET/REASSIGN/RECOVER/ESCALATE/BURN |

**Total: 82 test cases** across 4 patrol roles.

Each test provides simulated shell output and expects a structured JSON decision. Tests are split into "clear" cases (all models should agree) and "edge" cases (where model quality matters).

## How It Works

Each test case simulates a patrol agent decision point:

1. **System prompt** defines the role and formula step
2. **Shell output** provides the evidence (tmux status, git state, bead data)
3. **Model responds** with a JSON decision: `{"action": "...", "reason": "..."}`
4. **Assertions check** correctness, cost, latency, and safety

Promptfoo runs the same test across all three models and compares results.

## Sharing Results

```bash
# Generate markdown report
./scripts/results-to-discussion.sh results.json

# Post directly to GitHub Discussions
./scripts/results-to-discussion.sh results.json --post

# Post to a different repo
./scripts/results-to-discussion.sh results.json --post --repo your-org/gastown
```

## Contributing Test Cases

Test files are YAML in `tests/`. Each test case needs:

```yaml
- description: "What this test checks"
  vars:
    role: deacon|witness|refinery|dog
    role_context: >
      Context about the role and what it should know.
    formula_step: which-step
    allowed_actions: '["action1", "action2", ...]'
    shell_output: |
      $ command
      output
    context: "Additional context for the decision."
  assert:
    - type: javascript
      value: |
        const d = JSON.parse(output);
        return d.action === "expected-action";
```

Good test cases:
- Have a clear expected outcome
- Include realistic shell output
- Test one decision at a time
- Cover both "clear" and "edge" cases

## Project Structure

```
gt-model-eval/
├── promptfooconfig.yaml           # Main config (3 providers, assertions)
├── prompts/
│   └── patrol-decision.txt        # System prompt template
├── tests/
│   ├── deacon-zombie.yaml         # 10 tests
│   ├── deacon-plugin-gate.yaml    # 10 tests
│   ├── deacon-dog-health.yaml     # 10 tests
│   ├── witness-stuck.yaml         # 12 tests
│   ├── witness-cleanup.yaml       # 10 tests
│   ├── refinery-triage.yaml       # 10 tests
│   ├── refinery-conflict.yaml     # 10 tests
│   └── dog-orphan.yaml            # 10 tests
├── scripts/
│   └── results-to-discussion.sh   # Format + post results
└── README.md
```
