"use client";

import { useState } from "react";
import { useWorkflowStore } from "@/lib/stores/workflowStore";

interface Props {
  nodeId: string;
  onClose: () => void;
}

export function WorkflowProperties({ nodeId, onClose }: Props) {
  const { executionStatuses } = useWorkflowStore();
  const execStatus = executionStatuses[nodeId];

  const [contextVars, setContextVars] = useState<Array<{ key: string; value: string }>>([
    { key: "", value: "" },
  ]);

  const addVar = () => setContextVars((v) => [...v, { key: "", value: "" }]);
  const removeVar = (i: number) => setContextVars((v) => v.filter((_, idx) => idx !== i));
  const setVar = (i: number, field: "key" | "value", val: string) =>
    setContextVars((v) => v.map((item, idx) => (idx === i ? { ...item, [field]: val } : item)));

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <p className="text-xs font-semibold uppercase tracking-wider text-gray-400">Node Config</p>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-700">✕</button>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-5">
        {/* Node ID */}
        <div>
          <p className="mb-1 text-xs font-semibold text-gray-500 uppercase tracking-wide">Node ID</p>
          <p className="font-mono text-xs text-gray-700 break-all">{nodeId}</p>
        </div>

        {/* Execution status */}
        {execStatus && (
          <div>
            <p className="mb-1 text-xs font-semibold text-gray-500 uppercase tracking-wide">Execution</p>
            <div className="rounded-lg border p-3 text-xs space-y-1">
              <div className="flex justify-between">
                <span className="text-gray-500">Status</span>
                <span className={`font-semibold ${execStatus.status === "completed" ? "text-green-600" : execStatus.status === "error" ? "text-red-600" : "text-blue-600"}`}>
                  {execStatus.status}
                </span>
              </div>
              {execStatus.startedAt && (
                <div className="flex justify-between">
                  <span className="text-gray-500">Started</span>
                  <span>{new Date(execStatus.startedAt).toLocaleTimeString()}</span>
                </div>
              )}
              {execStatus.completedAt && (
                <div className="flex justify-between">
                  <span className="text-gray-500">Completed</span>
                  <span>{new Date(execStatus.completedAt).toLocaleTimeString()}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Context / variable mapping */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide">Context Variables</p>
            <button
              onClick={addVar}
              className="rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-100"
            >
              + Add
            </button>
          </div>
          <p className="mb-2 text-xs text-gray-400">
            Map variables from upstream nodes or set static values passed to this node as context.
          </p>
          <div className="space-y-2">
            {contextVars.map((v, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <input
                  value={v.key}
                  onChange={(e) => setVar(i, "key", e.target.value)}
                  placeholder="key"
                  className="w-1/2 rounded border border-gray-200 px-2 py-1 text-xs font-mono focus:outline-none focus:border-blue-400"
                />
                <span className="text-gray-300">→</span>
                <input
                  value={v.value}
                  onChange={(e) => setVar(i, "value", e.target.value)}
                  placeholder="value or $ref"
                  className="flex-1 rounded border border-gray-200 px-2 py-1 text-xs font-mono focus:outline-none focus:border-blue-400"
                />
                <button
                  onClick={() => removeVar(i)}
                  className="text-gray-300 hover:text-gray-600 text-xs"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-gray-400">
            Use <code className="bg-gray-100 rounded px-1">$nodeId.outputKey</code> to reference upstream node output.
          </p>
        </div>

        {/* Edge type hint */}
        <div>
          <p className="mb-1 text-xs font-semibold text-gray-500 uppercase tracking-wide">Edge Type Guide</p>
          <div className="space-y-1 text-xs text-gray-600">
            <div className="flex gap-2"><span className="text-blue-500">──→</span> Message / data flow</div>
            <div className="flex gap-2"><span className="text-orange-400">--→</span> Control flow (conditional)</div>
            <div className="flex gap-2"><span className="text-green-500">~~→</span> MCP invocation</div>
          </div>
        </div>
      </div>
    </div>
  );
}
