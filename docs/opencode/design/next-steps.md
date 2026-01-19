# OpenCode Integration: Next Steps

> **Status**: Active development  
> **Updated**: 2026-01-19  
> **Purpose**: Track what to work on next for OpenCode integration

---

## Current State

### Completed ✅

- **Phase 1: Claude Code Parity** - All 4 hooks implemented via OpenCode plugin
- **Documentation Reorganization** - Full `reference/` directory with stable docs
- **Validation System** - `CONTRIBUTING.md` + `validate-opencode-docs.sh`
- **Runtime-Agnostic Testing** - `test-runtime-e2e.sh` for both runtimes

### In Progress 🔄

- **Comprehensive E2E Testing** - Real-world integration tests
- **Regression Testing** - Ensure Claude Code still works

---

## Immediate Next Steps

### 1. Complete E2E Test Suite (Priority)

**Goal**: Test each key capability with realistic tasks, progressing simple → complex.

#### Test Progression

| Level | Scenario | What to Test |
|-------|----------|--------------|
| **L1: Basic** | Session creation | Plugin loads, hooks fire, gt commands work |
| **L2: Simple Task** | "Create a file" | Edit tools work, completion detected |
| **L3: Medium Task** | "Fix a bug in X" | Prime → understand → edit → test |
| **L4: Complex Task** | "Add feature Y" | Multi-file edits, compaction, session management |
| **L5: E2E Workflow** | Full Polecat lifecycle | Work assignment → execution → completion → handoff |

#### Key Capabilities to Test

| Capability | OpenCode | Claude Code | Test Script |
|------------|----------|-------------|-------------|
| Session creation | ✅ `session.created` | ✅ Hook | `test-runtime-e2e.sh` |
| Context recovery (prime) | ✅ PreCompact hook | ✅ Hook | TBD |
| Work assignment (mail) | ✅ Plugin tool | ✅ Hook | `test-opencode-polecat-e2e.sh` |
| Specialized agents (formulas) | ⚠️ TBD | ✅ `-p` flag | TBD |
| Compaction detection | ⚠️ TBD | N/A (external) | TBD |
| Session lifecycle | ⚠️ TBD | ✅ Manual | TBD |

**Tasks**:
- [ ] Run `./scripts/test-runtime-e2e.sh --runtime both` and fix issues
- [ ] Run existing OpenCode E2E tests and verify PASS
- [ ] Create L2-L5 test scenarios
- [ ] Test mixed primary/secondary runtime scenarios

### 2. Role Templates (CLAUDE.md → OPENCODE.md)

**Current State**:
- Claude Code uses `CLAUDE.md` files with role-specific instructions
- Templates are at `internal/templates/roles/*.md.tmpl` (embedded)
- Claude Code renders these and writes to workspace as `CLAUDE.md`
- OpenCode has no equivalent - test scripts create minimal `OPENCODE.md` manually

**Gap**: OpenCode doesn't get the full role instructions like Claude Code does.

**Implementation Options**:

| Option | How | Pros | Cons |
|--------|-----|------|------|
| **A: OPENCODE.md templates** | Create `internal/templates/roles/*-opencode.md.tmpl`, write as `OPENCODE.md` | Mirrors Claude pattern | Duplicates templates |
| **B: Unified templates** | Single template, write as runtime-appropriate file | No duplication | Need runtime detection |
| **C: Plugin injection** | Plugin injects role context via `chat.message` hook | Dynamic, no files | More complex plugin |

**Recommended**: Option B - Detect runtime, write to `CLAUDE.md` or `OPENCODE.md`.

**Tasks**:
- [ ] Add runtime detection to template rendering
- [ ] Create `OPENCODE.md` during workspace setup (rig creation, crew add)
- [ ] Verify plugin can read role from `OPENCODE.md` or environment

### 3. Formula/Specialized Agent Support

**Goal**: Use formulas (`-F`) to prompt specialized agents through OpenCode.

**Questions to Answer**:
- How do we pass formulas to OpenCode? (CLI flag? Environment variable?)
- How does OpenCode's agent system map to Gastown formulas?
- Can we use OpenCode custom agents for role-specific prompts?

**Proposed Approach**:
```bash
# Claude Code approach
claude -p "You are a Polecat worker..." -F polecat

# OpenCode equivalent options:
# Option A: Custom agent with system prompt in config
opencode --agent polecat

# Option B: Environment variable
GT_FORMULA=polecat opencode

# Option C: Plugin injects prompt on session.created + OPENCODE.md
# (requires Option 2 above)
```

**Implementation**: Update `design/role-permissions.md` with formula strategy.

### 4. Compaction Management

**Goal**: Detect when context is getting full and manage compaction proactively.

**Questions to Answer**:
- How do we know when we're close to needing compaction?
- Does OpenCode expose token count or context usage?
- Should we trigger compaction proactively or reactively?

**Current Understanding**:
- OpenCode fires `session.compacted` after compaction
- We have `experimental.session.compacting` hook for pre-compact
- No known API for token count / context usage

**Proposed Approach**:
1. Monitor message count as proxy for context fullness
2. Use compaction hook to inject critical context
3. Research OpenCode SDK for token usage endpoints

**Tasks**:
- [ ] Research OpenCode API for context/token usage
- [ ] Add message count monitoring to plugin
- [ ] Document compaction strategy in `design/`

### 5. Session Lifecycle Management

**Goal**: Know when to safely start a new session vs continue existing.

**Questions to Answer**:
- When should Polecat start fresh vs resume?
- How do we preserve work context across sessions?
- What's the handoff pattern between sessions?

**Proposed Session Lifecycle**:
```
1. Session Start
   - gt prime → recover context from notes
   - Check for assigned work

2. During Session
   - Monitor for compaction need
   - Preserve key decisions in notes

3. Session End
   - gt done → mark work complete
   - Store critical context for next session

4. Session Restart (new session)
   - Triggered by: compaction, timeout, explicit restart
   - gt prime → full context recovery
```

**Implementation**: Add lifecycle state machine to plugin or document pattern.

### 6. Claude Code Regression Testing

**Goal**: Verify Claude Code hasn't broken after OpenCode integration.

**Tasks**:
- [ ] Run Claude Code in polecat role, verify hook execution
- [ ] Run Claude Code in mayor role, verify interactive mode
- [ ] Compare behavior with and without OpenCode installed
- [ ] Document any differences in `archive/`

---

## Design Decisions Needed

| Decision | Options | Status |
|----------|---------|--------|
| Formula injection | Custom agents vs env var vs plugin | TBD |
| Compaction trigger | Proactive vs reactive | TBD |
| Session restart policy | Time-based vs context-based | TBD |
| Token monitoring | SDK vs proxy | TBD |

---

## Test Scripts

| Script | Purpose | Status |
|--------|---------|--------|
| `test-runtime-e2e.sh` | Runtime-agnostic test runner | ✅ Created |
| `test-opencode-polecat-e2e.sh` | Polecat lifecycle | ✅ Exists |
| `test-opencode-mayor-e2e.sh` | Mayor workflow | ✅ Exists |
| `test-opencode-formula-e2e.sh` | Formula/specialized agent | 📋 TODO |
| `test-opencode-compaction-e2e.sh` | Compaction behavior | 📋 TODO |
| `test-claude-regression.sh` | Claude Code regression | 📋 TODO |

---

## Quick Reference

| Need | Document |
|------|----------|
| **Understand OpenCode integration** | `docs/opencode/README.md` |
| **Work on plugin** | `design/gastown-plugin.md` + `internal/opencode/plugin/gastown.js` |
| **Understand events/hooks** | `reference/events.md` |
| **Run tests** | `scripts/test-runtime-e2e.sh`, `scripts/test-opencode-*.sh` |
| **Check docs** | `scripts/validate-opencode-docs.sh` |
| **Log changes** | `HISTORY.md` |

---

## For New Agents

If you're a new agent picking up this work:

1. **Read**: `docs/opencode/README.md` for overview
2. **Check**: `HISTORY.md` for recent changes
3. **Run**: `./scripts/validate-opencode-docs.sh` to find any doc issues
4. **Test**: `./scripts/test-runtime-e2e.sh --runtime opencode` to verify integration
5. **Continue**: Work on the "Immediate Next Steps" above
6. **Log**: Add entries to `HISTORY.md` when you make changes

**Implementation is at**: `internal/opencode/` (plugin, tests)
**Plans are at**: `docs/opencode/design/` (roadmaps, strategies)
