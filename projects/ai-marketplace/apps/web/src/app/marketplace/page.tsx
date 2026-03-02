"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AssetCard } from "@/components/marketplace/AssetCard";
import { AssetFilters } from "@/components/marketplace/AssetFilters";
import { AssetSearch } from "@/components/marketplace/AssetSearch";
import { fetchAssets } from "@/lib/api/assets";
import type { AssetFilter } from "@/lib/types";

export default function MarketplacePage() {
  const [filters, setFilters] = useState<AssetFilter>({
    type: "all",
    tags: [],
    complianceTier: "all",
    deploymentMode: "all",
    search: "",
  });

  const { data, isLoading, error } = useQuery({
    queryKey: ["assets", filters],
    queryFn: () => fetchAssets(filters),
    staleTime: 30_000,
  });

  return (
    <div className="flex gap-6">
      {/* Sidebar filters */}
      <div className="w-56 flex-shrink-0">
        <AssetFilters filters={filters} onChange={setFilters} />
      </div>

      {/* Main content */}
      <div className="flex-1 space-y-5">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">AI Asset Catalog</h1>
          <span className="text-sm text-gray-500">
            {data?.total ?? 0} assets
          </span>
        </div>

        <AssetSearch
          value={filters.search}
          onChange={(search) => setFilters((f) => ({ ...f, search }))}
        />

        {isLoading && (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-48 animate-pulse rounded-xl bg-gray-200" />
            ))}
          </div>
        )}

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-700">
            Failed to load assets. Check your API connection.
          </div>
        )}

        {data && (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {data.items.map((asset) => (
              <AssetCard key={asset.id} asset={asset} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
