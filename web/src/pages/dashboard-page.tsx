import { useTranslation } from "react-i18next";
import { LiveChart } from "@/components/dashboard/live-chart";
import { KpiCard } from "@/components/dashboard/kpi-card";
import { TopSources } from "@/components/dashboard/top-sources";
import { useLiveStats } from "@/lib/queries";
import { formatBytes, formatRate } from "@/lib/utils";

export default function DashboardPage() {
  const { t } = useTranslation();
  const { data } = useLiveStats();
  return (
    <div className="grid grid-cols-3 gap-4 p-6">
      <KpiCard title={t("dashboard.currentRate")} value={formatRate(data?.current_rate_bps ?? 0)} />
      <KpiCard title={t("dashboard.today")} value={formatBytes(data?.today_bytes ?? 0)} />
      <KpiCard title={t("dashboard.thisMonth")} value={formatBytes(data?.month_bytes ?? 0)} />
      <LiveChart />
      <TopSources since={0} />
    </div>
  );
}
