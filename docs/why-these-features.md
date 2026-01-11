# Why These Features?

> Gas Town's architecture explained through enterprise AI challenges

## The Problem

You have AI agents. Maybe a lot of them. They're writing code, reviewing PRs,
fixing bugs, adding features. But you can't answer basic questions:

- **Who did what?** Which agent wrote this buggy code?
- **Who's reliable?** Which agents consistently deliver quality?
- **Who can do this?** Which agent should handle this Go refactor?
- **What's connected?** Does this frontend change depend on a backend PR?
- **What's the full picture?** How's the project doing across 12 repos?

Traditional tools don't help. CI/CD tracks builds, not capability. Git tracks
commits, not agent performance. Project management tracks tickets, not the
nuanced reality of who actually did what, and how well.

## The Solution: A Work Ledger

Gas Town treats work as structured data. Every action is recorded. Every agent
has a track record. Every piece of work has provenance.

This isn't about surveillance. It's about **visibility** - the same visibility
you'd expect from any serious engineering system.

---

## Feature: Entity Tracking and Attribution

**The problem:** You deploy 50 agents across 10 projects. One of them introduces
a critical bug. Which one? Traditional git blame shows a generic "AI Assistant"
or worse, the human's name.

**The solution:** Every Gas Town agent has a distinct identity. Every action is
attributed:

```
Git commits:    gastown/polecats/toast <owner@example.com>
Beads records:  created_by: gastown/crew/joe
Event logs:     actor: gastown/polecats/nux
```

**Why it matters:**
- **Debugging:** Trace problems to specific agents
- **Compliance:** Audit trails for SOX, GDPR, enterprise policy
- **Accountability:** Know exactly who touched what, when

---

## Feature: Work History (Agent CVs)

**The problem:** You want to assign a complex Go refactor. You have 20 agents.
Some are great at Go. Some have never touched it. Some are flaky. How do you
choose?

**The solution:** Every agent accumulates a work history:

```bash
# What has this agent done?
bd audit --actor=gastown/polecats/toast

# Success rate on Go projects
bd stats --actor=gastown/polecats/toast --tag=go
```

**Why it matters:**
- **Performance management:** Objective data on agent reliability
- **Capability matching:** Route work to proven agents
- **Continuous improvement:** Identify underperforming agents for tuning

This is particularly valuable when **A/B testing models**. Deploy Claude vs GPT
on similar tasks, track their completion rates and quality, make informed decisions.

---

## Feature: Capability-Based Routing

**The problem:** You have work in Go, Python, TypeScript, Rust. You have agents
with varying capabilities. Manual assignment doesn't scale.

**The solution:** Work carries skill requirements. Agents have demonstrated
capabilities (derived from their work history). Matching is automatic:

```bash
# Agent capabilities (derived from work history)
bd skills gastown/polecats/toast
# → go: 47 tasks, python: 12 tasks, typescript: 3 tasks

# Route based on fit
gt dispatch gt-xyz --prefer-skill=go
```

**Why it matters:**
- **Efficiency:** Right agent for the right task
- **Quality:** Agents work in their strengths
- **Scale:** No human bottleneck on assignment

---

## Feature: Recursive Work Decomposition

**The problem:** Enterprise projects are complex. A "feature" becomes 50 tasks
across 8 repos involving 4 teams. Flat issue lists don't capture this structure.

**The solution:** Work decomposes naturally:

```
Epic: User Authentication System
├── Feature: Login Flow
│   ├── Task: API endpoint
│   ├── Task: Frontend component
│   └── Task: Integration tests
├── Feature: Session Management
│   └── ...
└── Feature: Password Reset
    └── ...
```

Each level has its own chain. Roll-ups are automatic. You always know where
you stand.

**Why it matters:**
- **Visibility:** See the forest and the trees
- **Coordination:** Dependencies are explicit
- **Progress tracking:** Accurate status at every level

---

## Feature: Cross-Project References

**The problem:** Your frontend can't ship until the backend API lands. They're
in different repos. Traditional tools don't track this.

**The solution:** Explicit cross-project dependencies:

```
depends_on:
  beads://github/acme/backend/be-456  # Backend API
  beads://github/acme/shared/sh-789   # Shared types
```

**Why it matters:**
- **No surprises:** You know what's blocking
- **Coordination:** Teams see their impact on others
- **Planning:** Realistic schedules based on actual dependencies

---

## Feature: Federation

**The problem:** Enterprise projects span multiple repositories, multiple teams,
sometimes multiple organizations (contractors, partners). Visibility is fragmented.

**The solution:** Federated workspaces that reference each other:

```bash
# Register remote workspace
gt remote add partner hop://partner.com/their-project

# Query across workspaces
bd list --remote=partner --tag=integration
```

**Why it matters:**
- **Enterprise scale:** Not limited to single-repo thinking
- **Contractor coordination:** Track delegated work
- **Distributed teams:** Unified view despite separate repos

---

## Feature: Validation and Quality Gates

**The problem:** An agent says "done." Is it actually done? Is the code quality
acceptable? Did it pass review?

**The solution:** Structured validation with attribution:

```json
{
  "validated_by": "gastown/refinery",
  "validation_type": "merge",
  "timestamp": "2025-01-15T10:30:00Z",
  "quality_signals": {
    "tests_passed": true,
    "review_approved": true,
    "lint_clean": true
  }
}
```

**Why it matters:**
- **Quality control:** Don't trust, verify
- **Audit trails:** Who approved what, when
- **Process enforcement:** Gates are data, not just policy

---

## Feature: Real-Time Activity Feed

**The problem:** Complex multi-agent work is opaque. You don't know what's
happening until it's done (or failed).

**The solution:** Work state as a real-time stream:

```bash
bd activity --follow

[14:32:08] + patrol-x7k.arm-ace bonded (5 steps)
[14:32:09] → patrol-x7k.arm-ace.capture in_progress
[14:32:10] ✓ patrol-x7k.arm-ace.capture completed
[14:32:14] ✓ patrol-x7k.arm-ace.decide completed
[14:32:17] ✓ patrol-x7k.arm-ace COMPLETE
```

**Why it matters:**
- **Debugging in real-time:** See problems as they happen
- **Status awareness:** Always know what's running
- **Pattern recognition:** Spot bottlenecks and inefficiencies

---

## The Enterprise Value Proposition

Gas Town is a developer tool - like an IDE, but for AI orchestration. However,
the architecture provides enterprise-grade foundations:

| Capability | Developer Benefit | Enterprise Benefit |
|------------|-------------------|-------------------|
| Attribution | Debug agent issues | Compliance audits |
| Work history | Tune agent assignments | Performance management |
| Skill routing | Faster task completion | Resource optimization |
| Federation | Multi-repo projects | Cross-org visibility |
| Validation | Quality assurance | Process enforcement |
| Activity feed | Real-time debugging | Operational awareness |

**For model evaluation:** Deploy different models on comparable tasks, track
outcomes objectively, make data-driven decisions about which models to use where.

**For long-horizon projects:** See how agents perform not just on single tasks,
but across complex, multi-phase, cross-functional initiatives.

**For cross-functional teams:** Unified visibility across repos, teams, and
even organizations.

---

## Design Philosophy

These features aren't bolted on. They're foundational:

1. **Attribution is not optional.** Every action has an actor.
2. **Work is data.** Not just tickets - structured, queryable data.
3. **History matters.** Track records determine trust.
4. **Scale is assumed.** Multi-repo, multi-agent, multi-org from day one.
5. **Verification over trust.** Quality gates are first-class primitives.

Gas Town is built to answer the questions enterprises will ask as AI agents
become central to their engineering workflows.
