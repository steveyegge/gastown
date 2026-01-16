# Gastown Design Documentation

> Technical design documents for Gastown architecture and features

## Overview

This directory contains design documents, architectural specifications, and planning materials for Gastown's core systems and features.

## Core Architecture

- [architecture.md](architecture.md) - Core Gastown architecture (two-level Beads, agent taxonomy)
- [operational-state.md](operational-state.md) - State management principles
- [property-layers.md](property-layers.md) - Configuration and property system

## Agent Systems

- [convoy-lifecycle.md](convoy-lifecycle.md) - Convoy workflow and lifecycle
- [dog-pool-architecture.md](dog-pool-architecture.md) - Dog worker pool design
- [watchdog-chain.md](watchdog-chain.md) - Watchdog monitoring system

## Infrastructure & Communication

- [mail-protocol.md](mail-protocol.md) - Inter-agent messaging protocol
- [plugin-system.md](plugin-system.md) - Plugin architecture and patterns
- [escalation-system.md](escalation-system.md) - Escalation routing and management
- [escalation.md](escalation.md) - Escalation design (legacy)

## Multi-Workspace

- [federation.md](federation.md) - Multi-workspace coordination (future)

## Opencode Orchestration

**New Feature**: Extending Gastown to support Opencode as an orchestration layer alongside Claude Code.

### Quick Start
- **[opencode-quickstart.md](opencode-quickstart.md)** ⭐ Start here for a one-page overview

### Opencode Integration

**Documentation Location**: All Opencode integration documentation has been moved to [`docs/opencode/`](../opencode/)

**Quick Links**:
- **[Opencode Documentation Index](../opencode/README.md)** - Complete documentation index
- [Quickstart Guide](../opencode/opencode-quickstart.md) - One-page summary
- [Technical Research](../opencode/technical-research.md) ⭐ **NEW** - Deep dive into Opencode repository
- [Concept Analysis](../opencode/opencode-concept-analysis.md) - Coupling analysis
- [Integration Architecture](../opencode/opencode-integration-architecture.md) - Implementation strategy
- [Experiments Checklist](../opencode/opencode-experiments.md) - Validation experiments

**Key Findings**:
- 13/23 Gastown concepts already runtime-agnostic
- Minimal changes needed: ~200-300 LOC in 5 files
- Session forking available via ACP `session/fork` (needs verification)
- Configuration schema: [https://opencode.ai/config.json](https://opencode.ai/config.json)

**Status**: Planning complete, ready for implementation (2026-01-16)

---

## Document Types

### Architecture Documents
Define the structure and organization of major systems. Focus on "how it works" and "why it's designed this way."

Examples: `architecture.md`, `dog-pool-architecture.md`

### Design Specifications
Detail the design of specific features or subsystems. Include interfaces, workflows, and implementation guidance.

Examples: `plugin-system.md`, `mail-protocol.md`

### Planning Documents
Outline plans for upcoming features, including goals, options, and open questions.

Examples: `opencode-*.md`, `federation.md`

## Related Documentation

- [../concepts/](../concepts/) - High-level concepts and terminology
- [../examples/](../examples/) - Practical examples and tutorials
- [../reference.md](../reference.md) - Command reference
- [../glossary.md](../glossary.md) - Terminology guide

## Contributing

When adding design documents:

1. **Choose the right format**:
   - Architecture: Explain structure and relationships
   - Design Spec: Detail interfaces and workflows
   - Planning: Explore options and decisions

2. **Use clear structure**:
   - Start with overview/problem statement
   - Include diagrams where helpful
   - Document alternatives and tradeoffs
   - Provide examples

3. **Cross-reference**:
   - Link to related docs
   - Update index when adding docs
   - Keep README.md updated

4. **Keep current**:
   - Add "Last Updated" date
   - Update status (Planning/Active/Implemented)
   - Archive obsolete docs

## Status Legend

- ✅ **Implemented** - Feature is live
- 🔄 **In Progress** - Actively being built
- 📝 **Planning** - Design phase
- 🔮 **Future** - Not yet scheduled
- 📦 **Archived** - Historical reference

---

Last Updated: 2026-01-15
