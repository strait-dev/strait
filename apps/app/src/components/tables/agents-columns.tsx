import { Badge } from "@strait/ui/components/badge";
import { Link } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import type { AgentListRow } from "@/components/agents/agent-list-utils";
import RelativeTime from "@/components/common/relative-time";
import StatusBadge from "@/components/dashboard/status-badge";
import { formatMicroUsd } from "@/lib/format";
import { EyeIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

export const agentColumns: ColumnDef<AgentListRow>[] = [
  createSelectColumn<AgentListRow>(),
  {
    accessorKey: "name",
    header: "Name",
    cell: ({ row }) => (
      <div className="flex flex-col gap-0.5">
        <Link
          className="font-medium hover:underline"
          params={{ id: row.original.id }}
          to="/app/agents/$id"
        >
          {row.original.name}
        </Link>
        {row.original.description && (
          <span className="text-muted-foreground text-xs">
            {row.original.description}
          </span>
        )}
      </div>
    ),
  },
  {
    accessorKey: "slug",
    header: "Slug",
    cell: ({ row }) => <code className="text-xs">{row.original.slug}</code>,
  },
  {
    accessorKey: "model",
    header: "Model",
    cell: ({ row }) => {
      let statusBadge = <Badge variant="outline">idle</Badge>;
      if (row.original.active_runs > 0) {
        statusBadge = <StatusBadge showDot status="executing" />;
      } else if (row.original.last_run_status) {
        statusBadge = (
          <StatusBadge showDot status={row.original.last_run_status} />
        );
      }

      return (
        <div className="flex flex-col gap-1">
          <code className="text-xs">{row.original.model}</code>
          {statusBadge}
        </div>
      );
    },
  },
  {
    accessorKey: "total_runs",
    header: "Runs",
    cell: ({ row }) => row.original.total_runs.toLocaleString(),
  },
  {
    accessorKey: "total_cost_microusd",
    header: "Cost",
    cell: ({ row }) => formatMicroUsd(row.original.total_cost_microusd),
  },
  {
    accessorKey: "last_run_at",
    header: "Last Run",
    cell: ({ row }) =>
      row.original.last_run_at ? (
        <RelativeTime value={row.original.last_run_at} />
      ) : (
        <span className="text-muted-foreground text-xs">Never</span>
      ),
  },
  createActionsColumn<AgentListRow>([
    { label: "View", icon: EyeIcon, onClick: () => undefined },
  ]),
];
