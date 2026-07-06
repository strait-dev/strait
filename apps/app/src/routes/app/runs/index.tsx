import { HugeiconsIcon } from "@hugeicons/react";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridSelectionBar,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { InputWithStartIcon } from "@strait/ui/components/input-with-start-icon";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import { FacetedStatusFilter } from "@/components/common/faceted-status-filter";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import {
  getResourceTableInitialState,
  RESOURCE_TABLE_CLASS_NAMES,
  RESOURCE_TABLE_EMPTY_CLASS_NAME,
} from "@/components/tables/resource-table";
import { createRunColumns } from "@/components/tables/runs-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { JobRun, PaginatedResponse, RunStatus } from "@/hooks/api/types";
import {
  runsQueryOptions,
  useCancelRun,
  useRetryRun,
} from "@/hooks/api/use-runs";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useAppReactTable } from "@/hooks/use-app-react-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  ActivityIcon,
  EyeIcon,
  RefreshIcon,
  SearchIcon,
  XCircleIcon,
} from "@/lib/icons";
import { runResourcePermissions } from "@/lib/resource-permissions";
import { seo } from "@/lib/seo";
import { RUN_STATUS_OPTIONS } from "@/lib/status";
import { stopInteractiveRowClick } from "@/lib/table-interactions";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  status: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
});

export const Route = createFileRoute("/app/runs/")({
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
  head: () => ({ meta: seo({ title: "Runs" }) }),
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: RunsPage,
});

const EMPTY_ARRAY: never[] = [];

function getRunSummary(runs: JobRun[]) {
  const totalRuns = runs.length;
  const succeeded = runs.filter((run) => run.status === "completed").length;
  const failed = runs.filter((run) =>
    ["failed", "crashed", "timed_out", "system_failed"].includes(run.status)
  ).length;
  const running = runs.filter((run) =>
    ["executing", "queued", "waiting"].includes(run.status)
  ).length;
  const successRate =
    totalRuns > 0 ? Math.round((succeeded / totalRuns) * 100) : 0;

  return { succeeded, failed, running, successRate };
}

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
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const actionPermissions = runResourcePermissions(permissions);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const selectedStatuses = (search.status ?? EMPTY_ARRAY) as RunStatus[];

  const typed = data as PaginatedResponse<JobRun> | undefined;
  const filteredData = (() => {
    let runs = hasProject ? (typed?.data ?? []) : [];
    const query = search.query?.trim().toLowerCase();
    if (query) {
      runs = runs.filter((run) =>
        [run.id, run.job_id, run.status, run.error, run.triggered_by]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(query))
      );
    }
    if (selectedStatuses.length === 0) {
      return runs;
    }
    return runs.filter((run) =>
      selectedStatuses.includes(run.status as RunStatus)
    );
  })();
  const tableData = useHydratedTableData(filteredData);

  const summary = getRunSummary(filteredData);

  const table = useAppReactTable({
    data: tableData.data,
    columns: createRunColumns({
      onView: (run) => {
        setSelectedRun(run);
        setSheetOpen(true);
      },
      onRetry: actionPermissions.canRetry
        ? (run) => retryRun.mutate({ run_id: run.id })
        : undefined,
      onCancel: actionPermissions.canCancel
        ? (run) => cancelRun.mutate({ run_id: run.id })
        : undefined,
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    initialState: getResourceTableInitialState(),
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

  function handleStatusFiltersChange(statuses: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        status: statuses.length > 0 ? statuses : undefined,
        cursor: undefined,
      }),
    });
  }

  function handleRowClick(run: JobRun) {
    setSelectedRun(run);
    setSheetOpen(true);
  }

  const emptyState = hasProject ? (
    <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="size-6 text-foreground"
            icon={ActivityIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No runs found</EmptyTitle>
        <EmptyDescription>
          No runs yet. Trigger a job to see run history.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Runs</h1>
      {filteredData.length > 0 && (
        <div className="flex flex-wrap items-center gap-4 pb-3 text-sm">
          <span className="text-muted-foreground">
            {filteredData.length} run{filteredData.length === 1 ? "" : "s"}
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="completed" />
            <span className="tabular-nums">{summary.succeeded}</span>
            <span className="text-muted-foreground">succeeded</span>
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="failed" />
            <span className="tabular-nums">{summary.failed}</span>
            <span className="text-muted-foreground">failed</span>
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="running" />
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
        <InputWithStartIcon
          aria-label="Search"
          containerClassName="w-full max-w-[500px]"
          icon={<HugeiconsIcon icon={SearchIcon} size={16} />}
          onChange={(e) =>
            navigate({
              search: (prev) => ({
                ...prev,
                query: e.target.value || undefined,
                cursor: undefined,
              }),
            })
          }
          placeholder="Search runs"
          value={search.query ?? ""}
        />

        <FacetedStatusFilter
          onChange={handleStatusFiltersChange}
          options={RUN_STATUS_OPTIONS.map((status) => ({
            label: status,
            value: status,
          }))}
          values={selectedStatuses}
        />
      </div>

      <section aria-label="Runs" onClickCapture={stopInteractiveRowClick}>
        <DataGrid
          emptyMessage={emptyState}
          loading={tableData.isLoading}
          onRowClick={handleRowClick}
          recordCount={
            tableData.isHydrated ? table.getRowModel().rows.length : 0
          }
          table={table}
          tableClassNames={RESOURCE_TABLE_CLASS_NAMES}
        >
          <DataGridContainer>
            <DataGridScrollArea>
              <DataGridTable />
            </DataGridScrollArea>
          </DataGridContainer>
          <CursorPagination
            cursor={{
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
            table={table}
          />
          <DataGridSelectionBar
            actions={[
              ...(selectedIds.length === 1
                ? [
                    {
                      label: "View",
                      icon: EyeIcon,
                      onClick: () => {
                        const run = table
                          .getRowModel()
                          .rows.find((r) => r.id === selectedIds[0])?.original;
                        if (run) {
                          handleRowClick(run);
                        }
                      },
                    },
                  ]
                : []),
              ...(actionPermissions.canRetry || actionPermissions.canCancel
                ? [
                    ...(actionPermissions.canRetry
                      ? [
                          {
                            label: "Retry",
                            icon: RefreshIcon,
                            onClick: () => {
                              for (const id of selectedIds) {
                                retryRun.mutate({ run_id: id });
                              }
                            },
                          },
                        ]
                      : []),
                    ...(actionPermissions.canCancel
                      ? [
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
                        ]
                      : []),
                  ]
                : []),
            ]}
          />
        </DataGrid>
      </section>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
