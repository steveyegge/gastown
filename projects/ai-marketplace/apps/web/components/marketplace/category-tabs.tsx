"use client"

import { cn } from "@/lib/utils"
import {
  LayoutGrid,
  Bot,
  Server,
  Wrench,
  Brain,
  Workflow,
} from "lucide-react"

// Main asset type tabs (horizontal)
const mainTabs = [
  { id: "all", label: "All", icon: LayoutGrid },
  { id: "agent", label: "Agents", icon: Bot },
  { id: "mcp-server", label: "MCP Servers", icon: Server },
  { id: "mcp-tool", label: "MCP Tools", icon: Wrench },
  { id: "model", label: "Models", icon: Brain },
  { id: "workflow-template", label: "Workflows", icon: Workflow },
] as const

interface CategoryTabsProps {
  selectedTab: string
  onTabChange: (tab: string) => void
}

export function CategoryTabs({ selectedTab, onTabChange }: CategoryTabsProps) {
  return (
    <div className="border-b border-border">
      <div className="flex items-center gap-1 px-1">
        {mainTabs.map((tab) => {
          const Icon = tab.icon
          const isSelected = selectedTab === tab.id
          
          return (
            <button
              key={tab.id}
              onClick={() => onTabChange(tab.id)}
              className={cn(
                "relative flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors",
                isSelected
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <Icon className="h-4 w-4" />
              <span>{tab.label}</span>
              {isSelected && (
                <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-foreground" />
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
