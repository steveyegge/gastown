"""
Denial Management Agent — RCM Healthcare Workflow
================================================
Uses Microsoft Agent Framework (agent-framework-core==1.0.0rc2, AzureOpenAIChatClient)
Analyzes insurance claim denials, identifies root causes, recommends corrective
actions, and drafts professional appeal letters.

HTTP server mode (default):
    python main.py

Debug with AI Toolkit Agent Inspector:
    python -m debugpy --listen 127.0.0.1:5679 -m agentdev run main.py --verbose --port 8087

Environment variables (see .env):
    AZURE_OPENAI_ENDPOINT          — your Azure OpenAI or Foundry-linked AOAI endpoint
    AZURE_OPENAI_DEPLOYMENT_NAME   — model deployment name (e.g. gpt-5.1 / gpt-4.1)

Note on Foundry: If you have an Azure AI Foundry project, it exposes an Azure OpenAI
compatible endpoint. Set AZURE_OPENAI_ENDPOINT to that resource's AOAI URL.
"""

from __future__ import annotations

import asyncio
import os
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
# RCM Domain Tools — Denial Management
# ---------------------------------------------------------------------------

# Standard denial reason code mapping (CARC / RARC)
_DENIAL_CODE_MAP: dict[str, dict] = {
    "CO-4": {
        "category": "Service inconsistent with modifier",
        "description": "The procedure code is inconsistent with the modifier submitted or a required modifier is missing.",
        "payer_notes": "Verify modifier accuracy against payer LCD/NCD policy.",
    },
    "CO-11": {
        "category": "Diagnosis code not covered",
        "description": "The diagnosis is inconsistent with the procedure.",
        "payer_notes": "Review ICD-10 linkage and ensure medical necessity documentation.",
    },
    "CO-22": {
        "category": "Coordination of benefits",
        "description": "This care may be covered by another payer per coordination of benefits.",
        "payer_notes": "Confirm primary/secondary payer order and resubmit with correct coordination.",
    },
    "CO-50": {
        "category": "Non-covered service",
        "description": "These services are not covered under the patient's benefit plan.",
        "payer_notes": "Check benefit plan exclusions; consider ABN if Medicare beneficiary.",
    },
    "CO-97": {
        "category": "Payment included in allowance for another service",
        "description": "The benefit for this service is included in the payment or allowance for another service.",
        "payer_notes": "Review bundling edits (NCCI); may need to unbundle or submit with modifier 59/XU.",
    },
    "PR-96": {
        "category": "Patient responsibility — non-covered charge",
        "description": "Non-covered charge; this is the patient's responsibility.",
        "payer_notes": "Bill patient or write off per contractual agreement; verify benefit plan.",
    },
    "CO-18": {
        "category": "Duplicate claim",
        "description": "Exact duplicate claim or service submitted previously.",
        "payer_notes": "Verify claim was not already paid; if unpaid, resubmit with original claim reference.",
    },
    "CO-29": {
        "category": "Timely filing",
        "description": "The time limit for filing has expired.",
        "payer_notes": "Submit proof of timely filing (clearinghouse confirmation, EHR logs) with appeal.",
    },
    "CO-167": {
        "category": "Diagnosis not covered",
        "description": "This diagnosis(es) is not covered; claim denied.",
        "payer_notes": "Review payer coverage policies; may need prior authorization or alternate code.",
    },
    "OA-23": {
        "category": "Payment adjusted — withholding",
        "description": "The impact of prior payer(s) adjudication including payments and/or adjustments.",
        "payer_notes": "Verify COB amounts from primary EOB; resubmit secondary with primary EOB attached.",
    },
}


def map_denial_code(
    denial_code: Annotated[str, "The denial reason code, e.g. CO-4, CO-97, PR-96"]
) -> str:
    """
    Maps an insurance denial reason code to a human-readable category,
    description, and payer-specific notes.
    """
    code = denial_code.strip().upper()
    info = _DENIAL_CODE_MAP.get(code)
    if info:
        return (
            f"Denial Code: {code}\n"
            f"Category: {info['category']}\n"
            f"Description: {info['description']}\n"
            f"Payer Notes: {info['payer_notes']}"
        )
    return (
        f"Denial Code: {code}\n"
        f"Category: Unknown / not in local reference\n"
        f"Description: Code not found in the standard CARC/RARC reference. "
        f"Consult the payer's remittance advice or contact payer provider services.\n"
        f"Payer Notes: Request itemized EOB from the payer to clarify denial reason."
    )


def recommend_appeal_strategy(
    denial_code: Annotated[str, "The denial reason code"],
    payer_name: Annotated[str, "Name of the insurance payer"],
    service_date: Annotated[str, "Date of service (YYYY-MM-DD)"],
    procedure_code: Annotated[str, "CPT or HCPCS procedure code"],
    diagnosis_code: Annotated[str, "ICD-10-CM diagnosis code"],
) -> str:
    """
    Recommends a corrective action and appeal strategy based on the denial code,
    payer, procedure, and diagnosis.
    """
    code = denial_code.strip().upper()
    strategies: dict[str, str] = {
        "CO-4": (
            "1. Audit the original claim for modifier accuracy.\n"
            "2. Verify that the modifier is allowed by the payer's policy for CPT {cpt}.\n"
            "3. If modifier was omitted, resubmit with corrected claim (not appeal).\n"
            "4. If modifier was correct, appeal with supporting documentation: operative note, "
            "office notes, payer modifier policy reference.\n"
            "5. Cite payer's own modifier guidelines in the appeal letter."
        ),
        "CO-29": (
            "1. Pull clearinghouse transmission logs showing original submission date.\n"
            "2. Obtain payer-accepted proof of timely filing (EDI 277CA acknowledgment).\n"
            "3. If filing was within timely limits, appeal with all proof documents.\n"
            "4. If genuinely late due to payer error or COB delay, cite the exceptions clause.\n"
            "5. Include signed patient financial responsibility form if applicable."
        ),
        "CO-97": (
            "1. Review NCCI bundling edits for CPT {cpt} and the companion code.\n"
            "2. Determine if a modifier (59, XU, XE, XP, XS) is appropriate to unbundle.\n"
            "3. If services are truly separate and distinct, appeal with:\n"
            "   - Operative notes documenting separate sessions or anatomical sites.\n"
            "   - CMS NCCI policy rationale.\n"
            "4. If bundled correctly by payer, write off the bundled amount per contract."
        ),
        "CO-22": (
            "1. Verify coordination of benefits (COB) order with patient.\n"
            "2. Obtain the primary payer EOB showing their adjudication.\n"
            "3. Resubmit to correct payer tier with primary EOB attached.\n"
            "4. If COB is in dispute, submit to both payers with a COB dispute letter."
        ),
        "CO-18": (
            "1. Pull the original claim from your billing system and compare.\n"
            "2. If already paid, reconcile the account — no action needed.\n"
            "3. If not paid and this is not a true duplicate, appeal with:\n"
            "   - Original claim number, dates, and service details.\n"
            "   - Explanation of why this is a separate and distinct service."
        ),
    }
    strategy = strategies.get(code, (
        "1. Review payer's denial rationale on the EOB/ERA carefully.\n"
        "2. Consult the payer's provider manual for appeal procedures.\n"
        "3. Gather all supporting clinical documentation (notes, orders, auth).\n"
        "4. Draft a formal appeal letter citing payer contract, medical necessity, and CPT guidelines.\n"
        "5. Submit within payer's appeal deadline (typically 90–180 days from denial date)."
    ))
    strategy = strategy.replace("{cpt}", procedure_code)
    deadline_note = f"\nAppeal Deadline: Most payers require appeals within 90–180 days of denial for {payer_name}.\n"
    return (
        f"Appeal Strategy for {code} — {payer_name}\n"
        f"Service Date: {service_date} | CPT: {procedure_code} | ICD-10: {diagnosis_code}\n"
        f"{'='*60}\n"
        f"{strategy}"
        f"{deadline_note}"
    )


def draft_appeal_letter(
    patient_name: Annotated[str, "Full name of the patient"],
    patient_dob: Annotated[str, "Patient date of birth (YYYY-MM-DD)"],
    member_id: Annotated[str, "Insurance member ID"],
    provider_name: Annotated[str, "Billing provider name"],
    payer_name: Annotated[str, "Insurance payer name"],
    claim_number: Annotated[str, "Original claim number"],
    denial_code: Annotated[str, "Denial reason code"],
    service_date: Annotated[str, "Date of service"],
    procedure_code: Annotated[str, "CPT/HCPCS code"],
    diagnosis_code: Annotated[str, "ICD-10-CM diagnosis code"],
    denial_reason_summary: Annotated[str, "Plain-language summary of why the claim was denied"],
    appeal_rationale: Annotated[str, "Clinical or administrative rationale for the appeal"],
) -> str:
    """
    Drafts a professional insurance claim appeal letter with all required
    payer fields, clinical justification, and supporting document checklist.
    """
    letter = f"""
[Date: {'{TODAY}'}]

Appeals Department
{payer_name}
[Payer Address]

Re: Formal Appeal — Claim Denial
Patient: {patient_name} | DOB: {patient_dob} | Member ID: {member_id}
Claim Number: {claim_number} | Service Date: {service_date}
Provider: {provider_name}
CPT: {procedure_code} | ICD-10: {diagnosis_code}
Denial Code: {denial_code}

Dear {payer_name} Appeals Department,

We are writing on behalf of {provider_name} to formally appeal the denial of the above-referenced
claim. The claim was denied with reason code {denial_code}: "{denial_reason_summary}".

REASON FOR APPEAL:
{appeal_rationale}

SUPPORTING DOCUMENTATION ENCLOSED:
  1. Copy of original claim (CMS-1500 / UB-04)
  2. Remittance advice / EOB showing denial
  3. Complete clinical documentation (office/operative notes, orders)
  4. Copy of payer authorization (if applicable)
  5. Relevant payer policy or LCD/NCD reference
  6. Proof of timely filing (if applicable — EDI acknowledgment)

We respectfully request that {payer_name} reconsider this denial and process the claim for
appropriate reimbursement per our provider agreement and the patient's benefit plan.

This service was medically necessary and was provided in accordance with the payer's coverage
guidelines. Denial of this claim may adversely affect the patient's access to care.

If additional information is needed, please contact our billing department at [BILLING PHONE].
We request a written response within 30 days per standard appeal procedures.

Sincerely,

{provider_name}
Billing/Appeals Department
[Address] | [Phone] | [Fax] | [NPI]

Attachments: See supporting documentation list above.
""".replace("{TODAY}", __import__("datetime").date.today().isoformat())
    return letter.strip()


# ---------------------------------------------------------------------------
# Agent System Prompt
# ---------------------------------------------------------------------------

AGENT_INSTRUCTIONS = """
You are an expert Revenue Cycle Management (RCM) specialist focused on insurance claim
denial management and appeals.

Your capabilities:
1. Map denial reason codes (CARC/RARC) to human-readable categories using map_denial_code.
2. Recommend evidence-based corrective actions and appeal strategies using recommend_appeal_strategy.
3. Draft professional, payer-specific appeal letters using draft_appeal_letter.

Workflow when a user presents a denial:
  Step 1: Call map_denial_code to explain the denial code.
  Step 2: Call recommend_appeal_strategy to outline the corrective actions.
  Step 3: Call draft_appeal_letter to produce a ready-to-send appeal letter.
  Step 4: Summarize the key actions the billing team must take.

Always be precise, professional, and compliant with HIPAA.
Reference CMS NCCI edits, payer LCDs, and provider manuals when appropriate.
"""

AGENT_TOOLS = [map_denial_code, recommend_appeal_strategy, draft_appeal_letter]


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
            "Edit denial_management/.env with your Azure OpenAI or Foundry-linked endpoint."
        )

    credential = DefaultAzureCredential()
    client = AzureOpenAIChatClient(
        endpoint=endpoint,
        deployment_name=deployment,
        credential=credential,
    )
    app.state.agent = client.as_agent(
        name="DenialManagementAgent",
        instructions=AGENT_INSTRUCTIONS,
        tools=AGENT_TOOLS,
    )
    yield
    # credential cleanup
    await credential.close()


app = FastAPI(
    title="Denial Management Agent",
    description="RCM healthcare agent for insurance claim denial analysis and appeal generation.",
    version="1.0.0",
    lifespan=lifespan,
)


@app.post("/", response_model=ChatResponse, summary="Send a message to the denial management agent")
@app.post("/chat", response_model=ChatResponse, include_in_schema=True, summary="Chat endpoint (alias)")
async def chat(request: ChatRequest) -> ChatResponse:
    """
    Send a list of chat messages to the Denial Management Agent.
    The agent will map denial codes, recommend strategies, and draft appeal letters.

    Example request body:
    ```json
    {
      "messages": [
        {"role": "user", "content": "Analyze denial CO-97 for CPT 27447. Payer: BCBS."}
      ]
    }
    ```
    """
    agent = app.state.agent

    # Convert request messages → agent Message objects
    agent_messages = [
        Message(role=m.role, text=m.content)
        for m in request.messages
    ]

    result = await agent.run(agent_messages)
    return ChatResponse(content=result.text or "")


@app.get("/health", summary="Health check")
async def health():
    return {"status": "ok", "agent": "DenialManagementAgent"}


# ---------------------------------------------------------------------------
# Entry Point
# ---------------------------------------------------------------------------

def main() -> None:
    port = int(os.environ.get("DEFAULT_AD_PORT", "8087"))
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=port,
        reload=False,
        app_dir=os.path.dirname(os.path.abspath(__file__)),
    )


if __name__ == "__main__":
    main()
