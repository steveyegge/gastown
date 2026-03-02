import axios from "axios";
import type { Asset, AssetFilter, AssetListResponse } from "@/lib/types";

const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:7071/api",
  timeout: 15_000,
  headers: { "Content-Type": "application/json" },
});

// Add auth token if available (MSAL)
api.interceptors.request.use((config) => {
  if (typeof window !== "undefined") {
    const token = sessionStorage.getItem("msal_token");
    if (token) config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// ─── Assets ─────────────────────────────────────────────────────────────────

export async function fetchAssets(filters: AssetFilter): Promise<AssetListResponse> {
  const params: Record<string, string | number> = {
    page: filters.page ?? 1,
    pageSize: filters.pageSize ?? 24,
  };
  if (filters.search) params.search = filters.search;
  if (filters.type !== "all") params.type = filters.type;
  if (filters.complianceTier !== "all") params.complianceTier = filters.complianceTier;
  if (filters.deploymentMode !== "all") params.deploymentMode = filters.deploymentMode;
  if (filters.tags.length > 0) params.tags = filters.tags.join(",");

  const { data } = await api.get<AssetListResponse>("/assets", { params });
  return data;
}

export async function fetchAsset(id: string): Promise<Asset> {
  const { data } = await api.get<Asset>(`/assets/${id}`);
  return data;
}

export async function addAssetToWorkspace(assetId: string, projectId: string): Promise<void> {
  await api.post(`/projects/${projectId}/assets`, { assetId });
}

export default api;
