import { useTranslation } from "react-i18next";
import { SettingsForm } from "@/components/settings/settings-form";

export default function SettingsPage() {
  const { t } = useTranslation();
  return (
    <div className="mx-auto max-w-3xl p-6">
      <h2 className="mb-4 text-xl font-semibold">{t("settings.title")}</h2>
      <p className="mb-6 text-sm text-muted-foreground">{t("settings.intro")}</p>
      <SettingsForm />
    </div>
  );
}
