import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useLiveStats } from "@/lib/queries";
import { formatRate } from "@/lib/utils";

interface Point {
  t: number;     // unix seconds
  rate: number;  // bytes/s
}

const WINDOW_SECONDS = 3600; // last 60 min

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
      const next = [...prev, { t: data.updated_at || Math.floor(Date.now() / 1000), rate: data.current_rate_bps }];
      return next.filter((p) => p.t >= cutoff);
    });
  }, [live.data]);

  return (
    <Card className="col-span-full">
      <CardHeader>
        <CardTitle>{t("dashboard.rateChartTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="h-72">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={points}>
            <XAxis
              dataKey="t"
              type="number"
              domain={["dataMin", "dataMax"]}
              tickFormatter={(t) => new Date(t * 1000).toLocaleTimeString()}
            />
            <YAxis tickFormatter={(v) => formatRate(v)} width={80} />
            <Tooltip
              labelFormatter={(t) => new Date((t as number) * 1000).toLocaleTimeString()}
              formatter={(v) => formatRate(v as number)}
            />
            <Line type="monotone" dataKey="rate" stroke="currentColor" dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  );
}
