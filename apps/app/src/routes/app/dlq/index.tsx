import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
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
import { useCallback, useState } from "react";
import { z } from "zod/v4";

import ErrorComponent from "@/components/common/error-component";
import { NoProjectState } from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import { TablePageSkeleton } from "@/components/common/table-page-skeleton";
import { RunDetailSheet } from "@/components/dashboard/run-detail-sheet";
import { dlqColumns } from "@/components/tables/dlq-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { JobRun, PaginatedResponse } from "@/hooks/api/types";
import {
  dlqQueryOptions,
  useBulkDiscardDlq,
  useBulkRetryDlq,
} from "@/hooks/api/use-dlq";
import {
  AlertIcon,
  FilterIcon,
  RefreshIcon,
  SearchIcon,
  TrashIcon,
} from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";

const ERROR_TYPE_OPTIONS = [
  "timeout",
  "connection",
  "validation",
  "internal",
] as const;

const searchSchema = z.object({
  query: z.string().optional(),
  errorType: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/dlq/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const session = (context as unknown as { session: { user: AuthUser } })
      .session;
    const hasProject = !!session?.user?.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(dlqQueryOptions());
    }
    return { hasProject };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: DlqPage,
});

function DlqPage() {
  const { hasProject } = Route.useLoaderData() as { hasProject: boolean };
  const { session } = Route.useRouteContext() as any;
  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  return <DlqPageContent />;
}

function DlqPageContent() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(dlqQueryOptions()) as {
    data: PaginatedResponse<JobRun>;
  };

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const bulkRetry = useBulkRetryDlq();
  const bulkDiscard = useBulkDiscardDlq();

  const table = useReactTable({
    data: data?.data ?? [],
    columns: dlqColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
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
        page: 1,
      }),
    });
  }

  const totalCount = data?.data?.length ?? 0;

  return (
    <Shell>
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
                  page: 1,
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
            {ERROR_TYPE_OPTIONS.map((errorType) => (
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

        {hasSelection && (
          <>
            <Button
              disabled={bulkRetry.isPending}
              onClick={handleBulkRetry}
              size="sm"
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              Retry Selected ({selectedIds.length})
            </Button>
            <Button
              disabled={bulkDiscard.isPending}
              onClick={handleBulkDiscard}
              size="sm"
              variant="destructive"
            >
              <HugeiconsIcon className="mr-1.5" icon={TrashIcon} size={14} />
              Discard Selected ({selectedIds.length})
            </Button>
          </>
        )}
      </div>

      {/* Table */}
      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const target = e.target as HTMLElement;
          if (target.closest("a, button, input[type=checkbox]")) {
            return;
          }
          const row = target.closest("tr[data-row-index]");
          if (!row) {
            return;
          }
          const idx = Number(row.getAttribute("data-row-index"));
          const run = table.getRowModel().rows[idx]?.original;
          if (run) {
            setSelectedRun(run);
            setSheetOpen(true);
          }
        }}
      >
        <div className="pt-2">
          <DataTable
            emptyState={
              <TableEmptyState
                description="No dead letter items. Failed runs that exhaust retries will appear here."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="size-6 text-foreground"
                    icon={AlertIcon}
                  />
                }
                title="No dead letter items"
              />
            }
            table={table}
          />
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
