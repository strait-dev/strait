import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { z } from "zod/v4";

import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import ChartTooltip from "@/components/dashboard/chart-tooltip";
import type {
  AgentAnalyticsCostSummary,
  AgentRankingRow,
  AgentTimelinePoint,
} from "@/hooks/api/use-agent-analytics";
import {
  agentAnalyticsCostsQueryOptions,
  agentTimelineQueryOptions,
  agentTopAgentsQueryOptions,
} from "@/hooks/api/use-agent-analytics";
import { formatMicroUsd } from "@/lib/format";
import { CHART_COLORS } from "@/lib/status-colors";
import type { AppRouteContext } from "@/routes/app/layout";

const searchSchema = z.object({
  days: z.coerce.number().optional().default(30),
});

export const Route = createFileRoute("/app/agents/cost-insights")({
  validateSearch: zodValidator(searchSchema),
  loader: ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: CostInsightsPage,
});

function StatCard({ label, value }: { label: string; value: string | number }) {
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
}

function CostInsightsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const { days } = Route.useSearch();
  const navigate = Route.useNavigate();

  const { data: timeline } = useQuery({
    ...agentTimelineQueryOptions(days),
    enabled: hasProject,
  });

  const { data: topAgents } = useQuery({
    ...agentTopAgentsQueryOptions(days),
    enabled: hasProject,
  });

  const { data: costs } = useQuery({
    ...agentAnalyticsCostsQueryOptions(days),
    enabled: hasProject,
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const timelineData = (timeline ?? []) as AgentTimelinePoint[];
  const topAgentsData = (topAgents ?? []) as AgentRankingRow[];
  const costData = costs as AgentAnalyticsCostSummary | null;

  const dayOptions = [7, 30, 90];

  return (
    <Shell>
      <div className="flex items-center justify-between pt-4 pb-4">
        <h1 className="font-normal text-xl tracking-tight sm:text-2xl">
          Cost Insights
        </h1>
        <div className="flex gap-1">
          {dayOptions.map((d) => (
            <button
              className={`rounded px-3 py-1.5 text-sm ${
                days === d
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground hover:bg-muted/80"
              }`}
              key={d}
              onClick={() => navigate({ search: { days: d } })}
              type="button"
            >
              {d}d
            </button>
          ))}
        </div>
      </div>

      {costData && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="Total Runs" value={costData.total_runs} />
          <StatCard
            label="Total Cost"
            value={formatMicroUsd(costData.total_cost_microusd)}
          />
          <StatCard
            label="Total Tokens"
            value={costData.total_tokens.toLocaleString()}
          />
          <StatCard
            label="Avg Cost / Run"
            value={formatMicroUsd(Math.round(costData.avg_cost_microusd))}
          />
        </div>
      )}

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Runs Over Time</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer height={280} width="100%">
              <AreaChart data={timelineData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="bucket" fontSize={12} />
                <YAxis fontSize={12} />
                <Tooltip content={<ChartTooltip />} />
                <Area
                  dataKey="completed"
                  fill={CHART_COLORS.active}
                  fillOpacity={0.2}
                  name="Completed"
                  stackId="1"
                  stroke={CHART_COLORS.active}
                  type="monotone"
                />
                <Area
                  dataKey="failed"
                  fill={CHART_COLORS.error}
                  fillOpacity={0.2}
                  name="Failed"
                  stackId="1"
                  stroke={CHART_COLORS.error}
                  type="monotone"
                />
              </AreaChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Top Agents by Cost</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer height={280} width="100%">
              <BarChart data={topAgentsData.slice(0, 8)} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis fontSize={12} type="number" />
                <YAxis
                  dataKey="agent_slug"
                  fontSize={12}
                  type="category"
                  width={120}
                />
                <Tooltip content={<ChartTooltip />} />
                <Bar
                  dataKey="cost_microusd"
                  fill={CHART_COLORS.warning}
                  name="Cost"
                  radius={[0, 4, 4, 0]}
                />
              </BarChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Top Agents</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="pb-2 font-medium">Agent</th>
                  <th className="pb-2 font-medium">Runs</th>
                  <th className="pb-2 font-medium">Cost</th>
                  <th className="pb-2 font-medium">Tokens</th>
                  <th className="pb-2 font-medium">Avg Duration</th>
                </tr>
              </thead>
              <tbody>
                {topAgentsData.map((agent) => (
                  <tr className="border-b" key={agent.agent_id}>
                    <td className="py-2 font-mono">{agent.agent_slug}</td>
                    <td className="py-2">{agent.runs}</td>
                    <td className="py-2">
                      {formatMicroUsd(agent.cost_microusd)}
                    </td>
                    <td className="py-2">
                      {agent.total_tokens.toLocaleString()}
                    </td>
                    <td className="py-2">
                      {agent.avg_duration_ms > 0
                        ? `${(agent.avg_duration_ms / 1000).toFixed(1)}s`
                        : "-"}
                    </td>
                  </tr>
                ))}
                {topAgentsData.length === 0 && (
                  <tr>
                    <td
                      className="py-8 text-center text-muted-foreground"
                      colSpan={5}
                    >
                      No agent data available for the selected period.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </Shell>
  );
}
