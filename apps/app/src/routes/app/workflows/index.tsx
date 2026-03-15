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
import TableEmptyState from "@/components/common/table-empty-state";
import { WorkflowDetailSheet } from "@/components/dashboard/workflow-detail-sheet";
import { workflowColumns } from "@/components/tables/workflows-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { Workflow } from "@/hooks/api/types";
import { workflowsQueryOptions } from "@/hooks/api/use-workflows";
import { PlusIcon, SearchIcon, WorkflowIcon } from "@/lib/icons";

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

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
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
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter, rowSelection },
    onGlobalFilterChange: setGlobalFilter,
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

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
            aria-label="Search"
            className="pl-9"
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder="Search workflows..."
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
        <DataTable
          emptyState={
            <TableEmptyState
              description="No workflows match the current filters."
              hideButton
              icon={
                <HugeiconsIcon
                  className="size-6 text-primary"
                  icon={WorkflowIcon}
                />
              }
              title="No workflows found"
            />
          }
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

      <WorkflowDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        workflow={selectedWorkflow}
      />
    </Shell>
  );
}
