import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { useDeleteSource, useSources } from "@/lib/queries";
import { formatBytes } from "@/lib/utils";
import { ChevronDown, ChevronRight, Pencil, Trash2 } from "lucide-react";
import type { Source } from "@/types/api";

export function SourceTable({ onEdit }: { onEdit: (s: Source) => void }) {
  const { t } = useTranslation();
  const { data, isLoading } = useSources();
  const del = useDeleteSource();
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const toggle = (id: number) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-8" />
          <TableHead>{t("sources.name")}</TableHead>
          <TableHead>{t("sources.urls")}</TableHead>
          <TableHead className="text-right">{t("sources.weight")}</TableHead>
          <TableHead>{t("sources.status")}</TableHead>
          <TableHead className="text-right">{t("sources.avgSpeed")}</TableHead>
          <TableHead className="text-right">{t("sources.successFail")}</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {isLoading && (
          <TableRow><TableCell colSpan={8} className="text-center text-muted-foreground">{t("common.loading")}</TableCell></TableRow>
        )}
        {!isLoading && data?.length === 0 && (
          <TableRow><TableCell colSpan={8} className="text-center text-muted-foreground">{t("sources.noSources")}</TableCell></TableRow>
        )}
        {data?.map((s) => {
          const isOpen = expanded.has(s.ID);
          return (
            <Fragment key={s.ID}>
              <TableRow>
                <TableCell>
                  <Button size="icon" variant="ghost" onClick={() => toggle(s.ID)} aria-label={t("sources.toggleUrls")}>
                    {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                  </Button>
                </TableCell>
                <TableCell className="font-medium">{s.Name}</TableCell>
                <TableCell className="text-muted-foreground">
                  {t("sources.urlCount", { count: s.URLs.length })}
                </TableCell>
                <TableCell className="text-right tabular-nums">{s.Weight}</TableCell>
                <TableCell>
                  {s.Enabled ? <Badge>{t("sources.enabled")}</Badge> : <Badge variant="secondary">{t("sources.disabled")}</Badge>}
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
                      if (confirm(t("common.confirmDelete", { name: s.Name }))) del.mutate(s.ID);
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
              {isOpen && (
                <TableRow>
                  <TableCell />
                  <TableCell colSpan={7}>
                    <ul className="space-y-1 text-sm text-muted-foreground">
                      {s.URLs.map((u, i) => (
                        <li key={i} className="truncate" title={u}>{u}</li>
                      ))}
                    </ul>
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          );
        })}
      </TableBody>
    </Table>
  );
}
