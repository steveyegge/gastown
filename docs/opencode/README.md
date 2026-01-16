# Opencode Integration Documentation

> Comprehensive documentation for integrating Opencode as an alternative orchestration layer in Gastown

**Location**: All Opencode integration documentation is centralized in this folder

**Repository Source**: Based on [anomalyco/opencode](https://github.com/anomalyco/opencode) GitHub repository

**Status**: Planning Complete - Ready for Implementation

---

## 📚 Documentation Index

### Quick Start

**[Quickstart Guide](opencode-quickstart.md)** - One-page summary to get oriented quickly

---

### Core Planning & Architecture

**[Concept Analysis](opencode-concept-analysis.md)** - Analysis of all 23 Gastown concepts and Claude Code coupling
- Identifies which concepts need abstraction
- Coupling levels: 13 runtime-agnostic, 2 low, 4 medium, 4 high
- Priority list with effort estimates

**[Integration Architecture](opencode-integration-architecture.md)** - Detailed integration strategy leveraging Opencode features
- Opencode research findings (plugins, SDK, tools, agents)
- Core logic extraction pattern
- Concept-by-concept integration with code examples
- 4-phase implementation roadmap

**[Orchestration Plan](opencode-orchestration.md)** - High-level architecture and strategy
- Executive summary
- Current state vs future state
- Design goals and architecture
- Integration points and workflow patterns

**[Implementation Guide](opencode-implementation-guide.md)** - Technical specifications
- Go interface definitions
- Code structure and organization
- Configuration examples
- Testing strategy

---

### Technical Research

**[Technical Research](technical-research.md)** ⭐ **NEW** - Deep dive into Opencode repository
- Session forking (`session/fork`) from ACP draft spec
- Configuration schema ([https://opencode.ai/config.json](https://opencode.ai/config.json))
- ACP (Agent Client Protocol) integration
- Plugin system internals with code examples
- HTTP API endpoints and usage
- Remote orchestration patterns
- Research gaps and experiments needed

---

### Research & Decision Support

**[Experiments Checklist](opencode-experiments.md)** - 12 validation experiments
- 5 basic experiments (MVP requirements)
- 4 advanced experiments (design improvements)
- 3 blocking experiments (must resolve)
- Procedures and success criteria for each

**[Decisions & Questions](opencode-decisions.md)** - Decision points and open questions
- 5 critical architectural decisions
- 10 open questions requiring research
- Known divergences between Claude and Opencode
- Assumptions registry with validation status

---

### Navigation

**[Index & Reading Paths](opencode-index.md)** - Comprehensive navigation hub
- Reading paths by role (Architects, Engineers, PMs, Users, Researchers)
- Reading paths by goal
- Documentation status tracking
- Implementation timeline

---

## 🎯 Quick Navigation

| Want to... | Read This |
|------------|-----------|
| Get oriented quickly | [Quickstart](opencode-quickstart.md) (5 min) |
| Understand concept coupling | [Concept Analysis](opencode-concept-analysis.md) |
| Learn how Opencode works | [Technical Research](technical-research.md) ⭐ |
| See integration patterns | [Integration Architecture](opencode-integration-architecture.md) |
| Understand the big picture | [Orchestration Plan](opencode-orchestration.md) |
| Get implementation details | [Implementation Guide](opencode-implementation-guide.md) |
| Run experiments | [Experiments Checklist](opencode-experiments.md) |
| Make decisions | [Decisions & Questions](opencode-decisions.md) |
| Navigate everything | [Full Index](opencode-index.md) |

---

## 📊 Documentation Stats

| Document | Size | Lines | Purpose |
|----------|------|-------|---------|
| Concept Analysis | 24KB | 730 | Coupling analysis of all Gastown concepts |
| Integration Architecture | 30KB | 940 | Integration strategy with code examples |
| Technical Research | 21KB | 670 | Deep dive into Opencode repository |
| Orchestration Plan | 20KB | 620 | High-level architecture |
| Implementation Guide | 18KB | 550 | Technical specifications |
| Experiments | 19KB | 600 | Validation experiments |
| Decisions | 14KB | 450 | Decision points and questions |
| Index | 14KB | 320 | Navigation hub |
| Quickstart | 5KB | 150 | One-page summary |

**Total**: 9 documents, ~165KB, 5,030 lines

---

## 🔍 Key Insights

### From Concept Analysis
- **13/23 Gastown concepts already runtime-agnostic**
- **Runtime config abstraction already exists** - just needs adoption
- **Minimal changes required**: ~200-300 LOC in 5 manager files

### From Integration Architecture
- **Core logic extraction pattern**: Extract once, adapt twice (Claude + Opencode)
- **Mail already works cross-runtime**: Same CLI command for both
- **Opencode provides natural integration points**: Plugins, tools, HTTP API

### From Technical Research
- **Session fork exists**: `session/fork` in ACP draft spec (needs verification)
- **Configuration schema**: [https://opencode.ai/config.json](https://opencode.ai/config.json)
- **Plugin events**: `session.created`, `tool.execute.before/after`, `stop`, etc.
- **HTTP API**: RESTful endpoints for remote orchestration
- **Permissions**: Fine-grained with wildcards (`"git *": "allow"`, `"rm *": "deny"`)

---

## 🚀 Implementation Roadmap

### Phase 1: Core Logic Extraction (Week 1)
- Extract `internal/mail/`, `internal/session/`, `internal/work/`
- Create runtime-agnostic interfaces

### Phase 2: Opencode Adapters (Week 2)
- Update plugins: `gastown.js`, `gastown-mail.js`, `gastown-polecat.js`
- Create custom tools: `mail.ts`, `convoy.ts`, `worktree.ts`
- Implement SDK wrapper: `internal/opencode/client.go`

### Phase 3: Unified Interface (Week 3)
- Update 5 agent managers to use runtime config
- Replace `claude.EnsureSettings` with `runtime.EnsureSettingsForRole`

### Phase 4: Testing & Documentation (Week 4)
- Run experiments (EXP-002, EXP-003, EXP-007, EXP-008, EXP-010, EXP-011, EXP-012)
- Integration tests
- User documentation

**Total Effort**: 3-4 weeks

---

## 🔗 External References

### Primary Sources
- [Opencode Repository](https://github.com/anomalyco/opencode) - Source of truth
- [ACP Specification](https://agentclientprotocol.com/) - Agent Client Protocol
- [Configuration Schema](https://opencode.ai/config.json) - Official schema
- [OpenCode Documentation](https://opencode.ai/docs/) - Official docs
- [DeepWiki](https://deepwiki.com/anomalyco/opencode/) - Technical deep dives

### Key Issues & RFCs
- [Issue #7978](https://github.com/anomalyco/opencode/issues/7978) - ACP draft methods (session/fork, session/list)
- [ACP RFC: Session Fork](https://agentclientprotocol.com/rfds/session-fork) - Fork specification
- [Issue #1473](https://github.com/anomalyco/opencode/issues/1473) - Hooks support discussion

---

## 📝 Document Updates

**Latest Changes** (2026-01-16):
- ✅ Reorganized all documentation under `docs/opencode/`
- ✅ Added comprehensive technical research from Opencode repository
- ✅ Documented `session/fork` from ACP draft spec
- ✅ Added configuration schema references
- ✅ Updated all cross-references

**Next Updates**:
- After EXP-007: Update with session fork findings
- After EXP-010: Complete plugin event catalog
- After EXP-011: Document plugin state persistence
- After EXP-012: Add remote orchestration patterns

---

**Last Updated**: 2026-01-16
**Status**: Documentation Complete - Implementation Ready
**Maintainer**: Gastown Team
