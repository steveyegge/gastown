import {
  app,
  type HttpRequest,
  type HttpResponseInit,
  type InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";

// POST /api/submissions/{id}/approve — approve a submission
app.http("approveSubmission", {
  methods: ["POST"],
  authLevel: "function",
  route: "submissions/{id}/approve",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const body = (await req.json().catch(() => ({}))) as Record<string, unknown>;
      const container = await getContainer(CONTAINERS.SUBMISSIONS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Submission not found" } };

      const updated = {
        ...resources[0],
        status: "approved",
        reviewedAt: new Date().toISOString(),
        reviewedBy: (body.reviewerId as string) ?? "system",
        reviewNotes: (body.notes as string) ?? "",
        distributionScope: (body.distributionScope as string) ?? "internal",
      };

      await container.items.upsert(updated);

      // Write immutable audit entry
      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "submission-approved",
        submissionId: id,
        tenantId: updated.tenantId,
        reviewedBy: updated.reviewedBy,
        distributionScope: updated.distributionScope,
        timestamp: new Date().toISOString(),
      });

      ctx.log(`Submission approved: ${id}`);
      return { status: 200, jsonBody: { id, status: "approved", distributionScope: updated.distributionScope } };
    } catch (err) {
      ctx.error("approveSubmission error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/submissions/{id}/reject — reject a submission
app.http("rejectSubmission", {
  methods: ["POST"],
  authLevel: "function",
  route: "submissions/{id}/reject",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const body = (await req.json().catch(() => ({}))) as Record<string, unknown>;
      const container = await getContainer(CONTAINERS.SUBMISSIONS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();

      if (!resources.length) return { status: 404, jsonBody: { error: "Submission not found" } };

      const updated = {
        ...resources[0],
        status: "rejected",
        reviewedAt: new Date().toISOString(),
        reviewedBy: (body.reviewerId as string) ?? "system",
        rejectionReason: (body.reason as string) ?? "Did not meet policy requirements",
        policyViolations: (body.policyViolations as string[]) ?? [],
      };

      await container.items.upsert(updated);

      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "submission-rejected",
        submissionId: id,
        tenantId: updated.tenantId,
        reviewedBy: updated.reviewedBy,
        rejectionReason: updated.rejectionReason,
        timestamp: new Date().toISOString(),
      });

      return { status: 200, jsonBody: { id, status: "rejected", reason: updated.rejectionReason } };
    } catch (err) {
      ctx.error("rejectSubmission error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/submissions — list pending submissions for review queue
app.http("listSubmissions", {
  methods: ["GET"],
  authLevel: "function",
  route: "submissions",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const status = req.query.get("status") ?? "submitted";
      const container = await getContainer(CONTAINERS.SUBMISSIONS);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.status = @status ORDER BY c.submittedAt ASC",
          parameters: [{ name: "@status", value: status }],
        })
        .fetchAll();
      return { status: 200, jsonBody: { items: resources, total: resources.length } };
    } catch (err) {
      ctx.error("listSubmissions error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
