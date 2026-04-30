import { Button } from "@/components/ui/button";
import { Pause, Play, Zap, RotateCcw } from "lucide-react";
import { useBurst, usePause, useResetDaily, useResume } from "@/lib/queries";
import { useLiveStore } from "@/lib/store";

export function ControlButtons() {
  const status = useLiveStore((s) => s.status);
  const pause = usePause();
  const resume = useResume();
  const burst = useBurst();
  const reset = useResetDaily();

  return (
    <div className="flex items-center gap-2">
      {status === "paused" ? (
        <Button size="sm" onClick={() => resume.mutate()} disabled={resume.isPending}>
          <Play className="mr-1 h-4 w-4" /> Resume
        </Button>
      ) : (
        <Button size="sm" variant="outline" onClick={() => pause.mutate()} disabled={pause.isPending}>
          <Pause className="mr-1 h-4 w-4" /> Pause
        </Button>
      )}
      <Button size="sm" variant="outline" onClick={() => burst.mutate(30)} disabled={burst.isPending}>
        <Zap className="mr-1 h-4 w-4" /> Burst 30m
      </Button>
      {status === "quota_reached" && (
        <Button size="sm" variant="outline" onClick={() => reset.mutate()} disabled={reset.isPending}>
          <RotateCcw className="mr-1 h-4 w-4" /> Reset daily
        </Button>
      )}
    </div>
  );
}
