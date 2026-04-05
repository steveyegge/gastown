# Gate Bead Instruction Template

This file defines the prose template embedded in every gate bead's description.
The `mol-decompose-with-gates` formula reads this template, populates the
placeholders, and passes it as the `--description` to `gt bead create`.

## Placeholders

| Placeholder | Source | Description |
|-------------|--------|-------------|
| `{{gate_id}}` | Formula runtime | The ID of this gate bead (filled after creation) |
| `{{plan_title}}` | Plan bead title | Human-readable name of the plan/epic |
| `{{plan_bead}}` | Formula var | The bead ID of the parent plan/epic |
| `{{review_steps}}` | Resolved config | Rendered review step sections (see below) |
| `{{step_names}}` | Resolved config | Comma-separated list of step names for summary |

## Template

Everything below the `---` is the literal template that goes into the gate
bead description. Placeholders are substituted at creation time.

---

## Context

This is a verification gate for plan **{{plan_title}}** (`{{plan_bead}}`).

You are a gate polecat. Your job is to execute the review steps listed below
against the implementation work that was done under this plan. You have no
memory of the implementation — everything you need is in this description and
in the code on the current branch.

**Configured review steps:** {{step_names}}

**How this gate works:**
- This bead is blocked by all implementation tasks under the plan.
- When you receive this bead, all implementation is complete.
- Execute each review step below in order.
- If ALL steps pass cleanly, close this gate bead (signals plan is ready).
- If ANY step finds issues, follow the retry loop protocol at the bottom.

## Review Steps

{{review_steps}}

<!-- Each review step is rendered from .gates.toml as:

### <step.name>

<step.description>

**Instructions:**

<step.instructions>

-->

## Retry Loop Protocol

When a review step finds issues, do NOT close this gate bead. Instead:

**1. File one fix bead per issue found:**

```bash
gt bead create "Fix: <concise issue description>" \
  --type=task \
  --description="## Context
Found during gate review of {{plan_title}} ({{plan_bead}}).
Gate bead: {{gate_id}}
Review step: <which step found this>

## Issue
<detailed description of what's wrong>

## Location
<file paths, line numbers, function names>

## Expected Fix
<what the fix should accomplish>

## Acceptance Criteria
- <how to verify the fix is correct>"
```

**2. Add each fix as a blocking dependency on THIS gate bead:**

```bash
gt bead dep add {{gate_id}} <fix-bead-id>
```

This ensures the gate bead becomes blocked again until all fixes are closed.

**3. Sling each fix to a polecat:**

```bash
gt sling <fix-bead-id> <rig>
```

This dispatches the fix work to an available worker.

**4. Exit — do NOT wait for fixes:**

Clear your assignment and let your session end. You are done for now.

The system handles the rest:
- Fix polecats work on each fix bead independently.
- As each fix bead closes, its blocking dep on this gate is satisfied.
- When ALL fix beads close, this gate bead becomes unblocked again.
- The stranded-bead scan re-dispatches this gate bead to a fresh polecat
  within 30 seconds of becoming unblocked.
- The fresh polecat re-runs all review steps from the top.

**Important:** Do NOT attempt to fix issues yourself. Your role is gate review,
not implementation. File precise fix beads so fix polecats can execute without
ambiguity.

## Clean Pass Signal

When ALL review steps pass with no issues found:

```bash
gt close {{gate_id}} --reason="Gate passed: all review steps clean"
```

Closing the gate bead signals that the plan's implementation has passed
verification and is ready for human review.

**Before closing, verify:**
- You executed every configured review step ({{step_names}})
- No issues were found in any step
- You are not skipping a step due to confusion or error

If you are uncertain whether something is an issue, err on the side of filing
a fix bead. False positives waste less time than missed defects.
