import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { Input } from "@strait/ui/components/input.tsx";
import { Shell } from "@strait/ui/components/shell.tsx";
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
import PageHeader from "@/components/common/page-header.tsx";
import { WorkflowDetailSheet } from "@/components/dashboard/workflow-detail-sheet.tsx";
import { workflowColumns } from "@/components/tables/workflows-columns.tsx";
import { DataTable } from "@/components/ui/data-table/data-table.tsx";
import type { Workflow } from "@/hooks/api/types.ts";
import { workflowsQueryOptions } from "@/hooks/api/use-workflows.ts";
import { PlusIcon, SearchIcon } from "@/lib/icons.ts";

export const Route = createFileRoute("/app/workflows/")({
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(workflowsQueryOptions());
  },
  component: WorkflowsPage,
});

function WorkflowsPage() {
  const { data } = useSuspenseQuery(workflowsQueryOptions());
  const [globalFilter, setGlobalFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "paused">(
    "all"
  );
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(
    null
  );
  const [sheetOpen, setSheetOpen] = useState(false);

  const filteredData = (data?.data ?? []).filter((workflow) => {
    if (statusFilter === "active" && !workflow.enabled) {
      return false;
    }
    if (statusFilter === "paused" && workflow.enabled) {
      return false;
    }
    return true;
  });

  const table = useReactTable({
    data: filteredData,
    columns: workflowColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    state: { globalFilter },
    onGlobalFilterChange: setGlobalFilter,
  });

  function handleRowClick(workflow: Workflow) {
    setSelectedWorkflow(workflow);
    setSheetOpen(true);
  }

  return (
    <Shell>
      <PageHeader
        button={
          <Button disabled>
            <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={16} />
            Create Workflow
          </Button>
        }
        text="Orchestrate multi-step workflows with dependency graphs."
        title="Workflows"
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
            placeholder="Search workflows..."
            value={globalFilter}
          />
        </div>

        <div className="flex rounded-md border">
          {(["all", "active", "paused"] as const).map((status) => (
            <button
              className={`px-3 py-1.5 font-medium text-xs capitalize transition-colors ${
                statusFilter === status
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              } ${status === "all" ? "" : "border-l"}`}
              key={status}
              onClick={() => setStatusFilter(status)}
              type="button"
            >
              {status}
            </button>
          ))}
        </div>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        onClick={(e) => {
          const row = (e.target as HTMLElement).closest("tr[data-row-index]");
          if (!row) {
            return;
          }
          const idx = Number(row.getAttribute("data-row-index"));
          const workflow = table.getRowModel().rows[idx]?.original;
          if (workflow) {
            handleRowClick(workflow);
          }
        }}
      >
        <DataTable emptyState={<div>No workflows found</div>} table={table} />
      </div>

      <WorkflowDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        workflow={selectedWorkflow}
      />
    </Shell>
  );
}
