# Decision 009: Session Events Architecture

**Status:** Accepted
**Date:** 2025-12-31
**Context:** Where should session events live? Beads, separate repo, or events.jsonl?

## Decision

Session events are **orchestration infrastructure**, not work items. They stay in
`events.jsonl` (outside beads). Work attribution happens by capturing `session_id`
on beads mutations (issue close, MR merge).

## Context

The seance feature needs to discover and resume Claude Code sessions. This requires:
1. **Pointer** to session (session_id) - for `claude --resume`
2. **Attribution** (which work happened in this session) - for entity CV

Claude Code already stores full session transcripts indefinitely. Gas Town doesn't
need to duplicate them - just point at them.

## The Separation

| Layer | Storage | Content | Retention |
|-------|---------|---------|-----------|
| **Orchestration** | `~/.events.jsonl` | session_start, nudges, mail routing | Ephemeral (auto-prune) |
| **Work** | Beads (rig-level) | Issues, MRs, convoys | Permanent (ledger) |
| **Entity activity** | Beads (entity chain) | Session digests | Permanent (CV) |
| **Transcript** | Claude Code | Full session content | Claude Code's retention |

## Why Not Beads for Events?

1. **Volume**: Orchestration events are high volume, would overwhelm work signal
2. **Ephemerality**: Most orchestration events don't need CV/ledger permanence
3. **Different audiences**: Work items are cross-agent; orchestration is internal
4. **Claude Code has it**: Transcripts already live there; we just need pointers

## Implementation

### Phase 1: Attribution (Now)
- `gt done` captures `CLAUDE_SESSION_ID` in issue close
- Beads supports `closed_by_session` field on issue mutations
- Events.jsonl continues to capture `session_start` for seance

### Phase 2: Session Digests (Future)
- Sessions as wisps: `session_start` creates ephemeral wisp
- Session work adds steps (issues closed, commits made)
- `session_end` squashes to digest
- Digest lives on entity chain (agent CV)

### Phase 3: Pruning (Future)
- Events.jsonl auto-prunes after N days
- Session digests provide permanent summary
- Full transcripts remain in Claude Code

## Consequences

**Positive:**
- Clean separation of concerns
- Work ledger stays focused on work
- CV attribution via session_id on beads mutations
- Seance works via events.jsonl discovery

**Negative:**
- Two systems to understand (events vs beads)
- Need to ensure session_id flows through commands

## Related

- `gt seance` - Session discovery and resume
- `gt-3zsml` - SessionStart hook passes session_id to gt prime
- PRIMING.md - "The Feed Is the Signal" section
- CONTEXT.md - Entity chains and CV model
