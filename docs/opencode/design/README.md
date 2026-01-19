# OpenCode Design Documents

> **Purpose**: Design decisions, proposals, and project roadmaps  
> **Scope**: Planning documents for future work and architectural decisions

---

## ⚠️ Maintenance Notice

**This README should be reviewed when any document in `design/` is created, modified, or removed.**

---

## What Belongs Here

| Type | Examples | Belongs in `design/`? |
|------|----------|----------------------|
| **Decisions** | Architectural choices, constraints | ✅ Yes |
| **Proposals** | Permission strategies, integration approaches | ✅ Yes |
| **Roadmaps** | Phase plans, next steps | ✅ Yes |
| **Implementation Guides** | How to build plugins | ❌ No → `reference/` |
| **Operational Docs** | Maintenance, troubleshooting | ❌ No → `reference/` |
| **Stable Reference** | Config options, events | ❌ No → `reference/` |

---

## Contents

### Core Design Documents

| Document | Purpose |
|----------|---------|
| [design-decisions.md](design-decisions.md) | **Core decisions, constraints, assumptions** |
| [next-steps.md](next-steps.md) | What to work on next (agent handoff) |

### Proposals (Under Consideration)

| Document | Proposal | Status |
|----------|----------|--------|
| [role-permissions.md](role-permissions.md) | Permission profiles per role per runtime | 🔄 Draft |

### Project Phases

| Phase | Focus | Status |
|-------|-------|--------|
| [phase1/](phase1/) | Claude Code parity | ✅ Complete |
| [phase2/](phase2/) | SDK-based orchestration | 📋 Future |

---

## Document Flow

```
Proposal (design/) → Decision (design/) → Implementation (code) → Reference (reference/)
```

1. **Proposals** start in `design/` as ideas
2. **Decisions** are captured in `design-decisions.md`
3. **Implementation** happens in code
4. **Reference** docs in `reference/` document the result

---

## Recently Moved to Reference

These were moved from `design/` to `reference/` since they're implementation/operational docs:

| Document | New Location |
|----------|--------------|
| `gastown-plugin.md` | `reference/plugin-implementation.md` |
| `maintenance.md` | `reference/maintenance.md` |

---

## Related Directories

| Directory | Purpose |
|-----------|---------|
| `reference/` | Stable, evergreen documentation |
| `archive/` | Point-in-time snapshots, test results |
