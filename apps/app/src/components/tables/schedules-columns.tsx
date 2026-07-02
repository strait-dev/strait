import { Badge } from "@strait/ui/components/badge";
import { StatusBadge } from "@strait/ui/components/status-badge";
import type { ColumnDef } from "@tanstack/react-table";
import { RelativeTime } from "@/components/common/relative-time";
import type { Job } from "@/hooks/api/types";
import {
  EyeIcon,
  PauseActionIcon,
  PencilEditIcon,
  PlayActionIcon,
  TrashIcon,
} from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type ScheduleColumnActions = {
  onView?: (schedule: Job) => void;
  onEdit?: (schedule: Job) => void;
  onTrigger?: (schedule: Job) => void;
  onPauseResume?: (schedule: Job) => void;
  onDelete?: (schedule: Job) => void;
};

export const createScheduleColumns = (
  actions: ScheduleColumnActions = {}
): ColumnDef<Job>[] => [
  createSelectColumn<Job>(),
  {
    accessorKey: "name",
    header: "Name",
  },
  {
    accessorKey: "cron",
    header: "Schedule",
    cell: ({ row }) => (
      <Badge mono size="xs" variant="secondary-light">
        {row.original.cron || "\u2014"}
      </Badge>
    ),
  },
  {
    accessorKey: "enabled",
    header: "Status",
    cell: ({ row }) => (
      <StatusBadge
        showDot
        status={
          row.original.paused || !row.original.enabled ? "paused" : "completed"
        }
      />
    ),
  },
  {
    accessorKey: "updated_at",
    header: "Last Updated",
    cell: ({ row }) => <RelativeTime value={row.original.updated_at} />,
  },
  createActionsColumn<Job>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Trigger",
      hidden: !actions.onTrigger,
      icon: PlayActionIcon,
      onClick: (row) => actions.onTrigger?.(row.original),
    },
    {
      label: "Edit",
      hidden: !actions.onEdit,
      icon: PencilEditIcon,
      onClick: (row) => actions.onEdit?.(row.original),
    },
    {
      label: "Pause / Resume",
      hidden: !actions.onPauseResume,
      icon: PauseActionIcon,
      onClick: (row) => actions.onPauseResume?.(row.original),
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
