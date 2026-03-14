import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { StatusBadge } from "@/components/dashboard/status-badge.tsx";
import type { Workflow } from "@/hooks/api/types.ts";

export const workflowColumns: ColumnDef<Workflow>[] = [
  {
    accessorKey: "name",
    header: "Name",
    cell: ({ row }) => (
      <div className="flex flex-col gap-0.5">
        <span className="font-medium">{row.original.name}</span>
        {row.original.description && (
          <span className="text-muted-foreground text-xs">
            {row.original.description}
          </span>
        )}
      </div>
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
    id: "steps",
    header: "Steps",
    cell: () => (
      <div className="flex items-center gap-1">
        <span className="text-muted-foreground text-xs">—</span>
      </div>
    ),
  },
  {
    accessorKey: "version",
    header: "Version",
    cell: ({ row }) => <code className="text-xs">v{row.original.version}</code>,
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
