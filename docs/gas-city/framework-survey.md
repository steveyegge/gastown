# Agent Orchestration Framework Survey

*Prepared for the Gas City project — a higher-level orchestration layer on top of Gas Town.*

## Executive Summary

The agent orchestration landscape in 2025-2026 has split into two distinct camps: **multi-agent frameworks** that coordinate LLM-powered agents through conversational or graph-based patterns, and **workflow orchestrators** that provide durable execution guarantees for long-running processes. Multi-agent frameworks (LangGraph, CrewAI, AutoGen, OpenAI Agents SDK, Claude Agent SDK) focus on defining how agents communicate, delegate, and share context, but most treat persistence and fault tolerance as afterthoughts. Workflow orchestrators (Temporal, Prefect) solve durability and observability at scale but lack native concepts for agent identity, role-based collaboration, or inter-agent communication protocols.

Gas Town occupies a unique position that neither camp addresses well. It orchestrates heterogeneous, independently-developed agent CLIs (Claude, Gemini, Codex, Cursor, etc.) through a loose-coupling model based on tmux sessions and environment variables. It provides identity and role management, structured work assignment (beads, convoys), a merge queue, and inter-agent communication — all without requiring agents to import a shared library or conform to a single SDK. No existing framework supports this "orchestrate anything that runs in a terminal" approach.

Gas City, as a higher-level layer on top of Gas Town, has the opportunity to bridge these two worlds: providing the workflow durability and observability of Temporal-class systems, combined with Gas Town's agent-agnostic orchestration and role taxonomy — while remaining SDK-independent and supporting agents at a process level rather than a library level.

## Comparison Table

| Framework | Approach | Persistence | Scale | Language | Key Strength | Key Weakness |
|-----------|----------|-------------|-------|----------|-------------|--------------|
| **LangGraph** | State machine / directed graph | Checkpointing (built-in) | Dozens of agents | Python, JS | Fine-grained control flow with human-in-the-loop | Tightly coupled to LangChain ecosystem |
| **CrewAI** | Role-based crews | Memory (short/long-term) | Small teams (2-10) | Python | Intuitive role metaphor, rapid prototyping | Limited to sequential/hierarchical flows |
| **AutoGen / MS Agent Framework** | Conversational agents | State management (v1.0) | Distributed networks | Python, .NET | Cross-language, enterprise-grade (post-merger) | Major API churn (AutoGen v0.2 to v0.4 to Agent Framework) |
| **Agency Swarm** | Organizational hierarchy | Thread-based (OpenAI) | Small agencies (3-8) | Python | Real-world org structure metaphor | Tightly coupled to OpenAI APIs |
| **MetaGPT** | Software company simulation | File-based artifacts | Fixed roles (5-7) | Python | End-to-end software generation from requirements | Rigid SOP structure, hard to customize |
| **Claude Agent SDK** | Agentic loop with subagents | Session-based, MCP tools | Subagent trees | Python, TypeScript | Battle-tested runtime (powers Claude Code), MCP integration | Anthropic-specific, still maturing multi-agent |
| **OpenAI Agents SDK** | Handoffs between agents | Tracing, sessions | Moderate | Python | Clean primitives, built-in guardrails and tracing | OpenAI-locked, limited persistence |
| **Temporal** | Durable execution workflows | Full durability (event sourcing) | Thousands of workflows | Go, Java, Python, TS, .NET | Production-grade fault tolerance and observability | No native agent concepts, steep learning curve |
| **Prefect** | Python-first workflow DAGs | Checkpoint-based | Hundreds of flows | Python | Dynamic control flow, easy Python authoring | No agent identity or communication model |

## Framework Deep Dives

### LangGraph (LangChain)

LangGraph models agent workflows as directed graphs where nodes represent agents, functions, or decision points, and edges define data flow. It provides explicit state schemas that are passed between nodes, enabling fine-grained control over what each agent sees and modifies. The framework supports cycles (not just DAGs), which is critical for agent loops that iterate until a condition is met.

LangGraph's checkpointing system persists graph state at each node boundary, enabling time-travel debugging and human-in-the-loop intervention at any point in the workflow. The LangGraph Platform (commercial) adds deployment, scaling, and a visual studio for designing flows. In 2025, LangGraph added streaming state updates and parallel execution within graph branches.

The primary limitation is ecosystem lock-in: LangGraph works best with LangChain's abstractions (chat models, tools, retrievers), making it cumbersome to integrate non-LangChain agents. State schemas must be defined upfront, which creates rigidity when agent behavior is dynamic. The framework also lacks native support for multi-process or multi-machine agent distribution — all agents run in the same Python process.

### CrewAI

CrewAI takes an organizational metaphor where each agent has a role, backstory, and goal. Agents are grouped into "crews" that execute tasks through configurable processes: sequential (pipeline), hierarchical (manager delegates), or consensus-based. The framework emphasizes simplicity — a crew of three agents with tools can be defined in under 50 lines of Python.

CrewAI Flows (introduced in 2025) provide a production architecture for composing crews into larger workflows with conditional routing and state management. The memory system supports short-term (conversation), long-term (cross-session), and entity memory (knowledge about specific topics). CrewAI Enterprise adds deployment, monitoring, and team management.

The main weakness is scalability: CrewAI is designed for small teams (typically 2-10 agents) working on discrete tasks. There is no native support for distributed execution, and all agents share a single process. The role-based abstraction can also be limiting for workflows that don't map to organizational hierarchies. Error handling and retry logic are basic compared to workflow orchestrators.

### AutoGen / Microsoft Agent Framework

AutoGen pioneered the conversational multi-agent pattern where agents communicate through asynchronous messages. Version 0.4 (2025) was a complete rewrite introducing event-driven messaging, pluggable components, and cross-language support (Python and .NET). In October 2025, Microsoft merged AutoGen with Semantic Kernel into the unified "Microsoft Agent Framework," targeting GA by Q1 2026.

The Agent Framework introduces a graph-based workflow API for explicit multi-agent execution paths, a robust state management system for long-running and human-in-the-loop scenarios, and production-grade enterprise support. The cross-organizational agent networks feature is notable — agents can operate across trust boundaries, which few other frameworks support.

The major concern is stability: AutoGen has undergone two major rewrites (v0.2 to v0.4, then the Agent Framework merger), breaking APIs each time. The community is fragmented across versions. The framework is also heavyweight for simple use cases and requires significant infrastructure (message brokers, state stores) for distributed operation.

### Agency Swarm

Agency Swarm maps the multi-agent problem onto real-world organizational structures. You define an "agency" with agents like CEO, Developer, and Virtual Assistant, connected by directional communication flows (the `>` operator defines who can initiate conversations with whom). Version 1.0 (2025) rebuilt on the OpenAI Agents SDK, giving it production-grade tracing and guardrails.

The framework provides type-safe tools via Pydantic, usage/cost tracking across multi-agent runs, and customizable agent instructions. The organizational metaphor makes it intuitive to design complex agent interactions — you think about reporting structures rather than graph edges.

Agency Swarm is tightly coupled to OpenAI's APIs and threading model, making it unusable with non-OpenAI models without significant modification. The scale is limited to small agencies (3-8 agents), and there is no durable persistence — state lives in OpenAI threads which have their own lifecycle and limitations. The project has a smaller community compared to LangGraph or CrewAI.

### MetaGPT

MetaGPT simulates an entire software company: Product Manager, Architect, Project Manager, and Engineer agents collaborate through Standard Operating Procedures (SOPs) encoded as prompt sequences. Given a one-line requirement, it produces PRDs, architecture designs, API specs, and working code. The 2025 launch of MGX (MetaGPT X) productized this as a hosted AI development team.

The SOP-based approach is MetaGPT's core innovation: agents validate each other's intermediate outputs before proceeding, significantly reducing error cascading. The AFlow paper (ICLR 2025 oral, top 1.8%) formalized automated agentic workflow generation, showing that structured agent collaboration can be optimized programmatically.

MetaGPT is narrowly focused on software development workflows with a fixed set of roles. Customizing the SOP or adding new roles requires deep framework knowledge. There is no general-purpose orchestration capability, no persistence across sessions, and limited support for human-in-the-loop intervention. It is a demonstration of structured multi-agent collaboration rather than a general framework.

### Claude Agent SDK (Anthropic)

The Claude Agent SDK exposes the same runtime powering Claude Code as a programmable library. It provides an agentic loop (tool use, orchestration, guardrails, tracing), subagent support for parallelization and context isolation, and native MCP (Model Context Protocol) integration for tool connectivity. The three-layer stack — MCP for tool communication, Agent Skills for portable capabilities, and the SDK as runtime — is a clean architectural separation.

Claude Code's multi-agent system demonstrates the SDK's capabilities: a team lead orchestrates specialist subagents (explore, code, test), each with isolated context windows. Opus 4.6 (February 2026) significantly improved agentic task performance. The SDK's battle-tested lineage (it powers one of the most widely-used coding agents) gives it credibility that newer frameworks lack.

The SDK is Anthropic-specific — it uses Claude models and Anthropic's API. Multi-agent orchestration beyond subagent trees (e.g., peer-to-peer agent communication, distributed agent networks) is still maturing. There is no built-in durable persistence or workflow recovery; sessions are ephemeral unless the application layer adds persistence.

### OpenAI Agents SDK

The Agents SDK (March 2025) is the production successor to OpenAI's experimental Swarm project. It provides three core primitives: Agents (LLMs with instructions and tools), Handoffs (delegation between agents), and Guardrails (input/output validation). The October 2025 AgentKit expansion added higher-level orchestration patterns.

The SDK includes built-in tracing for visualization and debugging of agentic flows, with the data usable for evaluation and model fine-tuning. The handoff primitive is elegant — an agent can route to another agent mid-conversation, transferring context and control. Sessions provide some state continuity.

Like Agency Swarm, the Agents SDK is locked to OpenAI's models and APIs. Persistence is limited to session-level state; there is no durable execution or workflow recovery. The framework is intentionally minimal ("very few abstractions"), which means production deployments need to build their own orchestration, state management, and error recovery layers on top.

### Temporal (Workflow Orchestrator)

Temporal provides durable execution where the full running state of a workflow survives process crashes, server failures, and network partitions through event sourcing. Activities (external calls) retry automatically with configurable policies. The 2025 OpenAI Agents SDK integration enables running AI agents with Temporal's durability guarantees.

For agent orchestration, Temporal offers: Schedules for proactive agent execution, Signals and Queries for real-time communication with running workflows, full UI visibility into every action in workflow history, and native support for long-running (days/weeks) processes. Multi-language support (Go, Java, Python, TypeScript, .NET) means agents in different languages can participate in the same workflow.

Temporal has no native concepts for agent identity, roles, or inter-agent communication. It sees agents as just another workflow activity. The learning curve is steep (deterministic workflow constraints, replay semantics), and self-hosting requires significant infrastructure (Temporal Server, persistence store, Elasticsearch). Temporal Cloud solves hosting but adds cost and vendor dependency.

### Prefect (Workflow Orchestrator)

Prefect turns Python functions into observable, schedulable workflows with `@task` and `@flow` decorators. It follows Python's native control flow — while loops, conditionals, exception handling — without requiring a compiled DAG. This makes it natural for agent state machines where the next step is determined at runtime.

Prefect provides autoscaling workers, enterprise auth, and rich observability including a dashboard for monitoring flow runs. It processes over 200 million tasks monthly in production. The event-driven architecture supports dynamic workflows that can react to external triggers.

Prefect lacks any agent-specific concepts — no roles, no inter-agent communication, no agent identity. It is a general-purpose workflow engine that happens to work well for orchestrating Python-based agent logic. Persistence is checkpoint-based rather than event-sourced, so recovery is less granular than Temporal. The framework is Python-only.

## Gap Analysis: What Problems Remain Unsolved

Surveying the landscape reveals several gaps that Gas Town already addresses and Gas City can build upon:

### 1. Agent-Agnostic Orchestration
Every multi-agent framework assumes agents are library objects instantiated within the same runtime. Gas Town's tmux-based approach is unique: it orchestrates any CLI agent (Claude, Gemini, Codex, Cursor) without requiring them to share an SDK, language, or process. No existing framework supports this.

### 2. Heterogeneous Agent Coordination
Existing frameworks assume homogeneous agents (same LLM provider, same tool interface). Gas Town's tiered integration model (Tier 0-3) allows agents at different levels of integration to coexist and collaborate. This is essential for real-world teams using multiple AI tools.

### 3. Persistent Identity with Ephemeral Sessions
Gas Town's polecat model (persistent identity, ephemeral sessions managed by a Witness) has no equivalent in any framework. Most frameworks either have fully ephemeral agents (new instance per task) or fully persistent ones (long-running process). The middle ground — durable identity with recyclable sessions — is novel.

### 4. Structured Work Assignment at Scale
Beads (issue tracking), convoys (batch tracking), and the sling mechanism provide work assignment infrastructure that no agent framework includes. Most frameworks rely on hardcoded task lists or simple queue-based distribution.

### 5. Merge Queue and Code Integration
The Refinery (automated testing and merging) is unique to Gas Town. No agent framework provides native support for the code review and merge workflow that is central to coding agent orchestration.

### 6. Role Taxonomy Beyond "Agent"
Gas Town's role taxonomy (Mayor, Deacon, Witness, Refinery, Polecat, Crew, Dog) provides specialized lifecycle management for different concerns. Existing frameworks offer at most "manager" and "worker" roles. The infrastructure roles (Witness for lifecycle management, Deacon for supervision) have no parallels.

### 7. Durability Without Determinism Constraints
Temporal provides durability but requires deterministic workflows and a specific programming model. Gas Town achieves coordination durability through its own mechanisms (mail, nudges, environment variables) without constraining how agents execute internally.

## Recommendations for Gas City Design

Based on this survey, the following recommendations emerge for Gas City's design:

### 1. Do Not Adopt an Existing Framework as Foundation
No existing framework matches Gas Town's architectural principles (loose coupling, agent-agnostic, tmux-based). Attempting to build Gas City on LangGraph, CrewAI, or AutoGen would require abandoning the core design that makes Gas Town unique. Gas City should remain a custom orchestration layer.

### 2. Adopt Temporal-Style Durability Concepts (Not Temporal Itself)
Temporal's event sourcing and durable execution model is the gold standard for workflow reliability. Gas City should incorporate similar concepts — workflow state persistence, activity retry policies, and time-travel debugging — but implemented in a way that works with Gas Town's process-level orchestration rather than requiring deterministic replay.

### 3. Invest in Observability Inspired by OpenTelemetry and Temporal
Gas Town already has OTel integration (per `otel-data-model.md`). Gas City should extend this with agent-level tracing (which agent did what, when, and why), convoy-level dashboards, and historical performance analytics. The tracing capabilities in the OpenAI Agents SDK and LangGraph Platform are good models for what developers expect.

### 4. Define a Gas City Provider Contract
The agent-provider-integration doc already hints at this. Gas City should formalize a provider contract that goes beyond Gas Town's Tier 0-3 model to include: capability advertisement (what can this agent do?), state reporting (what is this agent working on?), and health signaling (is this agent stuck?). This is the "Gas City provider contract" referenced in the existing docs.

### 5. Support Hierarchical and Peer-to-Peer Communication
Gas Town's current communication (nudges, mail) is primarily top-down. Gas City should add support for peer-to-peer agent communication patterns inspired by AutoGen's conversational model, while maintaining Gas Town's loose coupling through message queues or a pub/sub system rather than direct function calls.

### 6. Build Convoy-Level Workflow Orchestration
Convoys are Gas Town's most distinctive workflow primitive. Gas City should elevate convoys into a first-class workflow orchestration concept with: dependency tracking between convoy items, automatic parallelism detection (inspired by MetaGPT's assembly-line paradigm), progress monitoring and ETA estimation, and automatic escalation when items stall.

### 7. Keep the "Orchestrate Anything" Philosophy
The survey confirms that every other framework requires agents to be library objects. Gas Town's ability to orchestrate any terminal-based agent is a genuine competitive advantage. Gas City should extend this to support additional integration mechanisms (HTTP APIs, WebSockets, gRPC) beyond tmux, while maintaining the principle that agents never need to import Gas Town/City code.
