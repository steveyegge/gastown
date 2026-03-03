"use client"

import { useState } from "react"
import Link from "next/link"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Database,
  HardDrive,
  Table2,
  Layers,
  Sparkles,
  CheckCircle2,
  AlertCircle,
  XCircle,
  Plus,
  RefreshCw,
  Activity,
  ArrowRight,
  Clock,
  Upload,
  Download,
  Settings,
  TrendingUp,
  FileText,
  Shield,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"

// ── Types ────────────────────────────────────────────────────────────────────

type ConnectionStatus = "connected" | "warning" | "disconnected" | "not-configured"

interface DataSource {
  id: string
  name: string
  type: string
  href: string
  icon: React.ElementType
  color: string
  bgColor: string
  status: ConnectionStatus
  statusLabel: string
  description: string
  stats: { label: string; value: string }[]
}

interface RecentActivity {
  id: string
  action: string
  target: string
  source: string
  time: string
  icon: React.ElementType
  color: string
}

// ── Mock data ────────────────────────────────────────────────────────────────

const dataSources: DataSource[] = [
  {
    id: "storage",
    name: "Azure Storage",
    type: "Blob & File Storage",
    href: "/data/storage",
    icon: HardDrive,
    color: "text-blue-400",
    bgColor: "bg-blue-500/15",
    status: "connected",
    statusLabel: "Connected",
    description: "Browse containers, blobs, and file shares for your AI training data.",
    stats: [
      { label: "Containers", value: "12" },
      { label: "Storage Used", value: "2.4 TB" },
      { label: "Files", value: "184K" },
    ],
  },
  {
    id: "sql",
    name: "Azure SQL",
    type: "Relational Database",
    href: "/data/sql",
    icon: Table2,
    color: "text-emerald-400",
    bgColor: "bg-emerald-500/15",
    status: "connected",
    statusLabel: "Connected",
    description: "Query structured healthcare data across managed SQL databases.",
    stats: [
      { label: "Databases", value: "4" },
      { label: "Tables", value: "231" },
      { label: "Rows", value: "8.2M" },
    ],
  },
  {
    id: "cosmos",
    name: "Cosmos DB",
    type: "NoSQL Document DB",
    href: "/data/cosmos",
    icon: Database,
    color: "text-orange-400",
    bgColor: "bg-orange-500/15",
    status: "warning",
    statusLabel: "Provisioning",
    description: "Explore NoSQL containers powering the marketplace runtime data.",
    stats: [
      { label: "Containers", value: "10" },
      { label: "Documents", value: "—" },
      { label: "RU/s", value: "Serverless" },
    ],
  },
  {
    id: "fabric",
    name: "Microsoft Fabric",
    type: "Unified Analytics",
    href: "/data/fabric",
    icon: Layers,
    color: "text-violet-400",
    bgColor: "bg-violet-500/15",
    status: "not-configured",
    statusLabel: "Not configured",
    description: "Connect Fabric workspaces, lakehouses, and warehouses for large-scale ML.",
    stats: [
      { label: "Workspaces", value: "—" },
      { label: "Lakehouses", value: "—" },
      { label: "Pipelines", value: "—" },
    ],
  },
  {
    id: "datasets",
    name: "Built-in Datasets",
    type: "Curated Dataset Hub",
    href: "/data/datasets",
    icon: Sparkles,
    color: "text-sky-400",
    bgColor: "bg-sky-500/15",
    status: "connected",
    statusLabel: "48 datasets",
    description: "Curated healthcare, NLP, and tabular datasets ready for training and evaluation.",
    stats: [
      { label: "Datasets", value: "48" },
      { label: "Categories", value: "8" },
      { label: "Avg Size", value: "1.2 GB" },
    ],
  },
]

const recentActivity: RecentActivity[] = [
  { id: "1", action: "Downloaded", target: "claims-training-v3.parquet", source: "Azure Storage", time: "2 min ago", icon: Download, color: "text-blue-400" },
  { id: "2", action: "Queried", target: "encounters table — 12,400 rows", source: "Azure SQL", time: "18 min ago", icon: Table2, color: "text-emerald-400" },
  { id: "3", action: "Used dataset", target: "Healthcare NLP Corpus v2", source: "Built-in Datasets", time: "1 hr ago", icon: Sparkles, color: "text-sky-400" },
  { id: "4", action: "Uploaded", target: "model-outputs/batch-20260302.json", source: "Azure Storage", time: "3 hr ago", icon: Upload, color: "text-blue-400" },
  { id: "5", action: "Executed", target: "SELECT * FROM member_eligibility LIMIT 500", source: "Azure SQL", time: "5 hr ago", icon: Table2, color: "text-emerald-400" },
  { id: "6", action: "Browsed", target: "COSMOS assets container (24 items)", source: "Cosmos DB", time: "Yesterday", icon: Database, color: "text-orange-400" },
]

const quickStats = [
  { label: "Sources Connected", value: "3 / 5", delta: null, icon: CheckCircle2, color: "text-emerald-400", bg: "bg-emerald-500/10" },
  { label: "Total Datasets", value: "48", delta: "+3 this week", icon: Sparkles, color: "text-sky-400", bg: "bg-sky-500/10" },
  { label: "Storage Used", value: "2.4 TB", delta: "+120 GB today", icon: HardDrive, color: "text-blue-400", bg: "bg-blue-500/10" },
  { label: "Queries Today", value: "142", delta: "↑ 18% vs yesterday", icon: Activity, color: "text-violet-400", bg: "bg-violet-500/10" },
]

// ── Status helpers ─────────────────────────────────────────────────────────

function StatusIcon({ status }: { status: ConnectionStatus }) {
  if (status === "connected")
    return <CheckCircle2 className="h-4 w-4 text-emerald-400" />
  if (status === "warning")
    return <AlertCircle className="h-4 w-4 text-amber-400" />
  if (status === "disconnected")
    return <XCircle className="h-4 w-4 text-red-400" />
  return <XCircle className="h-4 w-4 text-muted-foreground" />
}

function statusBadgeVariant(status: ConnectionStatus) {
  if (status === "connected") return "bg-emerald-500/15 text-emerald-400"
  if (status === "warning") return "bg-amber-500/15 text-amber-400"
  if (status === "disconnected") return "bg-red-500/15 text-red-400"
  return "bg-secondary text-muted-foreground"
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function DataHubPage() {
  const [refreshing, setRefreshing] = useState(false)

  const handleRefresh = () => {
    setRefreshing(true)
    setTimeout(() => setRefreshing(false), 1200)
  }

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 p-6">
        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Database className="h-5 w-5 text-sky-400" />
              <h1 className="text-2xl font-bold text-foreground">Data Hub</h1>
            </div>
            <p className="text-sm text-muted-foreground">
              Unified access to storage, SQL, Cosmos DB, Microsoft Fabric, and curated datasets.
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleRefresh} className="gap-2">
              <RefreshCw className={cn("h-4 w-4", refreshing && "animate-spin")} />
              Refresh
            </Button>
            <Button size="sm" className="gap-2 bg-sky-600 hover:bg-sky-700 text-white">
              <Plus className="h-4 w-4" />
              Add Connection
            </Button>
          </div>
        </div>

        {/* Quick stats */}
        <div className="mb-6 grid grid-cols-4 gap-4">
          {quickStats.map((stat) => {
            const Icon = stat.icon
            return (
              <Card key={stat.label} className="border-border bg-card">
                <CardContent className="p-4">
                  <div className="flex items-center gap-3">
                    <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", stat.bg)}>
                      <Icon className={cn("h-5 w-5", stat.color)} />
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">{stat.label}</p>
                      <p className="text-xl font-bold text-foreground">{stat.value}</p>
                      {stat.delta && <p className="text-xs text-muted-foreground">{stat.delta}</p>}
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>

        <div className="grid grid-cols-3 gap-6">
          {/* Data source cards — 2/3 width */}
          <div className="col-span-2 space-y-4">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">Data Sources</h2>
            <div className="grid grid-cols-2 gap-4">
              {dataSources.map((source) => {
                const Icon = source.icon
                return (
                  <Link href={source.href} key={source.id}>
                    <Card className="group cursor-pointer border-border bg-card transition-all hover:border-sky-500/40 hover:shadow-lg hover:shadow-sky-500/5">
                      <CardHeader className="pb-2">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-3">
                            <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", source.bgColor)}>
                              <Icon className={cn("h-5 w-5", source.color)} />
                            </div>
                            <div>
                              <CardTitle className="text-sm font-semibold">{source.name}</CardTitle>
                              <CardDescription className="text-xs">{source.type}</CardDescription>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5">
                            <StatusIcon status={source.status} />
                            <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", statusBadgeVariant(source.status))}>
                              {source.statusLabel}
                            </span>
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent className="pb-3">
                        <p className="mb-3 text-xs text-muted-foreground leading-relaxed">{source.description}</p>
                        <div className="flex gap-4">
                          {source.stats.map((s) => (
                            <div key={s.label}>
                              <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{s.label}</p>
                              <p className="text-sm font-semibold text-foreground">{s.value}</p>
                            </div>
                          ))}
                        </div>
                        <div className="mt-3 flex items-center gap-1 text-xs font-medium text-sky-400 opacity-0 transition-all group-hover:opacity-100">
                          Browse {source.name}
                          <ArrowRight className="h-3 w-3" />
                        </div>
                      </CardContent>
                    </Card>
                  </Link>
                )
              })}
            </div>
          </div>

          {/* Recent activity — 1/3 width */}
          <div className="space-y-4">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">Recent Activity</h2>
            <Card className="border-border bg-card">
              <CardContent className="p-0">
                <ul className="divide-y divide-border">
                  {recentActivity.map((item) => {
                    const Icon = item.icon
                    return (
                      <li key={item.id} className="flex items-start gap-3 px-4 py-3">
                        <div className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded bg-secondary/60">
                          <Icon className={cn("h-3.5 w-3.5", item.color)} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <p className="text-xs text-foreground">
                            <span className="font-medium">{item.action}</span>
                          </p>
                          <p className="truncate text-xs text-muted-foreground">{item.target}</p>
                          <div className="mt-0.5 flex items-center gap-1.5">
                            <span className="text-[10px] text-muted-foreground/60">{item.source}</span>
                            <span className="text-[10px] text-muted-foreground/40">·</span>
                            <span className="flex items-center gap-0.5 text-[10px] text-muted-foreground/60">
                              <Clock className="h-2.5 w-2.5" />
                              {item.time}
                            </span>
                          </div>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              </CardContent>
            </Card>

            {/* Security notice */}
            <Card className="border-border bg-card">
              <CardContent className="p-4">
                <div className="flex items-start gap-3">
                  <Shield className="mt-0.5 h-5 w-5 shrink-0 text-emerald-400" />
                  <div>
                    <p className="text-xs font-semibold text-foreground">Secure by default</p>
                    <p className="mt-0.5 text-xs text-muted-foreground leading-relaxed">
                      All connections use managed identity and Azure RBAC. No credentials stored.
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </main>
    </div>
  )
}
