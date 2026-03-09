---
title: "PRD: Azure AI Asset Marketplace + Visual Agent Orchestration"
product: "AI Asset Marketplace (Agents, MCP Servers/Tools, Models)"
version: "0.9"
status: "Draft"
owners:
  - Product: TBD
  - Engineering: TBD
  - Security/Compliance: TBD
date: "2026-02-27"
target_release: "MVP in ~16 weeks (3–4 sprints)"
---

# 1. Purpose and vision

## 1.1 Problem statement
Enterprises want to adopt agentic AI quickly, but they struggle to discover trustworthy AI assets (agents, MCP servers/tools, models), understand compatibility, and deploy them consistently across environments with governance and observability. The platform must reduce fragmentation by enabling catalog, procurement, deployment, and operations in one workflow, while supporting multi-agent orchestration and enterprise controls. (The onsite agenda emphasizes aligning “what exists today vs what is desired,” clarifying “what gets deployed to the customer,” and doing requirements gathering via journey mapping.) [file:1]

## 1.2 Vision
Create a secure, enterprise-grade marketplace for AI assets—Agents, MCP servers/tools, and Models—built on the **Azure ecosystem** with a **visual orchestration** experience to compose and run multi-agent workflows and deploy them to customer environments. The platform will leverage Microsoft’s agent platform direction (Azure AI Foundry + Microsoft Agent Framework + MCP support) to move from prototype to production with observability, compliance, and governance built in. [page:0][page:1]

## 1.3 Goals (business + product)
- Reduce time-to-value for building, buying, and deploying agentic solutions by providing a “discover → evaluate → orchestrate → deploy → monitor” loop. [page:0][page:1]
- Enable customer-aligned solution delivery (PaaS vs SaaS) and clarify packaging/deployment responsibilities, as called out in the prework agenda. [file:1]
- Provide visual multi-agent orchestration and workflow execution aligned with Foundry Agent Service and Agent Framework patterns. [page:0][page:1]

## 1.4 Non-goals (MVP)
- General-purpose app marketplace for non-AI workloads.
- Building a new LLM training platform (use Azure AI Foundry / Azure Machine Learning instead). [page:1]
- Supporting non-Azure deployment targets in MVP (may be later roadmap).

---

# 2. Users and personas

## 3.1 Primary personas
1. **Enterprise Product Leader** (e.g., Optum product leader): Wants repeatable packaging and externalization for customers; decides PaaS vs SaaS. [file:1]  
2. **Enterprise Architect**: Aligns with enterprise architecture plans and integration patterns (e.g., CDP/UAP connection points, medallion approach). [file:1]  
3. **AI/ML Engineer**: Publishes, composes, and tests agents/tools/models; needs versioning and evaluation. [page:1]  
4. **Security Architect / Governance**: Needs policy enforcement, auditability, PII controls, and safe tool access. [page:0]  
5. **Customer Ops / Platform Engineer**: Deploys into customer environments and requires monitoring, rollbacks, and SLAs; agenda explicitly asks “what gets deployed to the customer.” [file:1]

## 3.2 Secondary personas
- **ISV / Partner publisher**: Submits assets (agents/tools/models) for marketplace distribution and monetization.
- **Business stakeholder** (e.g., clinical/quality/risk leader; provider/claims processor): Consumes solutions via guided workflows; aligns with journey mapping approach in Day 2 agenda. [file:1]

---

# 4. Scope and product concept

## 4.1 Product overview
The product is a marketplace + orchestration runtime that supports:
- **Asset catalog**: agents, MCP servers/tools, models, prompts/skills, evaluators, workflow templates.
- **Visual orchestration**: build multi-agent workflows with tool connections, policies, and data grounding.
- **Deployment**: publish to a Foundry project / Foundry Agent Service environment and optionally “export” deployment packages to customer tenant subscriptions.
- **Operations**: monitoring, tracing, evaluations, cost controls, and governance.

This is designed to map to Azure AI Foundry capabilities for agent creation, deployment, and governance, and to Microsoft Agent Framework for orchestration patterns and MCP-based tool connections. [page:0][page:1]

## 4.2 Deployment models (must support both)
- **SaaS**: Marketplace + orchestration hosted by provider; customers consume in a managed environment.
- **PaaS**: Customers deploy selected assets and workflows into their own Azure subscription(s), consistent with the prework’s “PaaS vs SaaS solution” decision point. [file:1]

---

# 5. User journeys (MVP)

## 5.1 Journey A: Discover → Orchestrate → Deploy (AI Engineer)
1. User searches marketplace for a “Denial Intelligence” workflow template and supporting agents. [file:1]
2. User inspects asset cards (capabilities, required tools/MCP servers, model dependencies, security posture).
3. User opens Visual Orchestrator, drags template into canvas, swaps model deployments (Azure OpenAI) and connects to MCP servers/tools.
4. User runs evaluation suite and verifies telemetry/traces.
5. User deploys to Foundry Agent Service environment and promotes to production with version pinning. [page:1][page:0]

## 5.2 Journey B: Governance approval (Security / Governance)
1. Governance reviewer receives “publish request” for new asset version.
2. Reviewer checks policy compliance, tool permissions, data egress constraints, and audit requirements.
3. Reviewer approves for internal-only or external-customer distribution.
4. System records an immutable audit trail and enforces runtime policy controls. [page:0]

## 5.3 Journey C: Customer persona journey mapping (Business stakeholder)
Use PRD/journey mapping sessions to define persona-specific workflows (e.g., clinical/quality/risk leader; providers/claims processor; ecosystem partners), as per Day 2 agenda. [file:1]

---

# 6. Functional requirements

## 6.1 Marketplace catalog (MVP)
- FR-1: Support listing types: Agent, Tool/MCP Server, Model, Workflow Template, Evaluator/Test Suite, Connector (OpenAPI). [page:0][page:1]
- FR-2: Search, filter, and compare assets by category, tags, supported domains, compatibility, and compliance tier.
- FR-3: Asset detail page must include:
  - Version, changelog, publisher identity, licensing/terms
  - Required dependencies (models, tools, MCP servers)
  - Supported deployment mode (SaaS, PaaS export)
  - Telemetry/evaluation artifacts and basic risk notes
- FR-4: Ratings and usage metrics (internal metrics first; external later).
- FR-5: “Add to workspace/project” to pull asset into a Foundry Project (or equivalent workspace abstraction). [page:1]

## 6.2 Publisher workflow (MVP)
- FR-6: Publisher onboarding and verification (org identity, contact, publisher keys).
- FR-7: Submission pipeline:
  - Validate metadata schema
  - Security scanning (code/package/container where applicable)
  - Policy checks (data handling declarations)
  - Human approval workflow
- FR-8: Versioning and deprecation policy, including “pinned versions” for production usage.

## 6.3 Visual agent orchestration (core requirement)
- FR-9: Provide a **visual canvas** to orchestrate multi-agent workflows:
  - Nodes: agents, tools/MCP servers, model deployments, retrieval/knowledge, evaluators, guards, human-in-the-loop steps
  - Edges: message flow, control flow, and data flow
  - Variables/context mapping between nodes
- FR-10: Support multi-agent workflow execution that is stateful
