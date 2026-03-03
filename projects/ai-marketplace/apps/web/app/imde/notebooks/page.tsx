"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  BookOpen,
  Plus,
  Search,
  Users,
  Clock,
  GitBranch,
  Copy,
  ExternalLink,
  Share2,
  Star,
  GitCommit,
  CheckCircle2,
  FileCode,
  Filter,
  Download,
  Lock,
  Globe,
  Bookmark,
} from "lucide-react"
import { cn } from "@/lib/utils"

interface Notebook {
  id: string
  name: string
  description: string
  author: string
  team: string
  tags: string[]
  lastModified: string
  version: string
  status: "active" | "review" | "archived"
  visibility: "private" | "team" | "org"
  stars: number
  forks: number
  commits: number
  sandbox: string
  runTime?: string
}

const notebooks: Notebook[] = [
  {
    id: "nb-001",
    name: "Denial_Prediction_EDA.ipynb",
    description: "Exploratory analysis of 18-month denial dataset — feature engineering for transformer fine-tuning",
    author: "Dr. Sarah Chen",
    team: "RCM AI Core",
    tags: ["denials", "EDA", "feature-eng", "transformer"],
    lastModified: "2 hours ago",
    version: "v2.4",
    status: "active",
    visibility: "team",
    stars: 8,
    forks: 3,
    commits: 47,
    sandbox: "RCM-Denial-Prediction-v3",
    runTime: "12m 34s",
  },
  {
    id: "nb-002",
    name: "ICD10_LLM_FineTune.ipynb",
    description: "Fine-tuning Llama 3.1 on ICD-10 coding task with LoRA adapters and PEFT",
    author: "James Rivera",
    team: "RCM AI Core",
    tags: ["LLM", "LoRA", "ICD-10", "fine-tuning"],
    lastModified: "1 day ago",
    version: "v1.8",
    status: "review",
    visibility: "team",
    stars: 14,
    forks: 6,
    commits: 89,
    sandbox: "ICD10-AutoCode-LLM",
    runTime: "2h 15m",
  },
  {
    id: "nb-003",
    name: "Payer_Rules_Analysis.ipynb",
    description: "Correlation analysis between payer coverage rules and prior auth approval rates by CPT code",
    author: "Amy Kowalski",
    team: "Billing Analytics",
    tags: ["auth", "payer", "CPT", "correlation"],
    lastModified: "3 days ago",
    version: "v3.1",
    status: "active",
    visibility: "org",
    stars: 22,
    forks: 9,
    commits: 134,
    sandbox: "Auth-Approval-Predictor",
    runTime: "5m 02s",
  },
  {
    id: "nb-004",
    name: "Model_Comparison_Benchmark.ipynb",
    description: "Side-by-side benchmark: XGBoost vs LightGBM vs Transformer on denial classification task",
    author: "Mike Johnson",
    team: "RCM AI Core",
    tags: ["benchmark", "comparison", "XGBoost", "LightGBM"],
    lastModified: "5 days ago",
    version: "v1.2",
    status: "active",
    visibility: "org",
    stars: 31,
    forks: 12,
    commits: 28,
    sandbox: "RCM-Denial-Prediction-v3",
    runTime: "45m 18s",
  },
  {
    id: "nb-005",
    name: "SHAP_Explainability.ipynb",
    description: "SHAP value analysis for prior auth model — explaining decisions to billing domain experts",
    author: "Priya Patel",
    team: "Billing Analytics",
    tags: ["explainability", "SHAP", "XAI", "compliance"],
    lastModified: "1 week ago",
    version: "v2.0",
    status: "review",
    visibility: "team",
    stars: 19,
    forks: 4,
    commits: 61,
    sandbox: "Auth-Approval-Predictor",
  },
]

const templates = [
  {
    id: "tpl-001",
    name: "LLM Fine-Tuning (LoRA/PEFT)",
    description: "End-to-end fine-tuning pipeline for healthcare domain using parameter-efficient techniques",
    tags: ["LLM", "LoRA", "HuggingFace"],
    icon: "🤖",
  },
  {
    id: "tpl-002",
    name: "Multi-Class Classification",
    description: "Standard template for denial/auth classification tasks with evaluation harness",
    tags: ["classification", "sklearn", "XGBoost"],
    icon: "📊",
  },
  {
    id: "tpl-003",
    name: "Clinical NLP Pipeline",
    description: "NER + relation extraction from clinical notes using BioBERT/ClinicalBERT",
    tags: ["NLP", "BioBERT", "NER"],
    icon: "🏥",
  },
  {
    id: "tpl-004",
    name: "Time-Series Forecasting",
    description: "Payer trend forecasting using Prophet + LSTM hybrid approach",
    tags: ["time-series", "Prophet", "LSTM"],
    icon: "📈",
  },
  {
    id: "tpl-005",
    name: "Model Explainability",
    description: "SHAP + LIME explanations with domain-expert-friendly output formatting",
    tags: ["SHAP", "XAI", "compliance"],
    icon: "🔍",
  },
  {
    id: "tpl-006",
    name: "Multimodal (Image + Text)",
    description: "Process EOB images + claim text together using Florence-2 + LLM fusion",
    tags: ["multimodal", "Florence-2", "OCR"],
    icon: "🖼️",
  },
]

function StatusBadge({ status }: { status: Notebook["status"] }) {
  const map: Record<Notebook["status"], string> = {
    active: "bg-emerald-500/20 text-emerald-400",
    review: "bg-amber-500/20 text-amber-400",
    archived: "bg-secondary text-muted-foreground",
  }
  return <Badge className={cn("text-xs", map[status])}>{status}</Badge>
}

function VisibilityIcon({ v }: { v: Notebook["visibility"] }) {
  if (v === "private") return <Lock className="h-3.5 w-3.5 text-muted-foreground" />
  if (v === "team") return <Users className="h-3.5 w-3.5 text-muted-foreground" />
  return <Globe className="h-3.5 w-3.5 text-muted-foreground" />
}

export default function IMDENotebooksPage() {
  const [search, setSearch] = useState("")
  const filtered = notebooks.filter(
    (n) =>
      n.name.toLowerCase().includes(search.toLowerCase()) ||
      n.description.toLowerCase().includes(search.toLowerCase()) ||
      n.tags.some((t) => t.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div className="flex min-h-screen bg-background">
      <AppSidebar />
      <div className="ml-64 flex-1 p-6">
        {/* Header */}
        <div className="mb-6 flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-violet-500/20">
              <BookOpen className="h-5 w-5 text-violet-400" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-foreground">Notebooks</h1>
              <p className="text-sm text-muted-foreground">Shared Jupyter notebooks with version control & collaboration</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="gap-2">
              <Download className="h-4 w-4" />
              Import
            </Button>
            <Button size="sm" className="gap-2 bg-violet-600 hover:bg-violet-700 text-white">
              <Plus className="h-4 w-4" />
              New Notebook
            </Button>
          </div>
        </div>

        <Tabs defaultValue="my-notebooks">
          <div className="mb-4 flex items-center justify-between">
            <TabsList>
              <TabsTrigger value="my-notebooks">My Notebooks</TabsTrigger>
              <TabsTrigger value="shared">Team Shared</TabsTrigger>
              <TabsTrigger value="templates">Templates</TabsTrigger>
            </TabsList>
            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search notebooks..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="h-9 pl-9 w-60"
                />
              </div>
              <Button variant="outline" size="sm" className="gap-2">
                <Filter className="h-4 w-4" />
                Filter
              </Button>
            </div>
          </div>

          <TabsContent value="my-notebooks" className="mt-0">
            <div className="space-y-3">
              {filtered.map((nb) => (
                <Card key={nb.id} className="border-border hover:border-violet-500/40 transition-all">
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between gap-4">
                      <div className="flex items-start gap-3 flex-1 min-w-0">
                        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-violet-500/10">
                          <FileCode className="h-4 w-4 text-violet-400" />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap mb-1">
                            <span className="font-medium text-sm text-foreground font-mono">{nb.name}</span>
                            <StatusBadge status={nb.status} />
                            <VisibilityIcon v={nb.visibility} />
                            <span className="text-xs text-muted-foreground">{nb.version}</span>
                          </div>
                          <p className="text-xs text-muted-foreground mb-2">{nb.description}</p>
                          <div className="flex flex-wrap gap-1 mb-2">
                            {nb.tags.map((t) => (
                              <span key={t} className="rounded-full bg-secondary px-2 py-0.5 text-[10px] text-muted-foreground">#{t}</span>
                            ))}
                          </div>
                          <div className="flex items-center gap-4 text-xs text-muted-foreground">
                            <span className="flex items-center gap-1"><Clock className="h-3.5 w-3.5" />{nb.lastModified}</span>
                            <span className="flex items-center gap-1"><GitCommit className="h-3.5 w-3.5" />{nb.commits} commits</span>
                            <span className="flex items-center gap-1"><Star className="h-3.5 w-3.5" />{nb.stars}</span>
                            <span className="flex items-center gap-1"><Copy className="h-3.5 w-3.5" />{nb.forks} forks</span>
                            {nb.runTime && <span className="flex items-center gap-1 text-amber-400/80">⏱ {nb.runTime}</span>}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-1 shrink-0">
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-muted-foreground hover:text-foreground">
                          <Share2 className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-muted-foreground hover:text-foreground">
                          <Copy className="h-4 w-4" />
                        </Button>
                        <Button variant="outline" size="sm" className="gap-1.5 text-xs">
                          <ExternalLink className="h-3.5 w-3.5" />
                          Open
                        </Button>
                      </div>
                    </div>
                    {/* Sandbox Tag */}
                    <div className="mt-3 pt-3 border-t border-border flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="text-muted-foreground/60">Sandbox:</span>
                      <span className="rounded bg-violet-500/10 px-2 py-0.5 text-violet-400 text-[10px] font-mono">{nb.sandbox}</span>
                      <span className="ml-auto text-muted-foreground/60">by {nb.author} · {nb.team}</span>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </TabsContent>

          <TabsContent value="shared" className="mt-0">
            <div className="space-y-3">
              {notebooks.filter(n => n.visibility === "org" || n.visibility === "team").map((nb) => (
                <Card key={nb.id} className="border-border hover:border-violet-500/40 transition-all">
                  <CardContent className="p-4">
                    <div className="flex items-center justify-between gap-4">
                      <div className="flex items-center gap-3">
                        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-500/10">
                          <BookOpen className="h-4 w-4 text-blue-400" />
                        </div>
                        <div>
                          <div className="flex items-center gap-2 mb-0.5">
                            <span className="font-medium text-sm font-mono">{nb.name}</span>
                            <VisibilityIcon v={nb.visibility} />
                          </div>
                          <p className="text-xs text-muted-foreground">{nb.team} · {nb.author}</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button variant="ghost" size="sm" className="gap-1.5 text-xs text-muted-foreground">
                          <Bookmark className="h-3.5 w-3.5" /> Save
                        </Button>
                        <Button variant="ghost" size="sm" className="gap-1.5 text-xs text-muted-foreground">
                          <Copy className="h-3.5 w-3.5" /> Fork
                        </Button>
                        <Button variant="outline" size="sm" className="gap-1.5 text-xs">
                          <ExternalLink className="h-3.5 w-3.5" /> Open
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </TabsContent>

          <TabsContent value="templates" className="mt-0">
            <div className="mb-4 rounded-lg border border-violet-500/30 bg-violet-500/5 p-4">
              <p className="text-sm text-violet-300">
                <strong>Pre-built templates</strong> for common RCM AI/ML tasks — fork any template to start a new notebook in your sandbox instantly.
              </p>
            </div>
            <div className="grid grid-cols-3 gap-4">
              {templates.map((tpl) => (
                <Card key={tpl.id} className="border-border hover:border-violet-500/40 transition-all cursor-pointer group">
                  <CardContent className="p-4 space-y-3">
                    <div className="text-3xl">{tpl.icon}</div>
                    <div>
                      <h3 className="text-sm font-semibold mb-1">{tpl.name}</h3>
                      <p className="text-xs text-muted-foreground">{tpl.description}</p>
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {tpl.tags.map((t) => (
                        <span key={t} className="rounded-full bg-secondary px-2 py-0.5 text-[10px] text-muted-foreground">
                          {t}
                        </span>
                      ))}
                    </div>
                    <Button variant="outline" size="sm" className="w-full gap-2 group-hover:border-violet-500/50">
                      <Copy className="h-3.5 w-3.5" />
                      Use Template
                    </Button>
                  </CardContent>
                </Card>
              ))}
            </div>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
