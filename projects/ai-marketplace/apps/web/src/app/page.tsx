import Link from "next/link";
import { ArrowRight, Bot, Cpu, FileCode2, Layers, Search, Zap } from "lucide-react";

const stats = [
  { label: "AI Agents", value: "240+", icon: Bot, color: "text-blue-600 bg-blue-50" },
  { label: "MCP Servers", value: "88", icon: Zap, color: "text-purple-600 bg-purple-50" },
  { label: "Models", value: "35+", icon: Cpu, color: "text-green-600 bg-green-50" },
  { label: "Workflow Templates", value: "120", icon: Layers, color: "text-orange-600 bg-orange-50" },
];

const featuredAssets = [
  {
    id: "agent-denial-intel",
    name: "Denial Intelligence Agent",
    type: "Agent",
    description: "Automates claims denial analysis and appeal recommendation using Azure OpenAI.",
    publisher: "Optum AI",
    verified: true,
    rating: 4.8,
    deployments: 340,
    tags: ["healthcare", "claims", "azure-openai"],
  },
  {
    id: "mcp-ehr-gateway",
    name: "EHR Gateway MCP Server",
    type: "MCP Server",
    description: "FHIR-compliant MCP server bridging AI agents with Electronic Health Record systems.",
    publisher: "HealthBridge",
    verified: true,
    rating: 4.6,
    deployments: 210,
    tags: ["healthcare", "FHIR", "EHR"],
  },
  {
    id: "template-rag-pipeline",
    name: "RAG Pipeline Template",
    type: "Workflow Template",
    description: "Enterprise RAG workflow with evaluation harness, grounding, and citation tracking.",
    publisher: "Azure AI",
    verified: true,
    rating: 4.9,
    deployments: 612,
    tags: ["RAG", "azure-ai-search", "evaluation"],
  },
];

export default function HomePage() {
  return (
    <div className="space-y-10">
      {/* Hero */}
      <div className="rounded-2xl bg-gradient-to-br from-azure-600 to-primary-700 p-10 text-white">
        <div className="max-w-3xl">
          <h1 className="text-4xl font-bold tracking-tight">AI Asset Marketplace</h1>
          <p className="mt-4 text-lg text-blue-100">
            Discover, evaluate, and deploy enterprise-grade AI agents, MCP servers, models, and
            workflow templates — with built-in governance and Azure AI Foundry integration.
          </p>
          <div className="mt-6 flex items-center gap-4">
            <Link
              href="/marketplace"
              className="inline-flex items-center gap-2 rounded-lg bg-white px-5 py-2.5 text-sm font-semibold text-azure-600 hover:bg-blue-50 transition-colors"
            >
              Browse Catalog <ArrowRight className="h-4 w-4" />
            </Link>
            <Link
              href="/orchestrator"
              className="inline-flex items-center gap-2 rounded-lg border border-white/30 bg-white/10 px-5 py-2.5 text-sm font-semibold text-white hover:bg-white/20 transition-colors"
            >
              Open Orchestrator
            </Link>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className="rounded-xl border bg-white p-5">
            <div className={`inline-flex rounded-lg p-2 ${s.color}`}>
              <s.icon className="h-5 w-5" />
            </div>
            <p className="mt-3 text-2xl font-bold">{s.value}</p>
            <p className="text-sm text-gray-500">{s.label}</p>
          </div>
        ))}
      </div>

      {/* Quick search */}
      <div>
        <div className="relative">
          <Search className="absolute left-4 top-3.5 h-5 w-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search agents, models, MCP servers, templates…"
            className="w-full rounded-xl border bg-white py-3 pl-11 pr-4 text-sm shadow-sm focus:border-azure-500 focus:outline-none focus:ring-1 focus:ring-azure-500"
          />
        </div>
      </div>

      {/* Featured assets */}
      <div>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-xl font-semibold">Featured Assets</h2>
          <Link href="/marketplace" className="text-sm text-azure-600 hover:underline">
            View all
          </Link>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {featuredAssets.map((asset) => (
            <AssetCard key={asset.id} asset={asset} />
          ))}
        </div>
      </div>
    </div>
  );
}

function AssetCard({ asset }: { asset: (typeof featuredAssets)[0] }) {
  const typeColors: Record<string, string> = {
    Agent: "bg-blue-50 text-blue-700",
    "MCP Server": "bg-purple-50 text-purple-700",
    "Workflow Template": "bg-orange-50 text-orange-700",
    Model: "bg-green-50 text-green-700",
  };

  return (
    <Link href={`/marketplace/${asset.id}`}>
      <div className="group cursor-pointer rounded-xl border bg-white p-5 shadow-sm transition-all hover:border-azure-300 hover:shadow-md">
        <div className="flex items-start justify-between">
          <span
            className={`inline-flex rounded-md px-2 py-0.5 text-xs font-medium ${typeColors[asset.type] ?? "bg-gray-100 text-gray-700"}`}
          >
            {asset.type}
          </span>
          {asset.verified && (
            <span className="text-xs text-green-600 font-medium">✓ Verified</span>
          )}
        </div>
        <h3 className="mt-3 font-semibold text-gray-900 group-hover:text-azure-600">
          {asset.name}
        </h3>
        <p className="mt-1 text-sm text-gray-500 line-clamp-2">{asset.description}</p>
        <div className="mt-4 flex items-center justify-between text-xs text-gray-400">
          <span>{asset.publisher}</span>
          <div className="flex items-center gap-3">
            <span>⭐ {asset.rating}</span>
            <span>{asset.deployments.toLocaleString()} deploys</span>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap gap-1">
          {asset.tags.map((tag) => (
            <span key={tag} className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
              {tag}
            </span>
          ))}
        </div>
      </div>
    </Link>
  );
}
