import { HugeiconsIcon } from "@hugeicons/react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridSelectionBar,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { InputWithStartIcon } from "@strait/ui/components/input-with-start-icon";
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
import { useEffect, useMemo, useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import { FacetedStatusFilter } from "@/components/common/faceted-status-filter";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import WorkflowDetailSheet from "@/components/dashboard/workflow-detail-sheet";
import { createWorkflowColumns } from "@/components/tables/workflows-columns";
import WorkflowFormDialog from "@/components/workflows/workflow-form-dialog";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { PaginatedResponse, Workflow } from "@/hooks/api/types";
import {
  useDeleteWorkflow,
  usePauseWorkflow,
  useResumeWorkflow,
  useTriggerWorkflow,
  workflowsQueryOptions,
} from "@/hooks/api/use-workflows";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  EyeIcon,
  PauseActionIcon,
  PlayActionIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  WorkflowIcon,
} from "@/lib/icons";
import { ENABLED_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

const createSearchSchema = z.preprocess(
  (value) => (value == null ? undefined : String(value)),
  z.string().optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  status: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
  create: createSearchSchema,
});

export const Route = createFileRoute("/app/workflows/")({
  head: () => ({ meta: [{ title: "Workflows · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
    query: search.query,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        workflowsQueryOptions({
          limit: deps.limit,
          cursor: deps.cursor,
          search: deps.query,
        })
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
      search: search.query,
    }),
    enabled: hasProject,
  });
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(
    null
  );
  const [sheetOpen, setSheetOpen] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Workflow | null>(null);
  const triggerWorkflow = useTriggerWorkflow();
  const pauseWorkflow = usePauseWorkflow();
  const resumeWorkflow = useResumeWorkflow();
  const deleteWorkflow = useDeleteWorkflow();
  const { isHydrated: permissionsHydrated, permissions } =
    useProjectPermissions(session.user.activeProjectId);

  useEffect(() => {
    if (search.create === "1" && permissionsHydrated) {
      if (permissions.canWriteWorkflows) {
        setFormOpen(true);
      }
      navigate({
        search: (prev) => ({ ...prev, create: undefined }),
        replace: true,
      });
    }
  }, [
    navigate,
    permissions.canWriteWorkflows,
    permissionsHydrated,
    search.create,
  ]);

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
  const tableData = useHydratedTableData(filteredData);

  const table = useReactTable({
    data: tableData.data,
    columns: createWorkflowColumns({
      onView: (workflow) => {
        setSelectedWorkflow(workflow);
        setSheetOpen(true);
      },
      onTrigger: permissions.canTriggerWorkflows
        ? (workflow) => triggerWorkflow.mutate({ workflowId: workflow.id })
        : undefined,
      onPauseResume: permissions.canWriteWorkflows
        ? (workflow) => {
            if (workflow.enabled) {
              pauseWorkflow.mutate({ workflowId: workflow.id });
            } else {
              resumeWorkflow.mutate({ workflowId: workflow.id });
            }
          }
        : undefined,
      onDelete: permissions.canWriteWorkflows
        ? (workflow) => setDeleteTarget(workflow)
        : undefined,
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

  function handleStatusFiltersChange(statuses: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        status: statuses.length > 0 ? statuses : undefined,
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
        <EmptyMedia media="icon" size="lg">
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
        <InputWithStartIcon
          aria-label="Search"
          containerClassName="w-full max-w-[500px]"
          icon={<HugeiconsIcon icon={SearchIcon} size={16} />}
          onChange={(e) =>
            navigate({
              search: (prev) => ({
                ...prev,
                query: e.target.value || undefined,
                cursor: undefined,
              }),
            })
          }
          placeholder="Search workflows"
          value={search.query ?? ""}
        />

        <FacetedStatusFilter
          onChange={handleStatusFiltersChange}
          options={ENABLED_STATUS_OPTIONS.map((status) => ({
            label: status,
            value: status,
          }))}
          values={selectedStatuses}
        />

        {permissions.canWriteWorkflows && (
          <Button
            className="shrink-0"
            render={(props) => (
              <a {...props} href="/app/workflows?create=1">
                {props.children}
              </a>
            )}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create workflow
          </Button>
        )}
      </div>

      <section aria-label="Workflows">
        <DataGrid
          emptyMessage={emptyState}
          loading={tableData.isLoading}
          onRowClick={handleRowClick}
          recordCount={
            tableData.isHydrated ? table.getRowModel().rows.length : 0
          }
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
              ...(permissions.canTriggerWorkflows
                ? [
                    {
                      label: "Trigger",
                      icon: PlayActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          triggerWorkflow.mutate({ workflowId: id });
                        }
                      },
                    },
                  ]
                : []),
              ...(permissions.canWriteWorkflows
                ? [
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
                  ]
                : []),
            ]}
          />
        </DataGrid>
      </section>

      <WorkflowDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        workflow={selectedWorkflow}
      />

      <WorkflowFormDialog
        onCreated={setSelectedWorkflow}
        onOpenChange={setFormOpen}
        open={formOpen}
      />

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
        open={!!deleteTarget}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete workflow?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes {deleteTarget?.name}. Existing workflow
              run history remains available from the runs view.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteWorkflow.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteWorkflow.mutate(deleteTarget.id, {
                    onSuccess: () => setDeleteTarget(null),
                  });
                }
              }}
            >
              <HugeiconsIcon className="size-4" icon={TrashIcon} />
              Delete workflow
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Shell>
  );
}
