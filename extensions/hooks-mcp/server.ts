/**
 * Gas Town Hooks MCP Server
 *
 * Agent-agnostic hook server that any runtime (Claude, Copilot, Gemini) can call
 * to emit Gas Town events, manage sessions, and enable Seance-like recovery.
 *
 * Tools:
 * - hook-session-start  : Emit session_start event (announces agent to Gas Town)
 * - hook-session-end    : Emit session_end event + save session summary
 * - hook-emit           : Emit arbitrary event to .events.jsonl
 * - hook-mail-send      : Send inter-agent mail (same as Claude's mail hook)
 * - hook-mail-check     : Check mailbox for incoming messages
 * - hook-nudge-check    : Drain pending nudges for a headless session
 * - hook-nudge-pending  : Check nudge count without draining
 * - hook-heartbeat      : Write liveness heartbeat for headless session
 * - seance-replay       : Load context from a previous session for recovery
 * - seance-list         : List discoverable sessions from .events.jsonl
 */
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import fs from "node:fs/promises";
import fsSync from "node:fs";
import os from "node:os";
import path from "node:path";
import { z } from "zod";

/** Resolve the town root */
function townRoot(): string {
  return process.env.GT_TOWN ?? path.join(os.homedir(), "gt");
}

const EVENTS_FILE = () => path.join(townRoot(), ".events.jsonl");
const SESSIONS_DIR = () => path.join(townRoot(), "sessions");
const MAIL_DIR = () => path.join(townRoot(), "mail");
const NUDGE_QUEUE_DIR = () => path.join(townRoot(), ".runtime", "nudge_queue");
const HEADLESS_SESSIONS_DIR = () => path.join(townRoot(), ".runtime", "sessions");
const HEARTBEAT_DIR = () => path.join(townRoot(), ".runtime", "heartbeat");

// ── Event helpers ─────────────────────────────────────────────────────────────

async function appendEvent(event: {
  type: string;
  actor: string;
  payload: unknown;
}): Promise<void> {
  const line = JSON.stringify({ ts: new Date().toISOString(), ...event });
  await fs.appendFile(EVENTS_FILE(), line + "\n", "utf-8");
}

interface EventRecord {
  ts: string;
  type: string;
  actor: string;
  payload: Record<string, unknown>;
}

async function readEvents(): Promise<EventRecord[]> {
  try {
    const raw = await fs.readFile(EVENTS_FILE(), "utf-8");
    return raw
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line) as EventRecord);
  } catch {
    return [];
  }
}

// ── Server factory ────────────────────────────────────────────────────────────

export function createServer(): McpServer {
  const server = new McpServer({
    name: "Gas Town Hooks Server",
    version: "1.0.0",
  });

  // ── hook-session-start ───────────────────────────────────────────────────
  server.tool(
    "hook-session-start",
    "Announce a new agent session to Gas Town. Call this at the start of every work session.",
    {
      sessionId: z.string().optional().describe("Session ID (auto-generated if omitted)"),
      agent: z.string().optional().describe("Agent name (default: github-copilot)"),
      rig: z.string().optional().describe("Rig name (default: mcpapps1)"),
      topic: z.string().optional().describe("Session topic or reason (e.g. 'dispatch', 'handoff', 'prime')"),
    },
    async ({ sessionId, agent, rig, topic }): Promise<CallToolResult> => {
      const id = sessionId ?? `ghcp-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      const rigName = rig ?? "mcpapps1";
      const agentName = agent ?? "github-copilot";

      await appendEvent({
        type: "session_start",
        actor: `${rigName}/${agentName}`,
        payload: {
          session_id: id,
          topic: topic ?? "prime",
          agent: agentName,
          rig: rigName,
          platform: "vscode-copilot",
        },
      });

      return { content: [{ type: "text", text: `Session started: ${id} (${agentName} on ${rigName})` }] };
    },
  );

  // ── hook-session-end ─────────────────────────────────────────────────────
  server.tool(
    "hook-session-end",
    "End a session and persist a summary for Seance recovery. Call before ending a work session.",
    {
      sessionId: z.string().describe("Session ID from hook-session-start"),
      summary: z.string().describe("What was accomplished, decisions made, and next steps"),
      issueIds: z.array(z.string()).optional().describe("bd issue IDs worked on"),
      rig: z.string().optional().describe("Rig name"),
    },
    async ({ sessionId, summary, issueIds, rig }): Promise<CallToolResult> => {
      const rigName = rig ?? "mcpapps1";
      const record = {
        sessionId,
        agent: "github-copilot",
        rig: rigName,
        issueIds: issueIds ?? [],
        summary,
        timestamp: new Date().toISOString(),
      };

      // Save to sessions dir
      const dir = SESSIONS_DIR();
      await fs.mkdir(dir, { recursive: true });
      await fs.writeFile(path.join(dir, `${sessionId}.json`), JSON.stringify(record, null, 2), "utf-8");

      // Emit event
      await appendEvent({
        type: "session_end",
        actor: `${rigName}/copilot`,
        payload: record,
      });

      return { content: [{ type: "text", text: `Session ended and saved: ${sessionId}` }] };
    },
  );

  // ── hook-emit ────────────────────────────────────────────────────────────
  server.tool(
    "hook-emit",
    "Emit an arbitrary event to Gas Town's .events.jsonl stream.",
    {
      eventType: z.string().describe("Event type (e.g. task_complete, escalation, checkpoint)"),
      actor: z.string().optional().describe("Actor identifier (default: mcpapps1/copilot)"),
      payload: z.record(z.string(), z.unknown()).optional().describe("Event payload"),
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

  // ── hook-mail-send ───────────────────────────────────────────────────────
  server.tool(
    "hook-mail-send",
    "Send inter-agent mail (same concept as Gas Town's mail hook). Messages are delivered to the recipient's mailbox as JSON files.",
    {
      to: z.string().describe("Recipient address (e.g. mcpapps1/witness, mayor)"),
      from: z.string().optional().describe("Sender address (default: mcpapps1/copilot)"),
      subject: z.string().describe("Mail subject"),
      body: z.string().describe("Mail body"),
    },
    async ({ to, from, subject, body }): Promise<CallToolResult> => {
      const mailDir = path.join(MAIL_DIR(), to.replace(/\//g, path.sep));
      await fs.mkdir(mailDir, { recursive: true });
      const msgId = `msg-${Date.now()}-${Math.random().toString(36).slice(2, 5)}`;
      const msg = {
        id: msgId,
        from: from ?? "mcpapps1/copilot",
        to,
        subject,
        body,
        timestamp: new Date().toISOString(),
      };
      await fs.writeFile(path.join(mailDir, `${msgId}.json`), JSON.stringify(msg, null, 2), "utf-8");

      await appendEvent({
        type: "mail_send",
        actor: msg.from,
        payload: { to, subject, msgId },
      });

      return { content: [{ type: "text", text: `Mail sent to ${to}: ${subject} (${msgId})` }] };
    },
  );

  // ── hook-mail-check ──────────────────────────────────────────────────────
  server.tool(
    "hook-mail-check",
    "Check mailbox for incoming messages. Returns unread mail as JSON.",
    {
      address: z.string().optional().describe("Mailbox address (default: mcpapps1/copilot)"),
    },
    async ({ address }): Promise<CallToolResult> => {
      const addr = address ?? "mcpapps1/copilot";
      const mailDir = path.join(MAIL_DIR(), addr.replace(/\//g, path.sep));
      try {
        const files = await fs.readdir(mailDir);
        const msgs = [];
        for (const f of files.filter((x) => x.endsWith(".json")).sort()) {
          const raw = await fs.readFile(path.join(mailDir, f), "utf-8");
          msgs.push(JSON.parse(raw));
        }
        if (msgs.length === 0) {
          return { content: [{ type: "text", text: `No mail for ${addr}` }] };
        }
        return { content: [{ type: "text", text: JSON.stringify(msgs, null, 2) }] };
      } catch {
        return { content: [{ type: "text", text: `No mailbox found for ${addr}` }] };
      }
    },
  );

  // ── seance-list ──────────────────────────────────────────────────────────
  server.tool(
    "seance-list",
    "List discoverable sessions from .events.jsonl (Gas Town Seance equivalent). Shows session_start events sorted by most recent.",
    {
      rig: z.string().optional().describe("Filter by rig name"),
      limit: z.number().optional().describe("Max results (default: 10)"),
    },
    async ({ rig, limit }): Promise<CallToolResult> => {
      const events = await readEvents();
      let sessions = events
        .filter((e) => e.type === "session_start" || e.type === "session_save" || e.type === "session_end")
        .reverse();

      if (rig) {
        sessions = sessions.filter((e) => e.actor.startsWith(rig + "/") || (e.payload as Record<string, unknown>).rig === rig);
      }

      const maxResults = limit ?? 10;
      sessions = sessions.slice(0, maxResults);

      if (sessions.length === 0) {
        return { content: [{ type: "text", text: "No sessions discovered." }] };
      }

      const summary = sessions.map((s) => {
        const p = s.payload as Record<string, unknown>;
        return `[${s.ts}] ${s.type} — ${s.actor} | session=${p.session_id ?? p.sessionId ?? "?"} topic=${p.topic ?? "-"}`;
      }).join("\n");

      return { content: [{ type: "text", text: summary }] };
    },
  );

  // ── seance-replay ────────────────────────────────────────────────────────
  server.tool(
    "seance-replay",
    "Load the full context from a previous session for recovery (GHCP Seance equivalent). " +
    "Returns the session summary, events, and any saved state. Use this at the start of a new session " +
    "to recover context from where a previous session left off.",
    {
      sessionId: z.string().optional().describe("Specific session ID to replay (default: most recent)"),
      rig: z.string().optional().describe("Filter by rig name"),
    },
    async ({ sessionId, rig }): Promise<CallToolResult> => {
      // Load session summary if available
      const sessDir = SESSIONS_DIR();
      let sessionRecord: Record<string, unknown> | null = null;

      if (sessionId) {
        try {
          const raw = await fs.readFile(path.join(sessDir, `${sessionId}.json`), "utf-8");
          sessionRecord = JSON.parse(raw);
        } catch {
          // Session file not found, will try events
        }
      } else {
        // Find most recent session file
        try {
          const files = (await fs.readdir(sessDir)).filter((f) => f.endsWith(".json")).sort().reverse();
          for (const f of files) {
            const raw = await fs.readFile(path.join(sessDir, f), "utf-8");
            const record = JSON.parse(raw);
            if (!rig || record.rig === rig) {
              sessionRecord = record;
              sessionId = record.sessionId;
              break;
            }
          }
        } catch {
          // No sessions dir
        }
      }

      // Load related events
      const events = await readEvents();
      const relatedEvents = sessionId
        ? events.filter((e) => {
            const p = e.payload as Record<string, unknown>;
            return p.session_id === sessionId || p.sessionId === sessionId;
          })
        : [];

      if (!sessionRecord && relatedEvents.length === 0) {
        return { content: [{ type: "text", text: "No session found to replay. Start fresh." }] };
      }

      const output: string[] = [];
      output.push("# Seance Replay — Previous Session Context\n");

      if (sessionRecord) {
        output.push(`**Session:** ${sessionRecord.sessionId}`);
        output.push(`**Agent:** ${sessionRecord.agent}`);
        output.push(`**Rig:** ${sessionRecord.rig}`);
        output.push(`**Time:** ${sessionRecord.timestamp}`);
        if (Array.isArray(sessionRecord.issueIds) && sessionRecord.issueIds.length > 0) {
          output.push(`**Issues:** ${sessionRecord.issueIds.join(", ")}`);
        }
        output.push("");
        output.push("## Summary");
        output.push(String(sessionRecord.summary));
      }

      if (relatedEvents.length > 0) {
        output.push("\n## Event Timeline");
        for (const e of relatedEvents) {
          output.push(`- [${e.ts}] ${e.type} — ${JSON.stringify(e.payload)}`);
        }
      }

      return { content: [{ type: "text", text: output.join("\n") }] };
    },
  );

  // ── hook-nudge-check ───────────────────────────────────────────────────
  server.tool(
    "hook-nudge-check",
    "Check and drain the nudge queue for a headless session. Returns pending nudges and removes them from the queue. " +
    "Headless agents (GHCP) should call this periodically or when prompted to check for incoming messages.",
    {
      sessionId: z.string().describe("Session ID to check (e.g. ghcp-1234567890-abc12)"),
    },
    async ({ sessionId }): Promise<CallToolResult> => {
      const queueDir = path.join(NUDGE_QUEUE_DIR(), sessionId);
      try {
        await fs.access(queueDir);
      } catch {
        return { content: [{ type: "text", text: "No pending nudges." }] };
      }
      const files = (await fs.readdir(queueDir))
        .filter((f) => f.endsWith(".json"))
        .sort();
      if (files.length === 0) {
        return { content: [{ type: "text", text: "No pending nudges." }] };
      }

      const nudges: unknown[] = [];
      for (const f of files) {
        const filePath = path.join(queueDir, f);
        try {
          const raw = await fs.readFile(filePath, "utf-8");
          const nudge = JSON.parse(raw);

          // Skip expired nudges
          if (nudge.expires_at && new Date(nudge.expires_at) < new Date()) {
            await fs.unlink(filePath).catch(() => {});
            continue;
          }

          // Claim the nudge (rename .json → .claimed to prevent double delivery)
          const claimedPath = filePath.replace(".json", `.claimed.${Date.now()}`);
          await fs.rename(filePath, claimedPath).catch(() => {});

          // Clean up the claimed file after reading
          await fs.unlink(claimedPath).catch(() => {});

          nudges.push(nudge);
        } catch {
          // Skip corrupt files
        }
      }

      if (nudges.length === 0) {
        return { content: [{ type: "text", text: "No pending nudges (all expired)." }] };
      }

      return { content: [{ type: "text", text: JSON.stringify(nudges, null, 2) }] };
    },
  );

  // ── hook-heartbeat ─────────────────────────────────────────────────────
  server.tool(
    "hook-heartbeat",
    "Write a heartbeat file for a headless session so Gas Town's daemon and witness " +
    "can detect session liveness. Call this periodically (e.g. every 60s) from a headless agent.",
    {
      sessionId: z.string().describe("Session ID for this headless session"),
      status: z.enum(["active", "idle", "busy"]).optional().describe("Current agent status (default: active)"),
    },
    async ({ sessionId, status }): Promise<CallToolResult> => {
      const dir = HEARTBEAT_DIR();
      await fs.mkdir(dir, { recursive: true });
      const hbPath = path.join(dir, `${sessionId}.json`);
      const heartbeat = {
        session_id: sessionId,
        timestamp: new Date().toISOString(),
        status: status ?? "active",
        platform: "vscode-copilot",
        pid: process.pid,
      };
      await fs.writeFile(hbPath, JSON.stringify(heartbeat) + "\n", "utf-8");
      return { content: [{ type: "text", text: `Heartbeat written for ${sessionId}` }] };
    },
  );

  // ── hook-nudge-pending ─────────────────────────────────────────────────
  server.tool(
    "hook-nudge-pending",
    "Check if there are pending nudges without draining them. Useful for deciding whether to call hook-nudge-check.",
    {
      sessionId: z.string().describe("Session ID to check"),
    },
    async ({ sessionId }): Promise<CallToolResult> => {
      const queueDir = path.join(NUDGE_QUEUE_DIR(), sessionId);
      try {
        await fs.access(queueDir);
        const files = (await fs.readdir(queueDir)).filter((f) => f.endsWith(".json"));
        return { content: [{ type: "text", text: `${files.length} pending nudge(s).` }] };
      } catch {
        return { content: [{ type: "text", text: "0 pending nudge(s)." }] };
      }
    },
  );

  return server;
}
