"use client"

import { useState, useRef } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Table2,
  ChevronRight,
  ChevronDown,
  Search,
  Play,
  Database,
  Server,
  Layers,
  Clock,
  CheckCircle2,
  AlertCircle,
  Download,
  Copy,
  RefreshCw,
  Save,
  X,
  Plus,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"

// ── Types ─────────────────────────────────────────────────────────────────────

interface SqlServer {
  name: string
  tier: string
  region: string
  status: "online" | "offline"
  databases: SqlDatabase[]
}

interface SqlDatabase {
  name: string
  tables: SqlTable[]
}

interface SqlTable {
  name: string
  schema: string
  rows: number
  columns: SqlColumn[]
}

interface SqlColumn {
  name: string
  type: string
  nullable: boolean
  key?: "PK" | "FK"
}

interface QueryResult {
  columns: string[]
  rows: Record<string, string | number | null>[]
  rowCount: number
  elapsed: number
}

// ── Mock data ──────────────────────────────────────────────────────────────────

const servers: SqlServer[] = [
  {
    name: "rcm-sql-prod.database.windows.net",
    tier: "General Purpose — 4 vCores",
    region: "East US",
    status: "online",
    databases: [
      {
        name: "rcm_core",
        tables: [
          {
            name: "member_eligibility",
            schema: "dbo",
            rows: 2_840_000,
            columns: [
              { name: "member_id", type: "nvarchar(36)", nullable: false, key: "PK" },
              { name: "payer_id", type: "nvarchar(20)", nullable: false },
              { name: "plan_name", type: "nvarchar(100)", nullable: true },
              { name: "effective_date", type: "date", nullable: false },
              { name: "termination_date", type: "date", nullable: true },
              { name: "check_date", type: "datetime2", nullable: false },
              { name: "status", type: "nvarchar(12)", nullable: false },
            ],
          },
          {
            name: "claims",
            schema: "dbo",
            rows: 4_100_000,
            columns: [
              { name: "claim_id", type: "nvarchar(36)", nullable: false, key: "PK" },
              { name: "member_id", type: "nvarchar(36)", nullable: false, key: "FK" },
              { name: "provider_npi", type: "nvarchar(10)", nullable: false },
              { name: "service_date", type: "date", nullable: false },
              { name: "total_billed", type: "decimal(10,2)", nullable: false },
              { name: "allowed_amount", type: "decimal(10,2)", nullable: true },
              { name: "denial_code", type: "nvarchar(8)", nullable: true },
              { name: "status", type: "nvarchar(20)", nullable: false },
            ],
          },
          {
            name: "denials",
            schema: "dbo",
            rows: 560_000,
            columns: [
              { name: "denial_id", type: "nvarchar(36)", nullable: false, key: "PK" },
              { name: "claim_id", type: "nvarchar(36)", nullable: false, key: "FK" },
              { name: "remark_code", type: "nvarchar(8)", nullable: false },
              { name: "denial_reason", type: "nvarchar(255)", nullable: true },
              { name: "denied_amount", type: "decimal(10,2)", nullable: false },
              { name: "appeal_status", type: "nvarchar(20)", nullable: true },
            ],
          },
          {
            name: "providers",
            schema: "dbo",
            rows: 48_000,
            columns: [
              { name: "provider_npi", type: "nvarchar(10)", nullable: false, key: "PK" },
              { name: "name", type: "nvarchar(150)", nullable: false },
              { name: "specialty", type: "nvarchar(80)", nullable: true },
              { name: "state", type: "char(2)", nullable: false },
            ],
          },
        ],
      },
      {
        name: "audit_log",
        tables: [
          {
            name: "agent_runs",
            schema: "dbo",
            rows: 92_000,
            columns: [
              { name: "run_id", type: "nvarchar(36)", nullable: false, key: "PK" },
              { name: "agent_id", type: "nvarchar(36)", nullable: false },
              { name: "started_at", type: "datetime2", nullable: false },
              { name: "duration_ms", type: "int", nullable: false },
              { name: "status", type: "nvarchar(12)", nullable: false },
            ],
          },
        ],
      },
    ],
  },
]

const sampleQueries: Record<string, string> = {
  "Top 10 denial patterns":
    `SELECT remark_code, denial_reason, COUNT(*) AS denial_count,
       SUM(denied_amount) AS total_denied
FROM dbo.denials
GROUP BY remark_code, denial_reason
ORDER BY denial_count DESC
OFFSET 0 ROWS FETCH NEXT 10 ROWS ONLY;`,

  "Eligibility check by payer":
    `SELECT payer_id, COUNT(*) AS checks,
       SUM(CASE WHEN status = 'Active' THEN 1 ELSE 0 END) AS active
FROM dbo.member_eligibility
GROUP BY payer_id
ORDER BY checks DESC;`,

  "Claims by status":
    `SELECT status, COUNT(*) AS count,
       SUM(total_billed) AS billed,
       SUM(allowed_amount) AS allowed
FROM dbo.claims
GROUP BY status
ORDER BY count DESC;`,
}

const mockResults: QueryResult = {
  columns: ["remark_code", "denial_reason", "denial_count", "total_denied"],
  rows: [
    { remark_code: "CO-4", denial_reason: "Service incompatible with diagnosis", denial_count: 12840, total_denied: 4200000.0 },
    { remark_code: "PR-96", denial_reason: "Non-covered charge", denial_count: 9210, total_denied: 3100000.0 },
    { remark_code: "CO-18", denial_reason: "Duplicate claim/service", denial_count: 7880, total_denied: 1850000.0 },
    { remark_code: "CO-11", denial_reason: "Dx inconsistent with procedure", denial_count: 6120, total_denied: 1700000.0 },
    { remark_code: "PI-4", denial_reason: "Auth required, not obtained", denial_count: 5400, total_denied: 2800000.0 },
  ],
  rowCount: 5,
  elapsed: 284,
}

// ── Components ────────────────────────────────────────────────────────────────

function formatRows(n: number) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M rows`
  if (n >= 1_000) return `${(n / 1000).toFixed(0)}K rows`
  return `${n} rows`
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AzureSqlPage() {
  const [expandedServers, setExpandedServers] = useState<Set<string>>(new Set(["rcm-sql-prod.database.windows.net"]))
  const [expandedDbs, setExpandedDbs] = useState<Set<string>>(new Set(["rcm-sql-prod.database.windows.net/rcm_core"]))
  const [selectedTable, setSelectedTable] = useState<{ server: string; db: string; table: string } | null>({
    server: "rcm-sql-prod.database.windows.net",
    db: "rcm_core",
    table: "denials",
  })
  const [query, setQuery] = useState(sampleQueries["Top 10 denial patterns"])
  const [results, setResults] = useState<QueryResult | null>(null)
  const [running, setRunning] = useState(false)
  const [activeTab, setActiveTab] = useState<"schema" | "query">("query")

  const runQuery = () => {
    setRunning(true)
    setTimeout(() => {
      setResults(mockResults)
      setRunning(false)
    }, 800)
  }

  const selectedTableData = (() => {
    if (!selectedTable) return null
    const srv = servers.find((s) => s.name === selectedTable.server)
    const db = srv?.databases.find((d) => d.name === selectedTable.db)
    return db?.tables.find((t) => t.name === selectedTable.table) ?? null
  })()

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 flex h-screen flex-col overflow-hidden p-6 pb-0">
        {/* Header */}
        <div className="mb-4 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Table2 className="h-5 w-5 text-emerald-400" />
              <h1 className="text-2xl font-bold text-foreground">Azure SQL</h1>
              <Badge className="bg-emerald-500/15 text-emerald-400">Connected</Badge>
            </div>
            <p className="text-sm text-muted-foreground">Browse schemas and run ad-hoc queries against managed SQL databases.</p>
          </div>
          <Button variant="outline" size="sm" className="gap-2"><RefreshCw className="h-3.5 w-3.5" /> Refresh</Button>
        </div>

        <div className="flex flex-1 gap-4 overflow-hidden pb-6">
          {/* Left: Server / DB / Table tree */}
          <div className="flex w-64 shrink-0 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2.5">
              <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Servers</p>
            </div>
            <div className="flex-1 overflow-y-auto p-2 text-xs">
              {servers.map((server) => {
                const sExpanded = expandedServers.has(server.name)
                return (
                  <div key={server.name}>
                    <button
                      onClick={() => setExpandedServers((prev) => { const n = new Set(prev); n.has(server.name) ? n.delete(server.name) : n.add(server.name); return n })}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 font-medium text-foreground hover:bg-secondary/50"
                    >
                      {sExpanded ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                      <Server className="h-3.5 w-3.5 shrink-0 text-emerald-400" />
                      <span className="truncate text-[11px]">{server.name.split(".")[0]}</span>
                      <span className={cn("ml-auto h-2 w-2 shrink-0 rounded-full", server.status === "online" ? "bg-emerald-400" : "bg-red-400")} />
                    </button>
                    {sExpanded && server.databases.map((db) => {
                      const dbKey = `${server.name}/${db.name}`
                      const dbExpanded = expandedDbs.has(dbKey)
                      return (
                        <div key={dbKey} className="ml-4">
                          <button
                            onClick={() => setExpandedDbs((prev) => { const n = new Set(prev); n.has(dbKey) ? n.delete(dbKey) : n.add(dbKey); return n })}
                            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                          >
                            {dbExpanded ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
                            <Database className="h-3.5 w-3.5 shrink-0 text-blue-400" />
                            <span className="truncate">{db.name}</span>
                          </button>
                          {dbExpanded && db.tables.map((table) => {
                            const isSelected = selectedTable?.server === server.name && selectedTable?.db === db.name && selectedTable?.table === table.name
                            return (
                              <button
                                key={table.name}
                                onClick={() => setSelectedTable({ server: server.name, db: db.name, table: table.name })}
                                className={cn(
                                  "ml-4 flex w-full items-center gap-2 rounded-md px-2 py-1.5 transition-colors",
                                  isSelected ? "bg-emerald-500/15 text-emerald-300 font-medium" : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                                )}
                              >
                                <Table2 className="h-3 w-3 shrink-0 text-emerald-400/70" />
                                <span className="truncate text-[11px]">{table.name}</span>
                                <span className="ml-auto text-[10px] text-muted-foreground/50">{formatRows(table.rows)}</span>
                              </button>
                            )
                          })}
                        </div>
                      )
                    })}
                  </div>
                )
              })}
            </div>
          </div>

          {/* Right: schema + query */}
          <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
            {/* Tabs */}
            <div className="flex items-center border-b border-border">
              {(["schema", "query"] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setActiveTab(tab)}
                  className={cn(
                    "px-4 py-2.5 text-xs font-medium capitalize transition-colors",
                    activeTab === tab
                      ? "border-b-2 border-emerald-400 text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {tab === "schema" ? `Schema${selectedTableData ? ` — ${selectedTableData.name}` : ""}` : "Query Editor"}
                </button>
              ))}
            </div>

            {activeTab === "schema" ? (
              <div className="flex-1 overflow-auto p-4">
                {selectedTableData ? (
                  <>
                    <div className="mb-4 flex items-center gap-3">
                      <Table2 className="h-5 w-5 text-emerald-400" />
                      <div>
                        <p className="text-sm font-semibold text-foreground">{selectedTable?.db}.{selectedTableData.schema}.{selectedTableData.name}</p>
                        <p className="text-xs text-muted-foreground">{formatRows(selectedTableData.rows)} · {selectedTableData.columns.length} columns</p>
                      </div>
                      <Button variant="outline" size="sm" className="ml-auto gap-2 text-xs" onClick={() => {
                        setQuery(`SELECT TOP 100 *\nFROM ${selectedTableData.schema}.${selectedTableData.name};`)
                        setActiveTab("query")
                      }}>
                        <Play className="h-3 w-3" /> Query this table
                      </Button>
                    </div>
                    <table className="w-full text-xs">
                      <thead className="text-left text-muted-foreground">
                        <tr className="border-b border-border">
                          <th className="pb-2 pr-4 font-medium">Column</th>
                          <th className="pb-2 pr-4 font-medium">Type</th>
                          <th className="pb-2 pr-4 font-medium">Nullable</th>
                          <th className="pb-2 font-medium">Key</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border">
                        {selectedTableData.columns.map((col) => (
                          <tr key={col.name}>
                            <td className="py-2 pr-4 font-medium text-foreground">{col.name}</td>
                            <td className="py-2 pr-4 font-mono text-muted-foreground">{col.type}</td>
                            <td className="py-2 pr-4 text-muted-foreground">{col.nullable ? "YES" : "NO"}</td>
                            <td className="py-2">
                              {col.key && (
                                <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium",
                                  col.key === "PK" ? "bg-amber-500/15 text-amber-400" : "bg-blue-500/15 text-blue-400"
                                )}>{col.key}</span>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">Select a table to view its schema.</p>
                )}
              </div>
            ) : (
              <div className="flex flex-1 flex-col overflow-hidden">
                {/* Sample queries */}
                <div className="flex items-center gap-2 border-b border-border px-4 py-2">
                  <span className="text-[10px] uppercase tracking-wider text-muted-foreground">Samples:</span>
                  {Object.keys(sampleQueries).map((name) => (
                    <button
                      key={name}
                      onClick={() => setQuery(sampleQueries[name])}
                      className="rounded bg-secondary px-2 py-0.5 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {name}
                    </button>
                  ))}
                </div>

                {/* Query editor */}
                <div className="relative p-4" style={{ height: "180px" }}>
                  <Textarea
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    className="h-full resize-none font-mono text-xs bg-secondary/30 border-border"
                    spellCheck={false}
                  />
                </div>

                {/* Run button */}
                <div className="flex items-center gap-2 border-t border-border px-4 py-2">
                  <Button size="sm" onClick={runQuery} disabled={running} className="gap-2 bg-emerald-600 hover:bg-emerald-700 text-white">
                    {running ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                    {running ? "Running…" : "Run Query"}
                  </Button>
                  <span className="text-xs text-muted-foreground">Ctrl+Enter</span>
                  {results && (
                    <>
                      <span className="ml-auto flex items-center gap-1 text-xs text-emerald-400">
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        {results.rowCount} rows · {results.elapsed}ms
                      </span>
                      <Button variant="ghost" size="sm" className="gap-1.5 text-xs text-muted-foreground">
                        <Download className="h-3.5 w-3.5" /> Export CSV
                      </Button>
                    </>
                  )}
                </div>

                {/* Results */}
                {results && (
                  <div className="flex-1 overflow-auto border-t border-border">
                    <table className="w-full text-xs">
                      <thead className="sticky top-0 bg-card/95 border-b border-border text-left text-muted-foreground">
                        <tr>
                          {results.columns.map((col) => (
                            <th key={col} className="px-4 py-2 font-medium">{col}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border">
                        {results.rows.map((row, i) => (
                          <tr key={i} className="hover:bg-secondary/30">
                            {results.columns.map((col) => (
                              <td key={col} className="px-4 py-2 text-foreground">
                                {row[col] !== null ? String(row[col]) : <span className="text-muted-foreground/40">NULL</span>}
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
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
