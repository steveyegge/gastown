import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

const RatingSchema = z.object({
  score: z.number().min(1).max(5),
  comment: z.string().max(500).optional(),
});

// POST /api/assets/{id}/ratings — submit a rating
app.http("rateAsset", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "assets/{id}/ratings",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const assetId = req.params.id;
    try {
      const body = await req.json();
      const parsed = RatingSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "score must be 1–5", details: parsed.error.flatten() } };
      }

      // Store individual rating record
      const ratingsContainer = await getContainer(CONTAINERS.RATINGS);
      await ratingsContainer.items.create({
        id: uuidv4(),
        assetId,
        tenantId: "default", // TODO: from JWT
        score: parsed.data.score,
        comment: parsed.data.comment ?? "",
        createdAt: new Date().toISOString(),
      });

      // Recompute aggregate on the asset record
      const { resources: allRatings } = await ratingsContainer.items
        .query({
          query: "SELECT c.score FROM c WHERE c.assetId = @assetId",
          parameters: [{ name: "@assetId", value: assetId }],
        })
        .fetchAll();

      const totalScore = allRatings.reduce((sum: number, r: { score: number }) => sum + r.score, 0);
      const avgRating = allRatings.length > 0 ? totalScore / allRatings.length : 0;

      const assetsContainer = await getContainer(CONTAINERS.ASSETS);
      const { resources: assetResults } = await assetsContainer.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: assetId }] })
        .fetchAll();

      if (assetResults.length > 0) {
        await assetsContainer.items.upsert({
          ...assetResults[0],
          rating: Math.round(avgRating * 10) / 10,
          reviewCount: allRatings.length,
        });
      }

      return {
        status: 201,
        jsonBody: { assetId, rating: Math.round(avgRating * 10) / 10, reviewCount: allRatings.length },
      };
    } catch (err) {
      ctx.error("rateAsset error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/assets/{id}/ratings — get ratings for an asset
app.http("getAssetRatings", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "assets/{id}/ratings",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const assetId = req.params.id;
    try {
      const ratingsContainer = await getContainer(CONTAINERS.RATINGS);
      const { resources } = await ratingsContainer.items
        .query({
          query: "SELECT c.id, c.score, c.comment, c.createdAt FROM c WHERE c.assetId = @assetId ORDER BY c.createdAt DESC",
          parameters: [{ name: "@assetId", value: assetId }],
        })
        .fetchAll();
      return { status: 200, jsonBody: { items: resources, total: resources.length } };
    } catch (err) {
      ctx.error("getAssetRatings error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
