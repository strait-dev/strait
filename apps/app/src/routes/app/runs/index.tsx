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
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useState } from "react";
import { z } from "zod/v4";
import PageHeader from "@/components/common/page-header";
import { RunDetailSheet } from "@/components/dashboard/run-detail-sheet";
import { StatusBadge } from "@/components/dashboard/status-badge";
import { runColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { JobRun, PaginatedResponse, RunStatus } from "@/hooks/api/types";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { CalendarIcon, FilterIcon, SearchIcon } from "@/lib/icons";

const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(20),
});

const STATUS_OPTIONS: RunStatus[] = [
  "queued",
  "executing",
  "completed",
  "failed",
  "timed_out",
  "canceled",
  "dead_letter",
  "waiting",
];

export const Route = createFileRoute("/app/runs/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(runsQueryOptions());
  },
  component: RunsPage,
});

function RunsPage() {
  const { data } = useSuspenseQuery(runsQueryOptions()) as {
    data: PaginatedResponse<JobRun>;
  };
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const selectedStatuses = (search.status ?? []) as RunStatus[];

  const table = useReactTable({
    data: data?.data ?? [],
    columns: runColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    state: { globalFilter: search.query ?? "" },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
  });

  function toggleStatus(status: RunStatus) {
    const current = new Set(selectedStatuses);
    if (current.has(status)) {
      current.delete(status);
    } else {
      current.add(status);
    }
    const arr = Array.from(current);
    navigate({
      search: (prev) => ({
        ...prev,
        status: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

  function handleRowClick(run: JobRun) {
    setSelectedRun(run);
    setSheetOpen(true);
  }

  return (
    <Shell>
      <PageHeader
        text="View and monitor all job run executions."
        title="Runs"
      />

      <div className="flex items-center gap-3 py-4">
        <div className="relative flex-1">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
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
            placeholder="Search by run ID or job name..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger>
            <Button variant="outline">
              <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
              Status
              {selectedStatuses.length > 0 && (
                <Badge size="xs" variant="default">
                  {selectedStatuses.length}
                </Badge>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {STATUS_OPTIONS.map((status) => (
              <DropdownMenuCheckboxItem
                checked={selectedStatuses.includes(status)}
                key={status}
                onCheckedChange={() => toggleStatus(status)}
              >
                <StatusBadge size="xs" status={status} />
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        <Button disabled size="sm" variant="outline">
          <HugeiconsIcon className="mr-1.5" icon={CalendarIcon} size={14} />
          Date Range
        </Button>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const target = e.target as HTMLElement;
          if (target.closest("a, button")) {
            return;
          }
          const tr = target.closest("tbody tr");
          if (!tr) {
            return;
          }
          const tbody = tr.closest("tbody");
          if (!tbody) {
            return;
          }
          const rows = Array.from(tbody.querySelectorAll(":scope > tr"));
          const idx = rows.indexOf(tr);
          if (idx < 0) {
            return;
          }
          const run = table.getRowModel().rows[idx]?.original;
          if (run) {
            handleRowClick(run);
          }
        }}
      >
        <DataTable emptyState={<div>No runs found</div>} table={table} />
      </div>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
