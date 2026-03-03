"use client"

import { useCallback, useRef, useState } from "react"
import { WorkflowNode, WorkflowEdge } from "@/lib/types"
import { WorkflowNodeCard, NODE_META } from "./workflow-node"
import { cn } from "@/lib/utils"

interface WorkflowCanvasProps {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
  selectedNode: string | null
  connectSourceId: string | null
  onSelectNode: (id: string | null) => void
  onUpdateNode: (id: string, position: { x: number; y: number }) => void
  onAddNode: (type: WorkflowNode["type"], position: { x: number; y: number }) => void
  onAddAssetNode: (assetId: string, assetType: "agent" | "tool", position: { x: number; y: number }) => void
  onConnectorClick: (nodeId: string, port: "in" | "out") => void
  onDeleteEdge: (edgeId: string) => void
}

const EDGE_COLORS: Record<string, string> = {
  default: "stroke-muted-foreground",
  true: "stroke-green-500",
  false: "stroke-red-500",
  loop: "stroke-purple-500",
  fan: "stroke-cyan-400",
}

const MARKER_FILL: Record<string, string> = {
  default: "fill-muted-foreground",
  true: "fill-green-500",
  false: "fill-red-500",
  loop: "fill-purple-500",
  fan: "fill-cyan-400",
}

const NODE_WIDTH = 200
const NODE_HEIGHT_APPROX = 90

export function WorkflowCanvas({
  nodes,
  edges,
  selectedNode,
  connectSourceId,
  onSelectNode,
  onUpdateNode,
  onAddNode,
  onAddAssetNode,
  onConnectorClick,
  onDeleteEdge,
}: WorkflowCanvasProps) {
  const canvasRef = useRef<HTMLDivElement>(null)
  const [draggedNodeId, setDraggedNodeId] = useState<string | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [hoveredEdge, setHoveredEdge] = useState<string | null>(null)

  const getCanvasPos = useCallback((e: React.DragEvent | React.MouseEvent) => {
    const rect = canvasRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return { x: e.clientX - rect.left, y: e.clientY - rect.top }
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
  }, [])

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      const pos = getCanvasPos(e)

      const nodeType = e.dataTransfer.getData("nodeType") as WorkflowNode["type"]
      const assetId = e.dataTransfer.getData("assetId")
      const assetType = e.dataTransfer.getData("assetType") as "agent" | "tool"

      if (nodeType) {
        onAddNode(nodeType, { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT_APPROX / 2 })
      } else if (assetId && assetType) {
        onAddAssetNode(assetId, assetType, { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT_APPROX / 2 })
      } else if (draggedNodeId) {
        onUpdateNode(draggedNodeId, {
          x: pos.x - dragOffset.x,
          y: pos.y - dragOffset.y,
        })
        setDraggedNodeId(null)
      }
    },
    [draggedNodeId, dragOffset, getCanvasPos, onAddNode, onAddAssetNode, onUpdateNode],
  )

  const handleNodeDragStart = useCallback(
    (nodeId: string) => (e: React.DragEvent) => {
      const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
      const canvasRect = canvasRef.current?.getBoundingClientRect()
      if (!canvasRect) return
      setDragOffset({
        x: e.clientX - rect.left,
        y: e.clientY - rect.top,
      })
      setDraggedNodeId(nodeId)
      e.dataTransfer.setData("text/plain", nodeId)
    },
    [],
  )

  /* ── SVG edge path helpers ─────────────────────────────────────────────── */

  const getConnectorPos = (
    node: WorkflowNode,
    side: "out" | "in",
    edgeType?: string,
    edgeIndex?: number,
    totalEdgesOnPort?: number,
  ) => {
    const x = side === "out" ? node.position.x + NODE_WIDTH : node.position.x
    let y = node.position.y + NODE_HEIGHT_APPROX / 2

    if (node.type === "condition" && side === "out") {
      // true branch from ~30% height, false from ~70%
      if (edgeType === "true") y = node.position.y + NODE_HEIGHT_APPROX * 0.3
      else y = node.position.y + NODE_HEIGHT_APPROX * 0.7
    } else if ((node.type === "fan-out" && side === "out") || (node.type === "fan-in" && side === "in")) {
      const idx = edgeIndex ?? 0
      const total = Math.max(totalEdgesOnPort ?? 1, 1)
      y = node.position.y + (NODE_HEIGHT_APPROX * (idx + 1)) / (total + 1)
    }

    return { x, y }
  }

  // Count how many edges share each port, for fan-out/fan-in positioning
  const outEdgeCount: Record<string, number> = {}
  const inEdgeCount: Record<string, number> = {}
  edges.forEach((e) => {
    outEdgeCount[e.source] = (outEdgeCount[e.source] ?? 0) + 1
    inEdgeCount[e.target] = (inEdgeCount[e.target] ?? 0) + 1
  })
  const outEdgeIdx: Record<string, number> = {}
  const inEdgeIdx: Record<string, number> = {}

  const getEdgePath = (edge: WorkflowEdge) => {
    const sourceNode = nodes.find((n) => n.id === edge.source)
    const targetNode = nodes.find((n) => n.id === edge.target)
    if (!sourceNode || !targetNode) return null

    const oIdx = outEdgeIdx[edge.source] ?? 0
    outEdgeIdx[edge.source] = oIdx + 1
    const iIdx = inEdgeIdx[edge.target] ?? 0
    inEdgeIdx[edge.target] = iIdx + 1

    const src = getConnectorPos(sourceNode, "out", edge.edgeType, oIdx, outEdgeCount[edge.source])
    const tgt = getConnectorPos(targetNode, "in", edge.edgeType, iIdx, inEdgeCount[edge.target])

    const dx = Math.abs(tgt.x - src.x)
    const cpOffset = Math.max(60, dx * 0.5)

    const path = `M ${src.x} ${src.y} C ${src.x + cpOffset} ${src.y}, ${tgt.x - cpOffset} ${tgt.y}, ${tgt.x} ${tgt.y}`
    const midX = (src.x + tgt.x) / 2
    const midY = (src.y + tgt.y) / 2

    return { path, midX, midY, src, tgt }
  }

  const edgeType = (edge: WorkflowEdge) => edge.edgeType ?? "default"

  return (
    <div
      ref={canvasRef}
      className={cn(
        "relative h-full w-full overflow-auto bg-background",
        "[background-image:radial-gradient(circle,var(--border)_1px,transparent_1px)]",
        "[background-size:24px_24px]",
        connectSourceId && "cursor-crosshair",
      )}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      onClick={() => onSelectNode(null)}
    >
      {/* Marker defs for each edge colour */}
      <svg className="pointer-events-none absolute inset-0 h-full w-full" style={{ minWidth: 2000, minHeight: 1200 }}>
        <defs>
          {Object.entries(MARKER_FILL).map(([key, fillClass]) => (
            <marker
              key={key}
              id={`arrow-${key}`}
              markerWidth="10"
              markerHeight="7"
              refX="9"
              refY="3.5"
              orient="auto"
            >
              <polygon points="0 0, 10 3.5, 0 7" className={fillClass} />
            </marker>
          ))}
        </defs>

        {/* Edges */}
        {edges.map((edge) => {
          const info = getEdgePath(edge)
          if (!info) return null
          const et = edgeType(edge)
          const colorClass = EDGE_COLORS[et] ?? EDGE_COLORS.default
          const isHovered = hoveredEdge === edge.id

          return (
            <g key={edge.id}>
              {/* Invisible wide hit zone */}
              <path
                d={info.path}
                fill="none"
                strokeWidth="12"
                stroke="transparent"
                className="pointer-events-auto cursor-pointer"
                onMouseEnter={() => setHoveredEdge(edge.id)}
                onMouseLeave={() => setHoveredEdge(null)}
                onClick={(e) => { e.stopPropagation(); onDeleteEdge(edge.id) }}
              />
              {/* Visible edge */}
              <path
                d={info.path}
                fill="none"
                className={cn(colorClass, "pointer-events-none transition-all")}
                strokeWidth={isHovered ? 3 : 2}
                strokeDasharray={isHovered ? "6 3" : undefined}
                markerEnd={`url(#arrow-${et})`}
              />
              {/* Edge label */}
              {edge.label && (
                <foreignObject
                  x={info.midX - 24}
                  y={info.midY - 10}
                  width={48}
                  height={20}
                  className="pointer-events-none"
                >
                  <div className="flex items-center justify-center rounded-full bg-card border border-border px-1.5 py-0.5 text-[10px] font-medium text-foreground shadow-sm">
                    {edge.label}
                  </div>
                </foreignObject>
              )}
              {/* Delete hint on hover */}
              {isHovered && (
                <foreignObject
                  x={info.midX - 28}
                  y={info.midY + 14}
                  width={56}
                  height={18}
                  className="pointer-events-none"
                >
                  <div className="flex items-center justify-center rounded bg-destructive/80 px-1 text-[9px] text-destructive-foreground">
                    click to delete
                  </div>
                </foreignObject>
              )}
            </g>
          )
        })}
      </svg>

      {/* Nodes */}
      {nodes.map((node) => (
        <WorkflowNodeCard
          key={node.id}
          node={node}
          isSelected={selectedNode === node.id}
          isConnectSource={connectSourceId === node.id}
          onSelect={() => onSelectNode(node.id)}
          onDragStart={handleNodeDragStart(node.id)}
          onConnectorClick={(port) => onConnectorClick(node.id, port)}
        />
      ))}

      {/* Empty state */}
      {nodes.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="text-center space-y-2">
            <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl border-2 border-dashed border-border">
              <svg className="h-8 w-8 text-muted-foreground" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
              </svg>
            </div>
            <p className="text-base font-medium text-muted-foreground">Drag components here to start</p>
            <p className="text-sm text-muted-foreground">
              Use a template or drag nodes from the left panel
            </p>
          </div>
        </div>
      )}

      {/* Connect mode overlay hint */}
      {connectSourceId && (
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-lg bg-cyan-500 px-4 py-2 text-sm font-medium text-white shadow-lg pointer-events-none">
          Click the input connector of a target node to connect, or press Esc to cancel
        </div>
      )}
    </div>
  )
}
