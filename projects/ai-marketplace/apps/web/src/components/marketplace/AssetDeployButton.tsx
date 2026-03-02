"use client";

import { useState } from "react";
import { addAssetToWorkspace } from "@/lib/api/assets";
import type { Asset } from "@/lib/types";

interface Props {
  asset: Pick<Asset, "id" | "name" | "deploymentModes">;
}

export function AssetDeployButton({ asset }: Props) {
  const [status, setStatus] = useState<"idle" | "adding" | "done" | "error">("idle");

  const handleAdd = async () => {
    setStatus("adding");
    try {
      // projectId will be selectable in future; use "default" for MVP
      await addAssetToWorkspace(asset.id, "default");
      setStatus("done");
    } catch {
      setStatus("error");
    }
  };

  return (
    <div className="rounded-xl border bg-white p-5 space-y-3">
      <h3 className="font-semibold text-gray-800">Deploy this asset</h3>

      <div className="flex flex-wrap gap-2">
        {asset.deploymentModes.map((mode) => (
          <span
            key={mode}
            className="rounded-full border px-3 py-1 text-xs font-medium text-gray-600"
          >
            {mode}
          </span>
        ))}
      </div>

      {status === "done" ? (
        <div className="rounded-lg bg-green-50 border border-green-200 p-3 text-sm text-green-700">
          ✓ Added to your workspace
        </div>
      ) : status === "error" ? (
        <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-sm text-red-700">
          Failed to add. Try again.
        </div>
      ) : (
        <button
          onClick={handleAdd}
          disabled={status === "adding"}
          className="w-full rounded-lg bg-blue-600 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-60"
        >
          {status === "adding" ? "Adding…" : "Add to workspace"}
        </button>
      )}

      <p className="text-xs text-gray-400">
        Adds this asset to your Foundry project workspace for orchestration and deployment.
      </p>
    </div>
  );
}
