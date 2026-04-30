import { Outlet } from "react-router";
import { Sidebar } from "./sidebar";
import { StatusPill } from "./status-pill";
import { ControlButtons } from "@/components/controls/control-buttons";
import { useEventStream } from "@/lib/sse";

export function Shell() {
  useEventStream(); // single SSE connection for the whole app
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
