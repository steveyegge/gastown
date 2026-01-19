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

### 1. Complete E2E Test Suite

**Goal**: Comprehensive real-world testing in temp folders for both runtimes.

**Tasks**:
- [ ] Run `./scripts/test-runtime-e2e.sh --runtime both` and fix issues
- [ ] Run existing OpenCode E2E tests and verify PASS
  - `./scripts/test-opencode-polecat-e2e.sh`
  - `./scripts/test-opencode-mayor-e2e.sh`
- [ ] Create Claude Code equivalent tests (or verify existing work)
- [ ] Test mixed primary/secondary runtime scenarios

**Key Files**:
- `scripts/test-runtime-e2e.sh` - Runtime-agnostic test runner
- `scripts/test-opencode-*.sh` - OpenCode-specific E2E tests
- `internal/opencode/integration_test.go` - Go unit tests

### 2. Claude Code Regression Testing

**Goal**: Verify Claude Code hasn't broken after OpenCode integration.

**Tasks**:
- [ ] Run Claude Code in polecat role, verify hook execution
- [ ] Run Claude Code in mayor role, verify interactive mode
- [ ] Compare behavior with and without OpenCode installed
- [ ] Document any differences in `archive/`

**Key Files**:
- `internal/claude/settings.go` - Claude Code settings management
- `internal/config/agents.go` - Agent preset definitions

### 3. Plugin Verification

**Goal**: Ensure gastown.js plugin works correctly in all scenarios.

**Tasks**:
- [ ] Verify `gt prime` works via plugin
- [ ] Verify `gt mail check --inject` for autonomous roles
- [ ] Verify `experimental.session.compacting` hook
- [ ] Test with different OpenCode versions

**Key Files**:
- `internal/opencode/plugin/gastown.js` - Plugin implementation
- `docs/opencode/design/gastown-plugin.md` - Plugin design doc

---

## Future Work

### Phase 2: SDK Orchestration

See `design/phase2/` for detailed plans.

**Goals**:
- Replace TUI polling with SDK event subscription
- Enable programmatic session management
- Support multi-session workflows

### Template-Based Permissions

See `design/role-permissions.md`.

**Goals**:
- Initialize `.opencode/` with role-based permissions during `gt init`
- Mirror Claude Code's template-based settings pattern

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
