import { create } from "zustand";
import type { SchedulerStatus, ServerEvent } from "@/types/api";

interface LiveState {
  status: SchedulerStatus;
  recentEvents: ServerEvent[]; // capped ring buffer (most-recent first)
  connected: boolean;
  setStatus: (s: SchedulerStatus) => void;
  setConnected: (c: boolean) => void;
  pushEvent: (e: ServerEvent) => void;
}

const MAX_RECENT = 100;

export const useLiveStore = create<LiveState>((set) => ({
  status: "idle",
  recentEvents: [],
  connected: false,
  setStatus: (status) => set({ status }),
  setConnected: (connected) => set({ connected }),
  pushEvent: (e) =>
    set((s) => {
      const next = [e, ...s.recentEvents].slice(0, MAX_RECENT);
      // Some event types double as state-change signals.
      let status = s.status;
      switch (e.Type) {
        case "service_started":
        case "service_stopped":
        case "daily_reset":
        case "monthly_reset":
          status = "idle";
          break;
        case "quota_reached_daily":
        case "quota_reached_monthly":
          status = "quota_reached";
          break;
      }
      return { recentEvents: next, status };
    }),
}));
