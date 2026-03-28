import type { JobRun, RunToolCall, RunUsage } from "@/hooks/api/types";

export type AgentCostBreakdown = {
  cost_microusd: number;
  label: string;
  total_tokens: number;
};

export type AgentToolSummary = {
  average_duration_ms: number;
  count: number;
  failed_count: number;
  tool_name: string;
};

export type AgentRunBreakdown = {
  cost_microusd: number;
  created_at: string;
  duration_ms: number;
  run_id: string;
  status: string;
  tool_calls: number;
  total_tokens: number;
};

export type AgentCostSummary = {
  average_cost_microusd: number;
  average_duration_ms: number;
  average_tokens_per_run: number;
  budget_limit_microusd?: number;
  budget_utilization_ratio?: number;
  daily: Array<{
    cost_microusd: number;
    date: string;
    total_tokens: number;
  }>;
  forecast: {
    projected_daily_cost_microusd: number;
    projected_monthly_cost_microusd: number;
  };
  latest_run_cost_microusd: number;
  models: AgentCostBreakdown[];
  providers: AgentCostBreakdown[];
  run_breakdown: AgentRunBreakdown[];
  tools: AgentToolSummary[];
  total_cost_microusd: number;
  total_tokens: number;
  usage_records: number;
};

function parseBudgetStringToMicrousd(value: string): number | undefined {
  const normalized = value.trim();
  if (!normalized.startsWith("$")) {
    return undefined;
  }

  const dollars = Number.parseFloat(normalized.slice(1));
  if (Number.isNaN(dollars) || dollars < 0) {
    return undefined;
  }

  return Math.round(dollars * 1_000_000);
}

export function readAgentBudgetLimitMicrousd(
  config: unknown
): number | undefined {
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    return undefined;
  }

  const record = config as Record<string, unknown>;
  const budget = record.budget;
  if (typeof budget === "number" && Number.isFinite(budget) && budget >= 0) {
    return Math.round(budget);
  }
  if (typeof budget === "string") {
    return parseBudgetStringToMicrousd(budget);
  }
  if (!budget || typeof budget !== "object" || Array.isArray(budget)) {
    return undefined;
  }

  const budgetRecord = budget as Record<string, unknown>;
  const maxCostMicrousd = budgetRecord.maxCostMicrousd;
  if (
    typeof maxCostMicrousd === "number" &&
    Number.isFinite(maxCostMicrousd) &&
    maxCostMicrousd >= 0
  ) {
    return Math.round(maxCostMicrousd);
  }

  return undefined;
}

function toDurationMilliseconds(run: JobRun): number {
  if (!(run.started_at && run.finished_at)) {
    return 0;
  }
  return Math.max(
    new Date(run.finished_at).getTime() - new Date(run.started_at).getTime(),
    0
  );
}

function buildForecast(
  daily: AgentCostSummary["daily"]
): AgentCostSummary["forecast"] {
  if (daily.length === 0) {
    return {
      projected_daily_cost_microusd: 0,
      projected_monthly_cost_microusd: 0,
    };
  }

  const averageDailyCost = Math.round(
    daily.reduce((sum, day) => sum + day.cost_microusd, 0) / daily.length
  );

  return {
    projected_daily_cost_microusd: averageDailyCost,
    projected_monthly_cost_microusd: averageDailyCost * 30,
  };
}

export function buildAgentCostSummary(
  runs: JobRun[],
  usageRecords: RunUsage[],
  toolCalls: RunToolCall[] = [],
  agentConfig?: unknown
): AgentCostSummary {
  const providerMap = new Map<string, AgentCostBreakdown>();
  const modelMap = new Map<string, AgentCostBreakdown>();
  const dailyMap = new Map<
    string,
    { cost_microusd: number; date: string; total_tokens: number }
  >();
  const usageByRun = new Map<
    string,
    { cost_microusd: number; total_tokens: number }
  >();
  const toolMap = new Map<
    string,
    {
      average_duration_ms: number;
      count: number;
      failed_count: number;
      total_duration_ms: number;
      tool_name: string;
    }
  >();
  const toolCallCountByRun = new Map<string, number>();

  let totalCost = 0;
  let totalTokens = 0;

  for (const usage of usageRecords) {
    totalCost += usage.cost_microusd;
    totalTokens += usage.total_tokens;

    const providerEntry = providerMap.get(usage.provider) ?? {
      cost_microusd: 0,
      label: usage.provider,
      total_tokens: 0,
    };
    providerEntry.cost_microusd += usage.cost_microusd;
    providerEntry.total_tokens += usage.total_tokens;
    providerMap.set(usage.provider, providerEntry);

    const modelKey = `${usage.provider}:${usage.model}`;
    const modelEntry = modelMap.get(modelKey) ?? {
      cost_microusd: 0,
      label: usage.model,
      total_tokens: 0,
    };
    modelEntry.cost_microusd += usage.cost_microusd;
    modelEntry.total_tokens += usage.total_tokens;
    modelMap.set(modelKey, modelEntry);

    const usageEntry = usageByRun.get(usage.run_id) ?? {
      cost_microusd: 0,
      total_tokens: 0,
    };
    usageEntry.cost_microusd += usage.cost_microusd;
    usageEntry.total_tokens += usage.total_tokens;
    usageByRun.set(usage.run_id, usageEntry);

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

  for (const toolCall of toolCalls) {
    const toolEntry = toolMap.get(toolCall.tool_name) ?? {
      average_duration_ms: 0,
      count: 0,
      failed_count: 0,
      total_duration_ms: 0,
      tool_name: toolCall.tool_name,
    };

    toolEntry.count += 1;
    toolEntry.total_duration_ms += toolCall.duration_ms ?? 0;
    if (toolCall.status !== "completed") {
      toolEntry.failed_count += 1;
    }
    toolMap.set(toolCall.tool_name, toolEntry);

    toolCallCountByRun.set(
      toolCall.run_id,
      (toolCallCountByRun.get(toolCall.run_id) ?? 0) + 1
    );
  }

  const latestRun = runs.reduce<JobRun | null>(
    (latest, run) =>
      !latest || run.created_at > latest.created_at ? run : latest,
    null
  );

  const runBreakdown = [...runs]
    .map((run) => {
      const usage = usageByRun.get(run.id) ?? {
        cost_microusd: 0,
        total_tokens: 0,
      };
      return {
        cost_microusd: usage.cost_microusd,
        created_at: run.created_at,
        duration_ms: toDurationMilliseconds(run),
        run_id: run.id,
        status: run.status,
        tool_calls: toolCallCountByRun.get(run.id) ?? 0,
        total_tokens: usage.total_tokens,
      };
    })
    .sort((left, right) => right.created_at.localeCompare(left.created_at));

  const durationValues = runBreakdown
    .map((run) => run.duration_ms)
    .filter((duration) => duration > 0);
  const budgetLimitMicrousd = readAgentBudgetLimitMicrousd(agentConfig);
  const daily = [...dailyMap.values()].sort((a, b) =>
    a.date.localeCompare(b.date)
  );

  return {
    average_cost_microusd:
      runs.length > 0 ? Math.round(totalCost / runs.length) : 0,
    average_duration_ms:
      durationValues.length > 0
        ? Math.round(
            durationValues.reduce((sum, duration) => sum + duration, 0) /
              durationValues.length
          )
        : 0,
    average_tokens_per_run:
      runs.length > 0 ? Math.round(totalTokens / runs.length) : 0,
    budget_limit_microusd: budgetLimitMicrousd,
    budget_utilization_ratio:
      budgetLimitMicrousd && budgetLimitMicrousd > 0
        ? totalCost / budgetLimitMicrousd
        : undefined,
    daily,
    forecast: buildForecast(daily),
    latest_run_cost_microusd: latestRun
      ? (usageByRun.get(latestRun.id)?.cost_microusd ?? 0)
      : 0,
    models: [...modelMap.values()].sort(
      (left, right) => right.cost_microusd - left.cost_microusd
    ),
    providers: [...providerMap.values()].sort(
      (left, right) => right.cost_microusd - left.cost_microusd
    ),
    run_breakdown: runBreakdown,
    tools: [...toolMap.values()]
      .map((tool) => ({
        average_duration_ms:
          tool.count > 0 ? Math.round(tool.total_duration_ms / tool.count) : 0,
        count: tool.count,
        failed_count: tool.failed_count,
        tool_name: tool.tool_name,
      }))
      .sort((left, right) => right.count - left.count),
    total_cost_microusd: totalCost,
    total_tokens: totalTokens,
    usage_records: usageRecords.length,
  };
}
