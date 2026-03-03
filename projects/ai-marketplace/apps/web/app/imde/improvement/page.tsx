"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import {
  RefreshCw,
  AlertTriangle,
  TrendingDown,
  TrendingUp,
  ArrowRight,
  Download,
  Play,
  CheckCircle2,
  Clock,
  Zap,
  BarChart3,
  Brain,
  Upload,
  Shield,
  Eye,
  ChevronRight,
  CircleDot,
  Database,
  GitCommit,
} from "lucide-react"
import { cn } from "@/lib/utils"

interface FailureCase {
  id: string
  claimId: string
  failureType: string
  category: string
  payer: string
  denialCode: string
  modelPrediction: string
  actualOutcome: string
  confidence: number
  impactDollars: number
  identifiedAt: string
  volume: number
}

interface RetrainJob {
  id: string
  name: string
  trigger: string
  status: "running" | "queued" | "completed" | "failed"
  progress: number
  startedAt: string
  eta?: string
  newCases: number
  baseModel: string
  expectedImprovement?: string
}

const failureCases: FailureCase[] = [
  {
    id: "fc-001",
    claimId: "CLM-2026-84512",
    failureType: "False Negative",
    category: "Payer Policy Change",
    payer: "UHC Commercial",
    denialCode: "CO-4",
    modelPrediction: "PAID",
    actualOutcome: "DENIED",
    confidence: 0.91,
    impactDollars: 12400,
    identifiedAt: "Mar 2, 2026",
    volume: 847,
  },
  {
    id: "fc-002",
    claimId: "CLM-2026-91073",
    failureType: "False Negative",
    category: "New Denial Pattern",
    payer: "BCBS Federal",
    denialCode: "PR-96",
    modelPrediction: "PAID",
    actualOutcome: "DENIED",
    confidence: 0.78,
    impactDollars: 8900,
    identifiedAt: "Mar 1, 2026",
    volume: 312,
  },
  {
    id: "fc-003",
    claimId: "CLM-2026-67241",
    failureType: "Low Confidence",
    category: "Rare CPT Code Combo",
    payer: "Aetna",
    denialCode: "CO-97",
    modelPrediction: "DENIED (51%)",
    actualOutcome: "PAID",
    confidence: 0.51,
    impactDollars: 3200,
    identifiedAt: "Feb 28, 2026",
    volume: 89,
  },
  {
    id: "fc-004",
    claimId: "CLM-2026-55830",
    failureType: "False Negative",
    category: "Multimodal Required",
    payer: "Medicare FFS",
    denialCode: "CO-16",
    modelPrediction: "PAID",
    actualOutcome: "DENIED",
    confidence: 0.84,
    impactDollars: 6700,
    identifiedAt: "Feb 27, 2026",
    volume: 156,
  },
]

const retrainJobs: RetrainJob[] = [
  {
    id: "rt-001",
    name: "denial-predictor-v2.2-patch",
    trigger: "UHC CO-4 policy change (847 new cases)",
    status: "running",
    progress: 62,
    startedAt: "Mar 2, 2026 12:00",
    eta: "~38 min remaining",
    newCases: 847,
    baseModel: "Denial Predictor v2.1",
    expectedImprovement: "+2.1% F1 on UHC segment",
  },
  {
    id: "rt-002",
    name: "denial-predictor-v2.1.1-bcbs",
    trigger: "BCBS Federal PR-96 pattern (312 cases)",
    status: "queued",
    progress: 0,
    startedAt: "—",
    newCases: 312,
    baseModel: "Denial Predictor v2.1",
    expectedImprovement: "+0.8% recall on BCBS",
  },
  {
    id: "rt-003",
    name: "denial-predictor-v2.1-hotfix",
    trigger: "Weekly drift monitor — 0.04 feature drift",
    status: "completed",
    progress: 100,
    startedAt: "Feb 28, 2026 09:00",
    newCases: 2341,
    baseModel: "Denial Predictor v2.1",
    expectedImprovement: "+0.6% overall F1",
  },
]

const driftMetrics = [
  { feature: "denial_code", drift: 0.041, threshold: 0.05, status: "warning" },
  { feature: "payer_id", drift: 0.018, threshold: 0.05, status: "ok" },
  { feature: "cpt_code_group", drift: 0.012, threshold: 0.05, status: "ok" },
  { feature: "claim_amount_bucket", drift: 0.056, threshold: 0.05, status: "alert" },
  { feature: "provider_specialty", drift: 0.009, threshold: 0.05, status: "ok" },
]

export default function IMDEImprovementPage() {
  const [pulling, setPulling] = useState(false)
  const [pulled, setPulled] = useState(false)

  const handlePull = async () => {
    setPulling(true)
    await new Promise((r) => setTimeout(r, 1800))
    setPulling(false)
    setPulled(true)
  }

  const totalImpact = failureCases.reduce((acc, f) => acc + f.impactDollars, 0)
  const totalCases = failureCases.reduce((acc, f) => acc + f.volume, 0)

  return (
    <div className="flex min-h-screen bg-background">
      <AppSidebar />
      <div className="ml-64 flex-1 p-6">
        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-violet-500/20">
              <RefreshCw className="h-5 w-5 text-violet-400" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-foreground">Continuous Improvement Loop</h1>
              <p className="text-sm text-muted-foreground">Pull production failures → retrain → push back — turning every payer change into a learning opportunity</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Badge className="bg-emerald-500/20 text-emerald-400 border-emerald-500/30">
              <CircleDot className="h-3 w-3 mr-1" /> Live Monitoring
            </Badge>
            <Button
              size="sm"
              className="gap-2 bg-violet-600 hover:bg-violet-700 text-white"
              disabled={pulling}
              onClick={handlePull}
            >
              {pulling ? (
                <><RefreshCw className="h-4 w-4 animate-spin" /> Pulling cases...</>
              ) : (
                <><Download className="h-4 w-4" /> Pull New Failures</>
              )}
            </Button>
          </div>
        </div>

        {/* Loop Diagram */}
        <div className="mb-6 flex items-center gap-0 rounded-xl border border-border bg-secondary/20 p-4">
          {[
            { icon: CircleDot, label: "Production", sub: "Live model inference", color: "text-emerald-400" },
            { icon: AlertTriangle, label: "Failure Detection", sub: "Drift + false negatives", color: "text-amber-400" },
            { icon: Download, label: "Pull Cases", sub: "Labeled failure set", color: "text-blue-400" },
            { icon: Brain, label: "Retrain", sub: "Incremental fine-tuning", color: "text-violet-400" },
            { icon: Shield, label: "Evaluate", sub: "Auto eval + governance", color: "text-orange-400" },
            { icon: Upload, label: "Push Update", sub: "Deploy to marketplace", color: "text-emerald-400" },
          ].map((step, i) => (
            <div key={step.label} className="flex items-center flex-1">
              <div className="flex-1 text-center">
                <div className={cn("flex h-10 w-10 items-center justify-center rounded-full bg-secondary mx-auto mb-1.5", step.color === "text-violet-400" && "ring-2 ring-violet-500/40")}>
                  <step.icon className={cn("h-4 w-4", step.color)} />
                </div>
                <div className="text-xs font-medium">{step.label}</div>
                <div className="text-[10px] text-muted-foreground">{step.sub}</div>
              </div>
              {i < 5 && <ChevronRight className="h-4 w-4 text-muted-foreground/40 shrink-0" />}
            </div>
          ))}
        </div>

        <div className="grid grid-cols-3 gap-6">
          {/* Main content */}
          <div className="col-span-2 space-y-6">
            {/* Impact Summary */}
            <div className="grid grid-cols-3 gap-4">
              {[
                { label: "Failure Cases Identified", value: totalCases.toLocaleString(), sub: "last 7 days", icon: AlertTriangle, color: "text-red-400" },
                { label: "Revenue at Risk", value: `$${(totalImpact / 1000).toFixed(0)}K`, sub: "recoverable with fix", icon: TrendingDown, color: "text-amber-400" },
                { label: "Retrain Jobs Active", value: "2", sub: "1 running, 1 queued", icon: RefreshCw, color: "text-violet-400" },
              ].map((s) => (
                <Card key={s.label} className="border-border">
                  <CardContent className="p-4">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs text-muted-foreground">{s.label}</span>
                      <s.icon className={cn("h-4 w-4", s.color)} />
                    </div>
                    <div className="text-2xl font-bold text-foreground">{s.value}</div>
                    <div className="text-xs text-muted-foreground">{s.sub}</div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Failure Cases */}
            {pulled && (
              <div className="rounded-lg border border-violet-500/30 bg-violet-500/5 p-4 flex items-center gap-3">
                <CheckCircle2 className="h-5 w-5 text-violet-400 shrink-0" />
                <div>
                  <div className="text-sm font-semibold text-violet-300">1,404 new failure cases pulled from production</div>
                  <div className="text-xs text-muted-foreground">Ready for review — confirm to add to retrain queue</div>
                </div>
                <Button size="sm" className="ml-auto bg-violet-600 hover:bg-violet-700 text-white gap-2">
                  <Play className="h-3.5 w-3.5" /> Trigger Retrain
                </Button>
              </div>
            )}

            <div>
              <h2 className="text-sm font-semibold mb-3 flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-400" />
                Production Failure Cases
              </h2>
              <div className="space-y-3">
                {failureCases.map((fc) => (
                  <Card key={fc.id} className="border-border hover:border-amber-500/30 transition-all">
                    <CardContent className="p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex-1">
                          <div className="flex items-center gap-2 mb-1">
                            <Badge className={cn(
                              "text-xs",
                              fc.failureType === "False Negative" ? "bg-red-500/20 text-red-400" :
                              "bg-amber-500/20 text-amber-400"
                            )}>
                              {fc.failureType}
                            </Badge>
                            <span className="text-xs font-medium">{fc.category}</span>
                            <span className="text-xs text-muted-foreground font-mono">{fc.claimId}</span>
                          </div>
                          <div className="flex items-center gap-4 text-xs text-muted-foreground">
                            <span>Payer: <strong className="text-foreground">{fc.payer}</strong></span>
                            <span>Code: <strong className="font-mono text-foreground">{fc.denialCode}</strong></span>
                            <span>Model said: <strong className="text-amber-400">{fc.modelPrediction}</strong></span>
                            <span>Actual: <strong className="text-red-400">{fc.actualOutcome}</strong></span>
                            <span>Confidence: <strong>{(fc.confidence * 100).toFixed(0)}%</strong></span>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-sm font-bold text-red-400">${fc.impactDollars.toLocaleString()}</div>
                          <div className="text-xs text-muted-foreground">{fc.volume} claims</div>
                          <Button variant="outline" size="sm" className="mt-2 h-6 text-[10px] gap-1">
                            <Play className="h-3 w-3" /> Add to Retrain
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>

            {/* Retrain Queue */}
            <div>
              <h2 className="text-sm font-semibold mb-3 flex items-center gap-2">
                <Brain className="h-4 w-4 text-violet-400" />
                Retrain Queue
              </h2>
              <div className="space-y-3">
                {retrainJobs.map((job) => (
                  <Card key={job.id} className="border-border">
                    <CardContent className="p-4">
                      <div className="flex items-start justify-between gap-3 mb-3">
                        <div>
                          <div className="flex items-center gap-2 mb-0.5">
                            <span className="font-mono text-sm font-medium">{job.name}</span>
                            <Badge className={cn(
                              "text-xs",
                              job.status === "running" ? "bg-blue-500/20 text-blue-400" :
                              job.status === "queued" ? "bg-amber-500/20 text-amber-400" :
                              job.status === "completed" ? "bg-emerald-500/20 text-emerald-400" :
                              "bg-red-500/20 text-red-400"
                            )}>
                              {job.status === "running" ? "🔄 " : ""}{job.status}
                            </Badge>
                          </div>
                          <p className="text-xs text-muted-foreground">Trigger: {job.trigger}</p>
                        </div>
                        <div className="text-right text-xs text-muted-foreground">
                          <div>{job.newCases.toLocaleString()} new cases</div>
                          <div>Base: {job.baseModel}</div>
                        </div>
                      </div>
                      {job.status === "running" && (
                        <div className="space-y-1">
                          <div className="flex justify-between text-xs">
                            <span className="text-muted-foreground">{job.eta}</span>
                            <span className="text-foreground">{job.progress}%</span>
                          </div>
                          <Progress value={job.progress} className="h-1.5" />
                        </div>
                      )}
                      {job.status === "completed" && (
                        <div className="flex items-center gap-2">
                          <CheckCircle2 className="h-4 w-4 text-emerald-400" />
                          <span className="text-xs text-emerald-400">{job.expectedImprovement} — </span>
                          <Button variant="outline" size="sm" className="h-6 text-xs gap-1.5">
                            <Upload className="h-3 w-3" /> Push Update
                          </Button>
                        </div>
                      )}
                      {job.status === "queued" && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <Clock className="h-4 w-4" />
                          <span>Waiting for compute — expected start: ~45 min</span>
                        </div>
                      )}
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>
          </div>

          {/* Right Panel */}
          <div className="space-y-4">
            {/* Feature Drift Monitor */}
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm flex items-center gap-2">
                  <BarChart3 className="h-4 w-4 text-violet-400" />
                  Feature Drift Monitor
                </CardTitle>
                <CardDescription className="text-xs">Production vs. training distribution — PSI score</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {driftMetrics.map((d) => (
                  <div key={d.feature}>
                    <div className="flex items-center justify-between mb-1 text-xs">
                      <span className={cn("font-mono", d.status === "alert" ? "text-red-400" : d.status === "warning" ? "text-amber-400" : "text-foreground")}>
                        {d.feature}
                      </span>
                      <div className="flex items-center gap-2">
                        <span className={cn("font-mono", d.status === "alert" ? "text-red-400 font-bold" : d.status === "warning" ? "text-amber-400" : "text-muted-foreground")}>
                          {d.drift.toFixed(3)}
                        </span>
                        {d.status === "alert" && <AlertTriangle className="h-3 w-3 text-red-400" />}
                        {d.status === "warning" && <AlertTriangle className="h-3 w-3 text-amber-400" />}
                        {d.status === "ok" && <CheckCircle2 className="h-3 w-3 text-emerald-400" />}
                      </div>
                    </div>
                    <div className="h-1.5 rounded-full bg-secondary overflow-hidden">
                      <div
                        className={cn(
                          "h-full rounded-full transition-all",
                          d.status === "alert" ? "bg-red-400" :
                          d.status === "warning" ? "bg-amber-400" :
                          "bg-emerald-400"
                        )}
                        style={{ width: `${Math.min((d.drift / 0.1) * 100, 100)}%` }}
                      />
                    </div>
                  </div>
                ))}
                <div className="pt-2 border-t border-border text-xs text-muted-foreground">
                  Threshold: 0.05 PSI · <span className="text-amber-400">claim_amount_bucket</span> alert triggered
                </div>
              </CardContent>
            </Card>

            {/* Impact History */}
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm flex items-center gap-2">
                  <TrendingUp className="h-4 w-4 text-emerald-400" />
                  Loop Impact (90 days)
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {[
                  { label: "Retrain cycles completed", value: "7" },
                  { label: "Cases used for retraining", value: "14,280" },
                  { label: "F1 improvement (cumulative)", value: "+4.2%" },
                  { label: "Revenue protected", value: "$1.2M" },
                  { label: "Avg. loop cycle time", value: "2.4 days" },
                  { label: "Models updated in prod", value: "3 models" },
                ].map((item) => (
                  <div key={item.label} className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">{item.label}</span>
                    <span className="font-semibold text-foreground">{item.value}</span>
                  </div>
                ))}
              </CardContent>
            </Card>

            {/* How it works */}
            <Card className="border-violet-500/30 bg-violet-500/5">
              <CardContent className="p-4 space-y-2">
                <div className="flex items-center gap-2 mb-2">
                  <RefreshCw className="h-4 w-4 text-violet-400" />
                  <span className="text-xs font-semibold text-violet-400">How the Loop Works</span>
                </div>
                {[
                  "Production model flags low-confidence predictions",
                  "Outcome tracker records actual claim resolution",
                  "Failure cases auto-labeled and segmented by root cause",
                  "Pull to IMDE with one click — or auto-pull on schedule",
                  "Incremental fine-tuning on failure set (keeps base model knowledge)",
                  "Auto-eval + governance gate before push",
                  "Every payer rule change becomes a training signal",
                ].map((item, i) => (
                  <div key={i} className="flex items-start gap-2 text-xs text-muted-foreground">
                    <span className="shrink-0 rounded-full bg-violet-500/20 text-violet-400 h-4 w-4 flex items-center justify-center text-[9px] mt-0.5">{i + 1}</span>
                    {item}
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  )
}
