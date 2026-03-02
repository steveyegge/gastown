"use client";

import Link from "next/link";
import type { Asset } from "@/lib/types";

const typeColors: Record<string, string> = {
  Agent: "bg-blue-50 text-blue-700 border-blue-100",
  "MCP Server": "bg-purple-50 text-purple-700 border-purple-100",
  Model: "bg-green-50 text-green-700 border-green-100",
  "Workflow Template": "bg-orange-50 text-orange-700 border-orange-100",
  Evaluator: "bg-yellow-50 text-yellow-700 border-yellow-100",
  Connector: "bg-gray-50 text-gray-700 border-gray-200",
};

interface Props {
  asset: Pick<
    Asset,
    | "id"
    | "name"
    | "type"
    | "description"
    | "publisher"
    | "rating"
    | "reviewCount"
    | "deploymentCount"
    | "tags"
    | "verified"
    | "complianceTier"
    | "deploymentModes"
  >;
}

export function AssetCard({ asset }: Props) {
  const typeColor = typeColors[asset.type] ?? "bg-gray-50 text-gray-700";

  return (
    <Link href={`/marketplace/${asset.id}`}>
      <article className="group flex h-full flex-col rounded-xl border bg-white p-5 shadow-sm transition-all hover:border-blue-300 hover:shadow-md">
        {/* Header */}
        <div className="flex items-start justify-between gap-2">
          <span
            className={`inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${typeColor}`}
          >
            {asset.type}
          </span>
          <div className="flex gap-1.5">
            {asset.verified && (
              <span className="text-xs text-green-600">✓</span>
            )}
            {asset.deploymentModes.map((mode) => (
              <span
                key={mode}
                className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500"
              >
                {mode}
              </span>
            ))}
          </div>
        </div>

        {/* Body */}
        <h3 className="mt-3 font-semibold text-gray-900 group-hover:text-blue-600 line-clamp-1">
          {asset.name}
        </h3>
        <p className="mt-1 flex-1 text-sm text-gray-500 line-clamp-2">{asset.description}</p>

        {/* Tags */}
        <div className="mt-3 flex flex-wrap gap-1">
          {asset.tags.slice(0, 3).map((tag) => (
            <span key={tag} className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
              {tag}
            </span>
          ))}
        </div>

        {/* Footer */}
        <div className="mt-4 flex items-center justify-between border-t pt-3 text-xs text-gray-400">
          <span className="truncate">{asset.publisher.name}</span>
          <div className="flex shrink-0 items-center gap-2">
            <span>⭐ {asset.rating.toFixed(1)}</span>
            <span>{asset.deploymentCount.toLocaleString()}</span>
          </div>
        </div>
      </article>
    </Link>
  );
}
