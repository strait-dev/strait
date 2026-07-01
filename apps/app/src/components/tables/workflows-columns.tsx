import { Badge } from "@strait/ui/components/badge";
import { StatusBadge } from "@strait/ui/components/status-badge";
import type { ColumnDef } from "@tanstack/react-table";
import { RelativeTime } from "@/components/common/relative-time";
import type { Workflow } from "@/hooks/api/types";
import {
  EyeIcon,
  PauseActionIcon,
  PlayActionIcon,
  TrashIcon,
} from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type WorkflowColumnActions = {
  onView?: (workflow: Workflow) => void;
  onTrigger?: (workflow: Workflow) => void;
  onPauseResume?: (workflow: Workflow) => void;
  onDelete?: (workflow: Workflow) => void;
};

export const createWorkflowColumns = (
  actions: WorkflowColumnActions = {}
): ColumnDef<Workflow>[] => [
  createSelectColumn<Workflow>(),
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
        <span className="text-muted-foreground text-xs">{"\u2014"}</span>
      </div>
    ),
  },
  {
    accessorKey: "version",
    header: "Version",
    cell: ({ row }) => (
      <Badge mono size="xs" variant="secondary-light">
        v{row.original.version}
      </Badge>
    ),
  },
  {
    accessorKey: "updated_at",
    header: "Last Updated",
    cell: ({ row }) => <RelativeTime value={row.original.updated_at} />,
  },
  createActionsColumn<Workflow>([
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
