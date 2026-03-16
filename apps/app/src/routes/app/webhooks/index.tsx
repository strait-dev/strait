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
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
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
import { formatDistanceToNow } from "date-fns";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import PageHeader from "@/components/common/page-header";
import { StatusBadge } from "@/components/dashboard/status-badge";
import { webhookColumns } from "@/components/tables/webhooks-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { WebhookSubscription } from "@/hooks/api/types";
import { webhooksQueryOptions } from "@/hooks/api/use-webhooks";
import {
  EyeIcon,
  FilterIcon,
  GlobeIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  WebhookIcon,
} from "@/lib/icons";

const STATUS_OPTIONS = ["Active", "Inactive"] as const;

const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/webhooks/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(webhooksQueryOptions());
  },
  component: WebhooksPage,
});

function WebhooksPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    webhooksQueryOptions({ query: search.query, page: search.page })
  );

  const selectedStatuses = search.status ?? [];

  const filteredData = useMemo(() => {
    const webhooks = data?.data ?? [];
    if (selectedStatuses.length === 0) {
      return webhooks;
    }
    return webhooks.filter((webhook) => {
      if (selectedStatuses.includes("Active") && webhook.active) {
        return true;
      }
      if (selectedStatuses.includes("Inactive") && !webhook.active) {
        return true;
      }
      return false;
    });
  }, [data?.data, selectedStatuses]);

  const [selectedWebhook, setSelectedWebhook] =
    useState<WebhookSubscription | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const table = useReactTable({
    data: filteredData,
    columns: webhookColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

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
        status: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

  function handleRowClick(webhook: WebhookSubscription) {
    setSelectedWebhook(webhook);
    setSheetOpen(true);
  }

  return (
    <Shell>
      <PageHeader
        button={
          <Button>
            <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={16} />
            Create Webhook
          </Button>
        }
        text="Manage webhook subscriptions and delivery status."
        title="Webhooks"
      />

      <div className="flex items-center gap-3 py-4">
        <div className="relative flex-1">
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
            placeholder="Search webhooks..."
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
          <DropdownMenuContent align="end" className="w-48">
            {STATUS_OPTIONS.map((status) => (
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

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
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
          const webhook = table.getRowModel().rows[idx]?.original;
          if (webhook) {
            handleRowClick(webhook);
          }
        }}
      >
        <DataTable
          emptyState={
            <div className="py-12 text-center text-muted-foreground">
              No webhooks configured.
            </div>
          }
          floatingBar={
            <DataTableFloatingBar
              actions={[
                ...(selectedIds.length === 1
                  ? [
                      {
                        label: "View",
                        icon: EyeIcon,
                        onClick: () => {
                          const webhook = table.getRowModel().rows.find(
                            (r) => r.id === selectedIds[0]
                          )?.original;
                          if (webhook) {
                            handleRowClick(webhook);
                          }
                        },
                      },
                    ]
                  : []),
                {
                  label: "Delete",
                  icon: TrashIcon,
                  onClick: () => {},
                  variant: "destructive" as const,
                },
              ]}
              onClearSelection={() => setRowSelection({})}
              selectedCount={selectedIds.length}
            />
          }
          table={table}
        />
      </div>

      <WebhookDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        webhook={selectedWebhook}
      />
    </Shell>
  );
}

function WebhookDetailSheet({
  webhook,
  open,
  onOpenChange,
}: {
  webhook: WebhookSubscription | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!webhook) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <HugeiconsIcon
              className="text-muted-foreground"
              icon={WebhookIcon}
              size={16}
            />
            Webhook Details
          </SheetTitle>
        </SheetHeader>

        <div className="mt-4 space-y-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge status={webhook.active ? "completed" : "pending"} />
          </div>

          {/* Endpoint */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Endpoint
            </h4>
            <div className="flex items-center gap-2">
              <HugeiconsIcon
                className="shrink-0 text-muted-foreground"
                icon={GlobeIcon}
                size={14}
              />
              <code className="break-all text-xs">{webhook.webhook_url}</code>
            </div>
          </div>

          {/* Event Types */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Subscribed Events
            </h4>
            <div className="flex flex-wrap gap-1.5">
              {webhook.event_types.map((event) => (
                <Badge key={event} variant="secondary">
                  {event}
                </Badge>
              ))}
            </div>
          </div>

          {/* Metadata */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Metadata
            </h4>
            <div className="space-y-1.5 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID</span>
                <code className="text-xs">{webhook.id}</code>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created</span>
                <span className="text-xs">
                  {formatDistanceToNow(new Date(webhook.created_at), {
                    addSuffix: true,
                  })}
                </span>
              </div>
            </div>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
