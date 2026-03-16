import type { RunContext } from "./job";

export type RunContextClient = {
  readonly checkpointRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly state: Record<string, unknown>;
      readonly source?: string;
    };
  }) => Promise<unknown>;
  readonly heartbeatRun: (input: {
    readonly pathParams: { readonly runID: string };
  }) => Promise<unknown>;
  readonly progressRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: { readonly percent: number; readonly message?: string };
  }) => Promise<unknown>;
  readonly logRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly level: string;
      readonly message: string;
      readonly data?: Record<string, unknown>;
    };
  }) => Promise<unknown>;
  readonly usageRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly provider: string;
      readonly model: string;
      readonly prompt_tokens?: number;
      readonly completion_tokens?: number;
      readonly total_tokens?: number;
      readonly cost_microusd?: number;
    };
  }) => Promise<unknown>;
  readonly toolCallRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly tool_name: string;
      readonly input?: Record<string, unknown>;
      readonly output?: Record<string, unknown>;
      readonly duration_ms?: number;
      readonly status?: string;
    };
  }) => Promise<unknown>;
  readonly outputRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly key: string;
      readonly value: Record<string, unknown>;
      readonly schema?: Record<string, unknown>;
    };
  }) => Promise<unknown>;
  readonly waitForEventRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly event_key: string;
      readonly timeout_secs?: number;
      readonly notify_url?: string;
    };
  }) => Promise<unknown>;
  readonly spawnRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly job_slug: string;
      readonly project_id: string;
      readonly payload?: Record<string, unknown>;
      readonly priority?: number;
    };
  }) => Promise<unknown>;
  readonly continueRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body?: { readonly payload?: Record<string, unknown> };
  }) => Promise<unknown>;
  readonly annotateRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: { readonly annotations: Record<string, string> };
  }) => Promise<unknown>;
  readonly completeRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body?: { readonly result?: Record<string, unknown> };
  }) => Promise<unknown>;
  readonly failRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: { readonly error: string };
  }) => Promise<unknown>;

  // KV state store (STR-10)
  readonly stateRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: { readonly key: string; readonly value: unknown };
  }) => Promise<unknown>;
  readonly getStateByRunId: (input: {
    readonly pathParams: { readonly runID: string };
  }) => Promise<unknown>;
  readonly getStateByRunIdAndKey: (input: {
    readonly pathParams: { readonly runID: string; readonly key: string };
  }) => Promise<unknown>;
  readonly deleteState: (input: {
    readonly pathParams: { readonly runID: string; readonly key: string };
  }) => Promise<unknown>;

  // LLM streaming (STR-11)
  readonly streamRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: {
      readonly chunk: string;
      readonly stream_id?: string;
      readonly done?: boolean;
    };
  }) => Promise<unknown>;
};

export type CreateRunContextOptions = {
  readonly attempt?: number;
  readonly signal?: AbortSignal;
};

const fireAndForget = (promise: Promise<unknown>) => {
  promise.catch(() => undefined);
};

export const createRunContext = (
  sdkClient: RunContextClient,
  runId: string,
  options?: CreateRunContextOptions
): RunContext => {
  const pathParams = { runID: runId } as const;
  const attempt = options?.attempt ?? 1;
  const signal = options?.signal ?? new AbortController().signal;

  return {
    runId,
    attempt,
    signal,

    logger: {
      info: (message, data) => {
        fireAndForget(
          sdkClient.logRun({
            pathParams,
            body: { level: "info", message, data },
          })
        );
      },
      warn: (message, data) => {
        fireAndForget(
          sdkClient.logRun({
            pathParams,
            body: { level: "warn", message, data },
          })
        );
      },
      error: (message, data) => {
        fireAndForget(
          sdkClient.logRun({
            pathParams,
            body: { level: "error", message, data },
          })
        );
      },
    },

    checkpoint: async (state) => {
      await sdkClient.checkpointRun({
        pathParams,
        body: { state, source: "sdk" },
      });
    },

    reportProgress: async (percent, message?) => {
      await sdkClient.progressRun({ pathParams, body: { percent, message } });
    },

    heartbeat: async () => {
      await sdkClient.heartbeatRun({ pathParams });
    },

    reportUsage: async (usage) => {
      await sdkClient.usageRun({
        pathParams,
        body: {
          provider: usage.provider,
          model: usage.model,
          prompt_tokens: usage.promptTokens,
          completion_tokens: usage.completionTokens,
          total_tokens: usage.totalTokens,
          cost_microusd: usage.costMicrousd,
        },
      });
    },

    logToolCall: async (toolCall) => {
      await sdkClient.toolCallRun({
        pathParams,
        body: {
          tool_name: toolCall.toolName,
          input: toolCall.input,
          output: toolCall.output,
          duration_ms: toolCall.durationMs,
          status: toolCall.status,
        },
      });
    },

    saveOutput: async (key, value, schema?) => {
      await sdkClient.outputRun({ pathParams, body: { key, value, schema } });
    },

    waitForEvent: async (eventKey, eventOptions?) => {
      const result = await sdkClient.waitForEventRun({
        pathParams,
        body: {
          event_key: eventKey,
          timeout_secs: eventOptions?.timeoutSecs,
          notify_url: eventOptions?.notifyUrl,
        },
      });
      return result as {
        status: string;
        eventKey: string;
        triggerId: string;
        expiresAt: string;
      };
    },

    spawn: async (spawnOptions) => {
      const result = await sdkClient.spawnRun({
        pathParams,
        body: {
          job_slug: spawnOptions.jobSlug,
          project_id: spawnOptions.projectId,
          payload: spawnOptions.payload,
          priority: spawnOptions.priority,
        },
      });
      return result as { id: string };
    },

    continue: async (payload?) => {
      const result = await sdkClient.continueRun({
        pathParams,
        body: payload ? { payload } : undefined,
      });
      return result as { id: string };
    },

    annotate: async (annotations) => {
      await sdkClient.annotateRun({ pathParams, body: { annotations } });
    },

    complete: async (result?) => {
      await sdkClient.completeRun({
        pathParams,
        body: result ? { result } : undefined,
      });
    },

    fail: async (error) => {
      await sdkClient.failRun({ pathParams, body: { error } });
    },

    state: {
      get: (key) =>
        sdkClient.getStateByRunIdAndKey({
          pathParams: { runID: runId, key },
        }),
      set: async (key, value) => {
        await sdkClient.stateRun({ pathParams, body: { key, value } });
      },
      delete: async (key) => {
        await sdkClient.deleteState({
          pathParams: { runID: runId, key },
        });
      },
      list: async () => {
        const result = await sdkClient.getStateByRunId({ pathParams });
        return result as Array<{
          key: string;
          value: unknown;
          updatedAt: string;
        }>;
      },
    },

    streamChunk: async (chunk, streamOptions?) => {
      await sdkClient.streamRun({
        pathParams,
        body: {
          chunk,
          stream_id: streamOptions?.streamId,
          done: streamOptions?.done,
        },
      });
    },
  };
};
