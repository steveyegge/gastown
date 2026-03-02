"use client"

import { Building2, Sparkles } from "lucide-react"

export function HeroBanner() {
  return (
    <div className="relative overflow-hidden rounded-xl border border-[var(--optum-orange)]/30 bg-gradient-to-br from-[var(--background)] via-[var(--card)] to-[var(--background)]">
      {/* Background pattern */}
      <div className="absolute inset-0 opacity-30">
        <div className="absolute inset-0" style={{
          backgroundImage: `repeating-linear-gradient(
            0deg,
            transparent,
            transparent 2px,
            rgba(var(--optum-orange), 0.03) 2px,
            rgba(var(--optum-orange), 0.03) 4px
          ),
          repeating-linear-gradient(
            90deg,
            transparent,
            transparent 2px,
            rgba(var(--optum-orange), 0.03) 2px,
            rgba(var(--optum-orange), 0.03) 4px
          )`
        }} />
      </div>
      
      {/* Gradient overlays - Optum brand colors */}
      <div className="absolute left-0 top-0 h-full w-1/3 bg-gradient-to-r from-[var(--optum-orange)]/15 via-transparent to-transparent" />
      <div className="absolute right-0 top-0 h-full w-1/3 bg-gradient-to-l from-[var(--uhg-blue)]/15 via-transparent to-transparent" />
      
      {/* Content */}
      <div className="relative flex flex-col items-center justify-center py-10 px-6">
        {/* UAP Logo */}
        <div className="mb-3 flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-[var(--optum-orange)] to-[var(--optum-orange-light)]">
            <Building2 className="h-6 w-6 text-white" />
          </div>
        </div>
        
        {/* Title */}
        <h1 className="mb-2 text-3xl font-bold tracking-tight text-foreground">
          AI Asset Marketplace
        </h1>
        <p className="mb-4 text-sm text-muted-foreground text-center max-w-md">
          Accelerating Healthcare RCM with AI-powered automation, governed models, and intelligent workflows
        </p>
        
        {/* Badges */}
        <div className="flex items-center gap-3">
          <span className="text-xs font-medium uppercase tracking-widest text-[var(--optum-orange)]">
            Agent Marketplace
          </span>
          <span className="flex items-center gap-1.5 rounded-full bg-[var(--uhg-blue)]/20 px-3 py-1 text-xs font-medium text-[var(--uhg-blue-light)]">
            <Sparkles className="h-3 w-3" />
            Optum RCM
          </span>
        </div>
      </div>
    </div>
  )
}
