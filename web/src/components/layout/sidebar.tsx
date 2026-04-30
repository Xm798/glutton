import { NavLink } from "react-router";
import { LayoutDashboard, Database, Settings, ScrollText } from "lucide-react";
import { cn } from "@/lib/utils";

const items = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/sources", label: "Sources", icon: Database, end: false },
  { to: "/settings", label: "Settings", icon: Settings, end: false },
  { to: "/events", label: "Events", icon: ScrollText, end: false },
];

export function Sidebar() {
  return (
    <nav className="flex h-full w-56 flex-col border-r bg-card p-3">
      <div className="px-3 py-4 text-xl font-semibold">Glutton</div>
      <ul className="space-y-1">
        {items.map(({ to, label, icon: Icon, end }) => (
          <li key={to}>
            <NavLink
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm",
                  isActive ? "bg-muted font-medium" : "text-muted-foreground hover:bg-muted/50",
                )
              }
            >
              <Icon className="h-4 w-4" />
              {label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
