export interface ModelMetrics {
  accuracy: number
  latency: number
  throughput: number
}

export interface ModelData {
  id: string
  name: string
  version: string
  publisher: string
  publisherVerified: boolean
  description: string
  category: string
  type: string
  status: string
  rating: number
  downloads: number
  lastUpdated: string
  compliance: string[]
  metrics: ModelMetrics
  teams: number
  tags?: string[]
  useCases?: string[]
  endpoint?: string
}

export const models: ModelData[] = [
  {
    id: "model-1",
    name: "ClinicalBERT-RCM",
    version: "v2.1.0",
    publisher: "Azure Healthcare AI",
    publisherVerified: true,
    description:
      "Healthcare-specialized language model fine-tuned on clinical notes and RCM documentation for superior healthcare NLP tasks. Achieves state-of-the-art accuracy on ICD coding, clinical entity extraction, and medical text summarization.",
    category: "NLP",
    type: "internal",
    status: "production",
    rating: 4.9,
    downloads: 12500,
    lastUpdated: "2026-02-15",
    compliance: ["HIPAA", "SOC2"],
    metrics: { accuracy: 94.2, latency: 45, throughput: 1200 },
    teams: 24,
    tags: ["nlp", "bert", "clinical", "healthcare", "ner"],
    useCases: [
      "Clinical entity extraction from physician notes",
      "ICD-10 code suggestion",
      "Medical document summarization",
      "Prior authorization text analysis",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/clinicalbert-rcm/v2",
  },
  {
    id: "model-2",
    name: "Claims-Predictor-XL",
    version: "v3.0.1",
    publisher: "RCM Analytics Team",
    publisherVerified: true,
    description:
      "Predicts claim denial probability and recommends corrective actions before submission. Trained on 10M+ claims with payer-specific rule sets. Integrates directly into pre-submission validation workflows.",
    category: "Prediction",
    type: "internal",
    status: "production",
    rating: 4.8,
    downloads: 8900,
    lastUpdated: "2026-02-20",
    compliance: ["HIPAA"],
    metrics: { accuracy: 91.5, latency: 120, throughput: 500 },
    teams: 18,
    tags: ["prediction", "claims", "denial", "xgboost", "rcm"],
    useCases: [
      "Pre-submission denial risk scoring",
      "Payer-specific rule validation",
      "Appeal recommendation engine",
      "Revenue leakage prevention",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/claims-predictor-xl/v3",
  },
  {
    id: "model-3",
    name: "DocVision-Medical",
    version: "v4.2.0",
    publisher: "Azure AI",
    publisherVerified: true,
    description:
      "Computer vision model for medical document extraction including EOBs, claims forms, and clinical records. Supports structured extraction of key fields with high confidence scoring and audit trails.",
    category: "Vision",
    type: "partner",
    status: "production",
    rating: 4.7,
    downloads: 15200,
    lastUpdated: "2026-02-18",
    compliance: ["HIPAA", "SOC2", "ISO27001"],
    metrics: { accuracy: 96.8, latency: 200, throughput: 300 },
    teams: 31,
    tags: ["ocr", "vision", "documents", "eob", "forms", "azure-ai"],
    useCases: [
      "EOB / ERA automated extraction",
      "Claims form digitization",
      "Clinical record ingestion",
      "Insurance card reading",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/docvision-medical/v4",
  },
  {
    id: "model-4",
    name: "ICD-Coder-Pro",
    version: "v1.8.2",
    publisher: "CodeRight AI",
    publisherVerified: true,
    description:
      "Automated ICD-10 and CPT code suggestion from clinical documentation with compliance validation. Reduces coding time by 60% and improves first-pass acceptance rates with real-time payer rule feedback.",
    category: "NLP",
    type: "partner",
    status: "production",
    rating: 4.6,
    downloads: 6700,
    lastUpdated: "2026-02-12",
    compliance: ["HIPAA"],
    metrics: { accuracy: 89.3, latency: 80, throughput: 800 },
    teams: 15,
    tags: ["icd-10", "cpt", "coding", "nlp", "compliance"],
    useCases: [
      "Automated ICD-10 code suggestion",
      "CPT code assignment from notes",
      "Code validation against payer rules",
      "Coding audit trail generation",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/icd-coder-pro/v1",
  },
  {
    id: "model-5",
    name: "Payment-Optimizer",
    version: "v2.0.0-beta",
    publisher: "Revenue AI Lab",
    publisherVerified: false,
    description:
      "Optimizes payment posting and identifies underpayment patterns using historical ERA/EOB data analysis. Beta version with early access for select teams pending production certification.",
    category: "Analytics",
    type: "internal",
    status: "beta",
    rating: 4.4,
    downloads: 2100,
    lastUpdated: "2026-02-22",
    compliance: ["HIPAA"],
    metrics: { accuracy: 87.1, latency: 150, throughput: 400 },
    teams: 6,
    tags: ["payment", "analytics", "era", "eob", "underpayment", "beta"],
    useCases: [
      "Underpayment detection",
      "Payment variance analysis",
      "ERA/EOB reconciliation",
      "Revenue recovery scoring",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/payment-optimizer/v2-beta",
  },
  {
    id: "model-6",
    name: "PatientMatch-NER",
    version: "v1.2.0",
    publisher: "Data Science Team",
    publisherVerified: true,
    description:
      "Named entity recognition for patient matching and record linkage across disparate healthcare systems. Uses probabilistic matching with configurable confidence thresholds and full audit trails.",
    category: "NLP",
    type: "internal",
    status: "review",
    rating: 4.3,
    downloads: 890,
    lastUpdated: "2026-02-25",
    compliance: ["HIPAA", "SOC2"],
    metrics: { accuracy: 92.4, latency: 35, throughput: 1500 },
    teams: 4,
    tags: ["ner", "patient-matching", "record-linkage", "nlp", "identity"],
    useCases: [
      "Cross-system patient identity matching",
      "Duplicate record detection",
      "Healthcare record linkage",
      "Patient demographic extraction",
    ],
    endpoint: "https://aimarket-hub.azure.com/endpoints/patientmatch-ner/v1",
  },
]
