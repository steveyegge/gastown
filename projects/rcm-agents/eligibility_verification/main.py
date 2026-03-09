"""
Eligibility Verification Agent — RCM Healthcare Workflow
=========================================================
Uses Microsoft Agent Framework (agent-framework-core==1.0.0rc2, AzureOpenAIChatClient)
Simulates insurance eligibility verification, returns structured benefit
summaries, and flags coverage issues.

HTTP server mode (default):
    python main.py

Debug with AI Toolkit Agent Inspector:
    python -m debugpy --listen 127.0.0.1:5680 -m agentdev run main.py --verbose --port 8088

Environment variables (see .env):
    AZURE_OPENAI_ENDPOINT          — your Azure OpenAI or Foundry-linked AOAI endpoint
    AZURE_OPENAI_DEPLOYMENT_NAME   — model deployment name (e.g. gpt-5.1 / gpt-4.1)

Note on Foundry: If you have an Azure AI Foundry project, it exposes an Azure OpenAI
compatible endpoint. Set AZURE_OPENAI_ENDPOINT to that resource's AOAI URL.
"""

from __future__ import annotations

import asyncio
import os
import random
from contextlib import asynccontextmanager
from typing import Annotated, Optional

from azure.identity import DefaultAzureCredential
from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel
import uvicorn

# Agent Framework — AzureOpenAIChatClient lives in agent-framework-core (no azure-ai-projects dep)
from agent_framework.azure import AzureOpenAIChatClient
from agent_framework import Message

# Load environment variables (override=True ensures deployed env vars take precedence)
load_dotenv(override=True)

# ---------------------------------------------------------------------------
# RCM Domain Tools — Eligibility Verification
# ---------------------------------------------------------------------------

# Simulated payer database for mock eligibility responses
_MOCK_PAYER_PLANS: dict[str, dict] = {
    "BCBS": {
        "full_name": "Blue Cross Blue Shield",
        "plan_types": ["PPO", "HMO", "EPO"],
        "avg_deductible": 1500,
        "avg_oop_max": 6000,
        "avg_copay_pcp": 30,
        "avg_copay_specialist": 60,
        "avg_coinsurance": 20,
    },
    "AETNA": {
        "full_name": "Aetna",
        "plan_types": ["PPO", "HMO", "HDHP"],
        "avg_deductible": 2000,
        "avg_oop_max": 7000,
        "avg_copay_pcp": 25,
        "avg_copay_specialist": 55,
        "avg_coinsurance": 20,
    },
    "CIGNA": {
        "full_name": "Cigna",
        "plan_types": ["PPO", "HMO"],
        "avg_deductible": 1800,
        "avg_oop_max": 6500,
        "avg_copay_pcp": 35,
        "avg_copay_specialist": 65,
        "avg_coinsurance": 20,
    },
    "UHC": {
        "full_name": "UnitedHealthcare",
        "plan_types": ["PPO", "HMO", "Choice Plus"],
        "avg_deductible": 1750,
        "avg_oop_max": 7500,
        "avg_copay_pcp": 30,
        "avg_copay_specialist": 60,
        "avg_coinsurance": 20,
    },
    "HUMANA": {
        "full_name": "Humana",
        "plan_types": ["PPO", "HMO", "Gold Plus"],
        "avg_deductible": 1200,
        "avg_oop_max": 5500,
        "avg_copay_pcp": 20,
        "avg_copay_specialist": 50,
        "avg_coinsurance": 15,
    },
    "MEDICAID": {
        "full_name": "Medicaid (State)",
        "plan_types": ["Managed Medicaid", "FFS"],
        "avg_deductible": 0,
        "avg_oop_max": 0,
        "avg_copay_pcp": 3,
        "avg_copay_specialist": 5,
        "avg_coinsurance": 0,
    },
    "MEDICARE": {
        "full_name": "Medicare",
        "plan_types": ["Part A", "Part B", "Advantage"],
        "avg_deductible": 1632,
        "avg_oop_max": 7550,
        "avg_copay_pcp": 20,
        "avg_copay_specialist": 20,
        "avg_coinsurance": 20,
    },
}

_COVERAGE_ISSUE_SCENARIOS = [
    None,  # No issues (most common)
    None,
    None,
    "inactive_coverage",
    "wrong_payer",
    "terminated_member",
    "pending_enrollment",
    "out_of_network_only",
]


def verify_eligibility(
    patient_name: Annotated[str, "Full name of the patient"],
    patient_dob: Annotated[str, "Patient date of birth (YYYY-MM-DD)"],
    member_id: Annotated[str, "Insurance member ID"],
    payer_name: Annotated[str, "Name of the insurance payer (e.g. BCBS, Aetna, UHC)"],
    service_date: Annotated[str, "Date of service or verification date (YYYY-MM-DD)"],
    provider_npi: Annotated[str, "Provider NPI number"],
) -> str:
    """
    Simulates a real-time eligibility verification (270/271 transaction) against
    the payer. Returns a structured JSON-like benefit summary with active
    coverage status, copay, deductible, and out-of-pocket max.
    """
    # Normalize payer key
    payer_key = payer_name.upper().replace(" ", "").replace("-", "")
    # Try to match partial payer name
    matched_key = next(
        (k for k in _MOCK_PAYER_PLANS if k in payer_key or payer_key in k),
        None,
    )
    payer_data = _MOCK_PAYER_PLANS.get(matched_key or "", {
        "full_name": payer_name,
        "plan_types": ["PPO"],
        "avg_deductible": 2000,
        "avg_oop_max": 8000,
        "avg_copay_pcp": 35,
        "avg_copay_specialist": 70,
        "avg_coinsurance": 20,
    })

    # Simulate a deterministic but realistic mock. Use member_id hash for consistency.
    seed = sum(ord(c) for c in member_id)
    rng = random.Random(seed)

    plan_type = rng.choice(payer_data["plan_types"])
    deductible = payer_data["avg_deductible"] + rng.randint(-200, 200)
    deductible_met = round(rng.uniform(0, deductible), 2)
    oop_max = payer_data["avg_oop_max"] + rng.randint(-500, 500)
    oop_met = round(rng.uniform(0, oop_max * 0.4), 2)
    copay_pcp = payer_data["avg_copay_pcp"]
    copay_specialist = payer_data["avg_copay_specialist"]
    coinsurance = payer_data["avg_coinsurance"]

    scenario = rng.choice(_COVERAGE_ISSUE_SCENARIOS)
    coverage_status = "Active" if scenario is None else "Issue Detected"

    result = {
        "verification_id": f"EV-{uuid4().hex[:8].upper()}",
        "transaction_type": "270/271 Eligibility Verification (Simulated)",
        "service_date": service_date,
        "provider_npi": provider_npi,
        "patient": {
            "name": patient_name,
            "dob": patient_dob,
            "member_id": member_id,
        },
        "payer": {
            "name": payer_data["full_name"],
            "payer_id": f"PAYER-{(matched_key or 'UNKNOWN')[:6]}",
        },
        "coverage_status": coverage_status,
        "plan_details": {
            "plan_type": plan_type,
            "effective_date": f"{service_date[:4]}-01-01",
            "termination_date": f"{service_date[:4]}-12-31",
            "group_number": f"GRP-{rng.randint(10000, 99999)}",
            "group_name": f"{payer_data['full_name']} Employer Group Plan",
        },
        "benefits": {
            "annual_deductible": deductible,
            "deductible_met_ytd": deductible_met,
            "deductible_remaining": round(max(0, deductible - deductible_met), 2),
            "out_of_pocket_max": oop_max,
            "out_of_pocket_met_ytd": oop_met,
            "out_of_pocket_remaining": round(max(0, oop_max - oop_met), 2),
            "copay_primary_care": copay_pcp,
            "copay_specialist": copay_specialist,
            "coinsurance_pct": coinsurance,
            "preventive_care": "Covered 100% (no cost share)",
            "mental_health": f"${copay_specialist} copay after deductible",
            "emergency_room": f"${rng.choice([150, 200, 250, 300])} copay (waived if admitted)",
            "urgent_care": f"${rng.choice([50, 75, 100])} copay",
        },
        "issue": scenario,
        "raw_271_summary": (
            f"ISA*00**00**ZZ*{payer_data['full_name'][:5].upper()}***271 Simulated Response "
            f"for Member {member_id}. Coverage: {coverage_status}."
        ),
    }

    import json
    return json.dumps(result, indent=2)


def get_benefit_summary(
    verification_json: Annotated[str, "JSON string returned by verify_eligibility"],
) -> str:
    """
    Parses the eligibility verification JSON and returns a clean, plain-language
    benefit summary suitable for front-desk staff or clinical teams.
    """
    import json
    try:
        data = json.loads(verification_json)
    except json.JSONDecodeError:
        return "Error: Could not parse eligibility verification data."

    patient = data.get("patient", {})
    payer = data.get("payer", {})
    benefits = data.get("benefits", {})
    plan = data.get("plan_details", {})
    status = data.get("coverage_status", "Unknown")
    issue = data.get("issue")

    summary_lines = [
        f"ELIGIBILITY BENEFIT SUMMARY",
        f"{'='*50}",
        f"Patient:       {patient.get('name', 'N/A')} (DOB: {patient.get('dob', 'N/A')})",
        f"Member ID:     {patient.get('member_id', 'N/A')}",
        f"Payer:         {payer.get('name', 'N/A')} (ID: {payer.get('payer_id', 'N/A')})",
        f"Plan:          {plan.get('plan_type', 'N/A')} — {plan.get('group_name', 'N/A')}",
        f"Coverage:      {plan.get('effective_date', 'N/A')} to {plan.get('termination_date', 'N/A')}",
        f"Status:        {'✅ ACTIVE' if status == 'Active' else '⚠️  ' + status}",
        f"",
        f"FINANCIAL RESPONSIBILITY",
        f"{'-'*50}",
        f"Annual Deductible:       ${benefits.get('annual_deductible', 0):,.2f}",
        f"  Deductible Met YTD:    ${benefits.get('deductible_met_ytd', 0):,.2f}",
        f"  Deductible Remaining:  ${benefits.get('deductible_remaining', 0):,.2f}",
        f"Out-of-Pocket Max:       ${benefits.get('out_of_pocket_max', 0):,.2f}",
        f"  OOP Met YTD:           ${benefits.get('out_of_pocket_met_ytd', 0):,.2f}",
        f"  OOP Remaining:         ${benefits.get('out_of_pocket_remaining', 0):,.2f}",
        f"",
        f"COPAYS & COST SHARE",
        f"{'-'*50}",
        f"Primary Care Copay:      ${benefits.get('copay_primary_care', 0)}",
        f"Specialist Copay:        ${benefits.get('copay_specialist', 0)}",
        f"Coinsurance:             {benefits.get('coinsurance_pct', 0)}% (after deductible)",
        f"Emergency Room:          {benefits.get('emergency_room', 'N/A')}",
        f"Urgent Care:             {benefits.get('urgent_care', 'N/A')}",
        f"Preventive Care:         {benefits.get('preventive_care', 'N/A')}",
        f"Mental Health:           {benefits.get('mental_health', 'N/A')}",
    ]

    if issue:
        summary_lines += [
            f"",
            f"⚠️  COVERAGE ISSUE DETECTED: {issue.replace('_', ' ').title()}",
        ]

    return "\n".join(summary_lines)


def check_eligibility_issues(
    verification_json: Annotated[str, "JSON string returned by verify_eligibility"],
) -> str:
    """
    Analyzes the eligibility verification response and flags any issues such as
    inactive coverage, wrong payer, terminated member, or pending enrollment.
    Returns actionable recommendations for each issue found.
    """
    import json
    try:
        data = json.loads(verification_json)
    except json.JSONDecodeError:
        return "Error: Could not parse eligibility verification data."

    issue = data.get("issue")
    patient = data.get("patient", {})
    payer = data.get("payer", {})
    status = data.get("coverage_status", "Unknown")

    if issue is None and status == "Active":
        return (
            "✅ No eligibility issues detected.\n"
            f"Coverage is active for patient {patient.get('name', 'N/A')} "
            f"with {payer.get('name', 'N/A')}.\n"
            "Proceed with scheduling and service delivery."
        )

    issue_map = {
        "inactive_coverage": {
            "severity": "HIGH",
            "description": "Patient's coverage appears inactive as of the service date.",
            "actions": [
                "Contact patient to confirm current insurance information.",
                "Ask for updated insurance card and re-verify.",
                "Check if there is a grace period or retroactive reinstatement.",
                "Collect self-pay deposit or financial counseling before service.",
                "Document all verification attempts per billing compliance policy.",
            ],
        },
        "wrong_payer": {
            "severity": "HIGH",
            "description": "The submitted payer does not match the payer on file for this member ID.",
            "actions": [
                "Review the patient's insurance card for the correct payer ID.",
                "Re-run eligibility with corrected payer name and member ID.",
                "Contact payer provider services to verify member enrollment.",
                "Update patient demographics in EHR/billing system.",
            ],
        },
        "terminated_member": {
            "severity": "CRITICAL",
            "description": "Member has been terminated from the plan.",
            "actions": [
                "Confirm termination date from payer.",
                "Check if termination is retroactive — assess financial exposure.",
                "Ask patient about COBRA, marketplace, or new group coverage.",
                "Collect patient responsibility or self-pay agreement.",
                "File claims only for services within active coverage dates.",
            ],
        },
        "pending_enrollment": {
            "severity": "MEDIUM",
            "description": "Member enrollment is pending — coverage is not yet active.",
            "actions": [
                "Confirm expected effective date with payer.",
                "Delay non-urgent services until coverage is confirmed active.",
                "For urgent services, document medical necessity and collect self-pay.",
                "Re-verify eligibility on the expected effective date.",
            ],
        },
        "out_of_network_only": {
            "severity": "MEDIUM",
            "description": "Provider is out-of-network for this patient's plan.",
            "actions": [
                "Notify patient of out-of-network status before service.",
                "Obtain signed financial responsibility / ABN from patient.",
                "Check if plan has out-of-network benefits and collect applicable cost share.",
                "Explore in-network referral options for the patient.",
            ],
        },
    }

    info = issue_map.get(issue or "", {
        "severity": "UNKNOWN",
        "description": f"An unrecognized eligibility issue was flagged: {issue}",
        "actions": [
            "Contact payer provider services for clarification.",
            "Do not proceed with service until issue is resolved.",
        ],
    })

    lines = [
        f"⚠️  ELIGIBILITY ISSUE DETECTED",
        f"Severity: {info['severity']}",
        f"Issue: {info['description']}",
        f"",
        f"RECOMMENDED ACTIONS:",
    ]
    for i, action in enumerate(info["actions"], 1):
        lines.append(f"  {i}. {action}")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Agent System Prompt
# ---------------------------------------------------------------------------

AGENT_INSTRUCTIONS = """
You are an expert Revenue Cycle Management (RCM) specialist focused on insurance
eligibility verification for healthcare providers.

Your capabilities:
1. Verify patient insurance eligibility using verify_eligibility with patient demographics.
2. Generate a clean benefit summary for staff using get_benefit_summary.
3. Analyze and flag coverage issues using check_eligibility_issues.

Workflow when a user requests eligibility verification:
  Step 1: Call verify_eligibility with all patient and payer details provided.
  Step 2: Call get_benefit_summary to present a human-readable benefit breakdown.
  Step 3: Call check_eligibility_issues to flag any problems and provide action items.
  Step 4: Summarize findings and recommended next steps for the front-desk or billing team.

Always format financial figures clearly (e.g., $1,500.00).
Flag any eligibility issues prominently — unchecked eligibility is a top source of denials.
Be professional and HIPAA-compliant. Never fabricate payer data.
"""

AGENT_TOOLS = [verify_eligibility, get_benefit_summary, check_eligibility_issues]


# ---------------------------------------------------------------------------
# Request / Response Models
# ---------------------------------------------------------------------------

class ChatMessageInput(BaseModel):
    role: str = "user"
    content: str


class ChatRequest(BaseModel):
    messages: list[ChatMessageInput]
    session_id: Optional[str] = None


class ChatResponse(BaseModel):
    role: str = "assistant"
    content: str


# ---------------------------------------------------------------------------
# FastAPI App + Agent Lifespan
# ---------------------------------------------------------------------------

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize the agent once on startup; clean up on shutdown."""
    endpoint = os.environ.get("AZURE_OPENAI_ENDPOINT", "")
    deployment = os.environ.get("AZURE_OPENAI_DEPLOYMENT_NAME", "gpt-4.1")

    if not endpoint:
        raise RuntimeError(
            "AZURE_OPENAI_ENDPOINT is not set. "
            "Edit eligibility_verification/.env with your Azure OpenAI or Foundry-linked endpoint."
        )

    credential = DefaultAzureCredential()
    client = AzureOpenAIChatClient(
        endpoint=endpoint,
        deployment_name=deployment,
        credential=credential,
    )
    app.state.agent = client.as_agent(
        name="EligibilityVerificationAgent",
        instructions=AGENT_INSTRUCTIONS,
        tools=AGENT_TOOLS,
    )
    yield
    await credential.close()


app = FastAPI(
    title="Eligibility Verification Agent",
    description="RCM healthcare agent for patient insurance eligibility verification.",
    version="1.0.0",
    lifespan=lifespan,
)


@app.post("/", response_model=ChatResponse, summary="Send a message to the eligibility verification agent")
@app.post("/chat", response_model=ChatResponse, include_in_schema=True, summary="Chat endpoint (alias)")
async def chat(request: ChatRequest) -> ChatResponse:
    """
    Send a list of chat messages to the Eligibility Verification Agent.
    The agent will verify eligibility, return benefit summaries, and flag coverage issues.

    Example request body:
    ```json
    {
      "messages": [
        {"role": "user", "content": "Verify eligibility for John Smith, DOB 1975-03-22, Member ID UHC987654321, UnitedHealthcare."}
      ]
    }
    ```
    """
    agent = app.state.agent

    agent_messages = [
        Message(role=m.role, text=m.content)
        for m in request.messages
    ]

    result = await agent.run(agent_messages)
    return ChatResponse(content=result.text or "")


@app.get("/health", summary="Health check")
async def health():
    return {"status": "ok", "agent": "EligibilityVerificationAgent"}


# ---------------------------------------------------------------------------
# Entry Point
# ---------------------------------------------------------------------------

def main() -> None:
    port = int(os.environ.get("DEFAULT_AD_PORT", "8088"))
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=port,
        reload=False,
        app_dir=os.path.dirname(os.path.abspath(__file__)),
    )


if __name__ == "__main__":
    main()
