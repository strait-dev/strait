import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";

import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
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
import { useState } from "react";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import StatusBadge from "@/components/dashboard/status-badge";
import { createRunColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, JobRun, PaginatedResponse } from "@/hooks/api/types";
import { jobQueryOptions } from "@/hooks/api/use-jobs";
import { runsQueryOptions } from "@/hooks/api/use-runs";
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

export const Route = createFileRoute("/app/schedules/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(jobQueryOptions(params.id)),
      context.queryClient.ensureQueryData(runsQueryOptions()),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: ScheduleDetailPage,
});

function ScheduleDetailPage() {
  const { id } = Route.useParams();
  usePageEvent("schedule_detail_viewed", { schedule_id: id });
  const { data: job } = useSuspenseQuery(jobQueryOptions(id)) as {
    data: Job | undefined;
  };
  const { data: runs } = useSuspenseQuery(runsQueryOptions()) as {
    data: PaginatedResponse<JobRun> | undefined;
  };
  const [activeTab, setActiveTab] = useState("history");

  const runsTable = useReactTable({
    data: runs?.data ?? [],
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

  return (
    <Shell>
      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-xl tracking-tight">
              {job.name}
            </h1>
            <StatusBadge
              showDot
              status={job.enabled ? "completed" : "paused"}
            />
          </div>
          {job.description && (
            <p className="text-muted-foreground text-sm">{job.description}</p>
          )}
        </div>
        <div className="flex gap-2">
          <Button>
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
          <Button variant="outline">
            <HugeiconsIcon
              className="mr-1.5"
              icon={job.enabled ? PauseActionIcon : PlayActionIcon}
              size={14}
            />
            {job.enabled ? "Pause" : "Resume"}
          </Button>
        </div>
      </div>

      {/* Cron Display Card */}
      <div className="rounded-md border p-4">
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
            <code className="font-normal text-sm">
              {job.cron || "No schedule"}
            </code>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="history">Run History</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6" value="history">
          <DataTable
            emptyState={
              <TableEmptyState
                description="No runs found for this schedule."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="size-6 text-foreground"
                    icon={ActivityIcon}
                  />
                }
                title="No runs found"
              />
            }
            table={runsTable}
          />
        </TabsContent>

        <TabsContent className="mt-6 space-y-6" value="settings">
          {/* Configuration */}
          <div className="space-y-3 rounded-md border p-4">
            <h4 className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
              Configuration
            </h4>
            <div className="space-y-2.5">
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
            </div>
          </div>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <div className="rounded-md border p-4">
              <h4 className="mb-3 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase tracking-wider">
                <HugeiconsIcon icon={TagIcon} size={12} />
                Tags
              </h4>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(job.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {String(val)}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
