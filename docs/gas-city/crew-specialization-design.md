# Crew Specialization and Capability-Based Dispatch

**Bead:** hq-q76
**Date:** 2026-03-10
**Participants:** Mayor, Overseer
**Related:** beads `agent-cost-optimization` branch, w-gc-001, w-gc-002, w-com-005, PR #2518, PR #2527

## Context

Three threads of work converge on the same design question: how should agent
work be labeled, matched, and routed to specialized workers?

1. **Beads `agent-cost-optimization` branch** — adds tier-based dispatch
   (tool/basic/standard/advanced/human) and cost-aware routing to beads. The
   tier system is an 80/20 proxy for capability matching. The branch's §13
   sketches the generalization: capability-based dispatch where tasks declare
   requirements and executors declare capability profiles.

2. **Gas City role format (w-gc-001, w-gc-002)** — the planned declarative
   layer that formalizes Gas Town's hardcoded roles into portable, user-definable
   schemas. PR #2518 prototypes a TOML parser. PR #2527 adds per-crew agent
   assignment.

3. **Wasteland reputation (stamps)** — multi-dimensional attestations grounding
   capability claims in evidence. Completions get validated and stamped, building
   a reputation signal tied to a rig handle.

This document captures design insights from a discussion about how these
systems should work together.

---

## 1. The Cellular Model: Agents as Startups

### Problem with the Planned Economy

The beads branch (§4) assumes a central dispatcher computes optimal
task-to-worker assignment: read each task's tier, find the cheapest agent that
meets it. This is the job-shop model — globally optimal but requires central
knowledge of all workers and all tasks.

### The Alternative: Distributed Dispatch

Instead of a central planner, each agent is a **mini-town** that:

- **Advertises capability upward** — what it can handle
- **Delegates downward** — to sub-agents and tools
- **Makes internal allocation decisions** — with local knowledge

A manager agent's capability is the **transitive closure** of everything beneath
it. The Mayor doesn't need `native_skills: [security-review]` if it manages a
crew member that has it. Each level hides internal delegation decisions behind
a capability abstraction.

### The Fractal Pattern

This recurses at every scale:

| Scale | Unit | Advertises | Delegates to |
|-------|------|-----------|--------------|
| Federation | Wasteland rig | Stamps, reputation | Its Gas Town |
| Town | Mayor | Rig capabilities | Crew, polecats |
| Crew | Specialist | Domain expertise | Sub-agents, tools |
| Worker | Polecat | Task completion | Tools |
| Tool | CLI command | Deterministic output | Nothing |

### Delegation as Alternative to Cognition

From beads §13.1: "Cognition is a meta-capability — the ability to derive other
capabilities at runtime, at a cost premium."

In the cellular model, **delegation replaces cognition**. A Sonnet-tier crew
member with the right sub-agents achieves what Opus achieves alone, cheaper:

```
security-lead (Sonnet, 15k tokens own work)
├── dep-scanner (Haiku, 5k tokens)
│   └── cargo-audit (tool, 0 tokens)
└── code-reviewer (Sonnet, 12k tokens)
    └── semgrep (tool, 0 tokens)

Total: 32k tokens at blended rate
vs. Opus doing it all: 50k tokens at Opus rate
```

The specialist has **pre-computed the decomposition strategy**, not just domain
knowledge.

---

## 2. Capability Profiles: Claims + Evidence

### No Universal Taxonomy

A universal lingua franca for capability labels is self-defeating. Taxonomies
ossify faster than problems evolve. "security.network.cors" doesn't help when
the task is "JWT tokens embedded in WebSocket upgrade headers with mTLS client
certs."

Instead, each department publishes capability advertisements **in its own
language**, including explicit negative space:

```yaml
crew: api-gateway-security
handles:
  - CORS configuration and debugging
  - CSP header policy
  - API key validation and rotation
  - Rate limiting and abuse prevention
does_not_handle:
  - Cryptographic primitives (→ crypto team)
  - User identity/password management (→ identity team)
  - Application-level RBAC (→ owning service)
example_tasks:
  - "Users getting 403 on cross-origin API calls"
  - "Need to add a new allowed origin for partner integration"
anti_examples:
  - "Need to rotate the TLS certificate" (→ infra)
  - "Implement role-based access control" (→ app team)
```

`example_tasks` and `anti_examples` are more valuable than any taxonomy —
they're grounded in the language of problems, not solutions. Dispatchers
pattern-match against examples.

### Two-Sided Knowledge

A task isn't done until requester and resolver agree, but they know different
things:

- **Requester** knows: whether the symptom is resolved, how to describe the
  problem in problem-space language
- **Resolver** knows: what the root cause was, which capability was exercised,
  how to describe the solution in solution-space language

Capability profiles need **both**. Capture as paired routing examples:

```yaml
routing_examples:
  - symptom: "403 errors on cross-origin API calls"
    resolution: "CORS allow-origin configuration"
    cost: 8000 tokens
    complexity: single-domain
```

Symptoms are what future dispatchers will see. Resolutions prove capability.

### Profiles Are Living Documents

A capability profile starts as a **claim** — authored by whoever creates the
department. Over time, the system grounds these claims in evidence. This
lifecycle shapes the profile format:

**A department begins with only claims.** It lists `handles`,
`does_not_handle`, `example_tasks`, and `anti_examples`. These are cheap to
store, free when dormant, and sufficient for initial routing. But claims are
unverified — a department that says it handles CORS may never have resolved a
CORS issue.

**Evidence accumulates through four channels:**

1. **Successful completions.** A task routed to this department was resolved and
   accepted by the requester. The symptom-resolution pair becomes a routing
   example grounded in real work. If the resolution was outside the department's
   stated capability (a "stretch completion"), the claim surface expands
   tentatively.

2. **Bounces.** A task arrived here and the department correctly rejected it —
   "not my problem, try the identity team." This sharpens the `does_not_handle`
   boundary. Conversely, if the department *tried* to resolve it and failed,
   that's a signal that the claimed capability is weaker than stated.

3. **Reopened tasks.** The requester accepted a resolution, but the fix didn't
   hold — the problem recurred. This is the strongest negative signal: the
   department's capability in that area is shallower than its claims suggest.

4. **Objective metrics.** Cost (tokens spent), duration, and bounce history are
   observed by the system, not self-reported. They provide calibration
   independent of what either requester or resolver claims.

**Claims drift toward evidence.** Over time, the `example_tasks` and
`anti_examples` in the profile shift from authored hypotheticals to real
routing outcomes. The `handles` and `does_not_handle` get refined by actual
boundaries discovered through bouncing. A department that started with a
plausible-sounding claim but never completed a matching task has an evidence
gap — and the routing system should treat it differently from a department
with 20 completions in the same area.

This lifecycle motivates a **tiered trust model** for routing decisions:

| Evidence level | Trust level | Routing behavior |
|---|---|---|
| Claim only (0 completions) | Speculative | Route only when no proven alternative |
| 1-3 completions | Tentative | Route low-risk tasks to build signal |
| 4-10 completions | Operational | Route normally |
| 10+ completions | Proven | Prefer for this task type |

The practical consequence: route critical work to proven departments, and use
low-risk tasks to build evidence for tentative ones (explore/exploit tradeoff).
This also explains why extending an existing department is almost always better
than creating a new one — the existing department has routing history that a
new one starts without.

---

## 3. Routing, Bouncing, and Learning

### Bouncing Is Learning

In oncall systems, a ticket bounces from group to group looking for the right
resolver. Each bounce adds routing knowledge: "Auth team looked at this, it's
not auth, here's what they found." This feels like failure but is actually the
system **discovering its own routing table**.

Each bounce is a training example:
- The department that bounced it learns a boundary (anti_example)
- The ticket accumulates diagnostic context from each attempt
- Failed claims + successful resolutions refine the routing signal

The failure mode is that this knowledge stays trapped in individual tickets
instead of feeding back into capability profiles. The system should extract
routing signals from bounces and update profiles automatically.

### Single Ownership for Cross-Cutting Tasks

Cross-cutting tasks should have a **single owning department** that decomposes
and delegates, not shared ownership. Reasons:

- Produces cleaner routing signals: "A handles cross-domain security problems
  by coordinating B and C" is more useful than "A+B+C resolved this"
- **Coordination capability** (decomposition, delegation, integration testing)
  is a real capability that belongs on the owner's profile
- Escalation paths remain clear

```yaml
routing_examples:
  - symptom: "Intermittent auth failures on partner WebSocket connections"
    resolution: "Cross-cutting: CORS + mTLS + JWT token lifecycle"
    delegated_to: [crypto-team, identity-team]
    own_contribution: "Diagnosis, decomposition, integration testing"
    complexity: cross-domain
    cost: 45000 tokens (own: 15000, delegated: 30000)
```

---

## 4. Department Lifecycle

§2 established how profiles evolve from claims to evidence through completions,
bounces, reopened tasks, and objective metrics. This section addresses the
management decisions that shape the department landscape.

### Prefer Editing to Creating

In agent-land, spinning up a new department is nearly free — unlike human orgs
where headcount and budget provide natural friction against duplication. This
means the proliferation risk is real: three departments with overlapping CORS
capability, no clear routing preference, and no economic pressure to
consolidate.

Before creating a new specialist, check for 70%+ capability overlap with an
existing crew member. **Extend the existing one.** The reason is in §2: an
existing department with routing history routes better than a new one with only
claims. Dormant departments cost nothing to maintain, but their accumulated
evidence is expensive to rebuild.

### Periodic Consolidation

The Mayor (or a dedicated org-design function) periodically reviews the
department landscape:

- Merge dormant departments with significant overlap
- Archive departments that have never successfully completed a task
- Rewrite profiles for departments that receive misrouted tasks repeatedly
  (the evidence channels from §2 flag these)

### When to Modify Claims

Evidence accumulation (§2) is automatic — the system observes completions and
bounces. But **claim modification** is a deliberate decision. A department
should update its `handles` and `does_not_handle` when:

1. **Stretch completions accumulate** — one stretch is tentative; three in the
   same area justify expanding the claim
2. **Bounce patterns stabilize** — repeated bounces in the same direction mean
   the boundary is real and should be documented
3. **Contested capabilities surface** — if reopened tasks cluster around a
   specific claim, either invest in the capability or explicitly drop it
4. **Explicit reorganization** — Mayor restructures departments for strategic
   reasons (entering a security hardening phase, shifting project priorities)

### Complexity Weighting

Evidence from completions isn't uniform. A department that resolved one gnarly
cross-cutting incident proves **decomposition capability**. Ten simple CORS
tickets prove **CORS reliability**. These are different signals for different
dispatch decisions:

- Simple playbook task → route to department with reliability evidence
- Ambiguous cross-domain task → route to department with decomposition evidence

Who labels complexity? Neither requester nor resolver alone:
- Resolver labels which capabilities were exercised (solution-space)
- Requester confirms problem is solved (acceptance)
- System observes cost, duration, bounce history (objective metrics)
- Time reveals whether the fix held (reopened = contested)

---

## 5. Implications for Gas City Role Format (w-gc-001)

The role format should support the cellular model. A role is not a flat
capability profile — it's a **recursive structure** that can contain sub-roles:

```yaml
role: security-lead
goal: Handle security-related work for this rig
layer: crew

# Capability advertisement (authored claims — see §2 for how these evolve)
handles:
  - CORS configuration and debugging
  - Security audit coordination
  - CSP header policy
does_not_handle:
  - Cryptographic primitives (→ crypto)
  - User identity management (→ identity)
example_tasks:
  - "Users getting 403 on cross-origin API calls"
  - "Security audit of the auth module"
anti_examples:
  - "Rotate the TLS certificate" (→ infra)

# Execution profile
cognition: standard          # Sonnet-tier is sufficient with sub-agents
tools: [cargo-audit, semgrep, CVE-lookup]
context_docs: [OWASP-top-10.md, project-security-policy.md]

# Delegation (cellular model)
sub_agents:
  - role: dependency-auditor
    cognition: basic
    tools: [cargo-audit]
  - role: code-reviewer
    cognition: standard
    tools: [semgrep]

# Evidence (system-populated from routing history — see §2)
# Not authored; built from completions, bounces, and objective metrics
track_record:
  routing_examples: []       # symptom+resolution pairs from real tasks
  proven_boundaries: []      # confirmed does_not_handle from bounces
  completions: 0
  trust_level: speculative   # → tentative → operational → proven
```

### Key Design Decisions

1. **No universal taxonomy** — departments advertise in their own language using
   examples and anti-examples
2. **Negative space is required** — `does_not_handle` prevents bouncing
3. **Sub-agents are first-class** — the role format is recursive
4. **Claims evolve toward evidence** — profiles are living documents (§2)
5. **Cognition tier from beads branch is preserved** — as the floor for each
   node in the delegation tree
6. **Delegation cost is visible** — the role knows what sub-agent execution costs

### Relationship to Existing Work

| Component | Source | Status |
|-----------|--------|--------|
| Tier system | Beads `agent-cost-optimization` | Implemented on branch |
| Agent registry | Beads `agent-cost-optimization` | Implemented on branch |
| TOML role parser | Gastown PR #2518 | Open PR |
| Per-crew agent assignment | Gastown PR #2527 | Open PR |
| Agent framework survey | Gastown PR #2581 (w-gc-004) | Merged |
| Wasteland stamps | HOP federation | Live |
| Role format design | **w-gc-001** | **This document informs it** |

---

## 6. Open Questions

1. **How deep should delegation recurse?** A crew member managing sub-agents
   that manage sub-sub-agents is powerful but complex. Is 2-3 levels sufficient?

2. **Who writes the initial role definition?** The Mayor? The Overseer? A
   dedicated org-design agent? The role author needs cross-cutting visibility to
   avoid duplication.

3. **How does the dynamic track record feed back into routing?** Does the
   dispatcher query track records at routing time, or are they pre-computed into
   a routing index?

4. **How do Wasteland stamps connect to local track records?** A rig's Wasteland
   reputation should reflect its departments' aggregated track records, but the
   mapping is non-trivial.

5. **What's the minimum viable version?** The full cellular model with recursive
   sub-agents and dynamic track records is the vision. What's the 80/20 slice
   that delivers value with the beads tier system and existing Gas Town
   primitives?
