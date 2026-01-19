# Opencode Orchestration: Quick Start

> One-page summary to get started with the Opencode orchestration plan
> 
> **Full Documentation**: See [opencode-index.md](opencode-index.md)

## What Is This?

A plan to extend Gastown's orchestration capabilities to support **Opencode** as an alternative to Claude Code. This enables:
- Running multiple concurrent Opencode sessions
- Orchestrating work across Opencode agents
- Remote Opencode execution (CI, distributed workers)
- Mixed Claude + Opencode workflows

## Current Status

✅ **Planning Complete** (2026-01-15)
- Comprehensive documentation created
- Architecture defined
- Experiments identified
- Ready to begin implementation

## For Decision Makers

**Read First**: [Decisions - Critical Decisions](opencode-decisions.md#critical-decisions-block-implementation)

**5 Decisions Needed**:
1. How to choose between Claude/Opencode backends? → Option A: Implicit from agent name
2. Where to store session state? → Option B: File-based
3. How to orchestrate remote sessions? → Pending experiment results
4. How to install Opencode plugin? → Option A: Auto-install
5. Backward compatibility? → ✅ **Decided**: Full compatibility (Option A)

**Timeline**: ~8 weeks (4 phases)

**Risk Level**: Medium (manageable with proper experimentation)

## For Architects

**Read First**: [Concept Analysis](opencode-concept-analysis.md) ⭐ **NEW - START HERE**

**What You'll Learn**:
- Which Gastown concepts are already runtime-agnostic (most are!)
- Which concepts need minimal abstraction (5 manager files)
- Coupling levels: 🟢 None (13 concepts) / 🟡 Low (2 concepts) / 🟠 Medium (4 concepts) / 🔴 High (4 concepts)
- Priority list: Use runtime config everywhere (~200-300 LOC changes)

**Then Read**:
- [Implementation Guide](opencode-implementation-guide.md#quick-reference) - Technical specs
- [Orchestration Plan](opencode-orchestration.md) - Overall strategy

**Estimated Work**: 1-2 weeks for core implementation

## For Engineers

**Read First**: [Concept Analysis - Priority List](opencode-concept-analysis.md#required-abstractions-priority-list)

**What You'll Build**:
```
internal/orchestration/           # New package
├── orchestrator.go              # Core interface
├── claude/orchestrator.go       # Claude backend
└── opencode/orchestrator.go     # Opencode backend
```

**Key Interface**:
```go
type Orchestrator interface {
    SpawnSession(ctx, cfg) (SessionID, error)
    AssignWork(ctx, id, work) error
    ListSessions(ctx) ([]*SessionInfo, error)
    // ... more methods
}
```

**Testing**: Unit tests + 12 experiments to validate assumptions

## For Researchers

**Read First**: [Experiments Checklist](opencode-experiments.md#experiment-tracking)

**Priority Experiments** (Run These First):
- 🔴 EXP-010: Plugin Event Catalog (BLOCKING)
- 🔴 EXP-011: Plugin State Persistence (BLOCKING)
- 🔴 EXP-012: Remote API Discovery (BLOCKING)

**Then**:
- 🟢 EXP-001 to EXP-005: Basic functionality validation
- 🟡 EXP-006 to EXP-009: Advanced features

## Quick Navigation

| Want to... | Read This |
|------------|-----------|
| Understand which concepts need changes | [Concept Analysis](opencode-concept-analysis.md) |
| Learn how Opencode works (plugins, SDK, agents) | [Integration Architecture](opencode-integration-architecture.md) ⭐ **NEW** |
| See core logic extraction patterns | [Integration Architecture - Principles](opencode-integration-architecture.md#key-design-principles) ⭐ **NEW** |
| See the big picture | [Orchestration Plan](opencode-orchestration.md) |
| See code structure | [Implementation Guide](opencode-implementation-guide.md) |
| Run experiments | [Experiments Checklist](opencode-experiments.md) |
| Make decisions | [Decisions & Questions](opencode-decisions.md) |
| Find any doc | [Index](opencode-index.md) |

## Next Steps

### This Week
1. [ ] Review and approve documentation
2. [ ] Assign experiment owners
3. [ ] Run blocking experiments (EXP-010, EXP-011, EXP-012)
4. [ ] Make critical decisions (D1-D4)

### Next 2 Weeks
1. [ ] Complete basic experiments
2. [ ] Update architecture based on findings
3. [ ] Begin Phase 1 implementation

### Next Month
1. [ ] Complete Phase 1-2 (orchestration abstraction + multi-session)
2. [ ] Begin remote support prototyping
3. [ ] Gather user feedback

## Key Contacts

- **Documentation Owner**: Gastown Team
- **Architecture Questions**: See [Orchestration Plan](opencode-orchestration.md)
- **Implementation Questions**: See [Implementation Guide](opencode-implementation-guide.md)
- **Experiment Questions**: See [Experiments Checklist](opencode-experiments.md)

## Success Metrics

**MVP (Phase 1-2)**:
- [ ] Spawn multiple Opencode sessions
- [ ] Assign work via `gt sling`
- [ ] Track sessions in `gt agents`
- [ ] Basic cross-session messaging

**Full Feature (Phase 3-4)**:
- [ ] Remote session support
- [ ] Mixed Claude + Opencode workflows
- [ ] Production-ready performance
- [ ] Complete documentation

---

**Last Updated**: 2026-01-15
**Status**: Ready to Begin
**Full Docs**: [opencode-index.md](opencode-index.md)
