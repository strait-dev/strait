import { Badge } from "@strait/ui/components/badge";
import { IdCell } from "@strait/ui/components/id-cell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { Link } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import { RelativeTime } from "@/components/common/relative-time";
import type { DisplayStatus, JobRun } from "@/hooks/api/types";
import { EyeIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

/** Compute human-readable duration between two ISO timestamps. */
function formatDuration(
  startedAt: string | null,
  finishedAt: string | null
): string {
  if (!(startedAt && finishedAt)) {
    return "\u2014";
  }
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime();
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const mins = Math.floor(ms / 60_000);
  const secs = Math.round((ms % 60_000) / 1000);
  return `${mins}m ${secs}s`;
}

type RunColumnActions = {
  onView?: (run: JobRun) => void;
  onRetry?: (run: JobRun) => void;
  onCancel?: (run: JobRun) => void;
};

export const createRunColumns = (
  actions: RunColumnActions = {}
): ColumnDef<JobRun>[] => [
  createSelectColumn<JobRun>(),
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <Link
        className="font-mono text-xs hover:underline"
        params={{ id: row.original.id }}
        to="/app/runs/$id"
      >
        {row.original.id.slice(0, 8)}
      </Link>
    ),
  },
  {
    accessorKey: "job_id",
    header: "Job",
    cell: ({ row }) => <IdCell id={row.original.job_id} length={8} />,
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => (
      <StatusBadge status={row.original.status as DisplayStatus} />
    ),
  },
  {
    id: "duration",
    header: "Duration",
    cell: ({ row }) =>
      formatDuration(
        row.original.started_at ?? null,
        row.original.finished_at ?? null
      ),
  },
  {
    accessorKey: "attempt",
    header: "Attempt",
  },
  {
    accessorKey: "triggered_by",
    header: "Trigger",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.triggered_by}
      </Badge>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Started",
    cell: ({ row }) => <RelativeTime value={row.original.created_at} />,
  },
  createActionsColumn<JobRun>([
    {
      label: "View",
      icon: EyeIcon,
      onClick: (row) => actions.onView?.(row.original),
    },
    {
      label: "Retry",
      hidden: !actions.onRetry,
      icon: RefreshIcon,
      onClick: (row) => actions.onRetry?.(row.original),
    },
    {
      label: "Cancel",
      icon: XCircleIcon,
      onClick: (row) => actions.onCancel?.(row.original),
    },
  ]),
];
