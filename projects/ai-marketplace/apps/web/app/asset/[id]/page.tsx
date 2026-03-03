"use client"

import { use } from "react"
import Link from "next/link"
import { notFound } from "next/navigation"
import { Header } from "@/components/marketplace/header"
import { ModelCard } from "@/components/marketplace/model-card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { assets } from "@/lib/mock-data"
import { 
  ArrowLeft, 
  BadgeCheck, 
  Star, 
  Download, 
  ExternalLink,
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
  Copy,
  Check
} from "lucide-react"
import { useState } from "react"

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
}

export default function AssetDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const asset = assets.find((a) => a.id === id)
  const [copied, setCopied] = useState(false)

  if (!asset) {
    notFound()
  }

  const Icon = iconMap[asset.icon] || Brain

  const handleCopy = () => {
    navigator.clipboard.writeText(`npx azure-ai add ${asset.id}`)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const relatedAssets = assets
    .filter((a) => a.category === asset.category && a.id !== asset.id)
    .slice(0, 3)

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <main className="mx-auto max-w-5xl px-6 py-8">
        <Link
          href="/"
          className="mb-6 inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Marketplace
        </Link>

        <div className="grid gap-8 lg:grid-cols-3">
          <div className="lg:col-span-2">
            <div className="mb-6 flex items-start gap-4">
              <div className="flex h-16 w-16 items-center justify-center rounded-xl bg-secondary">
                <Icon className="h-8 w-8 text-foreground" />
              </div>
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <h1 className="text-2xl font-semibold text-foreground">{asset.name}</h1>
                  <Badge variant="outline">{asset.pricing}</Badge>
                </div>
                <div className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
                  <span className="flex items-center gap-1">
                    {asset.publisher}
                    {asset.publisherVerified && (
                      <BadgeCheck className="h-4 w-4 text-accent" />
                    )}
                  </span>
                  <span>·</span>
                  <span>v{asset.version}</span>
                  <span>·</span>
                  <span className="flex items-center gap-1">
                    <Star className="h-3 w-3 fill-current text-yellow-500" />
                    {asset.rating}
                  </span>
                  <span>·</span>
                  <span className="flex items-center gap-1">
                    <Download className="h-3 w-3" />
                    {asset.downloads.toLocaleString()}
                  </span>
                </div>
              </div>
            </div>

            <Tabs defaultValue="overview" className="w-full">
              <TabsList className="w-full justify-start border-b border-border bg-transparent p-0">
                <TabsTrigger
                  value="overview"
                  className="rounded-none border-b-2 border-transparent px-4 pb-3 pt-2 data-[state=active]:border-primary data-[state=active]:bg-transparent"
                >
                  Overview
                </TabsTrigger>
                <TabsTrigger
                  value="documentation"
                  className="rounded-none border-b-2 border-transparent px-4 pb-3 pt-2 data-[state=active]:border-primary data-[state=active]:bg-transparent"
                >
                  Documentation
                </TabsTrigger>
                <TabsTrigger
                  value="changelog"
                  className="rounded-none border-b-2 border-transparent px-4 pb-3 pt-2 data-[state=active]:border-primary data-[state=active]:bg-transparent"
                >
                  Changelog
                </TabsTrigger>
                <TabsTrigger
                  value="reviews"
                  className="rounded-none border-b-2 border-transparent px-4 pb-3 pt-2 data-[state=active]:border-primary data-[state=active]:bg-transparent"
                >
                  Reviews
                </TabsTrigger>
                {asset.type === "model" && (
                  <TabsTrigger
                    value="model-card"
                    className="rounded-none border-b-2 border-transparent px-4 pb-3 pt-2 data-[state=active]:border-primary data-[state=active]:bg-transparent"
                  >
                    Model Card
                  </TabsTrigger>
                )}
              </TabsList>

              <TabsContent value="overview" className="mt-6">
                <div className="prose prose-invert max-w-none">
                  <p className="text-muted-foreground leading-relaxed">
                    {asset.description}
                  </p>

                  <h3 className="mt-6 text-lg font-medium text-foreground">Features</h3>
                  <ul className="mt-3 space-y-2 text-muted-foreground">
                    <li className="flex items-start gap-2">
                      <Check className="mt-1 h-4 w-4 text-green-500" />
                      Enterprise-grade security with Azure AD integration
                    </li>
                    <li className="flex items-start gap-2">
                      <Check className="mt-1 h-4 w-4 text-green-500" />
                      Automatic scaling and load balancing
                    </li>
                    <li className="flex items-start gap-2">
                      <Check className="mt-1 h-4 w-4 text-green-500" />
                      Comprehensive logging and monitoring
                    </li>
                    <li className="flex items-start gap-2">
                      <Check className="mt-1 h-4 w-4 text-green-500" />
                      Full API access with SDKs for major languages
                    </li>
                  </ul>

                  <h3 className="mt-6 text-lg font-medium text-foreground">Tags</h3>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {asset.tags.map((tag) => (
                      <Badge key={tag} variant="secondary" className="text-xs">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>
              </TabsContent>

              <TabsContent value="documentation" className="mt-6">
                <div className="rounded-lg border border-border bg-card p-6">
                  <h3 className="mb-4 text-lg font-medium text-foreground">Quick Start</h3>
                  <div className="rounded-md bg-secondary p-4 font-mono text-sm">
                    <code className="text-muted-foreground">
                      <span className="text-accent">import</span> {`{ ${asset.name.replace(/\s/g, '')} }`} <span className="text-accent">from</span> <span className="text-green-400">&apos;@azure-ai/{asset.id}&apos;</span>
                    </code>
                  </div>
                  <p className="mt-4 text-sm text-muted-foreground">
                    View full documentation on{" "}
                    <a href="#" className="text-accent hover:underline inline-flex items-center gap-1">
                      Azure AI Docs <ExternalLink className="h-3 w-3" />
                    </a>
                  </p>
                </div>
              </TabsContent>

              <TabsContent value="changelog" className="mt-6">
                <div className="space-y-4">
                  <div className="rounded-lg border border-border bg-card p-4">
                    <div className="flex items-center justify-between">
                      <span className="font-medium text-foreground">v{asset.version}</span>
                      <span className="text-sm text-muted-foreground">{asset.lastUpdated}</span>
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Latest release with performance improvements and bug fixes.
                    </p>
                  </div>
                </div>
              </TabsContent>

              <TabsContent value="reviews" className="mt-6">
                <div className="rounded-lg border border-border bg-card p-6 text-center">
                  <Star className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
                  <p className="text-muted-foreground">No reviews yet. Be the first to review!</p>
                </div>
              </TabsContent>

              {asset.type === "model" && (
                <TabsContent value="model-card" className="mt-6">
                  <ModelCard assetId={asset.id} />
                </TabsContent>
              )}
            </Tabs>
          </div>

          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-card p-4">
              <Link href={`/orchestration?add=${asset.id}`}>
                <Button className="w-full" size="lg">
                  Add to Workflow
                </Button>
              </Link>
              <Button variant="outline" className="mt-2 w-full">
                Install Standalone
              </Button>

              <div className="mt-4 rounded-md bg-secondary p-3">
                <p className="mb-2 text-xs text-muted-foreground">Install via CLI</p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 truncate font-mono text-xs text-foreground">
                    npx azure-ai add {asset.id}
                  </code>
                  <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleCopy}>
                    {copied ? (
                      <Check className="h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </Button>
                </div>
              </div>
            </div>

            <div className="rounded-lg border border-border bg-card p-4">
              <h3 className="mb-3 text-sm font-medium text-foreground">Details</h3>
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Type</dt>
                  <dd className="text-foreground capitalize">{asset.type.replace("-", " ")}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Category</dt>
                  <dd className="text-foreground">{asset.category}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="text-foreground">{asset.version}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Last Updated</dt>
                  <dd className="text-foreground">{asset.lastUpdated}</dd>
                </div>
              </dl>
            </div>

            {relatedAssets.length > 0 && (
              <div className="rounded-lg border border-border bg-card p-4">
                <h3 className="mb-3 text-sm font-medium text-foreground">Related Assets</h3>
                <div className="space-y-3">
                  {relatedAssets.map((related) => {
                    const RelatedIcon = iconMap[related.icon] || Brain
                    return (
                      <Link
                        key={related.id}
                        href={`/asset/${related.id}`}
                        className="flex items-center gap-3 rounded-md p-2 transition-colors hover:bg-secondary"
                      >
                        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-secondary">
                          <RelatedIcon className="h-4 w-4 text-foreground" />
                        </div>
                        <div className="flex-1 min-w-0">
                          <p className="truncate text-sm font-medium text-foreground">
                            {related.name}
                          </p>
                          <p className="text-xs text-muted-foreground">{related.publisher}</p>
                        </div>
                      </Link>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
