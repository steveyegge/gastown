"use client";

import type { AssetFilter, AssetType, ComplianceTier, DeploymentMode } from "@/lib/types";

const ASSET_TYPES: (AssetType | "all")[] = [
  "all", "Agent", "MCP Server", "Model", "Workflow Template", "Evaluator", "Connector",
];
const COMPLIANCE_TIERS: (ComplianceTier | "all")[] = ["all", "Standard", "Healthcare", "Financial", "Government"];
const DEPLOYMENT_MODES: (DeploymentMode | "all")[] = ["all", "SaaS", "PaaS"];

interface Props {
  filters: AssetFilter;
  onChange: (filters: AssetFilter) => void;
}

export function AssetFilters({ filters, onChange }: Props) {
  const set = <K extends keyof AssetFilter>(key: K, value: AssetFilter[K]) =>
    onChange({ ...filters, [key]: value, page: 1 });

  return (
    <div className="space-y-5 rounded-xl border bg-white p-4">
      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Type</p>
        <div className="space-y-1">
          {ASSET_TYPES.map((t) => (
            <button
              key={t}
              onClick={() => set("type", t)}
              className={`w-full rounded-md px-2.5 py-1.5 text-left text-sm transition ${
                filters.type === t
                  ? "bg-blue-50 font-medium text-blue-700"
                  : "text-gray-600 hover:bg-gray-50"
              }`}
            >
              {t === "all" ? "All types" : t}
            </button>
          ))}
        </div>
      </div>

      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Compliance</p>
        <div className="space-y-1">
          {COMPLIANCE_TIERS.map((c) => (
            <button
              key={c}
              onClick={() => set("complianceTier", c)}
              className={`w-full rounded-md px-2.5 py-1.5 text-left text-sm transition ${
                filters.complianceTier === c
                  ? "bg-blue-50 font-medium text-blue-700"
                  : "text-gray-600 hover:bg-gray-50"
              }`}
            >
              {c === "all" ? "All tiers" : c}
            </button>
          ))}
        </div>
      </div>

      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Deployment</p>
        <div className="space-y-1">
          {DEPLOYMENT_MODES.map((d) => (
            <button
              key={d}
              onClick={() => set("deploymentMode", d)}
              className={`w-full rounded-md px-2.5 py-1.5 text-left text-sm transition ${
                filters.deploymentMode === d
                  ? "bg-blue-50 font-medium text-blue-700"
                  : "text-gray-600 hover:bg-gray-50"
              }`}
            >
              {d === "all" ? "All modes" : d}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
