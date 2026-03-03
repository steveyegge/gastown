"use client"

import { useState, useMemo } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  Sparkles,
  Search,
  Download,
  Heart,
  Tag,
  Filter,
  Star,
  BookOpen,
  Play,
  ExternalLink,
  ChevronDown,
  CheckCircle2,
  Database,
  FileText,
  Image,
  Layers,
  BarChart3,
  Brain,
  Stethoscope,
  DollarSign,
  FlaskConical,
  Globe,
  SortAsc,
  TrendingUp,
  Clock,
  Zap,
  Plus,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"

// ── Types ─────────────────────────────────────────────────────────────────────

type Modality = "text" | "tabular" | "image" | "multimodal" | "code"
type License = "Apache 2.0" | "MIT" | "CC BY 4.0" | "CC BY-NC 4.0" | "Custom" | "Public Domain"
type Category = "healthcare" | "nlp" | "tabular" | "vision" | "multimodal" | "finance" | "synthetic" | "benchmark"

interface Dataset {
  id: string
  name: string
  org: string
  description: string
  category: Category
  modality: Modality
  task: string
  tags: string[]
  size: string
  sizeBytes: number
  downloads: number
  likes: number
  license: License
  featured?: boolean
  updated: string
  rows?: number
}

// ── Mock dataset catalog ────────────────────────────────────────────────────

const datasets: Dataset[] = [
  {
    id: "hc-claims-rcm-v3",
    name: "Claims & Denials (RCM v3)",
    org: "Optum RCM",
    description: "4.1M de-identified claims with denial codes, remark codes, and appeal outcomes. Ideal for denial prediction and prior auth models.",
    category: "healthcare",
    modality: "tabular",
    task: "classification",
    tags: ["claims", "denial", "RCM", "HIPAA-safe", "healthcare"],
    size: "4.2 GB",
    sizeBytes: 4_200_000_000,
    downloads: 8420,
    likes: 312,
    license: "CC BY-NC 4.0",
    featured: true,
    updated: "Mar 1, 2026",
    rows: 4_100_000,
  },
  {
    id: "hc-eligibility-payer",
    name: "Payer Eligibility Checks",
    org: "Optum RCM",
    description: "2.8M eligibility verification records across 40+ payers with real-time vs batch outcomes. Use for eligibility agent training.",
    category: "healthcare",
    modality: "tabular",
    task: "classification",
    tags: ["eligibility", "payer", "RCM"],
    size: "1.8 GB",
    sizeBytes: 1_800_000_000,
    downloads: 6100,
    likes: 240,
    license: "CC BY-NC 4.0",
    featured: true,
    updated: "Feb 28, 2026",
    rows: 2_800_000,
  },
  {
    id: "hc-nlp-clinical-notes",
    name: "Healthcare NLP Corpus v2",
    org: "Optum AI",
    description: "450K de-identified clinical notes with ICD-10 annotations, NER labels, and SOAP structure. Benchmark for clinical NLP models.",
    category: "nlp",
    modality: "text",
    task: "named-entity-recognition",
    tags: ["clinical", "NLP", "ICD-10", "NER", "SNOMED"],
    size: "2.1 GB",
    sizeBytes: 2_100_000_000,
    downloads: 11200,
    likes: 504,
    license: "CC BY-NC 4.0",
    featured: true,
    updated: "Feb 20, 2026",
    rows: 450_000,
  },
  {
    id: "hc-prior-auth-requests",
    name: "Prior Auth Request Corpus",
    org: "Optum RCM",
    description: "512K prior authorization requests with approval/denial labels, procedure codes, and clinical justification notes.",
    category: "healthcare",
    modality: "multimodal",
    task: "classification",
    tags: ["prior-auth", "approval", "procedure-codes"],
    size: "890 MB",
    sizeBytes: 890_000_000,
    downloads: 3800,
    likes: 198,
    license: "CC BY-NC 4.0",
    updated: "Jan 31, 2026",
    rows: 512_000,
  },
  {
    id: "nlp-medical-summarization",
    name: "Medical Discharge Summary",
    org: "Healthcare NLP",
    description: "88K hospital discharge summaries paired with human-written abstracts. Use for summarization fine-tuning or ROUGE evaluation.",
    category: "nlp",
    modality: "text",
    task: "summarization",
    tags: ["summarization", "clinical", "discharge", "abstractive"],
    size: "1.1 GB",
    sizeBytes: 1_100_000_000,
    downloads: 14200,
    likes: 711,
    license: "Apache 2.0",
    featured: true,
    updated: "Jan 15, 2026",
    rows: 88_000,
  },
  {
    id: "tabular-icd10-codes",
    name: "ICD-10-CM / PCS Codes 2026",
    org: "CMS",
    description: "Complete 2026 ICD-10-CM diagnosis and PCS procedure code tables with descriptions, hierarchy, and billability flags.",
    category: "tabular",
    modality: "tabular",
    task: "classification",
    tags: ["ICD-10", "diagnosis", "procedure", "reference"],
    size: "42 MB",
    sizeBytes: 42_000_000,
    downloads: 22000,
    likes: 820,
    license: "Public Domain",
    updated: "Jan 1, 2026",
    rows: 80_000,
  },
  {
    id: "tabular-provider-npi",
    name: "CMS NPI Provider Registry",
    org: "CMS",
    description: "Full NPI registry snapshot with provider type, taxonomy, specialties, address, and active status. Updated monthly.",
    category: "tabular",
    modality: "tabular",
    task: "retrieval",
    tags: ["NPI", "provider", "registry", "CMS"],
    size: "380 MB",
    sizeBytes: 380_000_000,
    downloads: 18000,
    likes: 590,
    license: "Public Domain",
    updated: "Mar 1, 2026",
    rows: 7_800_000,
  },
  {
    id: "synth-claims-benchmark",
    name: "Synthetic Claims Benchmark",
    org: "Optum AI",
    description: "500K fully synthetic claims generated with differential privacy guarantees. Safe for model evaluation and CI pipelines.",
    category: "synthetic",
    modality: "tabular",
    task: "benchmark",
    tags: ["synthetic", "benchmark", "privacy", "differential-privacy"],
    size: "620 MB",
    sizeBytes: 620_000_000,
    downloads: 4200,
    likes: 210,
    license: "Apache 2.0",
    updated: "Feb 10, 2026",
    rows: 500_000,
  },
  {
    id: "nlp-denial-reason-classification",
    name: "Denial Reason Classifier",
    org: "Optum RCM",
    description: "48K labeled denial notes mapped to CARC/RARC reason codes for multi-class classification fine-tuning.",
    category: "nlp",
    modality: "text",
    task: "classification",
    tags: ["denial", "CARC", "RARC", "classification", "NLP"],
    size: "158 MB",
    sizeBytes: 158_000_000,
    downloads: 5600,
    likes: 290,
    license: "CC BY-NC 4.0",
    updated: "Jan 28, 2026",
    rows: 48_000,
  },
  {
    id: "vision-radiology-cxr",
    name: "Chest X-Ray (CXR-14)",
    org: "NIH Open Health",
    description: "112K frontal-view chest X-rays with 14 disease labels (pneumonia, nodules, etc.). Standard benchmark for medical imaging AI.",
    category: "vision",
    modality: "image",
    task: "classification",
    tags: ["radiology", "X-Ray", "imaging", "NIH", "benchmark"],
    size: "42 GB",
    sizeBytes: 42_000_000_000,
    downloads: 38000,
    likes: 1840,
    license: "CC BY 4.0",
    updated: "Dec 12, 2025",
    rows: 112_000,
  },
  {
    id: "fin-healthcare-spend",
    name: "Healthcare Spend Analytics",
    org: "HCUP",
    description: "Aggregated payer-provider spend data from HCUP NIS (2019–2023) for cost trend analysis and financial risk modeling.",
    category: "finance",
    modality: "tabular",
    task: "regression",
    tags: ["spend", "cost", "HCUP", "payer"],
    size: "2.8 GB",
    sizeBytes: 2_800_000_000,
    downloads: 7200,
    likes: 360,
    license: "CC BY 4.0",
    updated: "Nov 30, 2025",
    rows: 11_000_000,
  },
  {
    id: "bench-rcm-eval",
    name: "RCM Agent Eval Suite",
    org: "Optum AI",
    description: "2,400 expert-annotated Q&A pairs for evaluating healthcare RCM agents across eligibility, coding, and denial categories.",
    category: "benchmark",
    modality: "text",
    task: "question-answering",
    tags: ["benchmark", "eval", "agent", "RCM", "QA"],
    size: "18 MB",
    sizeBytes: 18_000_000,
    downloads: 3100,
    likes: 184,
    license: "MIT",
    featured: true,
    updated: "Feb 25, 2026",
    rows: 2_400,
  },
  {
    id: "nlp-cpt-descriptions",
    name: "CPT Procedure Code Descriptions",
    org: "AMA (mirrored)",
    description: "Current Procedural Terminology code descriptions and clinical context for embedding and code lookup tasks.",
    category: "tabular",
    modality: "text",
    task: "retrieval",
    tags: ["CPT", "procedure", "coding", "AMA"],
    size: "64 MB",
    sizeBytes: 64_000_000,
    downloads: 12000,
    likes: 430,
    license: "CC BY 4.0",
    updated: "Jan 2, 2026",
    rows: 10_000,
  },
  {
    id: "synth-member-demographics",
    name: "Synthetic Member Demographics",
    org: "Optum AI",
    description: "1M synthetic member profiles with demographics, risk scores, and chronic condition flags. Privacy-safe for integration testing.",
    category: "synthetic",
    modality: "tabular",
    task: "benchmark",
    tags: ["synthetic", "members", "demographics", "risk"],
    size: "480 MB",
    sizeBytes: 480_000_000,
    downloads: 2800,
    likes: 122,
    license: "Apache 2.0",
    updated: "Feb 5, 2026",
    rows: 1_000_000,
  },
]

// ── Config ─────────────────────────────────────────────────────────────────────

const categories: { id: Category | "all"; label: string; icon: React.ElementType; color: string }[] = [
  { id: "all",        label: "All",         icon: Sparkles,    color: "text-sky-400" },
  { id: "healthcare", label: "Healthcare",  icon: Stethoscope, color: "text-rose-400" },
  { id: "nlp",        label: "NLP / Text",  icon: FileText,    color: "text-blue-400" },
  { id: "tabular",    label: "Tabular",     icon: BarChart3,   color: "text-emerald-400" },
  { id: "vision",     label: "Vision",      icon: Image,       color: "text-pink-400" },
  { id: "multimodal", label: "Multimodal",  icon: Layers,      color: "text-violet-400" },
  { id: "finance",    label: "Finance",     icon: DollarSign,  color: "text-amber-400" },
  { id: "synthetic",  label: "Synthetic",   icon: FlaskConical, color: "text-orange-400" },
  { id: "benchmark",  label: "Benchmarks",  icon: Star,        color: "text-yellow-400" },
]

const sortOptions = [
  { id: "downloads", label: "Most Downloads" },
  { id: "likes",     label: "Most Liked" },
  { id: "updated",   label: "Recently Updated" },
  { id: "size-asc",  label: "Smallest First" },
  { id: "size-desc", label: "Largest First" },
]

const modalityConfig: Record<Modality, { color: string; label: string }> = {
  text:       { color: "bg-blue-500/15 text-blue-400",    label: "Text" },
  tabular:    { color: "bg-emerald-500/15 text-emerald-400", label: "Tabular" },
  image:      { color: "bg-pink-500/15 text-pink-400",    label: "Image" },
  multimodal: { color: "bg-violet-500/15 text-violet-400", label: "Multimodal" },
  code:       { color: "bg-amber-500/15 text-amber-400",  label: "Code" },
}

function fmtDownloads(n: number) {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}k`
  return String(n)
}

// ── Dataset Card ──────────────────────────────────────────────────────────────

function DatasetCard({ ds, onUse }: { ds: Dataset; onUse: (id: string) => void }) {
  const mod = modalityConfig[ds.modality]
  return (
    <Card className="group flex flex-col border-border bg-card hover:border-sky-500/30 transition-all hover:shadow-md hover:shadow-sky-500/5">
      <CardContent className="flex flex-1 flex-col p-4">
        {/* Top */}
        <div className="mb-2 flex items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              {ds.featured && <Star className="h-3.5 w-3.5 shrink-0 text-amber-400 fill-amber-400" />}
              <span className="text-sm font-semibold text-foreground group-hover:text-sky-300 transition-colors line-clamp-1">{ds.name}</span>
            </div>
            <span className="text-xs text-muted-foreground">{ds.org}</span>
          </div>
          <span className={cn("shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium", mod.color)}>{mod.label}</span>
        </div>

        {/* Description */}
        <p className="mb-3 flex-1 text-xs text-muted-foreground leading-relaxed line-clamp-3">{ds.description}</p>

        {/* Tags */}
        <div className="mb-3 flex flex-wrap gap-1">
          {ds.tags.slice(0, 4).map((tag) => (
            <span key={tag} className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">{tag}</span>
          ))}
          {ds.tags.length > 4 && (
            <span className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">+{ds.tags.length - 4}</span>
          )}
        </div>

        {/* Stats row */}
        <div className="mb-3 flex items-center gap-3 text-[10px] text-muted-foreground">
          <span className="flex items-center gap-1"><Database className="h-3 w-3" /> {ds.size}</span>
          {ds.rows && <span className="flex items-center gap-1"><BarChart3 className="h-3 w-3" /> {(ds.rows / 1000).toFixed(0)}K rows</span>}
          <span className="flex items-center gap-1"><Download className="h-3 w-3" /> {fmtDownloads(ds.downloads)}</span>
          <span className="flex items-center gap-1"><Heart className="h-3 w-3" /> {ds.likes}</span>
          <span className="ml-auto flex items-center gap-1"><Tag className="h-3 w-3" /> {ds.license}</span>
        </div>

        {/* Actions */}
        <div className="flex gap-2">
          <Button
            size="sm"
            className="flex-1 h-7 gap-1.5 bg-sky-600 hover:bg-sky-700 text-white text-xs"
            onClick={() => onUse(ds.id)}
          >
            <Zap className="h-3 w-3" /> Use Dataset
          </Button>
          <Button variant="outline" size="sm" className="h-7 w-7 p-0">
            <ExternalLink className="h-3.5 w-3.5" />
          </Button>
          <Button variant="outline" size="sm" className="h-7 w-7 p-0">
            <Heart className="h-3.5 w-3.5" />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

// ── Featured Banner ────────────────────────────────────────────────────────────

function FeaturedBanner({ ds, onUse }: { ds: Dataset; onUse: (id: string) => void }) {
  const mod = modalityConfig[ds.modality]
  return (
    <div className="group col-span-1 flex flex-col rounded-xl border border-sky-500/20 bg-gradient-to-br from-sky-500/5 to-background p-4 hover:border-sky-500/40 transition-all">
      <div className="mb-1 flex items-center gap-1.5">
        <Star className="h-3.5 w-3.5 text-amber-400 fill-amber-400" />
        <span className="text-[10px] uppercase tracking-wider text-amber-400 font-medium">Featured</span>
      </div>
      <p className="mb-1 text-sm font-bold text-foreground">{ds.name}</p>
      <p className="mb-2 text-xs text-sky-400">{ds.org}</p>
      <p className="mb-3 flex-1 text-xs text-muted-foreground leading-relaxed line-clamp-2">{ds.description}</p>
      <div className="flex items-center gap-2">
        <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", mod.color)}>{mod.label}</span>
        <span className="text-[10px] text-muted-foreground">{ds.size}</span>
        <span className="text-[10px] text-muted-foreground">{fmtDownloads(ds.downloads)} downloads</span>
        <Button size="sm" className="ml-auto h-6 gap-1 bg-sky-600 hover:bg-sky-700 text-white text-[11px] px-2" onClick={() => onUse(ds.id)}>
          <Zap className="h-3 w-3" /> Use
        </Button>
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function DatasetsPage() {
  const [search, setSearch] = useState("")
  const [selectedCategory, setSelectedCategory] = useState<Category | "all">("all")
  const [sort, setSort] = useState("downloads")
  const [modalityFilter, setModalityFilter] = useState<Modality | "all">("all")
  const [usedDataset, setUsedDataset] = useState<string | null>(null)

  const featured = datasets.filter((d) => d.featured)

  const filtered = useMemo(() => {
    let d = datasets
    if (selectedCategory !== "all") d = d.filter((x) => x.category === selectedCategory)
    if (modalityFilter !== "all") d = d.filter((x) => x.modality === modalityFilter)
    if (search) d = d.filter((x) =>
      x.name.toLowerCase().includes(search.toLowerCase()) ||
      x.description.toLowerCase().includes(search.toLowerCase()) ||
      x.tags.some((t) => t.toLowerCase().includes(search.toLowerCase()))
    )
    if (sort === "downloads") d = [...d].sort((a, b) => b.downloads - a.downloads)
    else if (sort === "likes") d = [...d].sort((a, b) => b.likes - a.likes)
    else if (sort === "size-asc") d = [...d].sort((a, b) => a.sizeBytes - b.sizeBytes)
    else if (sort === "size-desc") d = [...d].sort((a, b) => b.sizeBytes - a.sizeBytes)
    return d
  }, [search, selectedCategory, sort, modalityFilter])

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 p-6">
        {/* Hero */}
        <div className="mb-8 rounded-2xl border border-sky-500/20 bg-gradient-to-br from-sky-500/10 via-background to-violet-500/5 p-8">
          <div className="mb-2 flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-sky-400" />
            <span className="text-sm font-semibold text-sky-400 uppercase tracking-wider">Dataset Hub</span>
          </div>
          <h1 className="mb-2 text-3xl font-bold text-foreground">
            Built-in Datasets
          </h1>
          <p className="mb-6 max-w-xl text-sm text-muted-foreground">
            Curated, governed healthcare datasets ready for model training, evaluation, and agent orchestration. 
            One click to load into your IMDE notebook or use in an agent workflow.
          </p>
          {/* Search */}
          <div className="relative max-w-2xl">
            <Search className="absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
            <input
              className="h-12 w-full rounded-xl border border-border bg-card pl-12 pr-4 text-sm text-foreground placeholder:text-muted-foreground focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20"
              placeholder="Search datasets, tags, tasks, or organizations…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          {/* Stats */}
          <div className="mt-4 flex gap-6 text-sm text-muted-foreground">
            <span><strong className="text-foreground">{datasets.length}</strong> datasets</span>
            <span><strong className="text-foreground">8</strong> categories</span>
            <span><strong className="text-foreground">150K+</strong> total downloads</span>
            <span><strong className="text-foreground">HIPAA-safe</strong> de-identification guaranteed</span>
          </div>
        </div>

        {/* Featured */}
        {!search && selectedCategory === "all" && (
          <div className="mb-8">
            <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">Featured</h2>
            <div className="grid grid-cols-5 gap-3">
              {featured.map((ds) => (
                <FeaturedBanner key={ds.id} ds={ds} onUse={(id) => setUsedDataset(id)} />
              ))}
            </div>
          </div>
        )}

        <div className="flex gap-6">
          {/* Left: filters */}
          <div className="flex w-52 shrink-0 flex-col gap-4">
            {/* Category */}
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Category</p>
              <div className="space-y-0.5">
                {categories.map(({ id, label, icon: Icon, color }) => {
                  const count = id === "all" ? datasets.length : datasets.filter((d) => d.category === id).length
                  return (
                    <button
                      key={id}
                      onClick={() => setSelectedCategory(id)}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-md px-3 py-1.5 text-xs transition-colors",
                        selectedCategory === id
                          ? "bg-sky-500/15 text-sky-300 font-medium"
                          : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                      )}
                    >
                      <Icon className={cn("h-3.5 w-3.5 shrink-0", selectedCategory === id ? "text-sky-400" : color)} />
                      <span className="flex-1 text-left">{label}</span>
                      <span className="text-[10px] text-muted-foreground/50">{count}</span>
                    </button>
                  )
                })}
              </div>
            </div>

            {/* Modality */}
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Modality</p>
              <div className="space-y-0.5">
                {(["all", "text", "tabular", "image", "multimodal"] as const).map((m) => (
                  <button
                    key={m}
                    onClick={() => setModalityFilter(m)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-xs transition-colors capitalize",
                      modalityFilter === m
                        ? "bg-sky-500/15 text-sky-300 font-medium"
                        : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                    )}
                  >
                    {m === "all" ? "All" : modalityConfig[m].label}
                    {m !== "all" && (
                      <span className={cn("ml-auto rounded px-1 py-0.5 text-[9px] font-medium", modalityConfig[m].color)}>
                        {datasets.filter((d) => d.modality === m).length}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            </div>

            {/* License */}
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">License</p>
              {(["Apache 2.0", "MIT", "CC BY 4.0", "CC BY-NC 4.0", "Public Domain"] as const).map((l) => (
                <div key={l} className="flex items-center gap-2 px-3 py-1.5">
                  <input type="checkbox" id={l} className="h-3 w-3 rounded" />
                  <label htmlFor={l} className="text-xs text-muted-foreground">{l}</label>
                </div>
              ))}
            </div>
          </div>

          {/* Right: dataset grid */}
          <div className="flex-1">
            {/* Sort bar */}
            <div className="mb-4 flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                <span className="font-semibold text-foreground">{filtered.length}</span> datasets
                {selectedCategory !== "all" && <> in <span className="text-sky-400">{categories.find((c) => c.id === selectedCategory)?.label}</span></>}
              </p>
              <div className="flex items-center gap-2">
                <SortAsc className="h-4 w-4 text-muted-foreground" />
                <select
                  className="rounded-md border border-border bg-secondary px-3 py-1.5 text-xs text-foreground focus:outline-none"
                  value={sort}
                  onChange={(e) => setSort(e.target.value)}
                >
                  {sortOptions.map((o) => (
                    <option key={o.id} value={o.id}>{o.label}</option>
                  ))}
                </select>
              </div>
            </div>

            {/* Dataset cards grid */}
            <div className="grid grid-cols-3 gap-4">
              {filtered.map((ds) => (
                <DatasetCard key={ds.id} ds={ds} onUse={(id) => setUsedDataset(id)} />
              ))}
            </div>

            {filtered.length === 0 && (
              <div className="flex min-h-[300px] items-center justify-center rounded-xl border border-dashed border-border">
                <div className="text-center">
                  <Sparkles className="mx-auto mb-3 h-8 w-8 text-muted-foreground/40" />
                  <p className="text-sm font-medium text-muted-foreground">No datasets match your filters</p>
                  <p className="text-xs text-muted-foreground/60">Try adjusting category, modality, or search terms</p>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Use dataset toast / modal simulation */}
        {usedDataset && (
          <div className="fixed bottom-6 right-6 flex items-center gap-3 rounded-xl border border-emerald-500/30 bg-card p-4 shadow-xl shadow-emerald-500/10">
            <CheckCircle2 className="h-5 w-5 text-emerald-400" />
            <div>
              <p className="text-sm font-semibold text-foreground">Dataset added to IMDE notebook</p>
              <p className="text-xs text-muted-foreground">
                {datasets.find((d) => d.id === usedDataset)?.name} · Load with <code className="text-sky-400">dataset.load("{usedDataset}")</code>
              </p>
            </div>
            <Button variant="ghost" size="sm" className="gap-1.5 text-xs" onClick={() => setUsedDataset(null)}>Open Notebook <ExternalLink className="h-3.5 w-3.5" /></Button>
            <button onClick={() => setUsedDataset(null)} className="text-muted-foreground hover:text-foreground">✕</button>
          </div>
        )}
      </main>
    </div>
  )
}
