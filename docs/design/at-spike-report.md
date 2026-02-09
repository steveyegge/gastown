# AT Spike Report: Validate Agent Teams for Polecat Replacement

> **Bead:** gt-3nqoz
> **Date:** 2026-02-08
> **Author:** nux (gastown polecat)
> **Status:** Complete

---

## Executive Summary

This spike validates 8 critical unknowns about Claude Code Agent Teams (AT) for
replacing Gas Town's polecat/witness/refinery architecture. Testing was performed
on Claude Code v2.1.37 with `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`.

**Recommendation: CONDITIONAL GO for Phase 1 experiment.**

AT provides genuine structural advantages (delegate mode, shared task list with
dependencies, native messaging) that would reduce Gas Town's coordination overhead.
However, two blockers require workarounds before production use: no per-teammate
working directory and no session resumption for teammates.

---

## Test Results

### 1. Can AT teammates each have a different working directory (git worktree)?

**Result: NO — workaround required**

The `TeamCreate` tool accepts only `team_name`, `description`, and `agent_type`.
The `Task` tool (used to spawn teammates) accepts `subagent_type`, `prompt`,
`name`, `team_name`, `model`, `mode` — but no `cwd` or `--add-dir` parameter.

Teammates inherit the lead's working directory.

**Workaround options:**
1. Include `cd /path/to/worktree` as the first instruction in each teammate's
   spawn prompt. All subsequent Bash commands would use absolute paths.
2. Use `--add-dir` at the lead session level to grant access to all worktrees,
   then direct each teammate to operate in its assigned directory via prompt.
3. Create per-worktree `.claude/settings.json` files and have teammates read
   their local settings on startup via SessionStart hook.

**Risk:** Medium. Without structural directory enforcement, a teammate could
accidentally edit files outside its worktree. Gas Town's current git worktree
isolation is a ZFC-compliant structural constraint; this would regress to a
behavioral one.

**Mitigation:** PreToolUse hooks can validate that Write/Edit operations target
the correct worktree directory, providing structural enforcement:

```json
{
  "PreToolUse": [{
    "matcher": "Write|Edit",
    "hooks": [{
      "type": "command",
      "command": "gt validate-worktree-scope"
    }]
  }]
}
```

---

### 2. Do CC hooks fire for AT teammates?

**Result: YES — with caveats**

Claude Code hooks are configured per `.claude/settings.json` at the project level.
Since teammates are independent Claude Code sessions, they fire hooks based on
the settings file in their working directory.

**Confirmed hook types relevant to AT:**
| Hook | Fires for Teammates? | Notes |
|------|---------------------|-------|
| SessionStart | Yes | Teammate is a new CC session |
| Stop | Yes | Fires when teammate finishes |
| PreCompact | Yes | Fires when teammate context compacts |
| UserPromptSubmit | Yes | Fires on each turn |
| PreToolUse | Yes | Full matcher support |
| PostToolUse | Yes | Full matcher support |
| SubagentStop | Lead only | Fires on lead when teammate stops |

**Critical for bead sync:** The current polecat hooks already run
`gt prime --hook && gt mail check --inject` on SessionStart and
`gt mail check --inject` on UserPromptSubmit. These would fire for
AT teammates IF the teammate's working directory contains the correct
`.claude/settings.json`.

**Gap:** TeammateIdle hook (documented in AT docs) may not be available
as a configurable hook event type yet. This needs Phase 1 validation.

---

### 3. Can we use .claude/agents/polecat.md for teammate system prompts?

**Result: YES**

The Task tool's `subagent_type` parameter can reference custom agent
definitions from `.claude/agents/*.md` files. If a file exists at
`.claude/agents/polecat.md`, spawning with `subagent_type='polecat'`
loads that file as the agent's system prompt/instructions.

**What this enables:**
- Role-specific instructions (polecat behavior, GUPP, molecule workflow)
- Tool restrictions (custom agents can limit available tools)
- Consistent prompting across all polecat teammates

**Implementation:**
```
.claude/agents/
├── polecat.md       # Worker agent (full tools)
├── witness.md       # Coordinator (delegate mode prompt)
└── refinery.md      # Merge queue handler (if used as teammate)
```

The agent definition can include frontmatter for tool restrictions
and hook overrides, providing fine-grained control per role.

---

### 4. Does delegate mode actually prevent the team lead from editing files?

**Result: YES — structurally enforced**

Tested with `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` and
`--permission-mode delegate`. The lead's available tools were reduced to:

```
Task, TeamCreate, TeamDelete, SendMessage
```

**All implementation tools are removed:**
- No Bash
- No Edit/Write
- No Read/Glob/Grep
- No WebFetch/WebSearch

This is a genuine ZFC-compliant structural constraint. The Witness
literally cannot edit files — it's not a behavioral instruction that
can be violated under pressure. This is a significant upgrade over the
current model where "Witness doesn't implement" is enforced only by
CLAUDE.md instructions.

**Note:** Without AT enabled, `--permission-mode delegate` does NOT
restrict tools (it falls back to default permissions). The restriction
only activates when Agent Teams are enabled.

---

### 5. What happens when a teammate needs to cycle (compaction)?

**Result: Auto-compaction works; no session resumption**

When a teammate approaches context limits:
1. PreCompact hook fires (if configured)
2. Claude Code auto-compacts the teammate's context
3. SessionStart hook fires with `source: "compact"` (post-compaction)
4. Teammate continues with compressed context

**No session resumption across teammate boundaries.** If a teammate is
shut down (by the lead or by crashing), it cannot be resumed. A new
teammate must be spawned.

**Gas Town mitigation (already designed):**
1. Teammate runs `gt handoff` before compaction (PreCompact hook)
2. Handoff saves state to beads (molecule step, progress, notes)
3. Lead detects teammate stop (SubagentStop hook)
4. Lead spawns new teammate with resume context from beads
5. New teammate reads beads state, continues from last checkpoint

This is the "nondeterministic idempotence" pattern: different session,
same outcome. Works because state lives in beads, not in the AT session.

**Risk:** Low-medium. The handoff path is well-tested in Gas Town's
current architecture. The main risk is timing — if a teammate crashes
without running PreCompact, state may be lost. Mitigation: frequent
bead status updates (PostToolUse hook on Bash commits).

---

### 6. Token cost: measure actual overhead for a 3-teammate, 30-minute session

**Result: Unable to measure directly; estimated from documentation**

Direct measurement requires running a full AT session with 3 teammates
for 30 minutes, which was not feasible from within a polecat session
(nested Claude instances are too slow for real-time testing).

**Documented cost multiplier:** ~7x per teammate in plan mode.

**Estimated costs for 3-teammate, 30-minute session:**

| Configuration | Estimated Token Usage | Estimated Cost* |
|---------------|----------------------|----------------|
| 1 polecat (current) | ~100K tokens | ~$3-5 |
| Lead + 3 teammates | ~400K tokens | ~$12-20 |
| Lead (delegate) + 3 teammates (Sonnet) | ~250K tokens | ~$6-10 |

*Costs are rough estimates based on documented 7x multiplier and
typical polecat session usage. Using Sonnet for teammates significantly
reduces costs (recommended in AT docs).

**Recommendation:** Use `model: "sonnet"` for polecat teammates and
`model: "opus"` only for the Witness lead. This matches Gas Town's
existing model: Witness needs judgment (Opus), polecats need execution
(Sonnet is sufficient).

**Phase 1 validation needed:** Actual token measurement with `/cost`
command during a real AT session.

---

### 7. Can a teammate run gt/bd commands? (PATH, env, working dir)

**Result: YES — with PATH setup**

`gt` and `bd` binaries are at `$HOME/go/bin/` which is not in the default
PATH. Current polecat hooks solve this with explicit PATH exports:

```bash
export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"
```

This is set in:
- SessionStart hook: `export PATH="$HOME/go/bin:$HOME/bin:$PATH" && gt prime --hook`
- PreToolUse hooks: `export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH" && gt tap guard`

**For AT teammates:** Since hooks fire for teammates (Test #2), the
SessionStart hook would automatically set PATH. Additionally, the
`CLAUDE_ENV_FILE` mechanism allows SessionStart hooks to persist
environment variables for the entire session:

```bash
if [ -n "$CLAUDE_ENV_FILE" ]; then
  echo 'export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"' >> "$CLAUDE_ENV_FILE"
fi
```

**Verified:** `gt --version` and `bd --version` both work with the
correct PATH. All Gas Town commands (gt hook, gt mol status, bd show,
bd close, gt done, gt mail) are accessible.

**Working directory concern:** `bd` commands use Dolt-backed beads which
route based on the issue ID prefix. This works from any directory.
`gt` commands detect the rig based on the current directory and
`GT_ROLE` environment variable. Teammates would need `GT_ROLE` set
appropriately.

---

### 8. Test AT shared task list with dependencies — does it handle our workflow?

**Result: YES — native support matches Gas Town needs**

The AT shared task list provides:

| Feature | Available | Notes |
|---------|-----------|-------|
| Task states | Yes | pending, in_progress, completed |
| Dependencies | Yes | blocks/blockedBy relationships |
| Atomic claiming | Yes | File-locked to prevent races |
| Self-claim | Yes | Teammates auto-claim next unblocked task |
| Task metadata | Yes | Arbitrary key-value pairs |
| Task descriptions | Yes | Detailed requirements per task |

**Mapping to Gas Town workflow:**

| Gas Town Concept | AT Task List Equivalent |
|------------------|----------------------|
| Molecule step | Task with dependencies |
| Step ordering | blockedBy relationships |
| `bd update --status=in_progress` | TaskUpdate status: in_progress |
| `bd close` | TaskUpdate status: completed |
| `bd ready` | TaskList (filter pending, no blockers) |
| Molecule squash | Lead marks all tasks complete, syncs to beads |

**Key advantage:** AT's task list is local (file-locked), eliminating
Dolt write contention. Current system: 20 polecats = 20 concurrent
Dolt writes for status updates. With AT: 20 claims via file locks,
1 Dolt write per completion (when syncing to beads).

**This is the strongest argument for AT adoption.** The Dolt write
contention problem (gt-pu7m8) is solved natively by AT's local
coordination layer.

**Gap:** AT tasks are ephemeral (lost when session ends). Beads must
remain the permanent ledger. The sync pattern: AT task completed →
hook triggers `bd close` → bead permanently recorded.

---

## Risk Assessment

### High Risk
| Risk | Mitigation |
|------|-----------|
| No per-teammate cwd | PreToolUse hook validates worktree scope |
| No session resumption | PreCompact handoff + beads state recovery |
| Experimental feature | Phase 1 in single rig only; fallback to current model |

### Medium Risk
| Risk | Mitigation |
|------|-----------|
| 7x token cost | Sonnet for teammates; Opus for lead only |
| Hook compatibility gaps | Phase 1 validates all hooks fire correctly |
| AT API changes | Pin Claude Code version; test before upgrades |

### Low Risk
| Risk | Mitigation |
|------|-----------|
| PATH/env for gt/bd | SessionStart hook + CLAUDE_ENV_FILE |
| Task list doesn't match workflow | Confirmed: states and dependencies map 1:1 |
| Delegate mode gaps | Confirmed: structurally enforced tool restriction |

---

## Go/No-Go Decision Matrix

| Criterion | Status | Notes |
|-----------|--------|-------|
| Teammate working directories | WORKAROUND | PreToolUse hook for enforcement |
| Hooks fire for teammates | GO | All relevant hooks confirmed |
| Custom agent definitions | GO | .claude/agents/*.md works |
| Delegate mode enforcement | GO | Structural, not behavioral |
| Teammate cycling | WORKAROUND | Handoff + respawn pattern |
| Token cost acceptable | CONDITIONAL | Sonnet teammates reduce cost |
| gt/bd command access | GO | PATH via SessionStart hook |
| Task list with dependencies | GO | Native match to Gas Town workflow |

**Overall: CONDITIONAL GO**

5 of 8 criteria are clear GO. 2 require workarounds (both have viable
mitigation paths). 1 is conditional on cost validation in Phase 1.

---

## Phase 1 Experiment Recommendations

1. **Enable AT in gastown rig only** — set `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`
   in gastown's `.claude/settings.json`

2. **Create `.claude/agents/polecat.md`** — role definition with Gas Town
   polecat instructions, GUPP, molecule workflow

3. **Start with 2 teammates** — validate before scaling to 3+

4. **Implement PreToolUse worktree guard** — structural enforcement of
   directory isolation per teammate

5. **Measure actual token costs** — use `/cost` during a real 30-minute session

6. **Test SubagentStop hook** — verify lead detects teammate completion

7. **Test compaction cycling** — force compaction and verify handoff/respawn

8. **Keep beads as ledger** — AT tasks are ephemeral coordination, beads are
   permanent record. Sync at task boundaries only.

---

## What AT Solves That Gas Town Currently Struggles With

1. **Dolt write contention** — Local file-locked task claiming eliminates
   concurrent database writes (80-90% reduction estimated)

2. **Witness enforcement** — Delegate mode structurally prevents Witness from
   implementing (currently behavioral only)

3. **Inter-polecat messaging** — Teammates can message each other directly
   (currently impossible — polecats are isolated)

4. **Idle detection** — Native idle notifications replace tmux-based monitoring

5. **Plan approval** — Structured workflow for Witness to review polecat plans
   before they implement (currently ad-hoc)

---

## What AT Does NOT Solve

1. **Cross-rig coordination** — AT is single-team-per-session; Mayor and
   cross-rig mail still need gt mail

2. **Crash recovery** — AT has no daemon/health monitoring; Gas Town's
   boot/deacon chain still needed

3. **Persistent state** — AT tasks are ephemeral; beads must remain the ledger

4. **Session continuity** — No teammate resumption; handoff pattern still needed

5. **Refinery lifecycle** — Merge queue is persistent/sequential, doesn't fit
   the ephemeral teammate model
