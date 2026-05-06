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

const VALID_STATUSES: SchedulerStatus[] = ["idle", "running", "paused", "quota_reached"];

export const useLiveStore = create<LiveState>((set) => ({
  status: "idle",
  recentEvents: [],
  connected: false,
  setStatus: (status) => set({ status }),
  setConnected: (connected) => set({ connected }),
  pushEvent: (e) =>
    set((s) => {
      const next = [e, ...s.recentEvents].slice(0, MAX_RECENT);
      let status = s.status;
      // Authoritative state-change events override anything else.
      if (e.Type === "state_changed") {
        const to = (e.Data?.to as string | undefined) ?? "";
        if ((VALID_STATUSES as string[]).includes(to)) {
          status = to as SchedulerStatus;
        }
      } else {
        switch (e.Type) {
          case "service_started":
          case "service_stopped":
          case "daily_reset":
          case "daily_reset_manual":
          case "monthly_reset":
            status = "idle";
            break;
          case "service_resumed":
            // Resumed could be either running or idle depending on whether
            // we were active before the pause. The next state_changed will
            // resolve it precisely; default to running for responsiveness.
            status = "running";
            break;
          case "service_paused":
            status = "paused";
            break;
          case "burst_started":
            status = "running";
            break;
          case "quota_reached_daily":
          case "quota_reached_monthly":
            status = "quota_reached";
            break;
        }
      }
      return { recentEvents: next, status };
    }),
}));
