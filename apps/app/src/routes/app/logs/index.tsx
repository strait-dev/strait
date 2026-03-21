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
import type { ColumnDef } from "@tanstack/react-table";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { formatDistanceToNow } from "date-fns";
import { useMemo, useState } from "react";
import { z } from "zod/v4";

import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { createActionsColumn } from "@/components/tables/shared-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { EventTrigger } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import {
  EyeIcon,
  FileTextIcon,
  FilterIcon,
  LinkSquareIcon,
  SearchIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

// --- Status styling ---

const STATUS_STYLES: Record<string, { dot: string; badge: string }> = {
  pending: {
    dot: "bg-chart-3",
    badge: "bg-chart-3/10 text-chart-3 border-chart-3/20",
  },
  received: {
    dot: "bg-info",
    badge: "bg-info/10 text-info border-info/20",
  },
  expired: {
    dot: "bg-warning",
    badge: "bg-warning/10 text-warning border-warning/20",
  },
  failed: {
    dot: "bg-destructive",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  canceled: {
    dot: "bg-muted-foreground",
    badge:
      "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
  },
};

const STATUS_OPTIONS = ["pending", "received", "expired", "failed", "canceled"];

// --- Columns ---

const logColumns: ColumnDef<EventTrigger>[] = [
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => {
      const style = STATUS_STYLES[row.original.status] ?? STATUS_STYLES.pending;
      return (
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge
            className={cn("shrink-0 capitalize", style.badge)}
            variant="outline"
          >
            {row.original.status}
          </Badge>
        </div>
      );
    },
  },
  {
    accessorKey: "event_key",
    header: "Event Key",
    cell: ({ row }) => (
      <span className="line-clamp-1 max-w-[400px] font-mono text-sm">
        {row.original.event_key}
      </span>
    ),
  },
  {
    accessorKey: "source_type",
    header: "Source",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.source_type}
      </Badge>
    ),
  },
  {
    accessorKey: "trigger_type",
    header: "Type",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.trigger_type}
      </Badge>
    ),
  },
  {
    accessorKey: "requested_at",
    header: "Time",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.requested_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<EventTrigger>([
    {
      label: "Copy Event Key",
      icon: FileTextIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(row.original.event_key);
      },
    },
    {
      label: "Copy Run ID",
      icon: LinkSquareIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(
          row.original.job_run_id || row.original.workflow_run_id
        );
      },
    },
    {
      label: "View Details",
      icon: EyeIcon,
      onClick: () => {
        // TODO: navigate to event detail
      },
    },
  ]),
];

// --- Route ---

const searchSchema = z.object({
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
    let items = hasProject ? (data?.data ?? []) : [];
    if (selectedStatuses.length > 0) {
      items = items.filter((e) => selectedStatuses.includes(e.status));
    }
    return items;
  }, [data?.data, selectedStatuses, hasProject]);

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
        <HugeiconsIcon className="size-6 text-primary" icon={FileTextIcon} />
      }
      title="No events found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
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
            {STATUS_OPTIONS.map((status) => {
              const style = STATUS_STYLES[status] ?? STATUS_STYLES.pending;
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
        <DataTable emptyState={emptyState} table={table} />
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

function ExpandedEventDetail({
  event,
  onClose,
}: {
  event: EventTrigger | null;
  onClose: () => void;
}) {
  if (!event) {
    return null;
  }

  const style = STATUS_STYLES[event.status] ?? STATUS_STYLES.pending;

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge className={cn("capitalize", style.badge)} variant="outline">
            {event.status}
          </Badge>
          <span className="text-muted-foreground text-xs">
            {new Date(event.requested_at).toLocaleString()}
          </span>
        </div>
        <Button onClick={onClose} size="sm" variant="ghost">
          Close
        </Button>
      </div>
      <p className="mb-2 font-mono text-sm">{event.event_key}</p>
      <div className="flex items-center gap-4 text-muted-foreground text-xs">
        <span>
          Source: <code className="font-mono">{event.source_type}</code>
        </span>
        <span>
          Type: <code className="font-mono">{event.trigger_type}</code>
        </span>
        {event.job_run_id && (
          <span>
            Run: <code className="font-mono">{event.job_run_id}</code>
          </span>
        )}
      </div>
      {event.request_payload != null && (
        <pre className="mt-3 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
          {JSON.stringify(event.request_payload, null, 2)}
        </pre>
      )}
      {event.error && (
        <p className="mt-2 text-destructive text-sm">{event.error}</p>
      )}
    </div>
  );
}
