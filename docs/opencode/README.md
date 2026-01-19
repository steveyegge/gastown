---
# OpenCode Integration Documentation
# Landing page for all OpenCode-related docs in Gastown.

last_updated: 2026-01-19
status: experimental  # experimental | stable | deprecated
production_ready: true
opencode_version_tested: 1.1.25

# Organization guidelines:
# - reference/: Stable docs on how things work
# - research/: Analysis, experiments, investigations  
# - planning/: Decisions, roadmaps, future work
# - archive/: Point-in-time results with date+commit metadata
# - Use lowercase-with-dashes for filenames (except README.md)
---

# OpenCode Integration

> **Status**: Experimental - Community-contributed feature  
> **Primary Runtime**: Claude Code (recommended for production)

This directory contains documentation for OpenCode integration with Gastown.

---

## Source of Truth

The **OpenCode repository** should be considered the authoritative source - documentation often lags behind:

| Resource | URL | Purpose |
|----------|-----|---------|
| **OpenCode Repo** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) | Source of truth for internals |
| **Awesome OpenCode** | [awesome-opencode/awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) | Community extensions, plugins, tools |
| **Ecosystem** | [opencode.ai/docs/ecosystem](https://opencode.ai/docs/ecosystem/) | Official ecosystem directory |
| **Official Docs** | [opencode.ai/docs](https://opencode.ai/docs/) | Official documentation |

> **Tip**: When OpenCode doesn't support something directly, check the ecosystem and awesome-opencode for community solutions before building it ourselves.

---

## Quick Start

| Goal | Document |
|------|----------|
| **Get oriented** | [opencode-quickstart.md](opencode-quickstart.md) |
| **See current status** | [integration-summary.md](integration-summary.md) |
| **Develop plugins** | [plugin-guide.md](plugin-guide.md) |
| **Find detailed navigation** | [opencode-index.md](opencode-index.md) |

---

## Documentation Categories

### Reference (Stable)

How things currently work:

| File | Purpose |
|------|---------|
| [integration-summary.md](integration-summary.md) | Current status, achievements, bugs fixed |
| [plugin-guide.md](plugin-guide.md) | Plugin development and event system |
| [opencode-reference.md](opencode-reference.md) | CLI/SDK/Server/Plugin reference |
| [maintenance.md](maintenance.md) | Keeping the integration up-to-date |
| [opencode-quickstart.md](opencode-quickstart.md) | Quick orientation guide |

### Research & Analysis

Investigation and analysis work:

| File | Purpose |
|------|---------|
| [technical-research.md](technical-research.md) | Deep dive into OpenCode repository |
| [opencode-concept-analysis.md](opencode-concept-analysis.md) | Concept coupling analysis |
| [opencode-experiments.md](opencode-experiments.md) | Validation experiments |

### Planning & Decisions

Project direction and future work:

| File | Purpose |
|------|---------|
| [opencode-decisions.md](opencode-decisions.md) | Decision points and open questions |
| [opencode-orchestration.md](opencode-orchestration.md) | High-level architecture vision |
| [opencode-integration-architecture.md](opencode-integration-architecture.md) | Detailed integration strategy |
| [opencode-implementation-guide.md](opencode-implementation-guide.md) | Technical specifications |
| [stage2-sdk-orchestration.md](stage2-sdk-orchestration.md) | Future SDK-based plans |

### Archive

Point-in-time results (each file includes date and commit metadata):

| File | Purpose |
|------|---------|
| [archive/e2e-test-results.md](archive/e2e-test-results.md) | E2E testing results |
| [archive/integration-test-results.md](archive/integration-test-results.md) | Integration test results |
| [archive/session-fork-test-results.md](archive/session-fork-test-results.md) | Fork testing results |
| [archive/impact-analysis.md](archive/impact-analysis.md) | Feature parity analysis |
| [archive/upstream-review.md](archive/upstream-review.md) | PR review notes |

### Navigation

| File | Purpose |
|------|---------|
| [opencode-index.md](opencode-index.md) | Comprehensive navigation by role/goal |

---

## Key Information

### Feature Parity

OpenCode achieves 100% feature parity with Claude Code:
- ✅ All 4 Claude hooks have OpenCode equivalents
- ✅ All 6 roles support OpenCode
- ✅ Session resume, fork (via HTTP API), export/import

### Known Issues

- **OpenCode tmux crash**: Upstream `proper-lockfile/onExit` bug may cause crashes inside tmux

---

## For Agents

Update **frontmatter** when:
- OpenCode version changes (`opencode_version_tested`)
- Status changes (`status`, `production_ready`)
- Making documentation changes (`last_updated`)

Update **document tables** when:
- Adding/removing/renaming documentation files

Update [integration-summary.md](integration-summary.md) when:
- Fixing bugs in the integration
- Adding new features
