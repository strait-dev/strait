import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
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
import { useState } from "react";
import { z } from "zod/v4";
import PageHeader from "@/components/common/page-header";
import { webhookColumns } from "@/components/tables/webhooks-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { WebhookSubscription } from "@/hooks/api/types";
import { webhooksQueryOptions } from "@/hooks/api/use-webhooks";
import { GlobeIcon, PlusIcon, WebhookIcon } from "@/lib/icons";

const searchSchema = z.object({
  query: z.string().optional(),
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
  const _navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    webhooksQueryOptions({ query: search.query, page: search.page })
  );

  const [selectedWebhook, _setSelectedWebhook] =
    useState<WebhookSubscription | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const table = useReactTable({
    data: data?.data ?? [],
    columns: webhookColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  return (
    <Shell>
      <PageHeader
        button={
          <Button size="sm">
            <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={14} />
            Create Webhook
          </Button>
        }
        text="Manage webhook subscriptions and delivery status."
        title="Webhooks"
      />

      <div className="pt-4">
        <DataTable
          emptyState={
            <div className="py-12 text-center text-muted-foreground">
              No webhooks configured.
            </div>
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
            <Badge
              className={
                webhook.active
                  ? "border-[hsl(var(--chart-1))] text-[hsl(var(--chart-1))]"
                  : "text-muted-foreground"
              }
              variant="outline"
            >
              {webhook.active ? "Active" : "Inactive"}
            </Badge>
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
                <Badge className="text-xs" key={event} variant="secondary">
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
