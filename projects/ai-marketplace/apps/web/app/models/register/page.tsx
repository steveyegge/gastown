"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  ArrowLeft,
  Brain,
  Check,
  AlertCircle,
  Loader2,
  Upload,
  Server,
  Shield,
  Tag,
  Info,
  ExternalLink,
  CheckCircle2,
} from "lucide-react"

const FRAMEWORKS = [
  "PyTorch",
  "TensorFlow",
  "ONNX",
  "Scikit-learn",
  "HuggingFace",
  "Custom",
  "Other",
] as const

const TASK_TYPES = [
  "NLP",
  "TextClassification",
  "TokenClassification",
  "QuestionAnswering",
  "Summarization",
  "TextGeneration",
  "ImageClassification",
  "ObjectDetection",
  "Regression",
  "Classification",
  "Forecasting",
  "Other",
] as const

const CATEGORIES = ["NLP", "Vision", "Prediction", "Analytics", "Generative AI", "Other"] as const

const COMPLIANCE_OPTIONS = ["HIPAA", "SOC2", "ISO27001", "FedRAMP", "PCI-DSS"] as const

type Step = 1 | 2 | 3

interface FormData {
  // Step 1 — Identity
  name: string
  version: string
  description: string
  type: "internal" | "partner" | "custom"
  // Step 2 — Artifact & Source
  modelUri: string
  framework: string
  taskType: string
  category: string
  // Step 3 — Governance
  compliance: string[]
  tags: string
  additionalInfo: string
}

const defaultForm: FormData = {
  name: "",
  version: "1",
  description: "",
  type: "custom",
  modelUri: "",
  framework: "",
  taskType: "",
  category: "NLP",
  compliance: [],
  tags: "",
  additionalInfo: "",
}

const STEPS: { id: Step; label: string; icon: React.ElementType }[] = [
  { id: 1, label: "Model Identity", icon: Brain },
  { id: 2, label: "Artifact & Source", icon: Server },
  { id: 3, label: "Governance", icon: Shield },
]

export default function RegisterModelPage() {
  const router = useRouter()
  const [step, setStep] = useState<Step>(1)
  const [form, setForm] = useState<FormData>(defaultForm)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [submitResult, setSubmitResult] = useState<{
    success: boolean
    message: string
    modelId?: string
    amlConfigured?: boolean
  } | null>(null)

  const update = (key: keyof FormData, value: unknown) =>
    setForm((f) => ({ ...f, [key]: value }))

  const toggleCompliance = (item: string) =>
    setForm((f) => ({
      ...f,
      compliance: f.compliance.includes(item)
        ? f.compliance.filter((c) => c !== item)
        : [...f.compliance, item],
    }))

  // --- Validation per step ---
  const step1Valid = form.name.trim() && form.version.trim() && form.description.trim()
  const step2Valid = form.modelUri.trim() && form.framework && form.taskType
  const canSubmit = step1Valid && step2Valid

  const handleSubmit = async () => {
    setIsSubmitting(true)
    setSubmitResult(null)

    const tagsObj: Record<string, string> = {}
    if (form.tags.trim()) {
      form.tags.split(",").forEach((t) => {
        const [k, v] = t.trim().split(":")
        if (k) tagsObj[k.trim()] = (v ?? "true").trim()
      })
    }

    try {
      const res = await fetch("/api/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: form.name.trim(),
          version: form.version.trim(),
          description: form.description.trim(),
          modelUri: form.modelUri.trim(),
          framework: form.framework,
          taskType: form.taskType,
          type: form.type,
          category: form.category,
          compliance: form.compliance,
          tags: tagsObj,
        }),
      })

      const data = await res.json()

      if (res.ok) {
        setSubmitResult({
          success: true,
          message: `"${form.name}" v${form.version} registered successfully in Azure ML workspace.`,
          modelId: data.marketplaceEntry?.id,
          amlConfigured: true,
        })
      } else if (res.status === 503) {
        // AML not configured — show a "preview" confirmation
        setSubmitResult({
          success: true,
          message: `Model metadata captured. Azure ML workspace not yet configured — connect AZURE_ML_WORKSPACE to persist registration.`,
          amlConfigured: false,
        })
      } else {
        setSubmitResult({
          success: false,
          message: data.error ?? "Registration failed. Please try again.",
          amlConfigured: true,
        })
      }
    } catch {
      setSubmitResult({
        success: false,
        message: "Network error — could not reach the registration API.",
      })
    } finally {
      setIsSubmitting(false)
    }
  }

  if (submitResult?.success) {
    return (
      <div className="min-h-screen bg-background">
        <AppSidebar />
        <main className="ml-64 p-6">
          <div className="mx-auto max-w-lg pt-16 text-center">
            <div className="mx-auto mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-emerald-500/20">
              <CheckCircle2 className="h-10 w-10 text-emerald-400" />
            </div>
            <h2 className="mb-2 text-2xl font-semibold text-foreground">
              {submitResult.amlConfigured === false ? "Request Captured" : "Model Registered"}
            </h2>
            <p className="mb-6 text-sm text-muted-foreground">{submitResult.message}</p>

            {!submitResult.amlConfigured && (
              <div className="mb-6 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-left text-sm">
                <p className="mb-2 font-medium text-amber-400">To enable live registration:</p>
                <ol className="list-decimal space-y-1 pl-4 text-amber-300/80">
                  <li>Create an Azure Machine Learning workspace in your subscription</li>
                  <li>
                    Set <code className="rounded bg-card px-1">AZURE_ML_WORKSPACE</code>,{" "}
                    <code className="rounded bg-card px-1">AZURE_ML_RESOURCE_GROUP</code>, and{" "}
                    <code className="rounded bg-card px-1">AZURE_SUBSCRIPTION_ID</code> as Container
                    App secrets
                  </li>
                  <li>Redeploy — the registration API will persist models to the AML registry</li>
                </ol>
              </div>
            )}

            <div className="flex justify-center gap-3">
              <Button
                variant="outline"
                onClick={() => {
                  setForm(defaultForm)
                  setSubmitResult(null)
                  setStep(1)
                }}
              >
                Register Another
              </Button>
              <Button
                className="bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white"
                onClick={() => router.push("/models")}
              >
                View Model Catalog
              </Button>
            </div>
          </div>
        </main>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />

      <main className="ml-64 p-6">
        {/* Breadcrumb */}
        <Link
          href="/models"
          className="mb-6 inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Model Marketplace
        </Link>

        <div className="mx-auto max-w-2xl">
          {/* Header */}
          <div className="mb-8">
            <h1 className="text-2xl font-semibold text-foreground">Register Custom Model (BYOM)</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Bring your own model from Azure ML, HuggingFace, or any artifact store into the AI
              Asset Marketplace for governed reuse across teams.
            </p>
          </div>

          {/* Integration callout */}
          <div className="mb-6 flex items-start gap-3 rounded-lg border border-[var(--uhg-blue)]/30 bg-[var(--uhg-blue)]/5 p-4">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--uhg-blue-light)]" />
            <div className="text-sm">
              <p className="font-medium text-foreground">Azure ML Model Registry Integration</p>
              <p className="mt-0.5 text-muted-foreground">
                Registered models are stored in your Azure ML workspace and surfaced here in the
                marketplace. Configure{" "}
                <code className="rounded bg-secondary px-1.5 py-0.5">AZURE_ML_WORKSPACE</code>,{" "}
                <code className="rounded bg-secondary px-1.5 py-0.5">AZURE_ML_RESOURCE_GROUP</code>, and{" "}
                <code className="rounded bg-secondary px-1.5 py-0.5">AZURE_SUBSCRIPTION_ID</code> as
                Container App environment variables to enable live registration.
              </p>
              <a
                href="https://learn.microsoft.com/azure/machine-learning/how-to-manage-models"
                target="_blank"
                rel="noopener noreferrer"
                className="mt-1 inline-flex items-center gap-1 text-[var(--uhg-blue-light)] hover:underline"
              >
                Azure ML Model Registry docs <ExternalLink className="h-3 w-3" />
              </a>
            </div>
          </div>

          {/* Step indicators */}
          <div className="mb-8 flex items-center gap-2">
            {STEPS.map((s, i) => (
              <div key={s.id} className="flex items-center gap-2">
                <div
                  className={cn(
                    "flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-colors",
                    step > s.id
                      ? "border-transparent bg-emerald-500/80 text-white"
                      : step === s.id
                      ? "border-[var(--optum-orange)] bg-[var(--optum-orange)]/10 text-[var(--optum-orange)]"
                      : "border-border bg-secondary/50 text-muted-foreground"
                  )}
                >
                  {step > s.id ? <Check className="h-4 w-4" /> : s.id}
                </div>
                <span
                  className={cn(
                    "text-sm",
                    step === s.id ? "font-medium text-foreground" : "text-muted-foreground"
                  )}
                >
                  {s.label}
                </span>
                {i < STEPS.length - 1 && <div className="mx-2 h-px w-8 bg-border" />}
              </div>
            ))}
          </div>

          {/* Step 1 — Model Identity */}
          {step === 1 && (
            <div className="space-y-5 rounded-xl border border-border bg-card p-6">
              <h2 className="flex items-center gap-2 text-base font-medium text-foreground">
                <Brain className="h-5 w-5 text-[var(--optum-orange)]" />
                Model Identity
              </h2>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Model Name <span className="text-destructive">*</span>
                  </label>
                  <input
                    type="text"
                    value={form.name}
                    onChange={(e) => update("name", e.target.value)}
                    placeholder="e.g. DenialPrediction-BERT"
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/30"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    Alphanumeric, hyphens allowed. Used as the model container name in Azure ML.
                  </p>
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Version <span className="text-destructive">*</span>
                  </label>
                  <input
                    type="text"
                    value={form.version}
                    onChange={(e) => update("version", e.target.value)}
                    placeholder="1"
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">Integer or semver (e.g. 1, 2, 1.0.0)</p>
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">Model Type</label>
                  <select
                    value={form.type}
                    onChange={(e) => update("type", e.target.value)}
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  >
                    <option value="internal">Internal (Optum built)</option>
                    <option value="partner">Partner model</option>
                    <option value="custom">Custom / BYOM</option>
                  </select>
                </div>

                <div className="sm:col-span-2">
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Description <span className="text-destructive">*</span>
                  </label>
                  <textarea
                    value={form.description}
                    onChange={(e) => update("description", e.target.value)}
                    rows={3}
                    placeholder="Describe what the model does, its training data, and intended use…"
                    className="w-full rounded-lg border border-border bg-background px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  />
                </div>
              </div>

              <div className="flex justify-end">
                <Button
                  disabled={!step1Valid}
                  onClick={() => setStep(2)}
                  className="bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white"
                >
                  Next: Artifact & Source →
                </Button>
              </div>
            </div>
          )}

          {/* Step 2 — Artifact & Source */}
          {step === 2 && (
            <div className="space-y-5 rounded-xl border border-border bg-card p-6">
              <h2 className="flex items-center gap-2 text-base font-medium text-foreground">
                <Server className="h-5 w-5 text-[var(--optum-orange)]" />
                Artifact & Source
              </h2>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Model Artifact URI <span className="text-destructive">*</span>
                  </label>
                  <div className="relative">
                    <Upload className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <input
                      type="text"
                      value={form.modelUri}
                      onChange={(e) => update("modelUri", e.target.value)}
                      placeholder="azureml://subscriptions/.../datastores/workspaceblobstore/paths/models/…"
                      className="h-10 w-full rounded-lg border border-border bg-background pl-10 pr-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                    />
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Azure Blob URI, AzureML datastore path, HuggingFace model ID (
                    <code>huggingface://models/org/model</code>), or MLflow model URI.
                  </p>
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Framework <span className="text-destructive">*</span>
                  </label>
                  <select
                    value={form.framework}
                    onChange={(e) => update("framework", e.target.value)}
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  >
                    <option value="">Select framework…</option>
                    {FRAMEWORKS.map((f) => (
                      <option key={f} value={f}>{f}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    Task Type <span className="text-destructive">*</span>
                  </label>
                  <select
                    value={form.taskType}
                    onChange={(e) => update("taskType", e.target.value)}
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  >
                    <option value="">Select task type…</option>
                    {TASK_TYPES.map((t) => (
                      <option key={t} value={t}>{t}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">Category</label>
                  <select
                    value={form.category}
                    onChange={(e) => update("category", e.target.value)}
                    className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                  >
                    {CATEGORIES.map((c) => (
                      <option key={c} value={c}>{c}</option>
                    ))}
                  </select>
                </div>
              </div>

              {/* AzureML info panel */}
              <div className="rounded-lg border border-border bg-secondary/30 p-4 text-sm">
                <p className="mb-2 font-medium text-foreground">Supported artifact sources</p>
                <ul className="space-y-1 text-muted-foreground">
                  <li className="flex items-start gap-2">
                    <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                    <span><strong>Azure ML Datastore:</strong> <code className="text-xs">azureml://subscriptions/…/datastores/…/paths/…</code></span>
                  </li>
                  <li className="flex items-start gap-2">
                    <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                    <span><strong>Azure Blob Storage:</strong> <code className="text-xs">https://&lt;storage&gt;.blob.core.windows.net/…</code></span>
                  </li>
                  <li className="flex items-start gap-2">
                    <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                    <span><strong>HuggingFace Hub:</strong> <code className="text-xs">huggingface://models/&lt;org&gt;/&lt;model&gt;</code></span>
                  </li>
                  <li className="flex items-start gap-2">
                    <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                    <span><strong>MLflow:</strong> <code className="text-xs">mlflow-model://…</code></span>
                  </li>
                </ul>
              </div>

              <div className="flex justify-between">
                <Button variant="outline" onClick={() => setStep(1)}>
                  ← Back
                </Button>
                <Button
                  disabled={!step2Valid}
                  onClick={() => setStep(3)}
                  className="bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white"
                >
                  Next: Governance →
                </Button>
              </div>
            </div>
          )}

          {/* Step 3 — Governance */}
          {step === 3 && (
            <div className="space-y-5 rounded-xl border border-border bg-card p-6">
              <h2 className="flex items-center gap-2 text-base font-medium text-foreground">
                <Shield className="h-5 w-5 text-[var(--optum-orange)]" />
                Governance & Metadata
              </h2>

              {/* Compliance */}
              <div>
                <label className="mb-2 block text-sm font-medium text-foreground">
                  Compliance Certifications
                </label>
                <div className="flex flex-wrap gap-2">
                  {COMPLIANCE_OPTIONS.map((badge) => (
                    <button
                      key={badge}
                      type="button"
                      onClick={() => toggleCompliance(badge)}
                      className={cn(
                        "flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors",
                        form.compliance.includes(badge)
                          ? "border-[var(--optum-orange)] bg-[var(--optum-orange)]/10 text-[var(--optum-orange)]"
                          : "border-border bg-secondary/50 text-muted-foreground hover:border-border/80 hover:text-foreground"
                      )}
                    >
                      {form.compliance.includes(badge) && <Check className="h-3 w-3" />}
                      {badge}
                    </button>
                  ))}
                </div>
                <p className="mt-1.5 text-xs text-muted-foreground">
                  Select all certifications that apply. These are stored as tags in Azure ML and shown
                  on the marketplace model card.
                </p>
              </div>

              {/* Tags */}
              <div>
                <label className="mb-1.5 flex items-center gap-2 text-sm font-medium text-foreground">
                  <Tag className="h-4 w-4" />
                  Tags
                </label>
                <input
                  type="text"
                  value={form.tags}
                  onChange={(e) => update("tags", e.target.value)}
                  placeholder="team:rcm, usecase:denial, env:production"
                  className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  Comma-separated key:value pairs. Stored as AML model tags.
                </p>
              </div>

              {/* Additional notes */}
              <div>
                <label className="mb-1.5 block text-sm font-medium text-foreground">
                  Additional Notes (optional)
                </label>
                <textarea
                  value={form.additionalInfo}
                  onChange={(e) => update("additionalInfo", e.target.value)}
                  rows={3}
                  placeholder="Known limitations, performance caveats, training methodology…"
                  className="w-full rounded-lg border border-border bg-background px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none"
                />
              </div>

              {/* Summary */}
              <div className="rounded-lg border border-border bg-secondary/30 p-4 text-sm">
                <p className="mb-3 font-medium text-foreground">Registration Summary</p>
                <dl className="grid grid-cols-2 gap-x-4 gap-y-2">
                  <dt className="text-muted-foreground">Model Name</dt>
                  <dd className="truncate font-medium text-foreground">{form.name}</dd>
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="text-foreground">{form.version}</dd>
                  <dt className="text-muted-foreground">Framework</dt>
                  <dd className="text-foreground">{form.framework}</dd>
                  <dt className="text-muted-foreground">Task Type</dt>
                  <dd className="text-foreground">{form.taskType}</dd>
                  <dt className="text-muted-foreground">Type</dt>
                  <dd className="capitalize text-foreground">{form.type}</dd>
                  <dt className="text-muted-foreground">Compliance</dt>
                  <dd className="text-foreground">
                    {form.compliance.length ? form.compliance.join(", ") : "None specified"}
                  </dd>
                </dl>
              </div>

              {submitResult && !submitResult.success && (
                <div className="flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                  <p className="text-sm text-destructive">{submitResult.message}</p>
                </div>
              )}

              <div className="flex justify-between">
                <Button variant="outline" onClick={() => setStep(2)}>
                  ← Back
                </Button>
                <Button
                  disabled={!canSubmit || isSubmitting}
                  onClick={handleSubmit}
                  className="gap-2 bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white"
                >
                  {isSubmitting ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Registering…
                    </>
                  ) : (
                    <>
                      <Shield className="h-4 w-4" />
                      Register Model
                    </>
                  )}
                </Button>
              </div>
            </div>
          )}
        </div>
      </main>
    </div>
  )
}
