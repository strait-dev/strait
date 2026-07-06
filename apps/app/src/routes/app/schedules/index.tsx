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
import { StatusBadge } from "@strait/ui/components/status-badge";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import { FacetedStatusFilter } from "@/components/common/faceted-status-filter";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import ScheduleDetailSheet from "@/components/dashboard/schedule-detail-sheet";
import JobFormDialog from "@/components/jobs/job-form-dialog";
import {
  getResourceTableInitialState,
  RESOURCE_TABLE_CLASS_NAMES,
  RESOURCE_TABLE_EMPTY_CLASS_NAME,
} from "@/components/tables/resource-table";
import { createScheduleColumns } from "@/components/tables/schedules-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import {
  schedulesQueryOptions,
  useDeleteSchedule,
  usePauseSchedule,
  useResumeSchedule,
  useTriggerSchedule,
} from "@/hooks/api/use-schedules";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useAppReactTable } from "@/hooks/use-app-react-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import { usePermissionGatedCreateQuery } from "@/hooks/use-permission-gated-create-query";
import {
  CalendarIcon,
  EyeIcon,
  PauseActionIcon,
  PlayActionIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
} from "@/lib/icons";
import { scheduleResourcePermissions } from "@/lib/resource-permissions";
import { seo } from "@/lib/seo";
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

const EMPTY_ARRAY: never[] = [];

export const Route = createFileRoute("/app/schedules/")({
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
        schedulesQueryOptions({
          limit: deps.limit,
          cursor: deps.cursor,
          search: deps.query,
        })
      );
    }
    return { hasProject, session };
  },
  head: () => ({ meta: seo({ title: "Schedules" }) }),
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: SchedulesPage,
});

function SchedulesPage() {
  usePageEvent("schedules_list_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const [selectedSchedule, setSelectedSchedule] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editingSchedule, setEditingSchedule] = useState<Job | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Job | null>(null);
  const triggerSchedule = useTriggerSchedule();
  const pauseSchedule = usePauseSchedule();
  const resumeSchedule = useResumeSchedule();
  const deleteSchedule = useDeleteSchedule();
  const { isHydrated: permissionsHydrated, permissions } =
    useProjectPermissions(session.user.activeProjectId);
  const actionPermissions = scheduleResourcePermissions(permissions);

  const openCreateDialog = () => {
    setEditingSchedule(null);
    setFormOpen(true);
  };

  usePermissionGatedCreateQuery({
    canCreate: actionPermissions.canCreate,
    create: search.create,
    isReady: permissionsHydrated,
    navigate,
    openCreateDialog,
  });

  const { data } = useQuery({
    ...schedulesQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
      search: search.query,
    }),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? EMPTY_ARRAY;

  const typed = data as PaginatedResponse<Job> | undefined;

  const filteredData = (() => {
    let jobs = hasProject ? (typed?.data ?? []) : [];
    const normalizedQuery = search.query?.trim().toLowerCase();
    if (normalizedQuery) {
      jobs = jobs.filter((job: Job) =>
        [job.name, job.slug, job.description, job.cron]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(normalizedQuery))
      );
    }
    if (selectedStatuses.length === 0) {
      return jobs;
    }
    return jobs.filter((job: Job) => {
      if (selectedStatuses.includes("Enabled") && job.enabled) {
        return true;
      }
      if (selectedStatuses.includes("Disabled") && !job.enabled) {
        return true;
      }
      return false;
    });
  })();

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const tableData = useHydratedTableData(filteredData);

  const table = useAppReactTable({
    data: tableData.data,
    columns: createScheduleColumns({
      onView: (schedule) => {
        setSelectedSchedule(schedule);
        setSheetOpen(true);
      },
      onEdit: actionPermissions.canEdit
        ? (schedule) => {
            setEditingSchedule(schedule);
            setFormOpen(true);
          }
        : undefined,
      onTrigger: actionPermissions.canTrigger
        ? (schedule) => triggerSchedule.mutate({ id: schedule.id })
        : undefined,
      onPauseResume: actionPermissions.canPauseResume
        ? (schedule) => {
            if (schedule.paused || !schedule.enabled) {
              resumeSchedule.mutate({ id: schedule.id });
            } else {
              pauseSchedule.mutate({ id: schedule.id });
            }
          }
        : undefined,
      onDelete: actionPermissions.canDelete
        ? (schedule) => setDeleteTarget(schedule)
        : undefined,
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    initialState: getResourceTableInitialState(),
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

  const enabled = filteredData.filter((schedule) => schedule.enabled).length;
  const paused = filteredData.filter(
    (schedule) => schedule.paused || !schedule.enabled
  ).length;
  const summary = { enabled, paused };

  function handleStatusFiltersChange(statuses: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        status: statuses.length > 0 ? statuses : undefined,
        cursor: undefined,
      }),
    });
  }

  const emptyState = hasProject ? (
    <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="size-6 text-foreground"
            icon={CalendarIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No schedules found</EmptyTitle>
        <EmptyDescription>
          No schedules yet. Add a cron expression to a job to create a schedule.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Schedules</h1>
      {filteredData.length > 0 && (
        <div className="flex flex-wrap items-center gap-4 pb-3 text-sm">
          <span className="text-muted-foreground">
            {filteredData.length} schedule
            {filteredData.length === 1 ? "" : "s"}
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="enabled" />
            <span className="tabular-nums">{summary.enabled}</span>
            <span className="text-muted-foreground">enabled</span>
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="paused" />
            <span className="tabular-nums">{summary.paused}</span>
            <span className="text-muted-foreground">paused</span>
          </span>
        </div>
      )}

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
          placeholder="Search schedules"
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

        {actionPermissions.canCreate && (
          <Button
            className="shrink-0"
            render={<Link search={{ create: "1" }} to="/app/schedules" />}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create schedule
          </Button>
        )}
      </div>

      <section aria-label="Schedules">
        <DataGrid
          emptyMessage={emptyState}
          loading={tableData.isLoading}
          onRowClick={(schedule) => {
            setSelectedSchedule(schedule);
            setSheetOpen(true);
          }}
          recordCount={
            tableData.isHydrated ? table.getRowModel().rows.length : 0
          }
          table={table}
          tableClassNames={RESOURCE_TABLE_CLASS_NAMES}
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
                        const schedule = table
                          .getRowModel()
                          .rows.find((r) => r.id === selectedIds[0])?.original;
                        if (schedule) {
                          setSelectedSchedule(schedule);
                          setSheetOpen(true);
                        }
                      },
                    },
                    ...(actionPermissions.canDelete
                      ? [
                          {
                            label: "Delete",
                            icon: TrashIcon,
                            onClick: () => {
                              const schedule = table
                                .getRowModel()
                                .rows.find(
                                  (r) => r.id === selectedIds[0]
                                )?.original;
                              if (schedule) {
                                setDeleteTarget(schedule);
                              }
                            },
                          },
                        ]
                      : []),
                  ]
                : []),
              ...(actionPermissions.canTrigger
                ? [
                    {
                      label: "Trigger",
                      icon: PlayActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          triggerSchedule.mutate({ id });
                        }
                      },
                    },
                  ]
                : []),
              ...(actionPermissions.canPauseResume
                ? [
                    {
                      label: "Pause",
                      icon: PauseActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          pauseSchedule.mutate({ id });
                        }
                      },
                    },
                    {
                      label: "Resume",
                      icon: PlayActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          resumeSchedule.mutate({ id });
                        }
                      },
                    },
                  ]
                : []),
            ]}
          />
        </DataGrid>
      </section>

      <ScheduleDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        schedule={selectedSchedule}
      />

      <JobFormDialog
        job={editingSchedule}
        kind="schedule"
        onOpenChange={setFormOpen}
        onSaved={(schedule) => {
          setSelectedSchedule((current) =>
            current?.id === schedule.id ? schedule : current
          );
        }}
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
            <AlertDialogTitle>Delete schedule?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes {deleteTarget?.name}. Existing run
              history remains available from the runs view.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteSchedule.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteSchedule.mutate(deleteTarget.id, {
                    onSuccess: () => setDeleteTarget(null),
                  });
                }
              }}
            >
              <HugeiconsIcon className="size-4" icon={TrashIcon} />
              Delete schedule
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Shell>
  );
}
