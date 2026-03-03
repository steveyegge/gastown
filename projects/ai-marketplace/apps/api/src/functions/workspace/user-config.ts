import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

/**
 * User configuration — persists per-user preferences to Cosmos DB.
 * Document ID = userId, partition key = userId.
 * Safe to upsert; creates on first write.
 */

const UserConfigSchema = z.object({
  /** UI theme preference */
  theme: z.enum(["light", "dark", "system"]).optional(),
  /** Preferred asset type filter on the catalog page */
  defaultTypeFilter: z.string().optional(),
  /** Preferred compliance tier filter */
  defaultComplianceFilter: z.string().optional(),
  /** Number of cards shown per page */
  pageSizePreference: z.number().min(6).max(100).optional(),
  /** Whether to show the onboarding banner */
  onboardingDismissed: z.boolean().optional(),
  /** Saved search queries (last 10) */
  recentSearches: z.array(z.string()).max(10).optional(),
  /** IDs of starred/bookmarked assets */
  starredAssetIds: z.array(z.string()).optional(),
  /** Arbitrary extra settings key-value pairs */
  extra: z.record(z.unknown()).optional(),
});

// GET /api/users/{userId}/config — retrieve user configuration
app.http("getUserConfig", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "users/{userId}/config",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const userId = req.params.userId;
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.userId = @userId",
          parameters: [{ name: "@userId", value: userId }],
        })
        .fetchAll();

      if (!resources.length) {
        // Return empty defaults rather than 404 — config is created lazily
        return {
          status: 200,
          jsonBody: { userId, theme: "system", starredAssetIds: [], recentSearches: [], extra: {} },
        };
      }
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getUserConfig error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// PUT /api/users/{userId}/config — full replace of user configuration
app.http("putUserConfig", {
  methods: ["PUT"],
  authLevel: "anonymous",
  route: "users/{userId}/config",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const userId = req.params.userId;
    try {
      const body = await req.json();
      const parsed = UserConfigSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const doc = {
        id: userId,
        userId,
        ...parsed.data,
        updatedAt: new Date().toISOString(),
      };

      const { resource } = await container.items.upsert(doc);
      return { status: 200, jsonBody: resource };
    } catch (err) {
      ctx.error("putUserConfig error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// PATCH /api/users/{userId}/config — partial update (merge)
app.http("patchUserConfig", {
  methods: ["PATCH"],
  authLevel: "anonymous",
  route: "users/{userId}/config",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const userId = req.params.userId;
    try {
      const body = await req.json();
      const parsed = UserConfigSchema.partial().safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.USER_CONFIG);

      // Load existing or start fresh
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.userId = @userId",
          parameters: [{ name: "@userId", value: userId }],
        })
        .fetchAll();

      const existing = resources[0] ?? { id: userId, userId, starredAssetIds: [], recentSearches: [] };

      // Merge arrays intelligently
      const merge = { ...existing, ...parsed.data };

      if (parsed.data.recentSearches !== undefined) {
        // Keep only the newest 10 entries
        merge.recentSearches = [...new Set([...(parsed.data.recentSearches ?? [])])].slice(0, 10);
      }

      const updated = { ...merge, updatedAt: new Date().toISOString() };
      const { resource } = await container.items.upsert(updated);
      return { status: 200, jsonBody: resource };
    } catch (err) {
      ctx.error("patchUserConfig error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/users/{userId}/config/stars/{assetId} — star an asset
app.http("starAsset", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "users/{userId}/config/stars/{assetId}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { userId, assetId } = req.params;
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.userId = @userId",
          parameters: [{ name: "@userId", value: userId }],
        })
        .fetchAll();

      const existing = resources[0] ?? { id: userId, userId, starredAssetIds: [], recentSearches: [] };
      const starredAssetIds: string[] = existing.starredAssetIds ?? [];
      if (!starredAssetIds.includes(assetId)) starredAssetIds.push(assetId);

      const { resource } = await container.items.upsert({
        ...existing,
        starredAssetIds,
        updatedAt: new Date().toISOString(),
      });
      return { status: 200, jsonBody: { userId, starredAssetIds: resource?.starredAssetIds } };
    } catch (err) {
      ctx.error("starAsset error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// DELETE /api/users/{userId}/config/stars/{assetId} — un-star an asset
app.http("unstarAsset", {
  methods: ["DELETE"],
  authLevel: "anonymous",
  route: "users/{userId}/config/stars/{assetId}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const { userId, assetId } = req.params;
    try {
      const container = await getContainer(CONTAINERS.USER_CONFIG);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.userId = @userId",
          parameters: [{ name: "@userId", value: userId }],
        })
        .fetchAll();

      if (!resources.length) return { status: 204 };

      const existing = resources[0];
      const starredAssetIds = ((existing.starredAssetIds ?? []) as string[]).filter((id) => id !== assetId);

      await container.items.upsert({ ...existing, starredAssetIds, updatedAt: new Date().toISOString() });
      return { status: 200, jsonBody: { userId, starredAssetIds } };
    } catch (err) {
      ctx.error("unstarAsset error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
