"use client"

import { useState, useMemo } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { HeroBanner } from "@/components/marketplace/hero-banner"
import { NavTabs } from "@/components/marketplace/nav-tabs"
import { AssetCard, ContributeCard } from "@/components/marketplace/plugin-card"
import { assets } from "@/lib/mock-data"
import { Search, ChevronDown, Copy } from "lucide-react"

export default function MarketplacePage() {
  const [selectedTab, setSelectedTab] = useState("agents")
  const [searchQuery, setSearchQuery] = useState("")
  const [category, setCategory] = useState("All Categories")
  const [sortBy, setSortBy] = useState("Name")

  const filteredAssets = useMemo(() => {
    return assets.filter((asset) => {
      const matchesTab =
        selectedTab === "agents" ? asset.type === "agent" :
        selectedTab === "tools"  ? (asset.type === "mcp-tool" || asset.type === "mcp-server") :
        selectedTab === "models" ? asset.type === "model" :
        selectedTab === "skills" ? asset.type === "workflow-template" :
        true // stats / mcp / default: show all

      const matchesSearch =
        searchQuery === "" ||
        asset.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        asset.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
        asset.summary.toLowerCase().includes(searchQuery.toLowerCase())

      return matchesTab && matchesSearch
    })
  }, [selectedTab, searchQuery])

  // Counts for tabs
  const counts = {
    agents: assets.filter(a => a.type === "agent").length,
    tools:  assets.filter(a => a.type === "mcp-tool" || a.type === "mcp-server").length,
    models: assets.filter(a => a.type === "model").length,
    skills: assets.reduce((acc, a) => acc + a.capabilities.length, 0),
  }

  // Get tab title
  const getTabTitle = () => {
    switch (selectedTab) {
      case "agents": return "Discover Agents"
      case "tools":  return "Discover Tools"
      case "models": return "Discover Models"
      case "skills": return "Discover Skills"
      case "stats":  return "Marketplace Stats"
      default:       return "Discover Agents"
    }
  }

  const getTabDescription = () => {
    switch (selectedTab) {
      case "agents": return "AI agents for healthcare RCM automation and intelligent workflows."
      case "tools":  return "MCP tools and servers to extend agent capabilities."
      case "models": return `${counts.models} foundation models deployed on Azure AI Foundry — click any card to view the live Model Card with benchmarks and safety metrics.`
      case "skills": return "Reusable skills that power agent actions and integrations."
      case "stats":  return "View marketplace analytics and trends."
      default:       return "AI agents for healthcare RCM automation and intelligent workflows."
    }
  }

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      
      <main className="ml-64 p-6">
        {/* Hero Banner */}
        <div className="mb-6">
          <HeroBanner />
        </div>
        
        {/* Navigation Tabs */}
        <div className="mb-8">
          <NavTabs
            selectedTab={selectedTab}
            onTabChange={setSelectedTab}
            counts={counts}
          />
        </div>
        
        {/* Page Title */}
        <div className="mb-6">
          <h2 className="mb-2 text-2xl font-semibold text-foreground">
            {getTabTitle()}
          </h2>
          <p className="text-muted-foreground">
            {getTabDescription()}
          </p>
        </div>
        
        {/* Install Command */}
        <div className="mb-6 inline-flex items-center gap-3 rounded-lg border border-[var(--optum-orange)]/30 bg-card/50 px-4 py-2.5">
          <span className="text-sm text-muted-foreground">Install any agent in one command:</span>
          <code className="text-sm">
            <span className="text-[var(--optum-orange)]">/agent install</span>
            <span className="text-[var(--success)]"> {'<name>'}@uap-marketplace</span>
          </code>
          <button className="text-muted-foreground hover:text-foreground transition-colors">
            <Copy className="h-4 w-4" />
          </button>
        </div>
        
        {/* Search and Filters */}
        <div className="mb-6 flex items-center gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search agents, tools, and skills..."
              className="h-11 w-full rounded-lg border border-border bg-card pl-10 pr-4 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50"
            />
          </div>
          <button className="flex h-11 items-center gap-2 rounded-lg border border-border bg-card px-4 text-sm text-muted-foreground hover:text-foreground transition-colors">
            {category}
            <ChevronDown className="h-4 w-4" />
          </button>
          <button className="flex h-11 items-center gap-2 rounded-lg border border-border bg-card px-4 text-sm text-muted-foreground hover:text-foreground transition-colors">
            Sort by {sortBy}
            <ChevronDown className="h-4 w-4" />
          </button>
        </div>
        
        {/* Cards Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {/* Contribute Card */}
          <ContributeCard />
          
          {/* Asset Cards */}
          {filteredAssets.map((asset) => (
            <AssetCard key={asset.id} asset={asset} />
          ))}
        </div>
        
        {filteredAssets.length === 0 && searchQuery && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-secondary">
              <Search className="h-8 w-8 text-muted-foreground" />
            </div>
            <h3 className="mb-2 text-lg font-medium text-foreground">No assets found</h3>
            <p className="max-w-md text-sm text-muted-foreground">
              Try adjusting your search to find what you're looking for.
            </p>
          </div>
        )}
      </main>
    </div>
  )
}
