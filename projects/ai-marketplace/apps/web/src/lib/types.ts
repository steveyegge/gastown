// ─── Asset types ────────────────────────────────────────────────────────────

export type AssetType =
  | "Agent"
  | "MCP Server"
  | "Model"
  | "Workflow Template"
  | "Evaluator"
  | "Connector";

export type DeploymentMode = "SaaS" | "PaaS";

export type ComplianceTier = "Standard" | "Healthcare" | "Financial" | "Government";

export interface AssetVersion {
  version: string;
  releasedAt: string;
  notes: string;
  isLatest: boolean;
}

export interface Publisher {
  id: string;
  name: string;
  verified: boolean;
  logoUrl?: string;
  contactEmail: string;
}

export interface AssetDependency {
  id: string;
  name: string;
  type: AssetType;
  version: string;
  required: boolean;
}

export interface EvaluationResult {
  id: string;
  name: string;
  score: number; // 0–100
  runAt: string;
  model: string;
  passed: boolean;
}

export interface Asset {
  id: string;
  name: string;
  type: AssetType;
  description: string;
  publisher: Publisher;
  latestVersion: string;
  versions: AssetVersion[];
  license: string;
  deploymentModes: DeploymentMode[];
  complianceTier: ComplianceTier;
  tags: string[];
  domains: string[];
  dependencies: AssetDependency[];
  evaluations: EvaluationResult[];
  rating: number;
  reviewCount: number;
  deploymentCount: number;
  createdAt: string;
  updatedAt: string;
  verified: boolean;
  riskNotes?: string;
}

export interface AssetListResponse {
  items: Asset[];
  total: number;
  page: number;
  pageSize: number;
}

// ─── Filter types ────────────────────────────────────────────────────────────

export interface AssetFilter {
  type: AssetType | "all";
  tags: string[];
  complianceTier: ComplianceTier | "all";
  deploymentMode: DeploymentMode | "all";
  search: string;
  page?: number;
  pageSize?: number;
}

// ─── Orchestration types ─────────────────────────────────────────────────────

export type WorkflowNodeType =
  | "agent"
  | "tool"
  | "model"
  | "knowledge"
  | "evaluator"
  | "guard"
  | "human"
  | "trigger"
  | "output";

export interface WorkflowNodeData {
  label: string;
  nodeType: WorkflowNodeType;
  assetId?: string;
  assetName?: string;
  config: Record<string, unknown>;
  status?: "idle" | "running" | "completed" | "error";
}

export interface WorkflowEdgeData {
  flowType: "message" | "control" | "data";
  label?: string;
}

export interface Workflow {
  id: string;
  name: string;
  description: string;
  createdAt: string;
  updatedAt: string;
  status: "draft" | "testing" | "deployed";
  nodes: unknown[];
  edges: unknown[];
}

// ─── Publisher / submission types ────────────────────────────────────────────

export type SubmissionStatus =
  | "draft"
  | "submitted"
  | "scanning"
  | "policy-review"
  | "human-review"
  | "approved"
  | "rejected"
  | "published";

export interface AssetSubmission {
  id: string;
  assetId?: string;
  assetName: string;
  publisherId: string;
  status: SubmissionStatus;
  submittedAt: string;
  reviewedAt?: string;
  reviewedBy?: string;
  rejectionReason?: string;
}
