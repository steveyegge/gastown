/**
 * Azure AI Foundry client — single control plane via AIProjectClient.
 *
 * All access to models (deployments), agents, and evaluations flows through
 * one `AIProjectClient` instance pointed at the project endpoint.
 *
 * Required environment variable:
 *   AZURE_AI_PROJECT_ENDPOINT
 *     – e.g. https://<hub>.services.ai.azure.com/api/projects/<project-name>
 *
 * Optional (only needed outside managed identity / az cli context):
 *   AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_CLIENT_SECRET
 */

import { AIProjectClient } from "@azure/ai-projects"
import { ClientSecretCredential, DefaultAzureCredential } from "@azure/identity"
import type { TokenCredential } from "@azure/core-auth"
import type {
  ModelCardData,
  EvaluationMetric,
  SafetyMetric,
  BenchmarkResult,
} from "./types"

// ── Evaluation REST types (partial — not yet in the JS SDK) ──────────────────

/** Scope for the Azure AI Foundry project data plane. */
const COGNITIVE_SCOPE = "https://ai.azure.com/.default"

/**
 * Evaluations REST API version.
 *
 * ⚠️  RESOURCE TYPE LIMITATION
 * The evaluations API is only available on **Azure AI Hub workspace** endpoints
 * (hostname pattern: `{hub}.api.azureml.ms`).  It is NOT available on the
 * basic AI Services / Azure OpenAI project endpoint used here
 * (`*.services.ai.azure.com`), which returns `400 "API version not supported"`
 * for every version because the feature is not provisioned on that resource SKU.
 *
 * To enable live evaluation data, set AZURE_AI_HUB_WORKSPACE_URL in .env.local
 * to your full AI Hub project workspace endpoint:
 *   https://{region}.api.azureml.ms/raisvc/v1.0/subscriptions/{sub}/
 *     resourceGroups/{rg}/providers/Microsoft.MachineLearningServices/
 *     workspaces/{hub-workspace}
 *
 * Until that is set this path is deliberately skipped — falls back to per-model
 * demo benchmark data from MODEL_METADATA.
 */
const EVALS_API_VERSION = "2024-10-01-preview"

interface EvalRun {
  id: string
  displayName?: string
  status?: string
  createdDateTime?: string
  tags?: Record<string, string>
  metrics?: Record<string, number>
}

// ── Singleton AIProjectClient ─────────────────────────────────────────────────

/**
 * Build an Azure TokenCredential.
 *
 * Prefers the service-principal values in AZURE_AD_CLIENT_ID /
 * AZURE_AD_CLIENT_SECRET / AZURE_AD_TENANT_ID (set in .env.local).
 * Falls back to DefaultAzureCredential (managed identity, az cli, env vars
 * with the standard AZURE_* prefix) when the AD vars are absent.
 */
function buildCredential(): TokenCredential {
  const clientId     = process.env.AZURE_AD_CLIENT_ID
  const clientSecret = process.env.AZURE_AD_CLIENT_SECRET
  const tenantId     = process.env.AZURE_AD_TENANT_ID

  if (clientId && clientSecret && tenantId) {
    return new ClientSecretCredential(tenantId, clientId, clientSecret)
  }
  return new DefaultAzureCredential()
}

let _client: AIProjectClient | null = null
let _credential: TokenCredential | null = null

function getCredential(): TokenCredential {
  if (!_credential) _credential = buildCredential()
  return _credential
}

/**
 * Returns the singleton AIProjectClient, constructing it on first call.
 * `AIProjectClient.fromEndpoint()` makes the project endpoint the single
 * control plane for deployments, agents, connections, datasets, and telemetry.
 */
function getClient(): AIProjectClient {
  if (_client) return _client
  const endpoint = process.env.AZURE_AI_PROJECT_ENDPOINT
  if (!endpoint) throw new Error("AZURE_AI_PROJECT_ENDPOINT is not configured")
  _client = AIProjectClient.fromEndpoint(endpoint, getCredential())
  return _client
}

// ── Deployments (models) ──────────────────────────────────────────────────────

/**
 * List every model deployment registered in this Foundry project.
 * Useful for building a live model catalog page.
 */
export async function listDeployments(): Promise<Array<{ name: string; type: string }>> {
  const results: Array<{ name: string; type: string }> = []
  for await (const d of getClient().deployments.list()) {
    results.push({ name: d.name, type: d.type })
  }
  return results
}

/**
 * Get a single deployment by name. Returns null when not found.
 */
export async function getDeployment(name: string) {
  try {
    return await getClient().deployments.get(name)
  } catch {
    return null
  }
}

// ── Agents ────────────────────────────────────────────────────────────────────

/**
 * List all agents in this project via the AgentsClient sub-client
 * vended by AIProjectClient.
 */
export async function listFoundryAgents() {
  const results = []
  for await (const agent of getClient().agents.listAgents()) {
    results.push(agent)
  }
  return results
}

// ── Telemetry ─────────────────────────────────────────────────────────────────

/**
 * Retrieve the App Insights connection string via the Foundry telemetry
 * operation. Falls back to the env var when the SDK call fails.
 */
export async function getAppInsightsConnectionString(): Promise<string | null> {
  try {
    return await getClient().telemetry.getApplicationInsightsConnectionString()
  } catch {
    return process.env.APPLICATIONINSIGHTS_CONNECTION_STRING ?? null
  }
}

// ── Evaluations (REST — AI Hub workspace only) ───────────────────────────────

/**
 * Fetch evaluation runs from an **AI Hub workspace** endpoint.
 *
 * This is intentionally guarded behind AZURE_AI_HUB_WORKSPACE_URL.  The
 * standard AI Services project endpoint (`services.ai.azure.com`) does NOT
 * support this API — it returns `400 "API version not supported"` for every
 * known version because the evaluations feature requires an AI Hub workspace
 * backed by Azure ML compute.  Calling it unconditionally wastes a round-trip
 * and produces a misleading error on every model card load.
 *
 * Set AZURE_AI_HUB_WORKSPACE_URL in .env.local to enable live eval data:
 *   AZURE_AI_HUB_WORKSPACE_URL=https://{region}.api.azureml.ms/raisvc/v1.0/
 *     subscriptions/{sub}/resourceGroups/{rg}/providers/
 *     Microsoft.MachineLearningServices/workspaces/{hub-ws}
 */
async function fetchEvaluationRuns(): Promise<EvalRun[]> {
  const hubWorkspaceUrl = process.env.AZURE_AI_HUB_WORKSPACE_URL
  if (!hubWorkspaceUrl) {
    // AI Services endpoint doesn't support evaluations — skip silently.
    return []
  }

  const { token } = await getCredential().getToken(COGNITIVE_SCOPE)
  const url = `${hubWorkspaceUrl.replace(/\/$/, "")}/evaluations/runs?api-version=${EVALS_API_VERSION}`
  const res = await fetch(url, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    next: { revalidate: 300 },
  } as RequestInit & { next?: { revalidate: number } })

  if (!res.ok) {
    throw new Error(`Foundry Hub evaluations API ${res.status}: ${await res.text()}`)
  }
  const body = await res.json()
  return (body.value ?? []) as EvalRun[]
}

// ── Metric mappers ────────────────────────────────────────────────────────────

const METRIC_DESCRIPTIONS: Record<string, string> = {
  groundedness: "Whether the response is grounded in the provided context",
  relevance: "How relevant the response is to the input query",
  coherence: "Logical and structural consistency of the response",
  fluency: "Grammatical correctness and readability",
  f1_score: "Token-level overlap with reference answers",
  bleu: "BLEU n-gram overlap with reference text",
  rouge: "ROUGE-L recall over reference summaries",
}
const PERCENTAGE_METRICS = new Set(["f1_score", "bleu", "rouge"])

function mapEvaluationMetrics(metrics: Record<string, number>): EvaluationMetric[] {
  return Object.entries(metrics)
    .filter(([k]) => !k.toLowerCase().includes("safety") && !k.toLowerCase().includes("defect"))
    .map(([key, value]) => {
      const lk = key.toLowerCase().replace(/[\s-]/g, "_")
      const isPct = PERCENTAGE_METRICS.has(lk)
      return {
        name: key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase()),
        score: isPct ? Math.round(value * 1000) / 1000 : Math.min(Number(value.toFixed(2)), 5),
        maxScore: isPct ? 1 : 5,
        description: METRIC_DESCRIPTIONS[lk] ?? `Evaluation metric: ${key}`,
      } satisfies EvaluationMetric
    })
}

const SAFETY_KEYS: Array<[string, string]> = [
  ["hate_unfairness", "Hate & Unfairness"],
  ["violence",        "Violence"],
  ["sexual",          "Sexual Content"],
  ["self_harm",       "Self-Harm"],
]

function mapSafetyMetrics(metrics: Record<string, number>): SafetyMetric[] {
  return SAFETY_KEYS.map(([key, label]) => {
    const rate = metrics[key] ?? metrics[`${key}_defect_rate`] ?? 0
    const severity: SafetyMetric["severity"] =
      rate > 0.1 ? "high" : rate > 0.05 ? "medium" : rate > 0.01 ? "low" : "none"
    return { category: label, severity, defectRate: rate }
  })
}

function findLatestRun(runs: EvalRun[], modelId: string): EvalRun | undefined {
  const matching = runs.filter(
    (r) =>
      (!r.status || r.status === "Completed") &&
      (r.displayName?.toLowerCase().includes(modelId.toLowerCase()) ||
        Object.values(r.tags ?? {}).some((v) =>
          v.toLowerCase().includes(modelId.toLowerCase())
        ) ||
        runs.length > 0) // fallback: accept any completed run
  )
  return matching.sort((a, b) =>
    (b.createdDateTime ?? "").localeCompare(a.createdDateTime ?? "")
  )[0]
}

// ── Asset ID → deployment name map ───────────────────────────────────────────
// Maps UI asset IDs to deployment names registered in this Foundry project.
// Extend as more models are onboarded.

const MODEL_TO_DEPLOYMENT: Record<string, string> = {
  // Maps UI asset IDs → Foundry deployment names (18 live deployments)
  "model-gpt4o":        "gpt-4o",
  "model-gpt5chat":     "gpt-5-chat",
  "model-gpt52":        "gpt-5.2",
  "model-gpt5mini":     "gpt-5-mini",
  "model-gpt5codex":    "gpt-5-codex",
  "model-o1":           "o1",
  "model-deepseek-r1":  "DeepSeek-R1-0528",
  "model-gpt-oss-120b": "gpt-oss-120b",
  "model-gpt4":         "gpt-4",
  "model-gpt41":        "gpt-4.1",
  "model-gpt35turbo":   "gpt-35-turbo",
  "model-mistral-docai":"mistral-document-ai-2505",
  "model-gptimage15":   "gpt-image-1.5",
  "model-gpt4omini-rt": "gpt-4o-mini-realtime-preview",
  "model-gpt4omini-tts":"gpt-4o-mini-tts",
  "model-ada002":       "text-embedding-ada-002",
  "model-embed3small":  "text-embedding-3-small",
  "model-embed3large":  "text-embedding-3-large",
}

// ── Fallback demo data ────────────────────────────────────────────────────────

/** Per-model benchmark / architecture metadata for all 18 Foundry deployments. */
const MODEL_METADATA: Record<string, {
  arch: string
  cutoff: string
  benchmarks: BenchmarkResult[]
  intendedUse: string[]
  isEmbedding?: boolean
  isVoice?: boolean
}> = {
  "model-gpt4o": {
    arch: "GPT-4o multimodal transformer, vision + audio + text, fine-tuned for healthcare",
    cutoff: "April 2024",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 90.2, dataset: "MedQA 4-option",    date: "2025-10-01" },
      { name: "MMLU Medicine",   score: 91.5, dataset: "MMLU Pro",           date: "2025-10-01" },
      { name: "MedMCQA",         score: 84.1, dataset: "MedMCQA Dev",        date: "2025-10-01" },
      { name: "HealthDocs OCR",  score: 95.3, dataset: "Internal EOB set",   date: "2025-10-01" },
    ],
    intendedUse: ["Complex clinical reasoning", "Multimodal document analysis", "Patient triage support", "RCM orchestration"],
  },
  "model-gpt5chat": {
    arch: "GPT-5 decoder-only transformer, 200B+ params, RLHF-tuned for instruction following",
    cutoff: "January 2026",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 93.1, dataset: "MedQA 4-option",    date: "2026-01-15" },
      { name: "MMLU Medicine",   score: 94.6, dataset: "MMLU Pro",           date: "2026-01-15" },
      { name: "ChatMD Eval",     score: 96.2, dataset: "Internal chat eval", date: "2026-01-15" },
    ],
    intendedUse: ["Patient intake conversations", "Clinical Q&A systems", "Care navigation bots", "Multi-turn health coaching"],
  },
  "model-gpt52": {
    arch: "GPT-5.2 decoder-only transformer with enhanced safety alignment and reduced hallucination tuning",
    cutoff: "February 2026",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 94.3, dataset: "MedQA 4-option",    date: "2026-02-01" },
      { name: "Hallucination",   score: 97.8, dataset: "TruthfulQA Med",    date: "2026-02-01" },
      { name: "MMLU Medicine",   score: 95.2, dataset: "MMLU Pro",           date: "2026-02-01" },
    ],
    intendedUse: ["Clinical documentation", "Evidence-based Q&A", "Healthcare content generation", "Safety-critical workflows"],
  },
  "model-gpt5mini": {
    arch: "GPT-5 Mini — distilled 7B decoder, low-latency, optimised for structured outputs",
    cutoff: "December 2025",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 82.1, dataset: "MedQA 4-option",    date: "2025-12-01" },
      { name: "Claim Scrub",     score: 94.5, dataset: "Internal RCM set",  date: "2025-12-01" },
      { name: "Code Extraction", score: 91.3, dataset: "ICD-10 benchmark",  date: "2025-12-01" },
    ],
    intendedUse: ["High-throughput claim scrubbing", "Eligibility checks", "Code extraction from notes", "Batch RCM processing"],
  },
  "model-gpt5codex": {
    arch: "GPT-5 Codex — code-specialized decoder with FHIR and HL7 vocabulary fine-tuning",
    cutoff: "November 2025",
    benchmarks: [
      { name: "FHIR Schema Gen",    score: 96.4, dataset: "FHIR R4 test suite", date: "2025-11-15" },
      { name: "HL7 Message Parse",  score: 94.8, dataset: "Internal HL7 set",   date: "2025-11-15" },
      { name: "HumanEval (Python)", score: 88.2, dataset: "HumanEval",          date: "2025-11-15" },
    ],
    intendedUse: ["FHIR integration code generation", "HL7 message parsing", "Healthcare API scaffolding", "RCM automation scripts"],
  },
  "model-o1": {
    arch: "o1 chain-of-thought LLM with extended reasoning tokens and verification steps",
    cutoff: "September 2024",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 91.8, dataset: "MedQA 4-option",    date: "2025-09-01" },
      { name: "GPQA Diamond",    score: 78.3, dataset: "GPQA Diamond",      date: "2025-09-01" },
      { name: "DiagStep Eval",   score: 89.6, dataset: "Differential Dx",   date: "2025-09-01" },
    ],
    intendedUse: ["Differential diagnosis support", "Coverage policy interpretation", "Multi-step clinical reasoning", "Audit trail generation"],
  },
  "model-deepseek-r1": {
    arch: "DeepSeek R1 — 671B MoE open-source reasoner with visible chain-of-thought",
    cutoff: "May 2025",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 87.4, dataset: "MedQA 4-option",    date: "2025-06-01" },
      { name: "MedMCQA",         score: 80.9, dataset: "MedMCQA Dev",       date: "2025-06-01" },
      { name: "COT Faithfulness", score: 92.1, dataset: "Internal eval",    date: "2025-06-01" },
    ],
    intendedUse: ["Explainable AI decision support", "Clinical benchmark evaluation", "Open-weight research", "Audit-ready reasoning chains"],
  },
  "model-gpt-oss-120b": {
    arch: "GPT OSS 120B — open-weight dense transformer, on-premise deployable, no telemetry",
    cutoff: "October 2025",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 85.6, dataset: "MedQA 4-option",    date: "2025-10-15" },
      { name: "MMLU Medicine",   score: 86.3, dataset: "MMLU Pro",           date: "2025-10-15" },
      { name: "Air-gap Perf",    score: 99.9, dataset: "Internal uptime",   date: "2025-10-15" },
    ],
    intendedUse: ["Air-gapped healthcare deployments", "On-premise HIPAA compliance", "Zero-egress clinical AI", "Private cloud deployments"],
  },
  "model-gpt4": {
    arch: "GPT-4 dense transformer, 1.8T params, RLHF, context window 128k tokens",
    cutoff: "April 2023",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 86.1, dataset: "MedQA 4-option",    date: "2025-08-01" },
      { name: "MMLU Medicine",   score: 88.4, dataset: "MMLU Pro",           date: "2025-08-01" },
      { name: "PubMedQA",        score: 79.5, dataset: "PubMedQA-L",         date: "2025-08-01" },
    ],
    intendedUse: ["Clinical note summarization", "EHR Q&A", "RCM documentation", "Multi-language healthcare content"],
  },
  "model-gpt41": {
    arch: "GPT-4.1 — 256k token context, improved long-document recall and instruction adherence",
    cutoff: "October 2024",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 88.7, dataset: "MedQA 4-option",    date: "2025-09-15" },
      { name: "LongDoc Recall",  score: 93.2, dataset: "LooGLE Med",        date: "2025-09-15" },
      { name: "MMLU Medicine",   score: 90.1, dataset: "MMLU Pro",           date: "2025-09-15" },
    ],
    intendedUse: ["Extended patient history analysis", "Multi-document clinical summarization", "Long-form RCM audits", "Comprehensive prior auth review"],
  },
  "model-gpt35turbo": {
    arch: "GPT-3.5 Turbo — 20B param decoder, optimised for chat and structured classification",
    cutoff: "September 2021",
    benchmarks: [
      { name: "MedQA (USMLE)",  score: 68.4, dataset: "MedQA 4-option",    date: "2025-06-01" },
      { name: "Claim Route Acc", score: 91.2, dataset: "Internal RCM set",  date: "2025-06-01" },
      { name: "Eligibility Acc", score: 93.5, dataset: "Internal eval",     date: "2025-06-01" },
    ],
    intendedUse: ["High-volume eligibility lookups", "Claim status routing", "Basic triage and routing", "Cost-efficient batch processing"],
  },
  "model-mistral-docai": {
    arch: "Mistral Document AI 2505 — LayoutLM + vision encoder fine-tuned on 50M+ healthcare forms",
    cutoff: "April 2025",
    benchmarks: [
      { name: "Form 1500 F1",    score: 97.3, dataset: "CMS-1500 test set", date: "2025-05-15" },
      { name: "UB-04 F1",        score: 96.8, dataset: "UB-04 test set",    date: "2025-05-15" },
      { name: "EOB Extraction",  score: 95.1, dataset: "Internal EOB set",  date: "2025-05-15" },
      { name: "DocVQA",          score: 93.7, dataset: "DocVQA Test",        date: "2025-05-15" },
    ],
    intendedUse: ["CMS-1500 / UB-04 form extraction", "EOB and ERA parsing", "Insurance card recognition", "Remittance advice digitization"],
  },
  "model-gptimage15": {
    arch: "GPT Image 1.5 — diffusion + vision encoder hybrid, 2B params, medical image fine-tuned",
    cutoff: "November 2025",
    benchmarks: [
      { name: "Wound Assessment", score: 88.9, dataset: "WoundCare bench",  date: "2025-12-01" },
      { name: "Report Annotation", score: 91.4, dataset: "RadReport set",   date: "2025-12-01" },
      { name: "Patient Diagram",   score: 94.2, dataset: "MedArt eval",     date: "2025-12-01" },
    ],
    intendedUse: ["Radiology report annotation", "Wound image assessment", "Patient-facing medical diagrams", "Clinical image Q&A"],
  },
  "model-gpt4omini-rt": {
    arch: "GPT-4o Mini Realtime — streaming audio-text transformer, <300ms latency",
    cutoff: "September 2024",
    benchmarks: [
      { name: "Transcription WER",  score: 97.2, dataset: "Clinical speech", date: "2025-10-01" },
      { name: "Note Accuracy",      score: 92.8, dataset: "Ambient doc eval", date: "2025-10-01" },
      { name: "Latency p95 (ms)",  score: 98.0, dataset: "Internal latency",  date: "2025-10-01" },
    ],
    isVoice: true,
    intendedUse: ["Ambient clinical documentation", "Patient phone bots", "Real-time transcription", "Voice-driven EHR updates"],
  },
  "model-gpt4omini-tts": {
    arch: "GPT-4o Mini TTS — neural TTS with 50+ voice styles, 24 languages supported",
    cutoff: "November 2024",
    benchmarks: [
      { name: "MOS Score",       score: 4.82, dataset: "Human eval panel",    date: "2025-11-15" },
      { name: "Language Cov.",   score: 96.7, dataset: "Coverage test",        date: "2025-11-15" },
      { name: "Clinical Terms",  score: 94.1, dataset: "Med pronunciation",    date: "2025-11-15" },
    ],
    isVoice: true,
    intendedUse: ["Appointment reminder calls", "Medication instruction narration", "Care plan audio delivery", "Multilingual patient communication"],
  },
  "model-ada002": {
    arch: "text-embedding-ada-002 — 1536-dim dense encoder, optimised for semantic similarity",
    cutoff: "September 2021",
    benchmarks: [
      { name: "MTEB Clinical",   score: 76.4, dataset: "MTEB clinical",     date: "2025-06-01" },
      { name: "BEIR BioASQ",     score: 71.2, dataset: "BioASQ",            date: "2025-06-01" },
      { name: "Code Retrieval",  score: 80.1, dataset: "CodeSearchNet",      date: "2025-06-01" },
    ],
    isEmbedding: true,
    intendedUse: ["Clinical semantic search", "RAG over EHR notes", "ICD/CPT code retrieval", "Duplicate claim detection"],
  },
  "model-embed3small": {
    arch: "text-embedding-3-small — 1536-dim encoder (256-dim truncatable), Matryoshka training",
    cutoff: "November 2023",
    benchmarks: [
      { name: "MTEB Clinical",   score: 81.3, dataset: "MTEB clinical",     date: "2025-08-01" },
      { name: "BEIR BioASQ",     score: 76.5, dataset: "BioASQ",            date: "2025-08-01" },
      { name: "Claim Similarity",score: 88.2, dataset: "Internal RCM set",  date: "2025-08-01" },
    ],
    isEmbedding: true,
    intendedUse: ["Real-time eligibility RAG", "Claim similarity scoring", "Semantic code search", "Healthcare knowledge retrieval"],
  },
  "model-embed3large": {
    arch: "text-embedding-3-large — 3072-dim encoder, best-in-class semantic fidelity",
    cutoff: "November 2023",
    benchmarks: [
      { name: "MTEB Clinical",   score: 85.7, dataset: "MTEB clinical",     date: "2025-08-01" },
      { name: "BEIR BioASQ",     score: 81.9, dataset: "BioASQ",            date: "2025-08-01" },
      { name: "FHIR Retrieval",  score: 93.4, dataset: "Internal FHIR set", date: "2025-08-01" },
    ],
    isEmbedding: true,
    intendedUse: ["Healthcare knowledge graph retrieval", "Complex FHIR resource lookup", "High-precision clinical concept matching", "Multi-hop RAG pipelines"],
  },
}

function getMockModelCard(modelId: string): ModelCardData {
  const meta = MODEL_METADATA[modelId]
  const deployment = MODEL_TO_DEPLOYMENT[modelId] ?? modelId

  const evaluationMetrics: EvaluationMetric[] = meta?.isEmbedding || meta?.isVoice
    ? [
        { name: "Relevance",    score: 4.8, maxScore: 5, description: METRIC_DESCRIPTIONS.relevance },
        { name: "Coherence",    score: 4.7, maxScore: 5, description: METRIC_DESCRIPTIONS.coherence },
        { name: "F1 Score",     score: 0.87, maxScore: 1, description: METRIC_DESCRIPTIONS.f1_score },
      ]
    : [
        { name: "Groundedness", score: 4.7, maxScore: 5, description: METRIC_DESCRIPTIONS.groundedness },
        { name: "Relevance",    score: 4.8, maxScore: 5, description: METRIC_DESCRIPTIONS.relevance },
        { name: "Coherence",    score: 4.6, maxScore: 5, description: METRIC_DESCRIPTIONS.coherence },
        { name: "Fluency",      score: 4.7, maxScore: 5, description: METRIC_DESCRIPTIONS.fluency },
      ]

  const safetyMetrics: SafetyMetric[] = [
    { category: "Hate & Unfairness", severity: "none", defectRate: 0.002 },
    { category: "Violence",          severity: "none", defectRate: 0.001 },
    { category: "Sexual Content",    severity: "none", defectRate: 0.001 },
    { category: "Self-Harm",         severity: "none", defectRate: 0.003 },
  ]

  const defaultBenchmarks: BenchmarkResult[] = [
    { name: "MedQA (USMLE)",  score: 80.0, dataset: "MedQA 4-option", date: "2025-10-01" },
    { name: "MMLU Medicine",   score: 82.0, dataset: "MMLU Pro",        date: "2025-10-01" },
  ]

  return {
    modelId,
    foundryModelId: deployment,
    architecture: meta?.arch ?? `${deployment} hosted on Azure AI Foundry`,
    trainingDataCutoff: meta?.cutoff ?? "2024",
    license: "Microsoft Azure AI Terms of Service",
    intendedUse: meta?.intendedUse ?? [
      "Healthcare NLP and reasoning",
      "Clinical documentation processing",
      "RCM automation workflows",
      "Compliant data handling",
    ],
    outOfScope: [
      "Real-time patient diagnosis without physician oversight",
      "Emergency medical decision-making systems",
      "Direct patient-facing clinical decision support without review",
    ],
    evaluationMetrics,
    safetyMetrics,
    benchmarks: meta?.benchmarks ?? defaultBenchmarks,
    limitations: [
      "Evaluated primarily on English-language clinical documents",
      "Performance may vary on rare medical specialties",
      "Requires HIPAA-compliant data handling in customer environments",
    ],
    evaluatedAt: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
    evaluationRunId: `demo-${modelId}`,
    source: "mock",
  }
}

// ── Public: model card ────────────────────────────────────────────────────────

/**
 * Fetch (or synthesise) a ModelCardData for the given asset model ID.
 *
 * Resolution order — all via the same project endpoint:
 *  1. `AIProjectClient.deployments.get(name)` → enriches architecture/version
 *  2. REST `GET {endpoint}/evaluations/runs`   → live evaluation metrics
 *  3. Demo data                                → always-available fallback
 */
export async function getModelCard(modelId: string): Promise<ModelCardData> {
  const endpoint = process.env.AZURE_AI_PROJECT_ENDPOINT
  if (!endpoint) {
    console.warn("[foundry-client] AZURE_AI_PROJECT_ENDPOINT not set — using demo data")
    return getMockModelCard(modelId)
  }

  // ① deployments — primary control plane via AIProjectClient
  let deploymentInfo: { name: string; type: string } | null = null
  try {
    const deploymentName = MODEL_TO_DEPLOYMENT[modelId]
    if (deploymentName) {
      const d = await getDeployment(deploymentName)
      if (d) deploymentInfo = { name: d.name, type: d.type }
    }
  } catch (err) {
    console.warn("[foundry-client] deployment lookup failed:", err)
  }

  // ② evaluation runs — only attempted when AZURE_AI_HUB_WORKSPACE_URL is set
  //    (AI Services endpoint *.services.ai.azure.com does NOT support this API)
  let evalRun: EvalRun | null = null
  if (process.env.AZURE_AI_HUB_WORKSPACE_URL) {
    try {
      const runs = await fetchEvaluationRuns()
      evalRun = findLatestRun(runs, modelId) ?? null
    } catch (err) {
      console.warn("[foundry-client] evaluation runs fetch failed:", err)
    }
  }

  const mock = getMockModelCard(modelId)

  if (!deploymentInfo && !evalRun?.metrics) {
    console.info(`[foundry-client] no live Foundry data for ${modelId} — returning demo card`)
    return mock
  }

  return {
    ...mock,
    ...(evalRun?.metrics && {
      evaluationMetrics: mapEvaluationMetrics(evalRun.metrics),
      safetyMetrics:     mapSafetyMetrics(evalRun.metrics),
      evaluatedAt:       evalRun.createdDateTime ?? mock.evaluatedAt,
      evaluationRunId:   evalRun.id,
      source:            "foundry" as const,
    }),
  }
}
