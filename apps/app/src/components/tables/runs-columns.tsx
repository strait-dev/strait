import { Badge } from "@strait/ui/components/badge.tsx";
import { Link } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { StatusBadge } from "@/components/dashboard/status-badge.tsx";
import type { JobRun } from "@/hooks/api/types.ts";

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

export const runColumns: ColumnDef<JobRun>[] = [
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
    cell: ({ row }) => (
      <span className="font-mono text-xs">
        {row.original.job_id.slice(0, 8)}
      </span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    id: "duration",
    header: "Duration",
    cell: ({ row }) =>
      formatDuration(row.original.started_at, row.original.finished_at),
  },
  {
    accessorKey: "attempt",
    header: "Attempt",
  },
  {
    accessorKey: "triggered_by",
    header: "Trigger",
    cell: ({ row }) => (
      <Badge className="text-xs capitalize" variant="outline">
        {row.original.triggered_by}
      </Badge>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Started",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
];
