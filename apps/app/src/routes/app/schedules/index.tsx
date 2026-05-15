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
import { useQuery } from "@tanstack/react-query";
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
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import ScheduleDetailSheet from "@/components/dashboard/schedule-detail-sheet";
import { createScheduleColumns } from "@/components/tables/schedules-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import {
  schedulesQueryOptions,
  usePauseSchedule,
  useResumeSchedule,
  useTriggerSchedule,
} from "@/hooks/api/use-schedules";
import {
  CalendarIcon,
  EyeIcon,
  FilterIcon,
  PauseActionIcon,
  PlayActionIcon,
  SearchIcon,
} from "@/lib/icons";
import { ENABLED_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(20),
});

export const Route = createFileRoute("/app/schedules/")({
  head: () => ({ meta: [{ title: "Schedules · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(schedulesQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: SchedulesPage,
});

function SchedulesPage() {
  usePageEvent("schedules_list_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [selectedSchedule, setSelectedSchedule] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const triggerSchedule = useTriggerSchedule();
  const pauseSchedule = usePauseSchedule();
  const resumeSchedule = useResumeSchedule();

  const { data } = useQuery({
    ...schedulesQueryOptions(),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const filteredData = useMemo(() => {
    const typed = data as PaginatedResponse<Job> | undefined;
    const jobs = hasProject ? (typed?.data ?? []) : [];
    if (selectedStatuses.length === 0) {
      return jobs;
    }
    return jobs.filter((job: Job) => {
      if (selectedStatuses.includes("Enabled") && job.enabled) {
        return true;
      }
      if (selectedStatuses.includes("Disabled") && !job.enabled) {
        return true;
      }
      return false;
    });
  }, [data, selectedStatuses, hasProject]);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const table = useReactTable({
    data: filteredData,
    columns: createScheduleColumns({
      onView: (schedule) => {
        setSelectedSchedule(schedule);
        setSheetOpen(true);
      },
      onTrigger: (schedule) => triggerSchedule.mutate({ id: schedule.id }),
      onPauseResume: (schedule) => {
        if (schedule.enabled) {
          pauseSchedule.mutate({ id: schedule.id });
        } else {
          resumeSchedule.mutate({ id: schedule.id });
        }
      },
    }),
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

  const summary = useMemo(() => {
    let enabled = 0;
    let paused = 0;
    for (const job of filteredData) {
      if (job.enabled) {
        enabled++;
      } else {
        paused++;
      }
    }
    return { enabled, paused };
  }, [filteredData]);

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

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No schedules yet. Add a cron expression to a job to create a schedule."
      hideButton
      icon={
        <HugeiconsIcon className="size-6 text-foreground" icon={CalendarIcon} />
      }
      title="No schedules found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Schedules</h1>
      {filteredData.length > 0 && (
        <div className="flex flex-wrap items-center gap-4 pb-3 text-sm">
          <span className="text-muted-foreground">
            {filteredData.length} schedule
            {filteredData.length === 1 ? "" : "s"}
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-success" />
            <span className="tabular-nums">{summary.enabled}</span>
            <span className="text-muted-foreground">enabled</span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-muted-foreground/40" />
            <span className="tabular-nums">{summary.paused}</span>
            <span className="text-muted-foreground">paused</span>
          </span>
        </div>
      )}

      <div className="flex items-center gap-3 pb-2.5">
        <div className="relative w-full max-w-[500px]">
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
            {ENABLED_STATUS_OPTIONS.map((status) => (
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
          ariaLabel="Schedules"
          emptyState={emptyState}
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
                    for (const id of selectedIds) {
                      triggerSchedule.mutate({ id });
                    }
                  },
                },
                {
                  label: "Pause",
                  icon: PauseActionIcon,
                  onClick: () => {
                    for (const id of selectedIds) {
                      pauseSchedule.mutate({ id });
                    }
                  },
                },
                {
                  label: "Resume",
                  icon: PlayActionIcon,
                  onClick: () => {
                    for (const id of selectedIds) {
                      resumeSchedule.mutate({ id });
                    }
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
