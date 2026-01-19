# Opencode Integration: Decision Points & Open Questions

> Quick reference for decisions needed and questions to answer
> 
> **Purpose**: Track uncertain areas requiring input, research, or experimentation
> **Related**: [orchestration.md](orchestration.md), [../research/experiments.md](../research/experiments.md)
> **Status**: Living Document
> **Updated**: 2026-01-15

## Critical Decisions (Block Implementation)

### D1: Orchestrator Selection Strategy

**Question**: How should Gastown choose between Claude and Opencode backends?

**Options**:

**A. Implicit from Agent Name** (Recommended)
- `agent: "claude"` → Use Claude backend
- `agent: "opencode"` → Use Opencode backend
- Clean, intuitive for users
- No new concepts to learn

**B. Explicit Backend Field**
```json
{
  "backend": "opencode",
  "agent": "opencode"
}
```
- More flexible (could use claude command with opencode backend?)
- More explicit, less magic
- Adds complexity

**C. Runtime Provider**
- Use existing `runtime.provider` field
- Already established pattern
- No new config needed

**Recommendation**: **Option A** (implicit) with **Option C** (provider) as implementation detail

**Decision Owner**: Team Lead
**Decision Deadline**: Before Phase 1 implementation
**Status**: ⬜ Pending

---

### D2: Session Registry Storage

**Question**: Where should session state be stored?

**Options**:

**A. In-Memory Only**
- Fast, simple
- Lost on restart
- Need to rebuild on `gt agents`

**B. File-Based (`.runtime/sessions.json`)**
- Persists across restarts
- Single source of truth per host
- Could grow large

**C. Beads Ledger**
- Distributed, git-backed
- Visible across hosts
- Heavier weight

**D. Hybrid (Memory + Periodic Sync to File)**
- Fast reads, durable writes
- Complexity in sync logic
- Best of both worlds?

**Recommendation**: **Option B** (file-based) for simplicity, with **Option D** (hybrid) for scale

**Decision Owner**: Architect
**Decision Deadline**: Before Phase 2 implementation
**Status**: ⬜ Pending

---

### D3: Remote Execution Architecture

**Question**: How should Gastown orchestrate remote Opencode sessions?

**Options**:

**A. SSH Push Model**
- Gastown SSHs to remote and spawns `opencode`
- Monitors via SSH polling
- Simple, no remote agent needed
- Higher latency, more SSH overhead

**B. Pull Model (Remote Agent)**
- Remote agent pulls work from central queue
- Spawns local Opencode
- Reports back via Beads commits
- More complex, but scales better

**C. API-Based (If Opencode Has API)**
- Use Opencode's native remote API
- Cleanest if available
- Depends on EXP-012 findings

**Recommendation**: **Option C** if available, else **Option B** for production, **Option A** for prototyping

**Decision Owner**: Tech Lead
**Decision Deadline**: After EXP-012 (Remote API Discovery)
**Status**: ⬜ Blocked (waiting on experiment)

---

### D4: Plugin Installation Strategy

**Question**: How should the Gastown plugin be installed in Opencode workspaces?

**Options**:

**A. Auto-Install on Session Spawn**
- `gt sling` automatically copies plugin
- Seamless user experience
- Risk of version drift

**B. Manual Installation Required**
- User runs `gt opencode init` once per workspace
- More explicit, less magic
- Extra setup step

**C. Global Plugin Installation**
- Install once in `~/.opencode/plugins/`
- All sessions use same version
- Requires Opencode to support global plugins

**Recommendation**: **Option A** (auto-install) with **Option C** (global) if supported

**Decision Owner**: UX Lead
**Decision Deadline**: Before Phase 1 implementation
**Status**: ⬜ Pending

---

### D5: Backward Compatibility Guarantee

**Question**: Must existing Claude-based workflows continue to work unchanged?

**Options**:

**A. Full Backward Compatibility**
- No breaking changes to existing configs
- Claude remains default
- Opencode is opt-in only

**B. Soft Migration**
- Deprecate some Claude-specific patterns
- Provide migration guide
- Both work but with warnings

**C. Breaking Change**
- Require config updates
- Cleaner architecture
- Painful migration

**Recommendation**: **Option A** (full compatibility)

**Decision Owner**: Product Manager
**Decision Deadline**: Now (before any implementation)
**Status**: ✅ **Decided: Option A**

---

## Open Questions (Research Needed)

### Q1: Opencode Event Lifecycle 🔴 Critical

**Question**: What events does the Opencode plugin system provide?

**Why It Matters**: Need equivalent of Claude's SessionStart and Compaction hooks

**Research Method**: EXP-010 (Plugin Event Catalog)

**Interim Assumption**: At minimum `session.created` exists (observed in current code)

**Risk if Unknown**: May not have hooks for cleanup or advanced lifecycle management

**Status**: 🔍 **Needs Research** (EXP-010)

---

### Q2: Opencode Session Resume 🟡 Important

**Question**: Can Opencode sessions be resumed like `claude --resume <session-id>`?

**Why It Matters**: Affects restart/recovery workflows

**Research Method**: EXP-007 (Session Resume)

**Interim Assumption**: Assume no resume support initially

**Fallback Plan**: Each spawn is a fresh session (acceptable but less ideal)

**Status**: 🔍 **Needs Research** (EXP-007)

---

### Q3: Opencode Resource Limits 🟡 Important

**Question**: How many concurrent Opencode sessions can run on one host?

**Why It Matters**: Affects pooling strategy and documentation

**Research Method**: EXP-009 (Resource Limits)

**Interim Assumption**: 10-15 sessions safe, 20-25 possible

**Fallback Plan**: Set conservative hard limit (10)

**Status**: 🔍 **Needs Research** (EXP-009)

---

### Q4: Opencode Remote API 🔴 Critical

**Question**: Does Opencode provide a remote API for session management?

**Why It Matters**: Affects remote orchestration architecture (D3)

**Research Method**: EXP-012 (Remote API Discovery)

**Interim Assumption**: No API, use SSH

**Fallback Plan**: Implement SSH-based remote executor

**Status**: 🔍 **Needs Research** (EXP-012)

---

### Q5: Plugin State Persistence 🔴 Critical

**Question**: Can Opencode plugins persist state across session restarts?

**Why It Matters**: Needed for tracking session metadata, work history

**Research Method**: EXP-011 (Plugin State Persistence)

**Interim Assumption**: Filesystem writes work

**Fallback Plan**: Store state in Gastown's `.runtime/` instead of plugin

**Status**: 🔍 **Needs Research** (EXP-011)

---

### Q6: Cross-Session Communication Latency 🟡 Important

**Question**: What's the latency for Beads mail between Opencode sessions?

**Why It Matters**: Affects real-time coordination patterns

**Research Method**: EXP-008 (Cross-Session Messaging)

**Interim Assumption**: < 1 second for local sessions

**Fallback Plan**: Document latency, suggest batching for performance

**Status**: 🔍 **Needs Research** (EXP-008)

---

### Q7: Opencode Headless Mode 🟡 Important

**Question**: Can Opencode run non-interactively (headless) in CI/scripts?

**Why It Matters**: Affects CI integration and automation workflows

**Research Method**: Manual testing + docs review

**Interim Assumption**: Yes (most modern CLIs support this)

**Fallback Plan**: Require TTY for Opencode (limit automation)

**Status**: 🔍 **Needs Research**

---

### Q8: Opencode Security Model 🟢 Nice-to-Have

**Question**: What permission model does Opencode use?

**Why It Matters**: Affects autonomous operation (like `--dangerously-skip-permissions` for Claude)

**Research Method**: Docs review + testing

**Interim Assumption**: Similar to Claude (prompt-based permissions)

**Fallback Plan**: Document required flags/config

**Status**: 🔍 **Needs Research** (low priority)

---

### Q9: Opencode MCP Integration 🟢 Nice-to-Have

**Question**: Does Opencode support MCP (Model Context Protocol) natively?

**Why It Matters**: Future integration with other tools/protocols

**Research Method**: Docs review + community research

**Interim Assumption**: Assume yes (modern agent)

**Fallback Plan**: Use Gastown's abstraction layer

**Status**: 🔍 **Needs Research** (low priority)

---

### Q10: Opencode Conversation Export 🟢 Nice-to-Have

**Question**: Can Opencode sessions export conversation history?

**Why It Matters**: Useful for debugging, audit trails, handoffs

**Research Method**: CLI flag discovery + testing

**Interim Assumption**: Some export mechanism exists

**Fallback Plan**: Use plugin to capture messages

**Status**: 🔍 **Needs Research** (low priority)

---

## Areas of Uncertainty (Design Tradeoffs)

### U1: Session ID Format

**Uncertainty**: What format should unified session IDs use?

**Current State**:
- Claude: Uses `CLAUDE_SESSION_ID` (format unknown)
- Opencode: Uses session ID (format TBD from EXP-001)
- Gastown: Uses tmux session names

**Options**:
- Use backend's native ID
- Create Gastown UUID, map to backend ID
- Use tmux session name as ID

**Impact**: Medium (affects registry, commands, logging)

**Resolution Path**: Prototype both, choose based on simplicity

---

### U2: Work Unit Format

**Uncertainty**: Should work units be backend-agnostic or backend-specific?

**Current State**:
- Beads mail is universal
- Issue/bead IDs are universal
- Execution details vary by backend

**Options**:
- Generic work unit (backend translates)
- Backend-specific work units
- Hybrid (common core + backend extensions)

**Impact**: Medium (affects API, testing)

**Resolution Path**: Start generic, add backend-specific as needed

---

### U3: Error Handling Strategy

**Uncertainty**: How to handle backend-specific errors consistently?

**Current State**:
- Claude errors surface directly
- Each backend has different failure modes

**Options**:
- Error translation layer (map to generic errors)
- Pass-through (expose backend-specific errors)
- Hybrid (map common, pass-through rare)

**Impact**: Low (affects user experience)

**Resolution Path**: Start with pass-through, translate common errors

---

### U4: Performance Optimization Priority

**Uncertainty**: Which performance characteristics matter most?

**Tradeoffs**:
- Spawn latency vs. Memory usage
- Message latency vs. Reliability
- State sync frequency vs. I/O overhead

**Current State**: Not measured

**Resolution Path**: Profile during Phase 1, optimize based on data

---

### U5: Configuration Complexity

**Uncertainty**: How much configuration should be exposed to users?

**Current State**:
- Claude has many knobs (hooks, args, etc.)
- Risk of option overload

**Options**:
- Minimal (sensible defaults, few overrides)
- Maximal (expose everything)
- Progressive (simple start, advanced for power users)

**Impact**: High (affects usability)

**Resolution Path**: Progressive disclosure (start minimal)

---

## Assumptions Registry

Track assumptions we're making and their validation status:

| ID | Assumption | Confidence | Validation Method | Status |
|----|------------|-----------|-------------------|--------|
| A1 | Opencode allows multiple concurrent sessions | High | EXP-001 | ⬜ Not Validated |
| A2 | Plugin can access filesystem for state | High | EXP-011 | ⬜ Not Validated |
| A3 | Beads mail works for cross-session messaging | High | EXP-008 | ⬜ Not Validated |
| A4 | SSH-based remote execution is viable | Medium | EXP-006 | ⬜ Not Validated |
| A5 | Opencode session spawn time < 5 seconds | Medium | EXP-001 | ⬜ Not Validated |
| A6 | Plugin events fire reliably | Medium | EXP-002, EXP-010 | ⬜ Not Validated |
| A7 | No port conflicts in multi-session | High | EXP-001 | ⬜ Not Validated |
| A8 | Resource usage scales linearly | Medium | EXP-009 | ⬜ Not Validated |
| A9 | Opencode has session resume | Low | EXP-007 | ⬜ Not Validated |
| A10 | Opencode has remote API | Low | EXP-012 | ⬜ Not Validated |

### Confidence Levels:
- **High**: Based on existing evidence or low risk
- **Medium**: Reasonable assumption but needs validation
- **Low**: Speculative, could easily be wrong

---

## Communication & Divergence Documentation

### Known Divergences (Claude vs Opencode)

| Feature | Claude | Opencode | Workaround Needed? |
|---------|--------|----------|-------------------|
| Hooks | Native settings.json | Plugin system | No (plugin works) |
| Resume | Native `--resume` | TBD | TBD (pending EXP-007) |
| Context File | CLAUDE.md | AGENTS.md | No (both work) |
| Ready Signal | Prompt prefix | TBD | TBD (pending EXP-004) |
| Cleanup | Compaction hook | TBD | TBD (pending EXP-010) |
| Session ID Env | CLAUDE_SESSION_ID | TBD | TBD (pending EXP-001) |

### User-Facing Differences

Document differences users will notice:

**When Using Opencode**:
- Plugin auto-runs `gt prime` (vs Claude hook)
- May not support resume (TBD)
- Different startup time (TBD from experiments)
- Different resource usage (TBD from experiments)

**When Using Claude** (No Changes):
- Everything works as before
- Default choice
- Fully tested

---

## Next Actions

### Immediate (This Week)
- [ ] Review decisions with team
- [ ] Assign experiment owners (EXP-010, EXP-011, EXP-012)
- [ ] Set up Opencode test environment
- [ ] Run blocking experiments

### Short-Term (Next 2 Weeks)
- [ ] Complete all basic experiments (EXP-001 to EXP-005)
- [ ] Make critical decisions (D1-D5)
- [ ] Update architecture based on findings
- [ ] Start Phase 1 implementation

### Medium-Term (Next Month)
- [ ] Complete advanced experiments (EXP-006 to EXP-009)
- [ ] Validate all assumptions
- [ ] Document all divergences
- [ ] Prototype remote execution

---

## Review Schedule

- **Weekly**: Update experiment status
- **Bi-Weekly**: Review open questions and decisions
- **Monthly**: Update architecture docs based on learnings

**Last Reviewed**: 2026-01-15
**Next Review**: 2026-01-22

---

**Last Updated**: 2026-01-15
**Owner**: Gastown Team
**Status**: Active Planning
