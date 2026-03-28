import type { DebugBundle } from "@/hooks/api/types";

export type RunTimelineItem =
  | {
      created_at: string;
      kind: "checkpoint";
      label: string;
      sequence: number;
      state: unknown;
    }
  | {
      created_at: string;
      kind: "event";
      label: string;
      level?: string;
      message: string;
      type: string;
    }
  | {
      cost_microusd: number;
      created_at: string;
      kind: "usage";
      label: string;
      model: string;
      provider: string;
      total_tokens: number;
    }
  | {
      created_at: string;
      duration_ms?: number;
      kind: "tool_call";
      label: string;
      status: string;
      tool_name: string;
    };

export type RunDebugSummary = {
  checkpoint_count: number;
  latest_checkpoint?: unknown;
  model_breakdown: Array<{
    cost_microusd: number;
    label: string;
    total_tokens: number;
  }>;
  tool_breakdown: Array<{
    count: number;
    failed_count: number;
    tool_name: string;
  }>;
  total_cost_microusd: number;
  total_tokens: number;
  usage_count: number;
};

export function buildRunTimeline(bundle: DebugBundle): RunTimelineItem[] {
  const checkpoints =
    bundle.checkpoints?.map((checkpoint) => ({
      created_at: checkpoint.created_at,
      kind: "checkpoint" as const,
      label: `Checkpoint #${checkpoint.sequence}`,
      sequence: checkpoint.sequence,
      state: checkpoint.state,
    })) ?? [];
  const events =
    bundle.events?.map((event) => ({
      created_at: event.created_at,
      kind: "event" as const,
      label: event.type,
      level: event.level,
      message: event.message,
      type: event.type,
    })) ?? [];
  const usage =
    bundle.usage?.map((item) => ({
      cost_microusd: item.cost_microusd,
      created_at: item.created_at,
      kind: "usage" as const,
      label: `${item.provider}:${item.model}`,
      model: item.model,
      provider: item.provider,
      total_tokens: item.total_tokens,
    })) ?? [];
  const toolCalls =
    bundle.tool_calls?.map((toolCall) => ({
      created_at: toolCall.created_at,
      duration_ms: toolCall.duration_ms,
      kind: "tool_call" as const,
      label: toolCall.tool_name,
      status: toolCall.status,
      tool_name: toolCall.tool_name,
    })) ?? [];

  return [...events, ...checkpoints, ...usage, ...toolCalls].sort(
    (left, right) => left.created_at.localeCompare(right.created_at)
  );
}

export function summarizeRunDebugBundle(bundle: DebugBundle): RunDebugSummary {
  const modelMap = new Map<
    string,
    { cost_microusd: number; label: string; total_tokens: number }
  >();
  const toolMap = new Map<
    string,
    { count: number; failed_count: number; tool_name: string }
  >();

  let totalCost = 0;
  let totalTokens = 0;

  for (const usage of bundle.usage ?? []) {
    totalCost += usage.cost_microusd;
    totalTokens += usage.total_tokens;

    const key = `${usage.provider}:${usage.model}`;
    const entry = modelMap.get(key) ?? {
      cost_microusd: 0,
      label: usage.model,
      total_tokens: 0,
    };
    entry.cost_microusd += usage.cost_microusd;
    entry.total_tokens += usage.total_tokens;
    modelMap.set(key, entry);
  }

  for (const toolCall of bundle.tool_calls ?? []) {
    const entry = toolMap.get(toolCall.tool_name) ?? {
      count: 0,
      failed_count: 0,
      tool_name: toolCall.tool_name,
    };
    entry.count += 1;
    if (toolCall.status !== "completed") {
      entry.failed_count += 1;
    }
    toolMap.set(toolCall.tool_name, entry);
  }

  return {
    checkpoint_count: bundle.checkpoints?.length ?? 0,
    latest_checkpoint: bundle.checkpoints?.at(-1)?.state,
    model_breakdown: [...modelMap.values()].sort(
      (left, right) => right.cost_microusd - left.cost_microusd
    ),
    tool_breakdown: [...toolMap.values()].sort(
      (left, right) => right.count - left.count
    ),
    total_cost_microusd: totalCost,
    total_tokens: totalTokens,
    usage_count: bundle.usage?.length ?? 0,
  };
}
