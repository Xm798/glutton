import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { useLiveStore } from "@/lib/store";

const VARIANT: Record<string, string> = {
  idle: "bg-muted text-muted-foreground",
  running: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  paused: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  quota_reached: "bg-red-500/15 text-red-700 dark:text-red-300",
};

export function StatusPill() {
  const { t } = useTranslation();
  const status = useLiveStore((s) => s.status);
  const connected = useLiveStore((s) => s.connected);
  return (
    <div className="flex items-center gap-2">
      <Badge className={VARIANT[status] ?? ""}>{t(`status.${status}`, { defaultValue: status })}</Badge>
      <span
        className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-500" : "bg-muted-foreground"}`}
        title={connected ? t("status.live") : t("status.disconnected")}
      />
    </div>
  );
}
