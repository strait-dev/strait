import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { lazy, Suspense, useMemo, useState } from "react";
import { z } from "zod/v4";
import {
  type AgentListRow,
  filterAgents,
} from "@/components/agents/agent-list-utils";
import CreateAgentDialog from "@/components/agents/create-agent-dialog";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { agentColumns } from "@/components/tables/agents-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import {
  agentListRowsQueryOptions,
  agentTopologyQueryOptions,
} from "@/hooks/api/use-agents";
import { SearchIcon, SparklesIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

const AgentTopologyFlow = lazy(
  () => import("@/components/agents/agent-topology-flow")
);

export const searchSchema = z.object({
  query: z.string().optional(),
});

export const Route = createFileRoute("/app/agents/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(agentListRowsQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: AgentsPage,
});

function AgentsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useQuery({
    ...agentListRowsQueryOptions(),
    enabled: hasProject,
  });
  const agents = data as AgentListRow[] | undefined;
  const { data: topology } = useQuery({
    ...agentTopologyQueryOptions(),
    enabled: hasProject,
  });

  const filteredData = useMemo(
    () => filterAgents(hasProject ? (agents ?? []) : [], search.query),
    [agents, hasProject, search.query]
  );

  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const table = useReactTable({
    data: filteredData,
    columns: agentColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined }),
      }),
    getRowId: (row) => row.id,
  });

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No agents yet. Create and deploy your first managed agent from the dashboard."
      hideButton
      icon={
        <HugeiconsIcon className="size-6 text-foreground" icon={SparklesIcon} />
      }
      title="No agents found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <div className="grid gap-4 pb-2 md:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Use Agents For</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-muted-foreground text-sm">
            <p>
              LLM-centric orchestration, token streaming, lightweight tools, and
              fast interactive loops.
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Use Jobs For</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-muted-foreground text-sm">
            <p>
              Heavy compute, long-running work, container-backed dependencies,
              and durable Fly Machine execution.
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Best Combined Pattern</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-muted-foreground text-sm">
            <p>
              Let agents plan and stream, then trigger a Job or Workflow when
              the work leaves the low-latency path.
            </p>
            <Link
              className="inline-flex text-primary text-sm underline underline-offset-4"
              to="/app/jobs"
            >
              Open Jobs
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Product Boundary</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-muted-foreground text-sm">
            <p>
              Agents and Jobs share the same project and account, but they are
              separate execution products with different runtime and cost
              envelopes.
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="flex flex-col gap-3 pb-2.5 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative w-full max-w-[500px]">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
            aria-label="Search agents"
            className="pl-9"
            onChange={(e) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: e.target.value || undefined,
                }),
              })
            }
            placeholder="Search agents by name, slug, or model..."
            value={search.query ?? ""}
          />
        </div>
        {hasProject && session.user.activeProjectId ? (
          <Button onClick={() => setCreateDialogOpen(true)} type="button">
            <HugeiconsIcon className="size-4" icon={SparklesIcon} />
            Create agent
          </Button>
        ) : null}
      </div>

      <DataTable<AgentListRow> emptyState={emptyState} table={table} />

      {hasProject && topology && topology.edges.length > 0 && (
        <Card className="mt-6">
          <CardHeader>
            <CardTitle className="text-sm">Agent Topology</CardTitle>
          </CardHeader>
          <CardContent>
            <Suspense
              fallback={
                <div className="flex h-48 items-center justify-center text-muted-foreground text-sm">
                  Loading topology...
                </div>
              }
            >
              <AgentTopologyFlow
                edges={topology.edges}
                nodes={topology.nodes}
              />
            </Suspense>
          </CardContent>
        </Card>
      )}

      {hasProject && session.user.activeProjectId ? (
        <CreateAgentDialog
          onOpenChange={setCreateDialogOpen}
          open={createDialogOpen}
          projectId={session.user.activeProjectId}
        />
      ) : null}
    </Shell>
  );
}
