"use client"

import { useState } from "react"
import Link from "next/link"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { 
  Search,
  Plus,
  MoreHorizontal,
  Activity,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ExternalLink,
  Settings
} from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface Deployment {
  id: string
  name: string
  workflow: string
  environment: "production" | "staging" | "development"
  status: "running" | "stopped" | "failed" | "deploying"
  region: string
  instances: number
  lastDeployed: string
  requests24h: number
}

const deployments: Deployment[] = [
  {
    id: "dep-1",
    name: "Customer Support Pipeline",
    workflow: "Customer Support Pipeline",
    environment: "production",
    status: "running",
    region: "East US",
    instances: 3,
    lastDeployed: "2 hours ago",
    requests24h: 15420,
  },
  {
    id: "dep-2",
    name: "Code Review Bot",
    workflow: "Code Review Workflow",
    environment: "production",
    status: "running",
    region: "West US 2",
    instances: 2,
    lastDeployed: "1 day ago",
    requests24h: 8930,
  },
  {
    id: "dep-3",
    name: "Data Processing Pipeline",
    workflow: "Data ETL Workflow",
    environment: "staging",
    status: "deploying",
    region: "Central US",
    instances: 1,
    lastDeployed: "5 minutes ago",
    requests24h: 0,
  },
  {
    id: "dep-4",
    name: "Security Scanner",
    workflow: "Security Incident Response",
    environment: "production",
    status: "failed",
    region: "East US 2",
    instances: 0,
    lastDeployed: "3 days ago",
    requests24h: 0,
  },
]

const statusConfig = {
  running: { icon: CheckCircle2, color: "text-green-500", bg: "bg-green-500/10" },
  stopped: { icon: XCircle, color: "text-muted-foreground", bg: "bg-muted" },
  failed: { icon: AlertCircle, color: "text-destructive", bg: "bg-destructive/10" },
  deploying: { icon: Activity, color: "text-accent", bg: "bg-accent/10" },
}

const envColors = {
  production: "bg-green-500/10 text-green-500 border-green-500/20",
  staging: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20",
  development: "bg-blue-500/10 text-blue-500 border-blue-500/20",
}

export default function DeploymentsPage() {
  const [searchQuery, setSearchQuery] = useState("")

  const filteredDeployments = deployments.filter((dep) =>
    dep.name.toLowerCase().includes(searchQuery.toLowerCase())
  )

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />

      <main className="ml-64 mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6 flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-foreground">Deployments</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Manage and monitor your deployed workflows
            </p>
          </div>
          <Link href="/deployments/new">
            <Button className="gap-2">
              <Plus className="h-4 w-4" />
              New Deployment
            </Button>
          </Link>
        </div>

        <div className="mb-6 flex items-center gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search deployments..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10 bg-secondary"
            />
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="gap-1">
              <span className="h-2 w-2 rounded-full bg-green-500" />
              {deployments.filter((d) => d.status === "running").length} Running
            </Badge>
            <Badge variant="outline" className="gap-1">
              <span className="h-2 w-2 rounded-full bg-destructive" />
              {deployments.filter((d) => d.status === "failed").length} Failed
            </Badge>
          </div>
        </div>

        <div className="rounded-lg border border-border">
          <div className="grid grid-cols-12 gap-4 border-b border-border bg-muted/30 px-4 py-3 text-sm font-medium text-muted-foreground">
            <div className="col-span-4">Name</div>
            <div className="col-span-2">Environment</div>
            <div className="col-span-2">Status</div>
            <div className="col-span-2">Region</div>
            <div className="col-span-2 text-right">Actions</div>
          </div>

          {filteredDeployments.map((deployment) => {
            const StatusIcon = statusConfig[deployment.status].icon
            return (
              <div
                key={deployment.id}
                className="grid grid-cols-12 gap-4 items-center border-b border-border px-4 py-4 last:border-0 hover:bg-muted/20 transition-colors"
              >
                <div className="col-span-4">
                  <Link
                    href={`/deployments/${deployment.id}`}
                    className="font-medium text-foreground hover:text-accent"
                  >
                    {deployment.name}
                  </Link>
                  <p className="text-sm text-muted-foreground">{deployment.workflow}</p>
                </div>
                <div className="col-span-2">
                  <Badge variant="outline" className={envColors[deployment.environment]}>
                    {deployment.environment}
                  </Badge>
                </div>
                <div className="col-span-2">
                  <div className="flex items-center gap-2">
                    <StatusIcon className={`h-4 w-4 ${statusConfig[deployment.status].color}`} />
                    <span className="text-sm capitalize text-foreground">{deployment.status}</span>
                  </div>
                </div>
                <div className="col-span-2">
                  <span className="text-sm text-muted-foreground">{deployment.region}</span>
                </div>
                <div className="col-span-2 flex justify-end gap-2">
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <ExternalLink className="h-4 w-4" />
                  </Button>
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <Settings className="h-4 w-4" />
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="h-8 w-8">
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem>View Logs</DropdownMenuItem>
                      <DropdownMenuItem>Edit Configuration</DropdownMenuItem>
                      <DropdownMenuItem>Rollback</DropdownMenuItem>
                      <DropdownMenuItem className="text-destructive">Stop</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            )
          })}
        </div>

        {filteredDeployments.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12">
            <Activity className="mb-3 h-12 w-12 text-muted-foreground" />
            <p className="text-lg text-foreground">No deployments found</p>
            <p className="text-sm text-muted-foreground">
              Create your first deployment to get started
            </p>
          </div>
        )}
      </main>
    </div>
  )
}
