import { HugeiconsIcon } from "@hugeicons/react";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import StatusBadge from "@/components/dashboard/status-badge";
import type { Workflow } from "@/hooks/api/types";
import { EyeIcon, KeyIcon, PauseActionIcon, PlayActionIcon } from "@/lib/icons";
import { isSingletonConfigured } from "@/lib/singleton";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

type WorkflowColumnActions = {
  onView?: (workflow: Workflow) => void;
  onTrigger?: (workflow: Workflow) => void;
  onPauseResume?: (workflow: Workflow) => void;
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
        <span className="flex items-center gap-1.5 font-medium">
          {row.original.name}
          {isSingletonConfigured(row.original) && (
            <span className="inline-flex" title="Singleton">
              <HugeiconsIcon
                className="size-3.5 text-muted-foreground"
                icon={KeyIcon}
              />
            </span>
          )}
        </span>
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
  createActionsColumn<Workflow>([
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
