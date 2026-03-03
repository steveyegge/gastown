import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

// ── Types ─────────────────────────────────────────────────────────────────────

interface DataSourceConfig {
  id: string;
  type: "storage" | "sql" | "cosmos" | "fabric";
  name: string;
  tenantId: string;
  connectionRef: string;  // Key Vault secret reference or managed identity resource id
  status: "active" | "inactive";
  metadata: Record<string, string | number | boolean>;
  createdAt: string;
  updatedAt: string;
}

// ── GET /api/data/sources  ─────────────────────────────────────────────────────
// Returns all configured data source connections for the tenant.
app.http("listDataSources", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "data/sources",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const tenantId = req.query.get("tenantId") ?? "default";
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG); // reusing user-config; in prod use a dedicated container
      const { resources } = await container.items
        .query<DataSourceConfig>({
          query: "SELECT * FROM c WHERE c.type IN ('storage','sql','cosmos','fabric') AND c.tenantId = @tid ORDER BY c.createdAt DESC",
          parameters: [{ name: "@tid", value: tenantId }],
        })
        .fetchAll();
      return { status: 200, jsonBody: { sources: resources } };
    } catch (err) {
      ctx.error("listDataSources error:", err);
      return { status: 500, jsonBody: { error: "Failed to retrieve data sources" } };
    }
  },
});

// ── GET /api/data/sources/{id}  ───────────────────────────────────────────────
app.http("getDataSource", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "data/sources/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { id } = req.params;
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query<DataSourceConfig>({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Data source not found" } };
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getDataSource error:", err);
      return { status: 500, jsonBody: { error: "Failed to retrieve data source" } };
    }
  },
});

// ── POST /api/data/sources  ───────────────────────────────────────────────────
// Register a new data source connection (connection string stored in Key Vault by caller).
app.http("createDataSource", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "data/sources",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = (await req.json()) as Partial<DataSourceConfig>;
      if (!body.type || !body.name || !body.connectionRef) {
        return { status: 400, jsonBody: { error: "type, name, and connectionRef are required" } };
      }
      const now = new Date().toISOString();
      const record: DataSourceConfig = {
        id: `ds-${body.type}-${Date.now()}`,
        type: body.type,
        name: body.name,
        tenantId: body.tenantId ?? "default",
        connectionRef: body.connectionRef,
        status: "active",
        metadata: body.metadata ?? {},
        createdAt: now,
        updatedAt: now,
      };
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      await container.items.create(record);
      return { status: 201, jsonBody: record };
    } catch (err) {
      ctx.error("createDataSource error:", err);
      return { status: 500, jsonBody: { error: "Failed to create data source" } };
    }
  },
});

// ── PATCH /api/data/sources/{id}  ─────────────────────────────────────────────
app.http("updateDataSource", {
  methods: ["PATCH"],
  authLevel: "anonymous",
  route: "data/sources/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { id } = req.params;
    try {
      const patch = (await req.json()) as Partial<DataSourceConfig>;
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query<DataSourceConfig>({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Data source not found" } };
      const updated: DataSourceConfig = { ...resources[0], ...patch, id, updatedAt: new Date().toISOString() };
      await container.items.upsert(updated);
      return { status: 200, jsonBody: updated };
    } catch (err) {
      ctx.error("updateDataSource error:", err);
      return { status: 500, jsonBody: { error: "Failed to update data source" } };
    }
  },
});

// ── DELETE /api/data/sources/{id}  ────────────────────────────────────────────
app.http("deleteDataSource", {
  methods: ["DELETE"],
  authLevel: "anonymous",
  route: "data/sources/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { id } = req.params;
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query<DataSourceConfig>({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Data source not found" } };
      await container.item(id, resources[0].tenantId).delete();
      return { status: 204 };
    } catch (err) {
      ctx.error("deleteDataSource error:", err);
      return { status: 500, jsonBody: { error: "Failed to delete data source" } };
    }
  },
});

// ── GET /api/data/datasets  ───────────────────────────────────────────────────
// Returns the built-in dataset catalog metadata.
app.http("listDatasets", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "data/datasets",
  handler: async (_req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    // In production this would query a dedicated datasets container.
    // Returning catalog summary here to demonstrate the endpoint.
    return {
      status: 200,
      jsonBody: {
        total: 14,
        categories: ["healthcare", "nlp", "tabular", "vision", "multimodal", "finance", "synthetic", "benchmark"],
        note: "Full dataset list served from /api/data/datasets endpoint. Use ?category=healthcare to filter.",
      },
    };
  },
});
