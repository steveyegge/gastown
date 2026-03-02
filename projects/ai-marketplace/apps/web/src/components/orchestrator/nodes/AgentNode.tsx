"use client";

import { memo } from "react";
import { Handle, Position, type NodeProps } from "reactflow";
import type { WorkflowNodeData } from "@/lib/types";

const statusColors = {
  idle: "border-gray-200 bg-white",
  running: "border-blue-400 bg-blue-50 animate-pulse",
  completed: "border-green-400 bg-green-50",
  error: "border-red-400 bg-red-50",
};

export const AgentNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 p-3 shadow-sm transition-all ${statusColors[data.status ?? "idle"]} ${selected ? "ring-2 ring-blue-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-blue-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">🤖</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        {data.assetName && (
          <p className="text-xs text-gray-400 leading-tight truncate max-w-[100px]">
            {data.assetName}
          </p>
        )}
      </div>
    </div>
    {data.status && data.status !== "idle" && (
      <div className={`mt-1 h-1 w-full rounded-full ${data.status === "running" ? "bg-blue-400" : data.status === "completed" ? "bg-green-400" : "bg-red-400"}`} />
    )}
    <Handle type="source" position={Position.Right} className="!bg-blue-400" />
  </div>
));
AgentNode.displayName = "AgentNode";

export const ToolNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 p-3 shadow-sm ${statusColors[data.status ?? "idle"]} ${selected ? "ring-2 ring-purple-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-purple-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">🔧</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        {data.assetName && (
          <p className="text-xs text-gray-400 leading-tight truncate max-w-[100px]">
            {data.assetName}
          </p>
        )}
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-purple-400" />
  </div>
));
ToolNode.displayName = "ToolNode";

export const ModelNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 p-3 shadow-sm ${statusColors[data.status ?? "idle"]} ${selected ? "ring-2 ring-green-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-green-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">🧠</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        {data.assetName && (
          <p className="text-xs text-gray-400 leading-tight truncate max-w-[100px]">
            {data.assetName}
          </p>
        )}
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-green-400" />
  </div>
));
ModelNode.displayName = "ModelNode";

export const HumanNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 border-amber-300 p-3 shadow-sm bg-amber-50 ${selected ? "ring-2 ring-amber-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-amber-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">👤</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        <p className="text-xs text-amber-600">Approval required</p>
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-amber-400" />
  </div>
));
HumanNode.displayName = "HumanNode";

export const GuardNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 border-red-300 p-3 shadow-sm bg-red-50 ${selected ? "ring-2 ring-red-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-red-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">🛡️</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        <p className="text-xs text-red-600">Policy guard</p>
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-red-400" />
  </div>
));
GuardNode.displayName = "GuardNode";

export const KnowledgeNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 border-teal-300 p-3 shadow-sm bg-teal-50 ${selected ? "ring-2 ring-teal-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-teal-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">📚</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        <p className="text-xs text-teal-600">Retrieval / RAG</p>
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-teal-400" />
  </div>
));
KnowledgeNode.displayName = "KnowledgeNode";

export const EvaluatorNode = memo(({ data, selected }: NodeProps<WorkflowNodeData>) => (
  <div
    className={`min-w-[140px] rounded-xl border-2 border-yellow-300 p-3 shadow-sm bg-yellow-50 ${selected ? "ring-2 ring-yellow-400 ring-offset-2" : ""}`}
  >
    <Handle type="target" position={Position.Left} className="!bg-yellow-400" />
    <div className="flex items-center gap-2">
      <span className="text-lg">📊</span>
      <div>
        <p className="text-xs font-semibold text-gray-800 leading-tight">{data.label}</p>
        <p className="text-xs text-yellow-600">Evaluator</p>
      </div>
    </div>
    <Handle type="source" position={Position.Right} className="!bg-yellow-400" />
  </div>
));
EvaluatorNode.displayName = "EvaluatorNode";
