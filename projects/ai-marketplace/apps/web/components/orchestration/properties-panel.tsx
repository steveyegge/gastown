"use client"

import { WorkflowNode, WorkflowNodeConfig } from "@/lib/types"
import { assets } from "@/lib/mock-data"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { X, Trash2, Settings2, Plus, Check } from "lucide-react"
import { NODE_META } from "./workflow-node"
import { cn } from "@/lib/utils"

/* ── Asset lists ─────────────────────────────────────────────────────────── */
const agentOptions = assets.filter((a) => a.type === "agent")
const toolOptions = assets.filter((a) => a.type === "mcp-tool" || a.type === "mcp-server")
const modelOptions = [
  { id: "gpt-4o", label: "GPT-4o" },
  { id: "gpt-4-turbo", label: "GPT-4 Turbo" },
  { id: "o3", label: "OpenAI o3" },
  { id: "azure-openai-gpt4o", label: "Azure OpenAI GPT-4o" },
  { id: "claude-3-7-sonnet", label: "Claude 3.7 Sonnet" },
  { id: "claude-3-5-haiku", label: "Claude 3.5 Haiku" },
  { id: "phi-4", label: "Phi-4 (Azure AI)" },
  { id: "llama-3.3-70b", label: "Llama 3.3 70B" },
]

/* ── helpers ──────────────────────────────────────────────────────────────── */

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{children}</p>
  )
}

function Field({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs font-medium">{label}</Label>
      {children}
      {hint && <p className="text-[10px] text-muted-foreground">{hint}</p>}
    </div>
  )
}

/* ── MultiSelect for tools ────────────────────────────────────────────────── */

function ToolMultiSelect({
  selectedIds,
  onChange,
}: {
  selectedIds: string[]
  onChange: (ids: string[]) => void
}) {
  const toggle = (id: string) => {
    if (selectedIds.includes(id)) onChange(selectedIds.filter((x) => x !== id))
    else onChange([...selectedIds, id])
  }

  return (
    <div className="space-y-1.5">
      <div className="max-h-[160px] overflow-y-auto rounded-md border border-border bg-secondary/20 p-1.5 space-y-1">
        {toolOptions.map((tool) => {
          const selected = selectedIds.includes(tool.id)
          return (
            <button
              key={tool.id}
              type="button"
              className={cn(
                "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors",
                selected ? "bg-accent/20 text-accent" : "hover:bg-secondary text-foreground",
              )}
              onClick={() => toggle(tool.id)}
            >
              <div className={cn("flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border", selected ? "border-accent bg-accent" : "border-muted-foreground/40")}>
                {selected && <Check className="h-3 w-3 text-accent-foreground" />}
              </div>
              <span className="truncate">{tool.name}</span>
              <Badge variant="outline" className="ml-auto shrink-0 text-[9px] py-0">{tool.type === "mcp-server" ? "Server" : "Tool"}</Badge>
            </button>
          )
        })}
      </div>
      {selectedIds.length > 0 && (
        <p className="text-[10px] text-muted-foreground">{selectedIds.length} tool{selectedIds.length > 1 ? "s" : ""} selected</p>
      )}
    </div>
  )
}

/* ── PropertiesPanel ──────────────────────────────────────────────────────── */

interface PropertiesPanelProps {
  node: WorkflowNode | null
  onClose: () => void
  onUpdateNode: (id: string, data: Partial<WorkflowNode["data"]>) => void
  onUpdateConfig: (id: string, config: Partial<WorkflowNodeConfig>) => void
  onDeleteNode: (id: string) => void
}

export function PropertiesPanel({
  node,
  onClose,
  onUpdateNode,
  onUpdateConfig,
  onDeleteNode,
}: PropertiesPanelProps) {
  if (!node) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-6 text-center">
        <Settings2 className="mb-3 h-8 w-8 text-muted-foreground" />
        <p className="text-sm font-medium text-foreground">No node selected</p>
        <p className="mt-1 text-xs text-muted-foreground">
          Click a node to configure it
        </p>
      </div>
    )
  }

  const meta = NODE_META[node.type] ?? NODE_META.agent
  const Icon = meta.icon
  const cfg = node.data.config ?? {}

  const setLabel = (label: string) => onUpdateNode(node.id, { label })
  const setDesc = (description: string) => onUpdateNode(node.id, { description })
  const setConfig = (patch: Partial<WorkflowNodeConfig>) => onUpdateConfig(node.id, patch)

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className={cn("flex items-center justify-between border-b border-border px-4 py-3", meta.headerBg)}>
        <div className="flex items-center gap-2">
          <div className={cn("flex h-7 w-7 items-center justify-center rounded-md", meta.headerBg)}>
            <Icon className={cn("h-4 w-4", meta.iconColor)} />
          </div>
          <div>
            <p className="text-sm font-semibold text-foreground">{meta.label}</p>
            <p className="text-xs text-muted-foreground">Properties</p>
          </div>
        </div>
        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* Common fields */}
        <div className="space-y-4 p-4 border-b border-border">
          <SectionLabel>General</SectionLabel>
          <Field label="Label">
            <Input
              value={node.data.label}
              onChange={(e) => setLabel(e.target.value)}
              className="h-8 text-sm"
            />
          </Field>
          <Field label="Description">
            <Textarea
              value={node.data.description || ""}
              onChange={(e) => setDesc(e.target.value)}
              rows={2}
              className="text-sm resize-none"
            />
          </Field>
        </div>

        {/* ── Start / Trigger ─────────────────────────────────────────────── */}
        {(node.type === "start" || node.type === "trigger") && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Trigger</SectionLabel>
            <Field label="Trigger Type">
              <Select
                value={(cfg.triggerType as string) || "manual"}
                onValueChange={(v) => setConfig({ triggerType: v as WorkflowNodeConfig["triggerType"] })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="manual">Manual</SelectItem>
                  <SelectItem value="http">HTTP Request</SelectItem>
                  <SelectItem value="schedule">Scheduled (Cron)</SelectItem>
                  <SelectItem value="event">Event / Message Queue</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Input Schema (JSON)" hint="Optional — defines the expected workflow input shape">
              <Textarea
                value={(cfg.inputSchema as string) || ""}
                onChange={(e) => setConfig({ inputSchema: e.target.value })}
                placeholder='{ "claimId": "string", "priority": "string" }'
                rows={3}
                className="font-mono text-xs resize-none"
              />
            </Field>
          </div>
        )}

        {/* ── Agent ───────────────────────────────────────────────────────── */}
        {node.type === "agent" && (
          <>
            <div className="space-y-4 p-4 border-b border-border">
              <SectionLabel>Agent Identity</SectionLabel>
              <Field label="Select Agent from Marketplace" hint="Binds a pre-built agent asset to this node">
                <Select
                  value={(cfg.agentId as string) || ""}
                  onValueChange={(v) => setConfig({ agentId: v === "none" ? undefined : v })}
                >
                  <SelectTrigger className="h-8 text-sm"><SelectValue placeholder="Choose an agent…" /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">— None (custom) —</SelectItem>
                    {agentOptions.map((a) => (
                      <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Language Model">
                <Select
                  value={(cfg.modelId as string) || "gpt-4o"}
                  onValueChange={(v) => setConfig({ modelId: v })}
                >
                  <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {modelOptions.map((m) => (
                      <SelectItem key={m.id} value={m.id}>{m.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <div className="space-y-4 p-4 border-b border-border">
              <SectionLabel>System Prompt</SectionLabel>
              <Textarea
                value={(cfg.systemPrompt as string) || ""}
                onChange={(e) => setConfig({ systemPrompt: e.target.value })}
                placeholder="You are an expert medical coding agent. Given a clinical note, extract and validate ICD-10 and CPT codes…"
                rows={5}
                className="text-xs resize-none"
              />
            </div>
            <div className="space-y-4 p-4 border-b border-border">
              <SectionLabel>Tools (MCP)</SectionLabel>
              <p className="text-[11px] text-muted-foreground">Tools this agent can invoke during execution</p>
              <ToolMultiSelect
                selectedIds={(cfg.selectedToolIds as string[]) || []}
                onChange={(ids) => setConfig({ selectedToolIds: ids })}
              />
            </div>
            <div className="space-y-4 p-4 border-b border-border">
              <SectionLabel>LLM Parameters</SectionLabel>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Temperature" hint="0 = deterministic">
                  <Input
                    type="number"
                    min={0}
                    max={2}
                    step={0.1}
                    value={(cfg.temperature as number) ?? 0.7}
                    onChange={(e) => setConfig({ temperature: parseFloat(e.target.value) })}
                    className="h-8 text-sm"
                  />
                </Field>
                <Field label="Max Tokens">
                  <Input
                    type="number"
                    min={256}
                    max={32000}
                    step={256}
                    value={(cfg.maxTokens as number) ?? 4096}
                    onChange={(e) => setConfig({ maxTokens: parseInt(e.target.value) })}
                    className="h-8 text-sm"
                  />
                </Field>
              </div>
            </div>
          </>
        )}

        {/* ── Tool ────────────────────────────────────────────────────────── */}
        {node.type === "tool" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>MCP Tool / Server</SectionLabel>
            <Field label="Select Tool from Marketplace">
              <Select
                value={(cfg.toolId as string) || ""}
                onValueChange={(v) => setConfig({ toolId: v === "none" ? undefined : v })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue placeholder="Choose a tool…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">— None (custom) —</SelectItem>
                  {toolOptions.map((t) => (
                    <SelectItem key={t.id} value={t.id}>{t.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Parameters (key=value, one per line)" hint="Runtime parameters passed to the tool">
              <Textarea
                value={Object.entries((cfg.params as Record<string, string>) || {})
                  .map(([k, v]) => `${k}=${v}`)
                  .join("\n")}
                onChange={(e) => {
                  const params: Record<string, string> = {}
                  e.target.value.split("\n").forEach((line) => {
                    const eq = line.indexOf("=")
                    if (eq > 0) params[line.slice(0, eq).trim()] = line.slice(eq + 1).trim()
                  })
                  setConfig({ params })
                }}
                placeholder={"format=json\ntimeout=30\npageSize=10"}
                rows={4}
                className="font-mono text-xs resize-none"
              />
            </Field>
          </div>
        )}

        {/* ── Condition ───────────────────────────────────────────────────── */}
        {node.type === "condition" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>If / Else Logic</SectionLabel>
            <Field label="Condition Expression" hint="JavaScript-style expression evaluated at runtime">
              <Textarea
                value={(cfg.conditionExpression as string) || ""}
                onChange={(e) => setConfig({ conditionExpression: e.target.value })}
                placeholder="output.claimStatus === 'clean' && output.confidence > 0.9"
                rows={3}
                className="font-mono text-xs resize-none"
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="True Branch Label">
                <Input
                  value={(cfg.trueBranchLabel as string) || "True"}
                  onChange={(e) => setConfig({ trueBranchLabel: e.target.value })}
                  className="h-8 text-sm"
                />
              </Field>
              <Field label="False Branch Label">
                <Input
                  value={(cfg.falseBranchLabel as string) || "False"}
                  onChange={(e) => setConfig({ falseBranchLabel: e.target.value })}
                  className="h-8 text-sm"
                />
              </Field>
            </div>
          </div>
        )}

        {/* ── Fan-Out ─────────────────────────────────────────────────────── */}
        {node.type === "fan-out" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Fan-Out (Parallel Dispatch)</SectionLabel>
            <p className="text-[11px] text-muted-foreground">
              Dispatches the workflow to multiple branches simultaneously — inspired by the Microsoft Semantic Kernel concurrent agent pattern.
            </p>
            <Field label="Number of Branches">
              <Input
                type="number"
                min={2}
                max={10}
                value={(cfg.branches as number) ?? 2}
                onChange={(e) => setConfig({ branches: parseInt(e.target.value) })}
                className="h-8 text-sm"
              />
            </Field>
            <Field label="Dispatch Strategy">
              <Select
                value={(cfg.fanOutStrategy as string) || "parallel"}
                onValueChange={(v) => setConfig({ fanOutStrategy: v as WorkflowNodeConfig["fanOutStrategy"] })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="parallel">Parallel (all at once)</SelectItem>
                  <SelectItem value="round-robin">Round-Robin (sequential distribution)</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
        )}

        {/* ── Fan-In ──────────────────────────────────────────────────────── */}
        {node.type === "fan-in" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Fan-In (Result Aggregation)</SectionLabel>
            <p className="text-[11px] text-muted-foreground">
              Collects results from parallel branches and merges them before continuing.
            </p>
            <Field label="Join Strategy">
              <Select
                value={(cfg.joinStrategy as string) || "all"}
                onValueChange={(v) => setConfig({ joinStrategy: v as WorkflowNodeConfig["joinStrategy"] })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All — wait for every branch</SelectItem>
                  <SelectItem value="any">Any — proceed on first completion</SelectItem>
                  <SelectItem value="first">First-N — configurable quorum</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Timeout (seconds)" hint="Max total wait time for parallel branches">
              <Input
                type="number"
                min={5}
                max={3600}
                value={(cfg.joinTimeout as number) ?? 120}
                onChange={(e) => setConfig({ joinTimeout: parseInt(e.target.value) })}
                className="h-8 text-sm"
              />
            </Field>
          </div>
        )}

        {/* ── Loop ────────────────────────────────────────────────────────── */}
        {node.type === "loop" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Loop</SectionLabel>
            <Field label="Continue Condition" hint="Loop repeats while this expression is true">
              <Textarea
                value={(cfg.loopCondition as string) || ""}
                onChange={(e) => setConfig({ loopCondition: e.target.value })}
                placeholder="output.hasMore === true && iteration < maxPages"
                rows={2}
                className="font-mono text-xs resize-none"
              />
            </Field>
            <Field label="Max Iterations" hint="Prevents infinite loops">
              <Input
                type="number"
                min={1}
                max={1000}
                value={(cfg.maxIterations as number) ?? 10}
                onChange={(e) => setConfig({ maxIterations: parseInt(e.target.value) })}
                className="h-8 text-sm"
              />
            </Field>
            <Field label="Loop Variable" hint="Variable name accumulating loop results">
              <Input
                value={(cfg.loopVariable as string) || "items"}
                onChange={(e) => setConfig({ loopVariable: e.target.value })}
                className="h-8 font-mono text-sm"
              />
            </Field>
          </div>
        )}

        {/* ── Approval ────────────────────────────────────────────────────── */}
        {node.type === "approval" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Approval Gate</SectionLabel>
            <Field label="Approval Message">
              <Textarea
                value={(cfg.approvalMessage as string) || ""}
                onChange={(e) => setConfig({ approvalMessage: e.target.value })}
                placeholder="Please review the generated claim before submission to the clearinghouse."
                rows={3}
                className="text-xs resize-none"
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Approver Role">
                <Input
                  value={(cfg.approverRole as string) || ""}
                  onChange={(e) => setConfig({ approverRole: e.target.value })}
                  placeholder="billing-admin"
                  className="h-8 text-sm"
                />
              </Field>
              <Field label="Timeout (min)">
                <Input
                  type="number"
                  min={1}
                  max={1440}
                  value={(cfg.timeoutMinutes as number) ?? 60}
                  onChange={(e) => setConfig({ timeoutMinutes: parseInt(e.target.value) })}
                  className="h-8 text-sm"
                />
              </Field>
            </div>
            <Field label="On Reject">
              <Select
                value={(cfg.onReject as string) || "abort"}
                onValueChange={(v) => setConfig({ onReject: v as WorkflowNodeConfig["onReject"] })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="abort">Abort workflow</SelectItem>
                  <SelectItem value="retry">Route back to previous agent</SelectItem>
                  <SelectItem value="skip">Skip and continue</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
        )}

        {/* ── Transform ───────────────────────────────────────────────────── */}
        {node.type === "transform" && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Data Transform</SectionLabel>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Input Format">
                <Select
                  value={(cfg.inputFormat as string) || "json"}
                  onValueChange={(v) => setConfig({ inputFormat: v })}
                >
                  <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="json">JSON</SelectItem>
                    <SelectItem value="text">Plain Text</SelectItem>
                    <SelectItem value="csv">CSV</SelectItem>
                    <SelectItem value="xml">XML</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Output Format">
                <Select
                  value={(cfg.outputFormat as string) || "json"}
                  onValueChange={(v) => setConfig({ outputFormat: v })}
                >
                  <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="json">JSON</SelectItem>
                    <SelectItem value="text">Plain Text</SelectItem>
                    <SelectItem value="csv">CSV</SelectItem>
                    <SelectItem value="markdown">Markdown</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <Field label="Transform Expression (JQ / JSONata)" hint="Expression applied to the input data">
              <Textarea
                value={(cfg.transformExpression as string) || ""}
                onChange={(e) => setConfig({ transformExpression: e.target.value })}
                placeholder=".claims[] | { id: .claimId, amount: .totalCharge, status: .status }"
                rows={4}
                className="font-mono text-xs resize-none"
              />
            </Field>
          </div>
        )}

        {/* ── End ─────────────────────────────────────────────────────────── */}
        {(node.type === "end" || node.type === "output") && (
          <div className="space-y-4 p-4 border-b border-border">
            <SectionLabel>Workflow Output</SectionLabel>
            <Field label="Output Format">
              <Select
                value={(cfg.endOutputFormat as string) || "json"}
                onValueChange={(v) => setConfig({ endOutputFormat: v as WorkflowNodeConfig["endOutputFormat"] })}
              >
                <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="json">JSON</SelectItem>
                  <SelectItem value="text">Plain Text</SelectItem>
                  <SelectItem value="markdown">Markdown Report</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Success Message">
              <Input
                value={(cfg.successMessage as string) || ""}
                onChange={(e) => setConfig({ successMessage: e.target.value })}
                placeholder="Claim successfully processed"
                className="h-8 text-sm"
              />
            </Field>
          </div>
        )}
      </div>

      {/* Footer */}
      <div className="border-t border-border p-4">
        <Button
          variant="destructive"
          size="sm"
          className="w-full gap-2"
          onClick={() => onDeleteNode(node.id)}
        >
          <Trash2 className="h-4 w-4" />
          Delete Node
        </Button>
      </div>
    </div>
  )
}
