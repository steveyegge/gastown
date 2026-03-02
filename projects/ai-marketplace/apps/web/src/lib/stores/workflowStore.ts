import { create } from "zustand";
import type { WorkflowNodeData } from "@/lib/types";
import {
  createWorkflow,
  updateWorkflow,
  runWorkflow as apiRunWorkflow,
} from "@/lib/api/workflows";
import type { Node, Edge } from "reactflow";

interface NodeExecutionStatus {
  nodeId: string;
  status: WorkflowNodeData["status"];
  startedAt?: string;
  completedAt?: string;
  error?: string;
}

interface WorkflowStore {
  workflowId: string | null;
  workflowName: string;
  isDirty: boolean;
  isSaving: boolean;
  isRunning: boolean;
  saveError: string | null;
  executionStatuses: Record<string, NodeExecutionStatus>;

  setWorkflowId: (id: string) => void;
  setWorkflowName: (name: string) => void;
  setIsDirty: (dirty: boolean) => void;
  setIsRunning: (running: boolean) => void;
  setExecutionStatus: (nodeId: string, status: WorkflowNodeData["status"]) => void;
  resetExecution: () => void;
  saveWorkflow: (nodes: Node<WorkflowNodeData>[], edges: Edge[]) => Promise<void>;
  runWorkflow: (nodes: Node<WorkflowNodeData>[], edges: Edge[]) => Promise<void>;
}

export const useWorkflowStore = create<WorkflowStore>((set, get) => ({
  workflowId: null,
  workflowName: "Denial Intelligence Workflow",
  isDirty: false,
  isSaving: false,
  isRunning: false,
  saveError: null,
  executionStatuses: {},

  setWorkflowId: (workflowId) => set({ workflowId }),
  setWorkflowName: (name) => set({ workflowName: name, isDirty: true }),
  setIsDirty: (isDirty) => set({ isDirty }),
  setIsRunning: (isRunning) => set({ isRunning }),
  setExecutionStatus: (nodeId, status) =>
    set((state) => ({
      executionStatuses: {
        ...state.executionStatuses,
        [nodeId]: {
          nodeId,
          status,
          startedAt: status === "running" ? new Date().toISOString() : state.executionStatuses[nodeId]?.startedAt,
          completedAt: (status === "completed" || status === "error") ? new Date().toISOString() : undefined,
        },
      },
    })),
  resetExecution: () =>
    set({ executionStatuses: {}, isRunning: false }),

  saveWorkflow: async (nodes, edges) => {
    set({ isSaving: true, saveError: null });
    try {
      const { workflowId, workflowName } = get();
      const payload = {
        name: workflowName,
        nodes: nodes.map((n) => ({ id: n.id, type: n.type, position: n.position, data: n.data })),
        edges: edges.map((e) => ({ id: e.id, source: e.source, target: e.target })),
      };
      const saved = workflowId
        ? await updateWorkflow(workflowId, payload)
        : await createWorkflow(payload);
      set({ workflowId: saved.id, isDirty: false });
    } catch (err) {
      set({ saveError: err instanceof Error ? err.message : "Save failed" });
    } finally {
      set({ isSaving: false });
    }
  },

  runWorkflow: async (nodes, edges) => {
    const { saveWorkflow, workflowId } = get();
    // Always save latest canvas state before running
    await saveWorkflow(nodes, edges);
    const id = get().workflowId ?? workflowId;
    if (!id) return;

    set({ isRunning: true, executionStatuses: {} });
    try {
      const result = await apiRunWorkflow(id);
      for (const r of result.results) {
        set((state) => ({
          executionStatuses: {
            ...state.executionStatuses,
            [r.nodeId]: {
              nodeId: r.nodeId,
              status: r.status === "success" ? "completed" : "error",
              startedAt: r.startedAt,
              completedAt: r.completedAt,
            },
          },
        }));
      }
    } catch (err) {
      set({ saveError: err instanceof Error ? err.message : "Run failed" });
    } finally {
      set({ isRunning: false });
    }
  },
}));
