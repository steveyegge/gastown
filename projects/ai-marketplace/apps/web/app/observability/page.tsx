"use client"

import { useState, useEffect } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Activity,
  Brain,
  Bot,
  Wrench,
  Server,
  CheckCircle2,
  XCircle,
  Loader2,
  Clock,
  AlertTriangle,
  RefreshCw,
  ChevronRight,
  Circle,
  Zap,
  Timer,
  TrendingUp,
  TrendingDown,
} from "lucide-react"
import { Button } from "@/components/ui/button"

// ─── Types ───────────────────────────────────────────────────────────────────

type RunStatus = "running" | "completed" | "failed" | "queued" | "idle"

interface Run {
  id: string
  name: string
  version?: string
  status: RunStatus
  startedAt: string
  duration: string | null
  requestsPerMin?: number
  errorRate?: number
  latencyMs?: number
  lastError?: string
  tags?: string[]
}

// ─── Mock data ────────────────────────────────────────────────────────────────

const modelRuns: Run[] = [
  {
    id: "mdl-001",
    name: "DenialPrediction-BERT",
    version: "v3.2",
    status: "running",
    startedAt: "2 min ago",
    duration: "2m 14s",
    requestsPerMin: 142,
    errorRate: 0.2,
    latencyMs: 86,
    tags: ["production", "NLP"],
  },
  {
    id: "mdl-002",
    name: "MedicalCodingLLM",
    version: "v1.8",
    status: "running",
    startedAt: "11 min ago",
    duration: "11m 03s",
    requestsPerMin: 67,
    errorRate: 0.0,
    latencyMs: 240,
    tags: ["production", "TextGeneration"],
  },
  {
    id: "mdl-003",
    name: "ClaimRoutingTransformer",
    version: "v2.1",
    status: "completed",
    startedAt: "34 min ago",
    duration: "28m 41s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 112,
    tags: ["batch"],
  },
  {
    id: "mdl-004",
    name: "PayerInsightGPT",
    version: "v0.9",
    status: "failed",
    startedAt: "1 hr ago",
    duration: "4m 12s",
    requestsPerMin: 0,
    errorRate: 100,
    latencyMs: 0,
    lastError: "OOMKilled — container exceeded 16 GB memory limit",
    tags: ["beta"],
  },
  {
    id: "mdl-005",
    name: "FraudScoringXGB",
    version: "v4.0",
    status: "queued",
    startedAt: "queued 8 min ago",
    duration: null,
    tags: ["scheduled"],
  },
  {
    id: "mdl-006",
    name: "AuthorizationPredictor",
    version: "v2.7",
    status: "idle",
    startedAt: "last run 6 hr ago",
    duration: "18m 02s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 55,
    tags: ["production"],
  },
]

const agentRuns: Run[] = [
  {
    id: "agt-001",
    name: "RCM Denial Agent",
    version: "v1.4",
    status: "running",
    startedAt: "5 min ago",
    duration: "5m 21s",
    requestsPerMin: 23,
    errorRate: 0.0,
    latencyMs: 1240,
    tags: ["production", "revenue-cycle"],
  },
  {
    id: "agt-002",
    name: "Prior Auth Orchestrator",
    version: "v2.0",
    status: "running",
    startedAt: "22 min ago",
    duration: "22m 07s",
    requestsPerMin: 9,
    errorRate: 1.1,
    latencyMs: 3400,
    tags: ["production", "authorization"],
  },
  {
    id: "agt-003",
    name: "Claims Coding Assistant",
    version: "v1.1",
    status: "completed",
    startedAt: "1 hr 10 min ago",
    duration: "45m 33s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 980,
    tags: ["batch"],
  },
  {
    id: "agt-004",
    name: "Eligibility Verification Bot",
    version: "v3.2",
    status: "failed",
    startedAt: "2 hr ago",
    duration: "1m 45s",
    requestsPerMin: 0,
    errorRate: 100,
    lastError: "Upstream payer API returned 503 — connection timeout after 3 retries",
    tags: ["production"],
  },
  {
    id: "agt-005",
    name: "Audit Trail Summarizer",
    version: "v1.0",
    status: "queued",
    startedAt: "queued 2 min ago",
    duration: null,
    tags: ["scheduled"],
  },
]

const mcpRuns: Run[] = [
  {
    id: "mcp-001",
    name: "EHR Data MCP Server",
    status: "running",
    startedAt: "3 hr 14 min ago",
    duration: "3h 14m",
    requestsPerMin: 204,
    errorRate: 0.3,
    latencyMs: 18,
    tags: ["core", "ehr"],
  },
  {
    id: "mcp-002",
    name: "Claims DB MCP Server",
    status: "running",
    startedAt: "3 hr 14 min ago",
    duration: "3h 14m",
    requestsPerMin: 88,
    errorRate: 0.0,
    latencyMs: 12,
    tags: ["core", "claims"],
  },
  {
    id: "mcp-003",
    name: "Payer File MCP Gateway",
    status: "running",
    startedAt: "47 min ago",
    duration: "47m 02s",
    requestsPerMin: 31,
    errorRate: 0.0,
    latencyMs: 44,
    tags: ["integration"],
  },
  {
    id: "mcp-004",
    name: "Document Intelligence MCP",
    status: "failed",
    startedAt: "18 min ago",
    duration: "3m 11s",
    requestsPerMin: 0,
    errorRate: 100,
    lastError: "Azure Document Intelligence quota exceeded — retry in 2 min",
    tags: ["azure-ai"],
  },
  {
    id: "mcp-005",
    name: "Billing Code Lookup MCP",
    status: "idle",
    startedAt: "last run 30 min ago",
    duration: "12m 40s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 6,
    tags: ["core"],
  },
]

const toolRuns: Run[] = [
  {
    id: "tl-001",
    name: "ICD-10 Code Validator",
    status: "running",
    startedAt: "ongoing",
    duration: "continuous",
    requestsPerMin: 512,
    errorRate: 0.1,
    latencyMs: 3,
    tags: ["utility", "inline"],
  },
  {
    id: "tl-002",
    name: "Remittance Parser",
    status: "running",
    startedAt: "12 min ago",
    duration: "12m 05s",
    requestsPerMin: 77,
    errorRate: 0.0,
    latencyMs: 9,
    tags: ["batch"],
  },
  {
    id: "tl-003",
    name: "FHIR Converter",
    status: "completed",
    startedAt: "1 hr ago",
    duration: "32m 18s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 21,
    tags: ["integration"],
  },
  {
    id: "tl-004",
    name: "Eligibility EDI Parser",
    status: "completed",
    startedAt: "2 hr ago",
    duration: "8m 44s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 14,
    tags: ["edi"],
  },
  {
    id: "tl-005",
    name: "Duplicate Claim Detector",
    status: "queued",
    startedAt: "queued 4 min ago",
    duration: null,
    tags: ["scheduled"],
  },
  {
    id: "tl-006",
    name: "NPI Registry Lookup",
    status: "idle",
    startedAt: "last run 45 min ago",
    duration: "0m 02s",
    requestsPerMin: 0,
    errorRate: 0.0,
    latencyMs: 2,
    tags: ["utility"],
  },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_META: Record<RunStatus, { label: string; color: string; dot: string; Icon: React.ElementType }> = {
  running: {
    label: "Running",
    color: "bg-emerald-500/15 text-emerald-400 border border-emerald-500/30",
    dot: "bg-emerald-400",
    Icon: Loader2,
  },
  completed: {
    label: "Completed",
    color: "bg-sky-500/15 text-sky-400 border border-sky-500/30",
    dot: "bg-sky-400",
    Icon: CheckCircle2,
  },
  failed: {
    label: "Failed",
    color: "bg-red-500/15 text-red-400 border border-red-500/30",
    dot: "bg-red-400",
    Icon: XCircle,
  },
  queued: {
    label: "Queued",
    color: "bg-amber-500/15 text-amber-400 border border-amber-500/30",
    dot: "bg-amber-400",
    Icon: Clock,
  },
  idle: {
    label: "Idle",
    color: "bg-secondary text-muted-foreground border border-border",
    dot: "bg-muted-foreground",
    Icon: Circle,
  },
}

function StatusPill({ status }: { status: RunStatus }) {
  const meta = STATUS_META[status]
  const Icon = meta.Icon
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium", meta.color)}>
      <Icon className={cn("h-3 w-3", status === "running" && "animate-spin")} />
      {meta.label}
    </span>
  )
}

function Metric({ label, value, sub, trend }: { label: string; value: string | number; sub?: string; trend?: "up" | "down" | "neutral" }) {
  return (
    <div className="flex flex-col">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="flex items-center gap-1 text-sm font-semibold text-foreground">
        {value}
        {trend === "up" && <TrendingUp className="h-3 w-3 text-emerald-400" />}
        {trend === "down" && <TrendingDown className="h-3 w-3 text-red-400" />}
      </span>
      {sub && <span className="text-xs text-muted-foreground">{sub}</span>}
    </div>
  )
}

function RunRow({ run }: { run: Run }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <tr
        onClick={() => run.lastError && setExpanded((p) => !p)}
        className={cn(
          "border-b border-border/60 transition-colors",
          run.lastError ? "cursor-pointer hover:bg-secondary/30" : "hover:bg-secondary/20"
        )}
      >
        {/* Name */}
        <td className="py-3 pl-4 pr-2">
          <div className="flex items-start gap-2">
            {run.lastError && (
              <ChevronRight
                className={cn(
                  "mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
                  expanded && "rotate-90"
                )}
              />
            )}
            <div>
              <p className="text-sm font-medium text-foreground">{run.name}</p>
              {run.version && <p className="text-xs text-muted-foreground">{run.version}</p>}
            </div>
          </div>
        </td>

        {/* Status */}
        <td className="px-3 py-3">
          <StatusPill status={run.status} />
        </td>

        {/* Started */}
        <td className="px-3 py-3 text-xs text-muted-foreground">{run.startedAt}</td>

        {/* Duration */}
        <td className="px-3 py-3 text-xs text-muted-foreground font-mono">
          {run.duration ?? <span className="text-muted-foreground/40">—</span>}
        </td>

        {/* req/min */}
        <td className="px-3 py-3 text-xs font-mono text-foreground">
          {run.requestsPerMin != null ? (
            <span className={cn(run.status === "running" && run.requestsPerMin > 0 ? "text-emerald-400" : "text-muted-foreground")}>
              {run.requestsPerMin}
            </span>
          ) : (
            <span className="text-muted-foreground/40">—</span>
          )}
        </td>

        {/* Error rate */}
        <td className="px-3 py-3 text-xs font-mono">
          {run.errorRate != null ? (
            <span className={cn(run.errorRate > 0 ? "text-red-400" : "text-emerald-400")}>
              {run.errorRate}%
            </span>
          ) : (
            <span className="text-muted-foreground/40">—</span>
          )}
        </td>

        {/* Latency */}
        <td className="px-3 py-3 pr-4 text-xs font-mono text-muted-foreground">
          {run.latencyMs != null && run.latencyMs > 0 ? (
            <span className={cn(run.latencyMs > 2000 ? "text-amber-400" : "text-foreground")}>
              {run.latencyMs >= 1000 ? `${(run.latencyMs / 1000).toFixed(1)}s` : `${run.latencyMs}ms`}
            </span>
          ) : (
            <span className="text-muted-foreground/40">—</span>
          )}
        </td>
      </tr>

      {/* Expanded error row */}
      {expanded && run.lastError && (
        <tr className="border-b border-border/60 bg-red-500/5">
          <td colSpan={7} className="px-4 py-2">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-red-400" />
              <p className="text-xs text-red-300">{run.lastError}</p>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

// ─── Tab data ─────────────────────────────────────────────────────────────────

const TABS = [
  { id: "models", label: "Models", icon: Brain, data: modelRuns },
  { id: "agents", label: "Agents", icon: Bot, data: agentRuns },
  { id: "mcp", label: "MCP Servers", icon: Server, data: mcpRuns },
  { id: "tools", label: "Tools", icon: Wrench, data: toolRuns },
] as const

type TabId = (typeof TABS)[number]["id"]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ObservabilityPage() {
  const [activeTab, setActiveTab] = useState<TabId>("models")
  const [lastRefresh, setLastRefresh] = useState(new Date())
  const [ticking, setTicking] = useState(0)

  // Simulate live clock tick every 15 s
  useEffect(() => {
    const t = setInterval(() => setTicking((n) => n + 1), 15_000)
    return () => clearInterval(t)
  }, [])

  const allRuns = [...modelRuns, ...agentRuns, ...mcpRuns, ...toolRuns]
  const totalRunning = allRuns.filter((r) => r.status === "running").length
  const totalFailed = allRuns.filter((r) => r.status === "failed").length
  const totalQueued = allRuns.filter((r) => r.status === "queued").length
  const totalCompleted = allRuns.filter((r) => r.status === "completed").length

  const currentTab = TABS.find((t) => t.id === activeTab)!
  const rows = currentTab.data

  const tabRunning = rows.filter((r) => r.status === "running").length
  const tabFailed = rows.filter((r) => r.status === "failed").length

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 p-6">

        {/* Page header */}
        <div className="mb-6 flex items-start justify-between">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold text-foreground">
              <Activity className="h-6 w-6 text-[var(--optum-orange)]" />
              Observability
            </h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              Live run status for models, agents, MCP servers, and tools
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            onClick={() => setLastRefresh(new Date())}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Refresh
          </Button>
        </div>

        {/* Summary banner */}
        <div className="mb-6 grid grid-cols-4 gap-4">
          {[
            {
              label: "Running",
              value: totalRunning,
              icon: Loader2,
              color:
                "border-emerald-500/30 bg-emerald-500/10 text-emerald-400",
              iconSpin: true,
            },
            {
              label: "Completed",
              value: totalCompleted,
              icon: CheckCircle2,
              color: "border-sky-500/30 bg-sky-500/10 text-sky-400",
            },
            {
              label: "Failed",
              value: totalFailed,
              icon: XCircle,
              color: "border-red-500/30 bg-red-500/10 text-red-400",
            },
            {
              label: "Queued",
              value: totalQueued,
              icon: Clock,
              color: "border-amber-500/30 bg-amber-500/10 text-amber-400",
            },
          ].map((s) => {
            const Icon = s.icon
            return (
              <div
                key={s.label}
                className={cn(
                  "flex items-center gap-4 rounded-xl border p-4",
                  s.color
                )}
              >
                <Icon
                  className={cn(
                    "h-8 w-8 shrink-0 opacity-70",
                    (s as {iconSpin?: boolean}).iconSpin && "animate-spin"
                  )}
                />
                <div>
                  <p className="text-2xl font-bold">{s.value}</p>
                  <p className="text-xs opacity-80">{s.label}</p>
                </div>
              </div>
            )
          })}
        </div>

        {/* Tab bar */}
        <div className="mb-4 flex items-center gap-1 rounded-xl border border-border bg-card p-1">
          {TABS.map((tab) => {
            const Icon = tab.icon
            const running = tab.data.filter((r) => r.status === "running").length
            const failed = tab.data.filter((r) => r.status === "failed").length
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "flex flex-1 items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                  activeTab === tab.id
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                <Icon className="h-4 w-4" />
                <span>{tab.label}</span>
                <span className="flex items-center gap-1">
                  {running > 0 && (
                    <span className="rounded-full bg-emerald-500/20 px-1.5 py-0.5 text-xs text-emerald-400">
                      {running}
                    </span>
                  )}
                  {failed > 0 && (
                    <span className="rounded-full bg-red-500/20 px-1.5 py-0.5 text-xs text-red-400">
                      {failed}
                    </span>
                  )}
                </span>
              </button>
            )
          })}
        </div>

        {/* Tab sub-header */}
        <div className="mb-3 flex items-center justify-between px-1">
          <div className="flex items-center gap-3 text-sm">
            <span className="font-medium text-foreground">{rows.length} {currentTab.label}</span>
            {tabRunning > 0 && (
              <span className="flex items-center gap-1 text-emerald-400">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                {tabRunning} active
              </span>
            )}
            {tabFailed > 0 && (
              <span className="flex items-center gap-1 text-red-400">
                <XCircle className="h-3.5 w-3.5" />
                {tabFailed} failed
              </span>
            )}
          </div>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400"
            />
            Live · refreshed {lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}
          </span>
        </div>

        {/* Table */}
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <table className="w-full text-left">
            <thead>
              <tr className="border-b border-border bg-secondary/30 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <th className="py-3 pl-4 pr-2">Name</th>
                <th className="px-3 py-3">Status</th>
                <th className="px-3 py-3">Started</th>
                <th className="px-3 py-3">Duration</th>
                <th className="px-3 py-3">
                  <span className="flex items-center gap-1">
                    <Zap className="h-3 w-3" />
                    Req/min
                  </span>
                </th>
                <th className="px-3 py-3">
                  <span className="flex items-center gap-1">
                    <AlertTriangle className="h-3 w-3" />
                    Error %
                  </span>
                </th>
                <th className="px-3 py-3 pr-4">
                  <span className="flex items-center gap-1">
                    <Timer className="h-3 w-3" />
                    Latency
                  </span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((run) => (
                <RunRow key={run.id} run={run} />
              ))}
            </tbody>
          </table>
        </div>

        <p className="mt-3 text-center text-xs text-muted-foreground">
          Click a failed row to expand the error detail. Data refreshes every 15 s.
        </p>
      </main>
    </div>
  )
}
