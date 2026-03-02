"use client"

import { useState } from "react"
import { AppSidebar } from "@/components/marketplace/app-sidebar"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Search,
  Shield,
  Clock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  FileText,
  Users,
  Activity,
  Eye,
  ThumbsUp,
  ThumbsDown,
  MoreHorizontal,
  Filter,
} from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface ApprovalRequest {
  id: string
  title: string
  type: "deployment" | "asset" | "policy-change"
  requester: string
  requestedAt: string
  status: "pending" | "approved" | "rejected"
  environment: string
  riskLevel: "low" | "medium" | "high"
}

interface AuditLog {
  id: string
  action: string
  actor: string
  target: string
  timestamp: string
  details: string
}

interface Policy {
  id: string
  name: string
  description: string
  status: "active" | "draft" | "disabled"
  scope: string
  lastModified: string
}

const approvalRequests: ApprovalRequest[] = [
  {
    id: "apr-1",
    title: "Deploy Customer Support Pipeline v2.1",
    type: "deployment",
    requester: "Sarah Chen",
    requestedAt: "2 hours ago",
    status: "pending",
    environment: "production",
    riskLevel: "medium",
  },
  {
    id: "apr-2",
    title: "Add GPT-5 Model to Approved Assets",
    type: "asset",
    requester: "Mike Johnson",
    requestedAt: "5 hours ago",
    status: "pending",
    environment: "all",
    riskLevel: "high",
  },
  {
    id: "apr-3",
    title: "Update Rate Limiting Policy",
    type: "policy-change",
    requester: "Alex Rivera",
    requestedAt: "1 day ago",
    status: "approved",
    environment: "production",
    riskLevel: "low",
  },
  {
    id: "apr-4",
    title: "Deploy Security Scanner v3.0",
    type: "deployment",
    requester: "Jordan Lee",
    requestedAt: "2 days ago",
    status: "rejected",
    environment: "staging",
    riskLevel: "medium",
  },
]

const auditLogs: AuditLog[] = [
  {
    id: "log-1",
    action: "Deployment Created",
    actor: "Sarah Chen",
    target: "Customer Support Pipeline",
    timestamp: "2026-02-27 14:32:15",
    details: "Deployed to production environment",
  },
  {
    id: "log-2",
    action: "Asset Approved",
    actor: "Admin",
    target: "GPT-4 Turbo Model",
    timestamp: "2026-02-27 13:15:42",
    details: "Added to approved assets list",
  },
  {
    id: "log-3",
    action: "Policy Updated",
    actor: "Security Team",
    target: "Data Encryption Policy",
    timestamp: "2026-02-27 11:08:33",
    details: "Enabled AES-256 encryption requirement",
  },
  {
    id: "log-4",
    action: "Access Revoked",
    actor: "Admin",
    target: "External API Key #847",
    timestamp: "2026-02-26 16:45:00",
    details: "Key expired and access revoked",
  },
  {
    id: "log-5",
    action: "Workflow Modified",
    actor: "Mike Johnson",
    target: "Code Review Bot",
    timestamp: "2026-02-26 14:22:18",
    details: "Added new condition node",
  },
]

const policies: Policy[] = [
  {
    id: "pol-1",
    name: "Production Deployment Approval",
    description: "Require manager approval for all production deployments",
    status: "active",
    scope: "Production Environment",
    lastModified: "2026-02-20",
  },
  {
    id: "pol-2",
    name: "Data Retention Policy",
    description: "Retain audit logs for 90 days, delete after",
    status: "active",
    scope: "All Environments",
    lastModified: "2026-02-15",
  },
  {
    id: "pol-3",
    name: "API Rate Limiting",
    description: "Limit to 1000 requests per minute per user",
    status: "active",
    scope: "Production Environment",
    lastModified: "2026-02-10",
  },
  {
    id: "pol-4",
    name: "Model Allowlist",
    description: "Only allow pre-approved AI models in workflows",
    status: "active",
    scope: "All Environments",
    lastModified: "2026-02-05",
  },
  {
    id: "pol-5",
    name: "Geo-Restriction Policy",
    description: "Restrict access to US and EU regions only",
    status: "draft",
    scope: "Production Environment",
    lastModified: "2026-02-01",
  },
]

const riskColors = {
  low: "bg-green-500/10 text-green-500 border-green-500/20",
  medium: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20",
  high: "bg-red-500/10 text-red-500 border-red-500/20",
}

const statusColors = {
  pending: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20",
  approved: "bg-green-500/10 text-green-500 border-green-500/20",
  rejected: "bg-red-500/10 text-red-500 border-red-500/20",
  active: "bg-green-500/10 text-green-500 border-green-500/20",
  draft: "bg-blue-500/10 text-blue-500 border-blue-500/20",
  disabled: "bg-muted text-muted-foreground border-border",
}

export default function GovernancePage() {
  const [searchQuery, setSearchQuery] = useState("")
  const [activeTab, setActiveTab] = useState("approvals")

  const pendingCount = approvalRequests.filter((r) => r.status === "pending").length

  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />

      <main className="ml-64 mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold text-foreground">Governance</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage approvals, policies, and audit logs
          </p>
        </div>

        {/* Overview Cards */}
        <div className="mb-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Pending Approvals
              </CardTitle>
              <Clock className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-foreground">{pendingCount}</div>
              <p className="text-xs text-muted-foreground">
                {pendingCount > 0 ? "Requires attention" : "All caught up"}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Active Policies
              </CardTitle>
              <Shield className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-foreground">
                {policies.filter((p) => p.status === "active").length}
              </div>
              <p className="text-xs text-muted-foreground">Across all environments</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Audit Events (24h)
              </CardTitle>
              <Activity className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-foreground">127</div>
              <p className="text-xs text-muted-foreground">+12% from yesterday</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                Compliance Score
              </CardTitle>
              <CheckCircle2 className="h-4 w-4 text-green-500" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-green-500">98%</div>
              <p className="text-xs text-muted-foreground">All requirements met</p>
            </CardContent>
          </Card>
        </div>

        {/* Tabs */}
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <div className="mb-6 flex items-center justify-between">
            <TabsList>
              <TabsTrigger value="approvals" className="gap-2">
                <Clock className="h-4 w-4" />
                Approvals
                {pendingCount > 0 && (
                  <Badge variant="secondary" className="ml-1 h-5 px-1.5">
                    {pendingCount}
                  </Badge>
                )}
              </TabsTrigger>
              <TabsTrigger value="policies" className="gap-2">
                <Shield className="h-4 w-4" />
                Policies
              </TabsTrigger>
              <TabsTrigger value="audit" className="gap-2">
                <FileText className="h-4 w-4" />
                Audit Log
              </TabsTrigger>
            </TabsList>

            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-64 pl-10 bg-secondary"
                />
              </div>
              <Button variant="outline" size="icon">
                <Filter className="h-4 w-4" />
              </Button>
            </div>
          </div>

          <TabsContent value="approvals" className="mt-0">
            <div className="rounded-lg border border-border">
              {approvalRequests.map((request, index) => (
                <div
                  key={request.id}
                  className={`flex items-center justify-between p-4 ${
                    index !== approvalRequests.length - 1 ? "border-b border-border" : ""
                  }`}
                >
                  <div className="flex items-center gap-4">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-secondary">
                      {request.type === "deployment" && <Activity className="h-5 w-5 text-accent" />}
                      {request.type === "asset" && <Shield className="h-5 w-5 text-green-500" />}
                      {request.type === "policy-change" && <FileText className="h-5 w-5 text-yellow-500" />}
                    </div>
                    <div>
                      <p className="font-medium text-foreground">{request.title}</p>
                      <div className="flex items-center gap-2 text-sm text-muted-foreground">
                        <Users className="h-3 w-3" />
                        <span>{request.requester}</span>
                        <span>·</span>
                        <span>{request.requestedAt}</span>
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    <Badge variant="outline" className={riskColors[request.riskLevel]}>
                      {request.riskLevel} risk
                    </Badge>
                    <Badge variant="outline" className={statusColors[request.status]}>
                      {request.status}
                    </Badge>

                    {request.status === "pending" && (
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-green-500 hover:bg-green-500/10">
                          <ThumbsUp className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-red-500 hover:bg-red-500/10">
                          <ThumbsDown className="h-4 w-4" />
                        </Button>
                      </div>
                    )}

                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <Eye className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </TabsContent>

          <TabsContent value="policies" className="mt-0">
            <div className="rounded-lg border border-border">
              {policies.map((policy, index) => (
                <div
                  key={policy.id}
                  className={`flex items-center justify-between p-4 ${
                    index !== policies.length - 1 ? "border-b border-border" : ""
                  }`}
                >
                  <div className="flex items-center gap-4">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-secondary">
                      <Shield className="h-5 w-5 text-foreground" />
                    </div>
                    <div>
                      <p className="font-medium text-foreground">{policy.name}</p>
                      <p className="text-sm text-muted-foreground">{policy.description}</p>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    <span className="text-sm text-muted-foreground">{policy.scope}</span>
                    <Badge variant="outline" className={statusColors[policy.status]}>
                      {policy.status}
                    </Badge>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem>Edit Policy</DropdownMenuItem>
                        <DropdownMenuItem>View History</DropdownMenuItem>
                        <DropdownMenuItem>Duplicate</DropdownMenuItem>
                        {policy.status === "active" ? (
                          <DropdownMenuItem className="text-destructive">Disable</DropdownMenuItem>
                        ) : (
                          <DropdownMenuItem>Enable</DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-4 flex justify-end">
              <Button className="gap-2">
                <Shield className="h-4 w-4" />
                Create Policy
              </Button>
            </div>
          </TabsContent>

          <TabsContent value="audit" className="mt-0">
            <div className="rounded-lg border border-border">
              <div className="grid grid-cols-12 gap-4 border-b border-border bg-muted/30 px-4 py-3 text-sm font-medium text-muted-foreground">
                <div className="col-span-3">Action</div>
                <div className="col-span-2">Actor</div>
                <div className="col-span-3">Target</div>
                <div className="col-span-2">Timestamp</div>
                <div className="col-span-2">Details</div>
              </div>

              {auditLogs.map((log) => (
                <div
                  key={log.id}
                  className="grid grid-cols-12 gap-4 items-center border-b border-border px-4 py-3 last:border-0 text-sm"
                >
                  <div className="col-span-3">
                    <span className="font-medium text-foreground">{log.action}</span>
                  </div>
                  <div className="col-span-2 text-muted-foreground">{log.actor}</div>
                  <div className="col-span-3 text-muted-foreground">{log.target}</div>
                  <div className="col-span-2 text-muted-foreground font-mono text-xs">
                    {log.timestamp}
                  </div>
                  <div className="col-span-2 text-muted-foreground truncate">{log.details}</div>
                </div>
              ))}
            </div>

            <div className="mt-4 flex items-center justify-between">
              <p className="text-sm text-muted-foreground">Showing 5 of 127 events</p>
              <Button variant="outline" size="sm">
                Load More
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </main>
    </div>
  )
}
