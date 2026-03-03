import {
  app,
  HttpRequest,
  HttpResponseInit,
  InvocationContext,
} from "@azure/functions";
import { v4 as uuidv4 } from "uuid";
import { getContainer, CONTAINERS } from "../../lib/cosmos/client";

// POST /api/workflows — create a new workflow
app.http("createWorkflow", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "workflows",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    try {
      const body = (await req.json()) as Record<string, unknown>;
      const container = await getContainer(CONTAINERS.WORKFLOWS);

      const workflow = {
        id: uuidv4(),
        tenantId: (body.tenantId as string) ?? "default",
        name: (body.name as string) ?? "Untitled Workflow",
        description: (body.description as string) ?? "",
        nodes: body.nodes ?? [],
        edges: body.edges ?? [],
        status: "draft" as const,
        version: 1,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };

      const { resource } = await container.items.create(workflow);
      return { status: 201, jsonBody: resource };
    } catch (err) {
      ctx.error("createWorkflow error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// GET /api/workflows/{id} — load a workflow by id
app.http("getWorkflow", {
  methods: ["GET"],
  authLevel: "anonymous",
  route: "workflows/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.WORKFLOWS);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) {
        return { status: 404, jsonBody: { error: "Workflow not found" } };
      }
      return { status: 200, jsonBody: resources[0] };
    } catch (err) {
      ctx.error("getWorkflow error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// PUT /api/workflows/{id} — update nodes/edges/name
app.http("updateWorkflow", {
  methods: ["PUT"],
  authLevel: "anonymous",
  route: "workflows/{id}",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const body = (await req.json()) as Record<string, unknown>;
      const container = await getContainer(CONTAINERS.WORKFLOWS);

      // Fetch existing to get tenantId (partition key)
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) {
        return { status: 404, jsonBody: { error: "Workflow not found" } };
      }

      const existing = resources[0];
      const updated = {
        ...existing,
        name: body.name ?? existing.name,
        description: body.description ?? existing.description,
        nodes: body.nodes ?? existing.nodes,
        edges: body.edges ?? existing.edges,
        version: existing.version + 1,
        updatedAt: new Date().toISOString(),
      };

      const { resource } = await container.items.upsert(updated);
      return { status: 200, jsonBody: resource };
    } catch (err) {
      ctx.error("updateWorkflow error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});

// POST /api/workflows/{id}/run — execute a workflow (simulate node-by-node)
app.http("runWorkflow", {
  methods: ["POST"],
  authLevel: "anonymous",
  route: "workflows/{id}/run",
  handler: async (req: HttpRequest, ctx: InvocationContext): Promise<HttpResponseInit> => {
    const id = req.params.id;
    try {
      const container = await getContainer(CONTAINERS.WORKFLOWS);
      const { resources } = await container.items
        .query({
          query: "SELECT * FROM c WHERE c.id = @id",
          parameters: [{ name: "@id", value: id }],
        })
        .fetchAll();

      if (!resources.length) {
        return { status: 404, jsonBody: { error: "Workflow not found" } };
      }

      const workflow = resources[0];
      const nodes: Array<{ id: string; type?: string }> = workflow.nodes ?? [];

      // Topological order: sort by edge dependencies (simple BFS from source nodes)
      const edges: Array<{ source: string; target: string }> = workflow.edges ?? [];
      const inDegree = new Map<string, number>();
      const adjacency = new Map<string, string[]>();

      for (const n of nodes) {
        inDegree.set(n.id, 0);
        adjacency.set(n.id, []);
      }
      for (const e of edges) {
        inDegree.set(e.target, (inDegree.get(e.target) ?? 0) + 1);
        adjacency.get(e.source)?.push(e.target);
      }

      const queue = nodes.filter((n) => (inDegree.get(n.id) ?? 0) === 0).map((n) => n.id);
      const executionOrder: string[] = [];
      while (queue.length) {
        const cur = queue.shift()!;
        executionOrder.push(cur);
        for (const next of adjacency.get(cur) ?? []) {
          const deg = (inDegree.get(next) ?? 1) - 1;
          inDegree.set(next, deg);
          if (deg === 0) queue.push(next);
        }
      }

      // Build execution result: each node gets a status + simulated latency
      const executionResults = executionOrder.map((nodeId, idx) => ({
        nodeId,
        status: "success" as const,
        startedAt: new Date(Date.now() + idx * 120).toISOString(),
        completedAt: new Date(Date.now() + idx * 120 + 95).toISOString(),
        outputSummary: `Node ${nodeId} executed successfully`,
      }));

      // Persist run record to audit-log container
      const auditContainer = await getContainer(CONTAINERS.AUDIT_LOG);
      await auditContainer.items.create({
        id: uuidv4(),
        type: "workflow-run",
        workflowId: id,
        tenantId: workflow.tenantId,
        executionResults,
        triggeredAt: new Date().toISOString(),
      });

      return {
        status: 200,
        jsonBody: {
          workflowId: id,
          status: "completed",
          executionOrder,
          results: executionResults,
        },
      };
    } catch (err) {
      ctx.error("runWorkflow error:", err);
      return { status: 500, jsonBody: { error: "Internal server error" } };
    }
  },
});
