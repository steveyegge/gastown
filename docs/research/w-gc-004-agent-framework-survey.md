# Survey: Agent Orchestration Frameworks vs Gas City

**Wanted:** w-gc-004 — Survey existing agent orchestration frameworks
**Completed by:** tmchow
**Date:** 2026-03-08

## Executive Summary

This survey examines seven major multi-agent orchestration frameworks and compares their role/task models to Gas City's declarative approach. The frameworks span a spectrum from minimal (OpenAI Swarm) to enterprise-grade (Microsoft Agent Framework), each with distinct philosophies on how agents are defined, coordinated, and equipped with tools.

Gas Town's key differentiator is its **process-model architecture** — persistent, named roles (Mayor, Witness, Polecat, etc.) with fixed responsibilities that communicate via hooks and beads, backed by Git and Dolt for crash-surviving state. This is closer to an operating system's process model than to the LLM-conversation-as-control-plane pattern used by most frameworks.

Gas City is the planned declarative layer on top of Gas Town — a role format and formula engine that would make Gas Town's patterns portable and user-definable. It doesn't exist yet as a standalone product (this is what w-gc-001 and w-gc-003 aim to build).

---

## Gas Town / Gas City: The Reference Architecture

Before comparing external frameworks, it's important to understand Gas Town's architecture in detail, since the goal is to identify what to borrow for Gas City's declarative role format.

**Repository:** [github.com/steveyegge/gastown](https://github.com/steveyegge/gastown) (Go, the `gt` binary)
**Companion:** [github.com/steveyegge/beads](https://github.com/steveyegge/beads) (Go, the `bd` binary)
**Current version:** v0.6.0

### Role Hierarchy (Four Layers)

Gas Town defines operational roles across a layered hierarchy. Roles are implemented as **Go template files** at `internal/templates/roles/*.md.tmpl` and injected into Claude Code sessions via the `gt prime` command. The `GT_ROLE` environment variable determines which role template gets rendered. Role detection also works by examining the current working directory path (e.g., `<rig>/witness/rig/` triggers the Witness role).

**Infrastructure Layer:**
- **Boot**: Handles initial context injection during startup — has its own template (`boot.md.tmpl`)
- **Deacon**: Central health supervisor at `~/gt/deacon/` — a "daemon beacon" running continuous Patrol cycles. Monitors system health, ensures worker activity, triggers recovery. Notorious early on for bugs (Yegge warned about it "spree-killing all the other workers while on patrol" before v0.4.0 fixes)
- **Dogs**: Deacon helper agents for infrastructure tasks (ephemeral, NOT for user work)

**Global Coordination Layer:**
- **Mayor**: Town-level coordinator at `~/gt/mayor/` — the human's primary AI concierge. Initiates Convoys, distributes work, coordinates across all Rigs. The Mayor CAN and SHOULD edit code when it is the fastest path. "Gas Town is a steam engine and the Mayor is the main drive shaft."

**Per-Rig Management Layer:**
- **Witness**: Per-rig patrol agent — oversees Polecats and Refinery, monitors progress, detects stuck agents, triggers recovery
- **Refinery**: Per-rig merge queue processor — handles quality control, merge conflict resolution, branch cleanup

**Worker Layer:**
- **Polecats**: Ephemeral workers spawning for single tasks, then terminated. Each gets its own **git worktree** (lightweight, sharing the bare repo) for complete isolation.
- **Crew**: Persistent helper agents for extended work — long-lived, user-managed, with their own full **clones** (not worktrees). Ideal for ongoing work relationships.

### Agent Identity (Three Persistent Elements)

Each agent has:
1. **Role Bead**: Defines rules and priming instructions for the role
2. **Agent Bead**: Persistent identity that survives session restarts — forms the foundation of the CV/reputation ledger
3. **Hook**: Bead-backed queue where work attaches

This separation of identity from session is a key differentiator — sessions are ephemeral, but the agent's identity and work state persist in Dolt. Every completion is recorded, every handoff logged, every closed bead becomes part of a permanent capability ledger.

### How Priming Works

When a session starts, `gt prime` executes a multi-step context injection:
1. Checks for slung work on the agent's hook
2. Detects autonomous mode and adjusts behavior
3. Outputs molecule context if working on a molecule step
4. Outputs previous session checkpoint for crash recovery
5. Runs `bd prime` to output beads workflow context
6. Runs `gt mail check --inject` to inject pending mail

### GUPP: Gas Town Universal Propulsion Principle

The scheduling axiom: **"If there is work on your hook, YOU MUST RUN IT."**

This ensures:
- Agents check hooks on startup and resume work automatically
- Work persists across session crashes via Git-backed state
- No central scheduler needed — pull-based execution model
- No confirmation, no questions, no waiting — immediate execution

### The Hook System

Every worker has a dedicated **hook** — a pinned bead where work is attached. The flow:

1. Work is assigned via `gt sling <bead-id> <rig>`
2. The work (a molecule) lands on the target agent's hook
3. GUPP activates: the agent detects work and executes immediately
4. On completion, the hook is cleared and the next molecule jumps to front

Communication primitives around hooks:
- **Mail**: Asynchronous persisted messaging for inter-agent coordination
- **Nudge**: Direct session injection via tmux (`gt nudge`)
- **Peek**: Status check without interruption (`gt peek`)

### The Molecule/Formula Stack (Workflow Primitives)

Layered workflow abstraction from template to execution:

| Level | Name | CLI Command | Description |
|---|---|---|---|
| Source | **Formula** | — | TOML source files at `internal/formula/formulas/` — defines loops, gates, composition |
| Compiled | **Protomolecule** | `bd cook` | Compiled, git-frozen workflow template ready for deployment |
| Active | **Molecule (Mol)** | `bd mol pour` | Running workflow instance tracked in beads, crash-surviving |
| Ephemeral | **Wisp** | — | Lightweight workflow existing only in memory during patrol cycles |

Example: `internal/formula/formulas/release.formula.toml` defines a "Standard release process" with steps: bump-version → run-tests → build → create-tag → publish.

### Gates (Async Coordination)

Blocking conditions that enable work suspension without blocking other tasks:
- `gh:run` — wait for GitHub Actions completion
- `gh:pr` — wait for pull request events
- `timer` — wait for duration to elapse
- `human` — wait for manual approval
- `mail` — wait for message from another agent

### Beads (Work Tracking)

Beads are atomic work items stored in a **Dolt database** (version-controlled SQL with Git semantics). As of Beads v0.51.0, Dolt is the exclusive backend — the old SQLite + JSONL pipeline was removed. Dolt's **cell-level merge** means concurrent updates from multiple agents can be resolved automatically at the column level, not the line level — critical for multi-agent operation.

Bead IDs use the format `prefix-XXXXX` (e.g., `gt-abc12`, `hq-x7k2m`), with hash-based IDs to prevent merge collisions across agents and branches. Beads transition through states: `open` → `working` → `done`/`parked`. The terms "bead" and "issue" are interchangeable.

**Bead types:**
- **Issue beads**: Work items with IDs, descriptions, status, assignees, dependencies, and blockers
- **Agent beads**: Identity beads tracking agent state and hooks — foundation of the reputation ledger
- **Hook beads**: Special pinned beads serving as an agent's work queue
- **Convoy beads**: Collections wrapping work items into trackable delivery units

**Higher-level aggregations:**
- **Epics**: Hierarchical collections organizing beads into trees (e.g., `bd-a3f8e9.1`, `bd-a3f8e9.2`)
- **Convoys**: Delivery units tracking composed goals like releases
- **Patrols**: Recurring workflows for queue cleanup and health checks

### Escalation Hierarchy

- **Tier 1** (Deacon): Infrastructure failures and daemon issues
- **Tier 2** (Mayor): Cross-rig coordination and resource conflicts
- **Tier 3** (Overseer/Human): Design decisions and human judgment

### Workspace Structure

```
~/gt/
├── deacon/          # Infrastructure agent
├── mayor/           # Town coordinator
├── <rig-name>/      # Per-project directory
│   ├── witness/     # Polecat supervisor
│   ├── refinery/    # Merge processor
│   ├── crew/        # Persistent helpers
│   ├── polecats/    # Ephemeral workers
│   └── rig/         # Git repository
└── routes.jsonl     # Prefix routing config
```

### Current State (v0.6.0, March 2026)

Gas Town is early-stage (released Jan 2026) but rapidly evolving — 1500+ GitHub issues, 450+ contributors. v0.6.0 added convoy ownership, checkpoint-based crash recovery, an agent factory with data-driven preset registry, Gemini and Copilot CLI integrations, non-destructive nudge delivery, and submodule support. Community ecosystem is growing: a Kubernetes operator (gastown-operator), a web GUI (gastown-gui), and a Rust port of beads.

The **Wasteland** federation layer just landed (PR #1552, March 2026) — linking thousands of Gas Towns into a trust-scored labor marketplace via Dolt and DoltHub. This is the system we're using right now to track this very task.

**Gas City** — the declarative role format and formula engine layer — is the planned next step. Currently, roles are defined as Go templates; Gas City would make them portable, user-definable, and composable via a structured schema. This survey directly informs that design.

---

## External Framework Summaries

### 1. AutoGen (Microsoft) → Microsoft Agent Framework

**Latest:** v0.4.7 (being superseded by Microsoft Agent Framework, RC Feb 2026)
**Repo:** [github.com/microsoft/autogen](https://github.com/microsoft/autogen)

**Role Model:**
- Agents defined via `AssistantAgent`, `UserProxyAgent`, `CodeExecutorAgent`, etc.
- Each agent gets a system message (role prompt), model client, and tool list.
- AutoGen 0.4 uses an event-driven, actor-based architecture with layered APIs: Core (messaging primitives) and AgentChat (high-level abstractions).

**Task Model:**
- Tasks are implicit — you pass a message string to a team, and the team orchestrates agents to solve it.
- No dedicated `Task` class. The conversation IS the task.
- Termination conditions (`MaxMessageTermination`, `TextMentionTermination`) define when the task is "done."

**Coordination:**
- Agents grouped into teams: `RoundRobinGroupChat`, `SelectorGroupChat`, `Swarm`.
- `SelectorGroupChat` uses an LLM to pick the next speaker each turn.
- `Swarm` mode uses explicit `HandoffMessage` for agent-to-agent routing.

**Minimal Example:**
```python
from autogen_agentchat.agents import AssistantAgent
from autogen_agentchat.teams import RoundRobinGroupChat
from autogen_agentchat.conditions import MaxMessageTermination

analyst = AssistantAgent("analyst", model_client=client,
    system_message="You analyze code for bugs and security issues.")
fixer = AssistantAgent("fixer", model_client=client,
    system_message="You fix bugs identified by the analyst.")

team = RoundRobinGroupChat([analyst, fixer],
    termination_condition=MaxMessageTermination(6))
result = await team.run(task="Review and fix auth.py")
```

**Tools:** Python functions wrapped as `FunctionTool`, assigned at agent construction.

**Key Strength:** Flexible orchestration patterns, strong async support, AutoGen Studio UI.

**Key Limitation:** Chat-as-control-flow can be opaque; the 0.2→0.4 transition created fragmentation. Now being merged into Microsoft Agent Framework — AutoGen will receive only bug fixes going forward.

**Status:** AutoGen and Semantic Kernel are merging into the **Microsoft Agent Framework** (GA targeted Q1 2026). The new framework adds a graph-based workflow API on top of Semantic Kernel's plugin model, combining AutoGen's multi-agent patterns with SK's enterprise foundations.

### 2. CrewAI

**Latest:** v1.10.1 (March 4, 2026)
**Repo:** [github.com/crewAIInc/crewAI](https://github.com/crewAIInc/crewAI)

**Role Model:**
- The **role / goal / backstory** triad — agents defined with a job title, objective, and narrative context.
- Deliberately intuitive: mirrors how you'd brief a human specialist.

**Task Model:**
- Explicit `Task` class with `description`, `expected_output`, and assigned `agent`.
- Tasks chain via `context` parameter — output of one task feeds into another.
- Supports structured output via `output_pydantic` or `output_json`.
- `human_input=True` pauses for human review before continuing.

**Coordination:**
- Process types: **Sequential** (ordered pipeline), **Hierarchical** (manager agent delegates).
- **Flows** (new in v1.x): Event-driven architecture using `@start()` and `@listen()` decorators for granular control. Complements the autonomous Crews pattern.

**Minimal Example:**
```python
from crewai import Agent, Task, Crew, Process

researcher = Agent(
    role="Senior Research Analyst",
    goal="Find cutting-edge AI developments",
    backstory="You are a veteran analyst at a leading think tank...",
    tools=[search_tool], llm="gpt-4o"
)
task = Task(
    description="Analyze latest AI agent frameworks",
    expected_output="A 3-paragraph summary with key comparisons",
    agent=researcher
)
crew = Crew(agents=[researcher], tasks=[task], process=Process.sequential)
result = crew.kickoff()
```

**Flows Example:**
```python
from crewai.flow.flow import Flow, listen, start

class ResearchFlow(Flow):
    @start()
    def gather_data(self):
        return search("agent frameworks 2026")

    @listen(gather_data)      # triggered when gather_data completes
    def analyze(self, data):
        return summarize(data)
```

**Tools:** Agent-level or task-level tool assignment. Built-in tools via `crewai-tools` package. Custom tools via `BaseTool` subclass or `@tool` decorator.

**Key Strength:** Lowest learning curve. The role/goal/backstory metaphor is immediately intuitive. Now fully independent of LangChain (rewritten from scratch).

**Key Limitation:** Limited control flow compared to graph-based frameworks. Sequential/hierarchical are the main patterns — no arbitrary branching or cycles. Token-heavy in hierarchical mode.

### 3. LangGraph (LangChain)

**Latest:** v1.0 (stable, no breaking changes until 2.0)
**Repo:** [github.com/langchain-ai/langgraph](https://github.com/langchain-ai/langgraph)

**Role Model:**
- Agents/nodes are **Python functions** in a `StateGraph`. Each node receives state, does work, returns state updates.
- Pre-built `create_react_agent()` for standard tool-calling loops.
- Sub-graphs can be embedded as nodes for hierarchical/multi-agent designs.

**Task Model:**
- No explicit task abstraction. The graph IS the task — you invoke it with initial state, and it flows through nodes.
- State is a `TypedDict` with annotated reducer functions for merging updates.
- Tasks are implicit in the graph topology and termination conditions.

**Coordination:**
- **Directed graph** with explicit edges (normal, conditional, cyclic).
- Conditional edges: a routing function inspects state and returns the next node name.
- **Cycles are first-class** — native support for iterative agent loops (ReAct, reflection).

**Minimal Example:**
```python
from langgraph.graph import StateGraph, START, END
from typing import TypedDict, Annotated
from langgraph.graph.message import add_messages

class State(TypedDict):
    messages: Annotated[list, add_messages]

def call_model(state: State):
    response = model.invoke(state["messages"])
    return {"messages": [response]}

def should_continue(state: State):
    if state["messages"][-1].tool_calls:
        return "tools"
    return END

graph = StateGraph(State)
graph.add_node("agent", call_model)
graph.add_node("tools", ToolNode([search_tool]))
graph.add_edge(START, "agent")
graph.add_conditional_edges("agent", should_continue)
graph.add_edge("tools", "agent")
app = graph.compile()
```

**Tools:** Bound to the model via `.bind_tools()`. `ToolNode` auto-executes tool calls. Per-agent scoping via sub-graphs.

**Key Strength:** Explicit topology control — you see and control exactly how nodes connect. First-class streaming, human-in-the-loop (`interrupt_before`/`interrupt_after`), and checkpointing (persistence, time-travel, fault recovery). Most flexible of all frameworks.

**Key Limitation:** Higher ceremony for simple use cases. Tight coupling to LangChain message schemas. Shared state type across all nodes can be constraining.

### 4. OpenAI Agents SDK (successor to Swarm)

**Latest:** Production-ready SDK (replaced Swarm in 2026)
**Repo:** [github.com/openai/openai-agents-python](https://openai.github.io/openai-agents-python/)

**Role Model:**
- Agents defined with `name`, `instructions` (system prompt), and `tools`/`handoffs`.
- Originally Swarm (~300 LOC, educational) → evolved into the Agents SDK with production features.

**Task Model:**
- No explicit task object. You call `Runner.run(agent, "your request")` and the agent handles it.
- Handoffs to other agents are represented as tools to the LLM (e.g., `transfer_to_refund_agent`).
- The Runner handles executing agents, handoffs, and tool calls.

**Coordination:**
- **Handoffs** are the core primitive — listed in the agent's `handoffs` param.
- No central orchestrator, planner, or DAG. Just agents handing off to each other.
- Agents SDK adds: guardrails, human-in-the-loop, tracing with dashboard viewer, session management.

**Minimal Example:**
```python
from agents import Agent, handoff, Runner

billing_agent = Agent(name="Billing agent",
    instructions="You handle billing inquiries.")
refund_agent = Agent(name="Refund agent",
    instructions="You process refund requests.")
triage_agent = Agent(name="Triage agent",
    instructions="Route customer requests to the right agent.",
    handoffs=[billing_agent, handoff(refund_agent)])

result = await Runner.run(triage_agent, "I want a refund")
print(f"Handled by: {result.last_agent.name}")  # "Refund agent"
```

**Tools:** Plain Python functions attached to agents. Auto-converted to JSON schemas.

**Key Strength:** Extreme simplicity and transparency. Minimal abstraction. The Agents SDK adds production-grade tracing and guardrails while keeping the handoff model.

**Key Limitation:** No complex routing, branching, or cycles beyond what handoffs express. Best for linear or tree-shaped workflows.

### 5. Claude Agent SDK (Anthropic)

**Latest:** Renamed from Claude Code SDK (March 2026)
**Repo:** [github.com/anthropics/claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python)

**Role Model:**
- Agents defined with a model, instructions (system prompt), and tools.
- Emphasizes a **single-agent-with-tools** pattern — one powerful agent with rich tool access.
- Multi-agent via hierarchical delegation: agents invoke sub-agents as tools.

**Task Model:**
- Single-agent loop: you give the agent a prompt, it reasons and calls tools until done.
- Sub-agents are invoked as tools within the parent agent's loop.
- No separate task abstraction — the prompt IS the task.

**Coordination:**
- Agent loop: Claude iteratively reasons → calls tools → processes results.
- The SDK manages the conversation turn cycle, tool execution, and result injection.
- Sub-agent delegation for multi-agent scenarios.

**Tools:** Custom tools implemented as **in-process MCP servers** (run inside your Python app, no separate process). Built-in tools: computer use, text editor, bash execution.

**Key Strength:** Deep integration with Claude's native capabilities (tool use, extended thinking). MCP-based tool system is composable and standards-based. Safety guardrails and human-in-the-loop built in.

**Key Limitation:** Less suited for swarm-style multi-agent patterns. The single-agent-with-delegation model is powerful but different from peer-to-peer agent coordination.

### 6. Google ADK (Agent Development Kit)

**Latest:** Updated Feb 26, 2026. Python + TypeScript support.
**Repo:** [github.com/google/adk-python](https://github.com/google/adk-python)

**Role Model:**
- Agents defined with `instructions`, `tools`, and `sub_agents` — supports a **tree-like hierarchy**.
- Code-first philosophy: "agent development should feel like software development."

**Task Model:**
- You send a message to the root agent; it delegates to sub-agents as needed.
- Built-in composite patterns handle orchestration: `SequentialAgent`, `LoopAgent`, `ParallelAgent`.
- Session state persists across interactions.

**Coordination:**
- Agent-as-tool pattern: parent agents delegate to child agents.
- Built-in patterns: `SequentialAgent` (pipeline), `LoopAgent` (iterative), `ParallelAgent`.
- Session management and state persistence built in.

**Minimal Example:**
```python
from google.adk import Agent

code_reviewer = Agent(
    name="code_reviewer",
    instructions="Review code for bugs and style issues.",
    tools=[lint_tool, test_tool]
)
coordinator = Agent(
    name="coordinator",
    instructions="Coordinate code review and fixes.",
    sub_agents=[code_reviewer, fixer_agent]
)
```

**Tools:** Python functions, API calls, or other agents. Expanding ecosystem with partner integrations (Hugging Face, GitHub, GitLab, Postman, Daytona).

**Key Strength:** Tight Google Cloud / Gemini integration. Web-based developer UI for testing/debugging. Growing third-party ecosystem. Model-agnostic despite Gemini optimization.

**Key Limitation:** Younger than competitors. Ecosystem still maturing. Tightest integration path locks you into Google Cloud.

### 7. Microsoft Agent Framework (Semantic Kernel + AutoGen)

**Latest:** Release Candidate, Feb 19, 2026 (GA targeted end of Q1 2026)
**Repo:** [learn.microsoft.com/en-us/agent-framework](https://learn.microsoft.com/en-us/agent-framework/overview/)

**Role Model:**
- Merges Semantic Kernel's **Kernel + Plugin** model with AutoGen's **multi-agent patterns**.
- Agents get a kernel (plugins + services), instructions, and participate in group chats.
- Plugins are collections of `@kernel_function`-decorated methods — highly composable.

**Task Model:**
- Graph-based workflow API defines multi-step, multi-agent task flows.
- `AgentGroupChat` with selection/termination strategies for conversational tasks.
- Process framework for structured, non-conversational workflows.

**Coordination:**
- New **graph-based workflow API** for complex multi-step, multi-agent workflows.
- Built-in orchestration patterns: sequential, parallel, Magentic (web + code + file agents).
- `AgentGroupChat` with selection strategies (round-robin, LLM-selected) and termination strategies.

**Plugin Example:**
```python
from semantic_kernel.functions import kernel_function

class GitPlugin:
    @kernel_function(description="Merge a branch into main")
    def merge_branch(self, branch_name: str) -> str:
        return git_merge(branch_name)

    @kernel_function(description="Run test suite")
    def run_tests(self) -> str:
        return execute_tests()
```

**Tools:** Plugin model — functions grouped into plugins, registered on kernels, selectively available to agents. Most mature tool/plugin system of any framework.

**Key Strength:** Enterprise-grade. Multi-language (Python, C#, Java). Mature plugin system. The merger creates the most comprehensive Microsoft offering for agentic AI.

**Key Limitation:** Heaviest framework. Migration path from AutoGen or SK required. Complexity may be overkill for simple use cases.

---

## Comparison Matrix

| Dimension | Gas Town | CrewAI | LangGraph | AutoGen/MAF | OpenAI Agents SDK | Claude Agent SDK | Google ADK |
|---|---|---|---|---|---|---|---|
| **Role definition** | Named operational roles (Mayor, Witness, Polecat) with Role Beads + Agent Beads | role/goal/backstory triad | Graph nodes (functions) | Agent classes + system prompts | instructions + handoffs | instructions + tools (MCP) | instructions + tools + sub_agents |
| **Task definition** | Beads (atomic work items with ID, status, assignee) + Molecules (multi-step workflows) | `Task` class with description + expected_output | Graph topology IS the task (invoke with initial state) | Implicit (message to team) | Implicit (message to Runner) | Implicit (prompt to agent loop) | Implicit (message to root agent) |
| **Role persistence** | Persistent (identity survives restarts via Agent Bead + GUPP) | Per-kickoff | Checkpoint-based | Per-session | Per-session | Per-session | Session-managed |
| **Coordination** | Hook + GUPP (pull-based) + molecules (workflow graphs) | Sequential / Hierarchical / Flows | Explicit graph edges | GroupChat + strategies / Graph workflows | Agent handoffs | Agent loop + delegation | Hierarchical + Sequential/Loop/Parallel |
| **Communication** | Hooks (bead-backed queues) + mail (async) + nudge/peek (direct) | Task context chaining | Typed state + reducers | Async messages / GroupChat | Conversation + handoff returns | Conversation turns | Message passing |
| **Workflow definition** | Formulas (TOML) → Protomolecules → Molecules + Gates | Tasks with expected_output | Graph edges (code) | GroupChat config | Routines (instructions) | Agent loop | SequentialAgent / LoopAgent |
| **Tool model** | Claude Code native tools per role | Agent/task-level tool lists | bind_tools() + ToolNode | FunctionTool on agents | Python functions | MCP servers (in-process) | Functions + partner integrations |
| **Parallelism** | 20-30 parallel polecats (OS processes, Git worktrees) | async_execution flag, kickoff_for_each | Parallel branches in graph | Async messaging, distributed runtime | Single-threaded handoffs | Single agent loop | ParallelAgent pattern |
| **State management** | Beads (Dolt database, cell-level merge) + bead state machine (open→working→done) | Crew memory (short/long/entity) | Checkpointed typed state | Message history | Conversation context | Conversation context | Session state + persistence |
| **Human-in-the-loop** | Tier 3 escalation + `human` gate + Mayor oversight | human_input flag on tasks | interrupt_before/after on nodes | UserProxyAgent | Agents SDK guardrails | Built-in safety patterns | Built-in support |
| **Maturity** | Early (Jan 2026, 450+ contributors) | Stable (v1.10.1) | Stable (v1.0) | Transitioning (→ MAF RC) | Production-ready | Active development | Growing (Feb 2026 update) |

---

## Gaps in Gas Town's Model (Relative to Frameworks)

### 1. No Declarative Role Schema (Yet)
Gas Town has named operational roles with Role Beads containing priming instructions, but roles are currently defined as **Go template files** (`internal/templates/roles/*.md.tmpl`) compiled into the `gt` binary. This means adding or modifying roles requires changing Go source code and recompiling. CrewAI's role/goal/backstory triad and Google ADK's instructions/tools/sub_agents format are both user-facing, documented, and easy to extend at runtime. **This is exactly what w-gc-001 aims to solve** — extracting role definitions into a structured, parseable format (YAML/TOML) that users can customize without touching Go code.

### 2. Formulas Are TOML-Only, Not Yet a Full DSL
Gas Town's Formula → Protomolecule → Molecule pipeline is a powerful workflow abstraction, but formulas are currently TOML files with an ad-hoc structure. Compare to LangGraph's typed Python graph definitions or Microsoft Agent Framework's graph-based workflow API — both offer IDE support, type checking, and programmatic composition. The formula engine (w-gc-003) could benefit from a more structured DSL or schema.

### 3. No Standardized Tool Schema
Frameworks like Claude Agent SDK (MCP), Microsoft Agent Framework (plugins/kernel functions), and Google ADK (partner integrations) have well-defined tool interfaces. Gas Town agents use Claude Code's native tools, but there's no Gas Town-specific tool abstraction, registry, or per-role tool scoping.

### 4. Observability & Tracing
OpenAI Agents SDK and Microsoft Agent Framework both include built-in tracing (OpenTelemetry, dashboard viewers). LangGraph has LangSmith integration. Gas Town has deacon/witness patrols, peek/nudge, and bead state transitions — these are functional but organic. There's no structured trace format, no query-able span data, no timeline visualization.

### 5. Dynamic Routing Is Gate-Based, Not Graph-Based
Gas Town has gates (gh:run, gh:pr, timer, human, mail) for async coordination, which is good. But compare to LangGraph's conditional edges where a function inspects state and routes to different nodes — Gas Town's molecules don't currently support arbitrary conditional branching based on agent output. The GUPP principle ("execute immediately") optimizes for throughput over routing flexibility.

### 6. Evolving Cross-Framework Portability
Gas Town was originally Claude Code-only, but v0.6.0 added Gemini and Copilot CLI integrations. However, the role template system (`gt prime` injecting Go templates via CLAUDE.md conventions) is still deeply tied to Claude Code's priming model. Every other framework surveyed is model-agnostic in its core abstractions. If Gas City aims to be a portable protocol, the role format should abstract over the LLM runtime, with adapter layers for Claude Code, Gemini, etc.

---

## Borrowable Patterns for Gas City

### 1. CrewAI's Role/Goal/Backstory Triad → Gas City Role Format
The triad is simple, intuitive, and proven at scale (v1.10, 30k+ stars). Gas Town already has Role Beads with priming instructions — the next step is to formalize this into a schema. A Gas City role definition could combine CrewAI's persona pattern with Gas Town's operational specifics:
```yaml
role: Witness
goal: Monitor polecat health and detect stuck workers
backstory: You oversee all active polecats in this rig...
layer: rig-management          # Gas Town hierarchy layer
tools: [patrol, health-check, escalate]
hooks: [polecat-completion, merge-ready]
gates: [human, timer]
constraints: [read-only-unless-escalating]
escalation_tier: 2
```
This maps Gas Town's existing patterns into a portable, user-definable format.

### 2. LangGraph's Checkpoint + Time-Travel → Bead Versioning
Gas Town already has Git-backed beads, which is philosophically similar to LangGraph's checkpointing. LangGraph v1.0 makes **time-travel debugging** a first-class feature (replay from any checkpoint, inspect state at any node). Gas Town could expose this explicitly: "show me the state of bead mp-001 at commit X" or "replay this polecat's molecule from the last gate." The Git history is already there — it just needs a query layer.

### 3. OpenAI Agents SDK's Typed Handoff → Formalizing SLING → HOOK → GUPP
Gas Town's sling/hook/GUPP flow is functionally similar to the Agents SDK's handoff pattern, but implemented via file-based hooks rather than in-process returns. The Agents SDK adds **tracing and span data** to handoffs, making the flow observable. Gas Town could add structured handoff events: who slung what to whom, when GUPP activated, what gate was hit.

### 4. Google ADK's Agent Hierarchy → Gas City's Town/Rig/Worker Model
Google ADK's tree-like agent hierarchy (agents with `sub_agents`) closely mirrors Gas Town's Town → Rig → Worker structure. ADK's built-in patterns are directly analogous:
- `SequentialAgent` → Refinery merge queue (sequential processing)
- `LoopAgent` → Deacon patrol cycles (recurring workflows)
- `ParallelAgent` → Polecat swarm (parallel execution)

These could inform how Gas City's formula engine (w-gc-003) composes agent behaviors declaratively.

### 5. Microsoft Agent Framework's Plugin Model → Gas City Tool Scoping
The Kernel + Plugin pattern (functions grouped into named plugins, selectively available to agents) could inform role-based tool scoping. Gas Town currently gives all agents Claude Code's full tool set. With plugins:
```yaml
role: Refinery
plugins: [git-merge, conflict-resolution, test-runner]
# Does NOT get: file-create, web-search, etc.
```
This maps to the principle of least privilege — agents only get the tools their role requires.

### 6. CrewAI Flows → Typed Hook Events
CrewAI's new Flows feature (event-driven, granular control) is philosophically aligned with Gas Town's hook + mail system. The key addition is **event typing** — Flows define explicit event types and handlers with schemas. Gas Town's hooks currently accept molecules (workflow instances) but the hook-trigger mechanism could be formalized:
```toml
[hook.events]
polecat_complete = { schema = "bead_id, branch, test_result" }
merge_conflict = { schema = "bead_id, conflicting_files" }
escalation = { schema = "tier, source_agent, reason" }
```

### 7. LangGraph's Conditional Edges → Formula Branching
LangGraph's conditional routing (a function inspects state and returns the next node) could enhance Gas Town's molecule system. Currently, molecules are linear with gates for async waits. Adding conditional branching:
```toml
[molecule.steps]
run_tests = { next_on_pass = "merge", next_on_fail = "notify_witness" }
```
This would make formulas more expressive without abandoning GUPP's pull-based execution model.

### 8. Gas Town's Unique Patterns (What Others Should Borrow)
It's worth noting what Gas Town does that no other framework matches:
- **GUPP** — pull-based, crash-surviving execution. No other framework has a comparable "work persists and auto-resumes" primitive.
- **Git-worktree isolation** — each polecat in its own worktree. Most frameworks share state; Gas Town gives each worker its own filesystem.
- **Separation of identity from session** — Agent Beads survive session crashes. Other frameworks lose agent state when sessions end. Every completion becomes part of a permanent capability ledger.
- **Dolt cell-level merge** — concurrent updates from multiple agents are resolved at the column level, not the line level. This is why 20-30 parallel agents can write to the same beads database without constant conflicts. No other framework has this.
- **Formula → Protomolecule → Molecule pipeline** — compiled, versioned workflow definitions (`bd cook` → `bd mol pour`). Closest analog is LangGraph's compiled graphs, but Gas Town's are Git-frozen and crash-surviving.
- **Escalation tiers** — structured escalation from Deacon (Tier 1) → Mayor (Tier 2) → Human (Tier 3) is more nuanced than most frameworks' binary human-in-the-loop.
- **The Wasteland** — no other agent framework has a concept of federating multiple users' agent workspaces into a trust-scored labor marketplace.

---

## Recommendations

1. **Design the Gas City role format as YAML with Gas Town operational fields** (feeds directly into w-gc-001). Borrow CrewAI's role/goal/backstory for persona, but add Gas Town-specific fields: layer, hooks, gates, escalation_tier, tools/plugins. The Role Bead should be generated from this YAML.

2. **Formalize hook events with schemas** — Gas Town's hooks are powerful but untyped. Add event schemas so that slung work, completions, escalations, and gate triggers all have defined structures. This makes the system debuggable and composable.

3. **Add structured observability** — a structured log of handoff events (who slung what to whom, GUPP activation, gate hits, escalation triggers) would bring Gas Town closer to what OpenAI Agents SDK and LangGraph Platform offer. Doesn't need to be OpenTelemetry — even a queryable JSONL event log per rig would be valuable.

4. **Consider MCP for the tool layer** — Claude Agent SDK's MCP-based tools are standards-aligned and would let Gas City define role-specific tool sets that work across LLM backends. This addresses the Claude Code coupling gap.

5. **Add conditional branching to formulas** — borrow LangGraph's conditional edges pattern for the formula engine (w-gc-003). Molecules should support "if test passes, merge; if test fails, notify witness" without requiring a new molecule.

6. **Don't converge on conversation-as-control-plane** — Gas Town's process-model approach (persistent named roles, parallel OS processes, Git-worktree isolation, hook-based communication, GUPP) is genuinely differentiated. Every other framework routes via LLM conversation or in-process function calls. Gas Town routes via Git state and file-based hooks. This is a feature, not a limitation — it's what enables 20-30 parallel agents and crash recovery. The goal should be to formalize these patterns, not to replace them with what everyone else is doing.

---

## Conclusion

This survey reveals a fundamental architectural split in the multi-agent space. Every external framework surveyed — AutoGen, CrewAI, LangGraph, OpenAI Agents SDK, Claude Agent SDK, Google ADK, Microsoft Agent Framework — uses **conversation as the control plane**. Agents coordinate by sending messages to each other through LLM inference. Whether it's CrewAI's task chaining, LangGraph's state-passing graph, or AutoGen's GroupChat, the LLM is always in the loop for routing and coordination decisions.

Gas Town is the only framework that uses a **process model**. Agents coordinate via external state in Dolt, with hooks, beads, and GUPP providing the scheduling and persistence primitives. The LLM does the *work* (writing code, reviewing, merging), but it does not do the *routing*. Routing is deterministic: sling puts work on a hook, GUPP activates, the agent executes. This is why Gas Town scales to 20-30 parallel agents while conversation-based frameworks struggle beyond 3-5 — every additional agent in a conversation multiplies token cost and routing complexity, while every additional polecat in Gas Town is just another independent process with its own worktree.

**The strategic implication for Gas City is clear: don't converge.** Gas City's declarative role format should formalize Gas Town's process-model patterns — operational roles with hooks, gates, escalation tiers, and GUPP semantics — not adopt the persona/graph/conversation paradigms from other frameworks. What Gas City should borrow is the *ergonomics* (CrewAI's intuitive role definition, LangGraph's typed state, Microsoft's plugin model), not the *architecture*.

Specifically:
- **From CrewAI**: the role/goal/backstory schema pattern — simple, intuitive, proven — as the user-facing layer of a Gas City role definition. But extend it with Gas Town operational fields (layer, hooks, gates, escalation_tier).
- **From LangGraph**: conditional branching for formulas, and the idea of making time-travel debugging a first-class feature over the existing Git/Dolt history.
- **From Microsoft Agent Framework**: the plugin model for role-scoped tool assignment, enabling least-privilege tool access per role.
- **From the field generally**: structured observability. Gas Town's organic monitoring (deacon patrols, witness nudges) works, but a queryable event log would make the system debuggable at scale.

The fact that Gas Town's architecture is unique is both its biggest risk and its biggest advantage. The risk is ecosystem isolation — every other framework's tooling, tutorials, and community knowledge assumes conversation-as-control-plane. The advantage is that Gas Town solves a problem no one else is solving: reliable coordination of many parallel, crash-prone, context-limited AI sessions working on real code at production scale.

---

## Sources

### Frameworks
- [AutoGen GitHub](https://github.com/microsoft/autogen)
- [AutoGen v0.4 announcement](https://www.microsoft.com/en-us/research/blog/autogen-v0-4-reimagining-the-foundation-of-agentic-ai-for-scale-extensibility-and-robustness/)
- [Microsoft Agent Framework overview](https://learn.microsoft.com/en-us/agent-framework/overview/)
- [Microsoft Agent Framework RC announcement](https://devblogs.microsoft.com/semantic-kernel/migrate-your-semantic-kernel-and-autogen-projects-to-microsoft-agent-framework-release-candidate/)
- [Semantic Kernel + AutoGen merger](https://visualstudiomagazine.com/articles/2025/10/01/semantic-kernel-autogen--open-source-microsoft-agent-framework.aspx)
- [CrewAI docs / changelog](https://docs.crewai.com/en/changelog)
- [CrewAI PyPI](https://pypi.org/project/crewai/)
- [LangGraph v1.0 announcement](https://blog.langchain.com/langchain-langgraph-1dot0/)
- [LangGraph GitHub](https://github.com/langchain-ai/langgraph)
- [OpenAI Swarm GitHub](https://github.com/openai/swarm)
- [OpenAI Agents SDK overview](https://lexogrine.com/blog/openai-swarm-multi-agent-framework-2026)
- [Claude Agent SDK docs](https://platform.claude.com/docs/en/agent-sdk/overview)
- [Claude Agent SDK GitHub](https://github.com/anthropics/claude-agent-sdk-python)
- [Google ADK docs](https://google.github.io/adk-docs/)
- [Google ADK GitHub](https://github.com/google/adk-python)
- [Google ADK integrations ecosystem](https://developers.googleblog.com/supercharge-your-ai-agents-adk-integrations-ecosystem/)

### Gas Town / Gas City
- [Gas Town GitHub](https://github.com/steveyegge/gastown) — source for role templates at `internal/templates/roles/*.md.tmpl`
- [Beads GitHub](https://github.com/steveyegge/beads) — the `bd` binary, Dolt-backed work tracking
- [Gas Town docs](https://docs.gastownhall.ai/)
- [Gas Town glossary](https://github.com/steveyegge/gastown/blob/main/docs/glossary.md)
- [Welcome to Gas Town (Yegge)](https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04)
- [Welcome to the Wasteland (Yegge, March 2026)](https://steve-yegge.medium.com/welcome-to-the-wasteland-a-thousand-gas-towns-a5eb9bc8dc1f)
- [Gas Town Emergency User Manual (Yegge)](https://steve-yegge.medium.com/gas-town-emergency-user-manual-cf0e4556d74b)
- [A Day in Gas Town (DoltHub)](https://www.dolthub.com/blog/2026-01-15-a-day-in-gas-town/)
- [Gas Town's Agent Patterns (Maggie Appleton)](https://maggieappleton.com/gastown)
- [GasTown and the Two Kinds of Multi-Agent](https://paddo.dev/blog/gastown-two-kinds-of-multi-agent/)
- [Gas Town architecture deep dive (DeepWiki)](https://deepwiki.com/numman-ali/n-skills/4.1.1-gas-town:-architecture-and-core-concepts)
- [Gas Town reading notes (Torq)](https://reading.torqsoftware.com/notes/software/ai-ml/agentic-coding/2026-01-15-gas-town-multi-agent-orchestration-framework/)
- [SE Daily Interview with Yegge](https://softwareengineeringdaily.com/2026/02/12/gas-town-beads-and-the-rise-of-agentic-development-with-steve-yegge/)
- [Wasteland CLI PR #1552](https://github.com/steveyegge/gastown/pull/1552)

### Comparative
- [AutoGen vs LangGraph vs CrewAI 2026](https://dev.to/synsun/autogen-vs-langgraph-vs-crewai-which-agent-framework-actually-holds-up-in-2026-3fl8)
- [The Great AI Agent Showdown of 2026](https://dev.to/topuzas/the-great-ai-agent-showdown-of-2026-openai-autogen-crewai-or-langgraph-1ea8)
- [LangGraph 2026 updates](https://www.agentframeworkhub.com/blog/langgraph-news-updates-2026)
