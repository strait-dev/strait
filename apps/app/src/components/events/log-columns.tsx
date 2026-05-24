import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { createActionsColumn } from "@/components/tables/shared-columns";
import type { EventTrigger } from "@/hooks/api/types";
import { EyeIcon, FileTextIcon, LinkSquareIcon } from "@/lib/icons";
import { EVENT_STATUS_STYLES } from "@/lib/status";

export const logColumns: ColumnDef<EventTrigger>[] = [
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => {
      const style =
        EVENT_STATUS_STYLES[row.original.status] ?? EVENT_STATUS_STYLES.waiting;
      return (
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge
            className={cn("shrink-0 capitalize", style.badge)}
            variant="outline"
          >
            {row.original.status}
          </Badge>
        </div>
      );
    },
  },
  {
    accessorKey: "event_key",
    header: "Event Key",
    cell: ({ row }) => (
      <span className="line-clamp-1 max-w-[400px] font-mono text-sm">
        {row.original.event_key}
      </span>
    ),
  },
  {
    accessorKey: "source_type",
    header: "Source",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.source_type}
      </Badge>
    ),
  },
  {
    accessorKey: "trigger_type",
    header: "Type",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.trigger_type}
      </Badge>
    ),
  },
  {
    accessorKey: "requested_at",
    header: "Time",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.requested_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<EventTrigger>([
    {
      label: "Copy Event Key",
      icon: FileTextIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(row.original.event_key);
      },
    },
    {
      label: "Copy Run ID",
      icon: LinkSquareIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(
          row.original.job_run_id ?? row.original.workflow_run_id ?? ""
        );
      },
    },
    {
      label: "View Details",
      icon: EyeIcon,
      onClick: () => {
        // TODO: navigate to event detail
      },
    },
  ]),
];
