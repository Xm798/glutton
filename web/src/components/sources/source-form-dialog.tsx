import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useCreateSource, useUpdateSource } from "@/lib/queries";
import type { Source, SourceInput } from "@/types/api";

interface Props {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  editing: Source | null;
}

const empty: SourceInput = { name: "", url: "", ua: "", weight: 1, enabled: true };

export function SourceFormDialog({ open, onOpenChange, editing }: Props) {
  const [form, setForm] = useState<SourceInput>(empty);
  const createMut = useCreateSource();
  const updateMut = useUpdateSource();

  useEffect(() => {
    if (editing) {
      setForm({
        name: editing.Name,
        url: editing.URL,
        ua: editing.UA,
        enabled: editing.Enabled,
        weight: editing.Weight,
      });
    } else {
      setForm(empty);
    }
  }, [editing, open]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (editing) {
      await updateMut.mutateAsync({ id: editing.ID, s: form });
    } else {
      await createMut.mutateAsync(form);
    }
    onOpenChange(false);
  };

  const pending = createMut.isPending || updateMut.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{editing ? "Edit source" : "Add source"}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-4">
            <div className="grid gap-1">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="url">URL</Label>
              <Input
                id="url"
                type="url"
                value={form.url}
                onChange={(e) => setForm({ ...form, url: e.target.value })}
                required
              />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="ua">User-Agent (optional)</Label>
              <Input
                id="ua"
                value={form.ua ?? ""}
                onChange={(e) => setForm({ ...form, ua: e.target.value })}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1">
                <Label htmlFor="weight">Weight</Label>
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
                <Label htmlFor="enabled">Enabled</Label>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={pending}>
              {editing ? "Save" : "Add"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
