import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useMemo, useState } from "react";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import { summarizeAgentRuns } from "@/components/agents/agent-detail-utils";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import StatusBadge from "@/components/dashboard/status-badge";
import { runColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { Agent, DisplayStatus, JobRun } from "@/hooks/api/types";
import {
  agentQueryOptions,
  agentRunsQueryOptions,
  useDeployAgent,
  useRunAgent,
} from "@/hooks/api/use-agents";
import {
  ActivityIcon,
  BriefcaseIcon,
  ClockIcon,
  PlayActionIcon,
  RefreshIcon,
  SparklesIcon,
  TagIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/agents/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(agentQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        agentRunsQueryOptions(params.id, { limit: 50 })
      ),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: AgentDetailPage,
});

const formatDateTime = (value: string | undefined) =>
  value ? new Date(value).toLocaleString() : "-";

const StatCard = ({ label, value }: { label: string; value: string | number }) => {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-muted-foreground text-sm">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="font-normal text-2xl tracking-tight">{value}</p>
      </CardContent>
    </Card>
  );
};

function AgentDetailPage() {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { data: agent } = useSuspenseQuery(agentQueryOptions(id)) as {
    data: Agent | undefined;
  };
  const { data: agentRuns } = useSuspenseQuery(
    agentRunsQueryOptions(id, { limit: 50 })
  ) as {
    data: JobRun[] | undefined;
  };
  const deployAgent = useDeployAgent();
  const runAgent = useRunAgent();
  const [activeTab, setActiveTab] = useState("overview");
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const runs = agentRuns ?? [];
  const summary = useMemo(() => summarizeAgentRuns(runs), [runs]);

  const table = useReactTable({
    data: runs,
    columns: runColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { rowSelection },
    getRowId: (row) => row.id,
  });

  if (!agent) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/agents" entity="Agent" />
      </Shell>
    );
  }

  const latestRun = summary.latestRun;

  return (
    <Shell>
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 flex-col gap-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate font-normal text-xl tracking-tight sm:text-2xl">
              {agent.name}
            </h1>
            <Badge variant="outline">{agent.model}</Badge>
            <code className="rounded bg-muted px-2 py-0.5 font-mono text-xs">
              {agent.slug}
            </code>
          </div>
          {agent.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {agent.description}
            </p>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            disabled={deployAgent.isPending}
            onClick={() => deployAgent.mutate({ agentId: agent.id })}
            size="sm"
            type="button"
            variant="outline"
          >
            <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
            {deployAgent.isPending ? "Deploying..." : "Deploy"}
          </Button>
          <Button
            disabled={runAgent.isPending}
            onClick={() =>
              runAgent.mutate(
                { agentId: agent.id },
                {
                  onSuccess: (run) =>
                    navigate({
                      params: { id: (run as JobRun).id },
                      to: "/app/runs/$id",
                    }),
                }
              )
            }
            size="sm"
            type="button"
          >
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            {runAgent.isPending ? "Starting..." : "Run agent"}
          </Button>
        </div>
      </div>

      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="config">Config</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="overview">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard label="Total Runs" value={summary.totalRuns} />
            <StatCard label="Successful" value={summary.successfulRuns} />
            <StatCard label="Failed" value={summary.failedRuns} />
            <StatCard label="Active" value={summary.activeRuns} />
          </div>

          <div className="grid gap-6 lg:grid-cols-[1.4fr_1fr]">
            <Card>
              <CardHeader>
                <CardTitle>Latest Activity</CardTitle>
              </CardHeader>
              <CardContent>
                {latestRun ? (
                  <div className="space-y-4">
                    <div className="flex items-center gap-2">
                      <StatusBadge
                        showDot
                        status={latestRun.status as DisplayStatus}
                      />
                      <span className="font-mono text-sm">{latestRun.id}</span>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <ConfigRow
                        icon={ClockIcon}
                        label="Started"
                        value={formatDateTime(latestRun.started_at ?? undefined)}
                      />
                      <ConfigRow
                        icon={ClockIcon}
                        label="Created"
                        value={formatDateTime(latestRun.created_at)}
                      />
                      <ConfigRow
                        icon={ActivityIcon}
                        label="Attempt"
                        value={String(latestRun.attempt)}
                      />
                      <ConfigRow
                        icon={SparklesIcon}
                        label="Trigger"
                        value={latestRun.triggered_by}
                      />
                    </div>
                    <Button
                      render={
                        <Link params={{ id: latestRun.id }} to="/app/runs/$id" />
                      }
                      size="sm"
                      variant="outline"
                    >
                      View run details
                    </Button>
                  </div>
                ) : (
                  <TableEmptyState
                    description="This agent has not been run yet. Deploy it and trigger the first local run."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={SparklesIcon}
                      />
                    }
                    title="No runs yet"
                  />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Definition</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <ConfigRow icon={TagIcon} label="Slug" value={agent.slug} />
                <ConfigRow icon={SparklesIcon} label="Model" value={agent.model} />
                <ConfigRow
                  icon={BriefcaseIcon}
                  label="Backing job"
                  value={agent.job_id}
                />
                <ConfigRow
                  icon={ClockIcon}
                  label="Created"
                  value={formatDateTime(agent.created_at)}
                />
                <ConfigRow
                  icon={ClockIcon}
                  label="Updated"
                  value={formatDateTime(agent.updated_at)}
                />
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="runs">
          {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
          <div
            className="[&_tbody_tr]:cursor-pointer"
            onClick={(event) => {
              const target = event.target as HTMLElement;
              if (target.closest("a, button")) {
                return;
              }
              const row = target.closest("tr[data-row-index]");
              if (!row) {
                return;
              }
              const index = Number(row.getAttribute("data-row-index"));
              const run = runs[index];
              if (!run) {
                return;
              }
              setSelectedRun(run);
              setSheetOpen(true);
            }}
          >
            <DataTable<JobRun>
              emptyState={
                <TableEmptyState
                  description="Once this agent starts running, execution history will appear here."
                  hideButton
                  icon={
                    <HugeiconsIcon
                      className="size-6 text-foreground"
                      icon={ActivityIcon}
                    />
                  }
                  title="No recent runs"
                />
              }
              table={table}
            />
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="config">
          <Card>
            <CardHeader>
              <CardTitle>Agent Config</CardTitle>
            </CardHeader>
            <CardContent>
              <pre className="max-h-[500px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
                {agent.config
                  ? JSON.stringify(agent.config, null, 2)
                  : "No config provided"}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
