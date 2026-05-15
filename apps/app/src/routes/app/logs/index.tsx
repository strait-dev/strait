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
import { cn } from "@strait/ui/utils/index";
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
import ExpandedEventDetail from "@/components/events/expanded-event-detail";
import { logColumns } from "@/components/events/log-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { EventTrigger, PaginatedResponse } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import { FileTextIcon, FilterIcon, SearchIcon } from "@/lib/icons";
import { EVENT_STATUS_STYLES, EVENT_STATUSES } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  statuses: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(50),
});

export const Route = createFileRoute("/app/logs/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(eventsQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: LogsPage,
});

function LogsPage() {
  usePageEvent("logs_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useQuery({
    ...eventsQueryOptions(),
    enabled: hasProject,
  });

  const [expandedLogId, setExpandedLogId] = useState<string | null>(null);

  const selectedStatuses = (search.statuses ?? []) as string[];

  const allLogs = useMemo(() => {
    const typed = data as PaginatedResponse<EventTrigger> | undefined;
    let items = hasProject ? (typed?.data ?? []) : [];
    if (selectedStatuses.length > 0) {
      items = items.filter((e: EventTrigger) =>
        selectedStatuses.includes(e.status)
      );
    }
    return items;
  }, [data, selectedStatuses, hasProject]);

  const table = useReactTable({
    data: allLogs,
    columns: logColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    state: { globalFilter: search.query ?? "" },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
    getRowId: (row) => row.id,
  });

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
        statuses: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

  function handleRowClick(event: EventTrigger) {
    setExpandedLogId((prev) => (prev === event.id ? null : event.id));
  }

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No log entries yet. Logs will appear as your jobs execute."
      hideButton
      icon={
        <HugeiconsIcon
          className="size-6 text-muted-foreground"
          icon={FileTextIcon}
        />
      }
      title="No events found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Logs</h1>
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
            placeholder="Search events by key or run ID..."
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
          <DropdownMenuContent align="end" className="w-36">
            {EVENT_STATUSES.map((status) => {
              const style =
                EVENT_STATUS_STYLES[status] ?? EVENT_STATUS_STYLES.pending;
              return (
                <DropdownMenuCheckboxItem
                  checked={selectedStatuses.includes(status)}
                  key={status}
                  onCheckedChange={() => toggleStatus(status)}
                >
                  <div className="flex items-center gap-2">
                    <span
                      className={cn("size-2 shrink-0 rounded-full", style.dot)}
                    />
                    <span className="capitalize">{status}</span>
                  </div>
                </DropdownMenuCheckboxItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation */}
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
          const event = table.getRowModel().rows[idx]?.original;
          if (event) {
            handleRowClick(event);
          }
        }}
      >
        <DataTable ariaLabel="Logs" emptyState={emptyState} table={table} />
      </div>

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
