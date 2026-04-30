import { useTranslation } from "react-i18next";
import { EventList } from "@/components/events/event-list";

export default function EventsPage() {
  const { t } = useTranslation();
  return (
    <div className="p-6">
      <h2 className="mb-1 text-xl font-semibold">{t("events.title")}</h2>
      <p className="mb-4 text-sm text-muted-foreground">{t("events.intro")}</p>
      <EventList />
    </div>
  );
}
