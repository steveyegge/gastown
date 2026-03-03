import { app, type HttpRequest, type HttpResponseInit, type InvocationContext } from "@azure/functions";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

// GET /api/assets — list/search assets with optional filters
app.http("getAssets", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "assets",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const { search, type, complianceTier, deploymentMode, tags } = Object.fromEntries(
        req.query.entries()
      );
      const page = parseInt(req.query.get("page") ?? "1", 10);
      const pageSize = Math.min(parseInt(req.query.get("pageSize") ?? "24", 10), 100);
      const offset = (page - 1) * pageSize;

      const container = await getContainer(CONTAINERS.ASSETS);

      // Build dynamic query
      const conditions: string[] = ["c.status = 'published'"];
      const parameters: { name: string; value: string }[] = [];

      if (type) {
        conditions.push("c.type = @type");
        parameters.push({ name: "@type", value: type });
      }
      if (complianceTier && complianceTier !== "all") {
        conditions.push("c.complianceTier = @complianceTier");
        parameters.push({ name: "@complianceTier", value: complianceTier });
      }
      if (deploymentMode && deploymentMode !== "all") {
        conditions.push("ARRAY_CONTAINS(c.deploymentModes, @deploymentMode)");
        parameters.push({ name: "@deploymentMode", value: deploymentMode });
      }
      if (search) {
        conditions.push("(CONTAINS(LOWER(c.name), LOWER(@search)) OR CONTAINS(LOWER(c.description), LOWER(@search)))");
        parameters.push({ name: "@search", value: search });
      }

      const where = conditions.length > 0 ? `WHERE ${conditions.join(" AND ")}` : "";
      const query = `SELECT * FROM c ${where} ORDER BY c.deploymentCount DESC OFFSET ${offset} LIMIT ${pageSize}`;
      const countQuery = `SELECT VALUE COUNT(1) FROM c ${where}`;

      const [{ resources: items }, { resources: countResult }] = await Promise.all([
        container.items.query({ query, parameters }).fetchAll(),
        container.items.query({ query: countQuery, parameters }).fetchAll(),
      ]);

      return {
        status: 200,
        jsonBody: {
          items,
          total: countResult[0] ?? 0,
          page,
          pageSize,
        },
      };
    } catch (err) {
      ctx.error("getAssets error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/assets/{id} — get single asset
app.http("getAsset", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "assets/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.ASSETS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Asset not found" } };
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getAsset error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
