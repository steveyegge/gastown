"use client";

import { Play, Save, Share2, Settings, PanelLeftOpen, PanelLeftClose, Trash2 } from "lucide-react";
import { useWorkflowStore } from "@/lib/stores/workflowStore";
import { toast } from "sonner";

interface Props {
  isPaletteOpen: boolean;
  onTogglePalette: () => void;
}

export function WorkflowToolbar({ isPaletteOpen, onTogglePalette }: Props) {
  const { workflowName, setWorkflowName, isDirty, isRunning, setIsRunning, resetExecution } =
    useWorkflowStore();

  const handleRun = () => {
    setIsRunning(true);
    toast.info("Workflow execution started", { description: "Agents are being dispatched…" });
    // TODO: call POST /api/workflows/{id}/run
    setTimeout(() => {
      setIsRunning(false);
      toast.success("Workflow completed", { description: "All agents finished successfully." });
    }, 4000);
  };

  const handleSave = () => {
    toast.success("Workflow saved");
    // TODO: call PUT /api/workflows/{id}
  };

  return (
    <div className="flex h-12 items-center gap-2 border-b bg-white px-3">
      <button
        onClick={onTogglePalette}
        className="rounded-md p-1.5 text-gray-500 hover:bg-gray-100 transition-colors"
        title="Toggle palette"
      >
        {isPaletteOpen ? (
          <PanelLeftClose className="h-4 w-4" />
        ) : (
          <PanelLeftOpen className="h-4 w-4" />
        )}
      </button>

      <div className="h-5 w-px bg-gray-200" />

      <input
        type="text"
        value={workflowName}
        onChange={(e) => setWorkflowName(e.target.value)}
        className="w-48 rounded-md border-transparent bg-transparent px-2 py-1 text-sm font-medium text-gray-800 hover:border-gray-200 focus:border-gray-300 focus:outline-none focus:ring-1 focus:ring-gray-200"
      />

      {isDirty && <span className="text-xs text-amber-500">● unsaved</span>}

      <div className="flex-1" />

      <button
        onClick={handleSave}
        className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 transition-colors"
      >
        <Save className="h-3.5 w-3.5" />
        Save
      </button>

      <button
        className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 transition-colors"
        title="Share workflow"
      >
        <Share2 className="h-3.5 w-3.5" />
        Share
      </button>

      <button
        onClick={handleRun}
        disabled={isRunning}
        className="inline-flex items-center gap-1.5 rounded-lg bg-azure-600 px-4 py-1.5 text-xs font-semibold text-white hover:bg-azure-700 disabled:opacity-50 transition-colors"
      >
        <Play className="h-3.5 w-3.5" />
        {isRunning ? "Running…" : "Run"}
      </button>
    </div>
  );
}
