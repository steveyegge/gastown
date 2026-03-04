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
  FlaskConical,
  BookOpen,
  GitBranch,
  RefreshCw,
  Upload,
  Database,
  HardDrive,
  Table2,
  Layers,
  Sparkles,
  Grid3X3,
  CloudCog,
  Server,
  Zap,
  ShieldCheck,
  LogOut,
} from "lucide-react"
import { useAccount, useMsal } from "@azure/msal-react"

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

// IMDE sub-navigation items
export const imdeSubItems: NavItem[] = [
  {
    label: "Workspace",
    href: "/imde",
    icon: Code2,
    description: "Sandboxes & compute",
  },
  {
    label: "Notebooks",
    href: "/imde/notebooks",
    icon: BookOpen,
    description: "Shared Jupyter notebooks",
  },
  {
    label: "Experiments",
    href: "/imde/experiments",
    icon: FlaskConical,
    description: "Run tracking & comparison",
  },
  {
    label: "Push to Marketplace",
    href: "/imde/push",
    icon: Upload,
    description: "One-command publish",
  },
  {
    label: "Collaboration",
    href: "/imde/collaboration",
    icon: GitBranch,
    description: "Version control & audit",
  },
  {
    label: "Improvement Loop",
    href: "/imde/improvement",
    icon: RefreshCw,
    badge: "Live",
    badgeColor: "bg-emerald-500/20 text-emerald-400",
    description: "Retrain from production",
  },
]

// Data sub-navigation items
export const dataSubItems: NavItem[] = [
  {
    label: "Overview",
    href: "/data",
    icon: Grid3X3,
    description: "All data sources & activity",
  },
  {
    label: "Azure Storage",
    href: "/data/storage",
    icon: HardDrive,
    description: "Blobs, containers & files",
  },
  {
    label: "Azure SQL",
    href: "/data/sql",
    icon: Table2,
    description: "Databases, tables & queries",
  },
  {
    label: "Cosmos DB",
    href: "/data/cosmos",
    icon: Database,
    description: "NoSQL containers & items",
  },
  {
    label: "Microsoft Fabric",
    href: "/data/fabric",
    icon: Layers,
    description: "Lakehouses & warehouses",
  },
  {
    label: "Built-in Datasets",
    href: "/data/datasets",
    icon: Sparkles,
    badge: "Hub",
    badgeColor: "bg-sky-500/20 text-sky-400",
    description: "Curated healthcare datasets",
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
    badge: "New",
    badgeColor: "bg-violet-500/20 text-violet-400",
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
    label: "Help & Docs",
    href: "/help",
    icon: HelpCircle,
  },
]

const settingsSubItems = [
  {
    label: "Infrastructure",
    href: "/settings/infrastructure",
    icon: Server,
    color: "text-sky-400",
    activeBg: "bg-sky-500/10",
    activeBorder: "border-sky-500/30",
  },
  {
    label: "Performance",
    href: "/settings/performance",
    icon: Zap,
    color: "text-amber-400",
    activeBg: "bg-amber-500/10",
    activeBorder: "border-amber-500/30",
  },
  {
    label: "Reliability",
    href: "/settings/reliability",
    icon: ShieldCheck,
    color: "text-emerald-400",
    activeBg: "bg-emerald-500/10",
    activeBorder: "border-emerald-500/30",
  },
]

function NavSection({ title, items, indent }: { title?: string; items: NavItem[]; indent?: boolean }) {
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
          const isActive = indent
            ? pathname === item.href || (item.href !== "/imde" && pathname.startsWith(item.href))
            : pathname === item.href

          return (
            <li key={item.href}>
              <Link
                href={item.href}
                className={cn(
                  "group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
                  indent ? "pl-6" : "",
                  isActive
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                )}
              >
                <span className={cn(
                  "flex shrink-0 items-center justify-center rounded-lg transition-colors",
                  indent ? "h-6 w-6" : "h-8 w-8",
                  isActive
                    ? "bg-[var(--optum-orange)]/20 text-[var(--optum-orange)]"
                    : "bg-secondary/50 text-muted-foreground group-hover:text-foreground"
                )}>
                  <Icon className={indent ? "h-3.5 w-3.5" : "h-4 w-4"} />
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
                  {!indent && item.description && (
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

function ImdeSection() {
  const pathname = usePathname()
  const isImde = pathname === "/imde" || pathname.startsWith("/imde/")
  const Icon = Code2
  const isMainActive = pathname === "/imde"

  return (
    <div className="mb-1">
      <Link
        href="/imde"
        className={cn(
          "group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
          isMainActive
            ? "bg-secondary text-foreground"
            : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
        )}
      >
        <span className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors",
          isImde
            ? "bg-violet-500/20 text-violet-400"
            : "bg-secondary/50 text-muted-foreground group-hover:text-foreground"
        )}>
          <Icon className="h-4 w-4" />
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate">IMDE</span>
            <span className="rounded px-1.5 py-0.5 text-xs font-medium bg-violet-500/20 text-violet-400">New</span>
          </div>
          <p className="truncate text-xs text-muted-foreground/70">Integrated Model Dev Environment</p>
        </div>
        <ChevronRight className={cn("h-4 w-4 transition-transform", isImde ? "rotate-90 text-violet-400" : "text-muted-foreground/40")} />
      </Link>
      {isImde && (
        <ul className="mt-0.5 space-y-0.5 pl-2">
          {imdeSubItems.map((item) => {
            const SubIcon = item.icon
            const isActive = pathname === item.href || (item.href !== "/imde" && pathname.startsWith(item.href))
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className={cn(
                    "group flex items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-all duration-150",
                    isActive
                      ? "bg-violet-500/10 text-violet-300"
                      : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                  )}
                >
                  <span className={cn(
                    "flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors",
                    isActive ? "text-violet-400" : "text-muted-foreground group-hover:text-foreground"
                  )}>
                    <SubIcon className="h-3.5 w-3.5" />
                  </span>
                  <span className="flex-1 truncate">{item.label}</span>
                  {item.badge && (
                    <span className={cn("rounded px-1 py-0.5 text-[10px] font-medium", item.badgeColor)}>
                      {item.badge}
                    </span>
                  )}
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function DataSection() {
  const pathname = usePathname()
  const isData = pathname === "/data" || pathname.startsWith("/data/")
  const Icon = Database
  const isMainActive = pathname === "/data"

  return (
    <div className="mb-1">
      <Link
        href="/data"
        className={cn(
          "group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
          isMainActive
            ? "bg-secondary text-foreground"
            : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
        )}
      >
        <span className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors",
          isData
            ? "bg-sky-500/20 text-sky-400"
            : "bg-secondary/50 text-muted-foreground group-hover:text-foreground"
        )}>
          <Icon className="h-4 w-4" />
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate">Data</span>
            <span className="rounded px-1.5 py-0.5 text-xs font-medium bg-sky-500/20 text-sky-400">New</span>
          </div>
          <p className="truncate text-xs text-muted-foreground/70">Storage, SQL, Cosmos & Fabric</p>
        </div>
        <ChevronRight className={cn("h-4 w-4 transition-transform", isData ? "rotate-90 text-sky-400" : "text-muted-foreground/40")} />
      </Link>
      {isData && (
        <ul className="mt-0.5 space-y-0.5 pl-2">
          {dataSubItems.map((item) => {
            const SubIcon = item.icon
            const isActive = item.href === "/data"
              ? pathname === "/data"
              : pathname === item.href || pathname.startsWith(item.href + "/")
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className={cn(
                    "group flex items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-all duration-150",
                    isActive
                      ? "bg-sky-500/10 text-sky-300"
                      : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                  )}
                >
                  <span className={cn(
                    "flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors",
                    isActive ? "text-sky-400" : "text-muted-foreground group-hover:text-foreground"
                  )}>
                    <SubIcon className="h-3.5 w-3.5" />
                  </span>
                  <span className="flex-1 truncate">{item.label}</span>
                  {item.badge && (
                    <span className={cn("rounded px-1 py-0.5 text-[10px] font-medium", item.badgeColor)}>
                      {item.badge}
                    </span>
                  )}
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function UserSection() {
  const { instance } = useMsal()
  const account = useAccount()

  if (!account) return null

  const initials = account.name
    ? account.name.split(" ").map((n) => n[0]).slice(0, 2).join("").toUpperCase()
    : account.username[0]?.toUpperCase() ?? "?"

  const handleSignOut = () => {
    instance.logoutRedirect({ postLogoutRedirectUri: "/" }).catch(console.error)
  }

  return (
    <div className="rounded-lg border border-border bg-secondary/30 p-3">
      <div className="flex items-center gap-2.5">
        {/* Avatar */}
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-[var(--optum-orange)]/20 text-xs font-bold text-[var(--optum-orange)]">
          {initials}
        </div>
        {/* Name + email */}
        <div className="flex-1 min-w-0">
          <p className="truncate text-xs font-semibold text-foreground leading-tight">
            {account.name ?? account.username}
          </p>
          <p className="truncate text-xs text-muted-foreground leading-tight">
            {account.username}
          </p>
        </div>
        {/* Sign out */}
        <button
          onClick={handleSignOut}
          title="Sign out"
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
        >
          <LogOut className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  )
}

function SettingsSection() {
  const pathname = usePathname()
  const isSettings = pathname.startsWith("/settings")

  return (
    <div className="mb-1">
      <Link
        href="/settings/infrastructure"
        className={cn(
          "group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-150",
          isSettings
            ? "bg-secondary text-foreground"
            : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
        )}
      >
        <span className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors",
          isSettings
            ? "bg-slate-500/20 text-slate-300"
            : "bg-secondary/50 text-muted-foreground group-hover:text-foreground"
        )}>
          <Settings className="h-4 w-4" />
        </span>
        <div className="flex-1 min-w-0">
          <span className="truncate">Settings</span>
          <p className="truncate text-xs text-muted-foreground/70">Persona-based configuration</p>
        </div>
        <ChevronRight className={cn(
          "h-4 w-4 transition-transform",
          isSettings ? "rotate-90 text-slate-400" : "text-muted-foreground/40"
        )} />
      </Link>

      {isSettings && (
        <ul className="mt-0.5 space-y-0.5 pl-2">
          {settingsSubItems.map((item) => {
            const SubIcon = item.icon
            const isActive = pathname.startsWith(item.href)
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className={cn(
                    "group flex items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-all duration-150",
                    isActive
                      ? cn(item.activeBg, "border", item.activeBorder, item.color)
                      : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                  )}
                >
                  <span className={cn(
                    "flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors",
                    isActive ? item.color : "text-muted-foreground group-hover:text-foreground"
                  )}>
                    <SubIcon className="h-3.5 w-3.5" />
                  </span>
                  <span className="flex-1 truncate">{item.label}</span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function DevelopmentSection() {
  return (
    <div className="mb-6">
      <h3 className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        Development
      </h3>
      <ul className="space-y-1">
        <li>
          <NavSection items={[{ label: "Agent Builder", href: "/orchestration", icon: Workflow, description: "Visual workflow designer" }]} />
        </li>
        <li>
          <ImdeSection />
        </li>
        <li>
          <NavSection items={[{ label: "One-Click Deploy", href: "/deployments", icon: Rocket, description: "CI/CD & deployment management" }]} />
        </li>
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
        <DevelopmentSection />
        <div className="mb-6">
          <h3 className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Data
          </h3>
          <ul className="space-y-1">
            <li><DataSection /></li>
          </ul>
        </div>
        <NavSection title="Operations" items={governanceItems} />
      </nav>
      
      {/* Bottom section */}
      <div className="border-t border-border p-3">
        <div className="mb-1"><SettingsSection /></div>
        <NavSection items={bottomNavItems} />
        
        {/* Signed-in user */}
        <div className="mt-3">
          <UserSection />
        </div>
      </div>
    </aside>
  )
}
