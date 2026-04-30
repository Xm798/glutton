import { useState } from "react";
import { Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useBurst } from "@/lib/queries";

export function BurstDialog() {
  const [open, setOpen] = useState(false);
  const [minutes, setMinutes] = useState<string>("30");
  const [volumeMB, setVolumeMB] = useState<string>("");
  const burst = useBurst();

  const parse = (s: string) => (s.trim() === "" ? 0 : Math.max(0, Number(s)));
  const m = parse(minutes);
  const mb = parse(volumeMB);
  const valid = m > 0 || mb > 0;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) return;
    await burst.mutateAsync({
      minutes: m,
      bytes: mb > 0 ? Math.round(mb * 1024 * 1024) : 0,
    });
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          <Zap className="mr-1 h-4 w-4" /> Burst…
        </Button>
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>Manual burst</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 py-4">
            <p className="text-sm text-muted-foreground">
              Force-run regardless of the cron window. Burst ends when either cap is reached.
              Leave a field blank to skip that cap. Daily/monthly quotas still apply.
            </p>
            <div className="grid gap-1">
              <Label htmlFor="burst-minutes">Duration (minutes)</Label>
              <Input
                id="burst-minutes"
                type="number"
                min={0}
                placeholder="e.g. 30"
                value={minutes}
                onChange={(e) => setMinutes(e.target.value)}
              />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="burst-volume">Total volume (MB)</Label>
              <Input
                id="burst-volume"
                type="number"
                min={0}
                placeholder="optional"
                value={volumeMB}
                onChange={(e) => setVolumeMB(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={burst.isPending || !valid}>
              Start burst
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
