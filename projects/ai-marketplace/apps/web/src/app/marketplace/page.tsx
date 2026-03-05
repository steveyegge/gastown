"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bot, Server, Brain, Layers, Globe, BarChart2, ExternalLink, TrendingUp, Package, ShieldCheck, Star } from "lucide-react";
import { AssetCard } from "@/components/marketplace/AssetCard";
import { AssetFilters } from "@/components/marketplace/AssetFilters";
import { AssetSearch } from "@/components/marketplace/AssetSearch";
import { fetchAssets } from "@/lib/api/assets";
import type { Asset, AssetFilter, AssetType } from "@/lib/types";

// ─── Tab definitions ──────────────────────────────────────────────────────────

type TabId = "agents" | "mcp" | "models" | "skills" | "stats";

interface TabDef {
  id: TabId;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  color: string;
  assetTypes: (AssetType | null)[];
  external?: string;
}

const TABS: TabDef[] = [
  { id: "agents",  label: "Agents",  icon: Bot,       color: "text-orange-500",  assetTypes: ["Agent"] },
  { id: "mcp",     label: "MCP",     icon: Server,    color: "text-purple-500",  assetTypes: ["MCP Server"] },
  { id: "models",  label: "Models",  icon: Brain,     color: "text-green-500",   assetTypes: ["Model"] },
  { id: "skills",  label: "Skills",  icon: Layers,    color: "text-yellow-500",  assetTypes: ["Workflow Template", "Evaluator", "Connector"] },
  { id: "stats",   label: "Stats",   icon: BarChart2, color: "text-blue-400",    assetTypes: [] },
];

const MCP_REGISTRY_URL = "https://mcpservers.org";

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function MarketplacePage() {
  const [activeTab, setActiveTab] = useState<TabId>("agents");
  const [filters, setFilters] = useState<AssetFilter>({
    type: "all",
    tags: [],
    complianceTier: "all",
    deploymentMode: "all",
    search: "",
  });

  // Fetch all assets for counts + stats
  const { data: allData } = useQuery({
    queryKey: ["assets-all"],
    queryFn: () => fetchAssets({ type: "all", tags: [], complianceTier: "all", deploymentMode: "all", search: "", pageSize: 500 }),
    staleTime: 60_000,
  });

  // Counts per tab
  const counts = useMemo(() => {
    const items = allData?.items ?? [];
    return {
      agents: items.filter((a) => a.type === "Agent").length,
      mcp:    items.filter((a) => a.type === "MCP Server").length,
      models: items.filter((a) => a.type === "Model").length,
      skills: items.filter((a) => ["Workflow Template", "Evaluator", "Connector"].includes(a.type)).length,
    } as Record<string, number>;
  }, [allData]);

  // Build type filter from active tab
  const activeTabDef = TABS.find((t) => t.id === activeTab)!;
  const typeFilter: AssetFilter = {
    ...filters,
    type: activeTab === "stats" ? "all"
      : activeTabDef.assetTypes.length === 1 ? (activeTabDef.assetTypes[0] as AssetType)
      : "all",
  };

  const { data, isLoading, error } = useQuery({
    queryKey: ["assets", typeFilter, activeTab],
    queryFn: () => fetchAssets(typeFilter),
    staleTime: 30_000,
    enabled: activeTab !== "stats",
  });

  // For multi-type tabs (skills), filter client-side
  const displayItems = useMemo(() => {
    if (!data) return [];
    if (activeTab === "skills") {
      return data.items.filter((a) => ["Workflow Template", "Evaluator", "Connector"].includes(a.type));
    }
    return data.items;
  }, [data, activeTab]);

  return (
    <div className="space-y-0">
      {/* ── Top tab bar ──────────────────────────────────────────────── */}
      <div className="flex items-center gap-1 rounded-xl border bg-gray-900 px-3 py-2 mb-5">
        {TABS.map((tab) => {
          const Icon = tab.icon;
          const count = counts[tab.id];
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all ${
                isActive
                  ? "bg-gray-700 text-white shadow"
                  : "text-gray-400 hover:bg-gray-800 hover:text-gray-200"
              }`}
            >
              <Icon className={`h-4 w-4 ${isActive ? tab.color : ""}`} />
              <span>{tab.label}</span>
              {count != null && count > 0 && (
                <span className={`rounded-full px-1.5 py-0.5 text-xs font-semibold ${isActive ? "bg-gray-600 text-white" : "bg-gray-700 text-gray-300"}`}>
                  {count}
                </span>
              )}
            </button>
          );
        })}

        {/* MCP Registry external link */}
        <a
          href={MCP_REGISTRY_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="ml-1 flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-gray-400 hover:bg-gray-800 hover:text-green-400 transition-all"
        >
          <Globe className="h-4 w-4" />
          <span>MCP Registry</span>
          <ExternalLink className="h-3 w-3" />
        </a>
      </div>

      {/* ── Stats tab ────────────────────────────────────────────────── */}
      {activeTab === "stats" && <StatsPanel items={allData?.items ?? []} loading={!allData} />}

      {/* ── Asset grid tabs ──────────────────────────────────────────── */}
      {activeTab !== "stats" && (
        <div className="flex gap-6">
          {/* Sidebar filters */}
          <div className="w-52 flex-shrink-0">
            <AssetFilters filters={filters} onChange={setFilters} />
          </div>

          {/* Grid */}
          <div className="flex-1 space-y-4">
            <div className="flex items-center justify-between">
              <h1 className="text-xl font-bold text-gray-900">
                {activeTabDef.label}
              </h1>
              <span className="text-sm text-gray-400">{data?.total ?? "–"} assets</span>
            </div>

            <AssetSearch
              value={filters.search}
              onChange={(search) => setFilters((f) => ({ ...f, search }))}
            />

            {isLoading && (
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="h-48 animate-pulse rounded-xl bg-gray-100" />
                ))}
              </div>
            )}

            {error && (
              <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700">
                Failed to load assets. Check your API connection.
              </div>
            )}

            {!isLoading && !error && displayItems.length === 0 && (
              <div className="rounded-xl border bg-white py-16 text-center text-gray-400">
                <Package className="mx-auto h-10 w-10 text-gray-200 mb-3" />
                <p className="text-sm">No {activeTabDef.label.toLowerCase()} found.</p>
              </div>
            )}

            {displayItems.length > 0 && (
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {displayItems.map((asset) => (
                  <AssetCard key={asset.id} asset={asset} />
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Stats panel ─────────────────────────────────────────────────────────────

function StatsPanel({ items, loading }: { items: Asset[]; loading: boolean }) {
  if (loading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="h-28 animate-pulse rounded-xl bg-gray-100" />
        ))}
      </div>
    );
  }

  const total = items.length;
  const byType = [
    { label: "Agents",    count: items.filter((a) => a.type === "Agent").length,             color: "bg-orange-500", text: "text-orange-600" },
    { label: "MCP",       count: items.filter((a) => a.type === "MCP Server").length,        color: "bg-purple-500", text: "text-purple-600" },
    { label: "Models",    count: items.filter((a) => a.type === "Model").length,             color: "bg-green-500",  text: "text-green-600" },
    { label: "Skills",    count: items.filter((a) => ["Workflow Template","Evaluator","Connector"].includes(a.type)).length, color: "bg-yellow-500", text: "text-yellow-600" },
  ];

  const totalDeploys  = items.reduce((s, a) => s + a.deploymentCount, 0);
  const avgRating     = items.length ? (items.reduce((s, a) => s + a.rating, 0) / items.length).toFixed(1) : "—";
  const verified      = items.filter((a) => a.verified).length;
  const topPublishers = Object.entries(
    items.reduce((acc, a) => { acc[a.publisher.name] = (acc[a.publisher.name] ?? 0) + 1; return acc; }, {} as Record<string, number>)
  ).sort((a, b) => b[1] - a[1]).slice(0, 5);

  const complianceDist = ["Standard", "Healthcare", "Financial", "Government"].map((tier) => ({
    label: tier,
    count: items.filter((a) => a.complianceTier === tier).length,
  }));

  const maxComplianceCount = Math.max(...complianceDist.map((c) => c.count), 1);

  return (
    <div className="space-y-6">
      {/* KPI row */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Total Assets" value={total} icon={Package} color="bg-blue-50 text-blue-600" />
        <StatCard label="Total Deployments" value={totalDeploys.toLocaleString()} icon={TrendingUp} color="bg-green-50 text-green-600" />
        <StatCard label="Avg Rating" value={`${avgRating} ★`} icon={Star} color="bg-yellow-50 text-yellow-600" />
        <StatCard label="Verified Assets" value={`${verified} / ${total}`} icon={ShieldCheck} color="bg-purple-50 text-purple-600" />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Assets by type */}
        <div className="rounded-xl border bg-white p-6">
          <h2 className="mb-4 font-semibold text-gray-800">Assets by Type</h2>
          <div className="space-y-3">
            {byType.map((t) => (
              <div key={t.label} className="flex items-center gap-3">
                <span className="w-16 text-sm text-gray-500">{t.label}</span>
                <div className="flex-1 rounded-full bg-gray-100 h-3 overflow-hidden">
                  <div
                    className={`h-3 rounded-full ${t.color}`}
                    style={{ width: total ? `${(t.count / total) * 100}%` : "0%" }}
                  />
                </div>
                <span className={`w-8 text-right text-sm font-semibold ${t.text}`}>{t.count}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Compliance distribution */}
        <div className="rounded-xl border bg-white p-6">
          <h2 className="mb-4 font-semibold text-gray-800">Compliance Distribution</h2>
          <div className="space-y-3">
            {complianceDist.map((c) => (
              <div key={c.label} className="flex items-center gap-3">
                <span className="w-24 text-sm text-gray-500">{c.label}</span>
                <div className="flex-1 rounded-full bg-gray-100 h-3 overflow-hidden">
                  <div
                    className="h-3 rounded-full bg-blue-500"
                    style={{ width: `${(c.count / maxComplianceCount) * 100}%` }}
                  />
                </div>
                <span className="w-8 text-right text-sm font-semibold text-blue-600">{c.count}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Top publishers */}
        <div className="rounded-xl border bg-white p-6">
          <h2 className="mb-4 font-semibold text-gray-800">Top Publishers</h2>
          {topPublishers.length === 0 ? (
            <p className="text-sm text-gray-400">No publishers found.</p>
          ) : (
            <ol className="space-y-2">
              {topPublishers.map(([name, count], i) => (
                <li key={name} className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="flex h-6 w-6 items-center justify-center rounded-full bg-gray-100 text-xs font-bold text-gray-500">
                      {i + 1}
                    </span>
                    <span className="text-sm text-gray-800">{name}</span>
                  </div>
                  <span className="rounded-full bg-blue-50 px-2 py-0.5 text-xs font-semibold text-blue-600">
                    {count} asset{count !== 1 ? "s" : ""}
                  </span>
                </li>
              ))}
            </ol>
          )}
        </div>

        {/* Recently added */}
        <div className="rounded-xl border bg-white p-6">
          <h2 className="mb-4 font-semibold text-gray-800">Recently Added</h2>
          {items.length === 0 ? (
            <p className="text-sm text-gray-400">No assets yet.</p>
          ) : (
            <ul className="space-y-2">
              {[...items]
                .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
                .slice(0, 5)
                .map((asset) => (
                  <li key={asset.id} className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm text-gray-800">{asset.name}</span>
                    <span className="shrink-0 text-xs text-gray-400">
                      {new Date(asset.createdAt).toLocaleDateString()}
                    </span>
                  </li>
                ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

function StatCard({
  label, value, icon: Icon, color,
}: {
  label: string;
  value: string | number;
  icon: React.ComponentType<{ className?: string }>;
  color: string;
}) {
  return (
    <div className="rounded-xl border bg-white p-5">
      <div className={`inline-flex rounded-lg p-2 ${color}`}>
        <Icon className="h-5 w-5" />
      </div>
      <p className="mt-3 text-2xl font-bold text-gray-900">{value}</p>
      <p className="text-sm text-gray-500">{label}</p>
    </div>
  );
}
