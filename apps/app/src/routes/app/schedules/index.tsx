import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
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
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import PageHeader from "@/components/common/page-header";
import { ScheduleDetailSheet } from "@/components/dashboard/schedule-detail-sheet";
import { scheduleColumns } from "@/components/tables/schedules-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import { schedulesQueryOptions } from "@/hooks/api/use-schedules";
import {
  EyeIcon,
  FilterIcon,
  PauseActionIcon,
  PlayActionIcon,
  SearchIcon,
} from "@/lib/icons";

const STATUS_OPTIONS = ["Enabled", "Disabled"] as const;

const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(20),
});

export const Route = createFileRoute("/app/schedules/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(schedulesQueryOptions());
  },
  component: SchedulesPage,
});

function SchedulesPage() {
  const { data } = useSuspenseQuery(schedulesQueryOptions()) as {
    data: PaginatedResponse<Job>;
  };
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [selectedSchedule, setSelectedSchedule] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const selectedStatuses = search.status ?? [];

  const filteredData = useMemo(() => {
    const jobs = data?.data ?? [];
    if (selectedStatuses.length === 0) {
      return jobs;
    }
    return jobs.filter((job) => {
      if (selectedStatuses.includes("Enabled") && job.enabled) {
        return true;
      }
      if (selectedStatuses.includes("Disabled") && !job.enabled) {
        return true;
      }
      return false;
    });
  }, [data?.data, selectedStatuses]);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const table = useReactTable({
    data: filteredData,
    columns: scheduleColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

  function toggleStatus(status: string) {
    const current = new Set(selectedStatuses);
    if (current.has(status)) {
      current.delete(status);
    } else {
      current.add(status);
    }
    const arr = Array.from(current);
    navigate({
      search: (prev) => ({
        ...prev,
        status: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

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
            aria-label="Search"
            className="pl-9"
            onChange={(e) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: e.target.value || undefined,
                  page: 1,
                }),
              })
            }
            placeholder="Search schedules..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
            Status
            {selectedStatuses.length > 0 && (
              <Badge variant="default">{selectedStatuses.length}</Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {STATUS_OPTIONS.map((status) => (
              <DropdownMenuCheckboxItem
                checked={selectedStatuses.includes(status)}
                key={status}
                onCheckedChange={() => toggleStatus(status)}
              >
                {status}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const target = e.target as HTMLElement;
          if (target.closest("a, button")) {
            return;
          }
          const row = target.closest("tr[data-row-index]");
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
        <DataTable
          emptyState={<div>No schedules found</div>}
          floatingBar={
            <DataTableFloatingBar
              actions={[
                ...(selectedIds.length === 1
                  ? [
                      {
                        label: "View",
                        icon: EyeIcon,
                        onClick: () => {
                          const schedule = table
                            .getRowModel()
                            .rows.find(
                              (r) => r.id === selectedIds[0]
                            )?.original;
                          if (schedule) {
                            setSelectedSchedule(schedule);
                            setSheetOpen(true);
                          }
                        },
                      },
                    ]
                  : []),
                {
                  label: "Trigger",
                  icon: PlayActionIcon,
                  onClick: () => {
                    /* TODO */
                  },
                },
                {
                  label: "Pause",
                  icon: PauseActionIcon,
                  onClick: () => {
                    /* TODO */
                  },
                },
              ]}
              onClearSelection={() => setRowSelection({})}
              selectedCount={selectedIds.length}
            />
          }
          table={table}
        />
      </div>

      <ScheduleDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        schedule={selectedSchedule}
      />
    </Shell>
  );
}
