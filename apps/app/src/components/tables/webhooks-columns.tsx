import { Badge } from "@strait/ui/components/badge";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { WebhookSubscription } from "@/hooks/api/types";

export const webhookColumns: ColumnDef<WebhookSubscription>[] = [
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
      <Badge
        className={
          row.original.active
            ? "border-[hsl(var(--chart-1))] text-[hsl(var(--chart-1))]"
            : "text-muted-foreground"
        }
        variant="outline"
      >
        {row.original.active ? "Active" : "Inactive"}
      </Badge>
    ),
  },
  {
    accessorKey: "event_types",
    header: "Events",
    cell: ({ row }) => (
      <div className="flex flex-wrap gap-1">
        {row.original.event_types.map((event) => (
          <Badge className="text-xs" key={event} variant="secondary">
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
];
