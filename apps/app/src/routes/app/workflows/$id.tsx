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
  type ColumnDef,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";
import TableEmptyState from "@/components/common/table-empty-state";
import { StatusBadge } from "@/components/dashboard/status-badge";
import { WorkflowDAG } from "@/components/dashboard/workflow-dag";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { WorkflowRun, WorkflowStep } from "@/hooks/api/types";
import {
  workflowQueryOptions,
  workflowRunsQueryOptions,
  workflowStepsQueryOptions,
} from "@/hooks/api/use-workflows";
import {
  ActivityIcon,
  ClockIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/workflows/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(workflowQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowStepsQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowRunsQueryOptions(params.id)),
    ]);
  },
  component: WorkflowDetailPage,
});

const workflowRunColumns: ColumnDef<WorkflowRun>[] = [
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.id.slice(0, 8)}</span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "triggered_by",
    header: "Trigger",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.triggered_by}
      </Badge>
    ),
  },
  {
    accessorKey: "workflow_version",
    header: "Version",
    cell: ({ row }) => (
      <code className="text-xs">v{row.original.workflow_version}</code>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Started",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
];

function WorkflowDetailPage() {
  const { id } = Route.useParams();
  const { data: workflow } = useSuspenseQuery(workflowQueryOptions(id));
  const { data: apiSteps } = useSuspenseQuery(workflowStepsQueryOptions(id));
  const { data: runs } = useSuspenseQuery(workflowRunsQueryOptions(id));
  const [activeTab, setActiveTab] = useState("dag");

  // Map API steps to the shape WorkflowDAG expects
  const dagSteps = (apiSteps ?? []).map((s: WorkflowStep) => ({
    id: s.id,
    name: s.step_ref,
    type: s.step_type,
    status: "pending" as const,
    dependencies: s.depends_on,
  }));

  const runsTable = useReactTable({
    data: runs ?? [],
    columns: workflowRunColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  if (!workflow) {
    return (
      <Shell>
        <div className="flex items-center justify-center py-20">
          <p className="text-muted-foreground">Workflow not found.</p>
        </div>
      </Shell>
    );
  }

  return (
    <Shell>
      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-2xl tracking-tight">
              {workflow.name}
            </h1>
            <StatusBadge
              showDot
              status={workflow.enabled ? "completed" : "paused"}
            />
          </div>
          {workflow.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {workflow.description}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Button size="sm">
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
          <Button size="sm" variant="outline">
            <HugeiconsIcon
              className="mr-1.5"
              icon={workflow.enabled ? PauseActionIcon : PlayActionIcon}
              size={14}
            />
            {workflow.enabled ? "Pause" : "Resume"}
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="dag">DAG</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6" value="dag">
          <WorkflowDAG steps={dagSteps} />
        </TabsContent>

        <TabsContent className="mt-6" value="runs">
          <DataTable
            emptyState={
              <TableEmptyState
                description="No runs found for this workflow."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="size-6 text-primary"
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
            <h3 className="font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h3>
            <div className="space-y-2.5">
              <ConfigRow
                icon={ClockIcon}
                label="Timeout"
                value={`${workflow.timeout_secs}s`}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Max Concurrent Runs"
                value={String(workflow.max_concurrent_runs)}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Max Parallel Steps"
                value={String(workflow.max_parallel_steps)}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Schedule"
                value={workflow.cron || "Manual"}
              />
              {workflow.cron_timezone && (
                <ConfigRow
                  icon={ClockIcon}
                  label="Timezone"
                  value={workflow.cron_timezone}
                />
              )}
              <ConfigRow
                icon={RefreshIcon}
                label="Skip If Running"
                value={workflow.skip_if_running ? "Yes" : "No"}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Version Policy"
                value={workflow.version_policy}
              />
            </div>
          </div>

          {/* Tags */}
          {workflow.tags && Object.keys(workflow.tags).length > 0 && (
            <div className="rounded-md border p-4">
              <h3 className="mb-3 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon icon={TagIcon} size={12} />
                Tags
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(workflow.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {val}
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

function ConfigRow({
  icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <HugeiconsIcon className="text-muted-foreground" icon={icon} size={14} />
      <span className="text-muted-foreground">{label}</span>
      <span className="ml-auto font-mono text-xs">{value}</span>
    </div>
  );
}
