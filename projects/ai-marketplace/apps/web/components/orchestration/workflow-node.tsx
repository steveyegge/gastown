"use client"

import { cn } from "@/lib/utils"
import { WorkflowNode, WorkflowEdge, WorkflowNodeType } from "@/lib/types"
import { assets } from "@/lib/mock-data"
import {
  Zap,
  Bot,
  Wrench,
  GitBranch,
  Send,
  ChevronsRight,
  Merge,
  RefreshCw,
  UserCheck,
  Shuffle,
  CheckCircle2,
  GripVertical,
  Layers,
} from "lucide-react"

/* ── Icon / colour maps ──────────────────────────────────────────────────── */

export const NODE_META: Record<
  string,
  {
    label: string
    icon: React.ElementType
    borderColor: string
    bgColor: string
    iconColor: string
    headerBg: string
    description: string
  }
> = {
  start: {
    label: "Start",
    icon: Zap,
    borderColor: "border-yellow-500/60",
    bgColor: "bg-yellow-500/5",
    iconColor: "text-yellow-400",
    headerBg: "bg-yellow-500/10",
    description: "Workflow entry point",
  },
  trigger: {
    label: "Start",
    icon: Zap,
    borderColor: "border-yellow-500/60",
    bgColor: "bg-yellow-500/5",
    iconColor: "text-yellow-400",
    headerBg: "bg-yellow-500/10",
    description: "Workflow entry point",
  },
  agent: {
    label: "AI Agent",
    icon: Bot,
    borderColor: "border-violet-500/60",
    bgColor: "bg-violet-500/5",
    iconColor: "text-violet-400",
    headerBg: "bg-violet-500/10",
    description: "LLM-powered reasoning step",
  },
  tool: {
    label: "MCP Tool",
    icon: Wrench,
    borderColor: "border-emerald-500/60",
    bgColor: "bg-emerald-500/5",
    iconColor: "text-emerald-400",
    headerBg: "bg-emerald-500/10",
    description: "External tool or API call",
  },
  condition: {
    label: "If / Else",
    icon: GitBranch,
    borderColor: "border-orange-500/60",
    bgColor: "bg-orange-500/5",
    iconColor: "text-orange-400",
    headerBg: "bg-orange-500/10",
    description: "Conditional branching",
  },
  "fan-out": {
    label: "Fan-Out",
    icon: ChevronsRight,
    borderColor: "border-cyan-500/60",
    bgColor: "bg-cyan-500/5",
    iconColor: "text-cyan-400",
    headerBg: "bg-cyan-500/10",
    description: "Parallel dispatch to branches",
  },
  "fan-in": {
    label: "Fan-In",
    icon: Merge,
    borderColor: "border-teal-500/60",
    bgColor: "bg-teal-500/5",
    iconColor: "text-teal-400",
    headerBg: "bg-teal-500/10",
    description: "Collect & merge parallel results",
  },
  loop: {
    label: "Loop",
    icon: RefreshCw,
    borderColor: "border-purple-500/60",
    bgColor: "bg-purple-500/5",
    iconColor: "text-purple-400",
    headerBg: "bg-purple-500/10",
    description: "Iterate while condition holds",
  },
  approval: {
    label: "Approval Gate",
    icon: UserCheck,
    borderColor: "border-amber-500/60",
    bgColor: "bg-amber-500/5",
    iconColor: "text-amber-400",
    headerBg: "bg-amber-500/10",
    description: "Human-in-the-loop review",
  },
  transform: {
    label: "Transform",
    icon: Shuffle,
    borderColor: "border-slate-400/60",
    bgColor: "bg-slate-400/5",
    iconColor: "text-slate-400",
    headerBg: "bg-slate-400/10",
    description: "Data shaping & mapping",
  },
  end: {
    label: "End",
    icon: CheckCircle2,
    borderColor: "border-blue-500/60",
    bgColor: "bg-blue-500/5",
    iconColor: "text-blue-400",
    headerBg: "bg-blue-500/10",
    description: "Workflow completion",
  },
  output: {
    label: "End",
    icon: Send,
    borderColor: "border-blue-500/60",
    bgColor: "bg-blue-500/5",
    iconColor: "text-blue-400",
    headerBg: "bg-blue-500/10",
    description: "Workflow completion",
  },
}

/* ── Palette sections ────────────────────────────────────────────────────── */

export const PALETTE_SECTIONS: {
  title: string
  nodes: { type: WorkflowNodeType; label: string; description: string }[]
}[] = [
  {
    title: "Flow Control",
    nodes: [
      { type: "start", label: "Start", description: "Workflow entry trigger" },
      { type: "end", label: "End", description: "Workflow completion" },
      { type: "condition", label: "If / Else", description: "Branch on condition" },
      { type: "loop", label: "Loop", description: "While / for iteration" },
    ],
  },
  {
    title: "Orchestration Patterns",
    nodes: [
      { type: "fan-out", label: "Fan-Out", description: "Parallel dispatch to branches" },
      { type: "fan-in", label: "Fan-In", description: "Merge parallel results" },
    ],
  },
  {
    title: "Agents & Tools",
    nodes: [
      { type: "agent", label: "AI Agent", description: "LLM-powered reasoning step" },
      { type: "tool", label: "MCP Tool", description: "External tool or API call" },
      { type: "transform", label: "Transform", description: "Data shaping & mapping" },
    ],
  },
  {
    title: "Human-in-the-Loop",
    nodes: [
      { type: "approval", label: "Approval Gate", description: "Requires human review" },
    ],
  },
]

/* ── Asset lookup helpers ─────────────────────────────────────────────────── */

const assetById = new Map(assets.map((a) => [a.id, a]))

function assetName(id?: string) {
  return id ? (assetById.get(id)?.name ?? id) : undefined
}

/* ── WorkflowNodeCard ────────────────────────────────────────────────────── */

interface WorkflowNodeCardProps {
  node: WorkflowNode
  isSelected: boolean
  isConnectSource: boolean
  onSelect: () => void
  onDragStart: (e: React.DragEvent) => void
  onConnectorClick: (port: "in" | "out") => void
}

export function WorkflowNodeCard({
  node,
  isSelected,
  isConnectSource,
  onSelect,
  onDragStart,
  onConnectorClick,
}: WorkflowNodeCardProps) {
  const meta = NODE_META[node.type] ?? NODE_META.agent
  const Icon = meta.icon
  const config = node.data.config ?? {}

  const details: string[] = []
  if (node.type === "agent") {
    const name = assetName(config.agentId)
    if (name) details.push(`Agent: ${name}`)
    const tools = config.selectedToolIds ?? []
    const toolNames = tools.slice(0, 2).map((id: string) => assetName(id) ?? id)
    if (toolNames.length) details.push(`Tools: ${toolNames.join(", ")}${tools.length > 2 ? ` +${tools.length - 2}` : ""}`)
    if (config.modelId) details.push(`Model: ${config.modelId as string}`)
  }
  if (node.type === "tool") {
    const name = assetName(config.toolId)
    if (name) details.push(`Tool: ${name}`)
  }
  if (node.type === "condition") {
    if (config.conditionExpression) details.push(`if (${config.conditionExpression as string})`)
    const t = (config.trueBranchLabel as string) || "True"
    const f = (config.falseBranchLabel as string) || "False"
    details.push(`${t} / ${f}`)
  }
  if (node.type === "fan-out") {
    const b = (config.branches as number) || 2
    const s = (config.fanOutStrategy as string) || "parallel"
    details.push(`${b} branches · ${s}`)
  }
  if (node.type === "fan-in") {
    const j = (config.joinStrategy as string) || "all"
    details.push(`Join: ${j}`)
  }
  if (node.type === "loop") {
    if (config.loopCondition) details.push(`while (${config.loopCondition as string})`)
    if (config.maxIterations) details.push(`max ${config.maxIterations as number} iters`)
  }
  if (node.type === "approval") {
    if (config.approverRole) details.push(`Approver: ${config.approverRole as string}`)
    if (config.timeoutMinutes) details.push(`Timeout: ${config.timeoutMinutes as number}m`)
  }
  if (node.type === "start" || node.type === "trigger") {
    const t = (config.triggerType as string) || "manual"
    details.push(`Trigger: ${t}`)
  }

  const isStartNode = node.type === "start" || node.type === "trigger"
  const isEndNode = node.type === "end" || node.type === "output"
  const isCondition = node.type === "condition"
  const isFanOut = node.type === "fan-out"
  const isFanIn = node.type === "fan-in"

  return (
    <div
      className={cn(
        "absolute cursor-pointer rounded-xl border-2 bg-card shadow-lg transition-all select-none",
        meta.borderColor,
        meta.bgColor,
        isSelected && "ring-2 ring-accent ring-offset-2 ring-offset-background scale-[1.02]",
        isConnectSource && "ring-2 ring-cyan-400 ring-offset-2 ring-offset-background",
      )}
      style={{ left: node.position.x, top: node.position.y, minWidth: 200 }}
      onClick={(e) => { e.stopPropagation(); onSelect() }}
      draggable
      onDragStart={onDragStart}
    >
      {/* Header band */}
      <div className={cn("flex items-center gap-2 rounded-t-xl px-3 py-2", meta.headerBg)}>
        <GripVertical className="h-3 w-3 text-muted-foreground cursor-grab shrink-0" />
        <div className={cn("flex h-6 w-6 shrink-0 items-center justify-center rounded-md", meta.headerBg)}>
          <Icon className={cn("h-3.5 w-3.5", meta.iconColor)} />
        </div>
        <p className="truncate text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
          {meta.label}
        </p>
      </div>

      {/* Body */}
      <div className="px-3 py-2.5">
        <p className="text-sm font-semibold text-foreground truncate">{node.data.label}</p>
        {node.data.description && (
          <p className="mt-0.5 text-xs text-muted-foreground truncate">{node.data.description}</p>
        )}
        {details.length > 0 && (
          <div className="mt-2 space-y-0.5 border-t border-border pt-2">
            {details.map((d, i) => (
              <p key={i} className="truncate font-mono text-[10px] text-muted-foreground">{d}</p>
            ))}
          </div>
        )}
      </div>

      {/* === Connectors === */}

      {/* Input (left) — hidden on start */}
      {!isStartNode && !isFanIn && (
        <button
          className="absolute -left-2 top-1/2 -translate-y-1/2 h-4 w-4 rounded-full border-2 border-muted-foreground/60 bg-background hover:border-accent hover:bg-accent/20 transition-colors z-10"
          onClick={(e) => { e.stopPropagation(); onConnectorClick("in") }}
          title="Input connector"
        />
      )}

      {/* Fan-in: 3 left connectors */}
      {isFanIn && (
        <>
          {[25, 50, 75].map((pct, i) => (
            <button
              key={i}
              className="absolute -left-2 h-4 w-4 rounded-full border-2 border-teal-500/70 bg-background hover:bg-teal-500/20 transition-colors z-10"
              style={{ top: `${pct}%`, transform: "translateY(-50%)" }}
              onClick={(e) => { e.stopPropagation(); onConnectorClick("in") }}
              title={`Merge input ${i + 1}`}
            />
          ))}
        </>
      )}

      {/* Output (right) — hidden on end */}
      {!isEndNode && !isCondition && !isFanOut && (
        <button
          className="absolute -right-2 top-1/2 -translate-y-1/2 h-4 w-4 rounded-full border-2 border-muted-foreground/60 bg-background hover:border-accent hover:bg-accent/20 transition-colors z-10"
          onClick={(e) => { e.stopPropagation(); onConnectorClick("out") }}
          title="Output connector"
        />
      )}

      {/* Condition: 2 output connectors (true=green top, false=red bottom) */}
      {isCondition && (
        <>
          <button
            className="absolute -right-2 top-[30%] -translate-y-1/2 h-4 w-4 rounded-full border-2 border-green-500 bg-background hover:bg-green-500/20 transition-colors z-10"
            onClick={(e) => { e.stopPropagation(); onConnectorClick("out") }}
            title="True branch"
          />
          <button
            className="absolute -right-2 top-[70%] -translate-y-1/2 h-4 w-4 rounded-full border-2 border-red-500 bg-background hover:bg-red-500/20 transition-colors z-10"
            onClick={(e) => { e.stopPropagation(); onConnectorClick("out") }}
            title="False branch"
          />
          <span className="absolute right-2 top-[22%] -translate-y-1/2 rounded-sm bg-green-500/20 px-1 text-[8px] font-medium text-green-400 pointer-events-none">T</span>
          <span className="absolute right-2 top-[78%] -translate-y-1/2 rounded-sm bg-red-500/20 px-1 text-[8px] font-medium text-red-400 pointer-events-none">F</span>
        </>
      )}

      {/* Fan-out: 3 output connectors */}
      {isFanOut && (
        <>
          {[25, 50, 75].map((pct, i) => (
            <button
              key={i}
              className="absolute -right-2 h-4 w-4 rounded-full border-2 border-cyan-500/70 bg-background hover:bg-cyan-500/20 transition-colors z-10"
              style={{ top: `${pct}%`, transform: "translateY(-50%)" }}
              onClick={(e) => { e.stopPropagation(); onConnectorClick("out") }}
              title={`Branch ${i + 1}`}
            />
          ))}
        </>
      )}
    </div>
  )
}

/* ── NodePaletteItem ─────────────────────────────────────────────────────── */

interface NodePaletteItemProps {
  type: WorkflowNodeType
  label: string
  description?: string
  onDragStart: (e: React.DragEvent, type: WorkflowNodeType) => void
  compact?: boolean
}

export function NodePaletteItem({ type, label, description, onDragStart, compact }: NodePaletteItemProps) {
  const meta = NODE_META[type] ?? NODE_META.agent
  const Icon = meta.icon
  return (
    <div
      className={cn(
        "flex cursor-grab items-center gap-2.5 rounded-lg border transition-all active:scale-[0.98]",
        meta.borderColor,
        "hover:brightness-110",
        compact ? "p-2" : "p-2.5",
      )}
      style={{ background: "var(--card)" }}
      draggable
      onDragStart={(e) => onDragStart(e, type)}
      title={description}
    >
      <div className={cn("flex shrink-0 items-center justify-center rounded-md", meta.headerBg, compact ? "h-6 w-6" : "h-7 w-7")}>
        <Icon className={cn(meta.iconColor, compact ? "h-3.5 w-3.5" : "h-4 w-4")} />
      </div>
      <div className="min-w-0">
        <p className={cn("font-medium text-foreground truncate", compact ? "text-xs" : "text-sm")}>{label}</p>
        {!compact && description && (
          <p className="text-[10px] text-muted-foreground truncate">{description}</p>
        )}
      </div>
    </div>
  )
}

/* ── WorkflowTemplate type (exported for use in page) ───────────────────── */

export interface WorkflowTemplate {
  id: string
  name: string
  description: string
  icon: React.ElementType
  tags: string[]
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
}

/* ── WorkflowTemplateCard ─────────────────────────────────────────────────── */

interface WorkflowTemplateCardProps {
  template: WorkflowTemplate
  onLoad: (template: WorkflowTemplate) => void
}

export function WorkflowTemplateCard({ template, onLoad }: WorkflowTemplateCardProps) {
  const Icon = template.icon
  return (
    <button
      className="group w-full rounded-lg border border-border bg-card p-3 text-left hover:border-accent transition-all"
      onClick={() => onLoad(template)}
    >
      <div className="flex items-start gap-2">
        <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent/10">
          <Icon className="h-4 w-4 text-accent" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground group-hover:text-accent transition-colors">{template.name}</p>
          <p className="mt-0.5 text-[11px] text-muted-foreground line-clamp-2">{template.description}</p>
          <div className="mt-1.5 flex flex-wrap gap-1">
            {template.tags.map((tag) => (
              <span key={tag} className="rounded-sm bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">{tag}</span>
            ))}
          </div>
        </div>
      </div>
    </button>
  )
}

/* ── AssetQuickAdd ────────────────────────────────────────────────────────── */

interface AssetQuickAddProps {
  onDragStart: (e: React.DragEvent, assetId: string, assetType: "agent" | "tool") => void
}

export function AssetQuickAdd({ onDragStart }: AssetQuickAddProps) {
  const agentAssets = assets.filter((a) => a.type === "agent")
  const toolAssets = assets.filter((a) => a.type === "mcp-tool" || a.type === "mcp-server")

  return (
    <div className="space-y-4">
      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Agents</p>
        <div className="space-y-1.5">
          {agentAssets.slice(0, 6).map((asset) => (
            <div
              key={asset.id}
              className="flex cursor-grab items-center gap-2 rounded-md border border-violet-500/30 bg-violet-500/5 p-2 hover:border-violet-500/60 transition-colors"
              draggable
              onDragStart={(e) => onDragStart(e, asset.id, "agent")}
            >
              <Bot className="h-3.5 w-3.5 shrink-0 text-violet-400" />
              <span className="truncate text-xs text-foreground">{asset.name}</span>
            </div>
          ))}
        </div>
      </div>
      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">MCP Tools</p>
        <div className="space-y-1.5">
          {toolAssets.slice(0, 6).map((asset) => (
            <div
              key={asset.id}
              className="flex cursor-grab items-center gap-2 rounded-md border border-emerald-500/30 bg-emerald-500/5 p-2 hover:border-emerald-500/60 transition-colors"
              draggable
              onDragStart={(e) => onDragStart(e, asset.id, "tool")}
            >
              <Wrench className="h-3.5 w-3.5 shrink-0 text-emerald-400" />
              <span className="truncate text-xs text-foreground">{asset.name}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
