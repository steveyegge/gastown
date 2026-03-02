"use client";

import dynamic from "next/dynamic";
import { useState } from "react";
import { NodePalette } from "@/components/orchestrator/NodePalette";
import { WorkflowToolbar } from "@/components/orchestrator/WorkflowToolbar";
import { WorkflowProperties } from "@/components/orchestrator/WorkflowProperties";

interface CanvasProps {
  onNodeSelect: (id: string | null) => void;
}

// Lazy-load the heavy React Flow canvas
const OrchestrationCanvas = dynamic<CanvasProps>(
  () => import("@/components/orchestrator/OrchestrationCanvas"),
  { ssr: false, loading: () => <CanvasPlaceholder /> }
);

export default function OrchestratorPage() {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [isPaletteOpen, setIsPaletteOpen] = useState(true);

  return (
    <div className="flex h-[calc(100vh-7rem)] rounded-xl border bg-white overflow-hidden">
      {/* Left: node palette */}
      {isPaletteOpen && (
        <div className="w-60 flex-shrink-0 border-r">
          <NodePalette />
        </div>
      )}

      {/* Center: canvas */}
      <div className="flex flex-1 flex-col">
        <WorkflowToolbar
          isPaletteOpen={isPaletteOpen}
          onTogglePalette={() => setIsPaletteOpen((v) => !v)}
        />
        <div className="flex-1">
          <OrchestrationCanvas onNodeSelect={setSelectedNodeId} />
        </div>
      </div>

      {/* Right: properties panel */}
      {selectedNodeId && (
        <div className="w-72 flex-shrink-0 border-l">
          <WorkflowProperties nodeId={selectedNodeId} onClose={() => setSelectedNodeId(null)} />
        </div>
      )}
    </div>
  );
}

function CanvasPlaceholder() {
  return (
    <div className="flex flex-1 items-center justify-center text-gray-400">
      <div className="text-center">
        <div className="mx-auto h-12 w-12 animate-pulse rounded-full bg-gray-200" />
        <p className="mt-3 text-sm">Loading canvas…</p>
      </div>
    </div>
  );
}
