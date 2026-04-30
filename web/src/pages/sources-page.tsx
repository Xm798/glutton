import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { SourceTable } from "@/components/sources/source-table";
import { SourceFormDialog } from "@/components/sources/source-form-dialog";
import type { Source } from "@/types/api";

export default function SourcesPage() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Source | null>(null);
  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t("sources.title")}</h2>
        <Button onClick={() => { setEditing(null); setOpen(true); }}>
          <Plus className="mr-1 h-4 w-4" /> {t("sources.addSource")}
        </Button>
      </div>
      <SourceTable onEdit={(s) => { setEditing(s); setOpen(true); }} />
      <SourceFormDialog open={open} onOpenChange={setOpen} editing={editing} />
    </div>
  );
}
