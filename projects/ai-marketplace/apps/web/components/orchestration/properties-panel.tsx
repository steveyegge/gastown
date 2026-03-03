"use client"

import { WorkflowNode } from "@/lib/types"
import { assets } from "@/lib/mock-data"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Checkbox } from "@/components/ui/checkbox"
import { Badge } from "@/components/ui/badge"
import { X, Trash2, Settings2 } from "lucide-react"

interface PropertiesPanelProps {
  node: WorkflowNode | null
  onClose: () => void
  onUpdateNode: (id: string, data: Partial<WorkflowNode["data"]>) => void
  onDeleteNode: (id: string) => void
}

const availableAgents = assets.filter((asset) => asset.type === "agent")
const availableTools = assets.filter((asset) => asset.type === "mcp-tool")
const availableModels = assets.filter((asset) => asset.type === "model")

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === "string")
}

export function PropertiesPanel({ node, onClose, onUpdateNode, onDeleteNode }: PropertiesPanelProps) {
  if (!node) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-6 text-center">
        <Settings2 className="mb-3 h-8 w-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Select a node to view and edit its properties
        </p>
      </div>
    )
  }

  const nodeConfig = (node.data.config ?? {}) as Record<string, unknown>
  const selectedToolIds = toStringArray(nodeConfig.selectedToolIds)
  const selectedModelId = typeof nodeConfig.modelId === "string" ? nodeConfig.modelId : ""
  const selectedAgentId = typeof nodeConfig.agentId === "string" ? nodeConfig.agentId : ""
  const selectedToolId = typeof nodeConfig.toolId === "string" ? nodeConfig.toolId : ""
  const conditionExpression =
    typeof nodeConfig.conditionExpression === "string"
      ? nodeConfig.conditionExpression
      : "input.score > 0.7"
  const trueBranchLabel =
    typeof nodeConfig.trueBranchLabel === "string" ? nodeConfig.trueBranchLabel : "True"
  const falseBranchLabel =
    typeof nodeConfig.falseBranchLabel === "string" ? nodeConfig.falseBranchLabel : "False"

  const updateConfig = (updates: Record<string, unknown>) => {
    onUpdateNode(node.id, {
      config: {
        ...nodeConfig,
        ...updates,
      },
    })
  }

  const toggleToolSelection = (toolId: string, checked: boolean) => {
    const nextToolIds = checked
      ? [...new Set([...selectedToolIds, toolId])]
      : selectedToolIds.filter((id) => id !== toolId)

    updateConfig({ selectedToolIds: nextToolIds })
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border p-4">
        <h3 className="font-medium text-foreground">Node Properties</h3>
        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex-1 space-y-4 overflow-auto p-4">
        <div className="space-y-2">
          <Label htmlFor="label">Label</Label>
          <Input
            id="label"
            value={node.data.label}
            onChange={(e) => onUpdateNode(node.id, { label: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="description">Description</Label>
          <Textarea
            id="description"
            value={node.data.description || ""}
            onChange={(e) => onUpdateNode(node.id, { description: e.target.value })}
            rows={3}
          />
        </div>

        <div className="space-y-2">
          <Label>Type</Label>
          <Input value={node.type} disabled className="capitalize" />
        </div>

        {node.type === "agent" && (
          <>
            <div className="space-y-2">
              <Label>Agent</Label>
              <Select
                value={selectedAgentId || "unselected-agent"}
                onValueChange={(value) =>
                  updateConfig({
                    agentId: value === "unselected-agent" ? "" : value,
                  })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select agent" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="unselected-agent">Not selected</SelectItem>
                  {availableAgents.map((agent) => (
                    <SelectItem key={agent.id} value={agent.id}>
                      {agent.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Model</Label>
              <Select
                value={selectedModelId || "unselected-model"}
                onValueChange={(value) =>
                  updateConfig({
                    modelId: value === "unselected-model" ? "" : value,
                  })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select model" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="unselected-model">Not selected</SelectItem>
                  {availableModels.map((model) => (
                    <SelectItem key={model.id} value={model.id}>
                      {model.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label>Tools</Label>
                <Badge variant="secondary">{selectedToolIds.length} selected</Badge>
              </div>

              <div className="max-h-40 space-y-2 overflow-auto rounded-md border p-2">
                {availableTools.map((tool) => {
                  const isChecked = selectedToolIds.includes(tool.id)

                  return (
                    <label
                      key={tool.id}
                      className="flex cursor-pointer items-start gap-2 rounded-sm p-2 hover:bg-secondary"
                    >
                      <Checkbox
                        checked={isChecked}
                        onCheckedChange={(checked) => toggleToolSelection(tool.id, Boolean(checked))}
                      />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-foreground">{tool.name}</p>
                        <p className="line-clamp-1 text-xs text-muted-foreground">{tool.summary}</p>
                      </div>
                    </label>
                  )
                })}
              </div>
            </div>
          </>
        )}

        {node.type === "tool" && (
          <div className="space-y-2">
            <Label>Tool</Label>
            <Select
              value={selectedToolId || "unselected-tool"}
              onValueChange={(value) =>
                updateConfig({
                  toolId: value === "unselected-tool" ? "" : value,
                })
              }
            >
              <SelectTrigger>
                <SelectValue placeholder="Select tool" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="unselected-tool">Not selected</SelectItem>
                {availableTools.map((tool) => (
                  <SelectItem key={tool.id} value={tool.id}>
                    {tool.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {node.type === "condition" && (
          <>
            <div className="space-y-2">
              <Label>Condition Expression</Label>
              <Textarea
                value={conditionExpression}
                onChange={(e) => updateConfig({ conditionExpression: e.target.value })}
                placeholder="e.g., input.priority === 'high'"
                rows={3}
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                JavaScript-style boolean expression for branching.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-2">
                <Label htmlFor="true-label">True Branch Label</Label>
                <Input
                  id="true-label"
                  value={trueBranchLabel}
                  onChange={(e) => updateConfig({ trueBranchLabel: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="false-label">False Branch Label</Label>
                <Input
                  id="false-label"
                  value={falseBranchLabel}
                  onChange={(e) => updateConfig({ falseBranchLabel: e.target.value })}
                />
              </div>
            </div>
          </>
        )}

        {node.type === "trigger" && (
          <div className="space-y-2">
            <Label>Trigger Type</Label>
            <Select defaultValue="http">
              <SelectTrigger>
                <SelectValue placeholder="Select trigger type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="http">HTTP Request</SelectItem>
                <SelectItem value="schedule">Schedule</SelectItem>
                <SelectItem value="event">Event</SelectItem>
                <SelectItem value="manual">Manual</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}
      </div>

      <div className="border-t border-border p-4">
        <Button
          variant="destructive"
          size="sm"
          className="w-full gap-2"
          onClick={() => onDeleteNode(node.id)}
        >
          <Trash2 className="h-4 w-4" />
          Delete Node
        </Button>
      </div>
    </div>
  )
}
