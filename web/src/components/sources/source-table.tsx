import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { useDeleteSource, useSources } from "@/lib/queries";
import { formatBytes } from "@/lib/utils";
import { Pencil, Trash2 } from "lucide-react";
import type { Source } from "@/types/api";

export function SourceTable({ onEdit }: { onEdit: (s: Source) => void }) {
  const { data, isLoading } = useSources();
  const del = useDeleteSource();

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>URL</TableHead>
          <TableHead className="text-right">Weight</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Avg speed</TableHead>
          <TableHead className="text-right">Success / Fail</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {isLoading && (
          <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">Loading…</TableCell></TableRow>
        )}
        {!isLoading && data?.length === 0 && (
          <TableRow><TableCell colSpan={7} className="text-center text-muted-foreground">No sources</TableCell></TableRow>
        )}
        {data?.map((s) => (
          <TableRow key={s.ID}>
            <TableCell className="font-medium">{s.Name}</TableCell>
            <TableCell className="max-w-xs truncate text-muted-foreground" title={s.URL}>{s.URL}</TableCell>
            <TableCell className="text-right tabular-nums">{s.Weight}</TableCell>
            <TableCell>
              {s.Enabled ? <Badge>Enabled</Badge> : <Badge variant="secondary">Disabled</Badge>}
            </TableCell>
            <TableCell className="text-right tabular-nums">{formatBytes(s.AvgSpeedBps)}/s</TableCell>
            <TableCell className="text-right tabular-nums">{s.SuccessCount} / {s.FailCount}</TableCell>
            <TableCell className="flex justify-end gap-1">
              <Button size="icon" variant="ghost" onClick={() => onEdit(s)}>
                <Pencil className="h-4 w-4" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                onClick={() => {
                  if (confirm(`Delete ${s.Name}?`)) del.mutate(s.ID);
                }}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
