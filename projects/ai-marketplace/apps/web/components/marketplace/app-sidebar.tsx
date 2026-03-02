"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"
import {
  LayoutGrid,
  Brain,
  Bot,
  Workflow,
  Shield,
  BarChart3,
  Settings,
  HelpCircle,
  Package,
  ChevronRight,
  Rocket,
  Code2,
  Activity,
  Users,
  Building2,
} from "lucide-react"

interface NavItem {
  label: string
  href: string
  icon: React.ElementType
  badge?: string
  badgeColor?: string
  description?: string
}

// UAP Marketplaces
const marketplaceItems: NavItem[] = [
  {
    label: "Agent Marketplace",
    href: "/",
    icon: Bot,
    description: "Reusable AI agents library",
  },
  {
    label: "Model Marketplace",
    href: "/models",
    icon: Brain,
    badge: "BYOM",
    badgeColor: "bg-[var(--optum-orange)]/20 text-[var(--optum-orange)]",
    description: "Governed AI models registry",
  },
]

// UAP Development & Deployment
const developmentItems: NavItem[] = [
  {
    label: "Agent Builder",
    href: "/orchestration",
    icon: Workflow,
    description: "Visual workflow designer",
  },
  {
    label: "IMDE",
    href: "/imde",
    icon: Code2,
    badge: "Dev",
    badgeColor: "bg-[var(--uhg-blue)]/20 text-[var(--uhg-blue-light)]",
    description: "Integrated Model Dev Environment",
  },
  {
    label: "One-Click Deploy",
    href: "/deployments",
    icon: Rocket,
    description: "CI/CD & deployment management",
  },
]

// UAP Governance & Operations
const governanceItems: NavItem[] = [
  {
    label: "Governance",
    href: "/governance",
    icon: Shield,
    description: "Compliance & audit workflows",
  },
  {
    label: "Observability",
    href: "/observability",
    icon: Activity,
    badge: "Live",
    badgeColor: "bg-emerald-500/20 text-emerald-400",
    description: "Run status: models, agents & tools",
  },
  {
    label: "Analytics",
    href: "/analytics",
    icon: BarChart3,
    description: "Workflow timing, SLA & adoption",
  },
]

const bottomNavItems: NavItem[] = [
  {
    label: "Settings",
    href: "/settings",
    icon: Settings,
  },
  {
    label: "Help & Docs",
    href: "/help",
    icon: HelpCircle,
  },
]

function NavSection({ title, items }: { title?: string; items: NavItem[] }) {
  const pathname = usePathname()

  return (
    <div className="mb-6">
      {title && (
        <h3 className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {title}
        </h3>
      )}
      <ul className="space-y-1">
        {items.map((item) => {
          const Icon = item.icon
          const isActive = pathname === item.href
          
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                className={cn(
                  "group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
                  isActive
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                )}
              >
                <span className={cn(
                  "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors",
                  isActive 
                    ? "bg-[var(--optum-orange)]/20 text-[var(--optum-orange)]" 
                    : "bg-secondary/50 text-muted-foreground group-hover:text-foreground"
                )}>
                  <Icon className="h-4 w-4" />
                </span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate">{item.label}</span>
                    {item.badge && (
                      <span className={cn(
                        "rounded px-1.5 py-0.5 text-xs font-medium",
                        item.badgeColor || "bg-secondary text-muted-foreground"
                      )}>
                        {item.badge}
                      </span>
                    )}
                  </div>
                  {item.description && (
                    <p className="truncate text-xs text-muted-foreground/70">
                      {item.description}
                    </p>
                  )}
                </div>
                {isActive && (
                  <ChevronRight className="h-4 w-4 text-[var(--optum-orange)]" />
                )}
              </Link>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

export function AppSidebar() {
  return (
    <aside className="fixed left-0 top-0 z-40 flex h-screen w-64 flex-col border-r border-border bg-sidebar">
      {/* Logo */}
      <div className="flex h-16 items-center gap-3 border-b border-border px-4">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-[var(--optum-orange)] to-[var(--optum-orange-light)]">
          <Building2 className="h-5 w-5 text-white" />
        </div>
        <div className="flex flex-col">
          <span className="text-sm font-bold text-foreground tracking-tight">AI Asset Marketplace</span>
          <span className="text-xs text-muted-foreground">Optum RCM Platform</span>
        </div>
      </div>
      
      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto p-3">
        <NavSection title="Marketplaces" items={marketplaceItems} />
        <NavSection title="Development" items={developmentItems} />
        <NavSection title="Operations" items={governanceItems} />
      </nav>
      
      {/* Bottom section */}
      <div className="border-t border-border p-3">
        <NavSection items={bottomNavItems} />
        
        {/* Organization Badge */}
        <div className="mt-3 rounded-lg border border-[var(--optum-orange)]/30 bg-[var(--optum-orange)]/5 p-3">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--optum-orange)]/20">
              <Users className="h-4 w-4 text-[var(--optum-orange)]" />
            </div>
            <div>
              <p className="text-xs font-semibold text-foreground">Optum RCM</p>
              <p className="text-xs text-muted-foreground">Healthcare Revenue Cycle</p>
            </div>
          </div>
        </div>
      </div>
    </aside>
  )
}
