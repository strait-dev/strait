import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { JobRun } from "@/hooks/api/types";
import { EyeIcon, RefreshIcon, TrashIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type DlqColumnActions = {
  onView?: (run: JobRun) => void;
  onRetry?: (run: JobRun) => void;
  onDiscard?: (run: JobRun) => void;
};

export const createDlqColumns = (
  actions: DlqColumnActions = {}
): ColumnDef<JobRun>[] => [
  createSelectColumn<JobRun>(),
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <code className="font-mono text-xs">{row.original.id.slice(0, 8)}</code>
    ),
  },
  {
    accessorKey: "job_id",
    header: "Job",
    cell: ({ row }) => (
      <span className="font-mono text-xs">
        {row.original.job_id.slice(0, 8)}
      </span>
    ),
  },
  {
    accessorKey: "error",
    header: "Error",
    cell: ({ row }) => (
      <span className="line-clamp-1 text-destructive text-sm">
        {row.original.error || "\u2014"}
      </span>
    ),
  },
  {
    accessorKey: "attempt",
    header: "Attempts",
    cell: ({ row }) => (
      <span className="text-sm">
        {row.original.attempt}/{row.original.max_attempts_override || "\u2014"}
      </span>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Failed At",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<JobRun>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Retry",
      icon: RefreshIcon,
      onClick: (row) => actions.onRetry?.(row.original),
    },
    {
      label: "Discard",
      icon: TrashIcon,
      onClick: (row) => actions.onDiscard?.(row.original),
      variant: "destructive",
    },
  ]),
];
