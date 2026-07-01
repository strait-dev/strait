import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";

import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ConfigRow } from "@strait/ui/components/config-row";
import {
  DataGrid,
  DataGridContainer,
  DataGridPagination,
  DataGridScrollArea,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useEffect, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import { createRunColumns } from "@/components/tables/runs-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, JobRun, PaginatedResponse } from "@/hooks/api/types";
import {
  jobQueryOptions,
  usePauseJob,
  useResumeJob,
  useTriggerJob,
} from "@/hooks/api/use-jobs";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  ActivityIcon,
  CalendarIcon,
  ClockIcon,
  GlobeIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/schedules/$id")({
  head: () => ({ meta: [{ title: "Schedule · Strait" }] }),
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await Promise.all([
      context.queryClient.ensureQueryData(jobQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        runsQueryOptions({ job_id: params.id })
      ),
    ]);
    return { session };
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: ScheduleDetailPage,
});

function ScheduleDetailPage() {
  const { id } = Route.useParams();
  const { session } = Route.useLoaderData();
  usePageEvent("schedule_detail_viewed", { schedule_id: id });
  const { data: job } = useSuspenseQuery(jobQueryOptions(id)) as {
    data: Job | undefined;
  };
  const { data: runs } = useSuspenseQuery(runsQueryOptions({ job_id: id })) as {
    data: PaginatedResponse<JobRun> | undefined;
  };
  const [activeTab, setActiveTab] = useState("history");
  const [isHydrated, setIsHydrated] = useState(false);
  const triggerJob = useTriggerJob();
  const pauseJob = usePauseJob();
  const resumeJob = useResumeJob();
  const projectId = session.user.activeProjectId || job?.project_id || "active";
  const { permissions } = useProjectPermissions(projectId);
  const tableData = useHydratedTableData(runs?.data ?? []);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  const runsTable = useReactTable({
    data: tableData.data,
    columns: createRunColumns(),
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  if (!job) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/schedules" entity="Schedule" />
      </Shell>
    );
  }

  const isPaused = job.paused || !job.enabled;
  const canMutateSchedule =
    permissions.canWriteJobs || job.created_by === session.user.id;
  const canTriggerSchedule = permissions.canTriggerJobs || canMutateSchedule;

  return (
    <Shell>
      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-xl tracking-tight">
              {job.name}
            </h1>
            <StatusBadge showDot status={isPaused ? "paused" : "completed"} />
          </div>
          {job.description && (
            <p className="text-muted-foreground text-sm">{job.description}</p>
          )}
        </div>
        <div className="flex gap-2">
          {canTriggerSchedule && (
            <Button
              disabled={!isHydrated || triggerJob.isPending}
              onClick={() => triggerJob.mutate({ id })}
            >
              <HugeiconsIcon
                className="mr-1.5 size-3.5"
                icon={PlayActionIcon}
              />
              Trigger
            </Button>
          )}
          {canMutateSchedule && (
            <Button
              disabled={
                !isHydrated || pauseJob.isPending || resumeJob.isPending
              }
              onClick={() =>
                isPaused ? resumeJob.mutate({ id }) : pauseJob.mutate({ id })
              }
              variant="outline"
            >
              <HugeiconsIcon
                className="mr-1.5 size-3.5"
                icon={isPaused ? PlayActionIcon : PauseActionIcon}
              />
              {isPaused ? "Resume" : "Pause"}
            </Button>
          )}
        </div>
      </div>

      {/* Cron Display Card */}
      <Card>
        <CardContent>
          <div className="flex items-center gap-3">
            <HugeiconsIcon
              className="text-muted-foreground"
              icon={CalendarIcon}
              size={20}
            />
            <div>
              <p className="font-medium text-muted-foreground text-xs uppercase">
                Cron Schedule
              </p>
              <Badge mono size="sm" variant="secondary-light">
                {job.cron || "No schedule"}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="history">Run History</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6" value="history">
          <DataGrid
            emptyMessage={
              <Empty className="h-[300px]">
                <EmptyHeader>
                  <EmptyMedia media="icon" size="lg">
                    <HugeiconsIcon
                      className="size-6 text-foreground"
                      icon={ActivityIcon}
                    />
                  </EmptyMedia>
                  <EmptyTitle>No runs found</EmptyTitle>
                  <EmptyDescription>
                    No runs yet. Runs will appear here each time the schedule
                    fires.
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            }
            loading={tableData.isLoading}
            recordCount={tableData.isHydrated ? (runs?.data.length ?? 0) : 0}
            table={runsTable}
            tableClassNames={{ base: "min-w-[1200px]" }}
          >
            <DataGridContainer>
              <DataGridScrollArea>
                <DataGridTable />
              </DataGridScrollArea>
              <DataGridPagination />
            </DataGridContainer>
          </DataGrid>
        </TabsContent>

        <TabsContent className="mt-6 space-y-6" value="settings">
          {/* Configuration */}
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
                Configuration
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5">
              <ConfigRow
                icon={GlobeIcon}
                label="Endpoint"
                value={job.endpoint_url || "-"}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Schedule"
                value={job.cron || "Manual"}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Retry"
                value={`${job.max_attempts} attempts`}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Timeout"
                value={`${job.timeout_secs}s`}
              />
            </CardContent>
          </Card>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase tracking-wider">
                  <HugeiconsIcon icon={TagIcon} size={12} />
                  Tags
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-1.5">
                {Object.entries(job.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {String(val)}
                  </Badge>
                ))}
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
