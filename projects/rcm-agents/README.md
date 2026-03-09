# RCM Agents — Revenue Cycle Management AI Workflows

Pre-built AI agent workflows for healthcare Revenue Cycle Management (RCM) built on
**Microsoft Agent Framework** (`agent-framework-azure-ai==1.0.0b260107`).

---

## Workflows

### 1. [Denial Management Agent](denial_management/README.md)

> **Path**: `denial_management/` | **Port**: 8087 (debug)

Automates insurance claim denial analysis and appeal generation.

- Maps denial reason codes (CARC/RARC) → human-readable categories
- Recommends corrective actions and appeal strategies per payer guidelines
- Drafts professional, payer-specific appeal letters

**Try it:**
```
Analyze denial CO-97 for CPT 27447, ICD-10 M17.11, BCBS payer.
Patient: Jane Doe, DOB 1980-05-14. Draft an appeal letter.
```

---

### 2. [Eligibility Verification Agent](eligibility_verification/README.md)

> **Path**: `eligibility_verification/` | **Port**: 8088 (debug)

Automates patient insurance eligibility checks with structured benefit summaries.

- Simulates 270/271 eligibility transactions (replaceable with real clearinghouse)
- Returns copay, deductible, OOP max, and coverage status
- Flags eligibility issues (inactive, terminated, wrong payer, pending enrollment)

**Try it:**
```
Verify eligibility for John Smith, DOB 1975-03-22,
Member ID UHC987654321, UnitedHealthcare. Service date 2025-06-15.
```

---

## Project Layout

```
rcm-agents/
├── .venv/                              ← shared virtual environment
├── .vscode/
│   ├── launch.json                     ← F5 debug configs (both agents)
│   └── tasks.json                      ← agentdev + AI Toolkit Inspector tasks
├── denial_management/
│   ├── main.py                         ← agent + tools + HTTP server
│   ├── requirements.txt
│   ├── .env                            ← FOUNDRY_PROJECT_ENDPOINT etc.
│   └── README.md
├── eligibility_verification/
│   ├── main.py                         ← agent + tools + HTTP server
│   ├── requirements.txt
│   ├── .env
│   └── README.md
└── README.md                           ← this file
```

---

## Setup

### 1. Prerequisites

- Python 3.10 or higher  
- Microsoft Foundry project with `gpt-5.1` deployed  
  *(or another model — see each agent's README)*  
- Azure CLI signed in: `az login`

### 2. Create & activate virtual environment

```bash
cd c:\gitrepos\gastown\projects\rcm-agents

# Create venv (one-time)
python -m venv .venv

# Activate
.venv\Scripts\activate          # Windows PowerShell
# source .venv/bin/activate     # macOS / Linux
```

### 3. Install dependencies

```bash
# Install both agents' dependencies into the shared venv
pip install -r denial_management/requirements.txt
pip install -r eligibility_verification/requirements.txt
```

### 4. Configure endpoints

Edit each `.env` file:

```env
# denial_management/.env
AZURE_OPENAI_ENDPOINT=https://<your-resource>.openai.azure.com/
AZURE_OPENAI_DEPLOYMENT_NAME=gpt-4.1
```

```env
# eligibility_verification/.env
AZURE_OPENAI_ENDPOINT=https://<your-resource>.openai.azure.com/
AZURE_OPENAI_DEPLOYMENT_NAME=gpt-4.1
```

> **Endpoint sources**:
> - **Azure OpenAI**: Azure Portal → Azure OpenAI resource → Overview → Endpoint  
> - **Azure AI Foundry**: AI Foundry Portal → Your project → Overview → Azure OpenAI Endpoint
> - Sign in with Azure CLI (`az login`) for `DefaultAzureCredential` to work

---

## Running the Agents

```bash
# Activation assumed (.venv\Scripts\activate)

# Run Denial Management Agent
python denial_management/main.py

# Run Eligibility Verification Agent (separate terminal)
python eligibility_verification/main.py
```

---

## Debugging with AI Toolkit Agent Inspector

1. Open VS Code with `c:\gitrepos\gastown\projects\rcm-agents` as workspace root
2. Set Python interpreter to `.venv`
3. **F5** → select:
   - `"Debug Denial Management Agent (HTTP)"` → Inspector on port 8087
   - `"Debug Eligibility Verification Agent (HTTP)"` → Inspector on port 8088
4. Set breakpoints in `main.py` files and interact through the Inspector

---

## Model Recommendation

Both agents use **`gpt-5.1`** (OpenAI via Microsoft Foundry):

| Metric      | Value                         |
|-------------|-------------------------------|
| Quality     | 0.903 (strong multi-step reasoning) |
| Throughput  | 75 tokens/sec                 |
| Context     | 200K input / 100K output      |
| Cost        | $3.4375 / 1M tokens           |

This model is well-suited for RCM workflows requiring structured output, multi-step
reasoning over claim records, and consistent medical document generation.

**Alternatives**:
- `gpt-4.1` — Larger context (1M tokens), great for bulk claim analysis
- `claude-sonnet-4-5` — Strong reasoning, excellent for nuanced clinical justifications
- `gpt-5.1-chat` — Multimodal, if you need to process scanned EOB images

---

## Extending These Agents

| Goal | Approach |
|------|----------|
| Connect to real payer APIs | Replace mock tools with clearinghouse HTTP calls |
| Add more denial codes | Extend `_DENIAL_CODE_MAP` in `denial_management/main.py` |
| Prior authorization checks | Add a `check_prior_auth()` tool to eligibility agent |
| Multi-agent orchestration | Use `WorkflowBuilder` with multiple `WorkflowExecutor` nodes |
| Deploy to production | Use `msft-foundry-deploy` or `azd up` with a Bicep manifest |
| Evaluate quality | Add `azure-ai-evaluation` metrics for faithfulness and correctness |

---

## Security & Compliance

> ⚠️ These agents process Protected Health Information (PHI).
> Before using with real patient data:
> - Ensure your Azure Foundry project is in a HIPAA-eligible region
> - Enable Azure Private Endpoints for network isolation
> - Configure Azure Monitor audit logging
> - Review your BAA (Business Associate Agreement) with Microsoft

---

## Package Versions (Actual)

| Package | Version |
|---------|----------|
| `agent-framework-core` | `1.0.0rc2` |
| `agent-framework-azure-ai` | `1.0.0rc2` |
| `fastapi` | `>=0.110.0` |
| `uvicorn[standard]` | `>=0.27.0` |

> **Note on `1.0.0b260107`**: Originally requested, but this version requires
> `azure-ai-projects` model classes (`PromptAgentDefinitionText`,
> `ResponseTextFormatConfigurationJsonObject`) that are not yet publicly available
> on PyPI as of March 2026. `1.0.0rc2` is the closest stable working version.
>
> Both versions use `AzureOpenAIChatClient` (from `agent-framework-core`) which
> has no dependency on `azure-ai-projects` model-specific classes.
