import { Badge } from "@strait/ui/components/badge";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { Link } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import { RelativeTime } from "@/components/common/relative-time";
import type { WebhookSubscription } from "@/hooks/api/types";
import { EyeIcon, TrashIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type WebhookColumnActions = {
  onView?: (webhook: WebhookSubscription) => void;
  onDelete?: (webhook: WebhookSubscription) => void;
  disabled?: boolean;
};

export const createWebhookColumns = (
  actions: WebhookColumnActions = {}
): ColumnDef<WebhookSubscription>[] => [
  createSelectColumn<WebhookSubscription>(),
  {
    accessorKey: "webhook_url",
    header: "Endpoint",
    cell: ({ row }) => (
      <Link
        aria-label={`View webhook ${row.original.webhook_url}`}
        className="block max-w-[300px] truncate font-mono text-xs hover:underline"
        params={{ id: row.original.id }}
        to="/app/webhooks/$id"
      >
        {row.original.webhook_url}
      </Link>
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
    cell: ({ row }) => <RelativeTime value={row.original.created_at} />,
  },
  createActionsColumn<WebhookSubscription>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Delete",
      hidden: !actions.onDelete,
      icon: TrashIcon,
      onClick: (row) => actions.onDelete?.(row.original),
      variant: "destructive",
    },
  ]),
];
