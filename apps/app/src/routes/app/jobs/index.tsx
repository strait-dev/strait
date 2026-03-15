import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useState } from "react";
import PageHeader from "@/components/common/page-header";
import { JobDetailSheet } from "@/components/dashboard/job-detail-sheet";
import { jobColumns } from "@/components/tables/jobs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import { jobsQueryOptions } from "@/hooks/api/use-jobs";
import { PlusIcon, SearchIcon } from "@/lib/icons";

export const Route = createFileRoute("/app/jobs/")({
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(jobsQueryOptions());
  },
  component: JobsPage,
});

function JobsPage() {
  const { data } = useSuspenseQuery(jobsQueryOptions()) as {
    data: PaginatedResponse<Job>;
  };
  const [globalFilter, setGlobalFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "paused">(
    "all"
  );
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const filteredData = (data?.data ?? []).filter((job) => {
    if (statusFilter === "active" && !job.enabled) {
      return false;
    }
    if (statusFilter === "paused" && job.enabled) {
      return false;
    }
    return true;
  });

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const table = useReactTable({
    data: filteredData,
    columns: jobColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter, rowSelection },
    onGlobalFilterChange: setGlobalFilter,
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

  function handleRowClick(job: Job) {
    setSelectedJob(job);
    setSheetOpen(true);
  }

  return (
    <Shell>
      <PageHeader
        button={
          <Button disabled>
            <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={16} />
            Create Job
          </Button>
        }
        text="Manage and monitor your scheduled and on-demand jobs."
        title="Jobs"
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
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder="Search jobs..."
            value={globalFilter}
          />
        </div>

        <Tabs
          onValueChange={(v) => setStatusFilter(v as typeof statusFilter)}
          value={statusFilter}
        >
          <TabsList>
            {(["all", "active", "paused"] as const).map((status) => (
              <TabsTrigger className="capitalize" key={status} value={status}>
                {status}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
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
          const job = table.getRowModel().rows[idx]?.original;
          if (job) {
            handleRowClick(job);
          }
        }}
      >
        <DataTable
          emptyState={<div>No jobs found</div>}
          floatingBar={
            <DataTableFloatingBar
              actions={[]}
              onClearSelection={() => setRowSelection({})}
              selectedCount={selectedIds.length}
            />
          }
          table={table}
        />
      </div>

      <JobDetailSheet
        job={selectedJob}
        onOpenChange={setSheetOpen}
        open={sheetOpen}
      />
    </Shell>
  );
}
