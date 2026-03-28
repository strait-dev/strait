import { Badge } from "@strait/ui/components/badge";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import StatusBadge from "@/components/dashboard/status-badge";
import type { WebhookSubscription } from "@/hooks/api/types";
import { EyeIcon, TrashIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type WebhookColumnActions = {
  onView?: (webhook: WebhookSubscription) => void;
  onDelete?: (webhook: WebhookSubscription) => void;
};

export const createWebhookColumns = (
  actions: WebhookColumnActions = {}
): ColumnDef<WebhookSubscription>[] => [
  createSelectColumn<WebhookSubscription>(),
  {
    accessorKey: "webhook_url",
    header: "Endpoint",
    cell: ({ row }) => (
      <span className="max-w-[300px] truncate font-mono text-xs">
        {row.original.webhook_url}
      </span>
    ),
  },
  {
    accessorKey: "active",
    header: "Status",
    cell: ({ row }) => (
      <StatusBadge status={row.original.active ? "completed" : "pending"} />
    ),
  },
  {
    accessorKey: "event_types",
    header: "Events",
    cell: ({ row }) => (
      <div className="flex flex-wrap gap-1">
        {(row.original.event_types ?? []).map((event) => (
          <Badge key={event} variant="secondary">
            {event}
          </Badge>
        ))}
      </div>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Created",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<WebhookSubscription>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Delete",
      icon: TrashIcon,
      onClick: (row) => actions.onDelete?.(row.original),
      variant: "destructive",
    },
  ]),
];
