import { useState, useMemo } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useLiveStore } from "@/lib/store";

const LEVEL_VARIANT: Record<string, string> = {
  info: "bg-muted text-muted-foreground",
  warn: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  error: "bg-red-500/15 text-red-700 dark:text-red-300",
};

export function EventList() {
  const events = useLiveStore((s) => s.recentEvents);
  const [filter, setFilter] = useState("");

  const filtered = useMemo(() => {
    if (!filter.trim()) return events;
    const f = filter.toLowerCase();
    return events.filter(
      (e) =>
        e.Type.toLowerCase().includes(f) ||
        e.Level.toLowerCase().includes(f) ||
        e.Message.toLowerCase().includes(f),
    );
  }, [events, filter]);

  return (
    <div className="space-y-3">
      <Input
        placeholder="Filter by type, level, or message…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      <Card className="divide-y">
        {filtered.length === 0 && (
          <div className="p-6 text-center text-sm text-muted-foreground">
            No events received yet.
          </div>
        )}
        {filtered.map((e, i) => (
          <div key={i} className="flex items-start gap-3 px-4 py-3">
            <Badge className={LEVEL_VARIANT[e.Level] ?? ""}>{e.Level}</Badge>
            <div className="flex-1">
              <div className="flex items-baseline justify-between">
                <span className="font-medium">{e.Type}</span>
                <span className="text-xs text-muted-foreground">
                  {new Date(e.TS).toLocaleTimeString()}
                </span>
              </div>
              <div className="text-sm text-muted-foreground">{e.Message}</div>
            </div>
          </div>
        ))}
      </Card>
    </div>
  );
}
