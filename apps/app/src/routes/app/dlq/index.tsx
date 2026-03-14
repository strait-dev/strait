import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { Input } from "@strait/ui/components/input.tsx";
import { Shell } from "@strait/ui/components/shell.tsx";
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
import PageHeader from "@/components/common/page-header.tsx";
import { RunDetailSheet } from "@/components/dashboard/run-detail-sheet.tsx";
import { dlqColumns } from "@/components/tables/dlq-columns.tsx";
import { DataTable } from "@/components/ui/data-table/data-table.tsx";
import type { JobRun, PaginatedResponse } from "@/hooks/api/types.ts";
import {
  dlqQueryOptions,
  useBulkDiscardDlq,
  useBulkRetryDlq,
} from "@/hooks/api/use-dlq.ts";
import { AlertIcon, RefreshIcon, SearchIcon, TrashIcon } from "@/lib/icons.ts";

const searchSchema = z.object({
  query: z.string().optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/dlq/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(dlqQueryOptions());
  },
  component: DlqPage,
});

function DlqPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    dlqQueryOptions({ query: search.query, page: search.page })
  ) as { data: PaginatedResponse<JobRun> };

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [selectedRun, _setSelectedRun] = useState<JobRun | null>(null);
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

  const totalCount = data?.total_count ?? 0;

  return (
    <Shell>
      <PageHeader
        text="Failed runs that have exhausted all retry attempts."
        title="Dead Letter Queue"
      />

      {/* Alert banner */}
      {totalCount > 0 && (
        <div className="flex items-center gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-destructive text-sm">
          <HugeiconsIcon className="shrink-0" icon={AlertIcon} size={16} />
          <span className="font-medium">
            {totalCount} failed run{totalCount === 1 ? "" : "s"} require
            attention
          </span>
        </div>
      )}

      {/* Toolbar */}
      <div className="flex items-center gap-2 pt-4">
        <div className="relative flex-1">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
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
      <div className="pt-2">
        <DataTable
          emptyState={
            <div className="py-12 text-center text-muted-foreground">
              No dead letter items found.
            </div>
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
