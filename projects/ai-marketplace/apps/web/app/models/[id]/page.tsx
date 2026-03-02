"use client"

import { use, useState } from "react"
import Link from "next/link"
import { notFound } from "next/navigation"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { models } from "@/lib/models-data"
import { cn } from "@/lib/utils"
import {
  ArrowLeft,
  Brain,
  CheckCircle2,
  Shield,
  Star,
  Users,
  Clock,
  Download,
  ExternalLink,
  Copy,
  Check,
  Zap,
  BarChart3,
  Code2,
  FileText,
  Activity,
  Cpu,
  Globe,
  AlertCircle,
} from "lucide-react"
import { Button } from "@/components/ui/button"

const statusColors: Record<string, string> = {
  production: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  beta: "bg-amber-500/20 text-amber-400 border-amber-500/30",
  review: "bg-blue-500/20 text-blue-400 border-blue-500/30",
}

const typeColors: Record<string, string> = {
  internal: "bg-cyan-500/20 text-cyan-400",
  partner: "bg-purple-500/20 text-purple-400",
}

type Tab = "overview" | "model-card" | "api" | "changelog"

export default function ModelDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const model = models.find((m) => m.id === id)
  const [activeTab, setActiveTab] = useState<Tab>("overview")
  const [copied, setCopied] = useState(false)

  if (!model) {
    notFound()
  }

  const relatedModels = models
    .filter((m) => m.category === model.category && m.id !== model.id)
    .slice(0, 3)

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const tabs: { id: Tab; label: string; icon: React.ElementType }[] = [
    { id: "overview", label: "Overview", icon: FileText },
    { id: "model-card", label: "Model Card", icon: BarChart3 },
    { id: "api", label: "API / SDK", icon: Code2 },
    { id: "changelog", label: "Changelog", icon: Activity },
  ]

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

        <div className="grid gap-6 lg:grid-cols-3">
          {/* Main content */}
          <div className="lg:col-span-2 space-y-6">
            {/* Header card */}
            <div className="rounded-xl border border-border bg-card p-6">
              <div className="flex items-start gap-4">
                <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-[var(--optum-orange)]/20 to-[var(--uhg-blue)]/20">
                  <Brain className="h-8 w-8 text-[var(--optum-orange)]" />
                </div>
                <div className="flex-1">
                  <div className="mb-1 flex flex-wrap items-center gap-2">
                    <h1 className="text-2xl font-semibold text-foreground">{model.name}</h1>
                    <span className={cn("rounded-full border px-2.5 py-0.5 text-xs font-medium", statusColors[model.status])}>
                      {model.status}
                    </span>
                    <span className={cn("rounded px-2 py-0.5 text-xs font-medium", typeColors[model.type])}>
                      {model.type}
                    </span>
                  </div>
                  <p className="mb-2 text-sm text-muted-foreground">
                    {model.version} · by{" "}
                    <span className="text-foreground">{model.publisher}</span>
                    {model.publisherVerified && (
                      <CheckCircle2 className="ml-1 inline h-3.5 w-3.5 text-[var(--optum-orange)]" />
                    )}
                  </p>
                  <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
                    <span className="flex items-center gap-1">
                      {[1, 2, 3, 4, 5].map((i) => (
                        <Star
                          key={i}
                          className={cn("h-3.5 w-3.5", i <= Math.floor(model.rating) ? "fill-amber-400 text-amber-400" : "fill-muted/30 text-muted/30")}
                        />
                      ))}
                      <span className="ml-1">{model.rating}</span>
                    </span>
                    <span className="flex items-center gap-1">
                      <Download className="h-3.5 w-3.5" />
                      {model.downloads.toLocaleString()} downloads
                    </span>
                    <span className="flex items-center gap-1">
                      <Users className="h-3.5 w-3.5" />
                      {model.teams} teams
                    </span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      Updated{" "}
                      {new Date(model.lastUpdated).toLocaleDateString("en-US", {
                        month: "short",
                        day: "numeric",
                        year: "numeric",
                      })}
                    </span>
                  </div>
                </div>
              </div>

              {/* Compliance badges */}
              <div className="mt-4 flex flex-wrap gap-2">
                {model.compliance.map((badge) => (
                  <span
                    key={badge}
                    className="flex items-center gap-1 rounded-full border border-border bg-secondary/50 px-3 py-0.5 text-xs text-muted-foreground"
                  >
                    <Shield className="h-3 w-3" />
                    {badge}
                  </span>
                ))}
              </div>
            </div>

            {/* Tabs */}
            <div className="rounded-xl border border-border bg-card">
              <div className="flex border-b border-border">
                {tabs.map((tab) => (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id)}
                    className={cn(
                      "flex items-center gap-2 px-5 py-3.5 text-sm font-medium transition-colors border-b-2 -mb-px",
                      activeTab === tab.id
                        ? "border-[var(--optum-orange)] text-foreground"
                        : "border-transparent text-muted-foreground hover:text-foreground"
                    )}
                  >
                    <tab.icon className="h-4 w-4" />
                    {tab.label}
                  </button>
                ))}
              </div>

              <div className="p-6">
                {/* Overview Tab */}
                {activeTab === "overview" && (
                  <div className="space-y-6">
                    <div>
                      <h3 className="mb-2 text-base font-medium text-foreground">Description</h3>
                      <p className="text-sm leading-relaxed text-muted-foreground">{model.description}</p>
                    </div>

                    {model.useCases && (
                      <div>
                        <h3 className="mb-3 text-base font-medium text-foreground">Use Cases</h3>
                        <ul className="space-y-2">
                          {model.useCases.map((uc) => (
                            <li key={uc} className="flex items-start gap-2 text-sm text-muted-foreground">
                              <Check className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
                              {uc}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}

                    {model.tags && (
                      <div>
                        <h3 className="mb-3 text-base font-medium text-foreground">Tags</h3>
                        <div className="flex flex-wrap gap-2">
                          {model.tags.map((tag) => (
                            <span
                              key={tag}
                              className="rounded-md bg-secondary px-2.5 py-1 text-xs text-muted-foreground"
                            >
                              {tag}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}

                {/* Model Card Tab */}
                {activeTab === "model-card" && (
                  <div className="space-y-6">
                    <div className="rounded-lg border border-[var(--optum-orange)]/30 bg-[var(--optum-orange)]/5 p-4">
                      <h3 className="mb-1 text-sm font-semibold text-foreground">Model Report Card</h3>
                      <p className="text-xs text-muted-foreground">
                        Governance-approved documentation for production use. HIPAA & SOC2 reviewed.
                      </p>
                    </div>

                    {/* Performance Metrics */}
                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">Performance Benchmarks</h3>
                      <div className="grid grid-cols-3 gap-4">
                        <div className="rounded-xl border border-border bg-secondary/30 p-4 text-center">
                          <Zap className="mx-auto mb-1 h-5 w-5 text-[var(--optum-orange)]" />
                          <p className="text-2xl font-bold text-emerald-400">{model.metrics.accuracy}%</p>
                          <p className="text-xs text-muted-foreground">Accuracy</p>
                        </div>
                        <div className="rounded-xl border border-border bg-secondary/30 p-4 text-center">
                          <Activity className="mx-auto mb-1 h-5 w-5 text-[var(--uhg-blue-light)]" />
                          <p className="text-2xl font-bold text-foreground">{model.metrics.latency}ms</p>
                          <p className="text-xs text-muted-foreground">P95 Latency</p>
                        </div>
                        <div className="rounded-xl border border-border bg-secondary/30 p-4 text-center">
                          <Cpu className="mx-auto mb-1 h-5 w-5 text-[var(--optum-teal)]" />
                          <p className="text-2xl font-bold text-foreground">{model.metrics.throughput}</p>
                          <p className="text-xs text-muted-foreground">Req/min</p>
                        </div>
                      </div>
                    </div>

                    {/* Compliance */}
                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">Compliance & Certifications</h3>
                      <div className="space-y-2">
                        {model.compliance.map((badge) => (
                          <div
                            key={badge}
                            className="flex items-center justify-between rounded-lg border border-border bg-card p-3"
                          >
                            <div className="flex items-center gap-2">
                              <Shield className="h-4 w-4 text-[var(--optum-orange)]" />
                              <span className="text-sm font-medium text-foreground">{badge}</span>
                            </div>
                            <span className="flex items-center gap-1 text-xs text-emerald-400">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              Certified
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>

                    {/* Intended use */}
                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">Intended Use</h3>
                      <div className="rounded-lg border border-border bg-secondary/30 p-4 text-sm text-muted-foreground">
                        <p>
                          This model is designed for use within Optum RCM healthcare workflows. It is NOT intended
                          for use in direct clinical decision-making, emergency care, or as a standalone diagnostic tool.
                          All outputs should be reviewed by qualified healthcare professionals.
                        </p>
                      </div>
                    </div>

                    {model.status !== "production" && (
                      <div className="flex items-start gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
                        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
                        <p className="text-sm text-amber-300">
                          This model is in <strong>{model.status}</strong> status and has not completed full production certification.
                          Use in non-production environments only.
                        </p>
                      </div>
                    )}
                  </div>
                )}

                {/* API Tab */}
                {activeTab === "api" && (
                  <div className="space-y-6">
                    {model.endpoint && (
                      <div>
                        <h3 className="mb-3 text-base font-medium text-foreground">Endpoint</h3>
                        <div className="flex items-center gap-2 rounded-lg border border-border bg-secondary/30 p-3">
                          <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <code className="flex-1 truncate font-mono text-sm text-foreground">{model.endpoint}</code>
                          <button
                            onClick={() => handleCopy(model.endpoint!)}
                            className="text-muted-foreground hover:text-foreground transition-colors"
                          >
                            {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
                          </button>
                        </div>
                      </div>
                    )}

                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">Python</h3>
                      <div className="rounded-lg bg-secondary p-4 font-mono text-sm">
                        <pre className="text-muted-foreground whitespace-pre-wrap">{`from azure.ai.foundry import FoundryClient

client = FoundryClient(
    endpoint="${model.endpoint ?? "https://aimarket-hub.azure.com"}",
    credential=DefaultAzureCredential()
)

response = client.models.invoke(
    model_id="${model.id}",
    input={"text": "Patient presented with..."},
)
print(response.result)`}</pre>
                      </div>
                    </div>

                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">TypeScript / Node.js</h3>
                      <div className="rounded-lg bg-secondary p-4 font-mono text-sm">
                        <pre className="text-muted-foreground whitespace-pre-wrap">{`import { FoundryClient } from "@azure/ai-foundry";

const client = new FoundryClient({
  endpoint: "${model.endpoint ?? "https://aimarket-hub.azure.com"}",
});

const response = await client.models.invoke({
  modelId: "${model.id}",
  input: { text: "Patient presented with..." },
});
console.log(response.result);`}</pre>
                      </div>
                    </div>

                    <div>
                      <h3 className="mb-3 text-base font-medium text-foreground">cURL</h3>
                      <div className="rounded-lg bg-secondary p-4 font-mono text-sm">
                        <pre className="text-muted-foreground whitespace-pre-wrap">{`curl -X POST \\
  "${model.endpoint ?? "https://aimarket-hub.azure.com"}/invoke" \\
  -H "Authorization: Bearer $AZURE_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"input": {"text": "Patient presented with..."}}'`}</pre>
                      </div>
                    </div>
                  </div>
                )}

                {/* Changelog Tab */}
                {activeTab === "changelog" && (
                  <div className="space-y-4">
                    <div className="rounded-lg border border-border bg-card p-4">
                      <div className="flex items-center justify-between">
                        <span className="font-medium text-foreground">{model.version}</span>
                        <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-xs font-medium text-emerald-400">Latest</span>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {new Date(model.lastUpdated).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" })}
                      </p>
                      <ul className="mt-3 space-y-1 text-sm text-muted-foreground">
                        <li className="flex items-start gap-2">
                          <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                          Performance improvements and latency optimization
                        </li>
                        <li className="flex items-start gap-2">
                          <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                          Updated compliance certifications
                        </li>
                        <li className="flex items-start gap-2">
                          <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                          Bug fixes and stability improvements
                        </li>
                      </ul>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Sidebar */}
          <div className="space-y-4">
            {/* Actions */}
            <div className="rounded-xl border border-border bg-card p-4 space-y-3">
              <Link href={`/orchestration?add=${model.id}`}>
                <Button className="w-full bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white">
                  Add to Workflow
                </Button>
              </Link>
              <Button variant="outline" className="w-full gap-2">
                <ExternalLink className="h-4 w-4" />
                View in Azure AI Foundry
              </Button>

              {model.endpoint && (
                <div className="rounded-md bg-secondary p-3">
                  <p className="mb-2 text-xs text-muted-foreground">Quick install</p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 truncate font-mono text-xs text-foreground">
                      az ai model install {model.id}
                    </code>
                    <button
                      onClick={() => handleCopy(`az ai model install ${model.id}`)}
                      className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                </div>
              )}
            </div>

            {/* Details */}
            <div className="rounded-xl border border-border bg-card p-4">
              <h3 className="mb-3 text-sm font-medium text-foreground">Details</h3>
              <dl className="space-y-2.5 text-sm">
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Category</dt>
                  <dd className="text-foreground">{model.category}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Type</dt>
                  <dd className="capitalize text-foreground">{model.type}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="text-foreground">{model.version}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Status</dt>
                  <dd className="capitalize text-foreground">{model.status}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Last Updated</dt>
                  <dd className="text-foreground">
                    {new Date(model.lastUpdated).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Active Teams</dt>
                  <dd className="text-foreground">{model.teams}</dd>
                </div>
              </dl>
            </div>

            {/* Performance summary */}
            <div className="rounded-xl border border-border bg-card p-4">
              <h3 className="mb-3 text-sm font-medium text-foreground">Performance</h3>
              <div className="space-y-3">
                <div>
                  <div className="mb-1 flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">Accuracy</span>
                    <span className="font-medium text-emerald-400">{model.metrics.accuracy}%</span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-secondary">
                    <div
                      className="h-full rounded-full bg-emerald-400"
                      style={{ width: `${model.metrics.accuracy}%` }}
                    />
                  </div>
                </div>
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground">P95 Latency</span>
                  <span className="text-foreground">{model.metrics.latency}ms</span>
                </div>
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground">Max Throughput</span>
                  <span className="text-foreground">{model.metrics.throughput} req/min</span>
                </div>
              </div>
            </div>

            {/* Related models */}
            {relatedModels.length > 0 && (
              <div className="rounded-xl border border-border bg-card p-4">
                <h3 className="mb-3 text-sm font-medium text-foreground">Related Models</h3>
                <div className="space-y-3">
                  {relatedModels.map((related) => (
                    <Link
                      key={related.id}
                      href={`/models/${related.id}`}
                      className="flex items-center gap-3 rounded-lg p-2 transition-colors hover:bg-secondary/50"
                    >
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-secondary">
                        <Brain className="h-4 w-4 text-muted-foreground" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium text-foreground">{related.name}</p>
                        <p className="text-xs text-muted-foreground">{related.publisher}</p>
                      </div>
                    </Link>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
