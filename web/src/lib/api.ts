import type {
  LiveStats,
  RuntimeConfig,
  Source,
  SourceInput,
  SourceTrafficSummary,
  TrafficBucket,
  VersionInfo,
} from "@/types/api";

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, body || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  version: () => request<VersionInfo>("/api/version"),

  getConfig: () => request<RuntimeConfig>("/api/config"),
  putConfig: (cfg: RuntimeConfig) =>
    request<void>("/api/config", { method: "PUT", body: JSON.stringify(cfg) }),

  listSources: () => request<Source[]>("/api/sources"),
  createSource: (s: SourceInput) =>
    request<Source>("/api/sources", { method: "POST", body: JSON.stringify(s) }),
  updateSource: (id: number, s: SourceInput) =>
    request<void>(`/api/sources/${id}`, { method: "PUT", body: JSON.stringify(s) }),
  deleteSource: (id: number) =>
    request<void>(`/api/sources/${id}`, { method: "DELETE" }),

  liveStats: () => request<LiveStats>("/api/stats/live"),
  trafficSince: (since: number) =>
    request<TrafficBucket[]>(`/api/stats/history?since=${since}`),
  trafficBySource: (since: number) =>
    request<SourceTrafficSummary[]>(`/api/stats/sources?since=${since}`),

  pause: () => request<void>("/api/control/pause", { method: "POST" }),
  resume: () => request<void>("/api/control/resume", { method: "POST" }),
  burst: (minutes: number) =>
    request<void>("/api/control/burst", {
      method: "POST",
      body: JSON.stringify({ minutes }),
    }),
  resetDaily: () => request<void>("/api/control/reset-daily", { method: "POST" }),
};

export { ApiError };
