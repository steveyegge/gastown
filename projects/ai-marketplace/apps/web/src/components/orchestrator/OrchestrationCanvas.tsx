"use client";

import { useCallback, useRef } from "react";
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  Panel,
  addEdge,
  useNodesState,
  useEdgesState,
  type Connection,
  type Node,
  type Edge,
  BackgroundVariant,
} from "reactflow";
import "reactflow/dist/style.css";

import { AgentNode } from "./nodes/AgentNode";
import { ToolNode } from "./nodes/ToolNode";
import { ModelNode } from "./nodes/ModelNode";
import { HumanNode } from "./nodes/HumanNode";
import { GuardNode } from "./nodes/GuardNode";
import { KnowledgeNode } from "./nodes/KnowledgeNode";
import { EvaluatorNode } from "./nodes/EvaluatorNode";
import { useWorkflowStore } from "@/lib/stores/workflowStore";
import type { WorkflowNodeData } from "@/lib/types";

const nodeTypes = {
  agent: AgentNode,
  tool: ToolNode,
  model: ModelNode,
  human: HumanNode,
  guard: GuardNode,
  knowledge: KnowledgeNode,
  evaluator: EvaluatorNode,
};

const defaultNodes: Node<WorkflowNodeData>[] = [
  {
    id: "trigger-1",
    type: "agent",
    position: { x: 80, y: 180 },
    data: {
      label: "Intake Agent",
      nodeType: "agent",
      assetName: "Claims Intake Agent",
      config: {},
      status: "idle",
    },
  },
  {
    id: "tool-1",
    type: "tool",
    position: { x: 320, y: 100 },
    data: {
      label: "EHR Gateway",
      nodeType: "tool",
      assetName: "EHR Gateway MCP Server",
      config: {},
      status: "idle",
    },
  },
  {
    id: "model-1",
    type: "model",
    position: { x: 320, y: 280 },
    data: {
      label: "GPT-4o",
      nodeType: "model",
      assetName: "Azure OpenAI GPT-4o",
      config: { maxTokens: 4096 },
      status: "idle",
    },
  },
  {
    id: "agent-2",
    type: "agent",
    position: { x: 560, y: 180 },
    data: {
      label: "Denial Intel Agent",
      nodeType: "agent",
      assetName: "Denial Intelligence Agent",
      config: {},
      status: "idle",
    },
  },
  {
    id: "human-1",
    type: "human",
    position: { x: 800, y: 180 },
    data: {
      label: "Review Gate",
      nodeType: "human",
      config: { approvalRequired: true },
      status: "idle",
    },
  },
];

const defaultEdges: Edge[] = [
  { id: "e1-t1", source: "trigger-1", target: "tool-1", label: "fetch records", animated: true },
  { id: "e1-m1", source: "trigger-1", target: "model-1", label: "context" },
  { id: "et1-a2", source: "tool-1", target: "agent-2" },
  { id: "em1-a2", source: "model-1", target: "agent-2" },
  { id: "ea2-h1", source: "agent-2", target: "human-1", label: "recommendation", animated: true },
];

interface Props {
  onNodeSelect: (nodeId: string | null) => void;
}

export default function OrchestrationCanvas({ onNodeSelect }: Props) {
  const [nodes, setNodes, onNodesChange] = useNodesState(defaultNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(defaultEdges);
  const reactFlowWrapper = useRef<HTMLDivElement>(null);
  const {
    isDirty,
    isSaving,
    isRunning,
    saveError,
    saveWorkflow,
    runWorkflow,
  } = useWorkflowStore();

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge({ ...params, animated: true }, eds)),
    [setEdges]
  );

  const onNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => onNodeSelect(node.id),
    [onNodeSelect]
  );

  const onPaneClick = useCallback(() => onNodeSelect(null), [onNodeSelect]);

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const nodeType = event.dataTransfer.getData("application/reactflow/type");
      const assetName = event.dataTransfer.getData("application/reactflow/label");
      if (!nodeType || !reactFlowWrapper.current) return;

      const bounds = reactFlowWrapper.current.getBoundingClientRect();
      const position = {
        x: event.clientX - bounds.left - 80,
        y: event.clientY - bounds.top - 20,
      };

      const newNode: Node<WorkflowNodeData> = {
        id: `${nodeType}-${Date.now()}`,
        type: nodeType,
        position,
        data: {
          label: assetName,
          nodeType: nodeType as WorkflowNodeData["nodeType"],
          assetName,
          config: {},
          status: "idle",
        },
      };

      setNodes((nds) => nds.concat(newNode));
    },
    [setNodes]
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
  }, []);

  return (
    <div ref={reactFlowWrapper} className="h-full w-full">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        onDrop={onDrop}
        onDragOver={onDragOver}
        nodeTypes={nodeTypes}
        fitView
        proOptions={{ hideAttribution: true }}
        className="bg-gray-50"
      >
        {/* ── Toolbar ─────────────────────────────────────────────────── */}
        <Panel position="top-right" className="flex items-center gap-2">
          {saveError && (
            <span className="text-xs text-red-500 bg-red-50 border border-red-200 rounded px-2 py-1">
              {saveError}
            </span>
          )}
          {isDirty && !isSaving && (
            <span className="text-xs text-amber-600">Unsaved changes</span>
          )}
          <button
            onClick={() => saveWorkflow(nodes, edges)}
            disabled={isSaving || isRunning}
            className="flex items-center gap-1.5 rounded-md border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm transition hover:bg-gray-50 disabled:opacity-50"
          >
            {isSaving ? (
              <>
                <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-gray-400 border-t-transparent" />
                Saving…
              </>
            ) : (
              <>
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="h-3.5 w-3.5">
                  <path d="M2.5 1A1.5 1.5 0 0 0 1 2.5v11A1.5 1.5 0 0 0 2.5 15h11a1.5 1.5 0 0 0 1.5-1.5V5.621a1.5 1.5 0 0 0-.44-1.06L11.439.94A1.5 1.5 0 0 0 10.38.5H2.5ZM5 7.5A1.5 1.5 0 0 1 6.5 6h3A1.5 1.5 0 0 1 11 7.5v5.5H5V7.5Z" />
                </svg>
                Save
              </>
            )}
          </button>
          <button
            onClick={() => runWorkflow(nodes, edges)}
            disabled={isSaving || isRunning}
            className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-50"
          >
            {isRunning ? (
              <>
                <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent" />
                Running…
              </>
            ) : (
              <>
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="h-3.5 w-3.5">
                  <path d="M3 3.732a1.5 1.5 0 0 1 2.305-1.265l6.706 4.268a1.5 1.5 0 0 1 0 2.53L5.305 13.533A1.5 1.5 0 0 1 3 12.268V3.732Z" />
                </svg>
                Run
              </>
            )}
          </button>
        </Panel>
        {/* ────────────────────────────────────────────────────────────── */}
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#d1d5db" />
        <Controls className="border bg-white shadow-sm" />
        <MiniMap
          nodeColor={(n) => {
            const colors: Record<string, string> = {
              agent: "#3b82f6",
              tool: "#8b5cf6",
              model: "#10b981",
              human: "#f59e0b",
              guard: "#ef4444",
              knowledge: "#14b8a6",
              evaluator: "#eab308",
            };
            return colors[n.type ?? ""] ?? "#9ca3af";
          }}
          className="border bg-white shadow-sm"
        />
      </ReactFlow>
    </div>
  );
}
