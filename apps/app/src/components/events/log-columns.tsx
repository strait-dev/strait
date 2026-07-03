import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { DropdownMenuItem } from "@strait/ui/components/dropdown-menu";
import { StatusBadge } from "@strait/ui/components/status-badge";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { createActionsColumn } from "@/components/tables/shared-columns";
import type { EventTrigger } from "@/hooks/api/types";
import { EyeIcon, FileTextIcon, LinkSquareIcon } from "@/lib/icons";

export const logColumns: ColumnDef<EventTrigger>[] = [
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "event_key",
    header: "Event key",
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
      label: "Copy event key",
      icon: FileTextIcon,
      render: (row) => (
        <DropdownMenuItem
          onClick={() => navigator.clipboard.writeText(row.original.event_key)}
        >
          <HugeiconsIcon className="mr-2 size-3.5" icon={FileTextIcon} />
          Copy event key
        </DropdownMenuItem>
      ),
    },
    {
      label: "Copy run ID",
      icon: LinkSquareIcon,
      render: (row) => (
        <DropdownMenuItem
          onClick={() =>
            navigator.clipboard.writeText(
              row.original.job_run_id ?? row.original.workflow_run_id ?? ""
            )
          }
        >
          <HugeiconsIcon className="mr-2 size-3.5" icon={LinkSquareIcon} />
          Copy run ID
        </DropdownMenuItem>
      ),
    },
    {
      label: "View details",
      icon: EyeIcon,
      onClick: () => {
        // TODO: navigate to event detail
      },
    },
  ]),
];
