import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { StatusBadge } from "@/components/dashboard/status-badge.tsx";
import type { Job } from "@/hooks/api/types.ts";

export const scheduleColumns: ColumnDef<Job>[] = [
  {
    accessorKey: "name",
    header: "Name",
  },
  {
    accessorKey: "cron",
    header: "Schedule",
    cell: ({ row }) => (
      <code className="text-xs">{row.original.cron || "\u2014"}</code>
    ),
  },
  {
    accessorKey: "enabled",
    header: "Status",
    cell: ({ row }) => (
      <StatusBadge
        showDot
        status={row.original.enabled ? "completed" : "paused"}
      />
    ),
  },
  {
    accessorKey: "updated_at",
    header: "Last Updated",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.updated_at), {
        addSuffix: true,
      }),
  },
];
