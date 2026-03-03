"use client"

import { useState, useEffect, useCallback } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
  MemoryStick,
  Layers,
  AlertCircle,
  Terminal,
  FlaskConical,
  Brain,
  RefreshCw,
  Trash2,
  RotateCcw,
  Server,
  CloudCog,
  Loader2,
  Info,
} from "lucide-react"
import { cn } from "@/lib/utils"

// ── Types ──────────────────────────────────────────────────────────────────────

interface AmlComputeInstance {
  name: string
  id: string
  computeType: string
  provisioningState: string
  vmSize: string
  state?: string
  description?: string
  createdBy?: string
  createdOn?: string
  modifiedOn?: string
  cpuCores?: number
  memoryGb?: number
  currentNodeCount?: number
  maxNodeCount?: number
  isGpu?: boolean
  gpuSpec?: string
  tags?: Record<string, string>
  sshPort?: number
  studioUrl?: string
}

interface VmSizeOption {
  id: string
  label: string
  tier: string
  isGpu: boolean
}

interface ComputeApiResponse {
  computes: AmlComputeInstance[]
  configured: boolean
  workspace?: string
  resourceGroup?: string
  subscription?: string
  vmSizes?: VmSizeOption[]
  error?: string
}

interface Sandbox {
  id: string
  name: string
  status: "running" | "idle" | "stopped" | "building" | "starting" | "stopping"
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
  studioUrl?: string
  amlName?: string
}

// ── Tool presets (pre-loaded in every sandbox) ────────────────────────────────

const toolPresets = [
  {
    category: "ML Frameworks",
    icon: Zap,
    color: "text-amber-400",
    bg: "bg-amber-500/10",
    tools: ["PyTorch 2.2", "TensorFlow 2.15", "scikit-learn", "XGBoost", "LightGBM"],
  },
  {
    category: "Data & ETL",
    icon: Database,
    color: "text-blue-400",
    bg: "bg-blue-500/10",
    tools: ["pandas", "Spark 3.5", "dbt", "Great Expectations", "Arrow"],
  },
  {
    category: "Dev Tools",
    icon: Code2,
    color: "text-violet-400",
    bg: "bg-violet-500/10",
    tools: ["JupyterLab", "VS Code Server", "git", "Poetry", "Docker"],
  },
  {
    category: "Observability",
    icon: Layers,
    color: "text-emerald-400",
    bg: "bg-emerald-500/10",
    tools: ["MLflow", "Weights & Biases", "Prometheus", "OpenTelemetry"],
  },
]

// ── Static fallback data ───────────────────────────────────────────────────────

const STATIC_SANDBOXES: Sandbox[] = [
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

// ── Helpers ────────────────────────────────────────────────────────────────────

function amlStateToSandboxStatus(state?: string, provisioningState?: string): Sandbox["status"] {
  const s = (state ?? "").toLowerCase()
  const p = (provisioningState ?? "").toLowerCase()
  if (s === "running" || s === "jobrunning") return "running"
  if (s === "stopped") return "stopped"
  if (s === "starting" || p === "creating") return "starting"
  if (s === "stopping") return "stopping"
  if (s === "restarting" || p === "updating") return "building"
  return "idle"
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 2) return "just now"
  if (mins < 60) return `${mins} min ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs} hr ago`
  return `${Math.floor(hrs / 24)} days ago`
}

function amlComputeToSandbox(c: AmlComputeInstance, index: number): Sandbox {
  const status = amlStateToSandboxStatus(c.state, c.provisioningState)
  const isRunning = status === "running"
  const defaultTeams = [["Dr. Sarah Chen", "Mike Johnson"], ["James Rivera", "Linda Park"], ["Amy Kowalski"], ["Kevin Wu"]]
  const defaultDs = [["Claims DB", "ERA/835 Feed"], ["Clinical Notes DB", "ICD-10-CM Ref"], ["Auth DB", "Payer Rules"], ["Billing Transactions DB"]]
  const defaultTools = [["PyTorch", "MLflow", "Jupyter Lab"], ["LangChain", "vLLM", "Jupyter Lab"], ["scikit-learn", "XGBoost", "Jupyter Lab"], ["TensorFlow", "Pandas"]]
  return {
    id: `aml-${c.name}`,
    name: c.name,
    status,
    owner: c.createdBy ?? "Team Member",
    team: defaultTeams[index % defaultTeams.length],
    computeType: c.isGpu ? "GPU-Accelerated" : (c.memoryGb ?? 0) >= 64 ? "Memory-Optimized" : "CPU-Optimized",
    cpu: c.cpuCores ?? 4,
    gpu: c.gpuSpec,
    memoryGb: c.memoryGb ?? 16,
    storageGb: 512,
    cpuUsage: isRunning ? Math.floor(Math.random() * 60) + 20 : 0,
    memUsage: isRunning ? Math.floor(Math.random() * 50) + 20 : 0,
    dataSources: defaultDs[index % defaultDs.length],
    preloadedTools: defaultTools[index % defaultTools.length],
    createdAt: c.createdOn
      ? new Date(c.createdOn).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
      : "—",
    lastActive: c.modifiedOn ? timeAgo(c.modifiedOn) : "Unknown",
    description: c.description ?? `${c.vmSize} compute instance in ai-project-q2w5uxlkh4c6o`,
    studioUrl: c.studioUrl,
    amlName: c.name,
  }
}

// ── Sub-components ─────────────────────────────────────────────────────────────

function StatusPill({ status }: { status: Sandbox["status"] }) {
  const map: Record<string, string> = {
    running: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
    idle: "bg-amber-500/20 text-amber-400 border-amber-500/30",
    stopped: "bg-secondary text-muted-foreground border-border",
    building: "bg-blue-500/20 text-blue-400 border-blue-500/30",
    starting: "bg-blue-500/20 text-blue-400 border-blue-500/30",
    stopping: "bg-orange-500/20 text-orange-400 border-orange-500/30",
  }
  const labels: Record<string, string> = {
    running: "● Running",
    idle: "● Idle",
    stopped: "○ Stopped",
    building: "⟳ Provisioning",
    starting: "⟳ Starting",
    stopping: "⟳ Stopping",
  }
  return (
    <span className={cn("rounded-full border px-2.5 py-0.5 text-xs font-medium", map[status] ?? map.idle)}>
      {labels[status] ?? status}
    </span>
  )
}

// ── Main Page ──────────────────────────────────────────────────────────────────

export default function IMDEWorkspacePage() {
  const [sandboxes, setSandboxes] = useState<Sandbox[]>(STATIC_SANDBOXES)
  const [amlComputes, setAmlComputes] = useState<AmlComputeInstance[]>([])
  const [vmSizes, setVmSizes] = useState<VmSizeOption[]>([])
  const [amlWorkspace, setAmlWorkspace] = useState<string | null>(null)
  const [amlConfigured, setAmlConfigured] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  const [pendingAction, setPendingAction] = useState<Record<string, string>>({})
  const [manageOpen, setManageOpen] = useState(false)
  const [provisionOpen, setProvisionOpen] = useState(false)
  const [provisionName, setProvisionName] = useState("")
  const [provisionVmSize, setProvisionVmSize] = useState("Standard_DS3_v2")
  const [provisionDesc, setProvisionDesc] = useState("")
  const [provisionLoading, setProvisionLoading] = useState(false)
  const [provisionError, setProvisionError] = useState<string | null>(null)
  const [provisionSuccess, setProvisionSuccess] = useState(false)

  // ── Fetch compute from API ──────────────────────────────────────────────────

  const fetchCompute = useCallback(async () => {
    setIsLoading(true)
    setLoadError(null)
    try {
      const res = await fetch("/api/aml/compute")
      const data: ComputeApiResponse = await res.json()
      setAmlConfigured(data.configured)
      setAmlWorkspace(data.workspace ?? null)
      if (data.vmSizes) setVmSizes(data.vmSizes)
      if (data.configured && data.computes.length > 0) {
        setAmlComputes(data.computes)
        setSandboxes(data.computes.map((c, i) => amlComputeToSandbox(c, i)))
      } else if (data.error && data.configured) {
        setLoadError(data.error)
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load compute data")
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => { fetchCompute() }, [fetchCompute])

  // ── Start / Stop action ─────────────────────────────────────────────────────

  const handleAction = useCallback(async (sb: Sandbox, action: "start" | "stop" | "restart") => {
    if (!sb.amlName) return
    setPendingAction((p) => ({ ...p, [sb.id]: action }))
    try {
      const res = await fetch(`/api/aml/compute/${encodeURIComponent(sb.amlName)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      })
      if (!res.ok) {
        const err = await res.json()
        console.error("Compute action failed:", err.error)
      } else {
        setSandboxes((prev) =>
          prev.map((s) =>
            s.id === sb.id
              ? { ...s, status: action === "start" ? "starting" : action === "stop" ? "stopping" : "building" }
              : s
          )
        )
        setTimeout(() => fetchCompute(), 5000)
      }
    } finally {
      setPendingAction((p) => { const n = { ...p }; delete n[sb.id]; return n })
    }
  }, [fetchCompute])

  // ── Delete compute ──────────────────────────────────────────────────────────

  const handleDelete = useCallback(async (computeName: string) => {
    if (!confirm(`Delete compute instance "${computeName}"? This cannot be undone.`)) return
    try {
      await fetch(`/api/aml/compute/${encodeURIComponent(computeName)}`, { method: "DELETE" })
      setAmlComputes((prev) => prev.filter((c) => c.name !== computeName))
      setSandboxes((prev) => prev.filter((s) => s.amlName !== computeName))
    } catch (err) {
      console.error("Delete failed:", err)
    }
  }, [])

  // ── Provision ──────────────────────────────────────────────────────────────

  const handleProvision = async () => {
    setProvisionLoading(true)
    setProvisionError(null)
    try {
      const res = await fetch("/api/aml/compute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: provisionName, vmSize: provisionVmSize, description: provisionDesc }),
      })
      const data = await res.json()
      if (!res.ok) { setProvisionError(data.error ?? "Provisioning failed"); return }
      setProvisionSuccess(true)
      const newSb: Sandbox = {
        id: `aml-${provisionName}`,
        name: provisionName,
        status: "building",
        owner: "You",
        team: ["You"],
        computeType: vmSizes.find((v) => v.id === provisionVmSize)?.isGpu ? "GPU-Accelerated" : "Standard",
        cpu: 4, memoryGb: 16, storageGb: 256, cpuUsage: 0, memUsage: 0,
        dataSources: [], preloadedTools: ["Jupyter Lab"],
        createdAt: new Date().toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }),
        lastActive: "just now",
        description: provisionDesc || `${provisionVmSize} compute instance (provisioning…)`,
        amlName: provisionName,
      }
      setSandboxes((prev) => [newSb, ...prev])
      setTimeout(() => { fetchCompute(); setProvisionSuccess(false) }, 30000)
    } catch (err) {
      setProvisionError(err instanceof Error ? err.message : "Request failed")
    } finally {
      setProvisionLoading(false)
    }
  }

  const closeProvision = () => {
    setProvisionOpen(false); setProvisionName(""); setProvisionDesc("")
    setProvisionVmSize("Standard_DS3_v2"); setProvisionError(null)
    setProvisionSuccess(false); setProvisionLoading(false)
  }

  const running = sandboxes.filter((s) => s.status === "running" || s.status === "starting").length
  const total = sandboxes.length

  return (
    <div className="flex min-h-screen bg-background">
      <AppSidebar />
      <div className="ml-64 flex-1 p-6">

        {/* AML Workspace Banner */}
        {amlConfigured && amlWorkspace && (
          <div className="mb-4 flex items-center gap-2 rounded-lg border border-violet-500/30 bg-violet-500/5 px-4 py-2.5">
            <CloudCog className="h-4 w-4 text-violet-400 shrink-0" />
            <span className="text-xs text-violet-300 font-medium">Azure ML Project:</span>
            <code className="text-xs text-violet-200 font-mono">{amlWorkspace}</code>
            <span className="ml-auto flex items-center gap-1 text-[10px] text-emerald-400">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
              Live
            </span>
          </div>
        )}
        {!amlConfigured && !isLoading && (
          <div className="mb-4 flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-2.5">
            <Info className="h-4 w-4 text-amber-400 shrink-0" />
            <span className="text-xs text-amber-300">
              Showing demo data — set{" "}
              <code className="font-mono">AZURE_ML_WORKSPACE=ai-project-q2w5uxlkh4c6o</code> to connect to Azure ML.
            </span>
          </div>
        )}
        {loadError && (
          <div className="mb-4 flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-2.5">
            <AlertCircle className="h-4 w-4 text-red-400 shrink-0" />
            <span className="text-xs text-red-300 flex-1 truncate">{loadError}</span>
            <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={fetchCompute}>Retry</Button>
          </div>
        )}

        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div className="flex items-center gap-3">
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
          <div className="flex items-center gap-2">
            <Badge className={cn("border", running > 0
              ? "bg-emerald-500/20 text-emerald-400 border-emerald-500/30"
              : "bg-secondary text-muted-foreground border-border"
            )}>
              {running}/{total} Active
            </Badge>
            <Button variant="outline" size="sm" className="gap-2" onClick={fetchCompute} disabled={isLoading}>
              {isLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              Refresh
            </Button>
            <Button variant="outline" size="sm" className="gap-2" onClick={() => setManageOpen(true)}>
              <Settings className="h-4 w-4" />
              Manage Compute
            </Button>
            <Button size="sm" className="gap-2 bg-violet-600 hover:bg-violet-700 text-white" onClick={() => setProvisionOpen(true)}>
              <Plus className="h-4 w-4" />
              New Sandbox
            </Button>
          </div>
        </div>

        {/* Stats Bar */}
        <div className="mb-6 grid grid-cols-4 gap-4">
          {[
            { label: "Active Sandboxes", value: `${running}`, sub: `of ${total} total`, icon: Code2, color: "text-violet-400" },
            { label: "GPU Instances", value: `${amlComputes.filter((c) => c.isGpu).length || sandboxes.filter((s) => s.gpu).length}`, sub: "GPU compute", icon: Zap, color: "text-amber-400" },
            { label: "Compute Types", value: `${[...new Set(amlComputes.map((c) => c.computeType))].length || 2}`, sub: "in workspace", icon: Server, color: "text-blue-400" },
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
              <h2 className="text-sm font-semibold text-foreground">
                {amlConfigured ? `Compute Instances · ${amlWorkspace}` : "Your Sandboxes"}
              </h2>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Shield className="h-3.5 w-3.5 text-emerald-400" />
                Enterprise network isolated · SOC2 compliant
              </div>
            </div>

            {isLoading && sandboxes.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-16 gap-3 text-muted-foreground">
                <Loader2 className="h-8 w-8 animate-spin text-violet-400" />
                <span className="text-sm">Loading compute instances from Azure ML…</span>
              </div>
            ) : (
              sandboxes.map((sb) => (
                <Card
                  key={sb.id}
                  className={cn("border-border transition-all hover:border-violet-500/40", sb.status === "stopped" && "opacity-60")}
                >
                  <CardHeader className="pb-3">
                    <div className="flex items-start justify-between">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <CardTitle className="text-sm font-semibold font-mono">{sb.name}</CardTitle>
                          <StatusPill status={sb.status} />
                          {(sb.status === "building" || sb.status === "starting" || sb.status === "stopping") && (
                            <Loader2 className="h-3 w-3 animate-spin text-blue-400" />
                          )}
                        </div>
                        <CardDescription className="text-xs">{sb.description}</CardDescription>
                      </div>
                      <div className="flex items-center gap-1">
                        {pendingAction[sb.id] ? (
                          <Button variant="ghost" size="icon" className="h-7 w-7" disabled>
                            <Loader2 className="h-4 w-4 animate-spin" />
                          </Button>
                        ) : sb.status === "running" ? (
                          <Button
                            variant="ghost" size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-red-400"
                            onClick={() => handleAction(sb, "stop")}
                            title="Stop compute"
                          >
                            <StopCircle className="h-4 w-4" />
                          </Button>
                        ) : (sb.status === "stopped" || sb.status === "idle") ? (
                          <Button
                            variant="ghost" size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-emerald-400"
                            onClick={() => handleAction(sb, "start")}
                            title="Start compute"
                          >
                            <Play className="h-4 w-4" />
                          </Button>
                        ) : null}
                        {sb.studioUrl && (
                          <a href={sb.studioUrl} target="_blank" rel="noreferrer">
                            <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-foreground" title="Open in Azure ML Studio">
                              <ExternalLink className="h-4 w-4" />
                            </Button>
                          </a>
                        )}
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex items-center gap-4 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1"><Cpu className="h-3.5 w-3.5" />{sb.cpu} vCPU</span>
                      <span className="flex items-center gap-1"><MemoryStick className="h-3.5 w-3.5" />{sb.memoryGb} GB RAM</span>
                      {sb.gpu && <span className="flex items-center gap-1 text-amber-400"><Zap className="h-3.5 w-3.5" />{sb.gpu}</span>}
                      <span className="flex items-center gap-1"><HardDrive className="h-3.5 w-3.5" />{sb.storageGb} GB</span>
                    </div>
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
                    <div className="flex flex-wrap gap-1.5">
                      {sb.dataSources.map((ds) => (
                        <span key={ds} className="flex items-center gap-1 rounded-full bg-blue-500/10 px-2 py-0.5 text-[10px] text-blue-400 border border-blue-500/20">
                          <Database className="h-2.5 w-2.5" />{ds}
                        </span>
                      ))}
                    </div>
                    <div className="flex items-center justify-between text-xs text-muted-foreground pt-1 border-t border-border">
                      <div className="flex items-center gap-1"><Users className="h-3.5 w-3.5" /><span>{sb.team.join(", ")}</span></div>
                      <div className="flex items-center gap-1"><Clock className="h-3.5 w-3.5" /><span>{sb.lastActive}</span></div>
                    </div>
                  </CardContent>
                </Card>
              ))
            )}
          </div>

          {/* Right panel */}
          <div className="space-y-4">
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

            <Card className="border-emerald-500/30 bg-emerald-500/5">
              <CardContent className="p-4 space-y-2">
                <div className="flex items-center gap-2 mb-2">
                  <Shield className="h-4 w-4 text-emerald-400" />
                  <span className="text-xs font-semibold text-emerald-400">Sandbox Security</span>
                </div>
                {["VNet-isolated per sandbox", "No direct prod DB access", "Data masking enforced",
                  "Audit log on all data reads", "MFA + RBAC scoped access", "SOC2 Type II compliant"].map((item) => (
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

      {/* ── Manage Compute Dialog ─────────────────────────────────────── */}
      <Dialog open={manageOpen} onOpenChange={setManageOpen}>
        <DialogContent className="max-w-3xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <CloudCog className="h-5 w-5 text-violet-400" />
              Manage Compute · {amlWorkspace ?? "ai-project-q2w5uxlkh4c6o"}
            </DialogTitle>
            <DialogDescription>
              All compute instances and clusters in the Azure ML workspace. Start, stop, or delete resources.
            </DialogDescription>
          </DialogHeader>

          {!amlConfigured ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              <CloudCog className="h-8 w-8 mx-auto mb-3 opacity-40" />
              Azure ML workspace not configured.
              <br />
              Set <code className="font-mono text-xs bg-secondary px-1.5 py-0.5 rounded ml-1">
                AZURE_ML_WORKSPACE=ai-project-q2w5uxlkh4c6o
              </code>
            </div>
          ) : amlComputes.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              {isLoading ? <Loader2 className="h-6 w-6 mx-auto mb-2 animate-spin" /> : null}
              {isLoading ? "Loading computes…" : "No compute resources found in workspace."}
            </div>
          ) : (
            <div className="space-y-2 mt-2">
              {amlComputes.map((c) => {
                const status = amlStateToSandboxStatus(c.state, c.provisioningState)
                const matchedSb = sandboxes.find((s) => s.amlName === c.name)
                return (
                  <div key={c.name} className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3">
                    <div className="space-y-0.5 min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm font-medium truncate">{c.name}</span>
                        <StatusPill status={status} />
                        <Badge variant="outline" className="text-[10px] h-4 shrink-0">
                          {c.computeType === "ComputeInstance" ? "Instance" : c.computeType === "AmlCompute" ? "Cluster" : c.computeType}
                        </Badge>
                      </div>
                      <div className="flex items-center gap-3 text-xs text-muted-foreground">
                        <span className="font-mono">{c.vmSize}</span>
                        <span className="flex items-center gap-1"><Cpu className="h-3 w-3" />{c.cpuCores ?? "?"} vCPU</span>
                        <span className="flex items-center gap-1"><MemoryStick className="h-3 w-3" />{c.memoryGb ?? "?"} GB</span>
                        {c.isGpu && <span className="text-amber-400 flex items-center gap-1"><Zap className="h-3 w-3" />{c.gpuSpec}</span>}
                      </div>
                    </div>
                    <div className="flex items-center gap-1 ml-3 shrink-0">
                      {(status === "stopped" || status === "idle") ? (
                        <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-emerald-400 hover:text-emerald-300"
                          onClick={() => matchedSb && handleAction(matchedSb, "start")}>
                          <Play className="h-3 w-3" />Start
                        </Button>
                      ) : status === "running" ? (
                        <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-orange-400 hover:text-orange-300"
                          onClick={() => matchedSb && handleAction(matchedSb, "stop")}>
                          <StopCircle className="h-3 w-3" />Stop
                        </Button>
                      ) : null}
                      <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-muted-foreground hover:text-blue-400"
                        onClick={() => matchedSb && handleAction(matchedSb, "restart")}>
                        <RotateCcw className="h-3 w-3" />Restart
                      </Button>
                      {c.studioUrl && (
                        <a href={c.studioUrl} target="_blank" rel="noreferrer">
                          <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-muted-foreground hover:text-foreground">
                            <ExternalLink className="h-3 w-3" />Studio
                          </Button>
                        </a>
                      )}
                      <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-muted-foreground hover:text-red-400"
                        onClick={() => handleDelete(c.name)}>
                        <Trash2 className="h-3 w-3" />Delete
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" size="sm" onClick={fetchCompute} disabled={isLoading}>
              {isLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : <RefreshCw className="h-3.5 w-3.5 mr-1" />}
              Refresh
            </Button>
            <Button size="sm" className="bg-violet-600 hover:bg-violet-700 text-white gap-2"
              onClick={() => { setManageOpen(false); setProvisionOpen(true) }}>
              <Plus className="h-3.5 w-3.5" />New Compute
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── New Sandbox / Provision Dialog ────────────────────────────── */}
      <Dialog open={provisionOpen} onOpenChange={(o) => { if (!o) closeProvision(); else setProvisionOpen(true) }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Plus className="h-5 w-5 text-violet-400" />
              New Compute Sandbox
            </DialogTitle>
            <DialogDescription>
              Provision a new Azure ML Compute Instance in{" "}
              <code className="text-xs font-mono bg-secondary px-1 py-0.5 rounded">
                {amlWorkspace ?? "ai-project-q2w5uxlkh4c6o"}
              </code>.
              Ready in 2–4 minutes.
            </DialogDescription>
          </DialogHeader>

          {provisionSuccess ? (
            <div className="py-8 text-center space-y-3">
              <CheckCircle2 className="h-10 w-10 text-emerald-400 mx-auto" />
              <p className="text-sm font-medium">Provisioning started!</p>
              <p className="text-xs text-muted-foreground">
                <code className="font-mono bg-secondary px-1 rounded">{provisionName}</code> is being created. Ready in 2–4 min.
              </p>
              <Button size="sm" onClick={closeProvision}>Done</Button>
            </div>
          ) : (
            <div className="space-y-4 py-2">
              <div className="space-y-1.5">
                <Label htmlFor="compute-name" className="text-xs font-medium">
                  Compute Name <span className="text-red-400">*</span>
                </Label>
                <Input
                  id="compute-name"
                  placeholder="e.g. denial-pred-gpu-01"
                  value={provisionName}
                  onChange={(e) => setProvisionName(e.target.value)}
                  className="font-mono text-sm h-8"
                />
                <p className="text-[10px] text-muted-foreground">Letters, numbers, hyphens. Start with letter. 2–32 chars.</p>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="vm-size" className="text-xs font-medium">
                  VM Size <span className="text-red-400">*</span>
                </Label>
                <Select value={provisionVmSize} onValueChange={setProvisionVmSize}>
                  <SelectTrigger id="vm-size" className="h-8 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(vmSizes.length > 0 ? vmSizes : [
                      { id: "Standard_DS3_v2", label: "DS3 v2 — 4 vCPU / 14 GB", tier: "CPU", isGpu: false },
                      { id: "Standard_D16s_v3", label: "D16s v3 — 16 vCPU / 64 GB", tier: "CPU", isGpu: false },
                      { id: "Standard_E8s_v3", label: "E8s v3 — 8 vCPU / 64 GB (Memory)", tier: "Memory", isGpu: false },
                      { id: "Standard_NC6s_v3", label: "NC6s v3 — 6 vCPU / V100 × 1", tier: "GPU", isGpu: true },
                      { id: "Standard_NC24s_v3", label: "NC24s v3 — 24 vCPU / V100 × 4", tier: "GPU", isGpu: true },
                      { id: "Standard_NC24ads_A100_v4", label: "NC24ads A100 v4 — 24 vCPU / A100 × 1", tier: "GPU", isGpu: true },
                    ]).map((v) => (
                      <SelectItem key={v.id} value={v.id}>
                        <div className="flex items-center gap-2">
                          {v.isGpu && <Zap className="h-3 w-3 text-amber-400 shrink-0" />}
                          <span className="font-mono text-xs">{v.label}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="compute-desc" className="text-xs font-medium">Description</Label>
                <Input
                  id="compute-desc"
                  placeholder="What will this sandbox be used for?"
                  value={provisionDesc}
                  onChange={(e) => setProvisionDesc(e.target.value)}
                  className="text-sm h-8"
                />
              </div>

              {provisionError && (
                <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/5 px-3 py-2 text-xs text-red-400">
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />{provisionError}
                </div>
              )}
              {!amlConfigured && (
                <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-400">
                  <Info className="h-3.5 w-3.5 shrink-0" />
                  Azure ML not configured — provision will fail without env vars.
                </div>
              )}
            </div>
          )}

          {!provisionSuccess && (
            <DialogFooter>
              <Button variant="outline" size="sm" onClick={closeProvision}>Cancel</Button>
              <Button
                size="sm"
                className="bg-violet-600 hover:bg-violet-700 text-white gap-2"
                onClick={handleProvision}
                disabled={provisionLoading || !provisionName.trim() || !provisionVmSize}
              >
                {provisionLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
                {provisionLoading ? "Provisioning…" : "Create Sandbox"}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
