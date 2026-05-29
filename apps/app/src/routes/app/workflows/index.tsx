import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridSelectionBar,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import WorkflowDetailSheet from "@/components/dashboard/workflow-detail-sheet";
import { createWorkflowColumns } from "@/components/tables/workflows-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { PaginatedResponse, Workflow } from "@/hooks/api/types";
import {
  usePauseWorkflow,
  useResumeWorkflow,
  useTriggerWorkflow,
  workflowsQueryOptions,
} from "@/hooks/api/use-workflows";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import {
  EyeIcon,
  FilterIcon,
  PauseActionIcon,
  PlayActionIcon,
  SearchIcon,
  WorkflowIcon,
} from "@/lib/icons";
import { ENABLED_STATUS_OPTIONS } from "@/lib/status";
import { stopInteractiveRowClick } from "@/lib/table-interactions";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  status: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
});

export const Route = createFileRoute("/app/workflows/")({
  head: () => ({ meta: [{ title: "Workflows · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        workflowsQueryOptions({ limit: deps.limit, cursor: deps.cursor })
      );
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: WorkflowsPage,
});

function WorkflowsPage() {
  usePageEvent("workflows_list_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const { data } = useQuery({
    ...workflowsQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
    }),
    enabled: hasProject,
  });
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(
    null
  );
  const [sheetOpen, setSheetOpen] = useState(false);
  const triggerWorkflow = useTriggerWorkflow();
  const pauseWorkflow = usePauseWorkflow();
  const resumeWorkflow = useResumeWorkflow();

  const selectedStatuses = search.status ?? [];

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const typed = data as PaginatedResponse<Workflow> | undefined;

  const filteredData = useMemo(() => {
    let workflows = hasProject ? (typed?.data ?? []) : [];
    const query = search.query?.trim().toLowerCase();
    if (query) {
      workflows = workflows.filter((workflow) =>
        [workflow.name, workflow.slug, workflow.description]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(query))
      );
    }
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
  }, [typed, hasProject, selectedStatuses, search.query]);

  const table = useReactTable({
    data: filteredData,
    columns: createWorkflowColumns({
      onView: (workflow) => {
        setSelectedWorkflow(workflow);
        setSheetOpen(true);
      },
      onTrigger: (workflow) =>
        triggerWorkflow.mutate({ workflowId: workflow.id }),
      onPauseResume: (workflow) => {
        if (workflow.enabled) {
          pauseWorkflow.mutate({ workflowId: workflow.id });
        } else {
          resumeWorkflow.mutate({ workflowId: workflow.id });
        }
      },
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({
          ...prev,
          query: query || undefined,
          cursor: undefined,
        }),
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
        cursor: undefined,
      }),
    });
  }

  function handleRowClick(workflow: Workflow) {
    setSelectedWorkflow(workflow);
    setSheetOpen(true);
  }

  const emptyState = hasProject ? (
    <Empty className="h-[300px]">
      <EmptyHeader>
        <EmptyMedia size="lg" variant="icon">
          <HugeiconsIcon
            className="size-6 text-foreground"
            icon={WorkflowIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No workflows found</EmptyTitle>
        <EmptyDescription>
          No workflows yet. Create a workflow to orchestrate multiple jobs.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Workflows</h1>
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
                  cursor: undefined,
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
            {ENABLED_STATUS_OPTIONS.map((status) => (
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

      <div onClickCapture={stopInteractiveRowClick}>
        <DataGrid
          emptyMessage={emptyState}
          onRowClick={handleRowClick}
          recordCount={typed?.data.length ?? 0}
          table={table}
          tableClassNames={{ base: "min-w-[1200px]" }}
        >
          <DataGridContainer>
            <DataGridScrollArea>
              <DataGridTable />
            </DataGridScrollArea>
          </DataGridContainer>
          <CursorPagination
            cursor={{
              pageSize: pagination.perPage,
              hasMore: typed?.has_more ?? false,
              canGoBack: pagination.canGoBack,
              onNext: () => {
                if (typed?.next_cursor) {
                  pagination.goNext(typed.next_cursor);
                }
              },
              onPrev: pagination.goPrev,
              onPageSizeChange: pagination.setPerPage,
            }}
            table={table}
          />
          <DataGridSelectionBar
            actions={[
              ...(selectedIds.length === 1
                ? [
                    {
                      label: "View",
                      icon: EyeIcon,
                      onClick: () => {
                        const workflow = table
                          .getRowModel()
                          .rows.find((r) => r.id === selectedIds[0])?.original;
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
                  for (const id of selectedIds) {
                    triggerWorkflow.mutate({ workflowId: id });
                  }
                },
              },
              {
                label: "Pause",
                icon: PauseActionIcon,
                onClick: () => {
                  for (const id of selectedIds) {
                    pauseWorkflow.mutate({ workflowId: id });
                  }
                },
              },
              {
                label: "Resume",
                icon: PlayActionIcon,
                onClick: () => {
                  for (const id of selectedIds) {
                    resumeWorkflow.mutate({ workflowId: id });
                  }
                },
              },
            ]}
          />
        </DataGrid>
      </div>

      <WorkflowDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        workflow={selectedWorkflow}
      />
    </Shell>
  );
}
