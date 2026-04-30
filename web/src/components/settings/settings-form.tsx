import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useConfig, usePutConfig } from "@/lib/queries";
import type { RuntimeConfig } from "@/types/api";

const empty: RuntimeConfig = {
  daily_quota_gb: 0,
  monthly_quota_gb: 0,
  max_rate_mbps: 10,
  max_concurrent: 4,
  time_windows: ["* 0-6 * * *"],
  default_ua: "",
  notifier_urls: [],
  subscribed_events: ["quota_reached_daily", "quota_reached_monthly", "sources_mass_failure"],
};

type LinesKey = "time_windows" | "notifier_urls" | "subscribed_events";
type NumKey = "daily_quota_gb" | "monthly_quota_gb" | "max_rate_mbps" | "max_concurrent";

export function SettingsForm() {
  const { data, isLoading } = useConfig();
  const put = usePutConfig();
  const [form, setForm] = useState<RuntimeConfig>(empty);

  useEffect(() => {
    if (data) setForm({ ...empty, ...data });
  }, [data]);

  if (isLoading) return <div className="text-muted-foreground">Loading…</div>;

  const linesField = (key: LinesKey, label: string, hint: string) => (
    <div className="grid gap-1">
      <Label htmlFor={key}>{label}</Label>
      <Textarea
        id={key}
        rows={3}
        value={(form[key] ?? []).join("\n")}
        onChange={(e) =>
          setForm({
            ...form,
            [key]: e.target.value.split("\n").map((s) => s.trim()).filter(Boolean),
          })
        }
      />
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );

  const numField = (key: NumKey, label: string, hint?: string) => (
    <div className="grid gap-1">
      <Label htmlFor={key}>{label}</Label>
      <Input
        id={key}
        type="number"
        min={0}
        value={form[key] ?? 0}
        onChange={(e) => setForm({ ...form, [key]: Number(e.target.value) })}
      />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault();
        put.mutate(form);
      }}
    >
      <Card>
        <CardHeader><CardTitle>Quotas & rate</CardTitle></CardHeader>
        <CardContent className="grid grid-cols-2 gap-4">
          {numField("daily_quota_gb", "Daily quota (GB)", "0 = unlimited")}
          {numField("monthly_quota_gb", "Monthly quota (GB)", "0 = unlimited")}
          {numField("max_rate_mbps", "Max rate (MB/s)", "Never zero — defaults to 10")}
          {numField("max_concurrent", "Max concurrent connections")}
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>Schedule</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          {linesField(
            "time_windows",
            "Time windows (5-field cron, one per line)",
            "Example: `* 0-6 * * *` — every minute of hours 0-6 (timezone follows TZ env var).",
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>HTTP</CardTitle></CardHeader>
        <CardContent className="grid gap-3">
          <div className="grid gap-1">
            <Label htmlFor="default_ua">Default User-Agent</Label>
            <Input
              id="default_ua"
              value={form.default_ua ?? ""}
              onChange={(e) => setForm({ ...form, default_ua: e.target.value })}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>Notifications</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          {linesField("notifier_urls", "Shoutrrr URLs (one per line)", "e.g. telegram://token@telegram?chats=123")}
          {linesField(
            "subscribed_events",
            "Subscribed event types (one per line)",
            "Examples: quota_reached_daily, quota_reached_monthly, sources_mass_failure, error_critical",
          )}
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button type="submit" disabled={put.isPending}>
          {put.isPending ? "Saving…" : "Save"}
        </Button>
      </div>
    </form>
  );
}
