import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import StatusBadge from "@/components/dashboard/status-badge";
import { DataTable } from "@/components/ui/data-table/data-table";
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
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(webhookQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        webhookDeliveriesQueryOptions(params.id)
      ),
    ]);
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
      if (row.original.status === "delivered") mapped = "completed";
      else if (row.original.status === "failed") mapped = "failed";
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
  usePageEvent("webhook_detail_viewed", { webhook_id: id });

  const { data: webhook } = useSuspenseQuery(webhookQueryOptions(id));
  const { data: deliveriesData } = useQuery(webhookDeliveriesQueryOptions(id));

  const [activeTab, setActiveTab] = useState("deliveries");
  const deleteWebhook = useDeleteWebhook();
  const testWebhook = useTestWebhook();

  const deliveries = deliveriesData?.data ?? [];

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
        <Button render={<Link to="/app/webhooks" />} size="sm" variant="ghost">
          <HugeiconsIcon icon={ChevronLeftIcon} size={14} />
        </Button>
        <div className="flex-1">
          <h1 className="flex items-center gap-2 font-semibold text-lg">
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
            size="sm"
            variant="outline"
          >
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Send test
          </Button>
          <Button
            onClick={() => deleteWebhook.mutate(webhook.id)}
            size="sm"
            variant="destructive"
          >
            <HugeiconsIcon className="mr-1.5" icon={TrashIcon} size={14} />
            Delete
          </Button>
        </div>
      </div>

      <Tabs onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="deliveries">Deliveries</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="deliveries">
          <DataTable
            emptyState={
              <TableEmptyState
                description="No deliveries have been sent to this webhook yet."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="text-muted-foreground"
                    icon={WebhookIcon}
                    size={24}
                  />
                }
                title="No deliveries"
              />
            }
            table={table}
          />
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
