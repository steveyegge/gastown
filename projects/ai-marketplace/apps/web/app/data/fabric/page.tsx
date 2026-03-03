"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Layers,
  ChevronRight,
  Plus,
  Settings,
  ExternalLink,
  RefreshCw,
  Play,
  Database,
  FileText,
  BarChart3,
  GitBranch,
  Zap,
  Clock,
  CheckCircle2,
  AlertCircle,
  XCircle,
  ArrowRight,
  Link2,
  Workflow,
  Table2,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"

// ── Types ─────────────────────────────────────────────────────────────────────

type ItemStatus = "active" | "paused" | "failed" | "draft"

interface FabricWorkspace {
  id: string
  name: string
  capacity: string
  region: string
  items: FabricItem[]
}

interface FabricItem {
  id: string
  name: string
  type: "lakehouse" | "warehouse" | "pipeline" | "notebook" | "report" | "semantic-model"
  status: ItemStatus
  lastRefresh?: string
  description?: string
  tags?: string[]
}

// ── Mock data ──────────────────────────────────────────────────────────────────

const workspaces: FabricWorkspace[] = [
  {
    id: "ws-optum-analytics",
    name: "Optum RCM Analytics",
    capacity: "F64 (64 CUs)",
    region: "East US",
    items: [
      {
        id: "lh-rcm-gold",
        name: "rcm-gold",
        type: "lakehouse",
        status: "active",
        lastRefresh: "10 min ago",
        description: "Gold-layer lakehouse for curated RCM data including claims, eligibility, and denials.",
        tags: ["healthcare", "gold-layer"],
      },
      {
        id: "lh-rcm-bronze",
        name: "rcm-bronze",
        type: "lakehouse",
        status: "active",
        lastRefresh: "1 hr ago",
        description: "Raw ingestion lakehouse receiving EHR exports and payer feeds.",
        tags: ["raw", "ingestion"],
      },
      {
        id: "wh-rcm-dw",
        name: "rcm-datawarehouse",
        type: "warehouse",
        status: "active",
        lastRefresh: "30 min ago",
        description: "SQL-queryable warehouse built on top of the gold lakehouse.",
        tags: ["sql", "warehouse"],
      },
      {
        id: "pl-ingestion",
        name: "Nightly EHR Ingestion",
        type: "pipeline",
        status: "active",
        lastRefresh: "6 hr ago",
        description: "Orchestrates file copy from SFTP → bronze lakehouse, plus schema normalization.",
        tags: ["pipeline", "ingestion"],
      },
      {
        id: "pl-transform",
        name: "Bronze → Gold Transformation",
        type: "pipeline",
        status: "active",
        lastRefresh: "6 hr ago",
        description: "Spark notebook pipeline that enriches and deduplicates into the gold layer.",
        tags: ["pipeline", "transform"],
      },
      {
        id: "nb-exploration",
        name: "RCM Data Exploration",
        type: "notebook",
        status: "active",
        lastRefresh: "2 days ago",
        description: "Shared Spark notebook for ad-hoc analysis of RCM data.",
      },
      {
        id: "rpt-denials",
        name: "Denial Analytics Dashboard",
        type: "report",
        status: "active",
        lastRefresh: "1 hr ago",
        description: "Power BI report tracking denial trends, reason codes, and recovery rates.",
        tags: ["denial", "powerbi"],
      },
      {
        id: "sm-rcm",
        name: "RCM Semantic Model",
        type: "semantic-model",
        status: "active",
        lastRefresh: "1 hr ago",
        description: "Unified semantic model exposing claims, members, and providers for AI queries.",
        tags: ["semantic", "AI-ready"],
      },
    ],
  },
  {
    id: "ws-ml-dev",
    name: "ML Development",
    capacity: "F8 (8 CUs)",
    region: "East US",
    items: [
      {
        id: "lh-ml-features",
        name: "ml-feature-store",
        type: "lakehouse",
        status: "active",
        lastRefresh: "4 hr ago",
        description: "Feature store for model training — claims-level features and member risk scores.",
        tags: ["features", "ml"],
      },
      {
        id: "pl-features",
        name: "Feature Engineering Pipeline",
        type: "pipeline",
        status: "paused",
        lastRefresh: "2 days ago",
        description: "Computes and materialises features for denial prediction models.",
        tags: ["ml", "pipeline"],
      },
    ],
  },
]

const itemTypeConfig: Record<FabricItem["type"], { icon: React.ElementType; color: string; bg: string; label: string }> = {
  lakehouse:       { icon: Database,  color: "text-violet-400", bg: "bg-violet-500/15", label: "Lakehouse" },
  warehouse:       { icon: Table2,    color: "text-emerald-400", bg: "bg-emerald-500/15", label: "Warehouse" },
  pipeline:        { icon: Workflow,  color: "text-orange-400", bg: "bg-orange-500/15", label: "Pipeline" },
  notebook:        { icon: FileText,  color: "text-sky-400",    bg: "bg-sky-500/15",    label: "Notebook" },
  report:          { icon: BarChart3, color: "text-pink-400",   bg: "bg-pink-500/15",   label: "Report" },
  "semantic-model":{ icon: GitBranch, color: "text-amber-400",  bg: "bg-amber-500/15",  label: "Semantic Model" },
}

function StatusBadge({ status }: { status: ItemStatus }) {
  const cfg = {
    active:  { cls: "bg-emerald-500/15 text-emerald-400", label: "Active" },
    paused:  { cls: "bg-amber-500/15 text-amber-400",     label: "Paused" },
    failed:  { cls: "bg-red-500/15 text-red-400",         label: "Failed" },
    draft:   { cls: "bg-secondary text-muted-foreground", label: "Draft" },
  }[status]
  return <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", cfg.cls)}>{cfg.label}</span>
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function FabricPage() {
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>("ws-optum-analytics")
  const [typeFilter, setTypeFilter] = useState<FabricItem["type"] | "all">("all")
  const [selectedItem, setSelectedItem] = useState<FabricItem | null>(null)

  const workspace = workspaces.find((w) => w.id === selectedWorkspace)
  const items = (workspace?.items ?? []).filter((i) => typeFilter === "all" || i.type === typeFilter)

  const typeCounts = Object.fromEntries(
    (["all", "lakehouse", "warehouse", "pipeline", "notebook", "report", "semantic-model"] as const).map((t) => [
      t,
      t === "all" ? workspace?.items.length ?? 0 : workspace?.items.filter((i) => i.type === t).length ?? 0,
    ])
  )

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 flex h-screen flex-col overflow-hidden p-6 pb-0">
        {/* Header */}
        <div className="mb-4 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Layers className="h-5 w-5 text-violet-400" />
              <h1 className="text-2xl font-bold text-foreground">Microsoft Fabric</h1>
              <Badge className="bg-violet-500/15 text-violet-400">2 Workspaces</Badge>
            </div>
            <p className="text-sm text-muted-foreground">
              Browse Fabric workspaces — lakehouses, warehouses, pipelines, notebooks, and reports.
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" className="gap-2"><RefreshCw className="h-3.5 w-3.5" /> Refresh</Button>
            <Button size="sm" className="gap-2 bg-violet-600 hover:bg-violet-700 text-white">
              <Link2 className="h-3.5 w-3.5" /> Connect Workspace
            </Button>
          </div>
        </div>

        {/* Workspace selector */}
        <div className="mb-4 flex gap-3">
          {workspaces.map((ws) => (
            <button
              key={ws.id}
              onClick={() => { setSelectedWorkspace(ws.id); setSelectedItem(null); setTypeFilter("all") }}
              className={cn(
                "flex flex-col rounded-lg border px-4 py-3 text-left transition-all",
                selectedWorkspace === ws.id
                  ? "border-violet-500/50 bg-violet-500/10"
                  : "border-border bg-card hover:border-violet-500/30"
              )}
            >
              <div className="flex items-center gap-2">
                <Layers className={cn("h-4 w-4", selectedWorkspace === ws.id ? "text-violet-400" : "text-muted-foreground")} />
                <span className="text-sm font-medium text-foreground">{ws.name}</span>
              </div>
              <p className="mt-0.5 text-xs text-muted-foreground">{ws.capacity} · {ws.region} · {ws.items.length} items</p>
            </button>
          ))}
          <button className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border px-4 py-3 text-muted-foreground hover:border-violet-500/30 hover:text-violet-400 transition-all">
            <Plus className="h-4 w-4" />
            <span className="mt-0.5 text-xs">Add</span>
          </button>
        </div>

        <div className="flex flex-1 gap-4 overflow-hidden pb-6">
          {/* Left: type filter */}
          <div className="flex w-44 shrink-0 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2.5">
              <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Item Type</p>
            </div>
            <div className="p-2 space-y-0.5">
              {(["all", "lakehouse", "warehouse", "pipeline", "notebook", "report", "semantic-model"] as const).map((t) => {
                const cfg = t === "all" ? { icon: Layers, color: "text-violet-400", label: "All Items" } : { ...itemTypeConfig[t], label: itemTypeConfig[t].label }
                const Icon = cfg.icon
                const count = typeCounts[t]
                if (count === 0 && t !== "all") return null
                return (
                  <button
                    key={t}
                    onClick={() => setTypeFilter(t)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors",
                      typeFilter === t
                        ? "bg-violet-500/15 text-violet-300 font-medium"
                        : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                    )}
                  >
                    <Icon className={cn("h-3.5 w-3.5 shrink-0", typeFilter === t ? "text-violet-400" : "text-muted-foreground")} />
                    <span className="flex-1 text-left">{cfg.label}</span>
                    <span className="text-[10px] text-muted-foreground/50">{count}</span>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Right: item grid */}
          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="mb-3 flex items-center justify-between">
              <p className="text-xs text-muted-foreground">{items.length} items</p>
            </div>
            <div className="grid grid-cols-3 gap-3 overflow-y-auto pr-1">
              {items.map((item) => {
                const { icon: Icon, color, bg, label } = itemTypeConfig[item.type]
                return (
                  <Card
                    key={item.id}
                    onClick={() => setSelectedItem(selectedItem?.id === item.id ? null : item)}
                    className={cn(
                      "cursor-pointer border-border bg-card transition-all hover:border-violet-500/40",
                      selectedItem?.id === item.id && "border-violet-500/50 bg-violet-500/5"
                    )}
                  >
                    <CardHeader className="pb-2">
                      <div className="flex items-start justify-between">
                        <div className={cn("flex h-9 w-9 items-center justify-center rounded-lg", bg)}>
                          <Icon className={cn("h-4 w-4", color)} />
                        </div>
                        <StatusBadge status={item.status} />
                      </div>
                      <CardTitle className="text-sm font-semibold">{item.name}</CardTitle>
                      <CardDescription className="text-xs">{label}</CardDescription>
                    </CardHeader>
                    <CardContent className="pb-3">
                      <p className="mb-2 text-xs text-muted-foreground leading-relaxed line-clamp-2">{item.description}</p>
                      {item.tags && (
                        <div className="mb-2 flex flex-wrap gap-1">
                          {item.tags.map((tag) => (
                            <span key={tag} className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">{tag}</span>
                          ))}
                        </div>
                      )}
                      {item.lastRefresh && (
                        <div className="flex items-center gap-1 text-[10px] text-muted-foreground/60">
                          <Clock className="h-2.5 w-2.5" /> Refreshed {item.lastRefresh}
                        </div>
                      )}
                      <div className="mt-2 flex gap-1">
                        {(item.type === "lakehouse" || item.type === "warehouse") && (
                          <Button variant="ghost" size="sm" className="h-6 px-2 text-[10px] text-muted-foreground hover:text-foreground gap-1">
                            <Table2 className="h-3 w-3" /> Query
                          </Button>
                        )}
                        {item.type === "pipeline" && (
                          <Button variant="ghost" size="sm" className="h-6 px-2 text-[10px] text-muted-foreground hover:text-foreground gap-1">
                            <Play className="h-3 w-3" /> Run
                          </Button>
                        )}
                        <Button variant="ghost" size="sm" className="h-6 px-2 text-[10px] text-muted-foreground hover:text-foreground gap-1 ml-auto">
                          <ExternalLink className="h-3 w-3" /> Open in Fabric
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          </div>
        </div>
      </main>
    </div>
  )
}
