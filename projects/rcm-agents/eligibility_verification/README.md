# Eligibility Verification Agent

An AI-powered Revenue Cycle Management (RCM) agent that automates patient insurance
eligibility checks using **Microsoft Agent Framework**.

## What It Does

Given patient demographics and insurance information, the agent:

1. **Simulates an eligibility verification** (270/271 transaction) — generates a
   realistic mock benefit response with deductible, copay, and OOP data.
2. **Returns a structured benefit summary** — formats financial details for front-desk
   staff and clinical teams.
3. **Flags coverage issues** — identifies inactive coverage, wrong payer, terminated
   membership, pending enrollment, and out-of-network status, with action items for each.

> **Note**: This agent uses a mock payer simulation engine. To connect to real payers,
> replace `verify_eligibility()` with an actual 270/271 clearinghouse API call.

### Simulated Payers

| Key       | Full Name            |
|-----------|----------------------|
| BCBS      | Blue Cross Blue Shield |
| AETNA     | Aetna                |
| CIGNA     | Cigna                |
| UHC       | UnitedHealthcare     |
| HUMANA    | Humana               |
| MEDICAID  | Medicaid (State)     |
| MEDICARE  | Medicare             |

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
#    Edit eligibility_verification/.env with your real values

# 4. Run the agent HTTP server
python eligibility_verification/main.py
```

---

## Configure

Edit `eligibility_verification/.env`:

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
3. Press **F5** → choose **"Debug Eligibility Verification Agent (HTTP)"**
4. AI Toolkit Agent Inspector opens on port **8088**
5. Set breakpoints in `main.py` and send test requests through the inspector

---

## Example Prompts

```
Check eligibility for patient John Smith, DOB 1975-03-22,
Member ID UHC987654321, UnitedHealthcare PPO plan.
Service date is 2025-06-15, provider NPI 1234567890.
```

```
Verify insurance for Maria Garcia, DOB 1990-11-08, BCBS member ID BC123456789.
What's her specialist copay and remaining deductible?
```

```
Run eligibility on: Patient: Bob Lee, DOB 1965-07-04, Member: MED-00112233,
Payer: Medicare. Service: 2025-07-01. Flag any issues.
```

---

## Architecture

```
main.py
  ├── Tools:
  │     verify_eligibility()       — 270/271 mock verification engine
  │     get_benefit_summary()       — plain-language benefit formatter
  │     check_eligibility_issues()  — issue detection + action items
  │
  ├── EligibilityVerificationExecutor (WorkflowExecutor)
  │     └── Wraps ChatAgent, handles list[ChatMessage] input
  │
  ├── WorkflowBuilder → .as_agent()
  │
  └── from_agent_framework(agent).run_async()  ← HTTP server
```

**Model**: `gpt-5.1` via Microsoft Foundry
- Quality index: 0.903 | Throughput: 75 tok/s | Context: 200K tokens

---

## Replacing Mock Verification with Real Payer APIs

The `verify_eligibility()` tool can be swapped for a real 270/271 clearinghouse:

```python
import httpx

def verify_eligibility(patient_name, patient_dob, member_id, payer_name, ...):
    # Example: call your clearinghouse (Availity, Change Healthcare, etc.)
    response = httpx.post(
        "https://your-clearinghouse.com/api/270",
        json={
            "memberId": member_id,
            "dateOfBirth": patient_dob,
            "payerId": resolve_payer_id(payer_name),
        },
        headers={"Authorization": f"Bearer {os.environ['CLEARINGHOUSE_TOKEN']}"},
    )
    return response.json()
```

Add `CLEARINGHOUSE_TOKEN` to your `.env` file and update `requirements.txt` with `httpx`.

---

## Extending the Agent

- **Add payers**: extend `_MOCK_PAYER_PLANS` with new payer profiles
- **Real-time verification**: replace mock with clearinghouse API (see above)
- **Prior auth check**: add a `check_prior_authorization()` tool
- **Automated alerts**: emit alerts when coverage issues are detected

---

## Important Notes

> ⚠️ **Pre-release packages**: Pin all `agent-framework-*` and
> `azure-ai-agentserver-*` versions before upgrading.

> ⚠️ **HIPAA**: Ensure your Foundry project and Azure environment are configured
> for HIPAA compliance when using real patient data.
