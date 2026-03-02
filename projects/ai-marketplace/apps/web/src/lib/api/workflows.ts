import type { Workflow } from "@/lib/types";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "/api";

interface RunResult {
  workflowId: string;
  status: "completed" | "failed";
  executionOrder: string[];
  results: Array<{
    nodeId: string;
    status: "success" | "error";
    startedAt: string;
    completedAt: string;
    outputSummary: string;
  }>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error((err as { error?: string }).error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

export async function createWorkflow(payload: {
  name: string;
  description?: string;
  nodes: unknown[];
  edges: unknown[];
  tenantId?: string;
}): Promise<Workflow> {
  return request<Workflow>("/workflows", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function getWorkflow(id: string): Promise<Workflow> {
  return request<Workflow>(`/workflows/${id}`);
}

export async function updateWorkflow(
  id: string,
  patch: Partial<Pick<Workflow, "name" | "description" | "nodes" | "edges">>
): Promise<Workflow> {
  return request<Workflow>(`/workflows/${id}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function runWorkflow(id: string): Promise<RunResult> {
  return request<RunResult>(`/workflows/${id}/run`, { method: "POST" });
}
