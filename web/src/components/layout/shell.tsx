import { useEffect } from "react";
import { Outlet } from "react-router";
import { Sidebar } from "./sidebar";
import { StatusPill } from "./status-pill";
import { ControlButtons } from "@/components/controls/control-buttons";
import { useEventStream } from "@/lib/sse";
import { useControlStatus } from "@/lib/queries";
import { useLiveStore } from "@/lib/store";

export function Shell() {
  useEventStream(); // single SSE connection for the whole app
  const { data: status } = useControlStatus();
  const setStatus = useLiveStore((s) => s.setStatus);

  // Seed and re-sync on every poll so a missed state_changed event (e.g. SSE
  // reconnect) doesn't leave the pill stuck on a stale value.
  useEffect(() => {
    if (status?.status) setStatus(status.status);
  }, [status?.status, setStatus]);

  return (
    <div className="flex h-screen">
      <Sidebar />
      <div className="flex flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b bg-card px-6">
          <StatusPill />
          <ControlButtons />
        </header>
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
