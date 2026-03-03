import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

const VersionSchema = z.object({
  version: z.string().regex(/^\d+\.\d+\.\d+$/, "Must be semver"),
  notes: z.string().min(10),
  repositoryUrl: z.string().url().optional(),
  containerImage: z.string().optional(),
  pinnedDependencies: z.record(z.string()).optional(),
});

// POST /api/assets/{id}/versions — publish a new version (FR-8)
app.http("publishVersion", {
  methods: ["POST"],
  authLevel: "function",
  route: "assets/{id}/versions",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const assetId = req.params.id;
    try {
      const body = await req.json();
      const parsed = VersionSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.ASSETS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: assetId }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Asset not found" } };

      const asset = resources[0];
      const newVersion = {
        version: parsed.data.version,
        notes: parsed.data.notes,
        releasedAt: new Date().toISOString(),
        isLatest: true,
        repositoryUrl: parsed.data.repositoryUrl,
        containerImage: parsed.data.containerImage,
        pinnedDependencies: parsed.data.pinnedDependencies ?? {},
        deprecated: false,
      };

      // Mark all prior versions as not latest
      const versions = ((asset.versions ?? []) as Array<{ isLatest: boolean }>).map((v) => ({
        ...v,
        isLatest: false,
      }));
      versions.push(newVersion);

      const updated = {
        ...asset,
        versions,
        latestVersion: parsed.data.version,
        updatedAt: new Date().toISOString(),
      };

      await container.items.upsert(updated);

      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "version-published",
        assetId,
        version: parsed.data.version,
        tenantId: asset.tenantId ?? "default",
        timestamp: new Date().toISOString(),
      });

      return { status: 201, jsonBody: { assetId, version: parsed.data.version, status: "published" } };
    } catch (err) {
      ctx.error("publishVersion error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/assets/{id}/versions/{version}/deprecate — deprecate a version (FR-8)
app.http("deprecateVersion", {
  methods: ["POST"],
  authLevel: "function",
  route: "assets/{id}/versions/{version}/deprecate",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { id: assetId, version } = req.params;
    try {
      const body = (await req.json().catch(() => ({}))) as { reason?: string };
      const container = await getContainer(CONTAINERS.ASSETS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: assetId }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Asset not found" } };

      const asset = resources[0];
      const versions = (asset.versions ?? []) as Array<{ version: string; deprecated?: boolean; deprecationReason?: string }>;
      const target = versions.find((v) => v.version === version);
      if (!target) return { status: 404, jsonBody: { error: `Version ${version} not found` } };

      target.deprecated = true;
      target.deprecationReason = body.reason ?? "Deprecated by publisher";

      await container.items.upsert({ ...asset, versions, updatedAt: new Date().toISOString() });

      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "version-deprecated",
        assetId,
        version,
        reason: target.deprecationReason,
        tenantId: asset.tenantId ?? "default",
        timestamp: new Date().toISOString(),
      });

      return { status: 200, jsonBody: { assetId, version, deprecated: true } };
    } catch (err) {
      ctx.error("deprecateVersion error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/projects/{projectId}/pins — pin a specific asset version for production (FR-8)
app.http("pinVersion", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "projects/{projectId}/pins",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const projectId = req.params.projectId;
    try {
      const { assetId, version } = (await req.json()) as { assetId: string; version: string };
      if (!assetId || !version) return { status: 400, jsonBody: { error: "assetId and version are required" } };

      const pinsContainer = await getContainer(CONTAINERS.VERSION_PINS);
      const pin = {
        id: `${projectId}-${assetId}`,
        projectId,
        assetId,
        pinnedVersion: version,
        pinnedAt: new Date().toISOString(),
        tenantId: "default",
      };
      await pinsContainer.items.upsert(pin);

      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "version-pinned",
        projectId,
        assetId,
        version,
        timestamp: new Date().toISOString(),
      });

      return { status: 200, jsonBody: pin };
    } catch (err) {
      ctx.error("pinVersion error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
