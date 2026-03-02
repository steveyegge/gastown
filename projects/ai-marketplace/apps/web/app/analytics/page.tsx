"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  BarChart3,
  Clock,
  Users,
  TrendingUp,
  TrendingDown,
  CheckCircle2,
  AlertTriangle,
  Timer,
  Activity,
  Brain,
  Bot,
  Wrench,
  ChevronDown,
  Calendar,
  RefreshCw,
  ArrowUpRight,
  ArrowDownRight,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  LineChart,
  Line,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts"

// ─── Color palette (Optum brand) ─────────────────────────────────────────────
const C = {
  orange: "#ff612b",
  orangeLight: "#ff8c5a",
  blue: "#1b4f8a",
  blueLight: "#3b82f6",
  teal: "#06b6d4",
  emerald: "#34d399",
  amber: "#fbbf24",
  red: "#f87171",
  purple: "#a78bfa",
  grid: "#ffffff10",
  text: "#94a3b8",
}

// ─── Static mock data ─────────────────────────────────────────────────────────

// Time-to-complete: avg minutes per workflow stage
const stageTimeData = [
  { stage: "Document Intake", avgMin: 1.2, p95Min: 2.8 },
  { stage: "Model Inference", avgMin: 0.7, p95Min: 1.4 },
  { stage: "Confidence Check", avgMin: 0.3, p95Min: 0.6 },
  { stage: "Human Review Queue", avgMin: 38.4, p95Min: 112.0 },
  { stage: "Human Decision", avgMin: 6.2, p95Min: 22.5 },
  { stage: "Approval & Routing", avgMin: 0.8, p95Min: 1.9 },
  { stage: "Downstream Write", avgMin: 1.1, p95Min: 2.4 },
]

// Human-in-loop wait time trend (last 14 days)
const humanWaitTrend = [
  { day: "Feb 17", avgWait: 44, p95Wait: 128 },
  { day: "Feb 18", avgWait: 51, p95Wait: 141 },
  { day: "Feb 19", avgWait: 29, p95Wait: 88 },
  { day: "Feb 20", avgWait: 22, p95Wait: 64 },
  { day: "Feb 21", avgWait: 18, p95Wait: 52 },
  { day: "Feb 22", avgWait: 56, p95Wait: 158 },
  { day: "Feb 23", avgWait: 61, p95Wait: 172 },
  { day: "Feb 24", avgWait: 47, p95Wait: 133 },
  { day: "Feb 25", avgWait: 38, p95Wait: 107 },
  { day: "Feb 26", avgWait: 35, p95Wait: 99 },
  { day: "Feb 27", avgWait: 33, p95Wait: 91 },
  { day: "Feb 28", avgWait: 29, p95Wait: 84 },
  { day: "Mar 1", avgWait: 26, p95Wait: 77 },
  { day: "Mar 2", avgWait: 24, p95Wait: 71 },
]

// Workflow funnel
const funnelData = [
  { stage: "Submitted", count: 9420, fill: C.blueLight },
  { stage: "In Progress", count: 8104, fill: C.teal },
  { stage: "Awaiting Human", count: 1342, fill: C.amber },
  { stage: "Completed", count: 7881, fill: C.emerald },
  { stage: "Failed / Rejected", count: 195, fill: C.red },
]

// API calls over time (last 30 days, sampled every 3 days)
const apiCallsTrend = [
  { date: "Feb 1", models: 12400, agents: 4200, tools: 8900 },
  { date: "Feb 4", models: 13800, agents: 4900, tools: 9600 },
  { date: "Feb 7", models: 11200, agents: 5100, tools: 10200 },
  { date: "Feb 10", models: 15600, agents: 5800, tools: 11400 },
  { date: "Feb 13", models: 14900, agents: 6200, tools: 12100 },
  { date: "Feb 16", models: 16700, agents: 6800, tools: 13300 },
  { date: "Feb 19", models: 13200, agents: 5900, tools: 11800 },
  { date: "Feb 22", models: 17800, agents: 7400, tools: 14200 },
  { date: "Feb 25", models: 19200, agents: 8100, tools: 15600 },
  { date: "Feb 28", models: 20400, agents: 8900, tools: 16900 },
  { date: "Mar 1", models: 21800, agents: 9400, tools: 18100 },
  { date: "Mar 2", models: 22600, agents: 9900, tools: 18800 },
]

// Top assets by unique teams using them
const topAssetsData = [
  { name: "DenialPrediction-BERT", teams: 48, type: "Model" },
  { name: "RCM Denial Agent", teams: 41, type: "Agent" },
  { name: "ICD-10 Validator", teams: 39, type: "Tool" },
  { name: "MedicalCodingLLM", teams: 36, type: "Model" },
  { name: "Prior Auth Orchestrator", teams: 30, type: "Agent" },
  { name: "EHR Data MCP Server", teams: 27, type: "MCP" },
  { name: "FHIR Converter", teams: 24, type: "Tool" },
  { name: "FraudScoringXGB", teams: 19, type: "Model" },
]

// Marketplace adoption (cumulative published assets over time)
const adoptionData = [
  { month: "Aug '25", models: 4, agents: 2, tools: 5, mcpServers: 1 },
  { month: "Sep '25", models: 6, agents: 4, tools: 7, mcpServers: 2 },
  { month: "Oct '25", models: 9, agents: 6, tools: 10, mcpServers: 3 },
  { month: "Nov '25", models: 12, agents: 9, tools: 14, mcpServers: 4 },
  { month: "Dec '25", models: 14, agents: 12, tools: 17, mcpServers: 5 },
  { month: "Jan '26", models: 18, agents: 16, tools: 22, mcpServers: 6 },
  { month: "Feb '26", models: 24, agents: 21, tools: 28, mcpServers: 8 },
  { month: "Mar '26", models: 26, agents: 23, tools: 31, mcpServers: 9 },
]

// SLA compliance breakdown (human review SLA = 60 min target)
const slaData = [
  { name: "Within 30 min", value: 38, fill: C.emerald },
  { name: "30–60 min", value: 29, fill: C.teal },
  { name: "60–120 min", value: 22, fill: C.amber },
  { name: "> 120 min", value: 11, fill: C.red },
]

// Error rate + throughput trend
const errorTrend = [
  { date: "Feb 17", errorRate: 1.8, throughput: 420 },
  { date: "Feb 19", errorRate: 2.1, throughput: 390 },
  { date: "Feb 21", errorRate: 1.4, throughput: 460 },
  { date: "Feb 23", errorRate: 3.2, throughput: 370 },
  { date: "Feb 25", errorRate: 1.1, throughput: 510 },
  { date: "Feb 27", errorRate: 0.9, throughput: 540 },
  { date: "Mar 1", errorRate: 0.7, throughput: 580 },
  { date: "Mar 2", errorRate: 0.6, throughput: 602 },
]

// Human review backlog by asset type
const backlogData = [
  { type: "Denial Claims", pending: 214, resolved: 1821, slaBreached: 38 },
  { type: "Prior Auth", pending: 187, resolved: 1402, slaBreached: 29 },
  { type: "Medical Coding", pending: 143, resolved: 988, slaBreached: 14 },
  { type: "Eligibility", pending: 76, resolved: 1104, slaBreached: 8 },
  { type: "Audit Reviews", pending: 41, resolved: 671, slaBreached: 4 },
]

// ─── Sub-components ───────────────────────────────────────────────────────────

function KpiCard({
  label,
  value,
  sub,
  trend,
  trendValue,
  icon: Icon,
  iconColor,
}: {
  label: string
  value: string
  sub?: string
  trend?: "up" | "down"
  trendValue?: string
  icon: React.ElementType
  iconColor: string
}) {
  const trendUp = trend === "up"
  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <div className="mb-3 flex items-start justify-between">
        <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", iconColor)}>
          <Icon className="h-5 w-5" />
        </div>
        {trendValue && (
          <span
            className={cn(
              "flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
              trendUp
                ? "bg-emerald-500/15 text-emerald-400"
                : "bg-red-500/15 text-red-400"
            )}
          >
            {trendUp ? <ArrowUpRight className="h-3 w-3" /> : <ArrowDownRight className="h-3 w-3" />}
            {trendValue}
          </span>
        )}
      </div>
      <p className="text-2xl font-bold text-foreground">{value}</p>
      <p className="mt-0.5 text-sm font-medium text-foreground">{label}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-4 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
      {children}
    </h2>
  )
}

function ChartCard({
  title,
  subtitle,
  children,
  className,
}: {
  title: string
  subtitle?: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("rounded-xl border border-border bg-card p-5", className)}>
      <div className="mb-4">
        <p className="text-sm font-semibold text-foreground">{title}</p>
        {subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}
      </div>
      {children}
    </div>
  )
}

const ASSET_TYPE_COLORS: Record<string, string> = {
  Model: C.orange,
  Agent: C.blueLight,
  Tool: C.teal,
  MCP: C.purple,
}

const tooltipStyle = {
  backgroundColor: "#0f172a",
  border: "1px solid #1e293b",
  borderRadius: "8px",
  fontSize: "12px",
  color: "#e2e8f0",
}

// ─── Page ─────────────────────────────────────────────────────────────────────

const DATE_RANGES = ["Last 7 days", "Last 14 days", "Last 30 days", "Last 90 days"] as const

export default function AnalyticsPage() {
  const [dateRange, setDateRange] = useState<string>("Last 14 days")
  const [showRangeMenu, setShowRangeMenu] = useState(false)

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 p-6">

        {/* ── Header ── */}
        <div className="mb-6 flex items-start justify-between">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold text-foreground">
              <BarChart3 className="h-6 w-6 text-[var(--optum-orange)]" />
              Analytics
            </h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              Workflow timing, human-in-loop performance, usage &amp; adoption across the AI Asset Marketplace
            </p>
          </div>
          <div className="flex items-center gap-2">
            {/* Date range picker */}
            <div className="relative">
              <Button
                variant="outline"
                size="sm"
                className="gap-2"
                onClick={() => setShowRangeMenu((p) => !p)}
              >
                <Calendar className="h-3.5 w-3.5" />
                {dateRange}
                <ChevronDown className="h-3.5 w-3.5" />
              </Button>
              {showRangeMenu && (
                <div className="absolute right-0 top-full z-10 mt-1 w-44 overflow-hidden rounded-lg border border-border bg-card shadow-lg">
                  {DATE_RANGES.map((r) => (
                    <button
                      key={r}
                      className={cn(
                        "w-full px-3 py-2 text-left text-sm transition-colors hover:bg-secondary",
                        dateRange === r ? "text-[var(--optum-orange)]" : "text-foreground"
                      )}
                      onClick={() => { setDateRange(r); setShowRangeMenu(false) }}
                    >
                      {r}
                    </button>
                  ))}
                </div>
              )}
            </div>
            <Button variant="outline" size="sm" className="gap-2">
              <RefreshCw className="h-3.5 w-3.5" />
              Refresh
            </Button>
          </div>
        </div>

        {/* ── KPI row ── */}
        <div className="mb-8 grid grid-cols-4 gap-4">
          <KpiCard
            label="Avg Workflow Time"
            value="48.9 min"
            sub="End-to-end, all stages"
            trend="down"
            trendValue="−12% vs prior period"
            icon={Timer}
            iconColor="bg-[var(--optum-orange)]/20 text-[var(--optum-orange)]"
          />
          <KpiCard
            label="Human Review Wait"
            value="24 min"
            sub="Avg queue wait (today)"
            trend="down"
            trendValue="−47% vs 2 wks ago"
            icon={Clock}
            iconColor="bg-amber-500/20 text-amber-400"
          />
          <KpiCard
            label="SLA Compliance"
            value="67%"
            sub="Reviews resolved < 60 min"
            trend="up"
            trendValue="+8pp this month"
            icon={CheckCircle2}
            iconColor="bg-emerald-500/20 text-emerald-400"
          />
          <KpiCard
            label="Total API Calls (Mar)"
            value="22,600"
            sub="Models + Agents + Tools"
            trend="up"
            trendValue="+18% vs Feb"
            icon={Activity}
            iconColor="bg-[var(--uhg-blue)]/20 text-[var(--uhg-blue-light)]"
          />
        </div>

        {/* ── Section 1: Human-in-Loop Timing ── */}
        <SectionTitle>
          <Clock className="h-4 w-4 text-amber-400" />
          Human-in-Loop Workflow Timing
        </SectionTitle>

        <div className="mb-8 grid grid-cols-5 gap-4">
          {/* Stage breakdown — wider */}
          <ChartCard
            className="col-span-3"
            title="Average Time to Complete by Workflow Stage"
            subtitle="Where time is being spent — avg vs p95 latency (minutes)"
          >
            <ResponsiveContainer width="100%" height={280}>
              <BarChart
                data={stageTimeData}
                layout="vertical"
                margin={{ top: 0, right: 20, left: 130, bottom: 0 }}
              >
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} horizontal={false} />
                <XAxis
                  type="number"
                  tick={{ fill: C.text, fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  tickFormatter={(v) => `${v}m`}
                />
                <YAxis
                  type="category"
                  dataKey="stage"
                  tick={{ fill: C.text, fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  width={125}
                />
                <Tooltip
                  contentStyle={tooltipStyle}
                  formatter={(v: number, name: string) => [`${v} min`, name === "avgMin" ? "Avg" : "p95"]}
                />
                <Legend iconType="square" wrapperStyle={{ fontSize: 11, color: C.text }} />
                <Bar dataKey="avgMin" name="Avg" fill={C.orange} radius={[0, 3, 3, 0]} />
                <Bar dataKey="p95Min" name="p95" fill={C.blueLight} radius={[0, 3, 3, 0]} fillOpacity={0.7} />
              </BarChart>
            </ResponsiveContainer>
            <div className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-xs text-amber-300">
              <strong>Bottleneck identified:</strong> "Human Review Queue" accounts for <strong>78%</strong> of total workflow time (38 min avg, 112 min p95). Consider adding auto-routing for high-confidence predictions (&gt; 95%).
            </div>
          </ChartCard>

          {/* Human wait trend */}
          <ChartCard
            className="col-span-2"
            title="Human Review Wait Time Trend"
            subtitle="Avg & p95 queue wait — last 14 days (minutes)"
          >
            <ResponsiveContainer width="100%" height={280}>
              <LineChart data={humanWaitTrend} margin={{ top: 5, right: 10, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} />
                <XAxis dataKey="day" tick={{ fill: C.text, fontSize: 10 }} tickLine={false} axisLine={false} interval={3} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={(v) => `${v}m`} />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number, n: string) => [`${v} min`, n === "avgWait" ? "Avg wait" : "p95 wait"]} />
                <Legend iconType="square" wrapperStyle={{ fontSize: 11, color: C.text }} />
                <Line type="monotone" dataKey="avgWait" name="Avg" stroke={C.amber} strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="p95Wait" name="p95" stroke={C.red} strokeWidth={2} dot={false} strokeDasharray="4 3" />
              </LineChart>
            </ResponsiveContainer>
            <p className="mt-2 text-xs text-muted-foreground">
              Trend improving: avg wait dropped from 51 min (Feb 18) to 24 min (Mar 2).
            </p>
          </ChartCard>
        </div>

        {/* ── Section 2: Human Review Backlog ── */}
        <div className="mb-8 grid grid-cols-5 gap-4">
          {/* Backlog by type */}
          <ChartCard
            className="col-span-3"
            title="Human Review Backlog by Workflow Type"
            subtitle="Pending items, resolved (this month), and SLA breaches"
          >
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={backlogData} margin={{ top: 0, right: 10, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} vertical={false} />
                <XAxis dataKey="type" tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} />
                <Tooltip contentStyle={tooltipStyle} />
                <Legend iconType="square" wrapperStyle={{ fontSize: 11, color: C.text }} />
                <Bar dataKey="resolved" name="Resolved" fill={C.emerald} stackId="a" radius={[0, 0, 0, 0]} />
                <Bar dataKey="pending" name="Pending" fill={C.amber} stackId="a" />
                <Bar dataKey="slaBreached" name="SLA Breached" fill={C.red} />
              </BarChart>
            </ResponsiveContainer>
          </ChartCard>

          {/* SLA pie */}
          <ChartCard
            className="col-span-2"
            title="Human Review SLA Distribution"
            subtitle="% of reviews resolved within time bands"
          >
            <div className="flex items-center gap-4">
              <ResponsiveContainer width="55%" height={200}>
                <PieChart>
                  <Pie
                    data={slaData}
                    cx="50%"
                    cy="50%"
                    innerRadius={55}
                    outerRadius={80}
                    paddingAngle={3}
                    dataKey="value"
                  >
                    {slaData.map((entry, i) => (
                      <Cell key={i} fill={entry.fill} />
                    ))}
                  </Pie>
                  <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [`${v}%`, "Share"]} />
                </PieChart>
              </ResponsiveContainer>
              <div className="flex-1 space-y-2">
                {slaData.map((d) => (
                  <div key={d.name} className="flex items-center gap-2 text-xs">
                    <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: d.fill }} />
                    <span className="flex-1 text-muted-foreground">{d.name}</span>
                    <span className="font-semibold text-foreground">{d.value}%</span>
                  </div>
                ))}
                <div className="mt-3 border-t border-border pt-2 text-xs">
                  <span className="text-muted-foreground">SLA target</span>
                  <span className="ml-1 font-semibold text-emerald-400">67% within 60 min</span>
                </div>
              </div>
            </div>
          </ChartCard>
        </div>

        {/* ── Section 3: Usage & API calls ── */}
        <SectionTitle>
          <Activity className="h-4 w-4 text-[var(--optum-orange)]" />
          API Usage &amp; Marketplace Adoption
        </SectionTitle>

        <div className="mb-8 grid grid-cols-2 gap-4">
          {/* API calls area chart */}
          <ChartCard
            title="API Calls Over Time"
            subtitle="Requests to models, agents, and tools — last 30 days"
          >
            <ResponsiveContainer width="100%" height={240}>
              <AreaChart data={apiCallsTrend} margin={{ top: 5, right: 10, left: -10, bottom: 0 }}>
                <defs>
                  <linearGradient id="gModels" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C.orange} stopOpacity={0.25} />
                    <stop offset="95%" stopColor={C.orange} stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="gAgents" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C.blueLight} stopOpacity={0.20} />
                    <stop offset="95%" stopColor={C.blueLight} stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="gTools" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C.teal} stopOpacity={0.20} />
                    <stop offset="95%" stopColor={C.teal} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} />
                <XAxis dataKey="date" tick={{ fill: C.text, fontSize: 10 }} tickLine={false} axisLine={false} interval={2} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [v.toLocaleString(), ""]} />
                <Legend iconType="square" wrapperStyle={{ fontSize: 11, color: C.text }} />
                <Area type="monotone" dataKey="models" name="Models" stroke={C.orange} fill="url(#gModels)" strokeWidth={2} />
                <Area type="monotone" dataKey="agents" name="Agents" stroke={C.blueLight} fill="url(#gAgents)" strokeWidth={2} />
                <Area type="monotone" dataKey="tools" name="Tools" stroke={C.teal} fill="url(#gTools)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </ChartCard>

          {/* Marketplace adoption line chart */}
          <ChartCard
            title="Marketplace Asset Adoption"
            subtitle="Cumulative published assets by type since launch"
          >
            <ResponsiveContainer width="100%" height={240}>
              <LineChart data={adoptionData} margin={{ top: 5, right: 10, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} />
                <XAxis dataKey="month" tick={{ fill: C.text, fontSize: 10 }} tickLine={false} axisLine={false} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} />
                <Tooltip contentStyle={tooltipStyle} />
                <Legend iconType="square" wrapperStyle={{ fontSize: 11, color: C.text }} />
                <Line type="monotone" dataKey="models" name="Models" stroke={C.orange} strokeWidth={2} dot={{ r: 3, fill: C.orange }} />
                <Line type="monotone" dataKey="agents" name="Agents" stroke={C.blueLight} strokeWidth={2} dot={{ r: 3, fill: C.blueLight }} />
                <Line type="monotone" dataKey="tools" name="Tools" stroke={C.teal} strokeWidth={2} dot={{ r: 3, fill: C.teal }} />
                <Line type="monotone" dataKey="mcpServers" name="MCP Servers" stroke={C.purple} strokeWidth={2} dot={{ r: 3, fill: C.purple }} />
              </LineChart>
            </ResponsiveContainer>
          </ChartCard>
        </div>

        {/* ── Section 4: Asset performance ── */}
        <SectionTitle>
          <Brain className="h-4 w-4 text-[var(--optum-orange)]" />
          Asset Performance &amp; Reach
        </SectionTitle>

        <div className="mb-8 grid grid-cols-5 gap-4">
          {/* Top assets */}
          <ChartCard
            className="col-span-3"
            title="Top Assets by Team Adoption"
            subtitle="Unique teams actively using each asset this month"
          >
            <ResponsiveContainer width="100%" height={260}>
              <BarChart
                data={topAssetsData}
                layout="vertical"
                margin={{ top: 0, right: 30, left: 160, bottom: 0 }}
              >
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} horizontal={false} />
                <XAxis type="number" tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} />
                <YAxis
                  type="category"
                  dataKey="name"
                  tick={{ fill: C.text, fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  width={155}
                />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [`${v} teams`, "Usage"]} />
                <Bar dataKey="teams" name="Teams" radius={[0, 4, 4, 0]}>
                  {topAssetsData.map((entry, i) => (
                    <Cell key={i} fill={ASSET_TYPE_COLORS[entry.type] ?? C.orange} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>

            {/* Asset type legend */}
            <div className="mt-3 flex flex-wrap gap-3">
              {Object.entries(ASSET_TYPE_COLORS).map(([type, color]) => (
                <span key={type} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: color }} />
                  {type}
                </span>
              ))}
            </div>
          </ChartCard>

          {/* Workflow funnel */}
          <ChartCard
            className="col-span-2"
            title="Workflow Funnel (Mar 2026)"
            subtitle="Task progression from submission to completion"
          >
            <div className="space-y-2.5 pt-2">
              {funnelData.map((d, i) => {
                const pct = Math.round((d.count / funnelData[0].count) * 100)
                return (
                  <div key={d.stage}>
                    <div className="mb-1 flex items-center justify-between text-xs">
                      <span className="text-muted-foreground">{d.stage}</span>
                      <span className="font-semibold text-foreground">{d.count.toLocaleString()}</span>
                    </div>
                    <div className="h-6 overflow-hidden rounded-md bg-secondary/50">
                      <div
                        className="h-full rounded-md transition-all duration-500"
                        style={{ width: `${pct}%`, background: d.fill, opacity: 0.85 }}
                      />
                    </div>
                    <p className="mt-0.5 text-right text-xs text-muted-foreground/60">{pct}%</p>
                  </div>
                )
              })}
            </div>
          </ChartCard>
        </div>

        {/* ── Section 5: Error rate & throughput ── */}
        <SectionTitle>
          <AlertTriangle className="h-4 w-4 text-red-400" />
          Error Rate &amp; Throughput
        </SectionTitle>

        <div className="mb-8 grid grid-cols-2 gap-4">
          <ChartCard
            title="Error Rate Trend"
            subtitle="% of failed API calls — last 14 days"
          >
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={errorTrend} margin={{ top: 5, right: 10, left: -15, bottom: 0 }}>
                <defs>
                  <linearGradient id="gError" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C.red} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={C.red} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} />
                <XAxis dataKey="date" tick={{ fill: C.text, fontSize: 10 }} tickLine={false} axisLine={false} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={(v) => `${v}%`} />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [`${v}%`, "Error rate"]} />
                <Area type="monotone" dataKey="errorRate" name="Error rate" stroke={C.red} fill="url(#gError)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </ChartCard>

          <ChartCard
            title="Daily Request Throughput"
            subtitle="Successful responses per day"
          >
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={errorTrend} margin={{ top: 5, right: 10, left: -10, bottom: 0 }}>
                <defs>
                  <linearGradient id="gThroughput" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C.emerald} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={C.emerald} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke={C.grid} />
                <XAxis dataKey="date" tick={{ fill: C.text, fontSize: 10 }} tickLine={false} axisLine={false} />
                <YAxis tick={{ fill: C.text, fontSize: 11 }} tickLine={false} axisLine={false} />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [v.toLocaleString(), "Requests/day"]} />
                <Area type="monotone" dataKey="throughput" name="Throughput" stroke={C.emerald} fill="url(#gThroughput)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </ChartCard>
        </div>

        {/* ── Footer note ── */}
        <p className="text-center text-xs text-muted-foreground">
          Data refreshed at {new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · All times in UTC-6 · Human-in-loop SLA target: 60 min · p95 latency target: 120 min
        </p>
      </main>
    </div>
  )
}
