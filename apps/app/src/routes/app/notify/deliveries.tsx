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
  type ColumnDef,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { NotificationMessage } from "@/hooks/api/types";
import { notifyDeliveriesQueryOptions } from "@/hooks/api/use-notify";
import { FilterIcon, MailIcon, SearchIcon } from "@/lib/icons";
import { NOTIFY_DELIVERY_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
});

export const Route = createFileRoute("/app/notify/deliveries")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(notifyDeliveriesQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyDeliveriesPage,
});

const formatDate = (value?: string) => {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
};

const columns: ColumnDef<NotificationMessage>[] = [
  {
    accessorKey: "id",
    header: "Message",
    cell: ({ row }) => row.original.id.slice(0, 8),
  },
  {
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => row.original.channel.toUpperCase(),
  },
  {
    accessorKey: "recipient_id",
    header: "Recipient",
    cell: ({ row }) => row.original.recipient_id,
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <NotifyStatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "suppression_reason",
    header: "Suppression",
    cell: ({ row }) => row.original.suppression_reason || "-",
  },
  {
    accessorKey: "created_at",
    header: "Created",
    cell: ({ row }) => formatDate(row.original.created_at),
  },
];

function NotifyDeliveriesPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  const { data } = useQuery({
    ...notifyDeliveriesQueryOptions(),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const filtered = useMemo(() => {
    const items = data ?? [];
    return items.filter((item) => {
      if (
        selectedStatuses.length > 0 &&
        !selectedStatuses.includes(item.status)
      ) {
        return false;
      }
      if (!search.query) {
        return true;
      }
      const q = search.query.toLowerCase();
      return (
        item.id.toLowerCase().includes(q) ||
        item.recipient_id.toLowerCase().includes(q) ||
        item.channel.toLowerCase().includes(q)
      );
    });
  }, [data, search.query, selectedStatuses]);

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const toggleStatus = (status: string) => {
    const next = new Set(selectedStatuses);
    if (next.has(status)) {
      next.delete(status);
    } else {
      next.add(status);
    }
    const values = Array.from(next);
    navigate({
      search: (prev) => ({
        ...prev,
        status: values.length > 0 ? values : undefined,
      }),
    });
  };

  return (
    <Shell>
      <div className="mb-3 flex items-center gap-3">
        <div className="relative w-full max-w-[420px]">
          <Input
            className="pl-9"
            onChange={(event) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: event.target.value || undefined,
                }),
              })
            }
            placeholder="Search message or recipient"
            value={search.query ?? ""}
          />
          <div className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground">
            <HugeiconsIcon className="size-4" icon={SearchIcon} />
          </div>
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5 size-4" icon={FilterIcon} />
            Status
            {selectedStatuses.length > 0 && (
              <Badge className="ml-2" variant="default">
                {selectedStatuses.length}
              </Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            {NOTIFY_DELIVERY_STATUS_OPTIONS.map((status) => (
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

      <DataTable
        emptyState={
          <TableEmptyState
            description="No notify deliveries found for this project yet."
            hideButton
            icon={
              <HugeiconsIcon
                className="size-6 text-foreground"
                icon={MailIcon}
              />
            }
            title="No deliveries"
          />
        }
        table={table}
      />
    </Shell>
  );
}
