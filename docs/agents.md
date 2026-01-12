# Gas Town Agent Architecture

Agent definitions for Gas Town components using Claude 2.1.1 frontmatter format.

**Agent file location:** `~/.claude/agents/gastown-*.md`

---

## Agent Definition Schema

Claude 2.1.1 uses YAML frontmatter for agent configuration:

```yaml
---
description: <one-line purpose>
model: <opus|sonnet|haiku>
tools:
  - <tool-name>
skills:
  - <skill-name>
hooks:
  SessionStart:
    - action: run
      command: "<shell command>"
permissionMode: <default|auto>
---

# Agent Title

<agent-specific instructions in markdown>
```

### Schema Fields

| Field | Required | Values | Purpose |
|-------|----------|--------|---------|
| `description` | Yes | string | One-line agent purpose |
| `model` | Yes | opus, sonnet, haiku | Model routing |
| `tools` | No | list | Enabled tool set |
| `skills` | No | list | Auto-triggered skills |
| `hooks` | No | object | Lifecycle hooks |
| `permissionMode` | No | default, auto | File/command permissions |

---

## Component-to-Agent Mapping

| Component | Agent File | Model | Permission Mode |
|-----------|------------|-------|-----------------|
| Polecat | `gastown-polecat.md` | sonnet | auto |
| Mayor | `gastown-mayor.md` | opus | default |
| Witness | `gastown-witness.md` | haiku | default |
| Refinery | `gastown-refinery.md` | haiku | default |
| Crew | `gastown-crew.md` | sonnet | default |

---

## Skill Assignment Rationale

### Polecat (Worker)
**Skills:** `beads`, `sk-implement`

- `beads` - Must track issue status during execution
- `sk-implement` - Core implementation workflow for task completion
- No `sk-research` - Polecats execute, don't explore (scope creep prevention)
- No `sk-validation-chain` - Relies on tests, not formal validation gates

### Mayor (Coordinator)
**Skills:** `beads`, `sk-gastown`, `sk-plan`, `sk-research`

- `beads` - Needs to create, route, and track work across rigs
- `sk-gastown` - Gas Town orchestration commands
- `sk-plan` - Creates implementation plans and epics
- `sk-research` - Deep exploration before planning
- No `sk-implement` - Mayor coordinates, doesn't implement

### Witness (Monitor)
**Skills:** `beads`

- `beads` - Read-only access to track issue status
- Minimal skills - observation role, not action
- No implementation or planning skills - escalates to Mayor

### Refinery (Merger)
**Skills:** `beads`

- `beads` - Must check issue status for MERGE_READY signal
- Minimal skills - single-purpose merge processor
- No implementation skills - only merges completed work

### Crew (Human-Managed)
**Skills:** `beads`, `sk-implement`, `sk-research`, `sk-validation-chain`

- `beads` - Full issue tracking capabilities
- `sk-implement` - Core implementation workflow
- `sk-research` - Can explore codebase with human guidance
- `sk-validation-chain` - Comprehensive quality gates
- Most complete skill set - human provides direction

---

## Hook Configuration

### SessionStart Hooks

| Component | Hook Command | Purpose |
|-----------|--------------|---------|
| Polecat | `gt prime && gt hook` | Initialize context + show hooked work |
| Mayor | `gt hook` | Show hooked work (context via rig CLAUDE.md) |
| Witness | `gt prime` | Initialize context only |
| Refinery | (none) | Triggered on demand |
| Crew | (none) | Waits for human direction |

### Hook Behavior

**Polecat:** Auto-executes hooked work (Propulsion Principle). `permissionMode: auto` enables autonomous operation.

**Mayor:** Shows hooked work, expects immediate execution but operates in `default` mode for safety.

**Witness:** Primes context, then surveys polecat status. No work hook.

**Refinery:** No startup hook. Triggered by mail or manual invocation.

**Crew:** No startup hook. Waits for human to provide direction.

---

## Tool Assignment

### Full Development Stack
**Polecat, Crew:** `Read`, `Grep`, `Glob`, `Bash`, `Edit`, `Write`, `LSP`

Complete toolset for code implementation.

### Read-Only Stack
**Witness, Refinery:** `Read`, `Bash`, `Grep`

Monitoring and git operations only. No file editing.

### Coordination Stack
**Mayor:** `Read`, `Grep`, `Glob`, `Bash`

Can read code for planning but cannot edit. Forces dispatch to workers.

---

## Verification Procedures

### Confirm Agent Loaded

Check the active agent in Claude Code session:

```bash
# Agent appears in session header
# Should show: "Using agent: gastown-<component>"
```

### Verify Hook Execution

```bash
# For Polecat - should show hooked work
gt hook

# For Mayor - should show context primed
gt hook

# For Witness - should show rig status
gt polecat list <rig>
```

### Verify Skills Available

Test skill triggers in session:
- Say "show blockers" - should activate beads skill
- Say "implement this" - should activate sk-implement (Polecat/Crew only)

### Verify Model Routing

Check model in session metadata or via response characteristics:
- Opus: More thorough analysis, strategic thinking
- Sonnet: Balanced implementation quality
- Haiku: Concise, focused responses

---

## Agent File Locations

All agent files live in the user's Claude configuration:

```
~/.claude/agents/
  gastown-polecat.md     # Worker agent
  gastown-mayor.md       # Global coordinator
  gastown-witness.md     # Lifecycle monitor
  gastown-refinery.md    # Merge processor
  gastown-crew.md        # Human-managed developer
```

### Loading Agents

Agents are loaded based on working directory detection or explicit selection:
- Polecat: Detected when in `<rig>/polecats/<name>/`
- Mayor: Detected when in `~/gt/` (town root)
- Witness: Detected when in `<rig>/witness/`
- Refinery: Detected when in `<rig>/refinery/`
- Crew: Detected when in `<rig>/crew/<name>/`
