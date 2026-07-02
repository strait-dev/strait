import { HugeiconsIcon } from "@hugeicons/react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
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
import { useEffect, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import { RESOURCE_TABLE_EMPTY_CLASS_NAME } from "@/components/tables/resource-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { WebhookDelivery } from "@/hooks/api/types";
import {
  useDeleteWebhook,
  useTestWebhook,
  webhookDeliveriesQueryOptions,
  webhookQueryOptions,
} from "@/hooks/api/use-webhooks";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  ChevronLeftIcon,
  ClockIcon,
  GlobeIcon,
  PlayActionIcon,
  TrashIcon,
  WebhookIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/webhooks/$id")({
  head: () => ({ meta: [{ title: "Webhook · Strait" }] }),
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await context.queryClient.ensureQueryData(webhookQueryOptions(params.id));
    return { session };
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
    header: "HTTP status",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => (
      <Badge mono size="xs" variant="secondary-light">
        {row.original.last_status_code ?? "-"}
      </Badge>
    ),
  },
  {
    accessorKey: "attempts",
    header: "Attempts",
    cell: ({ row }: { row: { original: WebhookDelivery } }) => (
      <Badge mono size="xs" variant="secondary-light">
        {row.original.attempts}/{row.original.max_attempts}
      </Badge>
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
  const { session } = Route.useLoaderData();
  const navigate = useNavigate();
  usePageEvent("webhook_detail_viewed", { webhook_id: id });

  const { data: webhook } = useSuspenseQuery(webhookQueryOptions(id));
  const [activeTab, setActiveTab] = useState("settings");
  const [isHydrated, setIsHydrated] = useState(false);
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
  const { permissions } = useProjectPermissions(session.user.activeProjectId);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  const deliveries = (deliveriesData?.data ?? []).filter(
    (delivery) => delivery.subscription_id === id
  );
  const tableData = useHydratedTableData(deliveries);

  const table = useReactTable({
    data: tableData.data,
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
          {permissions.canWriteWebhooks && (
            <>
              <Button
                disabled={!isHydrated || testWebhook.isPending}
                onClick={() => testWebhook.mutate(webhook.webhook_url)}
                variant="outline"
              >
                <HugeiconsIcon
                  className="mr-1.5"
                  icon={PlayActionIcon}
                  size={14}
                />
                Send test
              </Button>
              <AlertDialog>
                <AlertDialogTrigger
                  render={
                    <Button
                      disabled={!isHydrated || deleteWebhook.isPending}
                      variant="destructive"
                    />
                  }
                >
                  <HugeiconsIcon
                    className="mr-1.5"
                    icon={TrashIcon}
                    size={14}
                  />
                  Delete
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Delete webhook?</AlertDialogTitle>
                    <AlertDialogDescription>
                      This will permanently delete this webhook subscription.
                      Deliveries in progress will not be affected.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      disabled={deleteWebhook.isPending}
                      onClick={() => {
                        deleteWebhook.mutate(webhook.id, {
                          onSuccess: () => {
                            navigate({ to: "/app/webhooks" });
                          },
                        });
                      }}
                      variant="destructive"
                    >
                      Delete
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
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
            <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
              <EmptyHeader>
                <EmptyMedia media="icon" size="lg">
                  <HugeiconsIcon
                    className="text-muted-foreground"
                    icon={WebhookIcon}
                    size={24}
                  />
                </EmptyMedia>
                <EmptyTitle>Deliveries unavailable</EmptyTitle>
                <EmptyDescription>
                  Webhook deliveries are unavailable right now.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <DataGrid
              emptyMessage={
                <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
                  <EmptyHeader>
                    <EmptyMedia media="icon" size="lg">
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
              loading={deliveriesLoading || tableData.isLoading}
              recordCount={tableData.isHydrated ? deliveries.length : 0}
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
                <ConfigRow
                  action={
                    <StatusBadge
                      status={webhook.active ? "completed" : "pending"}
                    />
                  }
                  label="Status"
                />
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
