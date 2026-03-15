import { HugeiconsIcon } from "@hugeicons/react";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useState } from "react";
import PageHeader from "@/components/common/page-header";
import { ScheduleDetailSheet } from "@/components/dashboard/schedule-detail-sheet";
import { scheduleColumns } from "@/components/tables/schedules-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import { schedulesQueryOptions } from "@/hooks/api/use-schedules";
import { SearchIcon } from "@/lib/icons";

export const Route = createFileRoute("/app/schedules/")({
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(schedulesQueryOptions());
  },
  component: SchedulesPage,
});

function SchedulesPage() {
  const { data } = useSuspenseQuery(schedulesQueryOptions()) as {
    data: PaginatedResponse<Job>;
  };
  const [globalFilter, setGlobalFilter] = useState("");
  const [selectedSchedule, setSelectedSchedule] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const table = useReactTable({
    data: data?.data ?? [],
    columns: scheduleColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    state: { globalFilter },
    onGlobalFilterChange: setGlobalFilter,
  });

  return (
    <Shell>
      <PageHeader
        text="Jobs with cron schedules for recurring execution."
        title="Schedules"
      />

      <div className="flex items-center gap-3 py-4">
        <div className="relative flex-1">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
            className="pl-9"
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder="Search schedules..."
            value={globalFilter}
          />
        </div>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const row = (e.target as HTMLElement).closest("tr[data-row-index]");
          if (!row) {
            return;
          }
          const idx = Number(row.getAttribute("data-row-index"));
          const schedule = table.getRowModel().rows[idx]?.original;
          if (schedule) {
            setSelectedSchedule(schedule);
            setSheetOpen(true);
          }
        }}
      >
        <DataTable emptyState={<div>No schedules found</div>} table={table} />
      </div>

      <ScheduleDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        schedule={selectedSchedule}
      />
    </Shell>
  );
}
