import { EventList } from "@/components/events/event-list";

export default function EventsPage() {
  return (
    <div className="p-6">
      <h2 className="mb-1 text-xl font-semibold">Events</h2>
      <p className="mb-4 text-sm text-muted-foreground">
        Live tail of the last 100 events received via SSE.
      </p>
      <EventList />
    </div>
  );
}
