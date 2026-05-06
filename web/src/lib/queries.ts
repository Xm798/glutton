import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api, type EventHistoryQuery } from "./api";
import type { RuntimeConfig, SourceInput } from "@/types/api";

export const qk = {
  version: ["version"] as const,
  config: ["config"] as const,
  sources: ["sources"] as const,
  liveStats: ["stats", "live"] as const,
  trafficSince: (since: number) => ["stats", "history", since] as const,
  trafficBySource: (since: number) => ["stats", "sources", since] as const,
  eventHistory: (q: EventHistoryQuery) => ["events", "history", q] as const,
  controlStatus: ["control", "status"] as const,
  eventTypes: ["events", "types"] as const,
};

export const useVersion = () => useQuery({ queryKey: qk.version, queryFn: api.version });

export const useConfig = () =>
  useQuery({ queryKey: qk.config, queryFn: api.getConfig });

export const useSources = () =>
  useQuery({ queryKey: qk.sources, queryFn: api.listSources });

export const useLiveStats = () =>
  useQuery({
    queryKey: qk.liveStats,
    queryFn: api.liveStats,
    refetchInterval: 2_000,
  });

export const useTrafficSince = (since: number) =>
  useQuery({
    queryKey: qk.trafficSince(since),
    queryFn: () => api.trafficSince(since),
    refetchInterval: 30_000,
  });

export const useTrafficBySource = (since: number) =>
  useQuery({
    queryKey: qk.trafficBySource(since),
    queryFn: () => api.trafficBySource(since),
    refetchInterval: 30_000,
  });

export const useEventHistory = (q: EventHistoryQuery = { limit: 100 }) =>
  useQuery({
    queryKey: qk.eventHistory(q),
    queryFn: () => api.getEventHistory(q),
    refetchOnMount: "always",
    staleTime: 30_000,
  });

export const useEventTypes = () =>
  useQuery({
    queryKey: qk.eventTypes,
    queryFn: api.eventTypes,
    staleTime: 5 * 60_000,
  });

export const useControlStatus = () =>
  useQuery({
    queryKey: qk.controlStatus,
    queryFn: api.controlStatus,
    refetchInterval: 10_000,
  });

export const usePutConfig = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cfg: RuntimeConfig) => api.putConfig(cfg),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.config }),
  });
};

export const useCreateSource = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (s: SourceInput) => api.createSource(s),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.sources }),
  });
};

export const useUpdateSource = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, s }: { id: number; s: SourceInput }) => api.updateSource(id, s),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.sources }),
  });
};

export const useDeleteSource = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteSource(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.sources }),
  });
};

export const usePause = () => useMutation({ mutationFn: api.pause });
export const useResume = () => useMutation({ mutationFn: api.resume });
export const useBurst = () => useMutation({ mutationFn: api.burst });
export const useResetDaily = () => useMutation({ mutationFn: api.resetDaily });
