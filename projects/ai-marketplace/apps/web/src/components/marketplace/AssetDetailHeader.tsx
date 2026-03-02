import type { Asset } from "@/lib/types";

const typeColors: Record<string, string> = {
  Agent: "bg-blue-100 text-blue-800",
  "MCP Server": "bg-purple-100 text-purple-800",
  Model: "bg-green-100 text-green-800",
  "Workflow Template": "bg-orange-100 text-orange-800",
  Evaluator: "bg-yellow-100 text-yellow-800",
  Connector: "bg-gray-100 text-gray-800",
};

const tierColors: Record<string, string> = {
  Healthcare: "bg-red-50 text-red-700",
  Financial: "bg-amber-50 text-amber-700",
  Government: "bg-indigo-50 text-indigo-700",
  Standard: "bg-gray-50 text-gray-600",
};

interface Props {
  asset: Asset;
}

export function AssetDetailHeader({ asset }: Props) {
  return (
    <div className="rounded-xl border bg-white p-6 shadow-sm">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className={`rounded-md px-2.5 py-1 text-xs font-semibold ${typeColors[asset.type] ?? "bg-gray-100 text-gray-700"}`}>
              {asset.type}
            </span>
            <span className={`rounded-md px-2.5 py-1 text-xs font-medium ${tierColors[asset.complianceTier] ?? "bg-gray-50 text-gray-600"}`}>
              {asset.complianceTier}
            </span>
            {asset.verified && (
              <span className="rounded-md bg-green-50 px-2.5 py-1 text-xs font-medium text-green-700">
                ✓ Verified
              </span>
            )}
          </div>
          <h1 className="text-2xl font-bold text-gray-900">{asset.name}</h1>
          <p className="mt-1 text-gray-500">{asset.description}</p>

          <div className="mt-3 flex flex-wrap gap-1.5">
            {asset.tags.map((tag) => (
              <span key={tag} className="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600">
                {tag}
              </span>
            ))}
          </div>
        </div>

        <div className="flex shrink-0 flex-col items-end gap-2 text-sm">
          <div className="flex items-center gap-1">
            <span className="text-yellow-400">★</span>
            <span className="font-semibold">{asset.rating.toFixed(1)}</span>
            <span className="text-gray-400">({asset.reviewCount})</span>
          </div>
          <div className="text-gray-500">{asset.deploymentCount.toLocaleString()} deployments</div>
        </div>
      </div>

      {asset.riskNotes && (
        <div className="mt-4 flex gap-2 rounded-lg bg-amber-50 border border-amber-200 p-3 text-sm text-amber-800">
          <span>⚠️</span>
          <p>{asset.riskNotes}</p>
        </div>
      )}
    </div>
  );
}
