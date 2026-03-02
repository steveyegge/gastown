"use client"

import { useState, useMemo } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { 
  Search, 
  ChevronDown, 
  Brain, 
  Shield, 
  BarChart3, 
  Eye, 
  FileText,
  CheckCircle2,
  Clock,
  Users,
  Star,
  ThumbsUp,
  ThumbsDown,
  Heart,
  Copy,
  ExternalLink,
  Filter,
  Plus,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import Link from "next/link"
import { models } from "@/lib/models-data"

const categories = ["All Categories", "NLP", "Vision", "Prediction", "Analytics"]
const types = ["All Types", "Internal", "Partner"]
const statuses = ["All Status", "Production", "Beta", "Review"]

function ModelCard({ model }: { model: typeof models[0] }) {
  const statusColors: Record<string, string> = {
    production: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
    beta: "bg-amber-500/20 text-amber-400 border-amber-500/30",
    review: "bg-blue-500/20 text-blue-400 border-blue-500/30",
  }
  
  const typeColors: Record<string, string> = {
    internal: "bg-cyan-500/20 text-cyan-400",
    partner: "bg-purple-500/20 text-purple-400",
  }

  return (
    <Link href={`/models/${model.id}`} className="group block">
      <div className="flex h-full flex-col rounded-xl border border-border bg-card p-5 transition-all duration-200 hover:border-[var(--optum-orange)]/40 hover:bg-card/80">
        {/* Header */}
        <div className="mb-4 flex items-start justify-between">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-[var(--optum-orange)]/20 to-[var(--uhg-blue)]/20">
            <Brain className="h-6 w-6 text-[var(--optum-orange)]" />
          </div>
          <div className="flex items-center gap-2">
            <span className={cn(
              "rounded px-2 py-0.5 text-xs font-medium",
              typeColors[model.type]
            )}>
              {model.type}
            </span>
            <span className={cn(
              "rounded-full border px-2 py-0.5 text-xs font-medium",
              statusColors[model.status]
            )}>
              {model.status}
            </span>
          </div>
        </div>
        
        {/* Title and Version */}
        <div className="mb-2">
          <h3 className="text-base font-semibold text-foreground group-hover:text-[var(--optum-orange)] transition-colors">
            {model.name}
          </h3>
          <p className="text-xs text-muted-foreground">
            {model.version} · by {model.publisher}
            {model.publisherVerified && (
              <CheckCircle2 className="ml-1 inline h-3 w-3 text-[var(--optum-orange)]" />
            )}
          </p>
        </div>
        
        {/* Description */}
        <p className="mb-4 line-clamp-2 text-sm leading-relaxed text-muted-foreground">
          {model.description}
        </p>
        
        {/* Metrics */}
        <div className="mb-4 grid grid-cols-3 gap-2">
          <div className="rounded-lg bg-secondary/50 p-2 text-center">
            <p className="text-xs text-muted-foreground">Accuracy</p>
            <p className="text-sm font-semibold text-emerald-400">{model.metrics.accuracy}%</p>
          </div>
          <div className="rounded-lg bg-secondary/50 p-2 text-center">
            <p className="text-xs text-muted-foreground">Latency</p>
            <p className="text-sm font-semibold text-foreground">{model.metrics.latency}ms</p>
          </div>
          <div className="rounded-lg bg-secondary/50 p-2 text-center">
            <p className="text-xs text-muted-foreground">Teams</p>
            <p className="text-sm font-semibold text-foreground">{model.teams}</p>
          </div>
        </div>
        
        {/* Compliance Badges */}
        <div className="mb-4 flex flex-wrap gap-1.5">
          {model.compliance.map((badge) => (
            <span
              key={badge}
              className="flex items-center gap-1 rounded bg-secondary/60 px-2 py-0.5 text-xs text-muted-foreground"
            >
              <Shield className="h-3 w-3" />
              {badge}
            </span>
          ))}
        </div>
        
        {/* Footer */}
        <div className="mt-auto flex items-center justify-between border-t border-border/50 pt-4">
          <div className="flex items-center gap-0.5">
            {[1, 2, 3, 4, 5].map((i) => (
              <Star
                key={i}
                className={cn(
                  "h-3 w-3",
                  i <= Math.floor(model.rating)
                    ? "fill-amber-400 text-amber-400"
                    : "fill-muted/30 text-muted/30"
                )}
              />
            ))}
            <span className="ml-1.5 text-xs text-muted-foreground">{model.rating}</span>
          </div>
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1">
              <Users className="h-3 w-3" />
              {model.downloads.toLocaleString()}
            </span>
            <span className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {new Date(model.lastUpdated).toLocaleDateString("en-US", { month: "short", day: "numeric" })}
            </span>
          </div>
        </div>
      </div>
    </Link>
  )
}

export default function ModelMarketplacePage() {
  const [searchQuery, setSearchQuery] = useState("")
  const [selectedCategory, setSelectedCategory] = useState("All Categories")
  const [selectedType, setSelectedType] = useState("All Types")
  const [selectedStatus, setSelectedStatus] = useState("All Status")

  const filteredModels = useMemo(() => {
    return models.filter((model) => {
      const matchesSearch =
        searchQuery === "" ||
        model.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        model.description.toLowerCase().includes(searchQuery.toLowerCase())
      const matchesCategory =
        selectedCategory === "All Categories" || model.category === selectedCategory
      const matchesType =
        selectedType === "All Types" || model.type.toLowerCase() === selectedType.toLowerCase()
      const matchesStatus =
        selectedStatus === "All Status" || model.status.toLowerCase() === selectedStatus.toLowerCase()
      return matchesSearch && matchesCategory && matchesType && matchesStatus
    })
  }, [searchQuery, selectedCategory, selectedType, selectedStatus])

  // Stats
  const stats = {
    total: models.length,
    production: models.filter(m => m.status === "production").length,
    internal: models.filter(m => m.type === "internal").length,
    partner: models.filter(m => m.type === "partner").length,
  }

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      
      <main className="ml-64 p-6">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-semibold text-foreground">Model Marketplace</h1>
              <p className="mt-1 text-muted-foreground">
                One-stop registry for AI models — discover, govern, and reuse across teams
              </p>
            </div>
            <div className="flex items-center gap-3">
              <Button variant="outline" size="sm" className="gap-2">
                <FileText className="h-4 w-4" />
                Documentation
              </Button>
              <Button size="sm" className="gap-2 bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white">
                <Plus className="h-4 w-4" />
                Register Model (BYOM)
              </Button>
            </div>
          </div>
        </div>
        
        {/* Stats Cards */}
        <div className="mb-8 grid grid-cols-4 gap-4">
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--optum-orange)]/20">
                <Brain className="h-5 w-5 text-[var(--optum-orange)]" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-foreground">{stats.total}</p>
                <p className="text-xs text-muted-foreground">Total Models</p>
              </div>
            </div>
          </div>
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--success)]/20">
                <CheckCircle2 className="h-5 w-5 text-[var(--success)]" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-foreground">{stats.production}</p>
                <p className="text-xs text-muted-foreground">In Production</p>
              </div>
            </div>
          </div>
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--uhg-blue)]/20">
                <Users className="h-5 w-5 text-[var(--uhg-blue-light)]" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-foreground">{stats.internal}</p>
                <p className="text-xs text-muted-foreground">Internal Models</p>
              </div>
            </div>
          </div>
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--optum-teal)]/20">
                <ExternalLink className="h-5 w-5 text-[var(--optum-teal)]" />
              </div>
              <div>
                <p className="text-2xl font-semibold text-foreground">{stats.partner}</p>
                <p className="text-xs text-muted-foreground">Partner/BYOM Models</p>
              </div>
            </div>
          </div>
        </div>
        
        {/* Value Proposition - UAP PRD Aligned */}
        <div className="mb-8 rounded-xl border border-[var(--optum-orange)]/30 bg-gradient-to-r from-[var(--optum-orange)]/5 to-[var(--uhg-blue)]/5 p-6">
          <div className="flex items-start gap-6">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-[var(--optum-orange)]/20">
              <Shield className="h-6 w-6 text-[var(--optum-orange)]" />
            </div>
            <div>
              <h3 className="mb-2 text-lg font-semibold text-foreground">Governed Model Registry with BYOM Support</h3>
              <p className="text-sm text-muted-foreground leading-relaxed">
                Every model (internal or from partners) is registered with metadata, documented with Model Report Cards, 
                and monitored centrally. Models move through <span className="text-[var(--optum-orange)]">Draft → Review → Approved</span> before 
                production use. Share innovations so one team's model benefits others, while enforcing HIPAA/SOC2 standards 
                and avoiding duplicate development. MCP-Server integration ensures all models are exposed for agentic workflows.
              </p>
            </div>
          </div>
        </div>
        
        {/* Search and Filters */}
        <div className="mb-6 flex items-center gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search models by name or description..."
              className="h-11 w-full rounded-lg border border-border bg-card pl-10 pr-4 text-sm text-foreground placeholder:text-muted-foreground focus:border-[var(--optum-orange)]/50 focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50"
            />
          </div>
          <button className="flex h-11 items-center gap-2 rounded-lg border border-border bg-card px-4 text-sm text-muted-foreground hover:text-foreground transition-colors">
            {selectedCategory}
            <ChevronDown className="h-4 w-4" />
          </button>
          <button className="flex h-11 items-center gap-2 rounded-lg border border-border bg-card px-4 text-sm text-muted-foreground hover:text-foreground transition-colors">
            {selectedType}
            <ChevronDown className="h-4 w-4" />
          </button>
          <button className="flex h-11 items-center gap-2 rounded-lg border border-border bg-card px-4 text-sm text-muted-foreground hover:text-foreground transition-colors">
            {selectedStatus}
            <ChevronDown className="h-4 w-4" />
          </button>
        </div>
        
        {/* Results count */}
        <div className="mb-4 text-sm text-muted-foreground">
          Showing {filteredModels.length} models
        </div>
        
        {/* Model Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filteredModels.map((model) => (
            <ModelCard key={model.id} model={model} />
          ))}
        </div>
        
        {filteredModels.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-secondary">
              <Brain className="h-8 w-8 text-muted-foreground" />
            </div>
            <h3 className="mb-2 text-lg font-medium text-foreground">No models found</h3>
            <p className="max-w-md text-sm text-muted-foreground">
              Try adjusting your filters to find what you're looking for.
            </p>
          </div>
        )}
      </main>
    </div>
  )
}
