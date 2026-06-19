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
import JobDetailSheet from "@/components/dashboard/job-detail-sheet";
import JobFormDialog from "@/components/jobs/job-form-dialog";
import { createJobColumns } from "@/components/tables/jobs-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import {
  jobsQueryOptions,
  useDeleteJob,
  usePauseJob,
  useResumeJob,
  useTriggerJob,
} from "@/hooks/api/use-jobs";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  BriefcaseIcon,
  EyeIcon,
  PauseActionIcon,
  PlayActionIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
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

export const Route = createFileRoute("/app/jobs/")({
  head: () => ({ meta: [{ title: "Jobs · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
    query: search.query,
  }),
  loader: ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      context.queryClient
        .prefetchQuery(
          jobsQueryOptions({
            limit: deps.limit,
            cursor: deps.cursor,
            search: deps.query,
          })
        )
        .catch(() => undefined);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: JobsPage,
});

function JobsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  usePageEvent("jobs_list_viewed", {
    has_query: !!search.query,
    has_status_filter: (search.status?.length ?? 0) > 0,
    status_filter_count: search.status?.length ?? 0,
  });
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editingJob, setEditingJob] = useState<Job | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Job | null>(null);
  const triggerJob = useTriggerJob();
  const pauseJob = usePauseJob();
  const resumeJob = useResumeJob();
  const deleteJob = useDeleteJob();
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const [query, setQuery] = useState(search.query ?? "");

  useEffect(() => {
    setQuery(search.query ?? "");
  }, [search.query]);

  useEffect(() => {
    if (search.create === "1") {
      setEditingJob(null);
      setFormOpen(true);
      navigate({
        search: (prev) => ({ ...prev, create: undefined }),
        replace: true,
      });
    }
  }, [navigate, search.create]);

  const { data } = useQuery({
    ...jobsQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
      search: search.query,
    }),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const typed = data as PaginatedResponse<Job> | undefined;

  const filteredData = useMemo(() => {
    let jobs = hasProject ? (typed?.data ?? []) : [];
    const normalizedQuery = query.trim().toLowerCase();
    if (normalizedQuery) {
      jobs = jobs.filter((job: Job) =>
        [job.name, job.slug, job.description]
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
  }, [typed, selectedStatuses, hasProject, query]);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const tableData = useHydratedTableData(filteredData);

  const table = useReactTable({
    data: tableData.data,
    columns: createJobColumns({
      onView: (job) => {
        setSelectedJob(job);
        setSheetOpen(true);
      },
      onEdit: permissions.canWriteJobs
        ? (job) => {
            setEditingJob(job);
            setFormOpen(true);
          }
        : undefined,
      onTrigger: permissions.canTriggerJobs
        ? (job) => triggerJob.mutate({ id: job.id })
        : undefined,
      onPauseResume: permissions.canWriteJobs
        ? (job) => {
            if (job.paused || !job.enabled) {
              resumeJob.mutate({ id: job.id });
            } else {
              pauseJob.mutate({ id: job.id });
            }
          }
        : undefined,
      onDelete: permissions.canWriteJobs
        ? (job) => setDeleteTarget(job)
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

  function handleRowClick(job: Job) {
    setSelectedJob(job);
    setSheetOpen(true);
  }

  const emptyState = hasProject ? (
    <Empty className="h-[300px]">
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="size-6 text-foreground"
            icon={BriefcaseIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No jobs found</EmptyTitle>
        <EmptyDescription>
          No jobs yet. Deploy your first job using the Strait SDK.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Jobs</h1>
      <div className="flex items-center gap-3 pb-2.5">
        <InputWithStartIcon
          aria-label="Search"
          containerClassName="w-full max-w-[500px]"
          icon={<HugeiconsIcon icon={SearchIcon} size={16} />}
          onChange={(e) => {
            const nextQuery = e.target.value;
            setQuery(nextQuery);
            navigate({
              search: (prev) => ({
                ...prev,
                query: nextQuery || undefined,
                cursor: undefined,
              }),
            });
          }}
          placeholder="Search jobs"
          value={query}
        />

        <FacetedStatusFilter
          onChange={handleStatusFiltersChange}
          options={ENABLED_STATUS_OPTIONS.map((status) => ({
            label: status,
            value: status,
          }))}
          values={selectedStatuses}
        />

        {permissions.canWriteJobs && (
          <Button
            className="shrink-0"
            render={(props) => (
              <a {...props} href="/app/jobs?create=1">
                {props.children}
              </a>
            )}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create job
          </Button>
        )}
      </div>

      <section aria-label="Jobs">
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
                        const job = table
                          .getRowModel()
                          .rows.find((r) => r.id === selectedIds[0])?.original;
                        if (job) {
                          handleRowClick(job);
                        }
                      },
                    },
                  ]
                : []),
              ...(permissions.canTriggerJobs
                ? [
                    {
                      label: "Trigger",
                      icon: PlayActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          triggerJob.mutate({ id });
                        }
                      },
                    },
                  ]
                : []),
              ...(permissions.canWriteJobs
                ? [
                    {
                      label: "Pause",
                      icon: PauseActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          pauseJob.mutate({ id });
                        }
                      },
                    },
                    {
                      label: "Resume",
                      icon: PlayActionIcon,
                      onClick: () => {
                        for (const id of selectedIds) {
                          resumeJob.mutate({ id });
                        }
                      },
                    },
                  ]
                : []),
            ]}
          />
        </DataGrid>
      </section>

      <JobDetailSheet
        job={selectedJob}
        onOpenChange={setSheetOpen}
        open={sheetOpen}
      />

      <JobFormDialog
        job={editingJob}
        kind="job"
        onOpenChange={setFormOpen}
        onSaved={(job) => {
          setSelectedJob((current) => (current?.id === job.id ? job : current));
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
            <AlertDialogTitle>Delete job?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes {deleteTarget?.name}. Existing run
              history remains available from the runs view.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteJob.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteJob.mutate(deleteTarget.id, {
                    onSuccess: () => setDeleteTarget(null),
                  });
                }
              }}
            >
              <HugeiconsIcon className="size-4" icon={TrashIcon} />
              Delete job
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Shell>
  );
}
