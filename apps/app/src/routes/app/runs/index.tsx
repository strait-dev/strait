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
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import StatusBadge from "@/components/dashboard/status-badge";
import { createRunColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { JobRun, PaginatedResponse, RunStatus } from "@/hooks/api/types";
import {
  runsQueryOptions,
  useCancelRun,
  useRetryRun,
} from "@/hooks/api/use-runs";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import {
  ActivityIcon,
  EyeIcon,
  FilterIcon,
  RefreshIcon,
  SearchIcon,
  XCircleIcon,
} from "@/lib/icons";
import { RUN_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  cursor: z.string().optional(),
  perPage: z.number().optional(),
});

export const Route = createFileRoute("/app/runs/")({
  head: () => ({ meta: [{ title: "Runs · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        runsQueryOptions({ limit: deps.limit, cursor: deps.cursor })
      );
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: RunsPage,
});

function RunsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  usePageEvent("runs_list_viewed", {
    has_query: !!search.query,
    has_status_filter: (search.status?.length ?? 0) > 0,
    status_filter_count: search.status?.length ?? 0,
  });
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const { data } = useQuery({
    ...runsQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
    }),
    enabled: hasProject,
  });
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const selectedStatuses = (search.status ?? []) as RunStatus[];

  const typed = data as PaginatedResponse<JobRun> | undefined;
  const tableData = hasProject ? (typed?.data ?? []) : [];

  const summary = useMemo(() => {
    let succeeded = 0;
    let failed = 0;
    let running = 0;
    for (const run of tableData) {
      if (run.status === "completed") {
        succeeded++;
      } else if (
        run.status === "failed" ||
        run.status === "timed_out" ||
        run.status === "crashed" ||
        run.status === "system_failed"
      ) {
        failed++;
      } else if (run.status === "executing" || run.status === "dequeued") {
        running++;
      }
    }
    const completed = succeeded + failed;
    const successRate =
      completed > 0 ? Math.round((succeeded / completed) * 100) : null;
    return { succeeded, failed, running, successRate };
  }, [tableData]);

  const table = useReactTable({
    data: tableData,
    columns: createRunColumns({
      onView: (run) => {
        setSelectedRun(run);
        setSheetOpen(true);
      },
      onRetry: (run) => retryRun.mutate({ run_id: run.id }),
      onCancel: (run) => cancelRun.mutate({ run_id: run.id }),
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({
          ...prev,
          query: query || undefined,
          cursor: undefined,
        }),
      }),
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

  function toggleStatus(status: RunStatus) {
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
        cursor: undefined,
      }),
    });
  }

  function handleRowClick(run: JobRun) {
    setSelectedRun(run);
    setSheetOpen(true);
  }

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No runs yet. Trigger a job to see run history."
      hideButton
      icon={
        <HugeiconsIcon className="size-6 text-foreground" icon={ActivityIcon} />
      }
      title="No runs found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Runs</h1>
      {tableData.length > 0 && (
        <div className="flex flex-wrap items-center gap-4 pb-3 text-sm">
          <span className="text-muted-foreground">
            {tableData.length} run{tableData.length === 1 ? "" : "s"}
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-success" />
            <span className="tabular-nums">{summary.succeeded}</span>
            <span className="text-muted-foreground">succeeded</span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-destructive" />
            <span className="tabular-nums">{summary.failed}</span>
            <span className="text-muted-foreground">failed</span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-info" />
            <span className="tabular-nums">{summary.running}</span>
            <span className="text-muted-foreground">running</span>
          </span>
          {summary.successRate !== null && (
            <span className="ml-auto text-muted-foreground">
              <span className="font-medium text-foreground tabular-nums">
                {summary.successRate}%
              </span>{" "}
              success rate
            </span>
          )}
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
                  cursor: undefined,
                }),
              })
            }
            placeholder="Search by run ID or job name..."
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
            {RUN_STATUS_OPTIONS.map((status) => (
              <DropdownMenuCheckboxItem
                checked={selectedStatuses.includes(status)}
                key={status}
                onCheckedChange={() => toggleStatus(status)}
              >
                <StatusBadge status={status} />
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
          const run = table.getRowModel().rows[idx]?.original;
          if (run) {
            handleRowClick(run);
          }
        }}
      >
        <DataTable
          ariaLabel="Runs"
          cursorPagination={{
            pageSize: pagination.perPage,
            hasMore: typed?.has_more ?? false,
            canGoBack: pagination.canGoBack,
            onNext: () => {
              if (typed?.next_cursor) {
                pagination.goNext(typed.next_cursor);
              }
            },
            onPrev: pagination.goPrev,
            onPageSizeChange: pagination.setPerPage,
          }}
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
                          const run = table
                            .getRowModel()
                            .rows.find(
                              (r) => r.id === selectedIds[0]
                            )?.original;
                          if (run) {
                            handleRowClick(run);
                          }
                        },
                      },
                    ]
                  : []),
                {
                  label: "Retry",
                  icon: RefreshIcon,
                  onClick: () => {
                    for (const id of selectedIds) {
                      retryRun.mutate({ run_id: id });
                    }
                  },
                },
                {
                  label: "Cancel",
                  icon: XCircleIcon,
                  onClick: () => {
                    for (const id of selectedIds) {
                      cancelRun.mutate({ run_id: id });
                    }
                  },
                  variant: "destructive" as const,
                },
              ]}
              onClearSelection={() => setRowSelection({})}
              selectedCount={selectedIds.length}
            />
          }
          table={table}
        />
      </div>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
