"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import {
  Code2,
  Plus,
  Cpu,
  HardDrive,
  Zap,
  Users,
  Database,
  CheckCircle2,
  Clock,
  Play,
  StopCircle,
  Settings,
  ExternalLink,
  Shield,
  Globe,
  MemoryStick,
  Layers,
  AlertCircle,
  Terminal,
  FlaskConical,
  Brain,
} from "lucide-react"
import { cn } from "@/lib/utils"

interface Sandbox {
  id: string
  name: string
  status: "running" | "idle" | "stopped" | "building"
  owner: string
  team: string[]
  computeType: string
  cpu: number
  gpu?: string
  memoryGb: number
  storageGb: number
  cpuUsage: number
  memUsage: number
  dataSources: string[]
  preloadedTools: string[]
  createdAt: string
  lastActive: string
  description: string
}

const sandboxes: Sandbox[] = [
  {
    id: "sb-001",
    name: "RCM-Denial-Prediction-v3",
    status: "running",
    owner: "Dr. Sarah Chen",
    team: ["Sarah Chen", "Mike Johnson", "Priya Patel"],
    computeType: "GPU-Accelerated",
    cpu: 32,
    gpu: "NVIDIA A100 × 2",
    memoryGb: 128,
    storageGb: 2048,
    cpuUsage: 67,
    memUsage: 54,
    dataSources: ["Claims DB (prod-mirror)", "ERA/835 Feed", "Payer Rules Engine"],
    preloadedTools: ["PyTorch 2.3", "HuggingFace Transformers", "MLflow", "Jupyter Lab"],
    createdAt: "Feb 12, 2026",
    lastActive: "2 min ago",
    description: "Fine-tuning transformer model on denial reasons using 18 months of payer data",
  },
  {
    id: "sb-002",
    name: "ICD10-AutoCode-LLM",
    status: "running",
    owner: "James Rivera",
    team: ["James Rivera", "Linda Park"],
    computeType: "GPU-Accelerated",
    cpu: 16,
    gpu: "NVIDIA V100 × 4",
    memoryGb: 64,
    storageGb: 512,
    cpuUsage: 88,
    memUsage: 72,
    dataSources: ["Clinical Notes DB", "ICD-10-CM Reference", "Provider Encounter Feed"],
    preloadedTools: ["LangChain", "OpenAI SDK", "vLLM", "Jupyter Lab", "DVC"],
    createdAt: "Jan 28, 2026",
    lastActive: "15 min ago",
    description: "LLM-based ICD-10 auto-coding from clinical notes — multimodal extension in progress",
  },
  {
    id: "sb-003",
    name: "Auth-Approval-Predictor",
    status: "idle",
    owner: "Amy Kowalski",
    team: ["Amy Kowalski", "Tom Richards", "Sarah Chen"],
    computeType: "CPU-Optimized",
    cpu: 8,
    memoryGb: 32,
    storageGb: 256,
    cpuUsage: 4,
    memUsage: 18,
    dataSources: ["Authorization DB", "Payer Coverage Rules"],
    preloadedTools: ["scikit-learn", "XGBoost", "SHAP", "Jupyter Lab"],
    createdAt: "Feb 20, 2026",
    lastActive: "3 hours ago",
    description: "XGBoost ensemble for prior authorization approval likelihood scoring",
  },
  {
    id: "sb-004",
    name: "Billing-Anomaly-Detector",
    status: "stopped",
    owner: "Kevin Wu",
    team: ["Kevin Wu"],
    computeType: "Standard",
    cpu: 4,
    memoryGb: 16,
    storageGb: 128,
    cpuUsage: 0,
    memUsage: 0,
    dataSources: ["Billing Transactions DB"],
    preloadedTools: ["TensorFlow", "Pandas", "Jupyter Lab"],
    createdAt: "Feb 1, 2026",
    lastActive: "2 days ago",
    description: "Anomaly detection on billing codes — paused for domain expert review",
  },
]

const toolPresets = [
  {
    category: "LLMs & Gen AI",
    icon: Brain,
    color: "text-violet-400",
    bg: "bg-violet-500/10",
    tools: ["GPT-4o (Azure OAI)", "Claude 3.5 Sonnet", "Llama 3.1 70B", "LangChain", "LlamaIndex", "vLLM"],
  },
  {
    category: "ML / Deep Learning",
    icon: Layers,
    color: "text-blue-400",
    bg: "bg-blue-500/10",
    tools: ["PyTorch 2.3", "TensorFlow 2.16", "scikit-learn", "XGBoost", "LightGBM", "ONNX Runtime"],
  },
  {
    category: "Multimodal",
    icon: FlaskConical,
    color: "text-emerald-400",
    bg: "bg-emerald-500/10",
    tools: ["Florence-2", "Whisper", "CLIP", "LayoutLM", "Tesseract OCR", "OpenCV"],
  },
  {
    category: "MLOps & Tracking",
    icon: Terminal,
    color: "text-amber-400",
    bg: "bg-amber-500/10",
    tools: ["MLflow", "DVC", "Weights & Biases", "Ray Tune", "Optuna", "Great Expectations"],
  },
]

function StatusPill({ status }: { status: Sandbox["status"] }) {
  const map = {
    running: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
    idle: "bg-amber-500/20 text-amber-400 border-amber-500/30",
    stopped: "bg-secondary text-muted-foreground border-border",
    building: "bg-blue-500/20 text-blue-400 border-blue-500/30",
  }
  const labels = { running: "● Running", idle: "● Idle", stopped: "○ Stopped", building: "⟳ Building" }
  return (
    <span className={cn("rounded-full border px-2.5 py-0.5 text-xs font-medium", map[status])}>
      {labels[status]}
    </span>
  )
}

export default function IMDEWorkspacePage() {
  const [view, setView] = useState<"grid" | "list">("grid")

  const running = sandboxes.filter((s) => s.status === "running").length
  const total = sandboxes.length

  return (
    <div className="flex min-h-screen bg-background">
      <AppSidebar />
      <div className="ml-64 flex-1 p-6">
        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-3 mb-1">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-violet-500/20">
                <Code2 className="h-5 w-5 text-violet-400" />
              </div>
              <div>
                <h1 className="text-2xl font-bold text-foreground">IMDE Workspace</h1>
                <p className="text-sm text-muted-foreground">
                  Secure, production-like sandboxes for AI/ML model development
                </p>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Badge className="bg-emerald-500/20 text-emerald-400 border-emerald-500/30">
              {running}/{total} Active
            </Badge>
            <Button variant="outline" size="sm" className="gap-2">
              <Settings className="h-4 w-4" />
              Manage Compute
            </Button>
            <Button size="sm" className="gap-2 bg-violet-600 hover:bg-violet-700 text-white">
              <Plus className="h-4 w-4" />
              New Sandbox
            </Button>
          </div>
        </div>

        {/* Stats Bar */}
        <div className="mb-6 grid grid-cols-4 gap-4">
          {[
            { label: "Active Sandboxes", value: `${running}`, sub: `of ${total} total`, icon: Code2, color: "text-violet-400" },
            { label: "GPU Hours Used", value: "1,247", sub: "this month", icon: Zap, color: "text-amber-400" },
            { label: "Data Sources", value: "8", sub: "connected securely", icon: Database, color: "text-blue-400" },
            { label: "Team Members", value: "12", sub: "collaborators", icon: Users, color: "text-emerald-400" },
          ].map((stat) => (
            <Card key={stat.label} className="border-border">
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">{stat.label}</span>
                  <stat.icon className={cn("h-4 w-4", stat.color)} />
                </div>
                <div className="text-2xl font-bold text-foreground">{stat.value}</div>
                <div className="text-xs text-muted-foreground">{stat.sub}</div>
              </CardContent>
            </Card>
          ))}
        </div>

        <div className="grid grid-cols-3 gap-6">
          {/* Sandbox Cards */}
          <div className="col-span-2 space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-foreground">Your Sandboxes</h2>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Shield className="h-3.5 w-3.5 text-emerald-400" />
                Enterprise network isolated • SOC2 compliant
              </div>
            </div>
            {sandboxes.map((sb) => (
              <Card key={sb.id} className={cn("border-border transition-all hover:border-violet-500/40", sb.status === "stopped" && "opacity-60")}>
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <CardTitle className="text-sm font-semibold">{sb.name}</CardTitle>
                        <StatusPill status={sb.status} />
                      </div>
                      <CardDescription className="text-xs">{sb.description}</CardDescription>
                    </div>
                    <div className="flex items-center gap-1">
                      {sb.status === "running" ? (
                        <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-red-400">
                          <StopCircle className="h-4 w-4" />
                        </Button>
                      ) : sb.status !== "building" ? (
                        <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-emerald-400">
                          <Play className="h-4 w-4" />
                        </Button>
                      ) : null}
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-foreground">
                        <ExternalLink className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  {/* Compute Info */}
                  <div className="flex items-center gap-4 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1"><Cpu className="h-3.5 w-3.5" />{sb.cpu} vCPU</span>
                    <span className="flex items-center gap-1"><MemoryStick className="h-3.5 w-3.5" />{sb.memoryGb} GB RAM</span>
                    {sb.gpu && <span className="flex items-center gap-1 text-amber-400"><Zap className="h-3.5 w-3.5" />{sb.gpu}</span>}
                    <span className="flex items-center gap-1"><HardDrive className="h-3.5 w-3.5" />{sb.storageGb} GB</span>
                  </div>

                  {/* Usage */}
                  {sb.status !== "stopped" && (
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <div className="mb-1 flex justify-between text-xs">
                          <span className="text-muted-foreground">CPU</span>
                          <span className="text-foreground">{sb.cpuUsage}%</span>
                        </div>
                        <Progress value={sb.cpuUsage} className="h-1.5" />
                      </div>
                      <div>
                        <div className="mb-1 flex justify-between text-xs">
                          <span className="text-muted-foreground">Memory</span>
                          <span className="text-foreground">{sb.memUsage}%</span>
                        </div>
                        <Progress value={sb.memUsage} className="h-1.5" />
                      </div>
                    </div>
                  )}

                  {/* Data Sources */}
                  <div className="flex flex-wrap gap-1.5">
                    {sb.dataSources.map((ds) => (
                      <span key={ds} className="flex items-center gap-1 rounded-full bg-blue-500/10 px-2 py-0.5 text-[10px] text-blue-400 border border-blue-500/20">
                        <Database className="h-2.5 w-2.5" />{ds}
                      </span>
                    ))}
                  </div>

                  {/* Footer */}
                  <div className="flex items-center justify-between text-xs text-muted-foreground pt-1 border-t border-border">
                    <div className="flex items-center gap-1">
                      <Users className="h-3.5 w-3.5" />
                      <span>{sb.team.join(", ")}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      <span>{sb.lastActive}</span>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* Right panel */}
          <div className="space-y-4">
            {/* Data Sources */}
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm flex items-center gap-2">
                  <Database className="h-4 w-4 text-blue-400" />
                  Enterprise Data Sources
                </CardTitle>
                <CardDescription className="text-xs">Securely mirrored — no direct prod access</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2">
                {[
                  { name: "Claims Database", type: "SQL", status: "connected", freshness: "15 min lag" },
                  { name: "ERA/835 Feed", type: "Streaming", status: "connected", freshness: "Real-time" },
                  { name: "Clinical Notes", type: "Blob", status: "connected", freshness: "Daily" },
                  { name: "ICD-10-CM Ref", type: "Static", status: "connected", freshness: "Quarterly" },
                  { name: "Payer Rules Engine", type: "API", status: "connected", freshness: "On-demand" },
                  { name: "Auth DB (prod)", type: "SQL", status: "connected", freshness: "1 hr lag" },
                  { name: "Billing Transactions", type: "SQL", status: "connected", freshness: "Daily" },
                  { name: "Provider Encounters", type: "SQL", status: "pending", freshness: "Setup needed" },
                ].map((ds) => (
                  <div key={ds.name} className="flex items-center justify-between text-xs">
                    <div className="flex items-center gap-2">
                      <div className={cn("h-1.5 w-1.5 rounded-full", ds.status === "connected" ? "bg-emerald-400" : "bg-amber-400")} />
                      <span className="text-foreground font-medium">{ds.name}</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <span className="text-muted-foreground">{ds.freshness}</span>
                      <Badge variant="outline" className="text-[10px] h-4 px-1">{ds.type}</Badge>
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>

            {/* Pre-loaded Tools */}
            <Card className="border-border">
              <CardHeader className="pb-3">
                <CardTitle className="text-sm flex items-center gap-2">
                  <Layers className="h-4 w-4 text-violet-400" />
                  Pre-Loaded Tool Stack
                </CardTitle>
                <CardDescription className="text-xs">Available in every new sandbox</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {toolPresets.map((preset) => (
                  <div key={preset.category}>
                    <div className="flex items-center gap-2 mb-1.5">
                      <span className={cn("flex h-5 w-5 items-center justify-center rounded", preset.bg)}>
                        <preset.icon className={cn("h-3 w-3", preset.color)} />
                      </span>
                      <span className="text-xs font-medium text-foreground">{preset.category}</span>
                    </div>
                    <div className="flex flex-wrap gap-1 pl-7">
                      {preset.tools.map((t) => (
                        <span key={t} className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">{t}</span>
                      ))}
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>

            {/* Security Info */}
            <Card className="border-emerald-500/30 bg-emerald-500/5">
              <CardContent className="p-4 space-y-2">
                <div className="flex items-center gap-2 mb-2">
                  <Shield className="h-4 w-4 text-emerald-400" />
                  <span className="text-xs font-semibold text-emerald-400">Sandbox Security</span>
                </div>
                {[
                  "VNet-isolated per sandbox",
                  "No direct prod DB access",
                  "Data masking enforced",
                  "Audit log on all data reads",
                  "MFA + RBAC scoped access",
                  "SOC2 Type II compliant",
                ].map((item) => (
                  <div key={item} className="flex items-center gap-2 text-xs text-muted-foreground">
                    <CheckCircle2 className="h-3 w-3 text-emerald-400 shrink-0" />
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
