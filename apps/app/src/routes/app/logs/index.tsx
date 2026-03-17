import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { cn } from "@strait/ui/utils/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { formatDistanceToNow } from "date-fns";
import { useMemo, useState } from "react";
import { z } from "zod/v4";

import TableEmptyState from "@/components/common/table-empty-state";
import { createActionsColumn } from "@/components/tables/shared-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { RunEvent } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import {
  EyeIcon,
  FileTextIcon,
  FilterIcon,
  LinkSquareIcon,
  SearchIcon,
} from "@/lib/icons";

// --- Level styling ---

const LEVEL_STYLES: Record<string, { dot: string; badge: string }> = {
  info: {
    dot: "bg-info",
    badge: "bg-info/10 text-info border-info/20",
  },
  warn: {
    dot: "bg-warning",
    badge: "bg-warning/10 text-warning border-warning/20",
  },
  error: {
    dot: "bg-destructive",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  debug: {
    dot: "bg-muted-foreground",
    badge:
      "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
  },
};

const LEVEL_OPTIONS = ["info", "warn", "error", "debug"];

// --- Columns ---

const logColumns: ColumnDef<RunEvent>[] = [
  {
    accessorKey: "level",
    header: "Level",
    cell: ({ row }) => {
      const style = LEVEL_STYLES[row.original.level] ?? LEVEL_STYLES.info;
      return (
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge
            className={cn("shrink-0 capitalize", style.badge)}
            variant="outline"
          >
            {row.original.level}
          </Badge>
        </div>
      );
    },
  },
  {
    accessorKey: "message",
    header: "Message",
    cell: ({ row }) => (
      <span className="line-clamp-1 max-w-[400px]">{row.original.message}</span>
    ),
  },
  {
    accessorKey: "run_id",
    header: "Run ID",
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.run_id}</span>
    ),
  },
  {
    accessorKey: "type",
    header: "Type",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.type.replace("_", " ")}
      </Badge>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Time",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
  createActionsColumn<RunEvent>([
    {
      label: "Copy Message",
      icon: FileTextIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(row.original.message);
      },
    },
    {
      label: "Copy Run ID",
      icon: LinkSquareIcon,
      onClick: (row) => {
        navigator.clipboard.writeText(row.original.run_id);
      },
    },
    {
      label: "View Run",
      icon: EyeIcon,
      onClick: () => {
        // TODO: navigate to run detail
      },
    },
  ]),
];

// --- Route ---

const searchSchema = z.object({
  query: z.string().optional(),
  levels: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(50),
});

export const Route = createFileRoute("/app/logs/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(
      eventsQueryOptions({ type: "log" })
    );
  },
  component: LogsPage,
});

function LogsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    eventsQueryOptions({ type: "log", page: search.page })
  );

  const [expandedLogId, setExpandedLogId] = useState<string | null>(null);

  const selectedLevels = (search.levels ?? []) as string[];

  const allLogs = useMemo(() => {
    let logs = (data?.data ?? []).filter((e) => e.type === "log");
    if (selectedLevels.length > 0) {
      logs = logs.filter((log) => selectedLevels.includes(log.level));
    }
    return logs;
  }, [data?.data, selectedLevels]);

  const table = useReactTable({
    data: allLogs,
    columns: logColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    state: { globalFilter: search.query ?? "" },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
    getRowId: (row) => row.id,
  });

  function toggleLevel(level: string) {
    const current = new Set(selectedLevels);
    if (current.has(level)) {
      current.delete(level);
    } else {
      current.add(level);
    }
    const arr = Array.from(current);
    navigate({
      search: (prev) => ({
        ...prev,
        levels: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

  function handleRowClick(log: RunEvent) {
    setExpandedLogId((prev) => (prev === log.id ? null : log.id));
  }

  return (
    <Shell>
      <div className="flex items-center gap-3 pb-2.5">
        <div className="relative w-full max-w-[500px]">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
            aria-label="Search"
            className="pl-9"
            onChange={(e) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: e.target.value || undefined,
                  page: 1,
                }),
              })
            }
            placeholder="Search logs by message or run ID..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
            Level
            {selectedLevels.length > 0 && (
              <Badge variant="default">{selectedLevels.length}</Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-36">
            {LEVEL_OPTIONS.map((level) => {
              const style = LEVEL_STYLES[level] ?? LEVEL_STYLES.info;
              return (
                <DropdownMenuCheckboxItem
                  checked={selectedLevels.includes(level)}
                  key={level}
                  onCheckedChange={() => toggleLevel(level)}
                >
                  <div className="flex items-center gap-2">
                    <span
                      className={cn("size-2 shrink-0 rounded-full", style.dot)}
                    />
                    <span className="capitalize">{level}</span>
                  </div>
                </DropdownMenuCheckboxItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const target = e.target as HTMLElement;
          if (target.closest("a, button")) {
            return;
          }
          const row = target.closest("tr[data-row-index]");
          if (!row) {
            return;
          }
          const idx = Number(row.getAttribute("data-row-index"));
          const log = table.getRowModel().rows[idx]?.original;
          if (log) {
            handleRowClick(log);
          }
        }}
      >
        <DataTable
          emptyState={
            <TableEmptyState
              description="No log entries match the current filters."
              hideButton
              icon={
                <HugeiconsIcon
                  className="size-6 text-primary"
                  icon={FileTextIcon}
                />
              }
              title="No logs found"
            />
          }
          table={table}
        />
      </div>

      {/* Expanded log detail */}
      {expandedLogId && (
        <ExpandedLogDetail
          log={allLogs.find((l) => l.id === expandedLogId) ?? null}
          onClose={() => setExpandedLogId(null)}
        />
      )}
    </Shell>
  );
}

function ExpandedLogDetail({
  log,
  onClose,
}: {
  log: RunEvent | null;
  onClose: () => void;
}) {
  if (!log) {
    return null;
  }

  const style = LEVEL_STYLES[log.level] ?? LEVEL_STYLES.info;

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge className={cn("capitalize", style.badge)} variant="outline">
            {log.level}
          </Badge>
          <span className="text-muted-foreground text-xs">
            {new Date(log.created_at).toLocaleString()}
          </span>
        </div>
        <Button onClick={onClose} size="sm" variant="ghost">
          Close
        </Button>
      </div>
      <p className="mb-2 text-sm">{log.message}</p>
      <div className="flex items-center gap-4 text-muted-foreground text-xs">
        <span>
          Run: <code className="font-mono">{log.run_id}</code>
        </span>
        <span>
          Type: <code className="font-mono">{log.type}</code>
        </span>
      </div>
      {log.data != null && (
        <pre className="mt-3 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
          {JSON.stringify(log.data, null, 2)}
        </pre>
      )}
    </div>
  );
}
