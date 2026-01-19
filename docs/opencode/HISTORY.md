# OpenCode Integration History

> **Purpose**: Chronological log of integration work, decisions, and documentation updates  
> **Format**: Newest entries at top

---

## 2026-01-19

### Documentation Reorganization

Reorganized the entire `docs/opencode/` structure for clarity and discoverability.

**Reference Directory** (stable docs):
- Created `configuration.md` - Config, models, permissions, MCP, tools
- Created `integration-guide.md` - CLI, SDK, API, Plugin integration (one-stop shop)
- Renamed `api-reference.md` → `external-resources.md` - Links to official docs
- Created `events.md` - Event types reference with GitHub source link
- Rewrote `quickstart.md` - Practical getting-started guide

**Moved to Design** (formerly `planning/`):
- `plugin-guide.md` → `design/gastown-plugin.md` - Gastown-specific implementation plan
- `maintenance.md` → `design/maintenance.md` - Version compatibility, update procedures

**Moved to Archive**:
- `integration-summary.md` - Point-in-time status report (dated 2026-01-17)

**Key Decisions**:
- Events vs Hooks clarification: Events are read-only notifications; hooks can modify behavior
- Reference docs should be evergreen; time-sensitive content goes to archive
- Integration-guide is the one-stop shop for all integration methods

### Agent Feature Matrix Consolidation

Extracted stable runtime comparison data from OpenCode research into `docs/agent-features.md`.

**Updates to agent-features.md**:
- Reorganized into Quick Reference, Gastown Integration, SDK Comparison sections
- Added SDK language support comparison (TS/Python/Go across all runtimes)
- Clarified MCP tool search (only Claude Code has auto lazy loading)
- Added session fork SDK support for OpenCode
- Created events.md and linked as authoritative source
- Added thorough verification guidelines for updating the table

**New Requirements**:
- Added agent instruction to README requiring all OpenCode changes be logged to HISTORY.md
- Created HISTORY.md for chronological tracking of integration work

### Role-Based Permissions Planning

Created planning document for role-based permissions across agent runtimes.

**Documents Created**:
- `design/role-permissions.md` - Permission profiles by role type

**Key Analysis**:
- OpenCode supports granular allow/ask/deny permissions via JSON config
- Can define custom agents in `.opencode/config.jsonc` with role-specific profiles
- Plugin `permission.ask` hook could dynamically handle permissions based on GT_ROLE
- **Recommended**: Mirror Claude Code pattern with template-based `.opencode/` init

**Updates to agent-features.md**:
- Renamed "Permissions" to "Permission modes" with per-action control
- Added "Role-based agents" row for custom permission profiles

### Tools Reference

Created comprehensive tools reference documenting all 14 built-in tools.

**Documents Created**:
- `reference/tools.md` - Complete tools reference

**Content**:
- All built-in tools with permissions and descriptions
- 3 methods for custom tools: Plugin, Config, MCP
- Comparison matrix of custom tool approaches
- Permission wildcard patterns
- Source code references

### Customization Reference

Created comprehensive customization guide for extending OpenCode.

**Documents Created**:
- `reference/customization.md` - Agents, commands, skills, MCP

**Content**:
- Custom agents with different model/permission profiles
- Custom slash commands with arguments
- Skills (SKILL.md) for reusable instructions
- MCP servers - clarified when to use (external services only, not files)
- How OpenCode MCP config differs from Claude Code/Gemini
- Configuration precedence

### Events Reference Enhancement

Updated events.md with schema links and property details.

**Improvements**:
- Added direct link to types.gen.ts for type definitions
- Added common event structure documentation
- Added key properties for each event type
- Added example payloads for session.created, message.updated, tool.execute.after
- Added filtering tips (e.g., checking `role === "user"`)

### Best Practices Guide

Created performance and context optimization guide.

**Documents Created**:
- `reference/best-practices.md` - Performance, tool management, recommendations

**Content**:
- Tool count management strategies (agent profiles, limiting MCP per agent)
- Context optimization (short sessions, minimal injection, skills)
- Recommended MCP servers (evidence-based: github, postgres, memory, brave-search)
- MCP servers to avoid (filesystem redundant, heavy servers)
- Plugin performance patterns (non-blocking event handlers)
- Gastown role-based tool recommendations
- Performance checklist

### Directory Rename: planning/ → design/

Renamed `planning/` directory to `design/` for better semantic clarity.

**Rationale**:
- "planning" is a verb/action, "design" is a noun
- Contents are design documents, roadmaps, and implementation strategies
- Created `design/README.md` with maintenance notice

### Documentation Validation System

Created validation infrastructure for maintaining doc quality.

**Documents Created**:
- `CONTRIBUTING.md` - Standards, review triggers, agent checklists

**Scripts Created**:
- `scripts/validate-opencode-docs.sh` - Automated validation

**Checks**:
- Broken internal links
- Stale docs (code newer than docs)
- HISTORY.md freshness
- README inventory completeness
- Frontmatter requirements

**Usage**:
```bash
./scripts/validate-opencode-docs.sh           # Full validation
./scripts/validate-opencode-docs.sh --quick   # Pre-commit
```

### Runtime-Agnostic E2E Test Runner

Created unified E2E test runner for both Claude Code and OpenCode runtimes.

**Scripts Created**:
- `scripts/test-runtime-e2e.sh` - Runtime-agnostic test runner

**Features**:
- Run with `--runtime claude`, `--runtime opencode`, or `--runtime both`
- Mixed mode (`--mixed`) for primary/secondary runtime testing
- Specific test selection (`--test polecat`, `--test mayor`)
- Automatic prerequisite checking

**Usage**:
```bash
./scripts/test-runtime-e2e.sh --runtime opencode   # OpenCode only
./scripts/test-runtime-e2e.sh --runtime claude     # Claude Code only
./scripts/test-runtime-e2e.sh --runtime both       # Both runtimes
./scripts/test-runtime-e2e.sh --mixed              # Mixed primary/secondary
```

---

## 2026-01-17

### Integration Complete - 100% Feature Parity

Completed OpenCode integration with full Claude Code hook parity.

**Achievements**:
- All 4 Claude hooks implemented via OpenCode events/hooks
- Session fork verified via SDK/API
- All 6 Gastown roles support OpenCode
- Plugin system fully functional

**Bugs Fixed**:
- Empty slice to nil conversion in `agents.go`
- Missing Provider field in `RuntimeConfigFromPreset`
- Preset configs not normalized in `loader.go`
- GT_ROOT not used in `BuildStartupCommandWithAgentOverride`
- Agent override not used for runtime settings in `manager.go`

**Documents Created**:
- `archive/integration-summary.md` - Executive summary
- `archive/session-fork-test-results.md` - Fork testing
- `archive/integration-test-results.md` - E2E testing
- `design/gastown-plugin.md` - Plugin implementation guide

See: `archive/integration-summary.md`

---

## 2026-01-15

### Planning Phase Complete

Comprehensive documentation for OpenCode integration created.

**Documents Created**:
- `design/phase1/project-plan.md` - Phase 1 plan
- `design/phase1/decisions.md` - Architecture decisions
- `design/phase2/sdk-plan.md` - Future SDK orchestration
- `archive/concept-analysis.md` - Concept coupling analysis
- `archive/technical-research.md` - OpenCode research

**Key Decisions**:
- Use runtime abstraction layer (not OpenCode-specific code paths)
- Use plugin system over custom tools
- Use HTTP API for session forking

---

## 2026-01-14

### Initial Investigation

Started OpenCode integration research.

**Actions**:
- Cloned OpenCode repository for source investigation
- Identified plugin event system
- Discovered `experimental.session.compacting` hook
- Found session fork in HTTP API

**Documents Created**:
- `archive/experiments.md` - Experiment tracking

---

## Template for New Entries

```markdown
## YYYY-MM-DD

### Title

Brief description of what was done.

**Actions**:
- Action 1
- Action 2

**Documents Created/Updated**:
- `path/to/doc.md` - Brief description

**Key Decisions**:
- Decision 1 with rationale

See: `relevant/document.md` (if applicable)
```
