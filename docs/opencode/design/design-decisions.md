# OpenCode Integration: Design Decisions

> **Purpose**: Capture high-level design decisions for OpenCode integration  
> **Status**: Living document  
> **Updated**: 2026-01-19

---

## Core Principles

These are the guiding principles for OpenCode integration:

| Principle | Description |
|-----------|-------------|
| **Runtime Agnostic** | Code works with both Claude Code and OpenCode |
| **Claude Compatibility** | Reuse existing patterns (CLAUDE.md, hooks) where possible |
| **Plugin-First** | Prefer plugin injection over config changes |
| **Separation of Concerns** | Keep permissions in config, instructions in markdown |

---

## Decided ✅

### D1: Use CLAUDE.md for Both Runtimes

**Decision**: Reuse existing `CLAUDE.md` files for OpenCode.

**Rationale**:
- OpenCode can read `CLAUDE.md` via `instructions` config
- Avoids duplicating role templates
- Existing templates at `internal/templates/roles/*.md.tmpl` work unchanged

**Implementation**:
```jsonc
// .opencode/config.jsonc
{
  "agents": {
    "polecat": {
      "instructions": ["CLAUDE.md"],  // Reuse existing file
      "permission": { "*": "allow" }
    }
  }
}
```

**Date**: 2026-01-19

---

### D2: Separate Permissions from Instructions

**Decision**: Use OpenCode custom agents to separate config (permissions) from content (instructions).

**Rationale**:
- Permissions are machine-readable config
- Instructions are human-readable markdown
- Enables reuse and versioning of instructions
- Same pattern works for commands

**Implementation**:
```jsonc
{
  "agents": {
    "polecat": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["roles/polecat.md"],   // Content in markdown
      "permission": { "*": "allow" }           // Config in JSON
    }
  }
}
```

**Date**: 2026-01-19

---

### D3: Plugin Injection for Dynamic Context

**Decision**: Use gastown.js plugin to inject context on `session.created`.

**Rationale**:
- Works without config changes to workspaces
- Can use environment variables (GT_ROLE) to select behavior
- Already implemented for `gt prime`

**Implementation**: Plugin calls `gt prime` and injects output on session start.

**Date**: 2026-01-17 (Phase 1 complete)

---

### D4: Full Claude Code Backward Compatibility

**Decision**: No breaking changes to existing Claude Code workflows.

**Rationale**:
- Claude Code remains default
- OpenCode is opt-in
- Existing configs continue to work

**Status**: ✅ Confirmed (from phase1/decisions.md D5)

**Date**: 2026-01-15

---

## Pending 🔄

### P1: Template File Location for OpenCode

**Question**: Where should role instruction files live for OpenCode workspaces?

**Options**:
| Option | Path | Pros | Cons |
|--------|------|------|------|
| A | `CLAUDE.md` | Existing file, no duplication | Name confusion |
| B | `roles/*.md` | Clean separation | New convention |
| C | `.opencode/roles/*.md` | OpenCode-specific | Hidden |

**Leaning**: Option A (reuse CLAUDE.md) with plugin injection as backup

**Blocked by**: Real-world testing

---

### P2: Formula/Prompt Injection Strategy

**Question**: How do we pass role-specific prompts (formulas) to OpenCode?

**Options**:
| Option | How | Pros | Cons |
|--------|-----|------|------|
| A | Custom agents in config | Clean, built-in | Requires config per workspace |
| B | Plugin injection | Dynamic, no config | More complex plugin |
| C | CLAUDE.md + instructions config | Simple | Requires both file and config |

**Leaning**: Option A (custom agents) for production

**Blocked by**: Testing in real workflows

---

### P3: Compaction/Context Management

**Question**: How do we detect and manage context fullness?

**Current State**: Plugin has `experimental.session.compacting` hook

**Open Questions**:
- Does OpenCode expose token count?
- Should we trigger compaction proactively?
- How do we detect "close to full"?

**Blocked by**: OpenCode API research

---

### P4: Plugin Simplification - Extract Common Functionality

**Question**: gastown.js duplicates logic that should be centralized.

**Current Plugin Contains**:
| Lines | Functionality | Should Be... |
|-------|---------------|--------------|
| 20-38 | Structured logging | In gt CLI (`gt log`?) or core |
| 40-68 | Finding gt binary | Unnecessary if PATH is correct |
| 14-15 | Role definitions (autonomous/interactive) | In Go config, not JS |
| 133-145 | Debounce logic | Possibly in core |

**Comparison with Claude Hooks**:
```json
// Claude: Simple, just calls commands
"SessionStart": [{
  "command": "gt prime && gt mail check --inject && gt nudge deacon session-started"
}]
```

```javascript
// OpenCode: 190 lines of infrastructure + same commands
const findGt = async () => { ... }  // 30 lines
const log = (level, event, message) => { ... }  // 20 lines
const autonomousRoles = new Set([...])  // duplicated from Go
```

**Proposed Simplification**:
1. **PATH assumption**: Like Claude, assume `gt` is in PATH (set by tmux/shell)
2. **Role knowledge**: Plugin gets `GT_ROLE` env but shouldn't need role sets
3. **Logging**: Defer to `gt` - commands log their own output
4. **Commands**: Plugin just calls commands, no wrapper infrastructure

**Target Plugin** (minimal):
```javascript
export const GasTown = async ({ $ }) => ({
  event: async ({ event }) => {
    if (event?.type === "session.created") {
      await $`gt prime && gt mail check --inject && gt nudge deacon session-started`;
    }
    if (event?.type === "message.updated" && event.properties?.info?.role === "user") {
      await $`gt mail check --inject`;
    }
  },
  "experimental.session.compacting": async () => {
    await $`gt prime`;
  },
});
```

**Blocked by**: Verify PATH works in OpenCode plugin environment

---

## Constraints

| Constraint | Description | Source |
|------------|-------------|--------|
| OpenCode version | Requires plugin support | OpenCode 0.7.0+ |
| Node.js | Plugin runtime | Node 18+ |
| No OPENCODE.md auto-read | OpenCode doesn't auto-load OPENCODE.md | OpenCode docs |
| Environment variables | GT_ROLE, GT_BINARY_PATH must be set | Gastown |

---

## Assumptions

| ID | Assumption | Confidence | Validated? |
|----|------------|------------|------------|
| A1 | CLAUDE.md can be loaded via `instructions` config | High | ✅ Yes |
| A2 | Plugin can inject context on session.created | High | ✅ Yes |
| A3 | Custom agents support `instructions` array | High | ⬜ TBD |
| A4 | Commands can reference external files via `@` | Medium | ⬜ TBD |
| A5 | Plugin can access GT_ROLE environment variable | High | ✅ Yes |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [phase1/decisions.md](phase1/decisions.md) | Phase 1 detailed decisions and experiments |
| [plugin-implementation.md](../reference/plugin-implementation.md) | Plugin implementation details |
| [role-permissions.md](role-permissions.md) | Permission profiles per role |
| [next-steps.md](next-steps.md) | What to work on next |
| `../reference/customization.md` | How to configure custom agents |

---

## Decision Log

| Date | Decision | Summary |
|------|----------|---------|
| 2026-01-19 | D1, D2 | Use CLAUDE.md, separate permissions from instructions |
| 2026-01-17 | D3 | Plugin injection for context |
| 2026-01-15 | D4 | Full backward compatibility |
