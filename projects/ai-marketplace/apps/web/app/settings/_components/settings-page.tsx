"use client"

import { useState, useCallback } from "react"
import Link from "next/link"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Settings,
  Server,
  Zap,
  ShieldCheck,
  ChevronRight,
  Check,
  RotateCcw,
  Search,
  Info,
  Database,
  Briefcase,
  Scale,
  GitBranch,
  UserCog,
  BookOpen,
  FlaskConical,
  BarChart3,
  ClipboardList,
  Gavel,
  Rocket,
  Code2,
  Package,
  Building2,
  ClipboardCheck,
  ServerCog,
  Stethoscope,
} from "lucide-react"
import {
  PERSONA_TIERS,
  ALL_PERSONAS,
  getPersonaSettings,
  type PersonaId,
  type SettingCategory,
  type SettingField,
  type PersonaTier,
} from "../personas"

// ── Icon map (lucide names → components) ─────────────────────────────────────

const ICON_MAP: Record<string, React.ElementType> = {
  Database, Briefcase, Scale, ShieldCheck, GitBranch,
  UserCog, Stethoscope, BookOpen, FlaskConical, BarChart3,
  ServerCog, ClipboardList, Gavel, Rocket, Code2, Package,
  Building2, ClipboardCheck,
}

function PersonaIcon({ name, className }: { name: string; className?: string }) {
  const Icon = ICON_MAP[name] ?? Settings
  return <Icon className={className} />
}

// ── Category metadata ─────────────────────────────────────────────────────────

const CATEGORIES: {
  id: SettingCategory
  label: string
  shortLabel: string
  icon: React.ElementType
  description: string
  href: string
  color: string
  bgColor: string
  borderColor: string
}[] = [
  {
    id: "infrastructure",
    label: "Infrastructure",
    shortLabel: "Infra",
    icon: Server,
    description: "Backends, compute, Storage, IaC, network topology",
    href: "/settings/infrastructure",
    color: "text-sky-400",
    bgColor: "bg-sky-500/10",
    borderColor: "border-sky-500/40",
  },
  {
    id: "performance",
    label: "Performance",
    shortLabel: "Perf",
    icon: Zap,
    description: "Scaling, throughput, caching, latency tuning",
    href: "/settings/performance",
    color: "text-amber-400",
    bgColor: "bg-amber-500/10",
    borderColor: "border-amber-500/40",
  },
  {
    id: "reliability",
    label: "Reliability",
    shortLabel: "SLO",
    icon: ShieldCheck,
    description: "SLOs, error budgets, failover, backup, alerting",
    href: "/settings/reliability",
    color: "text-emerald-400",
    bgColor: "bg-emerald-500/10",
    borderColor: "border-emerald-500/40",
  },
]

// ── Tier color helpers ────────────────────────────────────────────────────────

function tierBorderClass(tier: PersonaTier): string {
  if (tier.tier === 1) return "border-[var(--optum-orange)]/40"
  if (tier.tier === 2) return "border-blue-500/40"
  return "border-violet-500/40"
}
function tierBgClass(tier: PersonaTier): string {
  if (tier.tier === 1) return "bg-[var(--optum-orange)]/10"
  if (tier.tier === 2) return "bg-blue-500/10"
  return "bg-violet-500/10"
}
function tierTextClass(tier: PersonaTier): string {
  if (tier.tier === 1) return "text-[var(--optum-orange)]"
  if (tier.tier === 2) return "text-blue-400"
  return "text-violet-400"
}
function tierActiveBg(tier: PersonaTier): string {
  if (tier.tier === 1) return "bg-[var(--optum-orange)]/20"
  if (tier.tier === 2) return "bg-blue-500/20"
  return "bg-violet-500/20"
}

// ── Setting field renderer ────────────────────────────────────────────────────

interface FieldProps {
  field: SettingField
  value: string | number | boolean | string[]
  onChange: (key: string, value: string | number | boolean | string[]) => void
}

function SettingFieldRow({ field, value, onChange }: FieldProps) {
  const isReadonly = field.type === "readonly"

  return (
    <div className="flex items-start gap-4 py-4 border-b border-border/50 last:border-0">
      {/* Label + description */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className={cn("text-sm font-medium", isReadonly ? "text-muted-foreground" : "text-foreground")}>
            {field.label}
          </span>
          {isReadonly && (
            <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-secondary text-muted-foreground">
              Read-only
            </span>
          )}
          {field.unit && (
            <span className="text-xs text-muted-foreground">({field.unit})</span>
          )}
        </div>
        <p className="mt-0.5 text-xs text-muted-foreground/70 leading-relaxed">
          {field.description}
        </p>
      </div>

      {/* Control */}
      <div className="flex-shrink-0 w-56">
        {field.type === "toggle" && (
          <button
            onClick={() => !isReadonly && onChange(field.key, !value)}
            className={cn(
              "relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              value ? "bg-[var(--optum-orange)]" : "bg-secondary",
              isReadonly && "opacity-50 cursor-not-allowed"
            )}
          >
            <span
              className={cn(
                "inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform",
                value ? "translate-x-6" : "translate-x-1"
              )}
            />
          </button>
        )}

        {field.type === "text" && (
          <input
            type="text"
            value={String(value)}
            onChange={(e) => onChange(field.key, e.target.value)}
            readOnly={isReadonly}
            className={cn(
              "w-full rounded-md border border-border bg-secondary/50 px-3 py-1.5 text-xs text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50 transition-colors",
              isReadonly && "opacity-60 cursor-not-allowed bg-secondary"
            )}
          />
        )}

        {field.type === "number" && (
          <input
            type="number"
            value={Number(value)}
            onChange={(e) => onChange(field.key, parseFloat(e.target.value) || 0)}
            readOnly={isReadonly}
            className={cn(
              "w-full rounded-md border border-border bg-secondary/50 px-3 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50 transition-colors",
              isReadonly && "opacity-60 cursor-not-allowed bg-secondary"
            )}
          />
        )}

        {field.type === "readonly" && (
          <div className="rounded-md border border-border bg-secondary/30 px-3 py-1.5 text-xs text-muted-foreground font-mono truncate">
            {String(value)}
          </div>
        )}

        {field.type === "select" && (
          <select
            value={String(value)}
            onChange={(e) => onChange(field.key, e.target.value)}
            disabled={isReadonly}
            className={cn(
              "w-full rounded-md border border-border bg-secondary/50 px-3 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50 transition-colors",
              isReadonly && "opacity-60 cursor-not-allowed"
            )}
          >
            {field.options?.map((opt) => (
              <option key={opt} value={opt} className="bg-background">
                {opt}
              </option>
            ))}
          </select>
        )}
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

interface SettingsPageContentProps {
  category: SettingCategory
}

export function SettingsPageContent({ category }: SettingsPageContentProps) {
  const [activePersona, setActivePersona] = useState<PersonaId>("devops-sre")
  const [search, setSearch] = useState("")
  const [savedState, setSavedState] = useState<Record<string, Record<string, string | number | boolean | string[]>>>({})
  const [justSaved, setJustSaved] = useState(false)

  const activeTier = PERSONA_TIERS.find((t) => t.personas.some((p) => p.id === activePersona))!
  const settings = getPersonaSettings(activePersona, category)
  const catMeta = CATEGORIES.find((c) => c.id === category)!

  // Build initial field values from defaults + saved overrides
  const getFieldValues = useCallback(() => {
    const storeKey = `${activePersona}:${category}`
    const saved = savedState[storeKey] ?? {}
    const defaults: Record<string, string | number | boolean | string[]> = {}
    settings?.fields.forEach((f) => {
      defaults[f.key] = f.defaultValue
    })
    return { ...defaults, ...saved }
  }, [activePersona, category, savedState, settings])

  const [localValues, setLocalValues] = useState<Record<string, string | number | boolean | string[]>>(getFieldValues)

  // When persona or category changes, reset local values
  const handlePersonaChange = (id: PersonaId) => {
    setActivePersona(id)
    const storeKey = `${id}:${category}`
    const saved = savedState[storeKey] ?? {}
    const newSettings = getPersonaSettings(id, category)
    const defaults: Record<string, string | number | boolean | string[]> = {}
    newSettings?.fields.forEach((f) => { defaults[f.key] = f.defaultValue })
    setLocalValues({ ...defaults, ...saved })
    setJustSaved(false)
  }

  const handleFieldChange = (key: string, value: string | number | boolean | string[]) => {
    setLocalValues((prev) => ({ ...prev, [key]: value }))
  }

  const handleSave = () => {
    const storeKey = `${activePersona}:${category}`
    setSavedState((prev) => ({ ...prev, [storeKey]: { ...localValues } }))
    setJustSaved(true)
    setTimeout(() => setJustSaved(false), 2000)
  }

  const handleReset = () => {
    const defaults: Record<string, string | number | boolean | string[]> = {}
    settings?.fields.forEach((f) => { defaults[f.key] = f.defaultValue })
    setLocalValues(defaults)
  }

  // Group fields by group key for display
  const fieldsByGroup: Record<string, SettingField[]> = {}
  settings?.fields.forEach((f) => {
    const g = f.group ?? "General"
    if (!fieldsByGroup[g]) fieldsByGroup[g] = []
    fieldsByGroup[g].push(f)
  })

  const filteredPersonas = search
    ? ALL_PERSONAS.filter(
        (p) =>
          p.label.toLowerCase().includes(search.toLowerCase()) ||
          p.desc.toLowerCase().includes(search.toLowerCase())
      )
    : null

  return (
    <div className="flex h-screen bg-background">
      <AppSidebar />

      <div className="ml-64 flex flex-1 flex-col overflow-hidden">
        {/* Page header */}
        <header className="flex h-14 items-center gap-3 border-b border-border px-6 flex-shrink-0">
          <Settings className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">Settings</span>
          <ChevronRight className="h-3.5 w-3.5 text-muted-foreground/40" />
          <span className={cn("text-sm font-medium", catMeta.color)}>{catMeta.label}</span>

          {/* Category tabs */}
          <div className="ml-auto flex items-center gap-1">
            {CATEGORIES.map((cat) => {
              const CatIcon = cat.icon
              const isActive = cat.id === category
              return (
                <Link
                  key={cat.id}
                  href={cat.href}
                  className={cn(
                    "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors",
                    isActive
                      ? cn(cat.bgColor, cat.color, "border", cat.borderColor)
                      : "text-muted-foreground hover:text-foreground hover:bg-secondary/50"
                  )}
                >
                  <CatIcon className="h-3.5 w-3.5" />
                  {cat.label}
                </Link>
              )
            })}
          </div>
        </header>

        <div className="flex flex-1 overflow-hidden">
          {/* Left: Persona picker */}
          <aside className="w-72 flex-shrink-0 border-r border-border flex flex-col overflow-hidden">
            <div className="p-3 border-b border-border">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground/50" />
                <input
                  type="text"
                  placeholder="Search personas…"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="w-full rounded-md border border-border bg-secondary/50 pl-8 pr-3 py-1.5 text-xs text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-[var(--optum-orange)]/50"
                />
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-2 space-y-3">
              {(filteredPersonas
                ? // Search mode: flat list
                  [{
                    tier: 0 as const,
                    label: `${filteredPersonas.length} result${filteredPersonas.length !== 1 ? "s" : ""}`,
                    shortLabel: "Results",
                    color: "gray",
                    borderColor: "border-border",
                    bgColor: "bg-secondary/30",
                    textColor: "text-muted-foreground",
                    badgeColor: "bg-secondary text-muted-foreground",
                    personas: filteredPersonas,
                  }]
                : PERSONA_TIERS
              ).map((tier) => (
                <div key={tier.tier}>
                  <div className={cn(
                    "flex items-center gap-2 rounded-md px-2 py-1.5 mb-1",
                    tier.tier !== 0 && tierBgClass(tier as PersonaTier)
                  )}>
                    <span className={cn(
                      "text-[10px] font-bold uppercase tracking-wider",
                      tier.tier !== 0 ? tierTextClass(tier as PersonaTier) : "text-muted-foreground"
                    )}>
                      {tier.label}
                    </span>
                    <span className={cn(
                      "rounded px-1.5 py-0.5 text-[10px] font-medium ml-auto",
                      tier.tier !== 0 ? (tier as PersonaTier).badgeColor : "bg-secondary text-muted-foreground"
                    )}>
                      {tier.personas.length}
                    </span>
                  </div>

                  <ul className="space-y-0.5">
                    {tier.personas.map((persona) => {
                      const isActive = persona.id === activePersona
                      const pTier = PERSONA_TIERS.find((t) => t.personas.some((p) => p.id === persona.id))
                      const hasSettings = !!getPersonaSettings(persona.id as PersonaId, category)

                      return (
                        <li key={persona.id}>
                          <button
                            onClick={() => handlePersonaChange(persona.id as PersonaId)}
                            className={cn(
                              "w-full group flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition-all",
                              isActive
                                ? cn(pTier ? tierActiveBg(pTier) : "bg-secondary", "border", pTier ? tierBorderClass(pTier) : "border-border")
                                : "hover:bg-secondary/50",
                              !hasSettings && "opacity-50"
                            )}
                          >
                            <span className={cn(
                              "flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-xs transition-colors",
                              isActive && pTier
                                ? cn(tierBgClass(pTier), tierTextClass(pTier))
                                : "bg-secondary/70 text-muted-foreground group-hover:text-foreground"
                            )}>
                              <PersonaIcon name={persona.icon} className="h-3.5 w-3.5" />
                            </span>
                            <div className="flex-1 min-w-0">
                              <p className={cn(
                                "text-xs font-medium leading-tight truncate",
                                isActive ? "text-foreground" : "text-muted-foreground group-hover:text-foreground"
                              )}>
                                {persona.shortLabel ?? persona.label}
                              </p>
                              {!hasSettings && (
                                <p className="text-[10px] text-muted-foreground/50">No settings in this category</p>
                              )}
                            </div>
                            {isActive && (
                              <ChevronRight className={cn(
                                "h-3.5 w-3.5 flex-shrink-0",
                                pTier ? tierTextClass(pTier) : "text-muted-foreground"
                              )} />
                            )}
                          </button>
                        </li>
                      )
                    })}
                  </ul>
                </div>
              ))}
            </div>

            {/* Optum footer */}
            <div className="border-t border-border p-3">
              <p className="text-[10px] text-muted-foreground/50 leading-relaxed text-center">
                © 2026 Optum, Inc. All rights reserved.
              </p>
            </div>
          </aside>

          {/* Right: Settings form */}
          <main className="flex-1 overflow-y-auto">
            {settings ? (
              <div className="max-w-3xl mx-auto p-6">
                {/* Persona + category header */}
                <div className={cn(
                  "rounded-xl border p-5 mb-6",
                  tierBgClass(activeTier),
                  tierBorderClass(activeTier)
                )}>
                  <div className="flex items-start gap-4">
                    <div className={cn(
                      "flex h-12 w-12 items-center justify-center rounded-xl border",
                      tierBgClass(activeTier),
                      tierBorderClass(activeTier)
                    )}>
                      <PersonaIcon
                        name={ALL_PERSONAS.find((p) => p.id === activePersona)?.icon ?? "Settings"}
                        className={cn("h-6 w-6", tierTextClass(activeTier))}
                      />
                    </div>
                    <div className="flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <h1 className="text-base font-semibold text-foreground">{settings.title}</h1>
                        <Badge variant="outline" className={cn("text-[10px]", tierTextClass(activeTier), tierBorderClass(activeTier))}>
                          {ALL_PERSONAS.find((p) => p.id === activePersona)?.label}
                        </Badge>
                        <Badge variant="outline" className={cn("text-[10px]", catMeta.color, catMeta.borderColor)}>
                          {catMeta.label}
                        </Badge>
                      </div>
                      <p className="mt-1 text-sm text-muted-foreground leading-relaxed">
                        {settings.description}
                      </p>
                    </div>
                  </div>
                </div>

                {/* Fields grouped by section */}
                <div className="space-y-5">
                  {Object.entries(fieldsByGroup).map(([group, fields]) => (
                    <Card key={group} className="border-border bg-card/50">
                      <CardHeader className="pb-2 pt-4 px-5">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                          {group}
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="px-5 pb-2">
                        {fields.map((field) => (
                          <SettingFieldRow
                            key={field.key}
                            field={field}
                            value={localValues[field.key] ?? field.defaultValue}
                            onChange={handleFieldChange}
                          />
                        ))}
                      </CardContent>
                    </Card>
                  ))}
                </div>

                {/* Save / Reset bar */}
                <div className="sticky bottom-0 mt-6 flex items-center justify-between rounded-xl border border-border bg-background/80 backdrop-blur-sm px-5 py-3 shadow-lg">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Info className="h-3.5 w-3.5" />
                    <span>Changes are saved per-persona and per-category.</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleReset}
                      className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
                    >
                      <RotateCcw className="h-3 w-3" />
                      Reset to defaults
                    </Button>
                    <Button
                      size="sm"
                      onClick={handleSave}
                      className={cn(
                        "h-8 gap-1.5 text-xs transition-all",
                        justSaved
                          ? "bg-emerald-600 hover:bg-emerald-600 text-white"
                          : "bg-[var(--optum-orange)] hover:bg-[var(--optum-orange-light)] text-white"
                      )}
                    >
                      {justSaved ? (
                        <>
                          <Check className="h-3 w-3" />
                          Saved
                        </>
                      ) : (
                        "Save changes"
                      )}
                    </Button>
                  </div>
                </div>
              </div>
            ) : (
              /* No settings for this persona in this category */
              <div className="flex flex-col items-center justify-center h-full text-center p-12">
                <div className={cn(
                  "flex h-16 w-16 items-center justify-center rounded-2xl border mb-4",
                  catMeta.bgColor, catMeta.borderColor
                )}>
                  <catMeta.icon className={cn("h-8 w-8", catMeta.color)} />
                </div>
                <h2 className="text-base font-semibold text-foreground mb-2">
                  No {catMeta.label} settings for this persona
                </h2>
                <p className="text-sm text-muted-foreground max-w-sm leading-relaxed mb-6">
                  The <span className="text-foreground font-medium">{ALL_PERSONAS.find((p) => p.id === activePersona)?.label}</span> persona
                  does not have configurable {catMeta.label.toLowerCase()} settings. Select a different persona from the left panel.
                </p>
                <div className="flex gap-2">
                  {CATEGORIES.filter((c) => c.id !== category).map((cat) => {
                    const hasOther = !!getPersonaSettings(activePersona, cat.id)
                    if (!hasOther) return null
                    const CatIcon = cat.icon
                    return (
                      <Link key={cat.id} href={cat.href}>
                        <Button variant="outline" size="sm" className={cn("gap-1.5 text-xs border", cat.borderColor, cat.color, cat.bgColor)}>
                          <CatIcon className="h-3.5 w-3.5" />
                          View {cat.label} settings
                        </Button>
                      </Link>
                    )
                  })}
                </div>
              </div>
            )}
          </main>
        </div>
      </div>
    </div>
  )
}
