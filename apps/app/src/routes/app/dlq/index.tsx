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
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
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
import { useCallback, useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import { createDlqColumns } from "@/components/tables/dlq-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { JobRun, PaginatedResponse } from "@/hooks/api/types";
import {
  dlqQueryOptions,
  useBulkDiscardDlq,
  useBulkRetryDlq,
  useDiscardDlqItem,
  useRetryDlqItem,
} from "@/hooks/api/use-dlq";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import {
  AlertIcon,
  FilterIcon,
  RefreshIcon,
  SearchIcon,
  TrashIcon,
} from "@/lib/icons";
import { DLQ_ERROR_TYPES } from "@/lib/status";
import { stopInteractiveRowClick } from "@/lib/table-interactions";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  errorType: z.array(z.string()).optional(),
  cursor: z.string().optional(),
  perPage: z.number().optional(),
});

export const Route = createFileRoute("/app/dlq/")({
  head: () => ({ meta: [{ title: "Dead letter queue · Strait" }] }),
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

  const typed = data as PaginatedResponse<JobRun> | undefined;
  const tableData = hasProject ? (typed?.data ?? []) : [];

  const table = useReactTable({
    data: tableData,
    columns: createDlqColumns({
      onView: (run) => {
        setSelectedRun(run);
        setSheetOpen(true);
      },
      onRetry: (run) => retryDlqItem.mutate({ id: run.id }),
      onDiscard: (run) => discardDlqItem.mutate({ id: run.id }),
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { rowSelection },
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );
  const hasSelection = selectedIds.length > 0;

  const handleBulkRetry = useCallback(() => {
    if (selectedIds.length === 0) {
      return;
    }
    bulkRetry.mutate({ ids: selectedIds });
  }, [selectedIds, bulkRetry]);

  const handleBulkDiscard = useCallback(() => {
    if (selectedIds.length === 0) {
      return;
    }
    bulkDiscard.mutate({ ids: selectedIds });
  }, [selectedIds, bulkDiscard]);

  const allDlqIds = (typed?.data ?? []).map((r) => r.id);
  const handleRetryAll = useCallback(() => {
    if (allDlqIds.length === 0) {
      return;
    }
    bulkRetry.mutate({ ids: allDlqIds });
  }, [allDlqIds, bulkRetry]);

  const selectedErrorTypes = search.errorType ?? [];

  function toggleErrorType(errorType: string) {
    const current = new Set(selectedErrorTypes);
    if (current.has(errorType)) {
      current.delete(errorType);
    } else {
      current.add(errorType);
    }
    const arr = Array.from(current);
    navigate({
      search: (prev) => ({
        ...prev,
        errorType: arr.length > 0 ? arr : undefined,
        cursor: undefined,
      }),
    });
  }

  const totalCount = tableData.length;

  const emptyState = hasProject ? (
    <Empty className="h-[300px]">
      <EmptyHeader>
        <EmptyMedia size="lg" variant="icon">
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
            placeholder="Search by job, run ID, or error..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
            Error Type
            {selectedErrorTypes.length > 0 && (
              <Badge variant="default">{selectedErrorTypes.length}</Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {DLQ_ERROR_TYPES.map((errorType) => (
              <DropdownMenuCheckboxItem
                checked={selectedErrorTypes.includes(errorType)}
                key={errorType}
                onCheckedChange={() => toggleErrorType(errorType)}
              >
                <span className="capitalize">{errorType}</span>
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {hasSelection ? (
          <>
            <Button
              disabled={bulkRetry.isPending}
              onClick={handleBulkRetry}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              Retry Selected ({selectedIds.length})
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
                    Discard Selected ({selectedIds.length})
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
      <div onClickCapture={stopInteractiveRowClick}>
        <div className="pt-2">
          <DataGrid
            emptyMessage={emptyState}
            onRowClick={(run) => {
              setSelectedRun(run);
              setSheetOpen(true);
            }}
            recordCount={typed?.data.length ?? 0}
            table={table}
            tableClassNames={{ base: "min-w-[1200px]" }}
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
      </div>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
