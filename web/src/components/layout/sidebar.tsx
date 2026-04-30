import { NavLink } from "react-router";
import { LayoutDashboard, Database, Settings, ScrollText, Languages } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { SUPPORTED_LANGUAGES } from "@/i18n";

const items = [
  { to: "/", labelKey: "nav.dashboard", icon: LayoutDashboard, end: true },
  { to: "/sources", labelKey: "nav.sources", icon: Database, end: false },
  { to: "/settings", labelKey: "nav.settings", icon: Settings, end: false },
  { to: "/events", labelKey: "nav.events", icon: ScrollText, end: false },
] as const;

export function Sidebar() {
  const { t, i18n } = useTranslation();

  return (
    <nav className="flex h-full w-56 flex-col border-r bg-card p-3">
      <div className="px-3 py-4 text-xl font-semibold">{t("app.name")}</div>
      <ul className="space-y-1">
        {items.map(({ to, labelKey, icon: Icon, end }) => (
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
              {t(labelKey)}
            </NavLink>
          </li>
        ))}
      </ul>
      <div className="mt-auto px-3 py-2">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <Languages className="h-4 w-4" />
          <select
            aria-label={t("common.language")}
            className="flex-1 rounded-md border bg-background px-2 py-1 text-xs"
            value={i18n.resolvedLanguage ?? "en"}
            onChange={(e) => i18n.changeLanguage(e.target.value)}
          >
            {SUPPORTED_LANGUAGES.map((l) => (
              <option key={l.code} value={l.code}>
                {l.label}
              </option>
            ))}
          </select>
        </label>
      </div>
    </nav>
  );
}
