import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Link } from "@tanstack/react-router";
import {
  type ColumnDef,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import TableEmptyState from "@/components/common/table-empty-state";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { SingletonHolder } from "@/hooks/api/types";
import { KeyIcon, LayersIcon } from "@/lib/icons";

const columns: ColumnDef<SingletonHolder>[] = [
  {
    accessorKey: "lock_key",
    header: "Key",
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.lock_key}</span>
    ),
  },
  {
    accessorKey: "holder_run_id",
    header: "Holder",
    cell: ({ row }) => (
      <Link
        className="font-mono text-xs hover:underline"
        params={{ id: row.original.holder_run_id }}
        to="/app/runs/$id"
      >
        {row.original.holder_run_id.slice(0, 8)}
      </Link>
    ),
  },
  {
    accessorKey: "acquired_at",
    header: "Held for",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.acquired_at), {
        addSuffix: true,
      }),
  },
  {
    accessorKey: "lease_until",
    header: "Lease",
    cell: ({ row }) =>
      row.original.lease_until
        ? formatDistanceToNow(new Date(row.original.lease_until), {
            addSuffix: true,
          })
        : "—",
  },
  {
    accessorKey: "waiters",
    header: "Waiters",
    cell: ({ row }) => (
      <Badge className="gap-1" variant="secondary">
        <HugeiconsIcon icon={LayersIcon} size={12} />
        {row.original.waiters}
      </Badge>
    ),
  },
];

type SingletonHoldersTableProps = {
  holders: SingletonHolder[];
  isLoading: boolean;
  hasNextPage?: boolean;
  isFetchingNextPage?: boolean;
  onLoadMore?: () => void;
};

const SingletonHoldersTable = ({
  holders,
  isLoading,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
}: SingletonHoldersTableProps) => {
  const table = useReactTable({
    data: holders,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <div className="flex flex-col gap-3">
      <DataTable
        ariaLabel="Singleton holders"
        emptyState={
          <TableEmptyState
            description={
              isLoading
                ? "Loading held keys..."
                : "No keys are currently held. Held keys appear here while runs hold a singleton lock."
            }
            hideButton
            icon={
              <HugeiconsIcon
                className="size-6 text-foreground"
                icon={KeyIcon}
              />
            }
            title={isLoading ? "Loading" : "No keys currently held"}
          />
        }
        table={table}
      />
      {hasNextPage ? (
        <div className="flex justify-center">
          <Button
            disabled={isFetchingNextPage}
            onClick={onLoadMore}
            size="sm"
            variant="outline"
          >
            {isFetchingNextPage ? "Loading..." : "Load more"}
          </Button>
        </div>
      ) : null}
    </div>
  );
};

export default SingletonHoldersTable;
