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
