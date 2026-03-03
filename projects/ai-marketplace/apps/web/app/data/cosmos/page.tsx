"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Database,
  ChevronRight,
  ChevronDown,
  Search,
  Play,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Download,
  Copy,
  Plus,
  Eye,
  Clock,
  Zap,
  FileJson,
  Info,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"

// ── Types ──────────────────────────────────────────────────────────────────────

interface CosmosContainer {
  id: string
  partitionKey: string
  throughput: string
  ttl?: string
  itemCount: number
  items: CosmosItem[]
}

interface CosmosItem {
  id: string
  [key: string]: unknown
}

// ── Mock data ──────────────────────────────────────────────────────────────────

const DB_NAME = "ai-marketplace"
const ACCOUNT = "ai-marketplace-cosmos-p7a65r22uhdxo"

const containers: CosmosContainer[] = [
  {
    id: "assets",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    itemCount: 48,
    items: [
      { id: "asset-001", tenantId: "optum", name: "Claims Denial Agent v2", type: "agent", status: "published", publisherId: "optum-rcm", createdAt: "2026-02-15T10:00:00Z" },
      { id: "asset-002", tenantId: "optum", name: "Eligibility Verifier", type: "agent", status: "published", publisherId: "optum-rcm", createdAt: "2026-01-20T09:30:00Z" },
      { id: "asset-003", tenantId: "optum", name: "GPT-4o (Azure AI Foundry)", type: "model", status: "active", endpoint: "https://...", createdAt: "2026-01-10T08:00:00Z" },
    ],
  },
  {
    id: "publishers",
    partitionKey: "/publisherId",
    throughput: "Serverless",
    itemCount: 6,
    items: [
      { id: "optum-rcm", publisherId: "optum-rcm", name: "Optum RCM Team", verified: true, assetCount: 12, createdAt: "2025-12-01T00:00:00Z" },
      { id: "openai-azure", publisherId: "openai-azure", name: "Azure OpenAI Service", verified: true, assetCount: 8, createdAt: "2025-12-01T00:00:00Z" },
    ],
  },
  {
    id: "sessions",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    ttl: "86400s (24h)",
    itemCount: 214,
    items: [
      { id: "sess-abc123", tenantId: "optum", userId: "user-001", agentId: "asset-001", startedAt: "2026-03-02T14:00:00Z", messageCount: 8, status: "active" },
      { id: "sess-def456", tenantId: "optum", userId: "user-002", agentId: "asset-002", startedAt: "2026-03-02T12:00:00Z", messageCount: 3, status: "ended" },
    ],
  },
  {
    id: "audit-log",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    ttl: "7776000s (90d)",
    itemCount: 4200,
    items: [
      { id: "aud-001", tenantId: "optum", action: "asset.published", actorId: "user-001", resourceId: "asset-001", timestamp: "2026-03-02T11:00:00Z" },
      { id: "aud-002", tenantId: "optum", action: "session.started", actorId: "user-002", resourceId: "sess-abc123", timestamp: "2026-03-02T14:00:00Z" },
    ],
  },
  {
    id: "ratings",
    partitionKey: "/assetId",
    throughput: "Serverless",
    itemCount: 312,
    items: [
      { id: "rat-001", assetId: "asset-001", userId: "user-001", rating: 5, comment: "Excellent agent!", createdAt: "2026-03-01T10:00:00Z" },
    ],
  },
  {
    id: "workflows",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    itemCount: 18,
    items: [
      { id: "wf-001", tenantId: "optum", name: "Claims Processing Pipeline", steps: 4, status: "active", lastRun: "2026-03-02T13:00:00Z" },
    ],
  },
  {
    id: "submissions",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    itemCount: 34,
    items: [
      { id: "sub-001", tenantId: "optum", assetId: "asset-xyz", submittedBy: "user-003", stage: "review", submittedAt: "2026-03-01T09:00:00Z" },
    ],
  },
  {
    id: "projects",
    partitionKey: "/tenantId",
    throughput: "Serverless",
    itemCount: 9,
    items: [
      { id: "proj-001", tenantId: "optum", name: "Q1 Denial Reduction", status: "active", assetIds: ["asset-001", "asset-002"], createdAt: "2026-01-05T00:00:00Z" },
    ],
  },
  {
    id: "version-pins",
    partitionKey: "/projectId",
    throughput: "Serverless",
    itemCount: 22,
    items: [
      { id: "vp-001", projectId: "proj-001", assetId: "asset-001", pinnedVersion: "2.1.0", pinnedAt: "2026-02-01T00:00:00Z" },
    ],
  },
  {
    id: "user-config",
    partitionKey: "/userId",
    throughput: "Serverless",
    itemCount: 47,
    items: [
      { id: "uc-001", userId: "user-001", theme: "dark", defaultTenantId: "optum", starredAssets: ["asset-001", "asset-003"], updatedAt: "2026-03-01T12:00:00Z" },
    ],
  },
]

const defaultQuery = `SELECT * FROM c
WHERE c.tenantId = "optum"
ORDER BY c._ts DESC
OFFSET 0 LIMIT 20`

// ── Page ──────────────────────────────────────────────────────────────────────

export default function CosmosDbPage() {
  const [selectedContainer, setSelectedContainer] = useState<string>("assets")
  const [selectedItem, setSelectedItem] = useState<CosmosItem | null>(null)
  const [query, setQuery] = useState(defaultQuery)
  const [activeTab, setActiveTab] = useState<"browse" | "query">("browse")
  const [running, setRunning] = useState(false)
  const [queryResults, setQueryResults] = useState<CosmosItem[] | null>(null)
  const [search, setSearch] = useState("")

  const container = containers.find((c) => c.id === selectedContainer)
  const filteredItems = (queryResults ?? container?.items ?? []).filter(
    (item) =>
      !search ||
      JSON.stringify(item).toLowerCase().includes(search.toLowerCase())
  )

  const runQuery = () => {
    setRunning(true)
    setTimeout(() => {
      setQueryResults(container?.items ?? [])
      setRunning(false)
    }, 600)
  }

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 flex h-screen flex-col overflow-hidden p-6 pb-0">
        {/* Header */}
        <div className="mb-4 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Database className="h-5 w-5 text-orange-400" />
              <h1 className="text-2xl font-bold text-foreground">Cosmos DB</h1>
              <Badge className="bg-amber-500/15 text-amber-400">
                <AlertCircle className="mr-1 h-3 w-3" />
                Provisioning
              </Badge>
            </div>
            <p className="text-sm text-muted-foreground">
              Browse containers and items in <span className="font-mono text-xs text-orange-400">{ACCOUNT}</span>
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-1.5 text-xs text-amber-400">
              <Info className="mr-1.5 inline h-3.5 w-3.5" />
              Account fixing in progress — showing last-known state
            </div>
            <Button variant="outline" size="sm" className="gap-2"><RefreshCw className="h-3.5 w-3.5" /> Refresh</Button>
          </div>
        </div>

        {/* Account stats */}
        <div className="mb-4 grid grid-cols-4 gap-3">
          <Card className="border-border bg-card">
            <CardContent className="flex items-center gap-3 p-3">
              <Database className="h-8 w-8 text-orange-400/70" />
              <div>
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Database</p>
                <p className="text-sm font-semibold text-foreground">{DB_NAME}</p>
              </div>
            </CardContent>
          </Card>
          <Card className="border-border bg-card">
            <CardContent className="flex items-center gap-3 p-3">
              <div className="h-8 w-8 rounded-lg bg-orange-500/15 flex items-center justify-center">
                <span className="text-sm font-bold text-orange-400">{containers.length}</span>
              </div>
              <div>
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Containers</p>
                <p className="text-sm font-semibold text-foreground">10 configured</p>
              </div>
            </CardContent>
          </Card>
          <Card className="border-border bg-card">
            <CardContent className="flex items-center gap-3 p-3">
              <Zap className="h-8 w-8 text-amber-400/70" />
              <div>
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Throughput</p>
                <p className="text-sm font-semibold text-foreground">Serverless</p>
              </div>
            </CardContent>
          </Card>
          <Card className="border-border bg-card">
            <CardContent className="flex items-center gap-3 p-3">
              <FileJson className="h-8 w-8 text-sky-400/70" />
              <div>
                <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Total Items</p>
                <p className="text-sm font-semibold text-foreground">{containers.reduce((s, c) => s + c.itemCount, 0).toLocaleString()}</p>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-1 gap-4 overflow-hidden pb-6">
          {/* Left: container list */}
          <div className="flex w-56 shrink-0 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2.5">
              <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Containers</p>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              {containers.map((c) => (
                <button
                  key={c.id}
                  onClick={() => { setSelectedContainer(c.id); setSelectedItem(null); setQueryResults(null) }}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-xs transition-colors",
                    selectedContainer === c.id
                      ? "bg-orange-500/15 text-orange-300 font-medium"
                      : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                  )}
                >
                  <Database className={cn("h-3.5 w-3.5 shrink-0", selectedContainer === c.id ? "text-orange-400" : "text-muted-foreground")} />
                  <span className="flex-1 truncate text-left">{c.id}</span>
                  <span className="text-[10px] text-muted-foreground/50">{c.itemCount}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Right: details */}
          <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
            {/* Tabs */}
            <div className="flex items-center justify-between border-b border-border px-2">
              <div className="flex">
                {(["browse", "query"] as const).map((tab) => (
                  <button
                    key={tab}
                    onClick={() => setActiveTab(tab)}
                    className={cn(
                      "px-4 py-2.5 text-xs font-medium capitalize transition-colors",
                      activeTab === tab
                        ? "border-b-2 border-orange-400 text-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                  >
                    {tab === "browse" ? "Items" : "Query Explorer"}
                  </button>
                ))}
              </div>
              {container && (
                <div className="flex items-center gap-3 pr-3 text-[10px] text-muted-foreground">
                  <span>Partition: <code className="text-orange-400">{container.partitionKey}</code></span>
                  {container.ttl && <span>TTL: <code className="text-amber-400">{container.ttl}</code></span>}
                  <span>Mode: <code className="text-emerald-400">{container.throughput}</code></span>
                </div>
              )}
            </div>

            {activeTab === "browse" ? (
              <div className="flex flex-1 overflow-hidden">
                {/* Item list */}
                <div className="flex w-72 shrink-0 flex-col border-r border-border overflow-hidden">
                  <div className="border-b border-border p-2">
                    <div className="relative">
                      <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                      <Input className="h-7 pl-8 text-xs bg-secondary border-0" placeholder="Filter items…" value={search} onChange={(e) => setSearch(e.target.value)} />
                    </div>
                  </div>
                  <div className="flex-1 overflow-y-auto divide-y divide-border">
                    {filteredItems.map((item) => (
                      <button
                        key={item.id}
                        onClick={() => setSelectedItem(item)}
                        className={cn(
                          "flex w-full flex-col gap-0.5 px-3 py-2.5 text-left text-xs transition-colors",
                          selectedItem?.id === item.id
                            ? "bg-orange-500/10 text-orange-300"
                            : "hover:bg-secondary/30 text-foreground"
                        )}
                      >
                        <div className="flex items-center gap-2">
                          <FileJson className="h-3.5 w-3.5 shrink-0 text-orange-400/70" />
                          <span className="font-mono font-medium truncate">{item.id}</span>
                        </div>
                        <p className="truncate pl-5 text-[10px] text-muted-foreground">
                          {Object.entries(item).filter(([k]) => k !== "id").slice(0, 2).map(([k, v]) => `${k}: ${v}`).join("  ·  ")}
                        </p>
                      </button>
                    ))}
                  </div>
                </div>

                {/* Item JSON viewer */}
                <div className="flex flex-1 flex-col overflow-hidden">
                  {selectedItem ? (
                    <>
                      <div className="flex items-center justify-between border-b border-border px-4 py-2">
                        <span className="font-mono text-xs font-medium text-foreground">{selectedItem.id}</span>
                        <Button variant="ghost" size="sm" className="gap-1.5 text-xs text-muted-foreground">
                          <Copy className="h-3.5 w-3.5" /> Copy JSON
                        </Button>
                      </div>
                      <pre className="flex-1 overflow-auto p-4 text-xs font-mono text-foreground leading-relaxed">
                        {JSON.stringify(selectedItem, null, 2)}
                      </pre>
                    </>
                  ) : (
                    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                      Select an item to view its JSON
                    </div>
                  )}
                </div>
              </div>
            ) : (
              <div className="flex flex-1 flex-col overflow-hidden">
                <div className="border-b border-border p-4">
                  <Textarea
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    className="h-28 resize-none font-mono text-xs bg-secondary/30 border-border"
                    spellCheck={false}
                  />
                  <div className="mt-2 flex items-center gap-2">
                    <Button size="sm" onClick={runQuery} disabled={running} className="gap-2 bg-orange-600 hover:bg-orange-700 text-white">
                      {running ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                      {running ? "Running…" : "Execute"}
                    </Button>
                    <span className="text-xs text-muted-foreground">SQL API (Cosmos DB)</span>
                    {queryResults && (
                      <span className="ml-auto text-xs text-emerald-400"><CheckCircle2 className="mr-1 inline h-3.5 w-3.5" />{queryResults.length} results</span>
                    )}
                  </div>
                </div>
                {queryResults && (
                  <div className="flex-1 overflow-auto p-4">
                    <pre className="text-xs font-mono text-foreground leading-relaxed">
                      {JSON.stringify(queryResults, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
