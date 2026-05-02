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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  type ColumnDef,
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
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import { DataTable } from "@/components/ui/data-table/data-table";
import type {
  NotificationMessage,
  NotifyDeliveryChannel,
  NotifyMessageStatus,
} from "@/hooks/api/types";
import {
  notifyCategoriesQueryOptions,
  notifyDeliveriesQueryOptions,
} from "@/hooks/api/use-notify";
import { FilterIcon, MailIcon, SearchIcon } from "@/lib/icons";
import {
  notifyCursorPageLimit,
  resolveNotifyNextCursor,
} from "@/lib/notify-cursor";
import { NOTIFY_DELIVERY_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  channel: z.string().optional(),
  category_key: z.string().optional(),
  from: z.string().optional(),
  to: z.string().optional(),
});

type NotifyDeliveriesSearch = z.infer<typeof searchSchema>;

export const Route = createFileRoute("/app/notify/deliveries")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(
          notifyDeliveriesQueryOptions({ limit: notifyCursorPageLimit })
        ),
        context.queryClient.ensureQueryData(notifyCategoriesQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyDeliveriesPage,
});

const deliveryChannelOptions: readonly (NotifyDeliveryChannel | "all")[] = [
  "all",
  "email",
  "inbox",
] as const;

const formatDate = (value?: string) => {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
};

const toDatetimeLocalInput = (value?: string) => {
  if (!value) {
    return "";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");

  return `${year}-${month}-${day}T${hours}:${minutes}`;
};

const createColumns = (): ColumnDef<NotificationMessage>[] => [
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
    accessorKey: "category_key",
    header: "Category",
    cell: ({ row }) => row.original.category_key || "-",
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
  {
    id: "actions",
    header: "Actions",
    cell: ({ row }) => {
      const message = row.original;
      if (message.recipient_type !== "subscriber") {
        return "-";
      }

      return (
        <div className="flex flex-wrap gap-2">
          <Button
            render={
              <Link
                params={{ id: message.recipient_id }}
                to="/app/notify/subscribers/$id"
              />
            }
            size="sm"
            variant="outline"
          >
            Subscriber
          </Button>
          {message.suppression_reason ? (
            <Button
              render={
                <Link
                  hash="suppression-controls"
                  params={{ id: message.recipient_id }}
                  to="/app/notify/subscribers/$id"
                />
              }
              size="sm"
              variant="secondary"
            >
              Unsuppress hint
            </Button>
          ) : null}
        </div>
      );
    },
  },
];

function NotifyDeliveriesPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  const [cursor, setCursor] = useState<string>();
  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>(
    []
  );

  const selectedStatuses = search.status ?? [];
  const selectedChannel =
    search.channel && search.channel !== "all"
      ? (search.channel as NotifyDeliveryChannel)
      : undefined;

  const categoriesQuery = useQuery({
    ...notifyCategoriesQueryOptions(),
    enabled: hasProject,
  });

  const deliveriesQuery = useQuery({
    ...notifyDeliveriesQueryOptions({
      status:
        selectedStatuses.length === 1
          ? (selectedStatuses[0] as NotifyMessageStatus)
          : undefined,
      channel: selectedChannel,
      category_key: search.category_key,
      from: search.from,
      to: search.to,
      limit: notifyCursorPageLimit,
      cursor,
    }),
    enabled: hasProject,
  });

  const pageItems = deliveriesQuery.data ?? [];
  const categories = categoriesQuery.data ?? [];
  const sortedCategoryKeys = [...categories]
    .map((category) => category.category_key)
    .sort((a, b) => a.localeCompare(b));
  const nextCursor = resolveNotifyNextCursor(pageItems, notifyCursorPageLimit);

  const filtered = useMemo(
    () =>
      pageItems.filter((item) => {
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
      }),
    [pageItems, search.query, selectedStatuses]
  );

  const table = useReactTable({
    data: filtered,
    columns: createColumns(),
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

  const updateFilters = (
    updater: (prev: NotifyDeliveriesSearch) => NotifyDeliveriesSearch
  ) => {
    setCursor(undefined);
    setCursorHistory([]);
    navigate({
      search: (prev) => updater(prev),
    });
  };

  const toggleStatus = (status: NotifyMessageStatus) => {
    const next = new Set(selectedStatuses);
    if (next.has(status)) {
      next.delete(status);
    } else {
      next.add(status);
    }
    const values = Array.from(next);
    updateFilters((prev) => ({
      ...prev,
      status: values.length > 0 ? values : undefined,
    }));
  };

  const goToNextPage = () => {
    if (!nextCursor) {
      return;
    }

    setCursorHistory((history) => [...history, cursor]);
    setCursor(nextCursor);
  };

  const goToPreviousPage = () => {
    setCursorHistory((history) => {
      if (history.length === 0) {
        return history;
      }

      const nextHistory = [...history];
      const previousCursor = nextHistory.pop();
      setCursor(previousCursor);
      return nextHistory;
    });
  };

  return (
    <Shell>
      <div className="mb-3 space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative w-full max-w-[420px]">
            <Input
              className="pl-9"
              onChange={(event) => {
                updateFilters((prev) => ({
                  ...prev,
                  query: event.target.value || undefined,
                }));
              }}
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

          <Button
            onClick={() =>
              updateFilters((prev) => ({
                ...prev,
                channel: undefined,
                category_key: undefined,
                from: undefined,
                to: undefined,
              }))
            }
            variant="ghost"
          >
            Clear advanced filters
          </Button>
        </div>

        <div className="grid gap-3 md:grid-cols-4">
          <Select
            onValueChange={(value) => {
              const nextValue = value ?? "all";
              updateFilters((prev) => ({
                ...prev,
                channel: nextValue === "all" ? undefined : nextValue,
              }));
            }}
            value={search.channel || "all"}
          >
            <SelectTrigger>
              <SelectValue placeholder="Channel" />
            </SelectTrigger>
            <SelectContent>
              {deliveryChannelOptions.map((channel) => (
                <SelectItem key={channel} value={channel}>
                  {channel === "all" ? "All channels" : channel}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            onValueChange={(value) => {
              const nextValue = value ?? "all";
              updateFilters((prev) => ({
                ...prev,
                category_key: nextValue === "all" ? undefined : nextValue,
              }));
            }}
            value={search.category_key || "all"}
          >
            <SelectTrigger>
              <SelectValue placeholder="Category" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All categories</SelectItem>
              {sortedCategoryKeys.map((categoryKey) => (
                <SelectItem key={categoryKey} value={categoryKey}>
                  {categoryKey}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Input
            onChange={(event) => {
              const value = event.target.value;
              updateFilters((prev) => ({
                ...prev,
                from: value ? new Date(value).toISOString() : undefined,
              }));
            }}
            type="datetime-local"
            value={toDatetimeLocalInput(search.from)}
          />

          <Input
            onChange={(event) => {
              const value = event.target.value;
              updateFilters((prev) => ({
                ...prev,
                to: value ? new Date(value).toISOString() : undefined,
              }));
            }}
            type="datetime-local"
            value={toDatetimeLocalInput(search.to)}
          />
        </div>
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

      <div className="mt-3 flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-xs">
          Showing up to {notifyCursorPageLimit} deliveries per page.
        </p>
        <div className="flex gap-2">
          <Button
            aria-label="Go to previous deliveries page"
            disabled={cursorHistory.length === 0 || deliveriesQuery.isFetching}
            onClick={goToPreviousPage}
            variant="outline"
          >
            Previous page
          </Button>
          <Button
            aria-label="Go to next deliveries page"
            disabled={!nextCursor || deliveriesQuery.isFetching}
            onClick={goToNextPage}
            variant="outline"
          >
            Next page
          </Button>
        </div>
      </div>
    </Shell>
  );
}
