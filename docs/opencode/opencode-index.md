# Opencode Orchestration: Documentation Index

> Central hub for all Opencode orchestration planning documentation
> 
> **Status**: Planning Phase
> **Created**: 2026-01-15
> **Last Updated**: 2026-01-15

## Overview

This index provides navigation across all Opencode orchestration planning documents. These documents outline the plan to extend Gastown's orchestration capabilities to support Opencode as an alternative to Claude Code.

## Document Organization

### 📘 Core Planning Documents

#### [Gastown Concepts: Claude Code Coupling Analysis](opencode-concept-analysis.md) ⭐
**Purpose**: Comprehensive analysis of all major Gastown concepts and their Claude Code dependencies

**Use When**:
- Understanding which concepts need abstraction
- Identifying tight coupling to Claude Code
- Planning minimal changes for Opencode support
- Reviewing architecture for multi-runtime support

**Key Sections**:
- Town-Level Concepts (Mayor, Deacon, Dogs)
- Rig-Level Concepts (Polecat, Witness, Refinery, Crew)
- Work Tracking (Beads, Hooks, Convoys, Molecules, Formulas, Wisps)
- Communication (Mail, Nudge, Handoff, Seance)
- Infrastructure (Tmux, Git Worktrees, Runtime Config)
- Coupling Analysis Summary
- Priority List for Abstractions

**Audience**: Architects, engineers, anyone needing to understand the codebase architecture

---

#### [Opencode Integration Architecture](opencode-integration-architecture.md) ⭐ **NEW**
**Purpose**: Detailed Opencode-specific integration strategy leveraging plugins, SDK, tools, and agents

**Use When**:
- Implementing Opencode adapters
- Understanding how to leverage Opencode's full capabilities
- Designing core logic extraction
- Planning plugin/tool development

**Key Sections**:
- Opencode Architecture Research (plugins, SDK, agents, tools)
- Core Logic Extraction Pattern
- Concept-by-Concept Integration (with code examples)
- Multi-Session Orchestration
- Configuration Management
- Implementation Roadmap (4 phases)
- Design Principles

**Audience**: Engineers implementing Opencode integration, architects designing abstractions

---

#### [Opencode Orchestration Plan](opencode-orchestration.md)
**Purpose**: Comprehensive strategy and architecture overview

**Use When**: 
- Understanding the overall vision
- Getting context on current state vs future state
- Reviewing high-level architecture
- Understanding risk assessment

**Key Sections**:
- Executive Summary
- Current State Analysis
- Design Goals & Architecture
- Integration Points
- Workflow Patterns
- Migration & Compatibility
- Implementation Phases

**Audience**: All stakeholders, architects, product managers

---

#### [Opencode Implementation Guide](opencode-implementation-guide.md)
**Purpose**: Detailed technical implementation specifications

**Use When**:
- Writing code for Opencode integration
- Understanding code structure and interfaces
- Looking for configuration examples
- Planning testing strategy

**Key Sections**:
- Code Structure (new packages)
- Interface Definitions
- Backend Implementations
- Integration Points
- Configuration Examples
- Testing Strategy
- Error Handling
- Performance Considerations

**Audience**: Engineers, technical leads

---

#### [Stage 2: SDK-Based Orchestration Layer](stage2-sdk-orchestration.md) 🆕
**Purpose**: Future architecture plan using OpenCode Go SDK for programmatic orchestration

**Use When**:
- Planning next-phase OpenCode integration
- Understanding SDK vs CLI trade-offs
- Designing the Orchestrator interface
- Evaluating vendor lock-in risks

**Key Sections**:
- Current Architecture Limitations
- Proposed SDK Architecture
- Orchestrator Interface Design
- Implementation Phases (4 phases, 2-3 weeks)
- Benefits vs Risks Analysis
- Decision Points

**Audience**: Architects, engineers planning Stage 2 work

---

### 🔬 Experimentation & Research

#### [Opencode Experiments Checklist](opencode-experiments.md)
**Purpose**: Hands-on validation of assumptions and unknowns

**Use When**:
- Planning experimental work
- Validating architecture assumptions
- Need to answer specific questions
- Tracking experiment status

**Key Sections**:
- Basic Experiments (MVP requirements)
- Advanced Experiments (design improvements)
- Blocking Experiments (must resolve before proceeding)
- Experiment tracking and results templates

**Audience**: Researchers, engineers, tech leads

---

#### [Decision Points & Open Questions](opencode-decisions.md)
**Purpose**: Track decisions needed and uncertain areas

**Use When**:
- Making architectural decisions
- Reviewing open questions
- Understanding assumptions
- Prioritizing research work

**Key Sections**:
- Critical Decisions (block implementation)
- Open Questions (need research)
- Areas of Uncertainty (design tradeoffs)
- Assumptions Registry
- Known Divergences (Claude vs Opencode)

**Audience**: Decision makers, architects, project managers

---

## Quick Links by Role

### 👔 For Project Managers

**Start Here**:
1. [Orchestration Plan - Executive Summary](opencode-orchestration.md#executive-summary)
2. [Orchestration Plan - Implementation Phases](opencode-orchestration.md#implementation-phases)
3. [Decisions - Critical Decisions](opencode-decisions.md#critical-decisions-block-implementation)
4. [Orchestration Plan - Risk Assessment](opencode-orchestration.md#risk-assessment)

**Key Questions You Can Answer**:
- What's the timeline?
- What are the risks?
- What decisions need to be made?
- What's blocking progress?

---

**For Architects**

**Start Here**:
1. [Concept Analysis - Overview](opencode-concept-analysis.md#overview)
2. [Integration Architecture - Research Findings](opencode-integration-architecture.md#research-findings) ⭐ **NEW**
3. [Integration Architecture - Integration Strategy](opencode-integration-architecture.md#integration-strategy-core-logic-extraction) ⭐ **NEW**
4. [Concept Analysis - Coupling Summary](opencode-concept-analysis.md#summary-coupling-analysis)
5. [Implementation Guide - Interface Definitions](opencode-implementation-guide.md#interface-definitions)

**Key Questions You Can Answer**:
- Which Gastown concepts need abstraction? (Concept Analysis)
- How does Opencode work? Plugins, SDK, agents? (Integration Architecture - Research)
- How to leverage Opencode features for each concept? (Integration Architecture - Concept Integration)
- What's the core logic extraction pattern? (Integration Architecture - Principles)
- How tightly coupled is each concept to Claude Code? (Concept Analysis)
- What are the minimal changes needed? (Concept Analysis - Priority List)

---

### 💻 For Engineers

**Start Here**:
1. [Implementation Guide - Code Structure](opencode-implementation-guide.md#code-structure)
2. [Implementation Guide - Backend Implementations](opencode-implementation-guide.md#backend-implementations)
3. [Implementation Guide - Configuration Examples](opencode-implementation-guide.md#configuration-examples)
4. [Experiments - Basic Experiments](opencode-experiments.md#basic-experiments-required-for-mvp)

**Key Questions You Can Answer**:
- What code do I need to write?
- What interfaces should I implement?
- How do I test my changes?
- What experiments do I need to run?

---

### 🔬 For Researchers

**Start Here**:
1. [Experiments - Blocking Experiments](opencode-experiments.md#blocking-experiments-must-resolve)
2. [Decisions - Open Questions](opencode-decisions.md#open-questions-research-needed)
3. [Decisions - Assumptions Registry](opencode-decisions.md#assumptions-registry)
4. [Experiments - Experiment Tracking](opencode-experiments.md#experiment-tracking)

**Key Questions You Can Answer**:
- What needs to be validated?
- What experiments should I run?
- What are we assuming?
- What's the priority?

---

### 🎨 For UX/Product

**Start Here**:
1. [Orchestration Plan - Workflow Patterns](opencode-orchestration.md#workflow-patterns)
2. [Orchestration Plan - Migration & Compatibility](opencode-orchestration.md#migration--compatibility)
3. [Decisions - D4: Plugin Installation Strategy](opencode-decisions.md#d4-plugin-installation-strategy)
4. [Orchestration Plan - Success Criteria](opencode-orchestration.md#success-criteria)

**Key Questions You Can Answer**:
- How will users interact with this?
- What workflows are supported?
- What's the migration path?
- What's the user experience?

---

## Reading Paths by Goal

### 🎯 "I need to understand the big picture"

1. [Concept Analysis - Overview](opencode-concept-analysis.md#overview) (5 min) ⭐ **START HERE**
2. [Concept Analysis - Summary](opencode-concept-analysis.md#summary-coupling-analysis) (10 min) ⭐
3. [Orchestration Plan - Executive Summary](opencode-orchestration.md#executive-summary) (5 min)
4. [Orchestration Plan - Architecture Overview](opencode-orchestration.md#architecture-overview) (10 min)
5. [Decisions - Known Divergences](opencode-decisions.md#known-divergences-claude-vs-opencode) (5 min)

**Total Time**: ~35 minutes

---

### 🎯 "I need to start implementing"

1. [Implementation Guide - Quick Reference](opencode-implementation-guide.md#quick-reference) (5 min)
2. [Implementation Guide - Interface Definitions](opencode-implementation-guide.md#interface-definitions) (15 min)
3. [Implementation Guide - Backend Implementations](opencode-implementation-guide.md#backend-implementations) (20 min)
4. [Implementation Guide - Testing Strategy](opencode-implementation-guide.md#testing-strategy) (10 min)

**Total Time**: ~50 minutes

---

### 🎯 "I need to validate assumptions"

1. [Decisions - Assumptions Registry](opencode-decisions.md#assumptions-registry) (5 min)
2. [Experiments - Blocking Experiments](opencode-experiments.md#blocking-experiments-must-resolve) (10 min)
3. [Experiments - Basic Experiments](opencode-experiments.md#basic-experiments-required-for-mvp) (15 min)
4. [Experiments - Experiment Tracking](opencode-experiments.md#experiment-tracking) (5 min)

**Total Time**: ~35 minutes

---

### 🎯 "I need to make decisions"

1. [Decisions - Critical Decisions](opencode-decisions.md#critical-decisions-block-implementation) (15 min)
2. [Decisions - Open Questions](opencode-decisions.md#open-questions-research-needed) (10 min)
3. [Orchestration Plan - Risk Assessment](opencode-orchestration.md#risk-assessment) (10 min)
4. [Decisions - Areas of Uncertainty](opencode-decisions.md#areas-of-uncertainty-design-tradeoffs) (10 min)

**Total Time**: ~45 minutes

---

## Status Summary

### Documentation Status

| Document | Status | Completeness | Next Update |
|----------|--------|--------------|-------------|
| [Concept Analysis](opencode-concept-analysis.md) | ✅ Complete | 100% | After adoption |
| [Integration Architecture](opencode-integration-architecture.md) | ✅ Complete | 100% | After Phase 1 |
| [Orchestration Plan](opencode-orchestration.md) | ✅ Complete | 100% | After experiments |
| [Implementation Guide](opencode-implementation-guide.md) | ✅ Complete | 100% | After Phase 1 |
| [Experiments Checklist](opencode-experiments.md) | ✅ Complete | 100% | As experiments run |
| [Decisions & Questions](opencode-decisions.md) | ✅ Complete | 100% | Weekly |

### Implementation Status

| Phase | Status | Target Date |
|-------|--------|-------------|
| Planning | ✅ Complete | 2026-01-15 |
| Experimentation | 🔄 In Progress | 2026-01-31 |
| Phase 1: Orchestrator Abstraction | ⬜ Not Started | 2026-02-15 |
| Phase 2: Multi-Session Support | ⬜ Not Started | 2026-03-01 |
| Phase 3: Remote Support | ⬜ Not Started | 2026-03-15 |
| Phase 4: Production Hardening | ⬜ Not Started | 2026-04-01 |

### Critical Path Items

**Blocking Now**:
- [ ] EXP-010: Plugin Event Catalog
- [ ] EXP-011: Plugin State Persistence
- [ ] EXP-012: Remote API Discovery
- [ ] D1: Orchestrator Selection Strategy
- [ ] D2: Session Registry Storage

**Blocking Phase 1**:
- [ ] D4: Plugin Installation Strategy
- [ ] D5: Backward Compatibility Guarantee ✅ **Decided**

**Blocking Phase 2**:
- [ ] EXP-001: Multi-Session Spawn
- [ ] EXP-009: Resource Limits

**Blocking Phase 3**:
- [ ] D3: Remote Execution Architecture
- [ ] EXP-006: Remote Session Creation

---

## Conventions & Standards

### Document Naming

- **Pattern**: `opencode-{topic}.md`
- **Topics**: `orchestration`, `implementation-guide`, `experiments`, `decisions`
- **Location**: `docs/design/`

### Cross-References

Use relative links for cross-document references:
```markdown
[Orchestration Plan](opencode-orchestration.md)
[Specific Section](opencode-orchestration.md#section-name)
```

### Status Icons

- ✅ Complete / Decided
- 🔄 In Progress
- ⬜ Not Started / Pending
- ❌ Failed / Blocked
- ⚠️ Warning / Risk
- 🔍 Needs Research
- 🟢 Low Priority
- 🟡 Medium Priority
- 🔴 High Priority / Blocking

### Update Frequency

- **Planning Docs**: After major milestones or decisions
- **Experiments**: After each experiment completion
- **Decisions**: Weekly during active development
- **Index**: When new docs added or major status changes

---

## Related Documentation

### Gastown Core Docs

- [Architecture](architecture.md) - Gastown's core architecture
- [Plugin System](plugin-system.md) - Plugin design principles
- [Federation](federation.md) - Multi-workspace coordination
- [Operational State](operational-state.md) - State management

### External Resources

- [Opencode Repository](https://github.com/anomalyco/opencode)
- [oh-my-opencode](https://github.com/code-yeongyu/oh-my-opencode)

### Code Locations

- `internal/runtime/` - Runtime abstraction layer
- `internal/config/` - Configuration types
- `internal/opencode/` - Opencode integration (existing)
- `internal/orchestration/` - Orchestration layer (to be created)

---

## Contact & Ownership

**Document Owner**: Gastown Team
**Maintainer**: TBD (assign after planning approval)
**Review Cadence**: Weekly during active development

**Questions?**
- Architecture: Review [Orchestration Plan](opencode-orchestration.md)
- Implementation: Review [Implementation Guide](opencode-implementation-guide.md)
- Experiments: Review [Experiments Checklist](opencode-experiments.md)
- Decisions: Review [Decisions & Questions](opencode-decisions.md)

---

## Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-01-15 | Initial documentation set created | Gastown Team |

---

**Last Updated**: 2026-01-15
**Status**: Living Document
**Next Review**: 2026-01-22
