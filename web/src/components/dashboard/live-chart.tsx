import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipProps,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useLiveStats, useTrafficSeries } from "@/lib/queries";
import { cn, formatBytes, formatRate } from "@/lib/utils";
import type { TrafficRange } from "@/types/api";

// Unifi-style blue accent; reads well over both light and dark cards.
const ACCENT = "#2f9bff";
const RANGES: TrafficRange[] = ["1h", "1d", "1w", "1m"];
const RANGE_LABELS: Record<TrafficRange, string> = {
  "1h": "1h",
  "1d": "1D",
  "1w": "1W",
  "1m": "1M",
};

type Metric = "rate" | "bytes";

interface Row {
  t: number;
  value: number;
}

function tickFormatter(range: TrafficRange) {
  return (v: number) => {
    const d = new Date(v * 1000);
    if (range === "1h" || range === "1d") {
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    }
    return d.toLocaleDateString([], { month: "2-digit", day: "2-digit" });
  };
}

function segmentClass(active: boolean) {
  return cn(
    "rounded-md px-2 py-1 font-medium transition-colors",
    active ? "bg-muted text-foreground" : "text-muted-foreground hover:text-foreground",
  );
}

// Defined at module level so it is never recreated during render.
// Uses `metric` prop to select the correct formatter.
function ValueTooltip({
  active,
  payload,
  metric,
}: TooltipProps<number, string> & { metric: Metric }) {
  if (!active || !payload?.length) return null;
  const p = payload[0];
  if (!p) return null;
  const ts = (p.payload as Row).t;
  const fmt = metric === "rate" ? formatRate : formatBytes;
  return (
    <div className="rounded-md border bg-popover/95 px-3 py-2 text-xs shadow-md backdrop-blur">
      <div className="font-medium tabular-nums" style={{ color: ACCENT }}>
        {fmt(Number(p.value))}
      </div>
      <div className="mt-0.5 text-muted-foreground">
        {new Date(ts * 1000).toLocaleString()}
      </div>
    </div>
  );
}

export function LiveChart() {
  const { t } = useTranslation();
  const [range, setRange] = useState<TrafficRange>("1d");
  const [metric, setMetric] = useState<Metric>("rate");
  const live = useLiveStats();
  const series = useTrafficSeries(range);

  const step = series.data?.step ?? 1;
  const rows: Row[] =
    series.data?.points.map((p) => ({
      t: p.t,
      value: metric === "rate" ? p.bytes / step : p.bytes,
    })) ?? [];

  const fmt = metric === "rate" ? formatRate : formatBytes;
  const current = live.data?.current_rate_bps ?? 0;

  return (
    <Card className="col-span-full">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0 pb-2">
        <div className="flex items-center gap-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("dashboard.rateChartTitle")}
          </CardTitle>
          <div className="inline-flex rounded-lg border p-0.5 text-xs">
            {(["rate", "bytes"] as Metric[]).map((m) => (
              <button
                key={m}
                type="button"
                aria-pressed={metric === m}
                onClick={() => setMetric(m)}
                className={segmentClass(metric === m)}
              >
                {t(`dashboard.metric.${m}`)}
              </button>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-2xl font-semibold tabular-nums" style={{ color: ACCENT }}>
            {formatRate(current)}
          </div>
          <div className="inline-flex rounded-lg border p-0.5 text-xs">
            {RANGES.map((r) => (
              <button
                key={r}
                type="button"
                aria-pressed={range === r}
                onClick={() => setRange(r)}
                className={cn(segmentClass(range === r), "tabular-nums")}
              >
                {RANGE_LABELS[r]}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className="h-72 pl-0">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={rows} margin={{ top: 8, right: 12, bottom: 0, left: 8 }}>
            <defs>
              <linearGradient id="rateFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={ACCENT} stopOpacity={0.35} />
                <stop offset="100%" stopColor={ACCENT} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid
              vertical={false}
              stroke="currentColor"
              strokeOpacity={0.12}
              strokeDasharray="3 3"
            />
            <XAxis
              dataKey="t"
              type="number"
              domain={["dataMin", "dataMax"]}
              tickLine={false}
              axisLine={false}
              minTickGap={48}
              tick={{ fontSize: 11, fill: "currentColor", opacity: 0.55 }}
              tickFormatter={tickFormatter(range)}
            />
            <YAxis
              width={64}
              tickLine={false}
              axisLine={false}
              tick={{ fontSize: 11, fill: "currentColor", opacity: 0.55 }}
              tickFormatter={(v) => fmt(v)}
            />
            <Tooltip
              content={<ValueTooltip metric={metric} />}
              cursor={{ stroke: ACCENT, strokeOpacity: 0.4, strokeWidth: 1 }}
            />
            <Area
              type="monotone"
              dataKey="value"
              stroke={ACCENT}
              strokeWidth={2}
              fill="url(#rateFill)"
              dot={false}
              activeDot={{ r: 3, fill: ACCENT, stroke: "var(--background)", strokeWidth: 2 }}
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  );
}
