import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button, buttonVariants } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ConfigRow } from "@strait/ui/components/config-row";
import {
  DataGrid,
  DataGridContainer,
  DataGridPagination,
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
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { WebhookDelivery } from "@/hooks/api/types";
import {
  useDeleteWebhook,
  useTestWebhook,
  webhookDeliveriesQueryOptions,
  webhookQueryOptions,
} from "@/hooks/api/use-webhooks";
import {
  ChevronLeftIcon,
  ClockIcon,
  GlobeIcon,
  PlayActionIcon,
  TrashIcon,
  WebhookIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/webhooks/$id")({
  head: () => ({ meta: [{ title: "Webhook · Strait" }] }),
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(webhookQueryOptions(params.id));
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: WebhookDetailPage,
});

const deliveryColumns = [
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => {
      let mapped: "completed" | "failed" | "pending" = "pending";
      if (row.original.status === "delivered") {
        mapped = "completed";
      } else if (
        row.original.status === "failed" ||
        row.original.status === "dead"
      ) {
        mapped = "failed";
      }
      return <StatusBadge status={mapped} />;
    },
  },
  {
    accessorKey: "last_status_code",
    header: "HTTP Status",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => (
      <span className="font-mono text-xs">
        {row.original.last_status_code ?? "-"}
      </span>
    ),
  },
  {
    accessorKey: "attempts",
    header: "Attempts",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => (
      <span className="font-mono text-xs">
        {row.original.attempts}/{row.original.max_attempts}
      </span>
    ),
  },
  {
    accessorKey: "last_error",
    header: "Error",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => (
      <span className="max-w-[300px] truncate text-muted-foreground text-xs">
        {row.original.last_error || "-"}
      </span>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Created",
    cell: ({ row }: { row: { original: WebhookDelivery } }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
];

function WebhookDetailPage() {
  const { id } = Route.useParams();
  const navigate = useNavigate();
  usePageEvent("webhook_detail_viewed", { webhook_id: id });

  const { data: webhook } = useSuspenseQuery(webhookQueryOptions(id));
  const [activeTab, setActiveTab] = useState("settings");
  const {
    data: deliveriesData,
    isError: deliveriesError,
    isLoading: deliveriesLoading,
  } = useQuery({
    ...webhookDeliveriesQueryOptions(id),
    enabled: activeTab === "deliveries",
    throwOnError: false,
  });

  const deleteWebhook = useDeleteWebhook();
  const testWebhook = useTestWebhook();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const deliveries = (deliveriesData?.data ?? []).filter(
    (delivery) => delivery.subscription_id === id
  );

  const table = useReactTable({
    data: deliveries,
    columns: deliveryColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getRowId: (row) => row.id,
  });

  if (!webhook) {
    return <EntityNotFound entity="webhook" />;
  }

  return (
    <Shell>
      <div className="mb-4 flex items-center gap-3">
        <Button render={<Link to="/app/webhooks" />} variant="ghost">
          <HugeiconsIcon icon={ChevronLeftIcon} size={14} />
        </Button>
        <div className="flex-1">
          <h1 className="flex items-center gap-2 text-balance font-normal text-xl tracking-tight">
            <HugeiconsIcon
              className="text-muted-foreground"
              icon={WebhookIcon}
              size={18}
            />
            Webhook
          </h1>
          <p className="font-mono text-muted-foreground text-xs">
            {webhook.webhook_url}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            onClick={() => testWebhook.mutate(webhook.webhook_url)}
            variant="outline"
          >
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Send test
          </Button>
          <button
            aria-label="Delete webhook"
            className={buttonVariants({ variant: "destructive" })}
            onClick={() => setDeleteDialogOpen(true)}
            onPointerDown={() => setDeleteDialogOpen(true)}
            type="button"
          >
            <HugeiconsIcon className="mr-1.5" icon={TrashIcon} size={14} />
            Delete
          </button>
          {deleteDialogOpen && (
            <div
              className="fixed inset-0 z-50 grid place-items-center bg-black/10 p-4"
              role="presentation"
            >
              <div
                aria-labelledby="delete-webhook-title"
                aria-modal="true"
                className="grid w-full max-w-sm gap-4 rounded-lg bg-background p-4 shadow-lg ring-1 ring-foreground/10"
                role="alertdialog"
              >
                <div className="grid gap-1.5">
                  <h2
                    className="font-medium text-base"
                    id="delete-webhook-title"
                  >
                    Delete webhook?
                  </h2>
                  <p className="text-muted-foreground text-sm">
                    This will permanently delete this webhook subscription.
                    Deliveries in progress will not be affected.
                  </p>
                </div>
                <div className="-mx-4 -mb-4 flex justify-end gap-2 border-t bg-muted/50 p-4">
                  <Button
                    onClick={() => setDeleteDialogOpen(false)}
                    type="button"
                    variant="outline"
                  >
                    Cancel
                  </Button>
                  <Button
                    disabled={deleteWebhook.isPending}
                    onClick={() => {
                      deleteWebhook.mutate(webhook.id, {
                        onSuccess: () => {
                          setDeleteDialogOpen(false);
                          navigate({ to: "/app/webhooks" });
                        },
                      });
                    }}
                    type="button"
                    variant="destructive"
                  >
                    Delete
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <Tabs onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="deliveries">Deliveries</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="deliveries">
          {deliveriesError ? (
            <output className="block rounded-lg border border-dashed p-8 text-center text-muted-foreground text-sm">
              Webhook deliveries are unavailable right now.
            </output>
          ) : (
            <DataGrid
              emptyMessage={
                <Empty className="h-[300px]">
                  <EmptyHeader>
                    <EmptyMedia size="lg" variant="icon">
                      <HugeiconsIcon
                        className="text-muted-foreground"
                        icon={WebhookIcon}
                        size={24}
                      />
                    </EmptyMedia>
                    <EmptyTitle>
                      {deliveriesLoading
                        ? "Loading deliveries"
                        : "No deliveries"}
                    </EmptyTitle>
                    <EmptyDescription>
                      {deliveriesLoading
                        ? "Loading deliveries..."
                        : "No deliveries have been sent to this webhook yet."}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              }
              recordCount={deliveries.length}
              table={table}
              tableClassNames={{ base: "min-w-[1200px]" }}
            >
              <DataGridContainer>
                <DataGridScrollArea>
                  <DataGridTable />
                </DataGridScrollArea>
                <DataGridPagination />
              </DataGridContainer>
            </DataGrid>
          )}
        </TabsContent>

        <TabsContent value="settings">
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Configuration</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <ConfigRow
                  icon={GlobeIcon}
                  label="Endpoint"
                  value={webhook.webhook_url}
                />
                <ConfigRow
                  icon={ClockIcon}
                  label="Created"
                  value={formatDistanceToNow(new Date(webhook.created_at), {
                    addSuffix: true,
                  })}
                />
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">Status</span>
                  <StatusBadge
                    status={webhook.active ? "completed" : "pending"}
                  />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Subscribed events</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-1.5">
                  {(webhook.event_types ?? []).map((event) => (
                    <Badge key={event} variant="secondary">
                      {event}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
