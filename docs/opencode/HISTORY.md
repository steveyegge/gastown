# OpenCode Integration History

> **Purpose**: Chronological log of integration work, decisions, and documentation updates  
> **Format**: Newest entries at top

---

## 2026-01-20

### E2E Test Robustness - Session Completion & Precise Logging

Improved the E2E testing infrastructure and OpenCode plugin to ensure reliable task completion detection and better debuggability.

**Root Cause of Hangs**:
The FixBug E2E test was hanging because completion detection relied on multiple idle events, which could be delayed or inconsistent depending on model speed and TUI behavior.

**Improvements**:
1. **Completion Detection**: Added regex-based message analysis in `gastown.js`. The plugin now scans message content for indicators of success (e.g., "tests passed", "bug fixed", "a - b") and emits `GASTOWN_TASK_COMPLETE` immediately.
2. **Precise Logging**:
   - `gastown.js` now truncates logged messages to 500 characters and data values to 200 characters to prevent log bloat.
   - Added `messageID` logging to `message.part.updated` events for easier tracing.
3. **Model Switch**: Changed default OpenCode model in `internal/e2e/runner.go` to `github-copilot/gpt-5-mini` for faster, more reliable results in test environments.
4. **Direct Execution**: Optimized `runner.go` to support direct `opencode run <prompt>` execution for non-interactive E2E validation.
5. **Orphan Cleanup**: Enhanced `cleanupTestProcesses()` in `runner.go` to use `pgrep -f` on both `sessionName` and `tmuxDir`, ensuring no zombie processes survive test completion or failure.

**Files Changed**:
- `internal/e2e/runner.go`: Updated default model, improved process cleanup, added direct run support.
- `internal/opencode/plugin/gastown.js`: Implemented log truncation, message content analysis, and completion signaling.

**Test Results**:
- `TestGastown_FixBug/opencode`: PASS
- `TestGastown_CreateFile/opencode`: PASS

---

## 2026-01-19 (Late Evening)

### Documentation Validation and contributing.md Cleanup

Fixed issues in the documentation validation system and improved the quality of OpenCode integration docs.

**Actions**:
- Improved `scripts/validate-opencode-docs.sh` to strip triple-backtick blocks and inline code before link extraction.
- Enhanced `scripts/validate-opencode-docs.sh` to correctly handle and skip anchors (e.g., `#section`) in local paths.
- Fixed a broken link in `docs/opencode/CONTRIBUTING.md`.
- Updated `docs/opencode/CONTRIBUTING.md` example formatting to properly demonstrate bad links without triggering validation errors.
- Verified all OpenCode L1-L5 E2E tests PASS in quick mode.

**Bug Fixes**:
- Validation script no longer extracts links from explanatory text or code blocks.
- Relative paths with anchors now resolve correctly (ignoring the anchor for file existence checks).

---

## 2026-01-19 (Evening)

### Comprehensive Testing Infrastructure

Completed full E2E testing infrastructure for OpenCode integration.

**Test Infrastructure Created**:
- `internal/testutil/fixtures.go` - Town fixture with proper PATH management
- `internal/testutil/wait.go` - Session waiting and diagnostic utilities
- `internal/integration/mayor_test.go` - Runtime-agnostic mayor workflow tests
- `internal/integration/fixture_test.go` - Town fixture and runtime settings tests

**Test Scripts Created**:
- `scripts/test-opencode-comprehensive-e2e.sh` - L1-L5 progression tests
- `scripts/test-opencode-formula-e2e.sh` - Formula/specialized agent tests
- `scripts/test-opencode-compaction-e2e.sh` - Compaction behavior tests
- `scripts/test-claude-regression.sh` - Claude Code regression tests

**Bug Fixes**:
- Added `install` to beadsExemptCommands in root.go (install shouldn't require beads check)
- Exported `NormalizeRuntimeConfig` for testutil (was internal `normalizeRuntimeConfig`)
- Fixed PATH management in testutil to prefer `~/go/bin` over fnm shell (beads version conflict)

**Key Decisions**:
- Use Go testing patterns instead of bash scripts for integration tests
- Testutil uses `gt install` and `runtime.EnsureSettingsForRole` instead of duplicating logic
- PATH explicitly managed in tests to ensure correct binary versions

**Global Git Configuration**:
- Added `sisyphus/` and `.sisyphus/` to global gitignore (`~/.gitignore_global`)

**Test Results**:
- All integration tests PASS (TestTownFixtureCreation, TestRuntimeSettings)
- All Claude regression tests PASS (7/7)
- All unit tests PASS across internal packages

---

## 2026-01-19

### Code Organization (Afternoon)

**OpenCode Package Cleanup**:
- Renamed `plugin.go` → `embed.go` (it embeds/installs, not the plugin)
- Consolidated `plugin_test.go` + `plugin_content_test.go` → `embed_test.go`
- Follows `internal/plugin/` pattern (file.go + file_test.go)

**Design Directory Reorganization**:
- Moved `maintenance.md` to `reference/` (operational, not design)
- Moved `gastown-plugin.md` to `reference/plugin-implementation.md`
- Created `design-decisions.md` for core integration patterns
- Updated `design/README.md` with clear criteria for what belongs where

**Plugin Simplification Analysis** (design-decisions.md P4):
- Identified core architecture issue: PATH set in each runtime's hooks, not once in core
- Proposed fix: Add PATH to `AgentEnv()` in `internal/config/env.go`
- Target plugin: ~15 lines vs current ~190 lines
- Tasks documented for simplifying both Claude hooks and gastown.js

**Test Infrastructure Plan** (next-steps.md):
- Proposed `internal/testutil/` package with fixtures
- Extract constants, logging, session waiting patterns
- Apply to all E2E tests for consistency

**Validation Script Improvement**:
- Git-based HISTORY.md change detection (replaces date-based check)
- No more "forever nag" - compares against last HISTORY.md commit

### Documentation Reorganization (Morning)

Reorganized the entire `docs/opencode/` structure for clarity and discoverability.

**Reference Directory** (stable docs):
- Created `configuration.md` - Config, models, permissions, MCP, tools
- Created `integration-guide.md` - CLI, SDK, API, Plugin integration (one-stop shop)
- Renamed `api-reference.md` → `external-resources.md` - Links to official docs
- Created `events.md` - Event types reference with GitHub source link
- Rewrote `quickstart.md` - Practical getting-started guide

**Moved to Design** (formerly `planning/`):
- `plugin-guide.md` → `reference/plugin-implementation.md` - Gastown-specific implementation plan
- `maintenance.md` → `reference/maintenance.md` - Version compatibility, update procedures

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
- `reference/plugin-implementation.md` - Plugin implementation guide

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
