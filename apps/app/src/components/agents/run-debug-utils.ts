import type { DebugBundle } from "@/hooks/api/types";

type JsonRecord = Record<string, unknown>;

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
      outbound_reason?: string;
      sandbox_executor?: string;
      sandbox_mode?: string;
      status: string;
      tool_name: string;
    };

export type RunDebugSummary = {
  blocked_tool_call_count: number;
  blocked_reason_breakdown: Array<{
    count: number;
    reason: string;
  }>;
  checkpoint_count: number;
  latest_checkpoint?: unknown;
  model_breakdown: Array<{
    cost_microusd: number;
    label: string;
    total_tokens: number;
  }>;
  tool_breakdown: Array<{
    blocked_count: number;
    count: number;
    executors: string[];
    failed_count: number;
    tool_name: string;
  }>;
  tool_details: Array<{
    created_at: string;
    duration_ms?: number;
    outbound_reason?: string;
    sandbox_executor?: string;
    sandbox_mode?: string;
    status: string;
    tool_name: string;
  }>;
  tool_executor_breakdown: Array<{
    blocked_count: number;
    count: number;
    executor: string;
  }>;
  total_cost_microusd: number;
  total_tokens: number;
  usage_count: number;
};

export type ErrorClassification = {
  error_class: string;
  suggestion?: string;
};

/**
 * Extracts the error classification from the debug bundle events.
 * Looks for error events that contain an `error_class` field in their data.
 */
export function extractErrorClassification(
  bundle: DebugBundle
): ErrorClassification | null {
  for (const event of bundle.events ?? []) {
    if (event.type !== "error" && event.level !== "error") {
      continue;
    }
    const data = asRecord(event.data);
    if (!data) {
      continue;
    }
    const errorClass =
      typeof data.error_class === "string" ? data.error_class : null;
    if (errorClass) {
      return {
        error_class: errorClass,
        suggestion:
          typeof data.suggestion === "string" ? data.suggestion : undefined,
      };
    }
  }
  return null;
}

function asRecord(value: unknown): JsonRecord | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as JsonRecord;
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim().length > 0
    ? value
    : undefined;
}

function extractToolSandboxMetadata(
  toolCall: NonNullable<DebugBundle["tool_calls"]>[number]
) {
  const input = asRecord(toolCall.input);
  const output = asRecord(toolCall.output);
  return {
    outbound_reason: readString(output?.outbound_reason),
    sandbox_executor: readString(output?.sandbox_executor),
    sandbox_mode: readString(input?.sandbox_mode),
  };
}

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
    bundle.tool_calls?.map((toolCall) => {
      const metadata = extractToolSandboxMetadata(toolCall);
      return {
        created_at: toolCall.created_at,
        duration_ms: toolCall.duration_ms,
        kind: "tool_call" as const,
        label: toolCall.tool_name,
        outbound_reason: metadata.outbound_reason,
        sandbox_executor: metadata.sandbox_executor,
        sandbox_mode: metadata.sandbox_mode,
        status: toolCall.status,
        tool_name: toolCall.tool_name,
      };
    }) ?? [];

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
    {
      blocked_count: number;
      count: number;
      executors: Set<string>;
      failed_count: number;
      tool_name: string;
    }
  >();
  const blockedReasonMap = new Map<string, { count: number; reason: string }>();
  const toolExecutorMap = new Map<
    string,
    { blocked_count: number; count: number; executor: string }
  >();
  const toolDetails: RunDebugSummary["tool_details"] = [];

  let totalCost = 0;
  let totalTokens = 0;
  let blockedToolCallCount = 0;

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
    const metadata = extractToolSandboxMetadata(toolCall);
    const executor =
      metadata.sandbox_executor ?? metadata.sandbox_mode ?? "in_process";
    const blocked = toolCall.status === "blocked";
    const entry = toolMap.get(toolCall.tool_name) ?? {
      blocked_count: 0,
      count: 0,
      executors: new Set<string>(),
      failed_count: 0,
      tool_name: toolCall.tool_name,
    };
    entry.count += 1;
    entry.executors.add(executor);
    if (toolCall.status !== "completed") {
      entry.failed_count += 1;
    }
    if (blocked) {
      entry.blocked_count += 1;
      blockedToolCallCount += 1;
    }
    toolMap.set(toolCall.tool_name, entry);

    if (metadata.outbound_reason) {
      const blockedReasonEntry = blockedReasonMap.get(
        metadata.outbound_reason
      ) ?? {
        count: 0,
        reason: metadata.outbound_reason,
      };
      blockedReasonEntry.count += 1;
      blockedReasonMap.set(metadata.outbound_reason, blockedReasonEntry);
    }

    const executorEntry = toolExecutorMap.get(executor) ?? {
      blocked_count: 0,
      count: 0,
      executor,
    };
    executorEntry.count += 1;
    if (blocked) {
      executorEntry.blocked_count += 1;
    }
    toolExecutorMap.set(executor, executorEntry);

    toolDetails.push({
      created_at: toolCall.created_at,
      duration_ms: toolCall.duration_ms,
      outbound_reason: metadata.outbound_reason,
      sandbox_executor: metadata.sandbox_executor,
      sandbox_mode: metadata.sandbox_mode,
      status: toolCall.status,
      tool_name: toolCall.tool_name,
    });
  }

  return {
    blocked_reason_breakdown: [...blockedReasonMap.values()].sort(
      (left, right) => right.count - left.count
    ),
    blocked_tool_call_count: blockedToolCallCount,
    checkpoint_count: bundle.checkpoints?.length ?? 0,
    latest_checkpoint: bundle.checkpoints?.at(-1)?.state,
    model_breakdown: [...modelMap.values()].sort(
      (left, right) => right.cost_microusd - left.cost_microusd
    ),
    tool_breakdown: [...toolMap.values()]
      .sort((left, right) => right.count - left.count)
      .map((tool) => ({
        blocked_count: tool.blocked_count,
        count: tool.count,
        executors: [...tool.executors].sort(),
        failed_count: tool.failed_count,
        tool_name: tool.tool_name,
      })),
    tool_details: [...toolDetails].sort((left, right) =>
      right.created_at.localeCompare(left.created_at)
    ),
    tool_executor_breakdown: [...toolExecutorMap.values()].sort(
      (left, right) => right.count - left.count
    ),
    total_cost_microusd: totalCost,
    total_tokens: totalTokens,
    usage_count: bundle.usage?.length ?? 0,
  };
}
