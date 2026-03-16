import type { RunContext } from "./job";
import { createRunContext, type RunContextClient } from "./run-context";

export type TestRunRecord = {
  readonly checkpoints: Record<string, unknown>[];
  readonly logs: {
    level: string;
    message: string;
    data?: Record<string, unknown>;
  }[];
  readonly usageReports: {
    provider: string;
    model: string;
    promptTokens?: number;
    completionTokens?: number;
    totalTokens?: number;
    costMicrousd?: number;
  }[];
  readonly toolCalls: {
    toolName: string;
    input?: Record<string, unknown>;
    output?: Record<string, unknown>;
    durationMs?: number;
    status?: string;
  }[];
  readonly outputs: { key: string; value: Record<string, unknown> }[];
  readonly progressUpdates: { percent: number; message?: string }[];
  readonly stateStore: Map<string, unknown>;
  readonly streamChunks: {
    chunk: string;
    streamId?: string;
    done?: boolean;
  }[];
  heartbeats: number;
  readonly spawns: {
    jobSlug: string;
    projectId: string;
    payload?: Record<string, unknown>;
  }[];
  readonly events: {
    eventKey: string;
    timeoutSecs?: number;
    notifyUrl?: string;
  }[];
  readonly annotations: Record<string, string>[];
  readonly continuations: { payload?: Record<string, unknown> }[];
  completed: boolean;
  failed: boolean;
  failError?: string;
  result?: Record<string, unknown>;
};

export const createTestContext = (
  runId = "test-run",
  options?: { attempt?: number; signal?: AbortSignal }
): { ctx: RunContext; record: TestRunRecord } => {
  const record: TestRunRecord = {
    checkpoints: [],
    logs: [],
    usageReports: [],
    toolCalls: [],
    outputs: [],
    progressUpdates: [],
    stateStore: new Map(),
    streamChunks: [],
    heartbeats: 0,
    spawns: [],
    events: [],
    annotations: [],
    continuations: [],
    completed: false,
    failed: false,
  };

  const mockClient: RunContextClient = {
    checkpointRun: ({ body }) => {
      record.checkpoints.push(body.state);
      return Promise.resolve();
    },
    heartbeatRun: () => {
      record.heartbeats++;
      return Promise.resolve();
    },
    progressRun: ({ body }) => {
      record.progressUpdates.push({
        percent: body.percent,
        message: body.message,
      });
      return Promise.resolve();
    },
    logRun: ({ body }) => {
      record.logs.push({
        level: body.level,
        message: body.message,
        data: body.data,
      });
      return Promise.resolve();
    },
    usageRun: ({ body }) => {
      record.usageReports.push({
        provider: body.provider,
        model: body.model,
        promptTokens: body.prompt_tokens,
        completionTokens: body.completion_tokens,
        totalTokens: body.total_tokens,
        costMicrousd: body.cost_microusd,
      });
      return Promise.resolve();
    },
    toolCallRun: ({ body }) => {
      record.toolCalls.push({
        toolName: body.tool_name,
        input: body.input,
        output: body.output,
        durationMs: body.duration_ms,
        status: body.status,
      });
      return Promise.resolve();
    },
    outputRun: ({ body }) => {
      record.outputs.push({ key: body.key, value: body.value });
      return Promise.resolve();
    },
    waitForEventRun: ({ body }) => {
      record.events.push({
        eventKey: body.event_key,
        timeoutSecs: body.timeout_secs,
        notifyUrl: body.notify_url,
      });
      return Promise.resolve({
        status: "waiting",
        eventKey: body.event_key,
        triggerId: "trigger_test",
        expiresAt: new Date(
          Date.now() + (body.timeout_secs ?? 300) * 1000
        ).toISOString(),
      });
    },
    spawnRun: ({ body }) => {
      record.spawns.push({
        jobSlug: body.job_slug,
        projectId: body.project_id,
        payload: body.payload,
      });
      return Promise.resolve({ id: `spawn_${record.spawns.length}` });
    },
    continueRun: ({ body }) => {
      record.continuations.push({
        payload: body?.payload as Record<string, unknown> | undefined,
      });
      return Promise.resolve({
        id: `continue_${record.continuations.length}`,
      });
    },
    annotateRun: ({ body }) => {
      record.annotations.push(body.annotations);
      return Promise.resolve();
    },
    completeRun: ({ body }) => {
      record.completed = true;
      record.result = body?.result as Record<string, unknown> | undefined;
      return Promise.resolve();
    },
    failRun: ({ body }) => {
      record.failed = true;
      record.failError = body.error;
      return Promise.resolve();
    },
    stateRun: ({ body }) => {
      record.stateStore.set(body.key, body.value);
      return Promise.resolve();
    },
    getStateByRunId: () =>
      Promise.resolve(
        Array.from(record.stateStore.entries()).map(([key, value]) => ({
          key,
          value,
          updatedAt: new Date().toISOString(),
        }))
      ),
    getStateByRunIdAndKey: ({ pathParams }) =>
      Promise.resolve(record.stateStore.get(pathParams.key)),
    deleteState: ({ pathParams }) => {
      record.stateStore.delete(pathParams.key);
      return Promise.resolve();
    },
    streamRun: ({ body }) => {
      record.streamChunks.push({
        chunk: body.chunk,
        streamId: body.stream_id,
        done: body.done,
      });
      return Promise.resolve();
    },
  };

  return { ctx: createRunContext(mockClient, runId, { attempt: options?.attempt, signal: options?.signal }), record };
};
