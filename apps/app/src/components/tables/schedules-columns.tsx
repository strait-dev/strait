import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import StatusBadge from "@/components/dashboard/status-badge";
import type { Job } from "@/hooks/api/types";
import { EyeIcon, PauseActionIcon, PlayActionIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type ScheduleColumnActions = {
  onView?: (schedule: Job) => void;
  onTrigger?: (schedule: Job) => void;
  onPauseResume?: (schedule: Job) => void;
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
  createActionsColumn<Job>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Trigger",
      icon: PlayActionIcon,
      onClick: (row) => actions.onTrigger?.(row.original),
    },
    {
      label: "Pause / Resume",
      icon: PauseActionIcon,
      onClick: (row) => actions.onPauseResume?.(row.original),
    },
  ]),
];
