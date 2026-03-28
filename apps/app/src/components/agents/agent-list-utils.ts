import type { Agent, JobRun, RunStatus, RunUsage } from "@/hooks/api/types";

const ACTIVE_STATUSES: ReadonlySet<RunStatus> = new Set([
  "dequeued",
  "delayed",
  "executing",
  "queued",
  "waiting",
]);

export type AgentListRow = Agent & {
  active_runs: number;
  last_run_at?: string;
  last_run_status?: RunStatus;
  total_cost_microusd: number;
  total_runs: number;
};

type AgentRunBundle = {
  runs: JobRun[];
  usage: RunUsage[];
};

export function buildAgentListRows(
  agents: Agent[],
  runsByAgent: Record<string, AgentRunBundle>
): AgentListRow[] {
  return sortAgentsByUpdatedAt(
    agents.map((agent) => {
      const bundle = runsByAgent[agent.id] ?? { runs: [], usage: [] };
      const latestRun = bundle.runs.reduce<JobRun | null>(
        (latest, run) =>
          !latest || run.created_at > latest.created_at ? run : latest,
        null
      );

      return {
        ...agent,
        active_runs: bundle.runs.filter((run) =>
          ACTIVE_STATUSES.has(run.status as RunStatus)
        ).length,
        last_run_at: latestRun?.created_at,
        last_run_status: latestRun?.status as RunStatus | undefined,
        total_cost_microusd: bundle.usage.reduce(
          (sum, usage) => sum + usage.cost_microusd,
          0
        ),
        total_runs: bundle.runs.length,
      };
    })
  );
}

export function filterAgents<
  T extends Pick<
    AgentListRow,
    "description" | "model" | "name" | "slug" | "updated_at"
  >,
>(agents: T[], query?: string): T[] {
  const normalizedQuery = query?.trim().toLowerCase();
  if (!normalizedQuery) {
    return sortAgentsByUpdatedAt(agents);
  }

  return sortAgentsByUpdatedAt(
    agents.filter((agent) =>
      [agent.name, agent.slug, agent.model, agent.description ?? ""]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery)
    )
  );
}

export function sortAgentsByUpdatedAt<
  T extends Pick<AgentListRow, "updated_at">,
>(agents: T[]): T[] {
  return [...agents].sort(
    (left, right) =>
      new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
  );
}
