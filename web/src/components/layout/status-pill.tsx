import { Badge } from "@/components/ui/badge";
import { useLiveStore } from "@/lib/store";

const VARIANT: Record<string, string> = {
  idle: "bg-muted text-muted-foreground",
  running: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  paused: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  quota_reached: "bg-red-500/15 text-red-700 dark:text-red-300",
};

const LABEL: Record<string, string> = {
  idle: "Idle",
  running: "Running",
  paused: "Paused",
  quota_reached: "Quota reached",
};

export function StatusPill() {
  const status = useLiveStore((s) => s.status);
  const connected = useLiveStore((s) => s.connected);
  return (
    <div className="flex items-center gap-2">
      <Badge className={VARIANT[status] ?? ""}>{LABEL[status] ?? status}</Badge>
      <span
        className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-500" : "bg-muted-foreground"}`}
        title={connected ? "Live" : "Disconnected"}
      />
    </div>
  );
}
