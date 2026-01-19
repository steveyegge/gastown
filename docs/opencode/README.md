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
| **Concept index** | [reference/api-reference.md](reference/api-reference.md) |
| **Project plan** | [planning/phase1/project-plan.md](planning/phase1/project-plan.md) |

---

## Directory Structure

```
docs/opencode/
├── README.md              # This landing page
├── reference/             # Stable documentation
│   ├── api-reference.md   # Concept index with all sources
│   ├── integration-summary.md
│   ├── plugin-guide.md
│   ├── maintenance.md
│   └── quickstart.md
├── planning/              # Project direction (by phase)
│   ├── phase1/            # Claude Code parity (current)
│   │   ├── project-plan.md
│   │   └── decisions.md
│   └── phase2/            # SDK orchestration (future)
│       ├── sdk-plan.md
│       ├── orchestrator-api.md
│       ├── orchestration-plan.md
│       └── integration-architecture.md
├── research/              # Analysis & experiments
│   ├── technical-research.md
│   ├── concept-analysis.md
│   └── experiments.md
└── archive/               # Point-in-time results
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

Organized by phase:

**Phase 1: Claude Code Parity** (Current)

| File | Purpose |
|------|---------|
| [phase1/project-plan.md](planning/phase1/project-plan.md) | Current plan, testing, compatibility |
| [phase1/decisions.md](planning/phase1/decisions.md) | Decision points and questions |

**Phase 2: SDK Orchestration** (Future)

| File | Purpose |
|------|---------|
| [phase2/sdk-plan.md](planning/phase2/sdk-plan.md) | SDK migration plan |
| [phase2/orchestrator-api.md](planning/phase2/orchestrator-api.md) | Orchestrator interface design |
| [phase2/architecture.md](planning/phase2/architecture.md) | High-level architecture |
| [phase2/integration-architecture.md](planning/phase2/integration-architecture.md) | Detailed strategy |

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
| **Project plan** | [planning/phase1/project-plan.md](planning/phase1/project-plan.md) | - |
| **API reference** | [reference/api-reference.md](reference/api-reference.md) | - |
| **Work on plugin** | [reference/plugin-guide.md](reference/plugin-guide.md) | `internal/opencode/plugin/gastown.js` |
| **Work on config** | [reference/api-reference.md](reference/api-reference.md) | `internal/config/agents.go` |
| **Work on tests** | [planning/phase1/project-plan.md](planning/phase1/project-plan.md) | `internal/opencode/integration_test.go` |

### What to Update

| When... | Update... |
|---------|-----------|
| **Fixing bugs** | `reference/integration-summary.md` |
| **Adding features** | `reference/integration-summary.md`, `planning/phase1/project-plan.md` |
| **Adding docs** | This README → document tables |
| **Completing a phase** | `planning/phase1/project-plan.md` or `planning/phase2/` |
