import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useCreateSource, useUpdateSource } from "@/lib/queries";
import type { Source, SourceInput } from "@/types/api";

interface Props {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  editing: Source | null;
}

const empty: SourceInput = { name: "", urls: [], ua: "", weight: 1, enabled: true };

function splitURLs(text: string): string[] {
  return text.split("\n").map((s) => s.trim()).filter(Boolean);
}

export function SourceFormDialog({ open, onOpenChange, editing }: Props) {
  const { t } = useTranslation();
  const [form, setForm] = useState<SourceInput>(empty);
  const [urlsText, setUrlsText] = useState("");
  const [urlError, setUrlError] = useState<string | null>(null);
  const createMut = useCreateSource();
  const updateMut = useUpdateSource();

  useEffect(() => {
    if (editing) {
      setForm({
        name: editing.Name,
        urls: [...editing.URLs],
        ua: editing.UA,
        enabled: editing.Enabled,
        weight: editing.Weight,
      });
      setUrlsText(editing.URLs.join("\n"));
      setUrlError(null);
    } else {
      setForm(empty);
      setUrlsText("");
      setUrlError(null);
    }
  }, [editing, open]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const urls = splitURLs(urlsText);
    if (urls.length === 0) {
      setUrlError(t("sources.urlsRequired"));
      return;
    }
    setUrlError(null);
    const payload: SourceInput = { ...form, urls };
    if (editing) {
      await updateMut.mutateAsync({ id: editing.ID, s: payload });
    } else {
      await createMut.mutateAsync(payload);
    }
    onOpenChange(false);
  };

  const pending = createMut.isPending || updateMut.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{editing ? t("sources.editSource") : t("sources.addSource")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-4">
            <div className="grid gap-1">
              <Label htmlFor="name">{t("sources.name")}</Label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="urls">{t("sources.urls")}</Label>
              <Textarea
                id="urls"
                rows={4}
                placeholder={t("sources.urlsPlaceholder")}
                value={urlsText}
                onChange={(e) => setUrlsText(e.target.value)}
                required
              />
              {urlError && <p className="text-sm text-destructive">{urlError}</p>}
            </div>
            <div className="grid gap-1">
              <Label htmlFor="ua">{t("sources.userAgentOptional")}</Label>
              <Input
                id="ua"
                value={form.ua ?? ""}
                onChange={(e) => setForm({ ...form, ua: e.target.value })}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1">
                <Label htmlFor="weight">{t("sources.weight")}</Label>
                <Input
                  id="weight"
                  type="number"
                  min={1}
                  value={form.weight}
                  onChange={(e) => setForm({ ...form, weight: Number(e.target.value) })}
                />
              </div>
              <div className="flex items-end gap-2">
                <Switch
                  id="enabled"
                  checked={form.enabled}
                  onCheckedChange={(v) => setForm({ ...form, enabled: v })}
                />
                <Label htmlFor="enabled">{t("sources.enabled")}</Label>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>
              {editing ? t("common.save") : t("common.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
