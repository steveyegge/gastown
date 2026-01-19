---
# OpenCode Integration Documentation
# Landing page for all OpenCode-related docs in Gastown.

last_updated: 2026-01-19
status: experimental  # experimental | stable | deprecated
production_ready: true
opencode_version_tested: 1.1.25

# Directory structure:
#   reference/  - Stable docs on how things work
#   planning/   - Project direction, decisions, roadmaps
#   research/   - Analysis, experiments, investigations
#   archive/    - Point-in-time results with date+commit metadata
#
# Naming conventions:
#   - All lowercase with hyphens (kebab-case)
#   - No "opencode-" prefix in subdirectories (redundant)
#   - README.md is the exception (uppercase for standard)
---

# OpenCode Integration

> **Status**: Experimental - Community-contributed feature  
> **Primary Runtime**: Claude Code (recommended for production)

---

## Source of Truth

The **OpenCode repository** is the authoritative source - documentation often lags behind:

| Resource | URL | Purpose |
|----------|-----|---------|
| **OpenCode Repo** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) | Source of truth |
| **Awesome OpenCode** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) | Community extensions |
| **Ecosystem** | [opencode.ai/docs/ecosystem](https://opencode.ai/docs/ecosystem/) | Official ecosystem |
| **Official Docs** | [opencode.ai/docs](https://opencode.ai/docs/) | Official documentation |

> **Tip**: Check community solutions before building something new.

---

## Quick Start

| Goal | Document |
|------|----------|
| **Get oriented** | [reference/quickstart.md](reference/quickstart.md) |
| **See current status** | [reference/integration-summary.md](reference/integration-summary.md) |
| **Topic reference** | [reference/api-reference.md](reference/api-reference.md) |
| **See project plan** | [planning/project-plan.md](planning/project-plan.md) |

---

## Directory Structure

```
docs/opencode/
├── README.md              # This landing page
├── index.md      # Detailed navigation by role/goal
├── reference/             # Stable documentation
│   ├── integration-summary.md
│   ├── plugin-guide.md
│   ├── api-reference.md
│   ├── maintenance.md
│   └── quickstart.md
├── planning/              # Project direction
│   ├── project-plan.md
│   ├── decisions.md
│   ├── orchestration.md
│   ├── integration-architecture.md
│   ├── implementation-guide.md
│   └── stage2-sdk.md
├── research/              # Analysis & experiments
│   ├── technical-research.md
│   ├── concept-analysis.md
│   └── experiments.md
└── archive/               # Point-in-time results
    ├── e2e-test-results.md
    ├── integration-test-results.md
    └── ...
```

---

## Documentation by Category

### reference/ - How Things Work

| File | Purpose |
|------|---------|
| [integration-summary.md](reference/integration-summary.md) | Current status, achievements, bugs fixed |
| [plugin-guide.md](reference/plugin-guide.md) | Plugin development and events |
| [api-reference.md](reference/api-reference.md) | CLI, SDK, Server, Plugins reference |
| [maintenance.md](reference/maintenance.md) | Keeping integration up-to-date |
| [quickstart.md](reference/quickstart.md) | Quick orientation |

### planning/ - Project Direction

| File | Purpose |
|------|---------|
| [project-plan.md](planning/project-plan.md) | **Current plan** - phases, testing, compatibility |
| [decisions.md](planning/decisions.md) | Decision points and questions |
| [orchestration.md](planning/orchestration.md) | High-level architecture |
| [integration-architecture.md](planning/integration-architecture.md) | Detailed strategy |
| [implementation-guide.md](planning/implementation-guide.md) | Technical specs |
| [stage2-sdk.md](planning/stage2-sdk.md) | Future SDK plans |

### research/ - Analysis & Experiments

| File | Purpose |
|------|---------|
| [technical-research.md](research/technical-research.md) | Deep dive into OpenCode repo |
| [concept-analysis.md](research/concept-analysis.md) | Concept coupling analysis |
| [experiments.md](research/experiments.md) | Validation experiments |

### archive/ - Historical Results

Point-in-time results with date and commit metadata.

| File | Purpose |
|------|---------|
| [e2e-test-results.md](archive/e2e-test-results.md) | E2E testing results |
| [integration-test-results.md](archive/integration-test-results.md) | Integration tests |
| [session-fork-test-results.md](archive/session-fork-test-results.md) | Fork testing |
| [impact-analysis.md](archive/impact-analysis.md) | Feature parity analysis |
| [upstream-review.md](archive/upstream-review.md) | PR review notes |

---

## For Agents

### Primary Entry Points

| Task | Document | Implementation |
|------|----------|----------------|
| **Current status** | [reference/integration-summary.md](reference/integration-summary.md) | - |
| **Project plan** | [planning/project-plan.md](planning/project-plan.md) | - |
| **API reference** | [reference/api-reference.md](reference/api-reference.md) | - |
| **Work on plugin** | [reference/plugin-guide.md](reference/plugin-guide.md) | `internal/opencode/plugin/gastown.js` |
| **Work on config** | [reference/api-reference.md](reference/api-reference.md) | `internal/config/agents.go` |
| **Work on tests** | [planning/project-plan.md](planning/project-plan.md) | `internal/opencode/integration_test.go` |

### What to Update

| When... | Update... |
|---------|-----------|
| **Fixing bugs** | `reference/integration-summary.md` |
| **Adding features** | `reference/integration-summary.md`, `planning/project-plan.md` |
| **Adding docs** | This README → document tables |
| **Completing a phase** | `planning/project-plan.md` |
