"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { cn } from "@/lib/utils"
import {
  HardDrive,
  Folder,
  FolderOpen,
  FileText,
  File,
  ChevronRight,
  ChevronDown,
  Search,
  Upload,
  Download,
  RefreshCw,
  Plus,
  Trash2,
  MoreHorizontal,
  Filter,
  Grid3X3,
  List,
  CheckCircle2,
  Clock,
  Database,
  Eye,
  Copy,
  Archive,
  Image,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"

// ── Types ─────────────────────────────────────────────────────────────────────

interface StorageAccount {
  name: string
  tier: string
  region: string
  used: string
  containers: Container[]
}

interface Container {
  name: string
  access: "Private" | "Blob" | "Container"
  blobs: Blob[]
}

interface Blob {
  name: string
  type: "parquet" | "json" | "csv" | "pkl" | "txt" | "image" | "archive"
  size: string
  modified: string
  tags?: string[]
}

// ── Mock data ─────────────────────────────────────────────────────────────────

const accounts: StorageAccount[] = [
  {
    name: "aimktstrp7a65r22",
    tier: "Standard LRS",
    region: "East US",
    used: "1.2 TB",
    containers: [
      {
        name: "training-data",
        access: "Private",
        blobs: [
          { name: "claims-training-v3.parquet", type: "parquet", size: "4.2 GB", modified: "Mar 2, 2026", tags: ["healthcare", "claims"] },
          { name: "member-eligibility-2025.parquet", type: "parquet", size: "1.8 GB", modified: "Feb 28, 2026", tags: ["eligibility"] },
          { name: "denial-reasons-annotated.csv", type: "csv", size: "234 MB", modified: "Feb 20, 2026", tags: ["denials", "labeled"] },
          { name: "auth-requests-q4.json", type: "json", size: "512 MB", modified: "Jan 31, 2026" },
          { name: "icd10-codes-2026.csv", type: "csv", size: "18 MB", modified: "Jan 2, 2026" },
        ],
      },
      {
        name: "model-artifacts",
        access: "Private",
        blobs: [
          { name: "rcm-classifier-v2.pkl", type: "pkl", size: "2.1 GB", modified: "Mar 1, 2026", tags: ["model", "v2"] },
          { name: "tokenizer-config.json", type: "json", size: "24 KB", modified: "Mar 1, 2026" },
          { name: "fine-tune-checkpoints.archive", type: "archive", size: "8.4 GB", modified: "Feb 25, 2026" },
        ],
      },
      {
        name: "raw-exports",
        access: "Private",
        blobs: [
          { name: "ehr-export-20260228.parquet", type: "parquet", size: "14 GB", modified: "Feb 28, 2026" },
          { name: "lab-results-jan26.csv", type: "csv", size: "880 MB", modified: "Jan 31, 2026" },
        ],
      },
      {
        name: "evaluation-outputs",
        access: "Blob",
        blobs: [
          { name: "batch-eval-20260302.json", type: "json", size: "48 MB", modified: "Mar 2, 2026", tags: ["evaluation"] },
          { name: "precision-recall-curves.png", type: "image", size: "320 KB", modified: "Mar 1, 2026" },
        ],
      },
    ],
  },
  {
    name: "aimlstoragedev",
    tier: "Standard GRS",
    region: "East US 2",
    used: "1.2 TB",
    containers: [
      {
        name: "shared-notebooks",
        access: "Private",
        blobs: [
          { name: "data-exploration.ipynb", type: "txt", size: "1.2 MB", modified: "Mar 2, 2026" },
          { name: "feature-engineering.ipynb", type: "txt", size: "3.8 MB", modified: "Feb 27, 2026" },
        ],
      },
    ],
  },
]

// ── Helpers ───────────────────────────────────────────────────────────────────

const fileIcons: Record<string, { icon: React.ElementType; color: string }> = {
  parquet: { icon: Database, color: "text-emerald-400" },
  json: { icon: FileText, color: "text-amber-400" },
  csv: { icon: FileText, color: "text-blue-400" },
  pkl: { icon: Archive, color: "text-violet-400" },
  txt: { icon: FileText, color: "text-muted-foreground" },
  image: { icon: Image, color: "text-pink-400" },
  archive: { icon: Archive, color: "text-orange-400" },
}

function FileIcon({ type }: { type: Blob["type"] }) {
  const { icon: Icon, color } = fileIcons[type] ?? { icon: File, color: "text-muted-foreground" }
  return <Icon className={cn("h-4 w-4 shrink-0", color)} />
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function AzureStoragePage() {
  const [expandedAccounts, setExpandedAccounts] = useState<Set<string>>(new Set(["aimktstrp7a65r22"]))
  const [expandedContainers, setExpandedContainers] = useState<Set<string>>(new Set(["aimktstrp7a65r22/training-data"]))
  const [selectedContainer, setSelectedContainer] = useState<{ account: string; container: string } | null>({
    account: "aimktstrp7a65r22",
    container: "training-data",
  })
  const [search, setSearch] = useState("")
  const [viewMode, setViewMode] = useState<"list" | "grid">("list")

  const toggleAccount = (name: string) => {
    setExpandedAccounts((prev) => {
      const next = new Set(prev)
      next.has(name) ? next.delete(name) : next.add(name)
      return next
    })
  }

  const toggleContainer = (key: string) => {
    setExpandedContainers((prev) => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }

  const selectedBlobs = (() => {
    if (!selectedContainer) return []
    const acc = accounts.find((a) => a.name === selectedContainer.account)
    const cont = acc?.containers.find((c) => c.name === selectedContainer.container)
    const blobs = cont?.blobs ?? []
    if (!search) return blobs
    return blobs.filter((b) => b.name.toLowerCase().includes(search.toLowerCase()))
  })()

  const selectedContainerData = selectedContainer
    ? accounts.find((a) => a.name === selectedContainer.account)?.containers.find((c) => c.name === selectedContainer.container)
    : null

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <main className="ml-64 flex h-screen flex-col overflow-hidden p-6 pb-0">
        {/* Header */}
        <div className="mb-4 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <HardDrive className="h-5 w-5 text-blue-400" />
              <h1 className="text-2xl font-bold text-foreground">Azure Storage</h1>
              <Badge className="bg-emerald-500/15 text-emerald-400">Connected</Badge>
            </div>
            <p className="text-sm text-muted-foreground">Browse containers, blobs, and files across storage accounts.</p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" className="gap-2"><RefreshCw className="h-3.5 w-3.5" /> Refresh</Button>
            <Button size="sm" className="gap-2 bg-blue-600 hover:bg-blue-700 text-white"><Upload className="h-3.5 w-3.5" /> Upload</Button>
          </div>
        </div>

        {/* Main layout: tree + blob list */}
        <div className="flex flex-1 gap-4 overflow-hidden pb-6">
          {/* Left tree panel */}
          <div className="flex w-64 shrink-0 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2.5">
              <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Storage Accounts</p>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              {accounts.map((account) => {
                const isExpanded = expandedAccounts.has(account.name)
                return (
                  <div key={account.name}>
                    <button
                      onClick={() => toggleAccount(account.name)}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs font-medium text-foreground hover:bg-secondary/50 transition-colors"
                    >
                      {isExpanded ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                      <HardDrive className="h-3.5 w-3.5 shrink-0 text-blue-400" />
                      <span className="truncate">{account.name}</span>
                    </button>
                    {isExpanded && (
                      <div className="ml-4 mt-0.5 space-y-0.5">
                        {account.containers.map((container) => {
                          const key = `${account.name}/${container.name}`
                          const isSelected = selectedContainer?.account === account.name && selectedContainer?.container === container.name
                          return (
                            <button
                              key={key}
                              onClick={() => { setSelectedContainer({ account: account.name, container: container.name }) }}
                              className={cn(
                                "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors",
                                isSelected
                                  ? "bg-blue-500/15 text-blue-300 font-medium"
                                  : "text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                              )}
                            >
                              {isSelected ? <FolderOpen className="h-3.5 w-3.5 shrink-0 text-blue-400" /> : <Folder className="h-3.5 w-3.5 shrink-0 text-amber-400/70" />}
                              <span className="truncate">{container.name}</span>
                              <span className="ml-auto text-[10px] text-muted-foreground/50">{container.blobs.length}</span>
                            </button>
                          )
                        })}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </div>

          {/* Right blob list panel */}
          <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
            {/* Toolbar */}
            <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-2.5">
              <div className="flex items-center gap-2 min-w-0">
                {selectedContainer ? (
                  <>
                    <HardDrive className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span className="text-xs text-muted-foreground">{selectedContainer.account}</span>
                    <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/40" />
                    <FolderOpen className="h-3.5 w-3.5 shrink-0 text-blue-400" />
                    <span className="text-xs font-medium text-foreground">{selectedContainer.container}</span>
                    {selectedContainerData && (
                      <Badge className={cn("ml-1 text-[10px]",
                        selectedContainerData.access === "Private" ? "bg-secondary text-muted-foreground" : "bg-amber-500/15 text-amber-400"
                      )}>
                        {selectedContainerData.access}
                      </Badge>
                    )}
                  </>
                ) : (
                  <span className="text-xs text-muted-foreground">Select a container</span>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <div className="relative">
                  <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    className="h-7 w-48 pl-8 text-xs bg-secondary border-0"
                    placeholder="Filter files…"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  onClick={() => setViewMode(viewMode === "list" ? "grid" : "list")}
                >
                  {viewMode === "list" ? <Grid3X3 className="h-3.5 w-3.5" /> : <List className="h-3.5 w-3.5" />}
                </Button>
              </div>
            </div>

            {/* Blob list */}
            <div className="flex-1 overflow-y-auto">
              {selectedBlobs.length === 0 ? (
                <div className="flex h-full items-center justify-center">
                  <p className="text-sm text-muted-foreground">No files found.</p>
                </div>
              ) : viewMode === "list" ? (
                <table className="w-full text-xs">
                  <thead className="sticky top-0 border-b border-border bg-card/95">
                    <tr className="text-left text-muted-foreground">
                      <th className="px-4 py-2.5 font-medium">Name</th>
                      <th className="px-4 py-2.5 font-medium">Tags</th>
                      <th className="px-4 py-2.5 font-medium">Size</th>
                      <th className="px-4 py-2.5 font-medium">Modified</th>
                      <th className="px-4 py-2.5 font-medium w-20">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {selectedBlobs.map((blob) => (
                      <tr key={blob.name} className="hover:bg-secondary/30 transition-colors">
                        <td className="px-4 py-2.5">
                          <div className="flex items-center gap-2.5">
                            <FileIcon type={blob.type} />
                            <span className="font-medium text-foreground">{blob.name}</span>
                          </div>
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="flex flex-wrap gap-1">
                            {blob.tags?.map((tag) => (
                              <span key={tag} className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">{tag}</span>
                            ))}
                          </div>
                        </td>
                        <td className="px-4 py-2.5 text-muted-foreground">{blob.size}</td>
                        <td className="px-4 py-2.5 text-muted-foreground">
                          <div className="flex items-center gap-1">
                            <Clock className="h-3 w-3" />
                            {blob.modified}
                          </div>
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="flex items-center gap-1">
                            <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" title="Download">
                              <Download className="h-3.5 w-3.5" />
                            </Button>
                            <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" title="Copy URL">
                              <Copy className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div className="grid grid-cols-3 gap-3 p-4">
                  {selectedBlobs.map((blob) => (
                    <Card key={blob.name} className="cursor-pointer border-border bg-secondary/30 hover:border-blue-500/40 transition-all">
                      <CardContent className="p-3">
                        <div className="mb-2 flex items-center gap-2">
                          <FileIcon type={blob.type} />
                          <span className="text-xs font-medium text-foreground truncate">{blob.name}</span>
                        </div>
                        <p className="text-[10px] text-muted-foreground">{blob.size} · {blob.modified}</p>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </div>

            {/* Status bar */}
            {selectedBlobs.length > 0 && (
              <div className="border-t border-border px-4 py-2 text-[10px] text-muted-foreground">
                {selectedBlobs.length} items · {selectedContainerData?.access} access
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
