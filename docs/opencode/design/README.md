# OpenCode Design Documents

> **Purpose**: Design decisions, implementation strategies, and project roadmaps  
> **Scope**: Future work, integration patterns, and operational strategies

---

## ⚠️ Maintenance Notice

**This README should be reviewed and updated when any document in `design/` is created, modified, or removed.**

---

## Contents

### Standalone Documents

| Document | Purpose | Status |
|----------|---------|--------|
| [design-decisions.md](design-decisions.md) | **Core design decisions, constraints, assumptions** | Active |
| [next-steps.md](next-steps.md) | What to work on next | Active |
| [maintenance.md](maintenance.md) | Version compatibility, update procedures | Active |
| [gastown-plugin.md](gastown-plugin.md) | Gastown plugin implementation strategy | Active |
| [role-permissions.md](role-permissions.md) | Role-based permission profiles across runtimes | Active |

### Project Phases

| Phase | Focus | Status |
|-------|-------|--------|
| [phase1/](phase1/) | Claude Code parity | ✅ Complete |
| [phase2/](phase2/) | SDK-based orchestration | 📋 Future |

---

## Document Types

This directory contains:

1. **Design Documents** - Implementation strategies and architecture decisions
2. **Roadmaps** - Project phases with milestones
3. **Operational Strategies** - Maintenance, compatibility, update procedures

---

## When to Add Documents Here

Add a document to `design/` when it:
- Proposes a new feature or integration approach
- Documents architectural decisions not yet implemented
- Defines project phases or milestones
- Outlines operational procedures that may change

**Do NOT add here**:
- Stable reference documentation → use `reference/`
- Point-in-time snapshots or test results → use `archive/`

---

## Related

| Directory | Purpose |
|-----------|---------|
| `reference/` | Stable, evergreen documentation |
| `archive/` | Point-in-time snapshots, test results |
| `HISTORY.md` | Chronological log of all changes |
