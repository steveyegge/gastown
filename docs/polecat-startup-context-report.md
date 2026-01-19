# Polecat Startup Context Report

**Bead**: hq-79ptcl - Debug: Report polecat startup context
**Date**: 2026-01-18
**Polecat**: keeper in gastown

## 1. Hook Status

```
🪝 Hook Status: gastown/polecats/keeper
Role: polecat

🚀 AUTONOMOUS MODE - Work on hook triggers immediate execution

🪝 Hooked: hq-79ptcl: Debug: Report polecat startup context
No molecule attached (hooked bead still triggers autonomous work)
```

## 2. Environment Variables (GT_* and BD_*)

```
BD_ACTOR=gastown/polecats/keeper
GT_POLECAT=keeper
GT_RIG=gastown
GT_ROLE=polecat
GT_ROOT=/home/ubuntu/pihealth
GT_TOWN_ROOT=/home/ubuntu/pihealth
```

## 3. Working Directory

**pwd**: `/home/ubuntu/pihealth/gastown/polecats/keeper/gastown`

**Contents**: gastown repo worktree with standard structure:
- `.beads/` (rig-level beads with redirect to town)
- `cmd/`, `internal/`, `docs/` (Go project structure)
- `AGENTS.md`, `CHANGELOG.md`, `README.md`, etc.

## 4. Assigned Bead

```
? hq-79ptcl · Debug: Report polecat startup context   [● P1 · HOOKED]
Owner: gastown/crew/validator · Assignee: gastown/polecats/keeper · Type: task
```

## 5. CLAUDE.md

No `CLAUDE.md` exists in the worktree. `AGENTS.md` at worktree root says:

> See **CLAUDE.md** for complete agent context... Full context is injected by `gt prime` at session start.

Context is delivered dynamically via `gt prime`, not via static CLAUDE.md file.

## 6. System Prompts / Role Context

From `gt prime`, I received comprehensive context including:

- **Role**: POLECAT (worker: keeper in gastown)
- **The Idle Polecat Heresy**: Must run `gt done` after completing work - no waiting for approval
- **GUPP (Propulsion Principle)**: If work is on hook, execute immediately - no confirmation needed
- **Self-Cleaning Model**: Polecats are self-cleaning; `gt done` submits to Refinery MQ, then session exits
- **Capability Ledger**: Work is tracked, quality completions accumulate
- **Two-Level Beads**: Town-level (hq-*) and rig-level beads with prefix routing
- **Startup Protocol**: Check hook → If work hooked, RUN IT → No decisions needed
- **Escalation Path**: Use `gt escalate` or mail Witness/Mayor when blocked
- **Completion Requirements**: Git must be clean, run `gt done` to submit to merge queue

Key operational rules:
- Never push directly to main (Refinery handles that)
- Never create GitHub PRs (direct push access in maintainer repos)
- Never wait idle - escalate or `gt done`
- File desire-path beads when CLI surprises reveal intuitive gaps

## Issues Encountered

1. **Mail System Failure**: `gt mail send` failed with error:
   - "Auto-import failed: import requires SQLite storage backend"
   - "invalid issue type: message"
   - Escalation filed: hq-e3a8f8

2. **Bead Not Found**: `bd show hq-79ptcl` initially worked but subsequent queries couldn't find the bead in the database. Possible sync/routing issue.

---
Report complete.
