# Denial Management Agent

An AI-powered Revenue Cycle Management (RCM) agent that automates insurance claim
denial analysis and appeal generation using **Microsoft Agent Framework**.

## What It Does

Given a denial reason code and claim details, the agent:

1. **Maps the denial code** — translates CARC/RARC codes (CO-4, CO-97, PR-96, etc.)
   into human-readable categories with payer-specific notes.
2. **Recommends an appeal strategy** — provides step-by-step corrective actions
   tailored to the denial code, payer, CPT, and ICD-10.
3. **Drafts a professional appeal letter** — generates a ready-to-send letter with all
   required claim identifiers, clinical justification, and document checklist.

### Supported Denial Codes (out of the box)

| Code   | Category                                  |
|--------|-------------------------------------------|
| CO-4   | Service inconsistent with modifier        |
| CO-11  | Diagnosis code not covered                |
| CO-18  | Duplicate claim                           |
| CO-22  | Coordination of benefits                  |
| CO-29  | Timely filing limit exceeded              |
| CO-50  | Non-covered service                       |
| CO-97  | Payment included in another service       |
| PR-96  | Patient responsibility — non-covered      |
| CO-167 | Diagnosis not covered                     |
| OA-23  | COB/withholding adjustment                |

---

## Prerequisites

- Python 3.10 or higher
- A Microsoft Foundry project with a deployed model (see [Configure](#configure))
- Azure CLI logged in or another supported Azure credential

---

## Quick Start

```bash
# 1. Navigate to the rcm-agents root
cd c:\gitrepos\gastown\projects\rcm-agents

# 2. Activate the shared virtual environment
.venv\Scripts\activate          # Windows
# source .venv/bin/activate     # macOS/Linux

# 3. Configure your Foundry endpoint
#    Edit denial_management/.env with your real values (see Configure section)

# 4. Run the agent HTTP server
python denial_management/main.py
```

The server starts on the default port (8080). Use the AI Toolkit Agent Inspector
or any HTTP client to send messages.

---

## Configure

Edit `denial_management/.env`:

```env
# Azure OpenAI endpoint (Azure OpenAI resource OR Foundry-linked AOAI endpoint)
AZURE_OPENAI_ENDPOINT=https://<your-resource>.openai.azure.com/

# Model deployment name (gpt-4.1 recommended, also works: gpt-5.1, gpt-4o)
AZURE_OPENAI_DEPLOYMENT_NAME=gpt-4.1
```

> **Tip**: Find your endpoint in Azure Portal → Azure OpenAI resource → Overview → Endpoint.
> If using AI Foundry, the AOAI-compatible endpoint is on the Foundry project's Overview page.

---

## Debug with AI Toolkit Agent Inspector (F5)

1. Open VS Code in `c:\gitrepos\gastown\projects\rcm-agents`
2. Select Python interpreter from `.venv`
3. Press **F5** → choose **"Debug Denial Management Agent (HTTP)"**
4. The AI Toolkit Agent Inspector opens automatically on port **8087**
5. Set breakpoints anywhere in `main.py` and send messages through the inspector

---

## Example Prompts

```
Analyze this denial: CO-97 for claim 2024-CLM-00412.
Patient: Jane Doe, DOB 1980-05-14, Member ID ABC123.
Payer: BCBS. Service Date: 2024-11-01. CPT: 27447, ICD-10: M17.11.
Draft a full appeal letter.
```

```
CO-29 denial from Aetna. Claim submitted 95 days after service date on 2024-09-15.
CPT 99213, ICD-10 J06.9. Provider: Northside Medical Group. What should I do?
```

---

## Architecture

```
main.py
  ├── Tools:
  │     map_denial_code()          — CARC/RARC code lookup
  │     recommend_appeal_strategy() — payer-specific corrective actions
  │     draft_appeal_letter()       — professional appeal letter generator
  │
  ├── DenialManagementExecutor (WorkflowExecutor)
  │     └── Wraps ChatAgent, handles list[ChatMessage] input
  │
  ├── WorkflowBuilder → .as_agent()
  │
  └── from_agent_framework(agent).run_async()  ← HTTP server
```

**Model**: `gpt-5.1` via Microsoft Foundry
- Quality index: 0.903 | Throughput: 75 tok/s | Context: 200K tokens
- Ideal for multi-step reasoning over claim records and payer policies

---

## Extending the Agent

- **Add denial codes**: extend `_DENIAL_CODE_MAP` in `main.py`
- **Payer-specific rules**: add payer name branches inside `recommend_appeal_strategy`
- **RAC/MAC integration**: add a new tool that calls your payer API / EMR
- **Multi-agent**: wrap in a `WorkflowBuilder` with a separate coding specialist agent

---

## Important Notes

> ⚠️ **Pre-release packages**: `agent-framework-*` and `azure-ai-agentserver-*` are
> in preview. Pin versions in `requirements.txt` before upgrading.

> ⚠️ **HIPAA**: This agent processes PHI. Ensure your Foundry project and Azure
> network are configured for HIPAA compliance before using with real patient data.
