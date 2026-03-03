"use client"

import Link from "next/link"
import { Asset, AssetType } from "@/lib/types"
import { Badge } from "@/components/ui/badge"
import { 
  MessageSquare, 
  Code, 
  Database, 
  GitBranch, 
  Table, 
  FileText, 
  Brain, 
  Eye, 
  Workflow, 
  Shield,
  BarChart,
  MessageCircle,
  BadgeCheck,
  Star,
  Calendar,
  DollarSign,
  Stethoscope,
  ClipboardList,
  Bot,
  Server,
  Wrench
} from "lucide-react"
import { cn } from "@/lib/utils"

const iconMap: Record<string, React.ElementType> = {
  MessageSquare,
  Code,
  Database,
  GitBranch,
  Table,
  FileText,
  Brain,
  Eye,
  Workflow,
  Shield,
  BarChart,
  MessageCircle,
  DollarSign,
  Stethoscope,
  ClipboardList,
}

// Type badge colors matching Azure AI Foundry style
const typeBadgeConfig: Record<AssetType, { label: string; className: string; icon: React.ElementType }> = {
  "agent": {
    label: "Agent",
    className: "bg-blue-500/20 text-blue-400 border-blue-500/30",
    icon: Bot,
  },
  "mcp-server": {
    label: "MCP Server",
    className: "bg-teal-500/20 text-teal-400 border-teal-500/30",
    icon: Server,
  },
  "mcp-tool": {
    label: "Tool",
    className: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
    icon: Wrench,
  },
  "model": {
    label: "Model",
    className: "bg-purple-500/20 text-purple-400 border-purple-500/30",
    icon: Brain,
  },
  "workflow-template": {
    label: "Workflow",
    className: "bg-amber-500/20 text-amber-400 border-amber-500/30",
    icon: Workflow,
  },
}

interface AssetCardProps {
  asset: Asset
  variant?: "list" | "card"
}

// Calculate star rating based on orchestration usage
function getUsageStars(usage: number): number {
  if (usage >= 2000) return 5
  if (usage >= 1000) return 4
  if (usage >= 500) return 3
  if (usage >= 200) return 2
  return 1
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString("en-US", { month: "short", year: "numeric" })
}

function StarRating({ stars, usage }: { stars: number; usage: number }) {
  return (
    <div className="flex items-center gap-0.5" title={`Used in ${usage.toLocaleString()} orchestrations`}>
      {[1, 2, 3, 4, 5].map((i) => (
        <Star
          key={i}
          className={cn(
            "h-3 w-3",
            i <= stars
              ? "fill-amber-400 text-amber-400"
              : "fill-muted/30 text-muted/30"
          )}
        />
      ))}
      <span className="ml-1.5 text-xs text-muted-foreground">{usage.toLocaleString()}</span>
    </div>
  )
}

export function AssetCard({ asset, variant = "card" }: AssetCardProps) {
  const Icon = iconMap[asset.icon] || Brain
  const typeConfig = typeBadgeConfig[asset.type]
  const TypeIcon = typeConfig.icon
  const usageStars = getUsageStars(asset.orchestrationUsage)

  // Card variant (default) - Azure AI Foundry style
  return (
    <Link href={`/asset/${asset.id}`} className="group block">
      <div className="flex h-full flex-col rounded-xl border border-border bg-card p-5 transition-all duration-200 hover:border-muted-foreground/40 hover:bg-card/80">
        {/* Header with icon and type badge */}
        <div className="mb-4 flex items-start justify-between">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-secondary/80">
            <Icon className="h-6 w-6 text-foreground" />
          </div>
          <div className={cn(
            "flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium",
            typeConfig.className
          )}>
            <TypeIcon className="h-3 w-3" />
            {typeConfig.label}
          </div>
        </div>
        
        {/* Title */}
        <h3 className="mb-1.5 text-base font-semibold text-foreground group-hover:text-blue-400 transition-colors">
          {asset.name}
        </h3>
        
        {/* Summary */}
        <p className="mb-4 line-clamp-2 text-sm leading-relaxed text-muted-foreground">
          {asset.summary}
        </p>
        
        {/* Capabilities as tags */}
        <div className="mb-4 flex flex-wrap gap-1.5">
          {asset.capabilities.slice(0, 3).map((cap) => (
            <span
              key={cap}
              className="rounded-md bg-secondary/60 px-2 py-0.5 text-xs text-muted-foreground"
            >
              {cap}
            </span>
          ))}
          {asset.capabilities.length > 3 && (
            <span className="rounded-md bg-secondary/60 px-2 py-0.5 text-xs text-muted-foreground">
              +{asset.capabilities.length - 3}
            </span>
          )}
        </div>
        
        {/* Star rating */}
        <div className="mb-4">
          <StarRating stars={usageStars} usage={asset.orchestrationUsage} />
        </div>
        
        {/* Footer */}
        <div className="mt-auto flex items-center justify-between border-t border-border/50 pt-4">
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-muted-foreground">{asset.publisher}</span>
            {asset.publisherVerified && (
              <BadgeCheck className="h-3.5 w-3.5 text-blue-400" />
            )}
          </div>
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1">
              <Calendar className="h-3 w-3" />
              {formatDate(asset.publishedDate)}
            </span>
            <Badge 
              variant="outline"
              className={cn(
                "text-xs",
                asset.pricing === "Free" && "border-emerald-500/30 bg-emerald-500/10 text-emerald-400",
                asset.pricing === "Pro" && "border-blue-500/30 bg-blue-500/10 text-blue-400",
                asset.pricing === "Enterprise" && "border-purple-500/30 bg-purple-500/10 text-purple-400"
              )}
            >
              {asset.pricing}
            </Badge>
          </div>
        </div>
      </div>
    </Link>
  )
}
