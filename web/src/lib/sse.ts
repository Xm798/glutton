import { useEffect } from "react";
import { useLiveStore } from "./store";
import type { ServerEvent } from "@/types/api";

const RECONNECT_MIN_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;

// Event types the Go backend emits. EventSource fires onmessage only for
// unnamed events; named events need explicit listeners.
const KNOWN_TYPES = [
  "service_started",
  "service_stopped",
  "quota_reached_daily",
  "quota_reached_monthly",
  "sources_mass_failure",
  "source_error",
  "daily_reset",
  "monthly_reset",
  "config_updated",
  "source_created",
  "source_updated",
  "source_deleted",
];

export function useEventStream(path = "/api/events") {
  const setConnected = useLiveStore((s) => s.setConnected);
  const pushEvent = useLiveStore((s) => s.pushEvent);

  useEffect(() => {
    let es: EventSource | null = null;
    let cancelled = false;
    let backoff = RECONNECT_MIN_MS;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const handle = (msg: MessageEvent) => {
      try {
        const e = JSON.parse(msg.data) as ServerEvent;
        pushEvent(e);
      } catch {
        /* drop malformed */
      }
    };

    const connect = () => {
      if (cancelled) return;
      es = new EventSource(path);
      es.onopen = () => {
        setConnected(true);
        backoff = RECONNECT_MIN_MS;
      };
      es.onerror = () => {
        setConnected(false);
        es?.close();
        if (cancelled) return;
        retryTimer = setTimeout(connect, backoff);
        backoff = Math.min(backoff * 2, RECONNECT_MAX_MS);
      };
      es.onmessage = handle;
      for (const t of KNOWN_TYPES) {
        es.addEventListener(t, handle as EventListener);
      }
    };

    connect();
    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      es?.close();
      setConnected(false);
    };
  }, [path, setConnected, pushEvent]);
}
