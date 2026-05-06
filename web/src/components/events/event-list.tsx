import { useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useLiveStore } from "@/lib/store";
import { useEventHistory } from "@/lib/queries";
import type { ServerEvent } from "@/types/api";

const LEVEL_VARIANT: Record<string, string> = {
  info: "bg-muted text-muted-foreground",
  warn: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  error: "bg-red-500/15 text-red-700 dark:text-red-300",
};

// Merge SSE events (in-memory ring) with persisted history rows fetched via
// REST. Dedupe key is `id` — the bus's monotonic publish id, persisted into
// the DB row's event_id column. SSE wins on collision so its (richer) Data
// payload survives. Rows without an id (legacy persisted before bus IDs were
// introduced) fall back to a (TS-millis, Type, Message) tuple.
function mergeEvents(history: ServerEvent[], live: ServerEvent[]): ServerEvent[] {
  const seenIds = new Set<number>();
  const seenLegacy = new Set<string>();
  const out: ServerEvent[] = [];
  const push = (e: ServerEvent) => {
    if (e.id && e.id > 0) {
      if (seenIds.has(e.id)) return;
      seenIds.add(e.id);
    } else {
      const ms = new Date(e.TS).getTime();
      const key = `${ms}|${e.Type}|${e.Message}`;
      if (seenLegacy.has(key)) return;
      seenLegacy.add(key);
    }
    out.push(e);
  };
  // Live first → SSE wins on dedupe (more accurate Data payload).
  live.forEach(push);
  history.forEach(push);
  out.sort((a, b) => new Date(b.TS).getTime() - new Date(a.TS).getTime());
  return out;
}

export function EventList() {
  const { t } = useTranslation();
  const live = useLiveStore((s) => s.recentEvents);
  const { data: historyRows = [] } = useEventHistory({ limit: 200 });
  const [filter, setFilter] = useState("");

  const history: ServerEvent[] = useMemo(
    () =>
      historyRows.map((r) => ({
        id: r.id,
        TS: new Date(r.Ts * 1000).toISOString(),
        Type: r.Type,
        Level: r.Level,
        Message: r.Message,
      })),
    [historyRows],
  );

  const merged = useMemo(() => mergeEvents(history, live), [history, live]);

  const filtered = useMemo(() => {
    if (!filter.trim()) return merged;
    const f = filter.toLowerCase();
    return merged.filter(
      (e) =>
        e.Type.toLowerCase().includes(f) ||
        e.Level.toLowerCase().includes(f) ||
        e.Message.toLowerCase().includes(f),
    );
  }, [merged, filter]);

  return (
    <div className="space-y-3">
      <Input
        placeholder={t("events.filterPlaceholder")}
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      <Card className="divide-y">
        {filtered.length === 0 && (
          <div className="p-6 text-center text-sm text-muted-foreground">
            {t("events.empty")}
          </div>
        )}
        {filtered.map((e) => (
          <div
            key={e.id && e.id > 0 ? `id:${e.id}` : `${e.TS}-${e.Type}-${e.Message}`}
            className="flex items-start gap-3 px-4 py-3"
          >
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
