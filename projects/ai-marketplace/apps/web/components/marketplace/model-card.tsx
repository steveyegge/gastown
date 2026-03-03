"use client"

import { useEffect, useState } from "react"
import {
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  Radar,
  ResponsiveContainer,
  Tooltip,
} from "recharts"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { Progress } from "@/components/ui/progress"
import { AlertCircle, CheckCircle2, ShieldCheck, Sparkles, Clock } from "lucide-react"
import type { ModelCardData, SafetyMetric } from "@/lib/types"

// ── props ─────────────────────────────────────────────────────────────────────

interface ModelCardProps {
  assetId: string
}

// ── helpers ───────────────────────────────────────────────────────────────────

const SEVERITY_CONFIG: Record<
  SafetyMetric["severity"],
  { label: string; color: string; icon: typeof CheckCircle2 }
> = {
  none: {
    label: "None",
    color: "text-emerald-400 border-emerald-400/30 bg-emerald-400/10",
    icon: CheckCircle2,
  },
  low: {
    label: "Low",
    color: "text-yellow-400 border-yellow-400/30 bg-yellow-400/10",
    icon: ShieldCheck,
  },
  medium: {
    label: "Medium",
    color: "text-orange-400 border-orange-400/30 bg-orange-400/10",
    icon: AlertCircle,
  },
  high: {
    label: "High",
    color: "text-red-400 border-red-400/30 bg-red-400/10",
    icon: AlertCircle,
  },
}

function formatDate(iso: string) {
  try {
    return new Intl.DateTimeFormat("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    }).format(new Date(iso))
  } catch {
    return iso
  }
}

// ── sub-components ────────────────────────────────────────────────────────────

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h4 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
      {children}
    </h4>
  )
}

function LoadingSkeleton() {
  return (
    <div className="space-y-8 animate-pulse">
      <div className="flex items-center gap-2">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-5 w-24" />
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <Skeleton className="h-56 rounded-lg" />
        <div className="space-y-4">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="space-y-1">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-2 w-full" />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ── main component ────────────────────────────────────────────────────────────

export function ModelCard({ assetId }: ModelCardProps) {
  const [data, setData] = useState<ModelCardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    fetch(`/api/foundry/${encodeURIComponent(assetId)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json() as Promise<ModelCardData>
      })
      .then((d) => {
        if (!cancelled) {
          setData(d)
          setLoading(false)
        }
      })
      .catch((err) => {
        if (!cancelled) {
          console.error("[ModelCard] fetch failed:", err)
          setError("Unable to load model card data.")
          setLoading(false)
        }
      })

    return () => { cancelled = true }
  }, [assetId])

  if (loading) return <LoadingSkeleton />

  if (error || !data) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        <AlertCircle className="h-4 w-4 shrink-0" />
        {error ?? "No model card data available."}
      </div>
    )
  }

  // Build radar data — normalise all metrics to 0-100
  const radarData = data.evaluationMetrics
    .filter((m) => m.maxScore === 5)
    .map((m) => ({
      metric: m.name,
      score: Math.round((m.score / m.maxScore) * 100),
      fullMark: 100,
    }))

  const scoreMetrics = data.evaluationMetrics.filter((m) => m.maxScore === 5)
  const otherMetrics = data.evaluationMetrics.filter((m) => m.maxScore !== 5)

  return (
    <div className="space-y-8">
      {/* ── header row ── */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Sparkles className="h-5 w-5 text-accent" />
          <span className="text-base font-semibold text-foreground">
            Azure AI Foundry Model Card
          </span>
          <Badge
            variant="outline"
            className={
              data.source === "foundry"
                ? "border-accent/40 bg-accent/10 text-accent"
                : "border-muted-foreground/30 text-muted-foreground"
            }
          >
            {data.source === "foundry" ? "Live · Foundry" : "Preview · Demo data"}
          </Badge>
        </div>
        {data.evaluatedAt && (
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <Clock className="h-3 w-3" />
            Evaluated {formatDate(data.evaluatedAt)}
          </span>
        )}
      </div>

      {/* ── evaluation metrics ── */}
      <div className="grid gap-6 md:grid-cols-2">
        {/* Radar chart */}
        {radarData.length > 2 && (
          <div className="rounded-lg border border-border bg-card p-4">
            <SectionHeading>Quality Metrics Overview</SectionHeading>
            <ResponsiveContainer width="100%" height={220}>
              <RadarChart data={radarData} margin={{ top: 0, right: 20, bottom: 0, left: 20 }}>
                <PolarGrid stroke="hsl(var(--border))" />
                <PolarAngleAxis
                  dataKey="metric"
                  tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "hsl(var(--card))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: "6px",
                    fontSize: "12px",
                    color: "hsl(var(--foreground))",
                  }}
                  formatter={(value: number) => [`${value}/100`, "Score"]}
                />
                <Radar
                  dataKey="score"
                  stroke="hsl(var(--accent))"
                  fill="hsl(var(--accent))"
                  fillOpacity={0.18}
                  strokeWidth={2}
                />
              </RadarChart>
            </ResponsiveContainer>
          </div>
        )}

        {/* Progress bars for 0-5 metrics */}
        {scoreMetrics.length > 0 && (
          <div className="rounded-lg border border-border bg-card p-4">
            <SectionHeading>Scores (0 – 5 scale)</SectionHeading>
            <div className="space-y-4">
              {scoreMetrics.map((m) => (
                <div key={m.name} className="space-y-1">
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-medium text-foreground">{m.name}</span>
                    <span className="tabular-nums text-muted-foreground">
                      {m.score.toFixed(1)} / {m.maxScore}
                    </span>
                  </div>
                  <Progress
                    value={(m.score / m.maxScore) * 100}
                    className="h-2"
                  />
                  <p className="text-xs text-muted-foreground">{m.description}</p>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Other metrics (F1, BLEU, etc.) */}
      {otherMetrics.length > 0 && (
        <div className="rounded-lg border border-border bg-card p-4">
          <SectionHeading>Additional Metrics</SectionHeading>
          <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3">
            {otherMetrics.map((m) => (
              <div
                key={m.name}
                className="rounded-md border border-border bg-secondary p-3"
              >
                <div className="text-xs text-muted-foreground">{m.name}</div>
                <div className="mt-1 text-xl font-semibold tabular-nums text-foreground">
                  {m.maxScore === 1 ? `${(m.score * 100).toFixed(1)}%` : m.score.toFixed(2)}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">{m.description}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── safety metrics ── */}
      <div className="rounded-lg border border-border bg-card p-4">
        <SectionHeading>Responsible AI — Safety Evaluation</SectionHeading>
        <div className="grid gap-3 sm:grid-cols-2">
          {data.safetyMetrics.map((sm) => {
            const cfg = SEVERITY_CONFIG[sm.severity]
            const Icon = cfg.icon
            return (
              <div
                key={sm.category}
                className="flex items-center justify-between rounded-md border border-border bg-secondary px-3 py-2"
              >
                <span className="text-sm text-foreground">{sm.category}</span>
                <span
                  className={`flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${cfg.color}`}
                >
                  <Icon className="h-3 w-3" />
                  {cfg.label}
                  {sm.defectRate > 0 && (
                    <span className="opacity-70">
                      · {(sm.defectRate * 100).toFixed(1)}%
                    </span>
                  )}
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {/* ── benchmarks ── */}
      {data.benchmarks.length > 0 && (
        <div className="rounded-lg border border-border bg-card p-4">
          <SectionHeading>Benchmark Results</SectionHeading>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left">
                  <th className="pb-2 font-medium text-muted-foreground">Benchmark</th>
                  <th className="pb-2 font-medium text-muted-foreground">Dataset</th>
                  <th className="pb-2 text-right font-medium text-muted-foreground">Score</th>
                  <th className="pb-2 text-right font-medium text-muted-foreground">Date</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {data.benchmarks.map((b) => (
                  <tr key={b.name} className="text-foreground">
                    <td className="py-2 font-medium">{b.name}</td>
                    <td className="py-2 text-muted-foreground">{b.dataset}</td>
                    <td className="py-2 text-right tabular-nums">
                      <span className="inline-block rounded-md bg-accent/15 px-2 py-0.5 text-accent font-semibold">
                        {b.score}
                      </span>
                    </td>
                    <td className="py-2 text-right text-muted-foreground">
                      {formatDate(b.date)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── model details ── */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* intended use */}
        {data.intendedUse && data.intendedUse.length > 0 && (
          <div className="rounded-lg border border-border bg-card p-4">
            <SectionHeading>Intended Use</SectionHeading>
            <ul className="space-y-1.5 text-sm text-muted-foreground">
              {data.intendedUse.map((u) => (
                <li key={u} className="flex items-start gap-2">
                  <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-400" />
                  {u}
                </li>
              ))}
            </ul>
          </div>
        )}

        {/* out of scope */}
        {data.outOfScope && data.outOfScope.length > 0 && (
          <div className="rounded-lg border border-border bg-card p-4">
            <SectionHeading>Out of Scope</SectionHeading>
            <ul className="space-y-1.5 text-sm text-muted-foreground">
              {data.outOfScope.map((u) => (
                <li key={u} className="flex items-start gap-2">
                  <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-orange-400" />
                  {u}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {/* ── technical details ── */}
      <div className="rounded-lg border border-border bg-card p-4">
        <SectionHeading>Technical Details</SectionHeading>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-4">
          {[
            { label: "Architecture", value: data.architecture },
            { label: "Training Cutoff", value: data.trainingDataCutoff },
            { label: "License", value: data.license },
            {
              label: "Evaluation ID",
              value: data.evaluationRunId
                ? `${data.evaluationRunId.slice(0, 12)}…`
                : undefined,
            },
          ]
            .filter((f) => f.value)
            .map((f) => (
              <div key={f.label} className="space-y-0.5">
                <dt className="text-xs text-muted-foreground">{f.label}</dt>
                <dd className="font-medium text-foreground">{f.value}</dd>
              </div>
            ))}
        </dl>
      </div>

      {/* ── limitations ── */}
      {data.limitations && data.limitations.length > 0 && (
        <div className="rounded-lg border border-amber-500/20 bg-amber-500/5 p-4">
          <SectionHeading>Known Limitations</SectionHeading>
          <ul className="space-y-1.5 text-sm text-muted-foreground">
            {data.limitations.map((l) => (
              <li key={l} className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-400" />
                {l}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* ── footer ── */}
      <div className="flex items-center gap-2 border-t border-border pt-4 text-xs text-muted-foreground">
        <ShieldCheck className="h-4 w-4 text-accent" />
        <span>
          Evaluation powered by{" "}
          <a
            href="https://ai.azure.com"
            target="_blank"
            rel="noopener noreferrer"
            className="text-accent hover:underline"
          >
            Azure AI Foundry
          </a>
          {data.evaluationRunId && ` · Run ID: ${data.evaluationRunId}`}
        </span>
      </div>
    </div>
  )
}
