import { SettingsForm } from "@/components/settings/settings-form";

export default function SettingsPage() {
  return (
    <div className="mx-auto max-w-3xl p-6">
      <h2 className="mb-4 text-xl font-semibold">Settings</h2>
      <p className="mb-6 text-sm text-muted-foreground">
        Config is persisted to the database. Quota, rate, and schedule changes take effect on the next process restart.
      </p>
      <SettingsForm />
    </div>
  );
}
