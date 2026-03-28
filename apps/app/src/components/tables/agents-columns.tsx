import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { Agent } from "@/hooks/api/types";
import { EyeIcon } from "@/lib/icons";
import { createActionsColumn, createSelectColumn } from "./shared-columns";

export const agentColumns: ColumnDef<Agent>[] = [
  createSelectColumn<Agent>(),
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
    accessorKey: "slug",
    header: "Slug",
    cell: ({ row }) => <code className="text-xs">{row.original.slug}</code>,
  },
  {
    accessorKey: "model",
    header: "Model",
    cell: ({ row }) => <code className="text-xs">{row.original.model}</code>,
  },
  {
    accessorKey: "updated_at",
    header: "Last Updated",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.updated_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<Agent>([
    { label: "View", icon: EyeIcon, onClick: () => undefined },
  ]),
];
