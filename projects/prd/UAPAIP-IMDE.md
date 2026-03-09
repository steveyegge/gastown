***

## 1. Product vision \& goals

**Vision**
Provide a secure, production-like AI sandbox where data scientists and RCM domain experts collaboratively build, evaluate, and continuously improve ML/LLM models for billing, coding, and denials, with one‑click promotion to an internal “RCM Model Marketplace”.[^2][^3]

**Primary goals**

- Reduce model development cycle from months to days via self‑service compute, pre‑built templates, and CI/CD.[^4][^3]
- Improve production success rate by enforcing prod‑like environments and standardized deployment paths.[^3]
- Enable RCM SMEs to review, test, and give feedback without needing deep ML tooling skills.[^1][^2]
- Support continuous learning from new claims/denials and payer changes.[^5][^3]

***

## 2. Target users \& personas

- Data Scientist / ML Engineer (primary builder): Needs full control over code, experiments, pipelines, and evaluation.
- RCM Domain Expert (SME): Needs low‑friction UI to test models on curated cases, view explanations/metrics, and give structured feedback.
- ML Platform Engineer: Owns workspace governance, networking, and CI/CD.
- Compliance \& Security Officer: Needs auditability, HIPAA controls, and clear PHI boundaries.[^6][^7][^5]

***

## 3. Scope and non‑goals (v1)

**In scope (v1)**

- Single RCM IMDE instance per tenant on Azure ML workspace + Azure AI Foundry hub/project.[^2][^3]
- Support for:
    - Tabular ML models (denials prediction, LOS, propensity) in Azure ML.
    - LLM/agentic workflows (code suggester, coding assistant, denial reason summarizer) via AI Foundry projects.[^1][^2]
- Basic internal “RCM Model Marketplace” UI (within Azure AI Foundry hub or custom app) listing approved models, metadata, and consumption endpoints.[^3][^2][^1]

**Out of scope (v1)**

- Cross‑cloud portability.
- Advanced multi‑tenant SaaS billing.
- Native EHR integration adapters (represented only as APIs/kafka connectors).

***

## 4. High‑level architecture

- Azure AI Foundry hub as the top‑level “RCM IMDE” shell (governance, projects, models, evaluations, agents).[^8][^2][^1]
- Azure ML workspace linked to the hub: training, pipelines, compute, registries, traditional ML, custom LLM fine‑tuning.[^2][^3]
- Azure DevOps / GitHub: Git repos, CI/CD pipelines to orchestrate training and deployment (ML + LLM flows).[^9][^10][^4]
- Secure data plane: HIPAA‑eligible services (Data Lake, SQL, Synapse/Fabric, Key Vault) in a locked‑down vNet.[^7][^5][^6]
- “Model Marketplace” implemented as:
    - Azure AI Foundry model catalog view filtered to RCM workspace/registry, and/or
    - Lightweight web front‑end over Azure ML registry + Foundry deployments.[^3][^1][^2]

***

## 5. Functional requirements

### 5.1 Secure sandbox with unified data

1. All IMDE projects run inside a dedicated Azure ML workspace and AI Foundry hub configured for PHI (HIPAA‑eligible services, BAA in place).[^5][^6][^7]
2. vNet‑integrated workspace and hub; no public inbound endpoints, private endpoints to data stores, Key Vault, and AI services.[^6][^3]
3. Standardized data access layer:
    - Pre‑defined “RCM datasets” (claims, encounters, denials, codes) registered in Azure ML and/or Fabric, with versioning and lineage.[^11][^3]
    - Access via managed identities and RBAC; no direct key access for users.[^6][^3]
4. Workspace policies:
    - Only approved compute SKUs, environments, and outbound domains.
    - Logging of all data access (Azure Monitor, Defender for Cloud).[^5][^6]

### 5.2 Pre‑loaded tools for ML, LLM, multimodal

1. Pre‑configured Azure ML project templates:
    - Tabular RCM ML template (training, evaluation, registration, deployment pipeline).
    - NLP/LLM classification template for denial reasons, and summarization template.[^12][^3]
2. AI Foundry project templates for:
    - Retrieval‑augmented generation over RCM knowledge bases (policy docs, payer bulletins).
    - Agent flows (e.g., claim review assistant) with built‑in evaluation and tracing.[^13][^8][^1]
3. Curated model catalog:
    - Allowed LLMs (Azure OpenAI, other Foundry models) tagged as “PHI allowed” or “non‑PHI only” according to compliance guidance.[^8][^5][^2]
    - Default baselines for BERT‑like models, tree‑based models, and large language models.
4. Standardized Docker environments for Python (ML) and LLM apps, stored as Azure ML environments and referenced by templates.[^12][^3]

### 5.3 Collaboration, version control, audit

1. All code stored in Git (Azure DevOps or GitHub) with branch protection and pull‑request workflows.[^10][^4][^9]
2. Azure ML experiments and runs automatically tagged with:
    - Git commit hash, dataset versions, environment image, and user identity.[^14][^3]
3. Notebooks:
    - Shared via Azure ML Studio and/or AI Foundry; read/write permissions via RBAC.
    - Versioning via Git; stored in repo, not only in workspace.
4. Auditability:
    - All model registration, endpoint deployment, and evaluation events logged to Log Analytics with user identity and timestamp.[^14][^3]
    - Tamper‑evident logs for compliance review.
5. RCM SME experience:
    - Web‑based UI (can be AI Foundry project/portal) to:
        - Select a candidate model, pick a test set (e.g., last 200 denials), and run evaluation.
        - View metrics, example‑level explanations, and leave structured feedback/comments.[^8][^1][^2]

### 5.4 One‑command push to “Model Marketplace”

1. Model registration:
    - Every training pipeline ends with Azure ML model registration (name, version, metadata like use case, owner, business unit).[^14][^3]
    - LLM flows register either as Foundry “model deployments” or as Azure ML endpoints linked to a registry entry.[^1][^2][^8]
2. “Publish to Marketplace” action:
    - CLI/SDK command and pipeline step that:
        - Validates required metadata (owner, SME approver, evaluation report, risk classification).
        - Promotes the model from dev registry to a central cross‑workspace registry and/or Foundry catalog “RCM” collection.[^15][^2][^1]
3. Marketplace UI:
    - Lists approved models with: description, performance summary, API endpoint, environments, last updated date, and contact.[^2][^3]
    - Filter by use case (e.g., “DRG prediction”, “denial risk scoring”).

### 5.5 Continuous improvement loop

1. Data feedback ingestion:
    - Periodic jobs (e.g., daily) that ingest recent claims/denials and label them as “production inference with ground truth available” into feature stores or ML datasets.[^3]
2. Monitoring \& triggers:
    - Deployed endpoints monitored for performance, drift, and data quality using Azure ML MLOps monitoring features.[^3]
    - Policy: if performance drops below thresholds or drift surpasses limits, create a retrain work item and/or trigger retraining pipeline automatically.[^3]
3. Retraining pipelines:
    - Parameterized training pipelines that can run on:
        - Sliding window (last N months) or incremental data.
    - On completion, candidates auto‑evaluated against baselines; only superior models get promoted to staging.[^3]
4. Safe rollout:
    - Blue‑green / canary deployments via managed online endpoints (traffic splitting), with auto‑rollback on degradation.[^16][^17][^3]

***

## 6. Non‑functional requirements

- Security \& compliance
    - HIPAA‑aligned configuration, PHI restricted to HIPAA‑eligible services, encryption in transit and at rest, RBAC, and least‑privilege access.[^7][^5][^6]
    - BAA confirmed as part of onboarding checklist.[^7][^6]
- Performance
    - Training jobs should auto‑scale on compute clusters; SLOs: typical training job under X hours for reference dataset size.
    - Inference: p95 latency target per use case (e.g., < 500ms for tabular, < 2s for LLM summarization).
- Reliability
    - At least two production regions or region + zone redundancy for critical endpoints.
    - Rollback procedures and deployment health checks defined in pipelines.[^10][^3]
- Usability
    - RCM SME flow must be fully in browser; no requirement to use CLI/SDK.
    - Clear separation between “builder” (DS/ML engineer) views and “consumer” (SME/RCM) views.

***

## 7. Integrations

- Azure DevOps or GitHub Actions for CI/CD of ML pipelines and infrastructure (Bicep/Terraform parameterized by environment).[^4][^9][^10]
- Azure Monitor + Log Analytics + Defender for Cloud for logs, metrics, and security alerts.[^6][^3]
- External RCM systems via:
    - Event streams (e.g., Kafka/Event Hubs) for near‑real‑time inference.
    - REST APIs for batch or synchronous scoring.

***

## 8. User journeys (v1)

### 8.1 Data scientist – new denial model

1. Clone RCM ML template repo; configure dataset references.
2. Commit changes → CI pipeline runs unit tests, linting.
3. Trigger training pipeline in Azure ML; monitor in Studio.
4. Successful run registers a model and evaluation report.
5. Submit PR and SME review; after approvals, run “Publish to Marketplace” pipeline to promote model and create staging endpoint.

### 8.2 RCM SME – validating candidate model

1. Open RCM IMDE portal (Foundry project/RCM app).
2. Select candidate model; choose a curated test set (e.g., payer X denials from last quarter).
3. Run evaluation; review metrics and case‑level outputs; leave feedback.
4. Approve or reject for production promotion; approval triggers CD pipeline to adjust traffic split to new version.

### 8.3 Continuous learning

1. Monitoring detects drift or performance drop.
2. Alert creates a ticket and/or triggers retraining pipeline.
3. New candidate model evaluated automatically against current prod; if better, DS/SME review and promotion via same marketplace workflow.

***

## 9. Milestones (high‑level)

- M1: Foundation (workspaces, hub, security baseline, basic templates) – 6–8 weeks.[^2][^6][^3]
- M2: Marketplace \& SME portal, “publish” flow – 4–6 weeks.
- M3: Monitoring, drift detection, automated retraining – 4–6 weeks.[^3]
- M4: Hardening for RCM (payer‑specific datasets, standard use‑case templates) – 4–8 weeks.

***

If you share your preferred Azure DevOps/GitHub stack and whether you want the “RCM IMDE” SME portal to live inside AI Foundry or as a separate web app, I can refine this into a more implementation‑ready spec (epics, user stories, and initial backlog).
<span style="display:none">[^18][^19][^20][^21]</span>

Reference:

[^1]: https://azure.microsoft.com/en-us/blog/azure-ai-foundry-your-ai-app-and-agent-factory/

[^2]: https://www.softwebsolutions.com/resources/what-is-azure-ai-foundry/

[^3]: https://azure.microsoft.com/en-us/products/machine-learning/mlops/?cdn=disable

[^4]: https://www.test-king.com/blog/mlops-with-microsoft-azure-what-you-need-to-know-in-2025/

[^5]: https://www.simbo.ai/blog/key-considerations-for-using-azure-ai-services-while-ensuring-compliance-with-hipaa-regulations-2933697/

[^6]: https://www.tcsa.in/resources/azure-hipaa-compliance-guide

[^7]: https://billingbenefit.com/is-microsoft-ai-hipaa-compliant-what-you-need-to-know-in-2025/

[^8]: https://www.youtube.com/watch?v=Sq8Cq7RZM2o

[^9]: https://www.youtube.com/watch?v=0yrkJJv--Tk

[^10]: https://www.youtube.com/watch?v=fcNANRfKpNw

[^11]: https://learn.microsoft.com/en-us/azure/architecture/ai-ml/guide/data-science-and-machine-learning

[^12]: https://learn.microsoft.com/en-us/azure/machine-learning/overview-what-is-azure-machine-learning?view=azureml-api-2

[^13]: https://azure.microsoft.com/en-us/blog/actioning-agentic-ai-5-ways-to-build-with-news-from-microsoft-ignite-2025/

[^14]: https://learn.microsoft.com/en-us/azure/machine-learning/concept-model-management-and-deployment?view=azureml-api-2

[^15]: https://learn.microsoft.com/en-us/azure/machine-learning/foundry-models-overview?view=azureml-api-2

[^16]: https://learn.microsoft.com/en-us/azure/machine-learning/how-to-deploy-online-endpoints?view=azureml-api-2

[^17]: https://oneuptime.com/blog/post/2026-02-16-how-to-configure-managed-online-endpoints-with-blue-green-deployment-in-azure-ml/view

[^18]: https://learn.microsoft.com/en-us/azure/ai-foundry/whats-new-ai-foundry

[^19]: https://devblogs.microsoft.com/foundry/whats-new-in-microsoft-foundry-dec-2025-jan-2026/

[^20]: https://www.oreateai.com/blog/azure-ai-foundry-whats-brewing-for-2025-and-beyond/c870e6e8c9e8c36ad4536bfcf5c8399a

[^21]: https://www.youtube.com/watch?v=fDwVolVG1sU

