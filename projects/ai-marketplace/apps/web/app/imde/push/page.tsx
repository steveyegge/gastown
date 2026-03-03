"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import {
  Upload,
  CheckCircle2,
  Circle,
  ArrowRight,
  Brain,
  FlaskConical,
  Shield,
  Package,
  Terminal,
  Clock,
  Zap,
  ChevronRight,
  Copy,
  ExternalLink,
  Star,
  BarChart3,
  Rocket,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

interface PublishStep {
  id: number
  label: string
  description: string
  icon: React.ElementType
  status: "completed" | "active" | "pending"
}

interface RecentPush {
  id: string
  modelName: string
  version: string
  pushedBy: string
  pushedAt: string
  status: "published" | "under-review" | "rejected"
  marketplaceUrl: string
  evals: { accuracy: string; f1: string; latency: string }
}

const recentPushes: RecentPush[] = [
  {
    id: "push-001",
    modelName: "Denial Predictor v2.1",
    version: "v2.1.0",
    pushedBy: "Sarah Chen",
    pushedAt: "Feb 25, 2026",
    status: "published",
    marketplaceUrl: "/models/denial-predictor",
    evals: { accuracy: "92.3%", f1: "91.8%", latency: "48ms" },
  },
  {
    id: "push-002",
    modelName: "ICD-10 AutoCoder v1.4",
    version: "v1.4.2",
    pushedBy: "James Rivera",
    pushedAt: "Feb 18, 2026",
    status: "under-review",
    marketplaceUrl: "/models/icd10-autocoder",
    evals: { accuracy: "89.1%", f1: "88.7%", latency: "210ms" },
  },
  {
    id: "push-003",
    modelName: "Auth Approval Scorer v3.0",
    version: "v3.0.0",
    pushedBy: "Amy Kowalski",
    pushedAt: "Jan 30, 2026",
    status: "published",
    marketplaceUrl: "/models/auth-scorer",
    evals: { accuracy: "88.4%", f1: "88.3%", latency: "3ms" },
  },
]

export default function IMDEPushPage() {
  const [step, setStep] = useState(1)
  const [formData, setFormData] = useState({
    modelName: "",
    version: "",
    description: "",
    sourceRun: "",
    category: "",
    tags: "",
    owner: "",
    environment: "",
  })
  const [publishing, setPublishing] = useState(false)
  const [published, setPublished] = useState(false)

  const steps: PublishStep[] = [
    { id: 1, label: "Select Run", description: "Choose the experiment run to publish", icon: FlaskConical, status: step > 1 ? "completed" : step === 1 ? "active" : "pending" },
    { id: 2, label: "Model Info", description: "Name, version, description & tags", icon: Brain, status: step > 2 ? "completed" : step === 2 ? "active" : "pending" },
    { id: 3, label: "Evaluation", description: "Automated eval suite runs here", icon: BarChart3, status: step > 3 ? "completed" : step === 3 ? "active" : "pending" },
    { id: 4, label: "Governance", description: "Compliance checks & approval gates", icon: Shield, status: step > 4 ? "completed" : step === 4 ? "active" : "pending" },
    { id: 5, label: "Publish", description: "One-click deploy to Model Marketplace", icon: Rocket, status: published ? "completed" : step === 5 ? "active" : "pending" },
  ]

  const handlePublish = async () => {
    setPublishing(true)
    await new Promise((r) => setTimeout(r, 2000))
    setPublishing(false)
    setPublished(true)
  }

  return (
    <div className="flex min-h-screen bg-background">
      <AppSidebar />
      <div className="ml-64 flex-1 p-6">
        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-violet-500/20">
              <Upload className="h-5 w-5 text-violet-400" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-foreground">Push to Marketplace</h1>
              <p className="text-sm text-muted-foreground">Publish a model from IMDE to the Model Marketplace with one command</p>
            </div>
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-border bg-secondary/30 px-3 py-2">
            <Terminal className="h-4 w-4 text-amber-400" />
            <code className="text-xs font-mono text-amber-300">imde push --run run-001 --to marketplace</code>
            <Button variant="ghost" size="icon" className="h-6 w-6 ml-1">
              <Copy className="h-3.5 w-3.5 text-muted-foreground" />
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-3 gap-6">
          {/* Left: Wizard */}
          <div className="col-span-2 space-y-6">
            {/* Stepper */}
            <Card className="border-border">
              <CardContent className="p-6">
                <div className="flex items-center gap-2">
                  {steps.map((s, i) => (
                    <div key={s.id} className="flex items-center gap-2 flex-1">
                      <button
                        className={cn(
                          "flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-medium transition-all w-full",
                          s.status === "completed" ? "text-emerald-400" :
                          s.status === "active" ? "bg-violet-500/10 text-violet-300" :
                          "text-muted-foreground"
                        )}
                        onClick={() => s.status !== "pending" && setStep(s.id)}
                      >
                        {s.status === "completed" ? (
                          <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
                        ) : (
                          <div className={cn(
                            "h-5 w-5 rounded-full border-2 flex items-center justify-center text-[10px] font-bold shrink-0",
                            s.status === "active" ? "border-violet-400 text-violet-400" : "border-muted-foreground/30 text-muted-foreground/30"
                          )}>
                            {s.id}
                          </div>
                        )}
                        <span className="hidden xl:block">{s.label}</span>
                      </button>
                      {i < steps.length - 1 && (
                        <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground/30" />
                      )}
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Step Content */}
            {step === 1 && (
              <Card className="border-border">
                <CardHeader>
                  <CardTitle className="text-base flex items-center gap-2">
                    <FlaskConical className="h-4 w-4 text-violet-400" />
                    Select Experiment Run
                  </CardTitle>
                  <CardDescription>Choose which run to package as a model artifact</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {[
                    { id: "run-001", name: "denial-bert-lr1e4-bs32", model: "bert-base-uncased", f1: "91.8%", accuracy: "92.3%", latency: "48ms", highlighted: true },
                    { id: "run-005", name: "denial-llama31-lora-r16", model: "Llama 3.1 8B (LoRA)", f1: "93.1%", accuracy: "93.4%", latency: "210ms", highlighted: false },
                    { id: "run-003", name: "xgboost-depth6-est500", model: "XGBoost", f1: "87.6%", accuracy: "88.4%", latency: "3ms", highlighted: false },
                    { id: "run-004", name: "lgbm-leaves64", model: "LightGBM", f1: "88.3%", accuracy: "89.1%", latency: "2ms", highlighted: false },
                  ].map((run) => (
                    <button
                      key={run.id}
                      onClick={() => setFormData((f) => ({ ...f, sourceRun: run.id }))}
                      className={cn(
                        "w-full rounded-lg border p-4 text-left transition-all hover:border-violet-500/50",
                        formData.sourceRun === run.id ? "border-violet-500/60 bg-violet-500/10" : "border-border"
                      )}
                    >
                      <div className="flex items-center justify-between mb-1">
                        <span className="font-mono text-sm font-medium">{run.name}</span>
                        {run.highlighted && <Badge className="bg-amber-500/20 text-amber-400 text-xs">Best F1</Badge>}
                        {formData.sourceRun === run.id && <CheckCircle2 className="h-4 w-4 text-violet-400" />}
                      </div>
                      <span className="text-xs text-muted-foreground">{run.model}</span>
                      <div className="mt-2 flex gap-4 text-xs">
                        <span>F1: <strong className="text-foreground">{run.f1}</strong></span>
                        <span>Acc: <strong className="text-foreground">{run.accuracy}</strong></span>
                        <span>Latency: <strong className="text-foreground">{run.latency}</strong></span>
                      </div>
                    </button>
                  ))}
                  <Button
                    className="w-full gap-2 bg-violet-600 hover:bg-violet-700 text-white mt-2"
                    disabled={!formData.sourceRun}
                    onClick={() => setStep(2)}
                  >
                    Continue <ArrowRight className="h-4 w-4" />
                  </Button>
                </CardContent>
              </Card>
            )}

            {step === 2 && (
              <Card className="border-border">
                <CardHeader>
                  <CardTitle className="text-base flex items-center gap-2">
                    <Brain className="h-4 w-4 text-violet-400" />
                    Model Information
                  </CardTitle>
                  <CardDescription>Marketplace listing details</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label className="text-xs">Model Name</Label>
                      <Input
                        placeholder="e.g. Denial Predictor"
                        value={formData.modelName}
                        onChange={(e) => setFormData((f) => ({ ...f, modelName: e.target.value }))}
                        className="h-9"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label className="text-xs">Version</Label>
                      <Input
                        placeholder="e.g. v2.1.0"
                        value={formData.version}
                        onChange={(e) => setFormData((f) => ({ ...f, version: e.target.value }))}
                        className="h-9"
                      />
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-xs">Description</Label>
                    <Textarea
                      placeholder="Describe what this model does, how it was trained, and intended use cases..."
                      value={formData.description}
                      onChange={(e) => setFormData((f) => ({ ...f, description: e.target.value }))}
                      rows={3}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label className="text-xs">Category</Label>
                      <Select onValueChange={(v) => setFormData((f) => ({ ...f, category: v }))}>
                        <SelectTrigger className="h-9">
                          <SelectValue placeholder="Select category" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="denial-management">Denial Management</SelectItem>
                          <SelectItem value="coding">Medical Coding</SelectItem>
                          <SelectItem value="auth">Prior Authorization</SelectItem>
                          <SelectItem value="billing">Billing & Claims</SelectItem>
                          <SelectItem value="forecasting">Forecasting & Analytics</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-1.5">
                      <Label className="text-xs">Deployment Target</Label>
                      <Select onValueChange={(v) => setFormData((f) => ({ ...f, environment: v }))}>
                        <SelectTrigger className="h-9">
                          <SelectValue placeholder="Select target" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="staging">Staging only</SelectItem>
                          <SelectItem value="production">Production-ready</SelectItem>
                          <SelectItem value="evaluation">Evaluation only</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-xs">Tags (comma-separated)</Label>
                    <Input
                      placeholder="denials, transformer, bert, RCM"
                      value={formData.tags}
                      onChange={(e) => setFormData((f) => ({ ...f, tags: e.target.value }))}
                      className="h-9"
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" className="flex-1" onClick={() => setStep(1)}>Back</Button>
                    <Button className="flex-1 gap-2 bg-violet-600 hover:bg-violet-700 text-white" onClick={() => setStep(3)}>
                      Continue <ArrowRight className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}

            {step === 3 && (
              <Card className="border-border">
                <CardHeader>
                  <CardTitle className="text-base flex items-center gap-2">
                    <BarChart3 className="h-4 w-4 text-violet-400" />
                    Automated Evaluation
                  </CardTitle>
                  <CardDescription>Standard eval suite run automatically before publishing</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {[
                    { check: "Holdout test set (10% stratified)", result: "F1: 91.8%, Acc: 92.3%", status: "pass" },
                    { check: "Fairness audit (payer / specialty / region)", result: "No significant disparity detected", status: "pass" },
                    { check: "Robustness (noisy input perturbations)", result: "±1.2% variance on noise=0.15", status: "pass" },
                    { check: "Latency SLA (p99 < 200ms)", result: "p99 = 48ms", status: "pass" },
                    { check: "Memory footprint < 2 GB", result: "Model size: 438 MB", status: "pass" },
                    { check: "SHAP explainability report", result: "Top 10 features documented", status: "pass" },
                    { check: "Data leakage scan", result: "No PHI in training artifacts", status: "pass" },
                    { check: "Adversarial input test", result: "Reviewed — 2 edge cases flagged (non-blocking)", status: "warn" },
                  ].map((item) => (
                    <div key={item.check} className="flex items-center gap-3 rounded-lg border border-border/50 bg-secondary/20 p-3">
                      {item.status === "pass" ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
                      ) : (
                        <Shield className="h-4 w-4 text-amber-400 shrink-0" />
                      )}
                      <div className="flex-1">
                        <div className="text-xs font-medium">{item.check}</div>
                        <div className="text-xs text-muted-foreground">{item.result}</div>
                      </div>
                      <Badge className={item.status === "pass" ? "bg-emerald-500/20 text-emerald-400" : "bg-amber-500/20 text-amber-400"}>
                        {item.status === "pass" ? "PASS" : "WARN"}
                      </Badge>
                    </div>
                  ))}
                  <div className="flex gap-2 mt-4">
                    <Button variant="outline" className="flex-1" onClick={() => setStep(2)}>Back</Button>
                    <Button className="flex-1 gap-2 bg-violet-600 hover:bg-violet-700 text-white" onClick={() => setStep(4)}>
                      All checks OK — Continue <ArrowRight className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}

            {step === 4 && (
              <Card className="border-border">
                <CardHeader>
                  <CardTitle className="text-base flex items-center gap-2">
                    <Shield className="h-4 w-4 text-violet-400" />
                    Governance Sign-off
                  </CardTitle>
                  <CardDescription>Required approvals before marketplace publication</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {[
                    { role: "Model Owner", person: "Dr. Sarah Chen", status: "approved", note: "Approved Feb 28, 2026" },
                    { role: "Data Privacy Officer", person: "Linda Park", status: "approved", note: "PHI review passed, HIPAA compliant" },
                    { role: "Clinical Domain Expert", person: "Dr. Raj Gupta (RCM)", status: "approved", note: "Feature set aligns with denial patterns" },
                    { role: "MLOps Engineer", person: "Kevin Wu", status: "pending", note: "Awaiting infra review (ETA: today)" },
                  ].map((item) => (
                    <div key={item.role} className="flex items-center gap-3 rounded-lg border border-border p-3">
                      <div className={cn(
                        "h-8 w-8 rounded-full flex items-center justify-center text-xs font-bold shrink-0",
                        item.status === "approved" ? "bg-emerald-500/20 text-emerald-400" : "bg-amber-500/20 text-amber-400"
                      )}>
                        {item.person[0]}
                      </div>
                      <div className="flex-1">
                        <div className="text-xs font-medium">{item.role}</div>
                        <div className="text-xs text-muted-foreground">{item.person} · {item.note}</div>
                      </div>
                      <Badge className={item.status === "approved" ? "bg-emerald-500/20 text-emerald-400" : "bg-amber-500/20 text-amber-400"}>
                        {item.status}
                      </Badge>
                    </div>
                  ))}
                  <div className="flex gap-2 mt-4">
                    <Button variant="outline" className="flex-1" onClick={() => setStep(3)}>Back</Button>
                    <Button className="flex-1 gap-2 bg-amber-600 hover:bg-amber-700 text-white" onClick={() => setStep(5)}>
                      3 of 4 approved — Continue <ArrowRight className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}

            {step === 5 && (
              <Card className={cn("border-border", published && "border-emerald-500/40")}>
                <CardHeader>
                  <CardTitle className="text-base flex items-center gap-2">
                    <Rocket className={cn("h-4 w-4", published ? "text-emerald-400" : "text-violet-400")} />
                    Publish to Model Marketplace
                  </CardTitle>
                  <CardDescription>
                    {published ? "Successfully published! Your model is live." : "Final step — publish your model to the marketplace"}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {published ? (
                    <div className="space-y-4">
                      <div className="flex items-center gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4">
                        <CheckCircle2 className="h-8 w-8 text-emerald-400 shrink-0" />
                        <div>
                          <div className="font-semibold text-emerald-300">Model published successfully!</div>
                          <div className="text-xs text-muted-foreground mt-0.5">
                            {formData.modelName || "Denial Predictor"} {formData.version || "v2.1.0"} is now live on the Model Marketplace
                          </div>
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <Button variant="outline" className="flex-1 gap-2">
                          <ExternalLink className="h-4 w-4" />
                          View in Marketplace
                        </Button>
                        <Button variant="outline" className="flex-1 gap-2" onClick={() => { setStep(1); setPublished(false); setFormData({ modelName: "", version: "", description: "", sourceRun: "", category: "", tags: "", owner: "", environment: "" }) }}>
                          <Upload className="h-4 w-4" />
                          Publish Another
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="space-y-4">
                      <div className="rounded-lg border border-border bg-secondary/20 p-4 font-mono text-xs space-y-1.5">
                        <div className="text-muted-foreground"># Publishing model artifact...</div>
                        <div className="text-emerald-400">✓ Packaging model weights + config ({formData.sourceRun || "run-001"})</div>
                        <div className="text-emerald-400">✓ Generating model card (README + eval report)</div>
                        <div className="text-emerald-400">✓ Pushing to ACR: aimktacrp7a65r22.azurecr.io/models/</div>
                        <div className="text-emerald-400">✓ Registering in Model Marketplace (Azure ML backend)</div>
                        <div className="text-amber-400">⏳ Registering governance metadata...</div>
                      </div>
                      <div className="flex gap-2">
                        <Button variant="outline" className="flex-1" onClick={() => setStep(4)}>Back</Button>
                        <Button
                          className="flex-1 gap-2 bg-emerald-600 hover:bg-emerald-700 text-white"
                          disabled={publishing}
                          onClick={handlePublish}
                        >
                          {publishing ? (
                            <><Clock className="h-4 w-4 animate-spin" /> Publishing...</>
                          ) : (
                            <><Rocket className="h-4 w-4" /> Publish to Marketplace</>
                          )}
                        </Button>
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
          </div>

          {/* Right: Recent Pushes */}
          <div className="space-y-4">
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm flex items-center gap-2">
                  <Package className="h-4 w-4 text-violet-400" />
                  Recent Publishes
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {recentPushes.map((p) => (
                  <div key={p.id} className="rounded-lg border border-border/50 bg-secondary/20 p-3 space-y-2">
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <div className="text-xs font-medium">{p.modelName}</div>
                        <div className="text-[10px] text-muted-foreground">{p.version} · {p.pushedBy}</div>
                      </div>
                      <Badge className={cn(
                        "text-[10px]",
                        p.status === "published" ? "bg-emerald-500/20 text-emerald-400" :
                        p.status === "under-review" ? "bg-amber-500/20 text-amber-400" :
                        "bg-red-500/20 text-red-400"
                      )}>
                        {p.status}
                      </Badge>
                    </div>
                    <div className="flex gap-3 text-[10px] text-muted-foreground">
                      <span>Acc: {p.evals.accuracy}</span>
                      <span>F1: {p.evals.f1}</span>
                      <span>⚡ {p.evals.latency}</span>
                    </div>
                    <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                      <span>{p.pushedAt}</span>
                      <Button variant="ghost" size="sm" className="h-5 gap-1 text-[10px] px-1.5">
                        <ExternalLink className="h-3 w-3" /> View
                      </Button>
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card className="border-violet-500/30 bg-violet-500/5">
              <CardContent className="p-4 space-y-2">
                <div className="flex items-center gap-2 mb-2">
                  <Terminal className="h-4 w-4 text-violet-400" />
                  <span className="text-xs font-semibold text-violet-400">CLI Quick Push</span>
                </div>
                <p className="text-xs text-muted-foreground mb-3">
                  From within your sandbox terminal, publish directly with the IMDE CLI:
                </p>
                <div className="rounded bg-background/50 border border-border p-3 font-mono text-[10px] space-y-1 text-emerald-400/90">
                  <div># Auto-select best run and push</div>
                  <div className="text-violet-300">imde push --auto --to marketplace</div>
                  <div className="mt-2"># Push specific run</div>
                  <div className="text-violet-300">imde push run-001 \</div>
                  <div className="text-violet-300 pl-4">--name "Denial Predictor" \</div>
                  <div className="text-violet-300 pl-4">--version v2.1.0 \</div>
                  <div className="text-violet-300 pl-4">--to marketplace</div>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  )
}
