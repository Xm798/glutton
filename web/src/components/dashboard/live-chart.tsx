import { useEffect, useRef, useState } from "react";
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
import { useLiveStats } from "@/lib/queries";
import { formatRate } from "@/lib/utils";

interface Point {
  t: number; // unix seconds
  rate: number; // bytes/s
}

const WINDOW_SECONDS = 3600; // last 60 min
// Unifi-style blue accent; reads well over both light and dark cards.
const ACCENT = "#2f9bff";

function RateTooltip({ active, payload }: TooltipProps<number, string>) {
  if (!active || !payload?.length) return null;
  const p = payload[0];
  if (!p) return null;
  const t = (p.payload as Point).t;
  return (
    <div className="rounded-md border bg-popover/95 px-3 py-2 text-xs shadow-md backdrop-blur">
      <div className="font-medium tabular-nums" style={{ color: ACCENT }}>
        {formatRate(Number(p.value))}
      </div>
      <div className="mt-0.5 text-muted-foreground">
        {new Date(t * 1000).toLocaleTimeString()}
      </div>
    </div>
  );
}

export function LiveChart() {
  const { t } = useTranslation();
  const live = useLiveStats();
  const [points, setPoints] = useState<Point[]>([]);
  const lastUpdated = useRef<number>(0);

  useEffect(() => {
    const data = live.data;
    if (!data) return;
    if (data.updated_at === lastUpdated.current) return;
    lastUpdated.current = data.updated_at;
    setPoints((prev) => {
      const cutoff = Math.floor(Date.now() / 1000) - WINDOW_SECONDS;
      const next = [
        ...prev,
        { t: data.updated_at || Math.floor(Date.now() / 1000), rate: data.current_rate_bps },
      ];
      return next.filter((p) => p.t >= cutoff);
    });
  }, [live.data]);

  const current = live.data?.current_rate_bps ?? 0;

  return (
    <Card className="col-span-full">
      <CardHeader className="flex flex-row items-end justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.rateChartTitle")}
        </CardTitle>
        <div className="text-2xl font-semibold tabular-nums" style={{ color: ACCENT }}>
          {formatRate(current)}
        </div>
      </CardHeader>
      <CardContent className="h-72 pl-0">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: 8 }}>
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
              tickFormatter={(v) =>
                new Date(v * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
              }
            />
            <YAxis
              width={64}
              tickLine={false}
              axisLine={false}
              tick={{ fontSize: 11, fill: "currentColor", opacity: 0.55 }}
              tickFormatter={(v) => formatRate(v)}
            />
            <Tooltip
              content={<RateTooltip />}
              cursor={{ stroke: ACCENT, strokeOpacity: 0.4, strokeWidth: 1 }}
            />
            <Area
              type="monotone"
              dataKey="rate"
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
