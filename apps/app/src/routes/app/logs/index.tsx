import { HugeiconsIcon } from "@hugeicons/react";
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
import ExpandedEventDetail from "@/components/events/expanded-event-detail";
import { logColumns } from "@/components/events/log-columns";
import { RESOURCE_TABLE_EMPTY_CLASS_NAME } from "@/components/tables/resource-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { EventTrigger, PaginatedResponse } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import { useAppReactTable } from "@/hooks/use-app-react-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import { FileTextIcon, SearchIcon } from "@/lib/icons";
import { seo } from "@/lib/seo";
import { EVENT_STATUSES } from "@/lib/status";
import { stopInteractiveRowClick } from "@/lib/table-interactions";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  statuses: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
});

export const Route = createFileRoute("/app/logs/")({
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 50,
    cursor: search.cursor,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        eventsQueryOptions({ limit: deps.limit, cursor: deps.cursor })
      );
    }
    return { hasProject, session };
  },
  head: () => ({ meta: seo({ title: "Logs" }) }),
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: LogsPage,
});

const EMPTY_ARRAY: never[] = [];

function LogsPage() {
  usePageEvent("logs_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate,
    { defaultPerPage: 50 }
  );
  const { data } = useQuery({
    ...eventsQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
    }),
    enabled: hasProject,
  });

  const [expandedLogId, setExpandedLogId] = useState<string | null>(null);

  const selectedStatuses = (search.statuses ?? EMPTY_ARRAY) as string[];

  const typed = data as PaginatedResponse<EventTrigger> | undefined;

  const allLogs = hasProject ? (typed?.data ?? []) : [];
  const tableData = useHydratedTableData(allLogs);

  const table = useAppReactTable({
    data: tableData.data,
    columns: logColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    state: { globalFilter: search.query ?? "" },
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

  function handleStatusFiltersChange(statuses: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        statuses: statuses.length > 0 ? statuses : undefined,
        cursor: undefined,
      }),
    });
  }

  function handleRowClick(event: EventTrigger) {
    setExpandedLogId((prev) => (prev === event.id ? null : event.id));
  }

  const emptyState = hasProject ? (
    <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="size-6 text-muted-foreground"
            icon={FileTextIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No events found</EmptyTitle>
        <EmptyDescription>
          No log entries yet. Logs will appear as your jobs execute.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Logs</h1>
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
          placeholder="Search events"
          value={search.query ?? ""}
        />

        <FacetedStatusFilter
          onChange={handleStatusFiltersChange}
          options={EVENT_STATUSES.map((status) => ({
            label: status,
            value: status,
          }))}
          values={selectedStatuses}
        />
      </div>

      <section aria-label="Logs" onClickCapture={stopInteractiveRowClick}>
        <DataGrid
          emptyMessage={emptyState}
          loading={tableData.isLoading}
          onRowClick={handleRowClick}
          recordCount={
            tableData.isHydrated ? table.getRowModel().rows.length : 0
          }
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
      </section>

      {/* Expanded detail */}
      {expandedLogId && (
        <ExpandedEventDetail
          event={allLogs.find((l) => l.id === expandedLogId) ?? null}
          onClose={() => setExpandedLogId(null)}
        />
      )}
    </Shell>
  );
}
