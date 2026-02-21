# Gas Town Feature Proposal: Next Evolution

**Author:** visionary (gastown/crew/visionary)
**Date:** 2026-02-21
**Status:** DRAFT - Awaiting engineer review

---

## Executive Summary

Gas Town has achieved something rare: a working multi-agent orchestration system where
20-30 AI agents coordinate autonomously via persistent work tracking, mail protocols,
and the Propulsion Principle (GUPP). The session-per-step model, Dolt-backed beads,
and hook-based lifecycle management are proven.

This proposal identifies the **top 10 features** that would take Gas Town from
"functional multi-agent workspace" to "self-improving autonomous development platform."
Each feature is ranked by impact, includes effort estimates, and specifies who benefits.

---

## What's Working Well

| Strength | Why It Matters |
|----------|----------------|
| **Two-Level Beads Architecture** | Town vs rig separation keeps concerns clean; prefix routing is elegant |
| **Dolt Branch-Per-Polecat** | Zero write contention at 50 concurrent writers. Proven at scale. |
| **Session-Per-Step Model** | Persistent sandbox + ephemeral sessions = reliable multi-step workflows |
| **Mail Protocol** | Simple, typed, human-readable. POLECAT_DONE -> MERGE_READY -> MERGED flow is solid |
| **GUPP (Propulsion Principle)** | Agents self-start on hook discovery. No polling, no supervisor bottleneck |
| **Hook-Based Lifecycle** | Integrates cleanly with Claude Code native hooks; SessionStart/PreCompact/PreToolUse |
| **Identity & Attribution** | Every action tracked to specific agent. Capability Ledger enables accountability |
| **Convoy Tracking** | Event-driven + stranded scan catches both normal completions and orphans |
| **Plugin Discovery Pattern** | State-as-data (wisps) not shadow config. Deacon patrol -> dog dispatch is clean |

## What's Painful

| Pain Point | Impact | Who Suffers |
|-----------|--------|-------------|
| **285 commands, no taxonomy** | Discovery hell. New agents/humans can't find what they need | Everyone |
| **Dolt SIGSEGV on filtered queries** | `bd list --json`, `--status=hooked` crash. Blocks gt hook, gt mail | All agents |
| **Agent Provider abstraction leaks** | Each AI model needs custom shims for process detection, readiness, text input | Deacon, Witness |
| **No escalation commands built** | Spec exists but `gt escalate`, stale patrol, multi-channel routing not implemented | Mayor, Human |
| **Plugin system incomplete** | `gt stale`, `gt dog dispatch`, `gt plugin digest` not yet built | Deacon, Dogs |
| **AT migration blockers** | No per-teammate cwd, no session resumption, PreToolUse drift risk | Witness, Polecats |
| **Hook ordering opacity** | No `gt tap list` showing active hooks for current context. Merge semantics unclear | All agents |
| **Cross-rig federation not built** | `beads://` references, `gt remote add` designed but not implemented | Mayor, Enterprise |
| **No auto-discovery for new rigs** | Manual routes.jsonl registration required | Human |
| **Convoy batch semantics confusing** | `gt sling a b c rig` creates 3 convoys (parallel), not 1 grouped convoy | Human, Mayor |

---

## Top 10 Feature Proposals

### 1. Intelligent Work Router ("The Dispatcher")

**Problem:** Today, humans manually sling work to rigs, and the Witness assigns polecats
round-robin. There's no intelligence about which agent is best suited for which task,
no load balancing across rigs, and no learning from past completions.

**Who Benefits:** Mayor, Human, Polecats

**Impact:** HIGH | **Effort:** L

**Implementation Sketch:**
- Add `gt dispatch` command that accepts a natural-language task description
- Mayor analyzes: task type, required skills, agent availability, historical success rates
- Query the Capability Ledger: which agents have completed similar beads successfully?
- Route to optimal rig + agent based on: (a) agent track record on similar work,
  (b) current load, (c) rig affinity (which rig owns the relevant code)
- Fallback: round-robin (current behavior) when no signal exists
- Store routing decisions as beads metadata for feedback loop

**Key insight:** The Capability Ledger already records every completion. Mining it for
routing intelligence is the highest-leverage use of existing data.

---

### 2. Agent Observability Dashboard v2 ("Mission Control")

**Problem:** The current web dashboard (htmx + Go) shows basic agent/polecat status.
There's no real-time view of: work throughput over time, agent cost per completion,
convoy progress, mail queue depth, Dolt health, or failure patterns. When something
goes wrong, humans discover it late.

**Who Benefits:** Human, Mayor, Deacon

**Impact:** HIGH | **Effort:** M

**Implementation Sketch:**
- Extend `internal/web/` dashboard with new panels:
  - **Throughput:** Beads closed per hour, per agent, per rig (time series)
  - **Cost:** Token usage per completion, per agent, per model (from costs-record hook)
  - **Health:** Dolt server status, mail queue depth, convoy completion rate
  - **Failures:** Failed merges, abandoned beads, escalation history
  - **Live view:** Real-time convoy progress with step-by-step status
- Add `gt metrics export` for Prometheus/Grafana integration
- Add WebSocket push for real-time updates (replace polling)
- Surface the data already being collected (costs hook, beads, wisps)

**Key insight:** Gas Town already *collects* most of this data. The gap is *surfacing*
it in a way humans can act on.

---

### 3. Self-Healing Agent Recovery ("Phoenix Protocol")

**Problem:** When agents crash, hang, or produce bad output, recovery is manual.
The Witness detects stale polecats and the Deacon has Boot as a watchdog, but there's
no automated recovery path: retry the failed step, reassign to a different agent,
or escalate with full context. Recent PRs (1816, 1818) show this is an active pain area -
stale polecats force-closing reassigned beads, stale branches before push.

**Who Benefits:** Witness, Deacon, Polecats, Human

**Impact:** HIGH | **Effort:** L

**Implementation Sketch:**
- Extend Witness patrol with recovery strategies:
  - **Retry:** Same step, same agent, fresh session (current: manual)
  - **Reassign:** Same step, different agent (new: requires hook transfer)
  - **Escalate:** Create P0 bead + mail Mayor with failure context
  - **Quarantine:** Mark agent as degraded, reduce assignment weight
- Add `failure_context` field to beads: capture last N lines of agent output before crash
- Implement `gt recover <bead-id>` command for manual intervention
- Strategy selection based on failure type:
  - Timeout -> retry (same agent)
  - SIGSEGV/panic -> reassign (different agent)
  - Merge conflict -> rebase + retry
  - 3 consecutive failures -> escalate

**Key insight:** The session-per-step model already supports retry (same sandbox,
new session). The missing piece is *automated strategy selection*.

---

### 4. Natural Language Task Dispatch ("Tell The Town")

**Problem:** Creating work requires knowing the beads/convoy/sling vocabulary.
Humans think "fix the login bug on the backend" not "bd create -t bug -p 1 -d '...'
&& gt convoy create && gt sling gt-xxx backend". The 285-command CLI has a steep
learning curve.

**Who Benefits:** Human (primary), Mayor

**Impact:** HIGH | **Effort:** M

**Implementation Sketch:**
- Add `gt ask "fix the login bug on the backend"` command
- Mayor (or a dedicated "dispatcher" agent) interprets the request:
  - Identifies target rig from context ("backend" -> backend rig)
  - Creates appropriate bead type (bug vs task vs feature)
  - Sets priority from urgency language
  - Creates convoy if multi-step
  - Slings to target rig
- Human gets confirmation: "Created gt-abc (P1 bug) in backend rig, assigned to polecat alpha"
- Add `gt ask --dry-run` to preview without executing
- Leverage Claude to parse intent, but execute via existing gt commands (no new state)

**Key insight:** The orchestration primitives are solid. The gap is a natural-language
front door that maps human intent to the right sequence of gt commands.

---

### 5. Cross-Town Federation ("The Alliance")

**Problem:** Gas Town currently operates as a single workspace. Organizations with
multiple teams need: shared visibility into other towns' work, ability to file
cross-town issues, and coordinated multi-town deployments. The `beads://` URI scheme
is designed but not implemented.

**Who Benefits:** Mayor, Enterprise teams, Human

**Impact:** MEDIUM-HIGH | **Effort:** XL

**Implementation Sketch:**
- Phase 1: Read-only federation
  - Implement `gt remote add <name> <url>` for town-to-town links
  - `bd show beads://github/acme/backend/be-456` resolves cross-town references
  - Remote query API: `gt remote query <town> "open P1 bugs"`
  - Uses GitHub API or git remote for transport (no new infra)
- Phase 2: Cross-town dispatch
  - `gt sling <bead> <remote-town>/<rig>` dispatches work across town boundaries
  - Convoy tracking spans towns (federated convoy IDs)
  - Mail routing across towns via Mayor-to-Mayor protocol
- Phase 3: Federation governance
  - Access control: which towns can dispatch to which rigs
  - Shared Capability Ledger: agent reputation portable across towns
  - Cost attribution across organizational boundaries

**Key insight:** The prefix-based routing system (routes.jsonl) already supports
remote resolution conceptually. The transport layer (git/GitHub) already exists.

---

### 6. Plugin Ecosystem & Marketplace ("The Bazaar")

**Problem:** The plugin system design is elegant (TOML frontmatter + markdown instructions,
gate types, wisp-based state tracking) but incomplete. Only `rebuild-gt` exists as an
example. There's no sharing mechanism, no version management, no community plugins.

**Who Benefits:** Deacon, Human, Community

**Impact:** MEDIUM | **Effort:** L

**Implementation Sketch:**
- Phase 1: Complete the plugin runtime
  - Implement `gt plugin list` (discovery from town + rig plugin dirs)
  - Implement `gt plugin run <name>` (manual trigger, bypassing gate)
  - Implement `gt plugin digest` (squash wisps into daily digest beads)
  - Add gate types: `event` (trigger on bead status change), `condition` (custom check)
- Phase 2: Plugin packaging
  - `gt plugin init <name>` scaffolds new plugin from template
  - `gt plugin test <name>` validates plugin.md format + gate configuration
  - `gt plugin export <name>` creates shareable archive
- Phase 3: Community sharing
  - `gt plugin install <url>` fetches and installs from git URL
  - Plugin registry (GitHub-based, like Homebrew taps)
  - Curated plugin categories: CI/CD, code quality, security, monitoring

**Starter plugins to build:**
- `security-audit` - Scan PRs for secret leaks, dependency vulns
- `test-coverage-gate` - Block merges below coverage threshold
- `cost-alert` - Notify when agent spend exceeds budget
- `standup-report` - Daily summary of all agents' work
- `github-sync` - Bidirectional sync between beads and GitHub Issues

---

### 7. Agent Memory & Learning ("The Archive")

**Problem:** Each agent session starts fresh (aside from CLAUDE.md and hook context).
Agents repeatedly rediscover the same patterns, make the same mistakes, and lack
institutional knowledge. The Capability Ledger tracks *what* was done but not
*how* or *what was learned*.

**Who Benefits:** All agents, Human

**Impact:** MEDIUM-HIGH | **Effort:** L

**Implementation Sketch:**
- Add `gt memory` subsystem:
  - `gt memory add <key> <value>` - Store a learning (backed by beads)
  - `gt memory search <query>` - Retrieve relevant memories
  - `gt memory inject` - Add relevant memories to current context (via hook)
- Memory categories:
  - **Codebase patterns:** "This repo uses X pattern for Y"
  - **Failure lessons:** "Don't do X because Y happens" (auto-captured from failed beads)
  - **Agent preferences:** "Human prefers Z workflow"
  - **Debugging insights:** "Error X is usually caused by Y"
- Memory is scoped: town-level (shared), rig-level (project), agent-level (personal)
- Hook integration: PreCompact hook auto-saves learnings before context eviction
- Memory decay: Auto-archive memories not accessed in 30 days

**Key insight:** Claude Code already has `.claude/` memory files. Gas Town can
systematize this into a shared, queryable knowledge base that benefits all agents.

---

### 8. Cost Optimization Engine ("The Accountant")

**Problem:** Multi-agent systems are expensive. Gas Town runs 20-30 concurrent Claude
sessions. There's no visibility into cost per task, no budget controls, no model
selection optimization, and no idle detection to prevent wasted spend.

**Who Benefits:** Human (primary), Deacon

**Impact:** MEDIUM-HIGH | **Effort:** M

**Implementation Sketch:**
- Build on existing `costs-record` Stop hook:
  - Aggregate costs by: agent, rig, bead, convoy, time period
  - `gt costs report` - Show cost breakdown
  - `gt costs budget set <rig> <amount/day>` - Set spending limits
  - `gt costs alert` - Notify when budget exceeded (via escalation system)
- Model selection optimization:
  - Tag beads with complexity: simple (Haiku), medium (Sonnet), complex (Opus)
  - Auto-select model based on bead tags + historical cost/quality ratio
  - `gt config model-policy` - Set per-rig model selection rules
- Idle detection:
  - Deacon patrol checks for idle agents (no tool calls in N minutes)
  - Auto-park idle agents to stop token burn
  - `gt idle report` - Show agent utilization rates
- Token optimization:
  - Track context window utilization per agent
  - Auto-trigger handoff when context is 80% full (prevent expensive compaction)
  - Measure tokens-per-completion as efficiency metric

**Key insight:** The costs-record hook already captures raw data. The missing piece
is aggregation, budgeting, and automated response to cost signals.

---

### 9. CI/CD Integration ("The Pipeline")

**Problem:** Gas Town operates alongside CI/CD but doesn't integrate with it.
When a polecat pushes code, CI runs independently. Failures aren't automatically
routed back as beads. There's no "watch CI and fix if broken" workflow.

**Who Benefits:** Polecats, Refinery, Human

**Impact:** MEDIUM | **Effort:** M

**Implementation Sketch:**
- GitHub Actions integration:
  - `gt ci watch <pr>` - Monitor CI status, create bead on failure
  - `gt ci fix <check-id>` - Auto-create bug bead from CI failure + sling to rig
  - GitHub webhook receiver (via `github-sheriff` plugin pattern)
- CI-aware merge queue:
  - Refinery waits for CI green before merging (currently: push + hope)
  - `gt refinery policy ci-required` - Enforce CI pass before merge
  - On CI failure: auto-create rework bead, sling back to original polecat
- Build caching:
  - Share build artifacts across polecat worktrees
  - `gt cache warm` - Pre-build common dependencies
- Test impact analysis:
  - Track which files affect which tests
  - Auto-run only affected tests in polecat sessions (faster feedback)

**Key insight:** The Refinery already manages merge ordering. Adding CI awareness
transforms it from "merge queue" to "quality gate."

---

### 10. Command Taxonomy & Progressive Disclosure ("The Map")

**Problem:** 285 command files with flat hierarchy. Agents and humans can't discover
capabilities. `gt --help` is overwhelming. New agents waste tokens exploring
commands. There's no concept of "beginner" vs "advanced" commands.

**Who Benefits:** Everyone (especially new agents and humans)

**Impact:** MEDIUM | **Effort:** S

**Implementation Sketch:**
- Reorganize commands into clear namespaces:
  ```
  gt work     - Find, claim, complete work (bd ready, bd show, bd close)
  gt ship     - Push, merge, deploy (gt done, gt refinery)
  gt talk     - Communicate (gt mail, gt nudge, gt escalate)
  gt agents   - Manage agents (gt polecat, gt crew, gt witness)
  gt town     - Town management (gt rig, gt config, gt doctor)
  gt observe  - Monitoring (gt costs, gt metrics, gt dashboard)
  gt learn    - Context (gt prime, gt memory, gt help)
  ```
- Add `gt quickstart` - Interactive tutorial for new agents
- Add `gt suggest` - Context-aware command suggestion ("did you mean...?")
- Progressive disclosure: `gt --help` shows 7 top-level groups, each expands
- Agent-role-aware help: Witness sees Witness-relevant commands first
- Command frequency tracking: Surface most-used commands per role

**Key insight:** This is the lowest-effort, highest-quality-of-life improvement.
Every agent wastes tokens on command discovery every session.

---

## Priority Matrix

| # | Feature | Impact | Effort | Priority Score | Ship Order |
|---|---------|--------|--------|----------------|------------|
| 10 | Command Taxonomy | MEDIUM | S | 8.0 | 1st - Quick win |
| 3 | Self-Healing Recovery | HIGH | L | 7.5 | 2nd - Reliability |
| 2 | Observability Dashboard | HIGH | M | 7.0 | 3rd - Visibility |
| 8 | Cost Optimization | MEDIUM-HIGH | M | 6.5 | 4th - Sustainability |
| 1 | Intelligent Work Router | HIGH | L | 6.5 | 5th - Intelligence |
| 4 | NL Task Dispatch | HIGH | M | 6.0 | 6th - Onboarding |
| 7 | Agent Memory & Learning | MEDIUM-HIGH | L | 6.0 | 7th - Knowledge |
| 6 | Plugin Ecosystem | MEDIUM | L | 5.5 | 8th - Extensibility |
| 9 | CI/CD Integration | MEDIUM | M | 5.0 | 9th - Automation |
| 5 | Cross-Town Federation | MEDIUM-HIGH | XL | 4.0 | 10th - Scale |

**Recommended first sprint:** Features 10, 3, and 2 (Command Taxonomy, Self-Healing,
Observability) - they reduce friction for everything else.

---

## Existing Work Alignment

Cross-referencing with current beads:

| Bead | Status | Relates To |
|------|--------|------------|
| gt-4vi (Token optimization) | Open | Feature 8 (Cost Optimization) |
| gt-c3w (Race in done.go) | In Progress | Feature 3 (Self-Healing) |
| gt-e4u (Race in Daemon) | Open | Feature 3 (Self-Healing) |
| gt-u9e (Test coverage) | In Progress | Foundation for all features |
| gt-qzp (Test failure) | Open | Foundation work |

Recent PR activity (20 merged in last 24h) shows heavy investment in:
- Reliability fixes (stale polecat detection, scanner errors, branch guards)
- Formula management (canonical source of truth, sync)
- Test coverage expansion
- Agent lifecycle improvements (process names, handoff)

This validates Features 3 (Self-Healing) and 10 (Command Taxonomy) as high-priority:
the team is already spending significant effort on reliability and discoverability.

---

## Open Questions for Engineer

1. **Model diversity:** Should the Intelligent Work Router consider routing simple tasks
   to cheaper models (Haiku) and complex tasks to Opus? Or is model consistency preferred?

2. **Federation scope:** Is cross-town federation a near-term need or a long-term vision?
   This determines whether to invest in the XL effort now or defer.

3. **Plugin security:** Should plugins be sandboxed? A malicious plugin.md could instruct
   a Dog to execute arbitrary commands. Is the Deacon's trust model sufficient?

4. **Memory persistence:** Should agent memories survive across major version upgrades?
   This affects storage format choices.

5. **Cost attribution:** When a convoy spans multiple rigs, which rig "owns" the cost?
   This matters for budget enforcement.

---

*Filed as bead gt-ire. Review and select items for the roadmap.*
