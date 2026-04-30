import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useTrafficBySource } from "@/lib/queries";
import { formatBytes } from "@/lib/utils";

export function TopSources({ since }: { since: number }) {
  const { t } = useTranslation();
  const { data } = useTrafficBySource(since);
  const top = (data ?? []).slice(0, 5);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("dashboard.topSources")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("dashboard.source")}</TableHead>
              <TableHead className="text-right">{t("dashboard.bytes")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {top.map((s) => (
              <TableRow key={s.id}>
                <TableCell>{s.name}</TableCell>
                <TableCell className="text-right tabular-nums">{formatBytes(s.bytes)}</TableCell>
              </TableRow>
            ))}
            {top.length === 0 && (
              <TableRow>
                <TableCell colSpan={2} className="text-center text-muted-foreground">
                  {t("dashboard.noTraffic")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
