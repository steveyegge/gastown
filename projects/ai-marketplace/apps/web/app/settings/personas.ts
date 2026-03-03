// Persona catalog shared across all settings sub-pages.
// Three tiers: First Party (Optum), Second Party (Provider), Third Party (Ecosystem Partners)

export type PersonaId =
  // First party
  | "data-engineer"
  | "product-manager"
  | "governance"
  | "security-engineer"
  | "devops-sre"
  // Second party (Provider)
  | "provider-admin"
  | "clinical-expert"
  | "domain-expert"
  | "provider-data-scientist"
  | "bi-analyst"
  | "it-security"
  | "operational-admin"
  | "review-board"
  // Third party (Ecosystem Partners)
  | "startup-science"
  | "ml-engineer"
  | "vendor-engineer"
  | "isv"
  | "external-auditor"

export type SettingCategory = "infrastructure" | "performance" | "reliability"

export interface Persona {
  id: PersonaId
  label: string
  shortLabel?: string
  desc: string
  icon: string   // lucide icon name, resolved in UI
  tier: 1 | 2 | 3
}

export interface PersonaTier {
  tier: 1 | 2 | 3
  label: string
  shortLabel: string
  color: string          // tailwind color token prefix, e.g. "orange"
  borderColor: string
  bgColor: string
  textColor: string
  badgeColor: string
  personas: Persona[]
}

export const PERSONA_TIERS: PersonaTier[] = [
  {
    tier: 1,
    label: "First Party — Optum",
    shortLabel: "Optum",
    color: "orange",
    borderColor: "border-[var(--optum-orange)]/40",
    bgColor: "bg-[var(--optum-orange)]/10",
    textColor: "text-[var(--optum-orange)]",
    badgeColor: "bg-[var(--optum-orange)]/20 text-[var(--optum-orange)]",
    personas: [
      {
        id: "data-engineer",
        label: "Data Engineer",
        desc: "Handles data ingestion, feature stores, and transformations.",
        icon: "Database",
        tier: 1,
      },
      {
        id: "product-manager",
        label: "Product Manager",
        desc: "Defines platform roadmap, requirements, strategy.",
        icon: "Briefcase",
        tier: 1,
      },
      {
        id: "governance",
        label: "Governance / Compliance",
        shortLabel: "Governance",
        desc: "Oversees responsible AI, approvals, and audit workflows.",
        icon: "Scale",
        tier: 1,
      },
      {
        id: "security-engineer",
        label: "Security Engineer",
        desc: "Ensures access controls, safety, isolation, secure integration.",
        icon: "ShieldCheck",
        tier: 1,
      },
      {
        id: "devops-sre",
        label: "DevOps / SRE",
        desc: "Automates deployments, CI/CD pipelines, and monitoring.",
        icon: "GitBranch",
        tier: 1,
      },
    ],
  },
  {
    tier: 2,
    label: "Second Party — Provider",
    shortLabel: "Provider",
    color: "blue",
    borderColor: "border-blue-500/40",
    bgColor: "bg-blue-500/10",
    textColor: "text-blue-400",
    badgeColor: "bg-blue-500/20 text-blue-400",
    personas: [
      {
        id: "provider-admin",
        label: "Provider Admin",
        desc: "Manages provider organization, user roles, platform configuration.",
        icon: "UserCog",
        tier: 2,
      },
      {
        id: "clinical-expert",
        label: "Clinical Expert",
        desc: "Reviews model outputs for clinical relevance and safety.",
        icon: "Stethoscope",
        tier: 2,
      },
      {
        id: "domain-expert",
        label: "Domain Expert",
        desc: "Validates models from a subject-matter perspective.",
        icon: "BookOpen",
        tier: 2,
      },
      {
        id: "provider-data-scientist",
        label: "Provider Data Scientist",
        shortLabel: "Data Scientist",
        desc: "Fine-tunes models with provider data; analyzes insights.",
        icon: "FlaskConical",
        tier: 2,
      },
      {
        id: "bi-analyst",
        label: "Data / BI Analyst",
        desc: "Runs analytics, reporting, and dashboards.",
        icon: "BarChart3",
        tier: 2,
      },
      {
        id: "it-security",
        label: "IT / Security Staff",
        desc: "Handles integration, identity, compliance with provider systems.",
        icon: "ServerCog",
        tier: 2,
      },
      {
        id: "operational-admin",
        label: "Operational Admin Staff",
        shortLabel: "Ops Admin",
        desc: "Uses workflow outputs and tools in daily operations.",
        icon: "ClipboardList",
        tier: 2,
      },
      {
        id: "review-board",
        label: "Review Board / Ethics",
        shortLabel: "Ethics Board",
        desc: "Approves model usage, ensures clinical and ethical compliance.",
        icon: "Gavel",
        tier: 2,
      },
    ],
  },
  {
    tier: 3,
    label: "Third Party — Ecosystem Partners",
    shortLabel: "Partners",
    color: "violet",
    borderColor: "border-violet-500/40",
    bgColor: "bg-violet-500/10",
    textColor: "text-violet-400",
    badgeColor: "bg-violet-500/20 text-violet-400",
    personas: [
      {
        id: "startup-science",
        label: "Startup Science Team",
        desc: "Brings their own models; collaborates on experiments.",
        icon: "Rocket",
        tier: 3,
      },
      {
        id: "ml-engineer",
        label: "Third-Party ML Engineer",
        shortLabel: "ML Engineer",
        desc: "Integrates models with platform APIs and frameworks.",
        icon: "Code2",
        tier: 3,
      },
      {
        id: "vendor-engineer",
        label: "Vendor Engineer",
        desc: "Builds connectors, plugins, or extensions.",
        icon: "Package",
        tier: 3,
      },
      {
        id: "isv",
        label: "ISVs",
        desc: "Independent companies offering software or models on the platform.",
        icon: "Building2",
        tier: 3,
      },
      {
        id: "external-auditor",
        label: "External Auditor / Evaluator",
        shortLabel: "Auditor",
        desc: "Provides external validation, safety, and compliance audits.",
        icon: "ClipboardCheck",
        tier: 3,
      },
    ],
  },
]

// Flat map for quick lookup
export const ALL_PERSONAS: Persona[] = PERSONA_TIERS.flatMap((t) => t.personas)

export function getPersona(id: PersonaId): Persona | undefined {
  return ALL_PERSONAS.find((p) => p.id === id)
}

export function getTierForPersona(id: PersonaId): PersonaTier | undefined {
  return PERSONA_TIERS.find((t) => t.personas.some((p) => p.id === id))
}

// ── Settings config per persona × category ───────────────────────────────────

export interface SettingField {
  key: string
  label: string
  description: string
  type: "text" | "number" | "toggle" | "select" | "multi-select" | "readonly"
  defaultValue: string | number | boolean | string[]
  options?: string[]
  unit?: string
  group?: string
}

export interface PersonaSettings {
  personaId: PersonaId
  category: SettingCategory
  title: string
  description: string
  fields: SettingField[]
}

export const SETTINGS_CATALOG: PersonaSettings[] = [
  // ── INFRASTRUCTURE ──────────────────────────────────────────────────────────
  {
    personaId: "data-engineer",
    category: "infrastructure",
    title: "Data Infrastructure",
    description: "Configure storage backends, feature stores, and pipeline execution environments.",
    fields: [
      { key: "feature_store_backend", label: "Feature Store Backend", description: "Primary backend for feature storage", type: "select", defaultValue: "Azure Redis Cache", options: ["Azure Redis Cache", "Azure SQL", "Cosmos DB", "Azure Table Storage"], group: "Feature Store" },
      { key: "pipeline_storage_account", label: "Pipeline Storage Account", description: "Azure Storage account for pipeline artifacts", type: "text", defaultValue: "aimarket-pipeline-eastus", group: "Storage" },
      { key: "transformation_compute", label: "Transformation Compute", description: "Spark or Databricks cluster for large-scale transforms", type: "select", defaultValue: "Azure Databricks (Standard_DS3_v2)", options: ["Azure Databricks (Standard_DS3_v2)", "Azure Synapse Spark", "Azure HDInsight", "Local"], group: "Compute" },
      { key: "delta_lake_enabled", label: "Delta Lake Format", description: "Use Delta Lake format for ACID transactions on data lakes", type: "toggle", defaultValue: true, group: "Storage" },
      { key: "ingestion_parallelism", label: "Max Ingestion Workers", description: "Maximum parallel workers for data ingestion jobs", type: "number", defaultValue: 16, unit: "workers", group: "Compute" },
      { key: "data_catalog", label: "Data Catalog Endpoint", description: "Azure Purview or Unity Catalog API endpoint", type: "text", defaultValue: "https://aimarket-purview.purview.azure.com", group: "Catalog" },
    ],
  },
  {
    personaId: "devops-sre",
    category: "infrastructure",
    title: "DevOps Infrastructure",
    description: "Manage CI/CD pipelines, container environments, and deployment targets.",
    fields: [
      { key: "acr_endpoint", label: "Container Registry", description: "Azure Container Registry login server", type: "text", defaultValue: "aimktacrp7a65r22.azurecr.io", group: "Containers" },
      { key: "k8s_cluster", label: "AKS Cluster", description: "Primary Kubernetes cluster for workload deployments", type: "text", defaultValue: "aimarket-aks-dev", group: "Kubernetes" },
      { key: "cicd_platform", label: "CI/CD Platform", description: "Pipeline orchestration system in use", type: "select", defaultValue: "Azure DevOps", options: ["Azure DevOps", "GitHub Actions", "Jenkins", "CircleCI"], group: "CI/CD" },
      { key: "artifact_feed", label: "Artifact Feed URL", description: "Azure Artifacts or npm/PyPI feed for packages", type: "text", defaultValue: "https://pkgs.dev.azure.com/optum/aimarket/_packaging/ai-assets/npm/registry/", group: "CI/CD" },
      { key: "infra_as_code", label: "IaC Framework", description: "Infrastructure provisioning toolchain", type: "select", defaultValue: "Bicep", options: ["Bicep", "Terraform", "ARM Templates", "Pulumi"], group: "IaC" },
      { key: "gitops_enabled", label: "GitOps Mode", description: "Enable GitOps-driven reconciliation via Flux or ArgoCD", type: "toggle", defaultValue: true, group: "Kubernetes" },
    ],
  },
  {
    personaId: "security-engineer",
    category: "infrastructure",
    title: "Security Infrastructure",
    description: "Define network policies, private endpoints, identity federation, and vault configuration.",
    fields: [
      { key: "keyvault_name", label: "Key Vault", description: "Primary Key Vault for secrets and certs", type: "text", defaultValue: "aimkt-kv-p7a65r22", group: "Secrets" },
      { key: "private_endpoint_enabled", label: "Private Endpoints", description: "Force all service traffic through private endpoints", type: "toggle", defaultValue: true, group: "Network" },
      { key: "vnet_name", label: "VNet Name", description: "Virtual Network for service isolation", type: "text", defaultValue: "aimarket-vnet-dev", group: "Network" },
      { key: "identity_provider", label: "Identity Provider", description: "Primary IdP for platform authentication", type: "select", defaultValue: "Microsoft Entra ID", options: ["Microsoft Entra ID", "Okta", "Ping Identity"], group: "Identity" },
      { key: "managed_identity_principal", label: "Managed Identity Principal", description: "System-assigned managed identity client ID", type: "readonly", defaultValue: "62a4c81d-…-8f21", group: "Identity" },
      { key: "defender_enabled", label: "Microsoft Defender for Cloud", description: "Enable continuous threat protection on all resources", type: "toggle", defaultValue: true, group: "Threat Protection" },
    ],
  },
  {
    personaId: "provider-admin",
    category: "infrastructure",
    title: "Provider Infrastructure",
    description: "Configure tenant-level storage quotas, user service limits, and provisioning settings.",
    fields: [
      { key: "tenant_storage_quota_tb", label: "Storage Quota", description: "Maximum tenant storage allocation", type: "number", defaultValue: 10, unit: "TB", group: "Quotas" },
      { key: "max_users", label: "Maximum Users", description: "Licensed user seat cap for this provider org", type: "number", defaultValue: 200, unit: "users", group: "Quotas" },
      { key: "provisioning_region", label: "Provisioning Region", description: "Azure region for all tenant resources", type: "select", defaultValue: "East US", options: ["East US", "East US 2", "West US 2", "West Europe", "Southeast Asia"], group: "Region" },
      { key: "sandbox_enabled", label: "Sandbox Environments", description: "Allow users to create isolated sandbox workspaces", type: "toggle", defaultValue: true, group: "Environments" },
      { key: "sso_domain", label: "SSO Domain", description: "Enterprise SSO domain for SAML/OIDC federation", type: "text", defaultValue: "yourorg.com", group: "Identity" },
    ],
  },
  {
    personaId: "governance",
    category: "infrastructure",
    title: "Governance Infrastructure",
    description: "Configure audit log destinations, data residency zones, and compliance archival.",
    fields: [
      { key: "audit_log_storage", label: "Audit Log Storage", description: "Azure Storage account for immutable audit logs", type: "text", defaultValue: "aimktauditlogs001", group: "Audit" },
      { key: "audit_retention_days", label: "Audit Retention", description: "Number of days to retain audit events", type: "number", defaultValue: 365, unit: "days", group: "Audit" },
      { key: "data_residency_region", label: "Data Residency Region", description: "Enforce all PII data to stay within this region", type: "select", defaultValue: "United States", options: ["United States", "European Union", "United Kingdom", "Australia"], group: "Compliance" },
      { key: "purview_workspace", label: "Microsoft Purview Workspace", description: "Data governance and lineage tracking workspace", type: "text", defaultValue: "aimarket-purview", group: "Catalog" },
      { key: "approval_workflow_engine", label: "Approval Workflow Engine", description: "Engine used for multi-stage approval flows", type: "select", defaultValue: "Azure Logic Apps", options: ["Azure Logic Apps", "Power Automate", "ServiceNow", "Jira Service Management"], group: "Workflow" },
    ],
  },
  {
    personaId: "product-manager",
    category: "infrastructure",
    title: "Platform Overview",
    description: "Read-only view of the service topology and environment configuration.",
    fields: [
      { key: "environment_name", label: "Environment", description: "Active deployment environment", type: "readonly", defaultValue: "ai-marketplace-dev", group: "Environment" },
      { key: "azure_region", label: "Azure Region", description: "Primary deployment region", type: "readonly", defaultValue: "East US", group: "Environment" },
      { key: "subscription_id", label: "Subscription", description: "Azure subscription in use", type: "readonly", defaultValue: "a7fecb91-4553-…", group: "Environment" },
      { key: "active_services", label: "Active Services", description: "Number of platform services currently running", type: "readonly", defaultValue: "12 of 14", group: "Services" },
      { key: "roadmap_link", label: "Roadmap Board", description: "Link to the product roadmap (Azure DevOps Board)", type: "text", defaultValue: "https://dev.azure.com/optum/aimarket/_boards/board", group: "Planning" },
    ],
  },
  // Remaining personas — infrastructure
  {
    personaId: "provider-data-scientist",
    category: "infrastructure",
    title: "ML Training Infrastructure",
    description: "Configure compute clusters, model registries, and experiment tracking backends.",
    fields: [
      { key: "training_compute", label: "Training Compute", description: "GPU/CPU cluster for model fine-tuning", type: "select", defaultValue: "Standard_NC6s_v3 (1× V100)", options: ["Standard_NC6s_v3 (1× V100)", "Standard_NC12s_v3 (2× V100)", "Standard_NC24s_v3 (4× V100)", "Standard_ND40rs_v2 (8× A100)", "CPU only"], group: "Compute" },
      { key: "model_registry", label: "Model Registry", description: "Registry for versioned model artifacts", type: "select", defaultValue: "Azure ML Registry", options: ["Azure ML Registry", "MLflow Server", "HuggingFace Hub (private)"], group: "Models" },
      { key: "experiment_tracker", label: "Experiment Tracker", description: "Run tracking and metric logging platform", type: "select", defaultValue: "Azure ML Experiments", options: ["Azure ML Experiments", "MLflow", "Weights & Biases", "Comet"], group: "Experiments" },
      { key: "dataset_mount_path", label: "Dataset Mount Path", description: "Cloud filesystem mount path for training datasets", type: "text", defaultValue: "/mnt/provider-datasets", group: "Storage" },
    ],
  },
  {
    personaId: "it-security",
    category: "infrastructure",
    title: "IT Integration Infrastructure",
    description: "Configure AD sync, SSO, network integration, and HSM settings.",
    fields: [
      { key: "ad_tenant_id", label: "Entra ID Tenant", description: "Microsoft Entra ID tenant for user federation", type: "text", defaultValue: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", group: "Identity" },
      { key: "sso_protocol", label: "SSO Protocol", description: "Federated authentication protocol", type: "select", defaultValue: "OIDC", options: ["OIDC", "SAML 2.0", "WS-Federation"], group: "Identity" },
      { key: "network_peering", label: "On-Prem VPN / Peering", description: "VPN Gateway or ExpressRoute circuit name", type: "text", defaultValue: "aimarket-vpngw-dev", group: "Network" },
      { key: "hsm_enabled", label: "Hardware Security Module", description: "Route key operations through Azure Dedicated HSM", type: "toggle", defaultValue: false, group: "Cryptography" },
    ],
  },
  {
    personaId: "ml-engineer",
    category: "infrastructure",
    title: "API & SDK Infrastructure",
    description: "Configure API gateway endpoints, SDK package feeds, and webhook destinations.",
    fields: [
      { key: "api_gateway_url", label: "API Gateway URL", description: "Base URL for all platform REST APIs", type: "text", defaultValue: "https://ai-marketplace-api-dev.azurewebsites.net", group: "API" },
      { key: "apim_subscription_key", label: "APIM Subscription Key Header", description: "Custom header for APIM subscription key", type: "text", defaultValue: "Ocp-Apim-Subscription-Key", group: "API" },
      { key: "sdk_feed", label: "SDK Package Feed", description: "npm/PyPI feed URL for @optum/ai-marketplace SDK", type: "text", defaultValue: "https://pkgs.dev.azure.com/optum/aimarket/_packaging/ai-assets/npm/registry/", group: "SDK" },
      { key: "webhook_endpoint", label: "Webhook Destination", description: "Your endpoint to receive platform event webhooks", type: "text", defaultValue: "", group: "Events" },
    ],
  },
  {
    personaId: "vendor-engineer",
    category: "infrastructure",
    title: "Plugin & Connector Infrastructure",
    description: "Configure plugin repository, connector storage, and extension SDK endpoints.",
    fields: [
      { key: "plugin_registry_url", label: "Plugin Registry", description: "URL of the platform plugin registry", type: "text", defaultValue: "https://plugins.ai-marketplace.optum.com", group: "Plugins" },
      { key: "connector_storage", label: "Connector Storage Account", description: "Storage account for connector bundles", type: "text", defaultValue: "aimarket-connectors-dev", group: "Storage" },
      { key: "extension_sdk_version", label: "Extension SDK Version", description: "Minimum supported extension SDK version", type: "select", defaultValue: "v2.3.0", options: ["v2.3.0", "v2.2.1", "v2.1.0"], group: "SDK" },
    ],
  },
  {
    personaId: "startup-science",
    category: "infrastructure",
    title: "Sandbox Infrastructure",
    description: "Configure sandbox compute, experiment isolation, and BYOM storage.",
    fields: [
      { key: "sandbox_compute", label: "Sandbox Compute Tier", description: "Compute tier allocated to your team sandbox", type: "select", defaultValue: "Standard_DS3_v2 (4 vCPU, 14 GB)", options: ["Standard_DS3_v2 (4 vCPU, 14 GB)", "Standard_DS4_v2 (8 vCPU, 28 GB)", "Standard_NC6s_v3 (6 vCPU, 1× V100)"], group: "Compute" },
      { key: "byom_storage", label: "BYOM Storage Account", description: "Dedicated storage for brought-your-own-model artifacts", type: "text", defaultValue: "aimarket-byom-dev", group: "BYOM" },
      { key: "isolation_mode", label: "Sandbox Isolation", description: "Network and data isolation level for experiments", type: "select", defaultValue: "Full VNet Isolation", options: ["Full VNet Isolation", "Shared Network (Restricted ACLs)", "Public (No Isolation)"], group: "Security" },
    ],
  },
  {
    personaId: "external-auditor",
    category: "infrastructure",
    title: "Audit Access Infrastructure",
    description: "Configure read-only audit data access, compliance export targets, and access windows.",
    fields: [
      { key: "audit_access_storage", label: "Audit Data Storage SAS", description: "SAS URI used to access the audit data lake", type: "text", defaultValue: "https://aimktauditlogs001.blob.core.windows.net/audit-export?sv=…", group: "Access" },
      { key: "access_window_days", label: "Access Window", description: "Number of days your temporary access window is valid", type: "readonly", defaultValue: "30 days (expires 2026-04-02)", group: "Access" },
      { key: "export_format", label: "Compliance Export Format", description: "File format for compliance data exports", type: "select", defaultValue: "JSON Lines (.jsonl)", options: ["JSON Lines (.jsonl)", "Parquet", "CSV"], group: "Export" },
    ],
  },
  // Personas with no infra settings — use empty array to show "not applicable"
  {
    personaId: "clinical-expert",
    category: "infrastructure",
    title: "Model Serving Endpoints",
    description: "Read-only view of model serving infrastructure relevant to clinical review.",
    fields: [
      { key: "inference_endpoint", label: "Primary Inference Endpoint", description: "REST endpoint for model scoring", type: "readonly", defaultValue: "https://aimarket-inference.azurecontainerapps.io", group: "Endpoints" },
      { key: "model_version", label: "Active Clinical Model Version", description: "Version of the model currently under clinical review", type: "readonly", defaultValue: "v2.1.4-rc", group: "Models" },
    ],
  },
  {
    personaId: "domain-expert",
    category: "infrastructure",
    title: "Domain Data Sources",
    description: "Configure domain-specific reference data sources used in model validation.",
    fields: [
      { key: "reference_data_path", label: "Reference Data Path", description: "Blob storage path containing ground-truth reference data", type: "text", defaultValue: "https://aimarket-data.blob.core.windows.net/domain-ref/", group: "Data" },
      { key: "validation_framework", label: "Validation Framework", description: "Framework used for domain model validation", type: "select", defaultValue: "Great Expectations", options: ["Great Expectations", "Deequ", "Custom", "None"], group: "Validation" },
    ],
  },
  {
    personaId: "bi-analyst",
    category: "infrastructure",
    title: "BI & Analytics Infrastructure",
    description: "Configure BI connectors, data warehouse endpoints, and dashboard source connections.",
    fields: [
      { key: "synapse_workspace", label: "Synapse Workspace", description: "Azure Synapse workspace for SQL and Spark analytics", type: "text", defaultValue: "aimarket-synapse-dev", group: "Warehouse" },
      { key: "powerbi_workspace", label: "Power BI Workspace ID", description: "ID of the Power BI workspace for published dashboards", type: "text", defaultValue: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", group: "Dashboards" },
      { key: "refresh_storage", label: "Dashboard Cache Storage", description: "Storage container for pre-aggregated dashboard caches", type: "text", defaultValue: "aimarket-bicache-dev", group: "Dashboards" },
    ],
  },
  {
    personaId: "operational-admin",
    category: "infrastructure",
    title: "Operational Tool Endpoints",
    description: "Configure workflow tool integration endpoints for daily operations.",
    fields: [
      { key: "workflow_api_url", label: "Workflow API URL", description: "API endpoint used by operational tools", type: "text", defaultValue: "https://ai-marketplace-api-dev.azurewebsites.net/api/workflows", group: "Tools" },
      { key: "notification_channel", label: "Notification Channel", description: "Teams or Slack webhook for workflow notifications", type: "text", defaultValue: "https://outlook.office.com/webhook/…", group: "Notifications" },
    ],
  },
  {
    personaId: "review-board",
    category: "infrastructure",
    title: "Review & Audit Access",
    description: "Configure audit trail access and review storage for ethics board workflows.",
    fields: [
      { key: "review_storage", label: "Review Artifacts Storage", description: "Storage account for model review packages", type: "text", defaultValue: "aimarket-reviews-dev", group: "Storage" },
      { key: "approval_system", label: "Approval System URL", description: "ServiceNow or Jira endpoint for approval tickets", type: "text", defaultValue: "https://optum.service-now.com/ai-review", group: "Workflow" },
    ],
  },
  {
    personaId: "isv",
    category: "infrastructure",
    title: "Marketplace Integration",
    description: "Configure marketplace listing endpoints, billing integration, and SDK settings.",
    fields: [
      { key: "listing_api_url", label: "Listing API URL", description: "API endpoint for submitting marketplace listings", type: "text", defaultValue: "https://ai-marketplace-api-dev.azurewebsites.net/api/catalog", group: "API" },
      { key: "billing_webhook", label: "Billing Webhook URL", description: "Your endpoint to receive usage billing events", type: "text", defaultValue: "", group: "Billing" },
      { key: "sdk_language", label: "Primary SDK Language", description: "Programming language for SDK integration", type: "select", defaultValue: "TypeScript", options: ["TypeScript", "Python", "C#", "Java", "Go"], group: "SDK" },
    ],
  },

  // ── PERFORMANCE ─────────────────────────────────────────────────────────────
  {
    personaId: "data-engineer",
    category: "performance",
    title: "Pipeline Performance",
    description: "Tune parallelism, batch sizing, caching strategy, and streaming throughput.",
    fields: [
      { key: "batch_size", label: "Ingestion Batch Size", description: "Number of records per pipeline micro-batch", type: "number", defaultValue: 10000, unit: "records", group: "Throughput" },
      { key: "stream_partitions", label: "Stream Partitions", description: "Kafka / Event Hubs partition count for real-time feeds", type: "number", defaultValue: 32, unit: "partitions", group: "Throughput" },
      { key: "cache_ttl", label: "Feature Cache TTL", description: "Time-to-live for cached feature values in Redis", type: "number", defaultValue: 300, unit: "seconds", group: "Caching" },
      { key: "cache_enabled", label: "Feature Cache", description: "Enable in-memory caching for hot features", type: "toggle", defaultValue: true, group: "Caching" },
      { key: "spark_driver_memory", label: "Spark Driver Memory", description: "JVM heap allocated to the Spark driver", type: "select", defaultValue: "8g", options: ["4g", "8g", "16g", "32g"], group: "Spark" },
      { key: "checkpoint_interval", label: "Checkpoint Interval", description: "Frequency of streaming checkpoints", type: "number", defaultValue: 60, unit: "seconds", group: "Throughput" },
    ],
  },
  {
    personaId: "devops-sre",
    category: "performance",
    title: "Deployment & Scaling Performance",
    description: "Configure auto-scaling policies, load balancing, and CDN settings.",
    fields: [
      { key: "min_replicas", label: "Min Replicas", description: "Minimum container replica count (Container Apps)", type: "number", defaultValue: 1, unit: "replicas", group: "Scaling" },
      { key: "max_replicas", label: "Max Replicas", description: "Maximum container replica count under load", type: "number", defaultValue: 10, unit: "replicas", group: "Scaling" },
      { key: "scale_up_threshold", label: "Scale-Up CPU Threshold", description: "CPU % that triggers a scale-out event", type: "number", defaultValue: 70, unit: "%", group: "Scaling" },
      { key: "cdn_enabled", label: "Azure CDN", description: "Serve static assets via Azure Front Door / CDN", type: "toggle", defaultValue: true, group: "CDN" },
      { key: "load_balancer_algo", label: "Load Balancing Algorithm", description: "Traffic routing strategy across replicas", type: "select", defaultValue: "Round Robin", options: ["Round Robin", "Least Connections", "IP Hash / Session Affinity"], group: "Load Balancing" },
      { key: "startup_probe_delay", label: "Startup Probe Delay", description: "Seconds before the first liveness probe fires", type: "number", defaultValue: 15, unit: "seconds", group: "Health" },
    ],
  },
  {
    personaId: "provider-data-scientist",
    category: "performance",
    title: "Training Job Performance",
    description: "Configure GPU quotas, distributed training strategy, and hyperparameter search budget.",
    fields: [
      { key: "gpu_quota", label: "GPU Quota", description: "Maximum GPU cores available per training job", type: "number", defaultValue: 4, unit: "GPUs", group: "Compute" },
      { key: "distributed_training", label: "Distributed Training", description: "Enable multi-node distributed training (DDP / DeepSpeed)", type: "toggle", defaultValue: false, group: "Compute" },
      { key: "hparam_trials", label: "Hyperparameter Trials", description: "Max trials for automated hyperparameter optimization", type: "number", defaultValue: 50, unit: "trials", group: "AutoML" },
      { key: "mixed_precision", label: "Mixed Precision (FP16)", description: "Use FP16 to speed up training on Tensor Core GPUs", type: "toggle", defaultValue: true, group: "Compute" },
      { key: "gradient_checkpointing", label: "Gradient Checkpointing", description: "Trade compute for memory to train larger models", type: "toggle", defaultValue: false, group: "Memory" },
    ],
  },
  {
    personaId: "bi-analyst",
    category: "performance",
    title: "Query & Dashboard Performance",
    description: "Set query timeouts, materialisation schedules, and result caching.",
    fields: [
      { key: "query_timeout", label: "Query Timeout", description: "Maximum execution time for ad-hoc queries", type: "number", defaultValue: 60, unit: "seconds", group: "Queries" },
      { key: "result_cache_enabled", label: "Result Set Caching", description: "Cache query results in Azure Synapse", type: "toggle", defaultValue: true, group: "Caching" },
      { key: "dashboard_refresh_interval", label: "Dashboard Refresh Interval", description: "How often live dashboards auto-refresh", type: "select", defaultValue: "5 minutes", options: ["1 minute", "5 minutes", "15 minutes", "30 minutes", "Manual"], group: "Dashboards" },
      { key: "materialized_views", label: "Materialised Views", description: "Pre-compute expensive aggregations nightly", type: "toggle", defaultValue: true, group: "Optimisation" },
    ],
  },
  {
    personaId: "ml-engineer",
    category: "performance",
    title: "API Performance",
    description: "Configure rate limits, response caching, and request batching at the API gateway.",
    fields: [
      { key: "rate_limit_rpm", label: "Rate Limit", description: "Max requests per minute per API key", type: "number", defaultValue: 1000, unit: "req/min", group: "Rate Limiting" },
      { key: "response_cache_enabled", label: "Response Caching", description: "Cache idempotent GET responses at the gateway", type: "toggle", defaultValue: true, group: "Caching" },
      { key: "cache_ttl", label: "Response Cache TTL", description: "Time-to-live for cached API responses", type: "number", defaultValue: 60, unit: "seconds", group: "Caching" },
      { key: "batch_inference_size", label: "Batch Inference Size", description: "Max items per batch scoring request", type: "number", defaultValue: 100, unit: "items", group: "Inference" },
      { key: "connection_timeout", label: "Connection Timeout", description: "HTTP connection timeout for downstream services", type: "number", defaultValue: 30, unit: "seconds", group: "Network" },
    ],
  },
  {
    personaId: "security-engineer",
    category: "performance",
    title: "Security Scan Performance",
    description: "Tune vulnerability scanning frequency, WAF rule processing, and key rotation schedules.",
    fields: [
      { key: "waf_mode", label: "WAF Mode", description: "Azure Front Door WAF enforcement mode", type: "select", defaultValue: "Prevention", options: ["Detection", "Prevention"], group: "WAF" },
      { key: "scan_frequency", label: "Vulnerability Scan Frequency", description: "How often Defender runs active vulnerability scans", type: "select", defaultValue: "Daily", options: ["Hourly", "Daily", "Weekly"], group: "Scanning" },
      { key: "key_rotation_days", label: "Key Rotation Interval", description: "Days between automatic Key Vault secret rotation", type: "number", defaultValue: 90, unit: "days", group: "Rotation" },
      { key: "ddos_protection", label: "DDoS Protection Standard", description: "Enable Azure DDoS Protection Standard plan", type: "toggle", defaultValue: true, group: "Network" },
    ],
  },
  {
    personaId: "provider-admin",
    category: "performance",
    title: "Tenant Performance Limits",
    description: "Configure per-tenant throttling, concurrency limits, and priority tiers.",
    fields: [
      { key: "concurrent_jobs", label: "Max Concurrent Jobs", description: "Maximum simultaneous workload jobs per tenant", type: "number", defaultValue: 20, unit: "jobs", group: "Concurrency" },
      { key: "throughput_tier", label: "Throughput Tier", description: "API throughput tier for this tenant", type: "select", defaultValue: "Standard (1,000 req/min)", options: ["Basic (100 req/min)", "Standard (1,000 req/min)", "Premium (10,000 req/min)"], group: "Throttling" },
      { key: "priority_boost", label: "Job Priority Boost", description: "Allow high-priority queue promotion for production jobs", type: "toggle", defaultValue: true, group: "Scheduling" },
    ],
  },
  {
    personaId: "governance",
    category: "performance",
    title: "Audit Event Performance",
    description: "Control audit stream throughput, event sampling, and log export batch size.",
    fields: [
      { key: "audit_batch_size", label: "Audit Export Batch Size", description: "Records per batch exported to the audit storage account", type: "number", defaultValue: 5000, unit: "records", group: "Export" },
      { key: "event_sampling_rate", label: "Read-Event Sampling Rate", description: "Fraction of read-only events to record (1 = all)", type: "number", defaultValue: 0.1, unit: "fraction (0–1)", group: "Sampling" },
      { key: "lag_alert_seconds", label: "Export Lag Alert", description: "Alert if audit stream falls behind by this many seconds", type: "number", defaultValue: 30, unit: "seconds", group: "Monitoring" },
    ],
  },
  // Personas with minimal or no performance settings
  {
    personaId: "it-security",
    category: "performance",
    title: "Integration Performance",
    description: "Configure connection pool sizes, timeout settings, and retry behaviour for integrations.",
    fields: [
      { key: "connection_pool_size", label: "Connection Pool Size", description: "Max simultaneous connections to the platform API", type: "number", defaultValue: 50, unit: "connections", group: "Pooling" },
      { key: "request_timeout", label: "Request Timeout", description: "HTTP request timeout for outbound integrations", type: "number", defaultValue: 30, unit: "seconds", group: "Timeouts" },
      { key: "retry_count", label: "Retry Count", description: "Number of retries on transient failures", type: "number", defaultValue: 3, unit: "retries", group: "Retries" },
    ],
  },
  {
    personaId: "vendor-engineer",
    category: "performance",
    title: "Extension Performance",
    description: "Set upload size limits, install timeout, and connector execution budgets.",
    fields: [
      { key: "max_upload_mb", label: "Max Upload Size", description: "Maximum connector bundle upload size", type: "number", defaultValue: 100, unit: "MB", group: "Upload" },
      { key: "install_timeout", label: "Install Timeout", description: "Maximum time allowed for extension install", type: "number", defaultValue: 120, unit: "seconds", group: "Execution" },
      { key: "execution_memory_mb", label: "Execution Memory", description: "Memory budget for running connector logic", type: "number", defaultValue: 512, unit: "MB", group: "Execution" },
    ],
  },
  {
    personaId: "startup-science",
    category: "performance",
    title: "Experiment Performance",
    description: "Control experiment concurrency, checkpointing, and early-stopping behaviour.",
    fields: [
      { key: "parallel_experiments", label: "Parallel Experiments", description: "Maximum concurrently running experiment runs", type: "number", defaultValue: 5, unit: "runs", group: "Concurrency" },
      { key: "early_stopping", label: "Early Stopping", description: "Terminate under-performing runs automatically", type: "toggle", defaultValue: true, group: "Optimisation" },
      { key: "checkpoint_every", label: "Checkpoint Every", description: "Save model checkpoint every N epochs", type: "number", defaultValue: 5, unit: "epochs", group: "Checkpointing" },
    ],
  },
  {
    personaId: "product-manager",
    category: "performance",
    title: "Platform Performance Metrics",
    description: "Read-only view of platform-wide performance indicators and SLA telemetry.",
    fields: [
      { key: "p95_latency_ms", label: "P95 API Latency (last 7d)", description: "95th percentile API response time", type: "readonly", defaultValue: "142 ms", group: "SLI" },
      { key: "throughput_rpm", label: "Peak Throughput (last 7d)", description: "Highest sustained requests-per-minute recorded", type: "readonly", defaultValue: "4,820 req/min", group: "SLI" },
      { key: "perf_dashboard", label: "Performance Dashboard", description: "Link to the Azure Monitor performance workbook", type: "text", defaultValue: "https://portal.azure.com/#@/dashboard/…", group: "Dashboards" },
    ],
  },
  {
    personaId: "review-board",
    category: "performance",
    title: "Review Workflow Performance",
    description: "Configure review queue target SLAs and escalation timing.",
    fields: [
      { key: "review_sla_hours", label: "Review SLA", description: "Target hours to complete an initial model review", type: "number", defaultValue: 72, unit: "hours", group: "SLA" },
      { key: "escalation_hours", label: "Escalation Threshold", description: "Hours before an overdue review is auto-escalated", type: "number", defaultValue: 96, unit: "hours", group: "SLA" },
    ],
  },
  {
    personaId: "clinical-expert",
    category: "performance",
    title: "Clinical Review Performance",
    description: "Configure clinical review queue size and scoring latency thresholds.",
    fields: [
      { key: "max_daily_reviews", label: "Max Daily Reviews", description: "Maximum model outputs queued for clinical review per day", type: "number", defaultValue: 100, unit: "records", group: "Queue" },
      { key: "score_latency_alert_ms", label: "Scoring Latency Alert", description: "Alert if scoring response exceeds this threshold", type: "number", defaultValue: 500, unit: "ms", group: "Latency" },
    ],
  },
  {
    personaId: "domain-expert",
    category: "performance",
    title: "Validation Performance",
    description: "Configure validation sample sizes and test execution budgets.",
    fields: [
      { key: "validation_sample_size", label: "Validation Sample Size", description: "Number of records to draw for each validation run", type: "number", defaultValue: 1000, unit: "records", group: "Sampling" },
      { key: "test_timeout_min", label: "Test Suite Timeout", description: "Maximum minutes allowed for a domain validation suite", type: "number", defaultValue: 30, unit: "minutes", group: "Execution" },
    ],
  },
  {
    personaId: "operational-admin",
    category: "performance",
    title: "Operational Throughput",
    description: "Configure concurrency and timeout budgets for daily operational workflows.",
    fields: [
      { key: "max_concurrent_workflows", label: "Max Concurrent Workflows", description: "Maximum workflows running simultaneously for this team", type: "number", defaultValue: 10, unit: "workflows", group: "Concurrency" },
      { key: "workflow_timeout_min", label: "Workflow Timeout", description: "Maximum minutes a single workflow run may take", type: "number", defaultValue: 60, unit: "minutes", group: "Timeouts" },
    ],
  },
  {
    personaId: "isv",
    category: "performance",
    title: "ISV API Performance",
    description: "Configure API request budget, listing endpoint rate limits, and billing callback timeout.",
    fields: [
      { key: "monthly_api_quota", label: "Monthly API Quota", description: "Total API calls included in your ISV plan per month", type: "readonly", defaultValue: "500,000 calls", group: "Quota" },
      { key: "listing_rps", label: "Listing API Rate Limit", description: "Max requests per second to the catalog/listing API", type: "number", defaultValue: 20, unit: "req/s", group: "Rate Limiting" },
      { key: "billing_callback_timeout", label: "Billing Callback Timeout", description: "Seconds before billing webhook callback times out", type: "number", defaultValue: 5, unit: "seconds", group: "Billing" },
    ],
  },
  {
    personaId: "external-auditor",
    category: "performance",
    title: "Audit Export Performance",
    description: "Control export batch sizes and throughput for compliance data retrieval.",
    fields: [
      { key: "export_batch_size", label: "Export Batch Size", description: "Number of events per exported file chunk", type: "number", defaultValue: 50000, unit: "events", group: "Export" },
      { key: "max_parallel_exports", label: "Max Parallel Exports", description: "Concurrent export threads allowed for this auditor", type: "number", defaultValue: 2, unit: "threads", group: "Export" },
    ],
  },

  // ── RELIABILITY ─────────────────────────────────────────────────────────────
  {
    personaId: "devops-sre",
    category: "reliability",
    title: "SLOs & Incident Management",
    description: "Define SLO targets, error budgets, alerting thresholds, and on-call escalation paths.",
    fields: [
      { key: "availability_slo", label: "Availability SLO", description: "Target uptime percentage for the platform", type: "number", defaultValue: 99.9, unit: "%", group: "SLOs" },
      { key: "latency_slo_ms", label: "Latency SLO (P99)", description: "P99 latency target in milliseconds", type: "number", defaultValue: 500, unit: "ms", group: "SLOs" },
      { key: "error_budget_policy", label: "Error Budget Policy", description: "Action to take when the error budget falls below 10%", type: "select", defaultValue: "Freeze releases", options: ["Freeze releases", "Alert only", "Alert + reduce change rate"], group: "Error Budgets" },
      { key: "alert_channel", label: "Alert Notification Channel", description: "PagerDuty, OpsGenie, or Teams webhook for incidents", type: "text", defaultValue: "https://events.pagerduty.com/integration/…", group: "Alerting" },
      { key: "runbook_url", label: "Runbook URL", description: "Primary operations runbook for incident response", type: "text", defaultValue: "https://confluence.optum.com/ai-marketplace/runbooks", group: "Incidents" },
      { key: "backup_frequency", label: "Backup Frequency", description: "How often configuration state is backed up", type: "select", defaultValue: "Daily", options: ["Hourly", "Daily", "Weekly"], group: "Backups" },
    ],
  },
  {
    personaId: "security-engineer",
    category: "reliability",
    title: "Security Reliability",
    description: "Configure failover policies, certificate renewal, and security baseline enforcement.",
    fields: [
      { key: "cert_auto_renew", label: "Certificate Auto-Renewal", description: "Automatically renew TLS certificates before expiry", type: "toggle", defaultValue: true, group: "Certificates" },
      { key: "cert_renewal_days_before", label: "Renewal Lead Time", description: "Days before expiry to trigger certificate renewal", type: "number", defaultValue: 30, unit: "days", group: "Certificates" },
      { key: "failover_policy", label: "Failover Policy", description: "Geographic failover strategy on regional outage", type: "select", defaultValue: "Active-Passive (East US 2)", options: ["Active-Passive (East US 2)", "Active-Active Multi-Region", "Manual Failover"], group: "Failover" },
      { key: "policy_enforcement", label: "Azure Policy Enforcement", description: "Enforcement mode for platform-wide security policies", type: "select", defaultValue: "Deny (enforce)", options: ["Deny (enforce)", "Audit (report only)", "Disabled"], group: "Policy" },
      { key: "mfa_required", label: "Enforce MFA", description: "Require Multi-Factor Authentication for all users", type: "toggle", defaultValue: true, group: "Identity" },
    ],
  },
  {
    personaId: "provider-admin",
    category: "reliability",
    title: "Tenant Reliability & SLAs",
    description: "Manage uptime SLA commitments, disaster recovery, and tenant backup policies.",
    fields: [
      { key: "contracted_sla", label: "Contracted SLA", description: "Agreed uptime SLA for this provider tenant", type: "select", defaultValue: "99.9% (≈ 8.7 h/year downtime)", options: ["99.0% (≈ 3.65 d/year)", "99.9% (≈ 8.7 h/year)", "99.95% (≈ 4.4 h/year)"], group: "SLA" },
      { key: "dr_rto_hours", label: "RTO Target", description: "Recovery Time Objective in hours for a DR event", type: "number", defaultValue: 4, unit: "hours", group: "Disaster Recovery" },
      { key: "dr_rpo_hours", label: "RPO Target", description: "Recovery Point Objective — maximum acceptable data loss", type: "number", defaultValue: 1, unit: "hours", group: "Disaster Recovery" },
      { key: "backup_retention_days", label: "Backup Retention", description: "Days to retain tenant data backups", type: "number", defaultValue: 30, unit: "days", group: "Backups" },
    ],
  },
  {
    personaId: "governance",
    category: "reliability",
    title: "Compliance Reliability",
    description: "Ensure audit trail continuity, policy refresh schedules, and control-failure alerting.",
    fields: [
      { key: "audit_trail_continuity", label: "Audit Trail Continuity", description: "Alert if a gap in the audit stream is detected", type: "toggle", defaultValue: true, group: "Audit" },
      { key: "policy_refresh_days", label: "Policy Refresh Cadence", description: "Days between automated policy-set refreshes from Purview", type: "number", defaultValue: 7, unit: "days", group: "Policy" },
      { key: "compliance_alert_email", label: "Compliance Alert Email", description: "Email recipients for control-failure notifications", type: "text", defaultValue: "compliance-alerts@optum.com", group: "Alerting" },
    ],
  },
  {
    personaId: "data-engineer",
    category: "reliability",
    title: "Pipeline Reliability",
    description: "Configure retry policies, dead-letter queues, and SLA breach alerting for pipelines.",
    fields: [
      { key: "pipeline_retry_count", label: "Pipeline Retry Count", description: "Number of automatic retries on job failure", type: "number", defaultValue: 3, unit: "retries", group: "Retries" },
      { key: "dead_letter_queue", label: "Dead-Letter Queue", description: "Azure Service Bus queue for failed pipeline events", type: "text", defaultValue: "aimarket-pipeline-dlq", group: "Error Handling" },
      { key: "pipeline_sla_alert_min", label: "Pipeline SLA Alert", description: "Alert if a pipeline run exceeds this duration", type: "number", defaultValue: 30, unit: "minutes", group: "SLA" },
      { key: "idempotency_enabled", label: "Idempotent Writes", description: "Enable idempotency keys to prevent duplicate data", type: "toggle", defaultValue: true, group: "Correctness" },
    ],
  },
  {
    personaId: "product-manager",
    category: "reliability",
    title: "Platform Reliability Dashboard",
    description: "Read-only view of platform reliability metrics and incident history.",
    fields: [
      { key: "mttr_hours", label: "MTTR (last 90 days)", description: "Mean Time To Recovery for recent incidents", type: "readonly", defaultValue: "0.8 hours", group: "Metrics" },
      { key: "incidents_90d", label: "Incidents (last 90 days)", description: "Total P1/P2 incidents in the past quarter", type: "readonly", defaultValue: "2", group: "Metrics" },
      { key: "availability_30d", label: "Availability (last 30 days)", description: "Measured platform availability percentage", type: "readonly", defaultValue: "99.97%", group: "Metrics" },
      { key: "status_page_url", label: "Status Page", description: "Public platform status page URL", type: "text", defaultValue: "https://status.ai-marketplace.optum.com", group: "Communication" },
    ],
  },
  {
    personaId: "it-security",
    category: "reliability",
    title: "Integration Reliability",
    description: "Configure circuit breakers, retry policies, and fallback endpoints for integrations.",
    fields: [
      { key: "circuit_breaker_enabled", label: "Circuit Breaker", description: "Open circuit after consecutive failures to prevent cascade", type: "toggle", defaultValue: true, group: "Resilience" },
      { key: "failure_threshold", label: "Failure Threshold", description: "Consecutive failures before opening the circuit", type: "number", defaultValue: 5, unit: "failures", group: "Resilience" },
      { key: "fallback_endpoint", label: "Fallback Endpoint", description: "Secondary API endpoint used when primary is degraded", type: "text", defaultValue: "", group: "Failover" },
      { key: "health_check_interval", label: "Health Check Interval", description: "Seconds between integration health probes", type: "number", defaultValue: 30, unit: "seconds", group: "Health" },
    ],
  },
  {
    personaId: "provider-data-scientist",
    category: "reliability",
    title: "Training Job Reliability",
    description: "Configure job preemption tolerance, spot instance fallback, and checkpoint recovery.",
    fields: [
      { key: "spot_instance_fallback", label: "Spot Instance Fallback", description: "Fall back to on-demand if spot capacity unavailable", type: "toggle", defaultValue: true, group: "Compute" },
      { key: "preemption_tolerance", label: "Preemption Tolerance", description: "Resume from last checkpoint after spot eviction", type: "toggle", defaultValue: true, group: "Compute" },
      { key: "max_run_hours", label: "Max Run Duration", description: "Hard cap on training job wall-clock time", type: "number", defaultValue: 24, unit: "hours", group: "Budget" },
    ],
  },
  {
    personaId: "ml-engineer",
    category: "reliability",
    title: "API Reliability",
    description: "Configure retry budget, timeout, and fallback model for inference API calls.",
    fields: [
      { key: "retry_budget", label: "Client Retry Budget", description: "Maximum retries a client SDK will attempt", type: "number", defaultValue: 3, unit: "retries", group: "Retries" },
      { key: "retry_backoff_ms", label: "Retry Backoff (initial)", description: "Initial backoff delay for exponential retry strategy", type: "number", defaultValue: 200, unit: "ms", group: "Retries" },
      { key: "fallback_model", label: "Fallback Model ID", description: "Model to invoke if primary model is unavailable", type: "text", defaultValue: "", group: "Fallback" },
      { key: "timeout_ms", label: "Request Timeout", description: "Max milliseconds to wait for an API response", type: "number", defaultValue: 10000, unit: "ms", group: "Timeouts" },
    ],
  },
  {
    personaId: "bi-analyst",
    category: "reliability",
    title: "Report Reliability",
    description: "Configure dashboard failure alerts, stale data thresholds, and refresh failure handling.",
    fields: [
      { key: "stale_data_alert_hours", label: "Stale Data Alert", description: "Alert if dashboard data has not refreshed within this window", type: "number", defaultValue: 6, unit: "hours", group: "Freshness" },
      { key: "refresh_failure_email", label: "Refresh Failure Email", description: "Email to notify when a scheduled refresh fails", type: "text", defaultValue: "bi-ops@yourorg.com", group: "Alerting" },
      { key: "fallback_cache_enabled", label: "Show Cached on Failure", description: "Display last successful refresh data on query failure", type: "toggle", defaultValue: true, group: "Fallback" },
    ],
  },
  {
    personaId: "clinical-expert",
    category: "reliability",
    title: "Clinical Safety Reliability",
    description: "Configure safety thresholds and auto-suspension rules for clinical model outputs.",
    fields: [
      { key: "auto_suspend_threshold", label: "Auto-Suspend Threshold", description: "Suspend model scoring if error rate exceeds this", type: "number", defaultValue: 5, unit: "% error rate", group: "Safety" },
      { key: "safety_alert_email", label: "Safety Alert Email", description: "Clinical safety team email for auto-suspend notifications", type: "text", defaultValue: "clinical-safety@yourorg.com", group: "Alerting" },
    ],
  },
  {
    personaId: "domain-expert",
    category: "reliability",
    title: "Validation Reliability",
    description: "Configure validation run retries and degraded-result alerting.",
    fields: [
      { key: "validation_retry", label: "Validation Retry Count", description: "Retries on transient data source failures during validation", type: "number", defaultValue: 2, unit: "retries", group: "Retries" },
      { key: "degraded_alert_threshold", label: "Degraded Alert Threshold", description: "Alert when pass rate drops below this percentage", type: "number", defaultValue: 90, unit: "% pass rate", group: "Quality" },
    ],
  },
  {
    personaId: "review-board",
    category: "reliability",
    title: "Approval Workflow Reliability",
    description: "Configure quorum requirements and escalation fallback for ethics review flows.",
    fields: [
      { key: "quorum_required", label: "Minimum Quorum", description: "Minimum reviewer approvals required to pass a model", type: "number", defaultValue: 3, unit: "reviews", group: "Approval" },
      { key: "escalation_fallback", label: "Escalation Fallback", description: "Email address for board chair when quorum is unmet", type: "text", defaultValue: "ethics-chair@optum.com", group: "Escalation" },
    ],
  },
  {
    personaId: "operational-admin",
    category: "reliability",
    title: "Workflow Operational Reliability",
    description: "Configure notification on workflow failure and retry settings.",
    fields: [
      { key: "failure_notification", label: "Workflow Failure Notification", description: "Teams or email for workflow failure alerts", type: "text", defaultValue: "ops-support@yourorg.com", group: "Notifications" },
      { key: "auto_retry_enabled", label: "Auto-Retry on Failure", description: "Automatically retry failed workflow steps", type: "toggle", defaultValue: true, group: "Retries" },
      { key: "max_retries", label: "Max Retries", description: "Maximum automatic retry attempts per workflow step", type: "number", defaultValue: 2, unit: "retries", group: "Retries" },
    ],
  },
  {
    personaId: "vendor-engineer",
    category: "reliability",
    title: "Extension Reliability",
    description: "Configure sandbox isolation, crash reporting, and extension circuit breaker.",
    fields: [
      { key: "sandbox_isolation", label: "Crash Isolation", description: "Isolate crashed extensions to prevent platform impact", type: "toggle", defaultValue: true, group: "Isolation" },
      { key: "crash_report_endpoint", label: "Crash Report Endpoint", description: "Your endpoint to receive extension crash reports", type: "text", defaultValue: "", group: "Reporting" },
    ],
  },
  {
    personaId: "startup-science",
    category: "reliability",
    title: "Experiment Reliability",
    description: "Configure experiment crash recovery, result persistence, and quota alerts.",
    fields: [
      { key: "auto_checkpoint_on_crash", label: "Auto-Checkpoint on Crash", description: "Save state on unexpected process termination", type: "toggle", defaultValue: true, group: "Recovery" },
      { key: "result_persistence_days", label: "Result Retention", description: "Days to retain experiment run artifacts and metrics", type: "number", defaultValue: 90, unit: "days", group: "Storage" },
      { key: "quota_alert_percent", label: "Quota Alert Threshold", description: "Alert when compute quota utilisation exceeds this %", type: "number", defaultValue: 80, unit: "%", group: "Quota" },
    ],
  },
  {
    personaId: "isv",
    category: "reliability",
    title: "ISV Integration Reliability",
    description: "Configure webhook retry policy and listing availability SLA.",
    fields: [
      { key: "webhook_retry_count", label: "Webhook Retry Count", description: "Retries before a failed webhook is dead-lettered", type: "number", defaultValue: 5, unit: "retries", group: "Webhooks" },
      { key: "listing_availability_sla", label: "Listing Availability SLA", description: "Guaranteed uptime for your listed product endpoints", type: "readonly", defaultValue: "99.9%", group: "SLA" },
      { key: "downtime_alert_email", label: "Downtime Alert Email", description: "Contact email for marketplace-initiated downtime alerts", type: "text", defaultValue: "", group: "Alerting" },
    ],
  },
  {
    personaId: "external-auditor",
    category: "reliability",
    title: "Audit Access Reliability",
    description: "Configure retry and fallback behaviour for audit data retrieval sessions.",
    fields: [
      { key: "session_timeout_min", label: "Session Timeout", description: "Minutes of inactivity before the audit session expires", type: "number", defaultValue: 60, unit: "minutes", group: "Session" },
      { key: "export_retry_count", label: "Export Retry Count", description: "Retries on failed compliance data export requests", type: "number", defaultValue: 3, unit: "retries", group: "Export" },
    ],
  },
]

/** Get settings for a specific persona + category combination */
export function getPersonaSettings(
  personaId: PersonaId,
  category: SettingCategory
): PersonaSettings | undefined {
  return SETTINGS_CATALOG.find(
    (s) => s.personaId === personaId && s.category === category
  )
}
