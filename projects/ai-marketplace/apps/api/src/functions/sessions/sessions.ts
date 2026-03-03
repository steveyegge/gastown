import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

/**
 * Sessions track a user's active interaction with an AI asset (agent run,
 * MCP tool call, workflow execution, etc.).  Documents live in the `sessions`
 * container that has a 24 h TTL so stale sessions are cleaned up automatically.
 */

const CreateSessionSchema = z.object({
  assetId: z.string().min(1),
  tenantId: z.string().optional(),
  userId: z.string().optional(),
  /** Optional input payload forwarded to the asset */
  input: z.record(z.unknown()).optional(),
  /** e.g. "agent-run" | "mcp-call" | "workflow-exec" | "preview" */
  sessionType: z.enum(["agent-run", "mcp-call", "workflow-exec", "preview"]).default("preview"),
  /** Configuration overrides for this session */
  config: z.record(z.unknown()).optional(),
});

const UpdateSessionSchema = z.object({
  status: z.enum(["running", "completed", "failed", "cancelled"]).optional(),
  output: z.record(z.unknown()).optional(),
  error: z.string().optional(),
  metrics: z
    .object({
      latencyMs: z.number().optional(),
      tokenCount: z.number().optional(),
      cost: z.number().optional(),
    })
    .optional(),
});

// POST /api/sessions — start a new session
app.http("createSession", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "sessions",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = await req.json();
      const parsed = CreateSessionSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.SESSIONS);
      const now = new Date().toISOString();

      const session = {
        id: uuidv4(),
        tenantId: parsed.data.tenantId ?? "default",
        userId: parsed.data.userId ?? "anonymous",
        assetId: parsed.data.assetId,
        sessionType: parsed.data.sessionType,
        input: parsed.data.input ?? {},
        config: parsed.data.config ?? {},
        status: "running" as const,
        startedAt: now,
        updatedAt: now,
        // TTL is set at the container level (24 h); individual sessions
        // can request a longer window by setting `ttl` explicitly.
        ttl: 86400,
      };

      const { resource } = await container.items.create(session);
      ctx.log(`Session started: ${session.id} for asset ${session.assetId}`);
      return { status: 201, jsonBody: resource };
    } catch (err) {
      ctx.error("createSession error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/sessions/{id} — get session details
app.http("getSession", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "sessions/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.SESSIONS);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Session not found" } };
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getSession error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// PATCH /api/sessions/{id} — update session status / output
app.http("updateSession", {
  methods: ["PATCH"],
  authLevel: "anonymous",
  route: "sessions/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const body = await req.json();
      const parsed = UpdateSessionSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.SESSIONS);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Session not found" } };

      const existing = resources[0];
      const now = new Date().toISOString();
      const updated = {
        ...existing,
        ...parsed.data,
        updatedAt: now,
        // Set completedAt when the session reaches a terminal state
        ...(parsed.data.status && ["completed", "failed", "cancelled"].includes(parsed.data.status)
          ? { completedAt: now }
          : {}),
      };

      const { resource } = await container.items.upsert(updated);
      return { status: 200, jsonBody: resource };
    } catch (err) {
      ctx.error("updateSession error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/sessions — list sessions for a tenant (optionally filtered by assetId)
app.http("listSessions", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "sessions",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const tenantId = req.query.get("tenantId") ?? "default";
    const assetId = req.query.get("assetId");
    const status = req.query.get("status");
    const pageSize = Math.min(parseInt(req.query.get("pageSize") ?? "20", 10), 100);
    const offset = (parseInt(req.query.get("page") ?? "1", 10) - 1) * pageSize;

    try {
      const container = await getContainer(CONTAINERS.SESSIONS);
      const conditions = ["c.tenantId = @tenantId"];
      const parameters: { name: string; value: string }[] = [{ name: "@tenantId", value: tenantId }];

      if (assetId) {
        conditions.push("c.assetId = @assetId");
        parameters.push({ name: "@assetId", value: assetId });
      }
      if (status) {
        conditions.push("c.status = @status");
        parameters.push({ name: "@status", value: status });
      }

      const where = `WHERE ${conditions.join(" AND ")}`;
      const query = `SELECT c.id, c.assetId, c.sessionType, c.status, c.startedAt, c.completedAt, c.metrics FROM c ${where} ORDER BY c.startedAt DESC OFFSET ${offset} LIMIT ${pageSize}`;
      const countQuery = `SELECT VALUE COUNT(1) FROM c ${where}`;

      const [{ resources: items }, { resources: countResult }] = await Promise.all([
        container.items.query({ query, parameters }).fetchAll(),
        container.items.query({ query: countQuery, parameters }).fetchAll(),
      ]);

      return {
        status: 200,
        jsonBody: { items, total: countResult[0] ?? 0, page: Math.floor(offset / pageSize) + 1, pageSize },
      };
    } catch (err) {
      ctx.error("listSessions error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// DELETE /api/sessions/{id} — cancel / delete a session
app.http("deleteSession", {
  methods: ["DELETE"],
  authLevel: "anonymous",
  route: "sessions/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.SESSIONS);
      const { resources } = await container.items
        .query({
          query: "SELECT c.id, c.tenantId FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Session not found" } };

      const { id: docId, tenantId } = resources[0];
      await container.item(docId, tenantId).delete();
      return { status: 204 };
    } catch (err) {
      ctx.error("deleteSession error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
