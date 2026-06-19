import { HugeiconsIcon } from "@hugeicons/react";
import { Button, buttonVariants } from "@strait/ui/components/button";
import { IdCell } from "@strait/ui/components/id-cell";
import type { ColumnDef } from "@tanstack/react-table";
import { RelativeTime } from "@/components/common/relative-time";
import type { JobRun } from "@/hooks/api/types";
import { EyeIcon, RefreshIcon, TrashIcon } from "@/lib/icons";
import { createSelectColumn } from "./shared-columns";

type DlqColumnActions = {
  onView?: (run: JobRun) => void;
  onRetry?: (run: JobRun) => void;
  onDiscard?: (run: JobRun) => void;
  disabled?: boolean;
};

export const createDlqColumns = (
  actions: DlqColumnActions = {}
): ColumnDef<JobRun>[] => [
  createSelectColumn<JobRun>(),
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <Button
        aria-label={`View run ${row.original.id}`}
        className="font-mono"
        disabled={actions.disabled}
        onClick={(event) => {
          event.stopPropagation();
          actions.onView?.(row.original);
        }}
        size="xs"
        variant="link"
      >
        {row.original.id.slice(0, 8)}
      </Button>
    ),
  },
  {
    accessorKey: "job_id",
    header: "Job",
    cell: ({ row }) => <IdCell id={row.original.job_id} length={8} />,
  },
  {
    accessorKey: "error",
    header: "Error",
    cell: ({ row }) => (
      <span className="line-clamp-1 text-sm">
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
    cell: ({ row }) => <RelativeTime value={row.original.created_at} />,
  },
  {
    id: "actions",
    cell: ({ row }) => {
      const run = row.original;
      return (
        <div className="flex items-center justify-end gap-1" data-no-row-click>
          {[
            { label: "View", icon: EyeIcon, onClick: actions.onView },
            { label: "Retry", icon: RefreshIcon, onClick: actions.onRetry },
            {
              label: "Discard",
              icon: TrashIcon,
              onClick: actions.onDiscard,
              destructive: true,
            },
          ]
            .filter((action) => !!action.onClick)
            .map((action) => (
              <button
                aria-label={action.label}
                className={buttonVariants({
                  size: "icon-sm",
                  variant: action.destructive ? "destructive" : "ghost",
                })}
                disabled={actions.disabled}
                key={action.label}
                onClick={(event) => {
                  event.stopPropagation();
                  action.onClick?.(run);
                }}
                type="button"
              >
                <HugeiconsIcon
                  aria-hidden="true"
                  className="size-3.5"
                  icon={action.icon}
                />
              </button>
            ))}
        </div>
      );
    },
    enableSorting: false,
    enableHiding: false,
  },
];
