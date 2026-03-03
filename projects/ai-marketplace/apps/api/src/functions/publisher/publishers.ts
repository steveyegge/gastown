import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { z } from "zod";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

const PublisherSchema = z.object({
  name: z.string().min(2).max(120),
  orgName: z.string().min(2),
  contactEmail: z.string().email(),
  website: z.string().url().optional(),
  description: z.string().min(20),
  dataHandlingDeclaration: z.string().min(10),
});

// POST /api/publishers — register a new publisher
app.http("registerPublisher", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "publishers",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = await req.json();
      const parsed = PublisherSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.PUBLISHERS);
      const publisher = {
        id: uuidv4(),
        tenantId: "default", // TODO: extract from JWT
        ...parsed.data,
        verified: false,
        publisherKey: uuidv4().replace(/-/g, ""),
        status: "pending" as const,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };

      await container.items.create(publisher);
      ctx.log(`Publisher registered: ${publisher.id}`);
      return {
        status: 201,
        jsonBody: { id: publisher.id, publisherKey: publisher.publisherKey, status: publisher.status },
      };
    } catch (err) {
      ctx.error("registerPublisher error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/publishers/{id} — get publisher profile
app.http("getPublisher", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "publishers/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.PUBLISHERS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Publisher not found" } };
      // Strip secret key from public response
      const { publisherKey: _key, ...pub } = resources[0];
      return { status: 200, jsonBody: pub };
    } catch (err) {
      ctx.error("getPublisher error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/publishers — list all publishers (admin)
app.http("listPublishers", {
  methods: ["GET"],
  authLevel: "function",
  route: "publishers",
  handler: async (_req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const container = await getContainer(CONTAINERS.PUBLISHERS);
      const { resources } = await container.items
        .query("SELECT c.id, c.name, c.orgName, c.contactEmail, c.verified, c.status, c.createdAt FROM c")
        .fetchAll();
      return { status: 200, jsonBody: { items: resources, total: resources.length } };
    } catch (err) {
      ctx.error("listPublishers error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
