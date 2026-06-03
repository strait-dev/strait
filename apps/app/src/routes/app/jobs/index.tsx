import { HugeiconsIcon } from "@hugeicons/react";
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
import { createJobColumns } from "@/components/tables/jobs-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, PaginatedResponse } from "@/hooks/api/types";
import {
  jobsQueryOptions,
  usePauseJob,
  useResumeJob,
  useTriggerJob,
} from "@/hooks/api/use-jobs";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import {
  BriefcaseIcon,
  EyeIcon,
  PauseActionIcon,
  PlayActionIcon,
  SearchIcon,
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
  const triggerJob = useTriggerJob();
  const pauseJob = usePauseJob();
  const resumeJob = useResumeJob();
  const [query, setQuery] = useState(search.query ?? "");

  useEffect(() => {
    setQuery(search.query ?? "");
  }, [search.query]);

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

  const table = useReactTable({
    data: filteredData,
    columns: createJobColumns({
      onView: (job) => {
        setSelectedJob(job);
        setSheetOpen(true);
      },
      onTrigger: (job) => triggerJob.mutate({ id: job.id }),
      onPauseResume: (job) => {
        if (job.enabled) {
          pauseJob.mutate({ id: job.id });
        } else {
          resumeJob.mutate({ id: job.id });
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
      </div>

      <div onClickCapture={stopInteractiveRowClick}>
        <DataGrid
          emptyMessage={emptyState}
          onRowClick={handleRowClick}
          recordCount={table.getRowModel().rows.length}
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
              {
                label: "Trigger",
                icon: PlayActionIcon,
                onClick: () => {
                  for (const id of selectedIds) {
                    triggerJob.mutate({ id });
                  }
                },
              },
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
            ]}
          />
        </DataGrid>
      </div>

      <JobDetailSheet
        job={selectedJob}
        onOpenChange={setSheetOpen}
        open={sheetOpen}
      />
    </Shell>
  );
}
