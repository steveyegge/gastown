/**
 * Azure Machine Learning workspace integration.
 *
 * Provides model registry operations (list, get, register) against the
 * Azure ML Management REST API.
 *
 * Required environment variables:
 *   AZURE_SUBSCRIPTION_ID   – Azure subscription (e.g. a7fecb91-...)
 *   AZURE_ML_RESOURCE_GROUP – Resource group containing the ML workspace
 *   AZURE_ML_WORKSPACE      – AML workspace name
 *
 * Optional (service-principal auth; falls back to DefaultAzureCredential):
 *   AZURE_AD_CLIENT_ID, AZURE_AD_CLIENT_SECRET, AZURE_AD_TENANT_ID
 */

import { DefaultAzureCredential, ClientSecretCredential } from "@azure/identity"
import type { TokenCredential } from "@azure/core-auth"

const AML_API_VERSION = "2023-10-01"
const MANAGEMENT_SCOPE = "https://management.azure.com/.default"
const MANAGEMENT_BASE = "https://management.azure.com"

// ── Types ─────────────────────────────────────────────────────────────────────

export type ModelFramework =
  | "PyTorch"
  | "TensorFlow"
  | "ONNX"
  | "Scikit-learn"
  | "HuggingFace"
  | "Custom"
  | "Other"

export type ModelTaskType =
  | "NLP"
  | "TextClassification"
  | "TokenClassification"
  | "QuestionAnswering"
  | "Summarization"
  | "TextGeneration"
  | "ImageClassification"
  | "ObjectDetection"
  | "Regression"
  | "Classification"
  | "Forecasting"
  | "Other"

export interface AmlModelVersion {
  /** Unique internal ID returned by AML: {modelName}/{version} */
  id: string
  /** Model name */
  name: string
  /** Semantic version string */
  version: string
  /** Free-text description */
  description?: string
  /** Where the model artifact lives (blob URI, mlflow model path, etc.) */
  modelUri?: string
  /** Framework used to train the model */
  flavors?: Record<string, unknown>
  /** User-defined tags */
  tags?: Record<string, string>
  /** Properties bag from AML registration */
  properties?: Record<string, string>
  /** Provisioning state: Succeeded | Failed | Creating */
  provisioningState?: string
  /** Stage: Development | Staging | Production | Archived */
  stage?: string
  /** ISO date string */
  createdTime?: string
  /** ISO date string */
  lastModifiedTime?: string
}

export interface RegisterModelInput {
  name: string
  version: string
  description: string
  modelUri: string
  framework: ModelFramework
  taskType: ModelTaskType
  tags?: Record<string, string>
  /** e.g. HIPAA, SOC2 */
  compliance?: string[]
  /** Internal | Partner */
  type?: string
  /** Category for AI Asset Marketplace */
  category?: string
}

// ── Credential ────────────────────────────────────────────────────────────────

let _credential: TokenCredential | null = null

function getCredential(): TokenCredential {
  if (_credential) return _credential
  const clientId     = process.env.AZURE_AD_CLIENT_ID
  const clientSecret = process.env.AZURE_AD_CLIENT_SECRET
  const tenantId     = process.env.AZURE_AD_TENANT_ID
  _credential =
    clientId && clientSecret && tenantId
      ? new ClientSecretCredential(tenantId, clientId, clientSecret)
      : new DefaultAzureCredential()
  return _credential
}

async function getToken(): Promise<string> {
  const token = await getCredential().getToken(MANAGEMENT_SCOPE)
  if (!token) throw new Error("Failed to acquire Azure management token")
  return token.token
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function amlBase(): string {
  const sub = process.env.AZURE_SUBSCRIPTION_ID
  const rg  = process.env.AZURE_ML_RESOURCE_GROUP
  const ws  = process.env.AZURE_ML_WORKSPACE
  if (!sub || !rg || !ws) {
    throw new Error(
      "Azure ML not configured. Set AZURE_SUBSCRIPTION_ID, AZURE_ML_RESOURCE_GROUP, AZURE_ML_WORKSPACE."
    )
  }
  return `${MANAGEMENT_BASE}/subscriptions/${sub}/resourceGroups/${rg}/providers/Microsoft.MachineLearningServices/workspaces/${ws}`
}

async function amlFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = await getToken()
  const url = `${amlBase()}${path}?api-version=${AML_API_VERSION}`
  const res = await fetch(url, {
    ...options,
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      ...(options.headers ?? {}),
    },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`AML API ${res.status} ${res.statusText}: ${body}`)
  }
  return res.json() as Promise<T>
}

// ── List all model names ──────────────────────────────────────────────────────

interface AmlModelListItem {
  name: string
  id: string
  type: string
}

/**
 * Returns the list of registered model names / containers from the AML workspace.
 */
export async function listAmlModels(): Promise<AmlModelListItem[]> {
  const result = await amlFetch<{ value: AmlModelListItem[] }>("/models")
  return result.value ?? []
}

// ── List versions of a specific model ────────────────────────────────────────

/**
 * Returns all versions of a given model container.
 */
export async function listAmlModelVersions(modelName: string): Promise<AmlModelVersion[]> {
  interface VersionResponse {
    value: Array<{
      name: string
      id: string
      properties: {
        description?: string
        modelUri?: string
        flavors?: Record<string, unknown>
        tags?: Record<string, string>
        properties?: Record<string, string>
        provisioningState?: string
        stage?: string
      }
      systemData?: {
        createdAt?: string
        lastModifiedAt?: string
      }
    }>
  }

  const result = await amlFetch<VersionResponse>(`/models/${encodeURIComponent(modelName)}/versions`)

  return (result.value ?? []).map((v) => ({
    id: `${modelName}/${v.name}`,
    name: modelName,
    version: v.name,
    description: v.properties?.description,
    modelUri: v.properties?.modelUri,
    flavors: v.properties?.flavors,
    tags: v.properties?.tags,
    properties: v.properties?.properties,
    provisioningState: v.properties?.provisioningState,
    stage: v.properties?.stage,
    createdTime: v.systemData?.createdAt,
    lastModifiedTime: v.systemData?.lastModifiedAt,
  }))
}

// ── Get a single model version ────────────────────────────────────────────────

export async function getAmlModelVersion(
  modelName: string,
  version: string
): Promise<AmlModelVersion | null> {
  try {
    interface VersionDetail {
      name: string
      id: string
      properties: {
        description?: string
        modelUri?: string
        flavors?: Record<string, unknown>
        tags?: Record<string, string>
        properties?: Record<string, string>
        provisioningState?: string
        stage?: string
      }
      systemData?: { createdAt?: string; lastModifiedAt?: string }
    }

    const v = await amlFetch<VersionDetail>(
      `/models/${encodeURIComponent(modelName)}/versions/${encodeURIComponent(version)}`
    )
    return {
      id: `${modelName}/${v.name}`,
      name: modelName,
      version: v.name,
      description: v.properties?.description,
      modelUri: v.properties?.modelUri,
      flavors: v.properties?.flavors,
      tags: v.properties?.tags,
      properties: v.properties?.properties,
      provisioningState: v.properties?.provisioningState,
      stage: v.properties?.stage,
      createdTime: v.systemData?.createdAt,
      lastModifiedTime: v.systemData?.lastModifiedAt,
    }
  } catch {
    return null
  }
}

// ── Register a model ──────────────────────────────────────────────────────────

/**
 * Register (create/update) a model version in the Azure ML workspace.
 *
 * This creates/updates the model container (name) and the specific version.
 * On success, returns the registered AmlModelVersion.
 */
export async function registerAmlModel(input: RegisterModelInput): Promise<AmlModelVersion> {
  const { name, version, description, modelUri, framework, taskType, tags = {}, compliance = [], type = "custom", category } = input

  // Build tags bag — store marketplace metadata in AML tags
  const allTags: Record<string, string> = {
    ...tags,
    "marketplace.framework": framework,
    "marketplace.taskType": taskType,
    "marketplace.type": type,
    "marketplace.registeredBy": "ai-asset-marketplace",
    ...(category ? { "marketplace.category": category } : {}),
    ...(compliance.length ? { "marketplace.compliance": compliance.join(",") } : {}),
  }

  // AML requires model container to exist first
  await amlFetch(`/models/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: JSON.stringify({
      properties: {
        description,
        tags: allTags,
      },
    }),
  })

  // Create version
  const body = {
    properties: {
      description,
      modelUri,
      tags: allTags,
      flavors: {
        [framework.toLowerCase()]: {},
      },
      properties: {
        taskType,
        stage: "Development",
      } as Record<string, string>,
    },
  }

  interface VersionDetail {
    name: string
    id: string
    properties: {
      description?: string
      modelUri?: string
      flavors?: Record<string, unknown>
      tags?: Record<string, string>
      properties?: Record<string, string>
      provisioningState?: string
      stage?: string
    }
    systemData?: { createdAt?: string; lastModifiedAt?: string }
  }

  const v = await amlFetch<VersionDetail>(
    `/models/${encodeURIComponent(name)}/versions/${encodeURIComponent(version)}`,
    { method: "PUT", body: JSON.stringify(body) }
  )

  return {
    id: `${name}/${v.name}`,
    name,
    version: v.name,
    description: v.properties?.description,
    modelUri: v.properties?.modelUri,
    flavors: v.properties?.flavors,
    tags: v.properties?.tags,
    properties: v.properties?.properties,
    provisioningState: v.properties?.provisioningState,
    stage: v.properties?.stage,
    createdTime: v.systemData?.createdAt,
    lastModifiedTime: v.systemData?.lastModifiedAt,
  }
}

// ── Sync helper — convert AML model versions to marketplace ModelData ─────────

import type { ModelData } from "./models-data"

/**
 * Convert a raw AML model version into the AI Asset Marketplace ModelData shape.
 * Used when syncing from AML into the marketplace catalog.
 */
export function amlModelToMarketplace(v: AmlModelVersion): ModelData {
  const tags = v.tags ?? {}
  const props = v.properties ?? {}

  return {
    id: `aml-${v.name}-${v.version}`.replace(/[^a-z0-9-]/gi, "-").toLowerCase(),
    name: v.name,
    version: `v${v.version}`,
    publisher: tags["marketplace.registeredBy"] ?? "Azure ML Workspace",
    publisherVerified: false,
    description: v.description ?? `${v.name} version ${v.version} registered in Azure ML`,
    category: (tags["marketplace.taskType"] ?? props["taskType"] ?? "Custom") as string,
    type: (tags["marketplace.type"] ?? "custom") as string,
    status: v.stage === "Production" ? "production" : v.stage === "Staging" ? "beta" : "review",
    rating: 0,
    downloads: 0,
    lastUpdated: v.lastModifiedTime?.slice(0, 10) ?? new Date().toISOString().slice(0, 10),
    compliance: (tags["marketplace.compliance"] ?? "").split(",").filter(Boolean),
    metrics: { accuracy: 0, latency: 0, throughput: 0 },
    teams: 0,
    tags: Object.entries(tags)
      .filter(([k]) => !k.startsWith("marketplace."))
      .map(([k, v]) => `${k}:${v}`),
    endpoint: v.modelUri,
    source: "azure-ml",
  } as ModelData & { source: string }
}

// ── Check if AML is configured ────────────────────────────────────────────────

export function isAmlConfigured(): boolean {
  return !!(
    process.env.AZURE_SUBSCRIPTION_ID &&
    process.env.AZURE_ML_RESOURCE_GROUP &&
    process.env.AZURE_ML_WORKSPACE
  )
}

// ── Compute management ────────────────────────────────────────────────────────
// Azure ML Compute Instances and Compute Clusters (AmlCompute)

export type ComputeInstanceState =
  | "Running"
  | "Stopped"
  | "Starting"
  | "Stopping"
  | "Restarting"
  | "Creating"
  | "Updating"
  | "Deleting"
  | "Unknown"
  | "UserSettingUp"
  | "SystemSettingUp"
  | "JobRunning"

export type ComputeType = "ComputeInstance" | "AmlCompute" | "Kubernetes" | "VirtualMachine"

export interface AmlComputeInstance {
  /** ARM resource name */
  name: string
  /** ARM resource ID */
  id: string
  /** Compute type */
  computeType: ComputeType
  /** Provisioning state from ARM */
  provisioningState: string
  /** VM size (e.g. Standard_NC24ads_A100_v4) */
  vmSize: string
  /** Current operational state for ComputeInstance */
  state?: ComputeInstanceState
  /** Human-readable description */
  description?: string
  /** Owner (principalId or UPN) */
  createdBy?: string
  /** ISO datetime */
  createdOn?: string
  /** ISO datetime */
  modifiedOn?: string
  /** Number of CPU cores from VM size (derived) */
  cpuCores?: number
  /** Memory in GB (derived) */
  memoryGb?: number
  /** For AmlCompute: current / max node counts */
  currentNodeCount?: number
  maxNodeCount?: number
  /** Whether GPU VM */
  isGpu?: boolean
  /** GPU spec string if applicable */
  gpuSpec?: string
  /** Subnet resource-id if VNet integrated */
  subnetId?: string
  /** Tags map */
  tags?: Record<string, string>
  /** SSH access info */
  sshPort?: number
  /** AML Studio URL */
  studioUrl?: string
}

interface AmlComputeListResponse {
  value: Array<{
    id: string
    name: string
    tags?: Record<string, string>
    properties: {
      computeType: string
      computeLocation?: string
      description?: string
      provisioningState?: string
      resourceId?: string
      createdOn?: string
      modifiedOn?: string
      createdBy?: { userObjectId?: string; userName?: string }
      properties?: {
        // ComputeInstance specific
        vmSize?: string
        state?: string
        sshSettings?: { sshPort?: number }
        subnet?: { id?: string }
        // AmlCompute specific
        currentNodeCount?: number
        targetNodeCount?: number
        scaleSettings?: { maxNodeCount?: number; minNodeCount?: number }
        vmPriority?: string
        // Kubernetes
        clusterPurpose?: string
      }
    }
  }>
  nextLink?: string
}

/** Azure VM size to compute shape (best-effort approximation for common sizes) */
function vmSizeToSpecs(vmSize: string): { cpu: number; memGb: number; isGpu: boolean; gpuSpec?: string } {
  const s = vmSize.toLowerCase()
  // GPU families
  if (s.includes("_a100")) return { cpu: 24, memGb: 220, isGpu: true, gpuSpec: "NVIDIA A100 × 1" }
  if (s.includes("nd96")) return { cpu: 96, memGb: 900, isGpu: true, gpuSpec: "NVIDIA A100 × 8" }
  if (s.includes("nc24ads")) return { cpu: 24, memGb: 220, isGpu: true, gpuSpec: "NVIDIA A100 × 1" }
  if (s.includes("nc48ads")) return { cpu: 48, memGb: 440, isGpu: true, gpuSpec: "NVIDIA A100 × 2" }
  if (s.includes("nc6") && !s.includes("nc6s")) return { cpu: 6, memGb: 56, isGpu: true, gpuSpec: "NVIDIA Tesla K80 × 1" }
  if (s.includes("nc6s_v3")) return { cpu: 6, memGb: 112, isGpu: true, gpuSpec: "NVIDIA V100 × 1" }
  if (s.includes("nc12s_v3")) return { cpu: 12, memGb: 224, isGpu: true, gpuSpec: "NVIDIA V100 × 2" }
  if (s.includes("nc24s_v3")) return { cpu: 24, memGb: 448, isGpu: true, gpuSpec: "NVIDIA V100 × 4" }
  if (s.includes("nc6s_v2")) return { cpu: 6, memGb: 112, isGpu: true, gpuSpec: "NVIDIA P100 × 1" }
  if (s.includes("nc12s_v2")) return { cpu: 12, memGb: 224, isGpu: true, gpuSpec: "NVIDIA P100 × 2" }
  if (s.includes("nv") ) return { cpu: 12, memGb: 112, isGpu: true, gpuSpec: "NVIDIA M60 × 2" }
  // CPU families
  if (s.includes("_d2_") || s.includes("d2s")) return { cpu: 2, memGb: 8, isGpu: false }
  if (s.includes("_d4_") || s.includes("d4s")) return { cpu: 4, memGb: 16, isGpu: false }
  if (s.includes("_d8_") || s.includes("d8s")) return { cpu: 8, memGb: 32, isGpu: false }
  if (s.includes("_d16_") || s.includes("d16s")) return { cpu: 16, memGb: 64, isGpu: false }
  if (s.includes("_d32_") || s.includes("d32s")) return { cpu: 32, memGb: 128, isGpu: false }
  if (s.includes("_d64_") || s.includes("d64s")) return { cpu: 64, memGb: 256, isGpu: false }
  if (s.includes("_e4_") || s.includes("e4s")) return { cpu: 4, memGb: 32, isGpu: false }
  if (s.includes("_e8_") || s.includes("e8s")) return { cpu: 8, memGb: 64, isGpu: false }
  if (s.includes("_e16_") || s.includes("e16s")) return { cpu: 16, memGb: 128, isGpu: false }
  if (s.includes("_e32_") || s.includes("e32s")) return { cpu: 32, memGb: 256, isGpu: false }
  if (s.includes("ds3")) return { cpu: 4, memGb: 14, isGpu: false }
  if (s.includes("ds4")) return { cpu: 8, memGb: 28, isGpu: false }
  if (s.includes("ds5")) return { cpu: 16, memGb: 56, isGpu: false }
  // Fallback
  return { cpu: 4, memGb: 16, isGpu: false }
}

function mapComputeInstance(raw: AmlComputeListResponse["value"][number]): AmlComputeInstance {
  const props = raw.properties
  const inner = props?.properties ?? {}
  const vmSize = inner.vmSize ?? "Standard_DS3_v2"
  const specs = vmSizeToSpecs(vmSize)
  const sub = process.env.AZURE_SUBSCRIPTION_ID ?? ""
  const rg = process.env.AZURE_ML_RESOURCE_GROUP ?? ""
  const ws = process.env.AZURE_ML_WORKSPACE ?? ""

  const studioUrl =
    raw.name && ws
      ? `https://ml.azure.com/compute/${raw.name}/detail?wsid=/subscriptions/${sub}/resourceGroups/${rg}/providers/Microsoft.MachineLearningServices/workspaces/${ws}`
      : undefined

  return {
    name: raw.name,
    id: raw.id,
    computeType: (props?.computeType ?? "ComputeInstance") as ComputeType,
    provisioningState: props?.provisioningState ?? "Unknown",
    vmSize,
    state: (inner.state ?? (props?.provisioningState === "Succeeded" ? "Running" : "Unknown")) as ComputeInstanceState,
    description: props?.description,
    createdBy: props?.createdBy?.userName,
    createdOn: props?.createdOn,
    modifiedOn: props?.modifiedOn,
    cpuCores: specs.cpu,
    memoryGb: specs.memGb,
    currentNodeCount: inner.currentNodeCount,
    maxNodeCount: inner.scaleSettings?.maxNodeCount,
    isGpu: specs.isGpu,
    gpuSpec: specs.gpuSpec,
    subnetId: inner.subnet?.id,
    tags: raw.tags,
    sshPort: inner.sshSettings?.sshPort,
    studioUrl,
  }
}

/**
 * List all compute resources in the Azure ML workspace.
 */
export async function listAmlCompute(): Promise<AmlComputeInstance[]> {
  const result = await amlFetch<AmlComputeListResponse>("/computes")
  return (result.value ?? []).map(mapComputeInstance)
}

/**
 * Get a single compute resource by name.
 */
export async function getAmlCompute(computeName: string): Promise<AmlComputeInstance> {
  const raw = await amlFetch<AmlComputeListResponse["value"][number]>(
    `/computes/${encodeURIComponent(computeName)}`
  )
  return mapComputeInstance(raw)
}

export type ComputeAction = "start" | "stop" | "restart"

/**
 * Start, stop, or restart a ComputeInstance.
 * (AmlCompute clusters scale automatically; start/stop applies to ComputeInstance only.)
 */
export async function controlAmlCompute(
  computeName: string,
  action: ComputeAction
): Promise<{ accepted: boolean }> {
  await amlFetch(`/computes/${encodeURIComponent(computeName)}/${action}`, {
    method: "POST",
    body: JSON.stringify({}),
  })
  return { accepted: true }
}

export interface ProvisionComputeInput {
  /** Name for the new compute instance */
  name: string
  /** VM size, e.g. Standard_DS3_v2 */
  vmSize: string
  /** Optional description */
  description?: string
  /** Whether to enable SSH */
  enableSsh?: boolean
  /** Tags */
  tags?: Record<string, string>
}

/**
 * Provision a new ComputeInstance in the Azure ML workspace.
 * Returns the ARM operation location header (async provision).
 */
export async function provisionComputeInstance(
  input: ProvisionComputeInput
): Promise<{ name: string; provisioningState: string }> {
  interface CreateResponse { name: string; properties: { provisioningState?: string } }
  const { name, vmSize, description, enableSsh = false, tags = {} } = input
  const body = {
    properties: {
      computeType: "ComputeInstance",
      description,
      properties: {
        vmSize,
        applicationSharingPolicy: "Personal",
        sshSettings: {
          sshPublicAccess: enableSsh ? "Enabled" : "Disabled",
        },
      },
    },
    tags: {
      "created-by": "imde-marketplace",
      ...tags,
    },
  }
  const result = await amlFetch<CreateResponse>(
    `/computes/${encodeURIComponent(name)}`,
    { method: "PUT", body: JSON.stringify(body) }
  )
  return {
    name: result.name,
    provisioningState: result.properties?.provisioningState ?? "Creating",
  }
}

/**
 * Delete a compute resource.
 */
export async function deleteAmlCompute(computeName: string): Promise<void> {
  await amlFetch(`/computes/${encodeURIComponent(computeName)}?underlyingResourceAction=Delete`, {
    method: "DELETE",
  })
}

/** Standard VM sizes available for new compute instances */
export const COMPUTE_VM_SIZES = [
  { id: "Standard_DS3_v2",      label: "DS3 v2 — 4 vCPU / 14 GB",    tier: "CPU",  isGpu: false },
  { id: "Standard_DS4_v2",      label: "DS4 v2 — 8 vCPU / 28 GB",    tier: "CPU",  isGpu: false },
  { id: "Standard_D16s_v3",     label: "D16s v3 — 16 vCPU / 64 GB",  tier: "CPU",  isGpu: false },
  { id: "Standard_E8s_v3",      label: "E8s v3 — 8 vCPU / 64 GB",    tier: "Memory", isGpu: false },
  { id: "Standard_E16s_v3",     label: "E16s v3 — 16 vCPU / 128 GB", tier: "Memory", isGpu: false },
  { id: "Standard_NC6s_v3",     label: "NC6s v3 — 6 vCPU / V100 × 1", tier: "GPU", isGpu: true },
  { id: "Standard_NC12s_v3",    label: "NC12s v3 — 12 vCPU / V100 × 2", tier: "GPU", isGpu: true },
  { id: "Standard_NC24s_v3",    label: "NC24s v3 — 24 vCPU / V100 × 4", tier: "GPU", isGpu: true },
  { id: "Standard_NC24ads_A100_v4", label: "NC24ads A100 v4 — 24 vCPU / A100 × 1", tier: "GPU", isGpu: true },
]
