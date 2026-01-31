# Agent Advice System

> Evolving guidance for agents based on learned patterns and operational experience

## Overview

The Agent Advice system allows operators to inject context-aware guidance into agent
sessions. Unlike static role templates, advice beads are dynamic - they can be created,
updated, and removed at runtime based on observed patterns and operational needs.

Advice is delivered during `gt prime` and appears in the agent's context as a dedicated
"Agent Advice" section, filtered to show only guidance relevant to that specific agent.

## Why Advice Matters

Static role templates capture general patterns, but every deployment evolves:

- **Learned patterns:** "Always run tests before gt done" emerges from failures
- **Rig-specific context:** Production rigs may need different guidance than dev rigs
- **Agent-specific coaching:** Some agents need reminders about specific behaviors
- **Temporary guidance:** "Avoid the /api/v2 endpoint until Monday" - time-bounded advice

The advice system bridges the gap between static documentation and dynamic operational
learning, creating a feedback loop where failures become guidance for future agents.

## Targeting Hierarchy

Advice uses a three-level targeting hierarchy. Each advice bead specifies one target
scope, and agents receive advice that matches their context:

| Scope | Field | Example | Matches |
|-------|-------|---------|---------|
| **Agent** | `advice_target_agent` | `gastown/polecats/alpha` | Only that specific agent |
| **Role** | `advice_target_role` | `polecat` | All polecats in all rigs |
| **Rig** | `advice_target_rig` | `gastown` | All agents in gastown |
| **Global** | (all empty) | - | Every agent everywhere |

### Matching Rules

1. **Agent-specific** advice matches only if `advice_target_agent` equals the agent's full ID
2. **Role-specific** advice matches if `advice_target_role` equals the agent's role type
3. **Rig-specific** advice matches if `advice_target_rig` equals the agent's rig
4. **Global** advice (no targeting fields set) matches all agents

All matching advice is shown - there's no "most specific wins" exclusion. An agent
might see global advice, role advice, and agent-specific advice simultaneously.

## Agent Identity Format

Advice targeting uses the standard Gas Town identity format:

| Role | Identity Format | Example |
|------|-----------------|---------|
| Polecat | `{rig}/polecats/{name}` | `gastown/polecats/alpha` |
| Crew | `{rig}/crew/{name}` | `gastown/crew/prime_analyst` |
| Witness | `{rig}/witness` | `gastown/witness` |
| Refinery | `{rig}/refinery` | `gastown/refinery` |
| Deacon | `deacon` | `deacon` |
| Mayor | `mayor` | `mayor` |

## Managing Advice

### The `bd advice` Command

The `bd advice` command provides dedicated subcommands for managing advice beads:

```bash
bd advice add     # Create new advice
bd advice list    # List advice beads
bd advice remove  # Remove advice
```

### Creating Advice with `bd advice add`

```bash
# Global advice (all agents)
bd advice add "Always verify git status before pushing" \
  -d "Run 'git status' to check for uncommitted changes before 'git push'"

# Role-targeted advice (all polecats)
bd advice add "Check hook before checking mail" \
  --role polecat \
  -d "The hook is authoritative. Always run 'gt hook' first on startup."

# Rig-targeted advice (all agents in gastown)
bd advice add "Use fimbaz account for spawning" \
  --rig gastown \
  -d "The matthewbaker account has credential issues. Use --account fimbaz."

# Agent-specific advice
bd advice add "You own the shiny formula" \
  --agent gastown/crew/prime_analyst \
  -d "Monitor polecats using shiny and iterate on the formula based on results."
```

### Targeting Flags

| Flag | Purpose | Example Value |
|------|---------|---------------|
| `--agent` | Target specific agent | `gastown/polecats/alpha` |
| `--role` | Target role class | `polecat`, `crew`, `witness` |
| `--rig` | Target all agents in rig | `gastown`, `beads` |

Only one targeting flag should be used per advice bead. Using multiple creates
ambiguous matching behavior.

### Listing Advice with `bd advice list`

```bash
# List all advice (formatted output)
bd advice list

# Verbose output with targeting details
bd advice list --verbose

# Filter by scope
bd advice list --role polecat
bd advice list --rig gastown
bd advice list --agent gastown/crew/joe
```

### Removing Advice with `bd advice remove`

```bash
# Close advice (marks as no longer active)
bd advice remove gt-tsk-xyz

# With removal reason
bd advice remove gt-tsk-xyz -r "No longer applicable after deploy"

# Permanently delete (not just close)
bd advice remove gt-tsk-xyz --delete
```

### Alternative: Using Generic Commands

You can also manage advice using standard `bd` commands:

```bash
# Create via bd create
bd create -t advice "Title" --advice-target-role polecat -d "Description"

# List via bd list
bd list -t advice --json

# Close via bd close
bd close gt-tsk-xyz -r "Reason"
```

The `bd advice` subcommands are convenience wrappers that provide cleaner syntax
and advice-specific filtering options.

## Delivery Pipeline

### When Advice is Delivered

Advice is output during `gt prime`, after role context and before handoff content:

```
gt prime flow:
├── Session metadata
├── Role context (template)
├── Agent Advice          ← advice delivered here
├── Handoff content
├── Auto-seance context
├── Attachment status
└── ...
```

### Output Format

Agents see advice in a dedicated section:

```
## 📝 Agent Advice

**[Polecat]** Check hook before checking mail
  The hook is authoritative. Always run 'gt hook' first on startup.

**[Global]** Always verify git status before pushing
  Run 'git status' to check for uncommitted changes before 'git push'
```

The scope indicator (`[Polecat]`, `[Global]`, etc.) shows why this advice matched.

## Best Practices

### Writing Good Advice

1. **Be specific:** "Run tests" is vague. "Run `make test` before `gt done`" is actionable.
2. **Explain why:** Include the reason or failure that prompted this advice.
3. **Keep it brief:** Agents have limited context. One clear point per advice bead.
4. **Use imperative mood:** "Check your hook" not "You should check your hook"

### When to Create Advice

- After observing a repeated failure pattern
- When a rig has specific configuration needs
- When an agent needs coaching on a specific behavior
- For temporary operational constraints

### When to Remove Advice

- When the underlying issue is fixed
- When the advice becomes standard practice (move to role template)
- When the advice is superseded by newer guidance
- When temporary constraints expire

## Integration with "Fail then File"

The advice system complements the "Fail then File" principle:

1. **<FAIL>** - Agent encounters an issue
2. **<FILE>** - Create a bug bead to track the issue
3. **<FIX>** - Resolve the underlying problem
4. **<ADVISE>** - Create advice to prevent recurrence

Example flow:
```bash
# 1. Agent fails: pushed with uncommitted changes
# 2. File the bug
bd create -t bug "Pushed incomplete work to main" -d "..."

# 3. Fix: add pre-push check
# 4. Create advice for all polecats
bd create -t advice "Verify git status before pushing" \
  --advice-target-role polecat \
  -d "Always run 'git status' before 'git push' to catch uncommitted changes."
```

## Technical Details

### Implementation

The advice delivery is implemented in `internal/cmd/prime_advice.go`:

- `outputAdviceContext()` - Main entry point, called from `gt prime`
- `queryAdviceBeads()` - Fetches all advice via `bd list -t advice --json`
- `filterApplicableAdvice()` - Matches advice to agent context
- `buildAgentID()` - Constructs agent identity from role context

### Query Limit

The system queries up to 100 advice beads. For deployments with more active advice,
consider consolidating related guidance or closing obsolete advice.

### Silent Failure

If `bd` is unavailable or the query fails, advice delivery is silently skipped.
This ensures `gt prime` doesn't fail due to advice system issues.

## Examples

### Scenario: Polecat Push Failures

Multiple polecats pushed incomplete work. Create role-wide advice:

```bash
bd create -t advice "Verify all changes before pushing" \
  --advice-target-role polecat \
  -d "Before 'git push':
1. Run 'git status' to see uncommitted changes
2. Run 'git diff' to review staged changes
3. Ensure tests pass locally"
```

### Scenario: Rig-Specific Configuration

The gastown rig requires a specific account:

```bash
bd create -t advice "Account configuration for gastown" \
  --advice-target-rig gastown \
  -d "Use '--account fimbaz' when spawning polecats. Other accounts may have credential issues."
```

### Scenario: Agent Coaching

A specific crew member keeps forgetting to sync beads:

```bash
bd create -t advice "Remember to sync beads" \
  --advice-target-agent gastown/crew/analyst \
  -d "Run 'bd sync' after creating or updating beads to persist changes."
```

## Related Concepts

- [Agent Identity](identity.md) - How agent IDs are constructed
- [Propulsion Principle](propulsion-principle.md) - Autonomous work execution
- [File After Fail](../../file-after-fail.md) - Tracking failures as beads
