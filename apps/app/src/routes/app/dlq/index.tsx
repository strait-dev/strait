import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
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
import { createDlqColumns } from "@/components/tables/dlq-columns";
import {
  getResourceTableInitialState,
  RESOURCE_TABLE_CLASS_NAMES,
  RESOURCE_TABLE_EMPTY_CLASS_NAME,
} from "@/components/tables/resource-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { JobRun, PaginatedResponse } from "@/hooks/api/types";
import {
  dlqQueryOptions,
  useBulkDiscardDlq,
  useBulkRetryDlq,
  useDiscardDlqItem,
  useRetryDlqItem,
} from "@/hooks/api/use-dlq";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useAppReactTable } from "@/hooks/use-app-react-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import { AlertIcon, RefreshIcon, SearchIcon, TrashIcon } from "@/lib/icons";
import { dlqResourcePermissions } from "@/lib/resource-permissions";
import { DLQ_ERROR_TYPES } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  errorType: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
});

const EMPTY_ARRAY: never[] = [];

export const Route = createFileRoute("/app/dlq/")({
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
        dlqQueryOptions({ limit: deps.limit, cursor: deps.cursor })
      );
    }
    return { hasProject, session };
  },
  head: () => ({ meta: [{ title: "Dead letter queue · Strait" }] }),
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: DlqPage,
});

function DlqPage() {
  usePageEvent("dlq_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const { data } = useQuery({
    ...dlqQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
    }),
    enabled: hasProject,
  });

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const bulkRetry = useBulkRetryDlq();
  const bulkDiscard = useBulkDiscardDlq();
  const retryDlqItem = useRetryDlqItem();
  const discardDlqItem = useDiscardDlqItem();
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const actionPermissions = dlqResourcePermissions(permissions);

  const typed = data as PaginatedResponse<JobRun> | undefined;
  const apiData = hasProject ? (typed?.data ?? EMPTY_ARRAY) : EMPTY_ARRAY;
  const selectedErrorTypes = search.errorType ?? EMPTY_ARRAY;

  const filteredData = (() => {
    let runs = apiData;
    const query = search.query?.trim().toLowerCase();
    if (query) {
      runs = runs.filter((run) =>
        [
          run.id,
          run.job_id,
          run.error,
          run.error_class,
          run.status,
          run.triggered_by,
        ]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(query))
      );
    }
    if (selectedErrorTypes.length === 0) {
      return runs;
    }
    return runs.filter((run) =>
      selectedErrorTypes.some((errorType) =>
        [run.error, run.error_class]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(errorType))
      )
    );
  })();
  const tableData = useHydratedTableData(filteredData);

  const table = useAppReactTable({
    data: tableData.data,
    columns: createDlqColumns({
      onView: (run) => {
        setSelectedRun(run);
        setSheetOpen(true);
      },
      onRetry: actionPermissions.canRetry
        ? (run) => retryDlqItem.mutate({ id: run.id })
        : undefined,
      onDiscard: actionPermissions.canDiscard
        ? (run) => discardDlqItem.mutate({ id: run.id })
        : undefined,
      disabled: !tableData.isHydrated,
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    initialState: getResourceTableInitialState(),
    onRowSelectionChange: setRowSelection,
    state: { rowSelection },
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );
  const hasSelection = selectedIds.length > 0;

  const handleBulkRetry = () => {
    if (selectedIds.length === 0) {
      return;
    }
    bulkRetry.mutate({ ids: selectedIds });
  };

  const handleBulkDiscard = () => {
    if (selectedIds.length === 0) {
      return;
    }
    bulkDiscard.mutate({ ids: selectedIds });
  };

  const allDlqIds = filteredData.map((r) => r.id);
  const handleRetryAll = () => {
    if (allDlqIds.length === 0) {
      return;
    }
    bulkRetry.mutate({ ids: allDlqIds });
  };

  function handleErrorTypeFiltersChange(errorTypes: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        errorType: errorTypes.length > 0 ? errorTypes : undefined,
        cursor: undefined,
      }),
    });
  }

  const totalCount = filteredData.length;

  const emptyState = hasProject ? (
    <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon className="size-6 text-foreground" icon={AlertIcon} />
        </EmptyMedia>
        <EmptyTitle>No dead letter items</EmptyTitle>
        <EmptyDescription>
          No dead letter items. Failed runs that exhaust retries will appear
          here.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Dead letter queue</h1>
      {/* Alert banner */}
      {totalCount > 0 && (
        <Alert variant="destructive">
          <HugeiconsIcon className="shrink-0" icon={AlertIcon} size={16} />
          <AlertDescription className="font-medium">
            {totalCount} failed run{totalCount === 1 ? "" : "s"} require
            attention
          </AlertDescription>
        </Alert>
      )}

      {/* Toolbar */}
      <div className="flex items-center gap-2 pb-2.5">
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
          placeholder="Search dead letters"
          value={search.query ?? ""}
        />

        <FacetedStatusFilter
          field="errorType"
          label="Error type"
          onChange={handleErrorTypeFiltersChange}
          options={DLQ_ERROR_TYPES.map((errorType) => ({
            label: errorType,
            value: errorType,
          }))}
          values={selectedErrorTypes}
        />

        {actionPermissions.canRetry && hasSelection ? (
          <>
            <Button
              disabled={bulkRetry.isPending}
              onClick={handleBulkRetry}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              Retry selected ({selectedIds.length})
            </Button>
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button
                    disabled={bulkDiscard.isPending}
                    variant="destructive"
                  >
                    <HugeiconsIcon
                      className="mr-1.5"
                      icon={TrashIcon}
                      size={14}
                    />
                    Discard selected ({selectedIds.length})
                  </Button>
                }
              />
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>
                    Discard {selectedIds.length} run
                    {selectedIds.length === 1 ? "" : "s"}?
                  </AlertDialogTitle>
                  <AlertDialogDescription>
                    Discarded runs are permanently removed from the dead letter
                    queue. This cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleBulkDiscard}>
                    Discard
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </>
        ) : (
          actionPermissions.canRetry &&
          totalCount > 0 && (
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button
                    className="ml-auto"
                    disabled={bulkRetry.isPending}
                    variant="outline"
                  >
                    <HugeiconsIcon
                      className="mr-1.5"
                      icon={RefreshIcon}
                      size={14}
                    />
                    Retry all ({totalCount})
                  </Button>
                }
              />
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>
                    Retry all {totalCount} dead letter run
                    {totalCount === 1 ? "" : "s"}?
                  </AlertDialogTitle>
                  <AlertDialogDescription>
                    Every run currently in the DLQ will be re-enqueued.
                    Long-failing jobs may simply fail again.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleRetryAll}>
                    Retry all
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )
        )}
      </div>

      {/* Table */}
      <section aria-label="Dead letter queue">
        <div className="pt-2">
          <DataGrid
            emptyMessage={emptyState}
            loading={tableData.isLoading}
            onRowClick={(run) => {
              setSelectedRun(run);
              setSheetOpen(true);
            }}
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
          </DataGrid>
        </div>
      </section>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
