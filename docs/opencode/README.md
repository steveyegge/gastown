---
# OpenCode Integration Documentation
# This is the landing page for all OpenCode-related docs in Gastown.
# Agents should update the fields below when making changes.

last_updated: 2026-01-19
status: experimental  # experimental | stable | deprecated
production_ready: true
opencode_version_tested: 1.1.25
gastown_branch: copilot/add-opencode-orchestration-layer-again

# Documentation organization guidelines for agents:
# - Active guides go in docs/opencode/ root
# - Point-in-time analysis/reports go in docs/opencode/archive/
# - Archive files must have date and commit metadata in their header
# - Use lowercase-with-dashes for all filenames (except README.md)
# - Update this README when adding/removing/renaming docs
---

# OpenCode Integration

> **Status**: ⚠️ **Experimental** - Community-contributed feature  
> **Primary Runtime**: Claude Code (recommended for production)  
> **This Integration**: Optional extension for users preferring OpenCode

This directory contains all documentation for OpenCode integration with Gastown.

**Repository Source**: [anomalyco/opencode](https://github.com/anomalyco/opencode)

---

## Quick Start

| Goal | Document |
|------|----------|
| **Get oriented** | [opencode-quickstart.md](opencode-quickstart.md) |
| **See current status** | [integration-summary.md](integration-summary.md) ⭐ |
| **Develop plugins** | [plugin-guide.md](plugin-guide.md) |
| **Maintain the integration** | [maintenance.md](maintenance.md) |

---

## Documentation Structure

### Active Documentation (root)

Core guides that are actively maintained:

| File | Purpose |
|------|---------|
| `README.md` | This landing page and organization guide |
| `integration-summary.md` | Current status, achievements, bugs fixed |
| `plugin-guide.md` | Plugin development and event system |
| `maintenance.md` | Keeping the integration up-to-date |
| `opencode-quickstart.md` | Quick orientation guide |
| `opencode-reference.md` | CLI/SDK/Server/Plugin reference |

### Planning & Architecture

Technical planning documents:

| File | Purpose |
|------|---------|
| `opencode-integration-architecture.md` | Detailed integration strategy |
| `opencode-orchestration.md` | High-level architecture |
| `opencode-implementation-guide.md` | Technical specifications |
| `opencode-concept-analysis.md` | Concept coupling analysis |
| `opencode-decisions.md` | Decisions and open questions |
| `opencode-experiments.md` | Validation experiments |
| `technical-research.md` | Research from OpenCode repo |
| `stage2-sdk-orchestration.md` | Future SDK-based plans |
| `opencode-index.md` | Comprehensive navigation hub |

### Archive (`archive/`)

Point-in-time analysis and test results. Each file includes date and commit metadata:

| File | Purpose |
|------|---------|
| `e2e-test-results.md` | E2E testing results (2026-01-17) |
| `integration-test-results.md` | Integration test results (2026-01-17) |
| `session-fork-test-results.md` | Fork testing results (2026-01-17) |
| `impact-analysis.md` | Feature parity analysis (2026-01-17) |
| `upstream-review.md` | PR review notes (2026-01-17) |

---

## Key Information

### Feature Parity

OpenCode achieves 100% feature parity with Claude Code:

- ✅ All 4 Claude hooks have OpenCode equivalents
- ✅ All 6 roles support OpenCode (Mayor, Deacon, Witness, Refinery, Polecat, Crew)
- ✅ Session resume, fork (via HTTP API), export/import
- ✅ Plugin system with event handlers

### Known Issues

- **OpenCode tmux crash**: OpenCode may crash inside tmux due to upstream `proper-lockfile/onExit` bug. This is an OpenCode issue, not Gastown.

---

## External References

- [OpenCode Repository](https://github.com/anomalyco/opencode)
- [OpenCode Documentation](https://opencode.ai/docs/)
- [ACP Specification](https://agentclientprotocol.com/)
- [Configuration Schema](https://opencode.ai/config.json)

---

## For Agents: When to Update This README

Update the **frontmatter** when:
- OpenCode version changes (`opencode_version_tested`)
- Status changes (`status`, `production_ready`)
- Making significant documentation changes (`last_updated`)

Update the **document tables** when:
- Adding new documentation files
- Moving files to/from archive
- Renaming files

Update `integration-summary.md` when:
- Fixing bugs in the integration
- Adding new features
- Changing tested versions
