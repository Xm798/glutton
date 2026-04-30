import { BrowserRouter, Route, Routes } from "react-router";
import { Shell } from "@/components/layout/shell";
import DashboardPage from "@/pages/dashboard-page";
import SourcesPage from "@/pages/sources-page";
import SettingsPage from "@/pages/settings-page";
import EventsPage from "@/pages/events-page";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Shell />}>
          <Route index element={<DashboardPage />} />
          <Route path="/sources" element={<SourcesPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/events" element={<EventsPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
