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
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";

import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import WorkflowDetailSheet from "@/components/dashboard/workflow-detail-sheet";
import { workflowColumns } from "@/components/tables/workflows-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { Workflow } from "@/hooks/api/types";
import { workflowsQueryOptions } from "@/hooks/api/use-workflows";
import {
  EyeIcon,
  FilterIcon,
  PauseActionIcon,
  PlayActionIcon,
  SearchIcon,
  WorkflowIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

const STATUS_OPTIONS = ["Enabled", "Disabled"] as const;

const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
  perPage: z.number().optional().default(20),
});

export const Route = createFileRoute("/app/workflows/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(workflowsQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: WorkflowsPage,
});

function WorkflowsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const { data } = useQuery({
    ...workflowsQueryOptions(),
    enabled: hasProject,
  });
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(
    null
  );
  const [sheetOpen, setSheetOpen] = useState(false);

  const selectedStatuses = search.status ?? [];

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const filteredData = useMemo(() => {
    const workflows = hasProject ? (data?.data ?? []) : [];
    if (selectedStatuses.length === 0) {
      return workflows;
    }
    return workflows.filter((workflow) => {
      if (selectedStatuses.includes("Enabled") && workflow.enabled) {
        return true;
      }
      if (selectedStatuses.includes("Disabled") && !workflow.enabled) {
        return true;
      }
      return false;
    });
  }, [data?.data, hasProject, selectedStatuses]);

  const table = useReactTable({
    data: filteredData,
    columns: workflowColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
      }),
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

  function toggleStatus(status: string) {
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

  function handleRowClick(workflow: Workflow) {
    setSelectedWorkflow(workflow);
    setSheetOpen(true);
  }

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No workflows yet. Create a workflow to orchestrate multiple jobs."
      hideButton
      icon={
        <HugeiconsIcon className="size-6 text-foreground" icon={WorkflowIcon} />
      }
      title="No workflows found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

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
            placeholder="Search workflows..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
            Status
            {selectedStatuses.length > 0 && (
              <Badge variant="default">{selectedStatuses.length}</Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {STATUS_OPTIONS.map((status) => (
              <DropdownMenuCheckboxItem
                checked={selectedStatuses.includes(status)}
                key={status}
                onCheckedChange={() => toggleStatus(status)}
              >
                {status}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
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
          const workflow = table.getRowModel().rows[idx]?.original;
          if (workflow) {
            handleRowClick(workflow);
          }
        }}
      >
        <DataTable
          emptyState={emptyState}
          floatingBar={
            <DataTableFloatingBar
              actions={[
                ...(selectedIds.length === 1
                  ? [
                      {
                        label: "View",
                        icon: EyeIcon,
                        onClick: () => {
                          const workflow = table
                            .getRowModel()
                            .rows.find(
                              (r) => r.id === selectedIds[0]
                            )?.original;
                          if (workflow) {
                            handleRowClick(workflow);
                          }
                        },
                      },
                    ]
                  : []),
                {
                  label: "Trigger",
                  icon: PlayActionIcon,
                  onClick: () => {
                    /* TODO */
                  },
                },
                {
                  label: "Pause",
                  icon: PauseActionIcon,
                  onClick: () => {
                    /* TODO */
                  },
                },
              ]}
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
