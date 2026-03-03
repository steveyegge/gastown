"use client"

import { cn } from "@/lib/utils"
import { WorkflowNode } from "@/lib/types"
import { assets } from "@/lib/mock-data"
import { 
  Zap, 
  Bot, 
  Wrench, 
  GitBranch, 
  Send,
  GripVertical
} from "lucide-react"

const nodeIcons: Record<string, React.ElementType> = {
  trigger: Zap,
  agent: Bot,
  tool: Wrench,
  condition: GitBranch,
  output: Send,
}

const nodeColors: Record<string, string> = {
  trigger: "border-yellow-500/50 bg-yellow-500/10",
  agent: "border-accent/50 bg-accent/10",
  tool: "border-green-500/50 bg-green-500/10",
  condition: "border-orange-500/50 bg-orange-500/10",
  output: "border-blue-500/50 bg-blue-500/10",
}

const iconColors: Record<string, string> = {
  trigger: "text-yellow-500",
  agent: "text-accent",
  tool: "text-green-500",
  condition: "text-orange-500",
  output: "text-blue-500",
}

const assetNameById = new Map(assets.map((asset) => [asset.id, asset.name]))

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === "string")
}

interface WorkflowNodeCardProps {
  node: WorkflowNode
  isSelected: boolean
  onSelect: () => void
  onDragStart: (e: React.DragEvent) => void
}

export function WorkflowNodeCard({ node, isSelected, onSelect, onDragStart }: WorkflowNodeCardProps) {
  const Icon = nodeIcons[node.type] || Bot
  const config = (node.data.config ?? {}) as Record<string, unknown>

  const selectedAgentName =
    typeof config.agentId === "string" ? assetNameById.get(config.agentId) : undefined
  const selectedModelName =
    typeof config.modelId === "string" ? assetNameById.get(config.modelId) : undefined
  const selectedToolName =
    typeof config.toolId === "string" ? assetNameById.get(config.toolId) : undefined
  const selectedTools = toStringArray(config.selectedToolIds)
    .map((id) => assetNameById.get(id))
    .filter((name): name is string => Boolean(name))

  const conditionExpression =
    typeof config.conditionExpression === "string" ? config.conditionExpression : undefined
  const trueBranchLabel = typeof config.trueBranchLabel === "string" ? config.trueBranchLabel : "True"
  const falseBranchLabel = typeof config.falseBranchLabel === "string" ? config.falseBranchLabel : "False"

  return (
    <div
      className={cn(
        "absolute cursor-pointer rounded-lg border-2 bg-card p-3 shadow-lg transition-all",
        nodeColors[node.type],
        isSelected && "ring-2 ring-accent ring-offset-2 ring-offset-background"
      )}
      style={{
        left: node.position.x,
        top: node.position.y,
        minWidth: 160,
      }}
      onClick={onSelect}
      draggable
      onDragStart={onDragStart}
    >
      <div className="flex items-center gap-2">
        <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab" />
        <div className={cn("flex h-8 w-8 items-center justify-center rounded-md", nodeColors[node.type])}>
          <Icon className={cn("h-4 w-4", iconColors[node.type])} />
        </div>
        <div className="flex-1 min-w-0">
          <p className="truncate text-sm font-medium text-foreground">{node.data.label}</p>
          {node.data.description && (
            <p className="truncate text-xs text-muted-foreground">{node.data.description}</p>
          )}
        </div>
      </div>

      {node.type === "agent" && (selectedAgentName || selectedModelName || selectedTools.length > 0) && (
        <div className="mt-2 space-y-1">
          {selectedAgentName && (
            <p className="truncate text-[11px] text-muted-foreground">Agent: {selectedAgentName}</p>
          )}
          {selectedModelName && (
            <p className="truncate text-[11px] text-muted-foreground">Model: {selectedModelName}</p>
          )}
          {selectedTools.length > 0 && (
            <p className="truncate text-[11px] text-muted-foreground">
              Tools: {selectedTools.slice(0, 2).join(", ")}
              {selectedTools.length > 2 ? ` +${selectedTools.length - 2}` : ""}
            </p>
          )}
        </div>
      )}

      {node.type === "tool" && selectedToolName && (
        <div className="mt-2">
          <p className="truncate text-[11px] text-muted-foreground">Tool: {selectedToolName}</p>
        </div>
      )}

      {node.type === "condition" && conditionExpression && (
        <div className="mt-2 space-y-1">
          <p className="truncate font-mono text-[10px] text-muted-foreground">{conditionExpression}</p>
          <p className="truncate text-[11px] text-muted-foreground">
            {trueBranchLabel} / {falseBranchLabel}
          </p>
        </div>
      )}
      
      {/* Connection points */}
      <div className="absolute -left-2 top-1/2 h-3 w-3 -translate-y-1/2 rounded-full border-2 border-muted-foreground bg-background" />
      <div className="absolute -right-2 top-1/2 h-3 w-3 -translate-y-1/2 rounded-full border-2 border-muted-foreground bg-background" />
    </div>
  )
}

interface NodePaletteItemProps {
  type: WorkflowNode["type"]
  label: string
  onDragStart: (e: React.DragEvent, type: WorkflowNode["type"]) => void
}

export function NodePaletteItem({ type, label, onDragStart }: NodePaletteItemProps) {
  const Icon = nodeIcons[type] || Bot

  return (
    <div
      className={cn(
        "flex cursor-grab items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-secondary",
        nodeColors[type]
      )}
      draggable
      onDragStart={(e) => onDragStart(e, type)}
    >
      <div className={cn("flex h-8 w-8 items-center justify-center rounded-md", nodeColors[type])}>
        <Icon className={cn("h-4 w-4", iconColors[type])} />
      </div>
      <span className="text-sm font-medium text-foreground">{label}</span>
    </div>
  )
}
