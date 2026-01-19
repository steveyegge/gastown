---
# OpenCode Integration Documentation
# Landing page for all OpenCode-related docs in Gastown.
#
# Directory structure:
#   reference/  - Stable docs on how things work
#   design/   - Design documents, roadmaps, implementation strategies
#   archive/    - Point-in-time analysis/research (with date+commit frontmatter)
#
# Naming conventions:
#   - All lowercase with hyphens (kebab-case)
#   - README.md exception (uppercase for standard)
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

## ⚠️ Agent Instruction

**All modifications to OpenCode integration must be logged to [HISTORY.md](HISTORY.md).**

When committing changes to:
- `docs/opencode/**` - Any documentation updates
- `internal/opencode/**` - Plugin or integration code
- `internal/config/agents.go` - Agent preset changes
- OpenCode-related test files

Add an entry to HISTORY.md with: date, what changed, why, and any key decisions.

---

## Quick Start

| Goal | Document |
|------|----------|
| **Compare runtimes** | [../agent-features.md](../agent-features.md) |
| **Get oriented** | [reference/quickstart.md](reference/quickstart.md) |
| **Configuration** | [reference/configuration.md](reference/configuration.md) |
| **Integration guide** | [reference/integration-guide.md](reference/integration-guide.md) |
| **External resources** | [reference/external-resources.md](reference/external-resources.md) |
| **Change history** | [HISTORY.md](HISTORY.md) |

---

## Directory Structure

```
docs/opencode/
├── README.md              # This landing page
├── HISTORY.md             # Chronological log of work
├── CONTRIBUTING.md        # Doc standards, review triggers
├── reference/             # Stable documentation
│   ├── quickstart.md      # Getting started
│   ├── configuration.md   # Config files, models, permissions
│   ├── customization.md   # Agents, commands, skills, MCP
│   ├── tools.md           # Built-in tools, custom tool options
│   ├── best-practices.md  # Performance, tool management
│   ├── integration-guide.md # CLI, SDK, API, Plugin
│   ├── events.md          # Event types for plugins/SDK
│   └── external-resources.md # Links to official docs, GitHub
├── design/              # Design documents, roadmaps
│   ├── maintenance.md     # Version compatibility, update procedures
│   ├── gastown-plugin.md  # Gastown plugin implementation
│   ├── phase1/            # Claude Code parity
│   └── phase2/            # SDK orchestration (future)
└── archive/               # Point-in-time snapshots
    ├── integration-summary.md # Status as of 2026-01-17
    ├── e2e-test-results.md
    └── ...
```

---

## Documentation by Category

### reference/ - Stable Documentation

| File | Purpose |
|------|---------|
| [quickstart.md](reference/quickstart.md) | Getting started |
| [configuration.md](reference/configuration.md) | Config, models, permissions |
| [customization.md](reference/customization.md) | Agents, commands, skills, MCP |
| [tools.md](reference/tools.md) | Built-in tools, custom tool options |
| [best-practices.md](reference/best-practices.md) | Performance, tool management |
| [integration-guide.md](reference/integration-guide.md) | CLI, SDK, API, Plugin integration |
| [plugin-implementation.md](reference/plugin-implementation.md) | How gastown.js plugin works |
| [maintenance.md](reference/maintenance.md) | Version compatibility, troubleshooting |
| [events.md](reference/events.md) | Event types for plugins/SDK |
| [external-resources.md](reference/external-resources.md) | Links to official docs, GitHub |

### design/ - Design Documents

See [design/README.md](design/README.md) for full contents.

Organized by phase:

**Phase 1: Claude Code Parity** (Current)

| File | Purpose |
|------|---------|
| [phase1/project-plan.md](design/phase1/project-plan.md) | Current plan, testing, compatibility |
| [phase1/decisions.md](design/phase1/decisions.md) | Decision points and questions |

**Phase 2: SDK Orchestration** (Future)

| File | Purpose |
|------|---------|
| [phase2/sdk-plan.md](design/phase2/sdk-plan.md) | SDK migration plan |
| [phase2/orchestrator-api.md](design/phase2/orchestrator-api.md) | Orchestrator interface design |
| [phase2/orchestration-plan.md](design/phase2/orchestration-plan.md) | Multi-session orchestration |
| [phase2/integration-architecture.md](design/phase2/integration-architecture.md) | Detailed strategy |

### archive/ - Point-in-Time Documentation

Analysis, research, and test results organized by phase. See [archive/README.md](archive/README.md) for frontmatter template.

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
| **Current status** | [archive/integration-summary.md](archive/integration-summary.md) | - |
| **Project plan** | [design/phase1/project-plan.md](design/phase1/project-plan.md) | - |
| **API reference** | [reference/external-resources.md](reference/external-resources.md) | - |
| **Work on plugin** | [reference/plugin-implementation.md](reference/plugin-implementation.md) | `internal/opencode/plugin/gastown.js` |
| **Work on config** | [reference/external-resources.md](reference/external-resources.md) | `internal/config/agents.go` |
| **Work on tests** | [design/phase1/project-plan.md](design/phase1/project-plan.md) | `internal/opencode/integration_test.go` |

### What to Update

| When... | Update... |
|---------|-----------|
| **Fixing bugs** | `archive/integration-summary.md` |
| **Adding features** | `archive/integration-summary.md`, `design/phase1/project-plan.md` |
| **Adding docs** | This README → document tables |
| **Completing a phase** | `design/phase1/project-plan.md` or `design/phase2/` |
