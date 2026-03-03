import { app, type HttpRequest, type HttpResponseInit, type InvocationContext } from "@azure/functions";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client.js";
import { z } from "zod";

const SubmissionSchema = z.object({
  assetName: z.string().min(3),
  type: z.enum(["Agent", "MCP Server", "Model", "Workflow Template", "Evaluator", "Connector"]),
  description: z.string().min(50),
  version: z.string().regex(/^\d+\.\d+\.\d+$/),
  license: z.string(),
  deploymentModes: z.array(z.enum(["SaaS", "PaaS"])).min(1),
  repositoryUrl: z.string().url().optional(),
  containerImage: z.string().optional(),
  dataHandlingDeclaration: z.string(),
});

// POST /api/submissions — submit a new asset for review
app.http("createSubmission", {
  methods: ["POST"],
  authLevel: "function",
  route: "submissions",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = await req.json();
      const parsed = SubmissionSchema.safeParse(body);
      if (!parsed.success) {
        return { status: 400, jsonBody: { error: "Validation failed", details: parsed.error.flatten() } };
      }

      const container = await getContainer(CONTAINERS.SUBMISSIONS);
      const submission = {
        id: `sub-${Date.now()}`,
        ...parsed.data,
        status: "submitted",
        submittedAt: new Date().toISOString(),
        publisherId: "TODO-from-auth", // populate from JWT claims
        tenantId: "TODO-from-auth",
      };

      await container.items.create(submission);

      // Security scan: emit a structured trace event consumed by the scan processor Function.
      // When AZURE_STORAGE_QUEUE_SCAN_URL is set, the scan-processor Function picks this up
      // from the audit-log container's change feed / storage queue and runs policy checks.
      const scanEvent = {
        event: "SECURITY_SCAN_REQUESTED",
        submissionId: submission.id,
        assetType: (parsed.data as Record<string, unknown>).assetType ?? "unknown",
        requestedAt: new Date().toISOString(),
        queueTarget: process.env["AZURE_STORAGE_QUEUE_SCAN_URL"] ?? "(not configured)",
      };
      ctx.log(JSON.stringify(scanEvent));
      // TODO(infra): replace ctx.log with QueueServiceClient.sendMessage once
      // AZURE_STORAGE_QUEUE_SCAN_URL is provisioned in local.settings.json and infra/main.bicep.
      ctx.log(`Submission created: ${submission.id}`);

      return { status: 201, jsonBody: { id: submission.id, status: submission.status } };
    } catch (err) {
      ctx.error("createSubmission error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/submissions/{id} — get submission status
app.http("getSubmission", {
  methods: ["GET"],
  authLevel: "function",
  route: "submissions/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.SUBMISSIONS);
      const { resources } = await container.items
        .query({ query: "SELECT * FROM c WHERE c.id = @id", parameters: [{ name: "@id", value: id }] })
        .fetchAll();
      if (!resources.length) return { status: 404, jsonBody: { error: "Submission not found" } };
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getSubmission error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
