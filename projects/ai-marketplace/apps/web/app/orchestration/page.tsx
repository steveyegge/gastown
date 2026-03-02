"use client"

import { useState, useCallback } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { WorkflowCanvas } from "@/components/orchestration/workflow-canvas"
import { NodePaletteItem } from "@/components/orchestration/workflow-node"
import { PropertiesPanel } from "@/components/orchestration/properties-panel"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { WorkflowNode, WorkflowEdge } from "@/lib/types"
import { sampleWorkflows } from "@/lib/mock-data"
import { 
  Save, 
  Play, 
  RotateCcw, 
  ZoomIn, 
  ZoomOut, 
  Maximize2,
  Plus,
  ChevronRight
} from "lucide-react"
import Link from "next/link"

const nodeTypes: { type: WorkflowNode["type"]; label: string }[] = [
  { type: "trigger", label: "Trigger" },
  { type: "agent", label: "AI Agent" },
  { type: "tool", label: "Tool" },
  { type: "condition", label: "Condition" },
  { type: "output", label: "Output" },
]

export default function OrchestrationPage() {
  const [workflowName, setWorkflowName] = useState("New Workflow")
  const [nodes, setNodes] = useState<WorkflowNode[]>(sampleWorkflows[0]?.nodes || [])
  const [edges, setEdges] = useState<WorkflowEdge[]>(sampleWorkflows[0]?.edges || [])
  const [selectedNode, setSelectedNode] = useState<string | null>(null)

  const handleAddNode = useCallback((type: WorkflowNode["type"], position: { x: number; y: number }) => {
    const newNode: WorkflowNode = {
      id: `node-${Date.now()}`,
      type,
      position,
      data: {
        label: `New ${type.charAt(0).toUpperCase() + type.slice(1)}`,
        description: "Configure this node",
      },
    }
    setNodes((prev) => [...prev, newNode])
  }, [])

  const handleUpdateNodePosition = useCallback((id: string, position: { x: number; y: number }) => {
    setNodes((prev) =>
      prev.map((node) => (node.id === id ? { ...node, position } : node))
    )
  }, [])

  const handleUpdateNodeData = useCallback((id: string, data: Partial<WorkflowNode["data"]>) => {
    setNodes((prev) =>
      prev.map((node) =>
        node.id === id ? { ...node, data: { ...node.data, ...data } } : node
      )
    )
  }, [])

  const handleDeleteNode = useCallback((id: string) => {
    setNodes((prev) => prev.filter((node) => node.id !== id))
    setEdges((prev) => prev.filter((edge) => edge.source !== id && edge.target !== id))
    setSelectedNode(null)
  }, [])

  const handlePaletteDragStart = useCallback((e: React.DragEvent, type: WorkflowNode["type"]) => {
    e.dataTransfer.setData("nodeType", type)
  }, [])

  const handleClearCanvas = useCallback(() => {
    setNodes([])
    setEdges([])
    setSelectedNode(null)
  }, [])

  const selectedNodeData = nodes.find((n) => n.id === selectedNode) || null

  return (
    <div className="flex h-screen bg-background">
      <AppSidebar />

      <div className="flex flex-col flex-1 ml-64 overflow-hidden">
      {/* Toolbar */}
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-4">
          <Input
            value={workflowName}
            onChange={(e) => setWorkflowName(e.target.value)}
            className="h-8 w-48 bg-secondary"
          />
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon" className="h-8 w-8" title="Zoom In">
              <ZoomIn className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" className="h-8 w-8" title="Zoom Out">
              <ZoomOut className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" className="h-8 w-8" title="Fit to Screen">
              <Maximize2 className="h-4 w-4" />
            </Button>
            <Button 
              variant="ghost" 
              size="icon" 
              className="h-8 w-8" 
              title="Clear Canvas"
              onClick={handleClearCanvas}
            >
              <RotateCcw className="h-4 w-4" />
            </Button>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" className="gap-2">
            <Save className="h-4 w-4" />
            Save Draft
          </Button>
          <Button variant="outline" size="sm" className="gap-2">
            <Play className="h-4 w-4" />
            Test Run
          </Button>
          <Link href="/deployments/new">
            <Button size="sm" className="gap-2">
              Deploy
              <ChevronRight className="h-4 w-4" />
            </Button>
          </Link>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Left Sidebar - Node Palette */}
        <aside className="w-64 shrink-0 border-r border-border overflow-auto">
          <div className="p-4">
            <h3 className="mb-4 text-sm font-medium text-foreground">Components</h3>
            <div className="space-y-2">
              {nodeTypes.map(({ type, label }) => (
                <NodePaletteItem
                  key={type}
                  type={type}
                  label={label}
                  onDragStart={handlePaletteDragStart}
                />
              ))}
            </div>

            <div className="mt-6">
              <h4 className="mb-3 text-sm font-medium text-foreground">From Marketplace</h4>
              <Link href="/">
                <Button variant="outline" size="sm" className="w-full gap-2">
                  <Plus className="h-4 w-4" />
                  Add Asset
                </Button>
              </Link>
            </div>
          </div>
        </aside>

        {/* Canvas */}
        <div className="flex-1">
          <WorkflowCanvas
            nodes={nodes}
            edges={edges}
            selectedNode={selectedNode}
            onSelectNode={setSelectedNode}
            onUpdateNode={handleUpdateNodePosition}
            onAddNode={handleAddNode}
          />
        </div>

        {/* Right Sidebar - Properties */}
        <aside className="w-72 shrink-0 border-l border-border">
          <PropertiesPanel
            node={selectedNodeData}
            onClose={() => setSelectedNode(null)}
            onUpdateNode={handleUpdateNodeData}
            onDeleteNode={handleDeleteNode}
          />
        </aside>
      </div>
      </div>
    </div>
  )
}
