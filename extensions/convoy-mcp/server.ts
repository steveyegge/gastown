/**
 * Gas Town Convoy MCP Server
 *
 * Tools:
 * - gt-status      : Opens the Convoy dashboard UI (convoys, ready work, agents)
 * - gt-sling       : Assigns a bd issue to a Gas Town rig
 * - bd-close       : Closes a bd issue with a reason
 * - get-gastown-data : App-only polling tool used by the UI (hidden from model)
 *
 * Gracefully degrades when gt/bd are not installed — returns a setup guide.
 */
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult, ReadResourceResult } from "@modelcontextprotocol/sdk/types.js";
import { execFile } from "node:child_process";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import {
  RESOURCE_MIME_TYPE,
  RESOURCE_URI_META_KEY,
  registerAppResource,
  registerAppTool,
} from "@modelcontextprotocol/ext-apps/server";
import { z } from "zod";

const execFileAsync = promisify(execFile);

const DIST_DIR = import.meta.filename.endsWith(".ts")
  ? path.join(import.meta.dirname, "dist")
  : import.meta.dirname;

const RESOURCE_URI = "ui://gastown-convoy/mcp-app.html";

// ── Rig list: maps app folder names to rig names ──────────────────────────────
// Agents can add rigs here as new MCPapp folders are created.
const KNOWN_RIGS = [
  "GetTimeMCPapp",
  "ThreeJSMCPapp",
  "MapMCPapp",
  "BudgetMCPapp",
  "SystemMonitorMCPapp",
  "InViewMCPapp",
  "TranscriptMCPapp",
  "AzureMapsMCPapp",
  "ConvoyMCPapp",
];

// ── CLI helpers ───────────────────────────────────────────────────────────────

/** Resolve the town root (GT_TOWN env or ~/gt) */
function townRoot(): string {
  return process.env.GT_TOWN ?? path.join(os.homedir(), "gt");
}

/** Run a gt or bd command safely (no shell expansion). Returns stdout or throws. */
async function runCmd(
  cmd: "gt" | "bd",
  args: string[],
  cwd?: string,
): Promise<string> {
  const workDir = cwd ?? townRoot();
  const opts = { cwd: workDir, timeout: 15_000, env: { ...process.env } };
  // On Windows, try bare name first (works for .exe Go binaries on PATH),
  // then fall back to .cmd wrapper (works for npm-installed tools).
  const attempts = process.platform === "win32" ? [cmd, `${cmd}.cmd`] : [cmd];
  let lastErr: unknown;
  for (const executable of attempts) {
    try {
      const { stdout } = await execFileAsync(executable, args, opts);
      return stdout.trim();
    } catch (err: unknown) {
      const code = (err as NodeJS.ErrnoException).code;
      // If the executable was found but the command failed (non-zero exit),
      // that's the real error — don't try other executables.
      if (code !== "ENOENT" && code !== "EINVAL") {
        throw err;
      }
      lastErr = err;
    }
  }
  throw lastErr;
}

/** Returns true if `gt` is available on PATH */
async function gtInstalled(): Promise<boolean> {
  try {
    await runCmd("gt", ["version"]);
    return true;
  } catch {
    return false;
  }
}

/** Returns true if `bd` is available on PATH */
async function bdInstalled(): Promise<boolean> {
  try {
    await runCmd("bd", ["--version"]);
    return true;
  } catch {
    return false;
  }
}

// ── Data fetchers ─────────────────────────────────────────────────────────────

interface ReadyIssue {
  id: string;
  title: string;
  priority: number;
  type: string;
  status: string;
}

interface ConvoyRow {
  id: string;
  name: string;
  status: string;
  total: number;
  done: number;
  inProgress: number;
}

interface AgentRow {
  name: string;
  rig: string;
  status: string;
  currentIssue?: string;
}

export interface GastownData {
  installed: { gt: boolean; bd: boolean };
  readyIssues: ReadyIssue[];
  convoys: ConvoyRow[];
  agents: AgentRow[];
  fetchedAt: string;
  errors: string[];
}

async function fetchGastownData(): Promise<GastownData> {
  const [gtOk, bdOk] = await Promise.all([gtInstalled(), bdInstalled()]);
  const errors: string[] = [];
  let readyIssues: ReadyIssue[] = [];
  let convoys: ConvoyRow[] = [];
  let agents: AgentRow[] = [];

  if (bdOk) {
    try {
      const raw = await runCmd("bd", ["ready", "--json"]);
      // bd ready --json returns a JSON array or newline-delimited JSON objects
      // bd returns "issue_type" but our interface expects "type"
      const parsed = parseJsonOutput<Array<Record<string, unknown>>>(raw);
      readyIssues = Array.isArray(parsed)
        ? parsed.map((i) => ({
            id: String(i.id ?? ""),
            title: String(i.title ?? ""),
            priority: Number(i.priority ?? 2),
            type: String(i.issue_type ?? i.type ?? "unknown"),
            status: String(i.status ?? "open"),
          }))
        : [];
    } catch (e) {
      errors.push(`bd ready: ${String(e)}`);
    }
  }

  if (gtOk) {
    try {
      const raw = await runCmd("gt", ["convoy", "list", "--json"]);
      const parsed = parseJsonOutput<ConvoyRow[]>(raw);
      convoys = Array.isArray(parsed) ? parsed : [];
    } catch (e) {
      errors.push(`gt convoy list: ${String(e)}`);
    }

    try {
      // gt agents list doesn't support --json; returns plain text
      const raw = await runCmd("gt", ["agents", "list"]);
      // Filter out informational messages like "No agent sessions running."
      const lines = raw.split("\n").filter((l) => {
        const trimmed = l.trim();
        return trimmed && !trimmed.startsWith("No agent") && !trimmed.startsWith("No active");
      });
      // Parse plain-text output: each line is roughly "name  rig  status"
      agents = lines.map((line) => {
        const parts = line.trim().split(/\s{2,}/);
        return { name: parts[0] ?? "unknown", rig: parts[1] ?? "", status: parts[2] ?? "unknown" };
      });
    } catch (e) {
      // gt agents list requires tmux — soft error, not fatal
      const msg = String(e);
      if (msg.includes("tmux")) {
        errors.push("gt agents: requires tmux (install psmux: winget install psmux)");
      } else {
        errors.push(`gt agents: ${msg.substring(0, 120)}`);
      }
    }
  }

  return {
    installed: { gt: gtOk, bd: bdOk },
    readyIssues,
    convoys,
    agents,
    fetchedAt: new Date().toISOString(),
    errors,
  };
}

/** Parse JSON that may be a single array, single object, or newline-delimited objects */
function parseJsonOutput<T>(raw: string): T | null {
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    // Try newline-delimited JSON
    const lines = raw.split("\n").filter((l) => l.trim().startsWith("{"));
    if (lines.length > 0) {
      try {
        return lines.map((l) => JSON.parse(l)) as unknown as T;
      } catch {
        return null;
      }
    }
    return null;
  }
}

/** Plain-text summary for the model (shown outside the UI) */
function buildStatusText(data: GastownData): string {
  if (!data.installed.gt && !data.installed.bd) {
    return [
      "Gas Town is not installed or not on PATH.",
      "",
      "To install:",
      "  npm install -g @gastown/gt",
      "  # or: go install github.com/steveyegge/gastown/cmd/gt@latest",
      "",
      "Then initialise your workspace:",
      "  gt install ~/gt --git",
      "  cd ~/gt",
      "  gt mayor attach",
    ].join("\n");
  }

  const lines: string[] = [];

  // Diagnostic header: show what's connected
  lines.push("── Gas Town Dashboard ──");
  lines.push(`  gt: ${data.installed.gt ? "✓ connected" : "✗ not found"}`);
  lines.push(`  bd: ${data.installed.bd ? "✓ connected" : "✗ not found"}`);
  lines.push(`  fetched: ${data.fetchedAt}`);
  lines.push("");

  if (data.readyIssues.length > 0) {
    lines.push(`Ready work (${data.readyIssues.length} unblocked issues):`);
    for (const i of data.readyIssues.slice(0, 5)) {
      lines.push(`  [${i.id}] P${i.priority} ${i.type}: ${i.title}`);
    }
    if (data.readyIssues.length > 5) lines.push(`  … and ${data.readyIssues.length - 5} more`);
  } else {
    lines.push("Ready work: 0 unblocked issues (bd ready returned empty)");
  }

  if (data.convoys.length > 0) {
    lines.push(`\nActive convoys (${data.convoys.length}):`);
    for (const c of data.convoys.slice(0, 5)) {
      lines.push(`  [${c.id}] ${c.name} — ${c.done}/${c.total} done`);
    }
  } else {
    lines.push("Convoys: none active");
  }

  if (data.agents.length > 0) {
    lines.push(`\nAgents (${data.agents.length}):`);
    for (const a of data.agents.slice(0, 5)) {
      lines.push(`  ${a.name} → ${a.rig} [${a.status}]`);
    }
  } else {
    lines.push("Agents: none detected");
  }

  if (data.errors.length > 0) {
    lines.push(`\n⚠ Warnings (${data.errors.length}):`);
    for (const e of data.errors) {
      lines.push(`  • ${e}`);
    }
  }

  return lines.join("\n");
}

// ── Server factory ────────────────────────────────────────────────────────────

export function createServer(): McpServer {
  const server = new McpServer({
    name: "Gas Town Convoy Server",
    version: "1.0.0",
  });

  // ── UI resource ──────────────────────────────────────────────────────────
  registerAppResource(
    server,
    RESOURCE_URI,
    RESOURCE_URI,
    { mimeType: RESOURCE_MIME_TYPE },
    async (): Promise<ReadResourceResult> => {
      const html = await fs.readFile(path.join(DIST_DIR, "mcp-app.html"), "utf-8");
      return {
        contents: [{ uri: RESOURCE_URI, mimeType: RESOURCE_MIME_TYPE, text: html }],
      };
    },
  );

  // ── gt-status: main launcher tool ────────────────────────────────────────
  registerAppTool(
    server,
    "gt-status",
    {
      title: "Gas Town Convoy Dashboard",
      description:
        "Opens an interactive Gas Town orchestration dashboard showing ready work (bd issues), " +
        "active convoys, and agent status. Use this to view and manage parallel MCP app development " +
        "across all *MCPapp/ folders in this repo.",
      inputSchema: {},
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async (): Promise<CallToolResult> => {
      const data = await fetchGastownData();
      return {
        content: [{ type: "text", text: buildStatusText(data) }],
      };
    },
  );

  // ── get-gastown-data: app-only polling tool ───────────────────────────────
  registerAppTool(
    server,
    "get-gastown-data",
    {
      title: "Get Gas Town Data",
      description: "Returns current Gas Town convoy, ready-work, and agent data as JSON. Used by the UI for live polling.",
      inputSchema: {},
      _meta: {
        [RESOURCE_URI_META_KEY]: RESOURCE_URI,
        ui: { resourceUri: RESOURCE_URI, visibility: ["app"] },
      },
    },
    async (): Promise<CallToolResult> => {
      const data = await fetchGastownData();
      return {
        content: [{ type: "text", text: JSON.stringify(data) }],
      };
    },
  );

  // ── gt-sling: assign issue to rig ────────────────────────────────────────
  registerAppTool(
    server,
    "gt-sling",
    {
      title: "Sling Issue to Rig",
      description:
        "Assign a bd issue to a Gas Town rig (MCP app folder) for agent work. " +
        `Known rigs: ${KNOWN_RIGS.join(", ")}.`,
      inputSchema: {
        issueId: z.string().min(1).describe("The bd issue ID to assign (e.g. bd-abc12)"),
        rig: z.string().min(1).describe(`The rig name to assign to. One of: ${KNOWN_RIGS.join(", ")}`),
      },
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async ({ issueId, rig }): Promise<CallToolResult> => {
      if (!await gtInstalled()) {
        return { content: [{ type: "text", text: "gt is not installed. Run: npm install -g @gastown/gt" }] };
      }

      try {
        const out = await runCmd("gt", ["sling", issueId, rig]);
        return { content: [{ type: "text", text: out || `Slinging ${issueId} to rig ${rig} — dispatched.` }] };
      } catch (e) {
        return { content: [{ type: "text", text: `gt sling failed: ${String(e)}` }], isError: true };
      }
    },
  );

  // ── bd-close: close an issue ─────────────────────────────────────────────
  registerAppTool(
    server,
    "bd-close",
    {
      title: "Close bd Issue",
      description: "Close a bd issue with an optional reason.",
      inputSchema: {
        issueId: z.string().min(1).describe("The bd issue ID to close (e.g. bd-abc12)"),
        reason: z.string().optional().describe("Reason for closing (optional)"),
      },
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async ({ issueId, reason }): Promise<CallToolResult> => {
      if (!await bdInstalled()) {
        return { content: [{ type: "text", text: "bd is not installed. See: https://github.com/steveyegge/beads" }] };
      }

      const closeArgs = ["close", issueId];
      if (reason) closeArgs.push("--reason", reason);

      try {
        const out = await runCmd("bd", closeArgs);
        return { content: [{ type: "text", text: out || `Closed ${issueId}.` }] };
      } catch (e) {
        return { content: [{ type: "text", text: `bd close failed: ${String(e)}` }], isError: true };
      }
    },
  );

  // ── gt-session-save: persist session context for recovery ─────────────────
  registerAppTool(
    server,
    "gt-session-save",
    {
      title: "Save Session Context",
      description:
        "Persist a structured session summary to ~/gt/sessions/ so the next agent session " +
        "can recover context (GHCP Seance equivalent). Call this before ending a work session.",
      inputSchema: {
        summary: z.string().describe("Plain-text summary of what was accomplished, decisions made, and next steps"),
        issueIds: z.array(z.string()).optional().describe("bd issue IDs worked on"),
        rig: z.string().optional().describe("Rig name this session was working in"),
      },
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async ({ summary, issueIds, rig }): Promise<CallToolResult> => {
      const sessDir = path.join(townRoot(), "sessions");
      await fs.mkdir(sessDir, { recursive: true });
      const sessionId = `ghcp-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      const record = {
        sessionId,
        agent: "github-copilot",
        rig: rig ?? "mcpapps1",
        issueIds: issueIds ?? [],
        summary,
        timestamp: new Date().toISOString(),
      };
      const filePath = path.join(sessDir, `${sessionId}.json`);
      await fs.writeFile(filePath, JSON.stringify(record, null, 2), "utf-8");

      // Also emit to .events.jsonl for Seance compatibility
      await appendEvent({
        type: "session_save",
        actor: `${record.rig}/copilot`,
        payload: record,
      });

      return { content: [{ type: "text", text: `Session saved: ${sessionId}\n${filePath}` }] };
    },
  );

  // ── gt-session-load: load previous session context ────────────────────────
  registerAppTool(
    server,
    "gt-session-load",
    {
      title: "Load Previous Session",
      description:
        "Load the most recent session summary from ~/gt/sessions/ for context recovery. " +
        "Optionally filter by rig name. Returns the session summary and metadata.",
      inputSchema: {
        rig: z.string().optional().describe("Filter by rig name (default: any)"),
        count: z.number().optional().describe("Number of recent sessions to return (default: 1)"),
      },
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async ({ rig, count }): Promise<CallToolResult> => {
      const sessDir = path.join(townRoot(), "sessions");
      try {
        const files = await fs.readdir(sessDir);
        const jsonFiles = files.filter((f) => f.endsWith(".json")).sort().reverse();
        if (jsonFiles.length === 0) {
          return { content: [{ type: "text", text: "No previous sessions found." }] };
        }

        const limit = count ?? 1;
        const sessions: unknown[] = [];
        for (const file of jsonFiles) {
          if (sessions.length >= limit) break;
          const raw = await fs.readFile(path.join(sessDir, file), "utf-8");
          const record = JSON.parse(raw);
          if (rig && record.rig !== rig) continue;
          sessions.push(record);
        }

        if (sessions.length === 0) {
          return { content: [{ type: "text", text: `No sessions found for rig: ${rig}` }] };
        }

        return { content: [{ type: "text", text: JSON.stringify(sessions, null, 2) }] };
      } catch {
        return { content: [{ type: "text", text: "No sessions directory found. No prior sessions." }] };
      }
    },
  );

  // ── gt-emit-event: write to .events.jsonl (hook bridge) ──────────────────
  registerAppTool(
    server,
    "gt-emit-event",
    {
      title: "Emit Gas Town Event",
      description:
        "Write a structured event to ~/gt/.events.jsonl (Gas Town's canonical event stream). " +
        "This bridges GHCP sessions with Gas Town's Seance discovery system.",
      inputSchema: {
        eventType: z.string().describe("Event type (e.g. session_start, session_end, task_complete, escalation)"),
        actor: z.string().optional().describe("Actor identifier (default: mcpapps1/copilot)"),
        payload: z.record(z.string(), z.unknown()).optional().describe("Arbitrary JSON payload"),
      },
      _meta: { [RESOURCE_URI_META_KEY]: RESOURCE_URI },
    },
    async ({ eventType, actor, payload }): Promise<CallToolResult> => {
      await appendEvent({
        type: eventType,
        actor: actor ?? "mcpapps1/copilot",
        payload: payload ?? {},
      });
      return { content: [{ type: "text", text: `Event emitted: ${eventType}` }] };
    },
  );

  return server;
}

// ── Event stream helper ───────────────────────────────────────────────────────

async function appendEvent(event: {
  type: string;
  actor: string;
  payload: unknown;
}): Promise<void> {
  const eventsFile = path.join(townRoot(), ".events.jsonl");
  const line = JSON.stringify({
    ts: new Date().toISOString(),
    ...event,
  });
  await fs.appendFile(eventsFile, line + "\n", "utf-8");
}
