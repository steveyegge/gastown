import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

// Projects container — workspace abstraction (FR-5)
const PROJECTS_CONTAINER = CONTAINERS.PROJECTS;

const ProjectSchema = z.object({
  name: z.string().min(2),
  description: z.string().optional(),
  tenantId: z.string().optional(),
});

// POST /api/projects — create a new workspace project
app.http("createProject", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "projects",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = await req.json();
      const parsed = ProjectSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(PROJECTS_CONTAINER);
      const project = {
        id: uuidv4(),
        tenantId: parsed.data.tenantId ?? "default",
        name: parsed.data.name,
        description: parsed.data.description ?? "",
        assetIds: [] as string[],
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };

      await container.items.create(project);
      return { status: 201, jsonBody: project };
    } catch (err) {
      ctx.error("createProject error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/projects/{id} — get project details with asset list
app.http("getProject", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "projects/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(PROJECTS_CONTAINER);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Project not found" } };
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getProject error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/projects/{id}/assets — add an asset to a project (FR-5)
app.http("addAssetToProject", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "projects/{id}/assets",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const projectId = req.params.id;
    try {
      const { assetId } = (await req.json()) as { assetId: string };
      if (!assetId) return { status: 400, jsonBody: { error: "assetId is required" } };

      const container = await getContainer(PROJECTS_CONTAINER);

      // Upsert a default project if it doesn't exist yet
      let { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: projectId }] })
        .fetchAll();

      if (!resources.length && projectId === "default") {
        const defaultProject = {
          id: "default",
          tenantId: "default",
          name: "Default Workspace",
          description: "Auto-created default workspace",
          assetIds: [assetId],
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
        await container.items.create(defaultProject);
        return { status: 200, jsonBody: { projectId, assetId, assetIds: [assetId] } };
      }

      if (!resources.length) return { status: 404, jsonBody: { error: "Project not found" } };

      const project = resources[0];
      const assetIds: string[] = project.assetIds ?? [];
      if (!assetIds.includes(assetId)) assetIds.push(assetId);

      await container.items.upsert({ ...project, assetIds, updatedAt: new Date().toISOString() });

      // Log to audit trail
      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "asset-added-to-project",
        projectId,
        assetId,
        tenantId: project.tenantId,
        timestamp: new Date().toISOString(),
      });

      return { status: 200, jsonBody: { projectId, assetId, assetIds } };
    } catch (err) {
      ctx.error("addAssetToProject error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// DELETE /api/projects/{id}/assets/{assetId} — remove an asset from a project
app.http("removeAssetFromProject", {
  methods: ["DELETE"],
  authLevel: "anonymous",
  route: "projects/{id}/assets/{assetId}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { id: projectId, assetId } = req.params;
    try {
      const container = await getContainer(PROJECTS_CONTAINER);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: projectId }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Project not found" } };

      const project = resources[0];
      const assetIds = (project.assetIds as string[]).filter((id: string) => id !== assetId);
      await container.items.upsert({ ...project, assetIds, updatedAt: new Date().toISOString() });

      return { status: 200, jsonBody: { projectId, assetIds } };
    } catch (err) {
      ctx.error("removeAssetFromProject error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/projects — list all projects for a tenant
app.http("listProjects", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "projects",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const tenantId = req.query.get("tenantId") ?? "default";
    const pageSize = Math.min(parseInt(req.query.get("pageSize") ?? "20", 10), 100);
    const offset = (parseInt(req.query.get("page") ?? "1", 10) - 1) * pageSize;
    try {
      const container = await getContainer(PROJECTS_CONTAINER);
      const query = `SELECT c.id, c.name, c.description, c.assetIds, c.createdAt, c.updatedAt FROM c WHERE c.tenantId = @tenantId ORDER BY c.updatedAt DESC OFFSET ${offset} LIMIT ${pageSize}`;
      const countQuery = "SELECT VALUE COUNT(1) FROM c WHERE c.tenantId = @tenantId";
      const parameters = [{ name: "@tenantId", value: tenantId }];

      const [{ resources: items }, { resources: countResult }] = await Promise.all([
        container.items.query({ query, parameters }).fetchAll(),
        container.items.query({ query: countQuery, parameters }).fetchAll(),
      ]);

      return { status: 200, jsonBody: { items, total: countResult[0] ?? 0, pageSize } };
    } catch (err) {
      ctx.error("listProjects error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
