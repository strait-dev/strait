import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import StatusBadge from "@/components/dashboard/status-badge";
import type { Job } from "@/hooks/api/types";
import { EyeIcon, PauseActionIcon, PlayActionIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type JobColumnActions = {
  onView?: (job: Job) => void;
  onTrigger?: (job: Job) => void;
  onPause?: (job: Job) => void;
};

export const createJobColumns = (actions: JobColumnActions = {}): ColumnDef<Job>[] => [
  createSelectColumn<Job>(),
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
    accessorKey: "cron",
    header: "Schedule",
    cell: ({ row }) => (
      <code className="text-xs">{row.original.cron || "\u2014"}</code>
    ),
  },
  {
    accessorKey: "max_attempts",
    header: "Max Attempts",
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
  createActionsColumn<Job>([
    { label: "View", icon: EyeIcon, onClick: (row) => actions.onView?.(row.original) },
    { label: "Trigger", icon: PlayActionIcon, onClick: (row) => actions.onTrigger?.(row.original) },
    { label: "Pause", icon: PauseActionIcon, onClick: (row) => actions.onPause?.(row.original) },
  ]),
];
