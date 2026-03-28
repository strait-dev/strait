import type { JobRun, RunUsage } from "@/hooks/api/types";

export type AgentCostSummary = {
  average_cost_microusd: number;
  daily: Array<{
    cost_microusd: number;
    date: string;
    total_tokens: number;
  }>;
  latest_run_cost_microusd: number;
  providers: Array<{
    cost_microusd: number;
    provider: string;
    total_tokens: number;
  }>;
  total_cost_microusd: number;
  total_tokens: number;
  usage_records: number;
};

export function buildAgentCostSummary(
  runs: JobRun[],
  usageRecords: RunUsage[]
): AgentCostSummary {
  const providerMap = new Map<
    string,
    { cost_microusd: number; provider: string; total_tokens: number }
  >();
  const dailyMap = new Map<
    string,
    { cost_microusd: number; date: string; total_tokens: number }
  >();

  let totalCost = 0;
  let totalTokens = 0;

  for (const usage of usageRecords) {
    totalCost += usage.cost_microusd;
    totalTokens += usage.total_tokens;

    const providerEntry = providerMap.get(usage.provider) ?? {
      cost_microusd: 0,
      provider: usage.provider,
      total_tokens: 0,
    };
    providerEntry.cost_microusd += usage.cost_microusd;
    providerEntry.total_tokens += usage.total_tokens;
    providerMap.set(usage.provider, providerEntry);

    const day = usage.created_at.slice(0, 10);
    const dailyEntry = dailyMap.get(day) ?? {
      cost_microusd: 0,
      date: day,
      total_tokens: 0,
    };
    dailyEntry.cost_microusd += usage.cost_microusd;
    dailyEntry.total_tokens += usage.total_tokens;
    dailyMap.set(day, dailyEntry);
  }

  const latestRun = runs.reduce<JobRun | null>(
    (latest, run) =>
      !latest || run.created_at > latest.created_at ? run : latest,
    null
  );

  const latestRunCostMicrousd = latestRun
    ? usageRecords
        .filter((usage) => usage.run_id === latestRun.id)
        .reduce((sum, usage) => sum + usage.cost_microusd, 0)
    : 0;

  return {
    average_cost_microusd:
      runs.length > 0 ? Math.round(totalCost / runs.length) : 0,
    daily: [...dailyMap.values()].sort((a, b) => a.date.localeCompare(b.date)),
    latest_run_cost_microusd: latestRunCostMicrousd,
    providers: [...providerMap.values()].sort(
      (a, b) => b.cost_microusd - a.cost_microusd
    ),
    total_cost_microusd: totalCost,
    total_tokens: totalTokens,
    usage_records: usageRecords.length,
  };
}
