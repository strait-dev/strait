import { fromPromise } from "../composition/result";
import { type WaitForRunOptions, waitForRun } from "../composition/wait";
import type {
  FutureLocalExecutorHooks,
  SchemaInput,
  TriggerResult,
} from "./types";
import { extractEntityId, requireProjectId, resolveSchema } from "./utils";

/**
 * Context object passed to the job's `run` handler.
 *
 * Provides access to the run ID, attempt number, abort signal, and helper
 * methods for logging, progress reporting, checkpointing, and heartbeats.
 *
 * **Important:** The SDK defines this interface but does not provide a default
 * implementation. Your executor/worker must supply concrete implementations
 * when calling `job.run(payload, ctx)`. The methods on this type are a
 * contract between the job definition and the hosting runtime.
 *
 * @example
 * ```ts
 * // In your executor/worker:
 * const ctx: RunContext = {
 *   runId: "run_123",
 *   attempt: 1,
 *   signal: controller.signal,
 *   logger: createRunLogger(runId),
 *   checkpoint: (state) => db.saveCheckpoint(runId, state),
 *   reportProgress: (pct) => api.updateProgress(runId, pct),
 *   heartbeat: () => api.heartbeat(runId),
 * };
 * await job.run(payload, ctx);
 * ```
 */
export type RunContext = {
  readonly runId: string;
  readonly attempt: number;
  readonly signal: AbortSignal;
  readonly logger: {
    readonly info: (message: string, data?: Record<string, unknown>) => void;
    readonly warn: (message: string, data?: Record<string, unknown>) => void;
    readonly error: (message: string, data?: Record<string, unknown>) => void;
  };
  readonly checkpoint: (state: Record<string, unknown>) => Promise<void>;
  readonly reportProgress: (percent: number, message?: string) => Promise<void>;
  readonly heartbeat: () => Promise<void>;

  readonly reportUsage?: (usage: {
    readonly provider: string;
    readonly model: string;
    readonly promptTokens?: number;
    readonly completionTokens?: number;
    readonly totalTokens?: number;
    readonly costMicrousd?: number;
  }) => Promise<void>;
  readonly logToolCall?: (toolCall: {
    readonly toolName: string;
    readonly input?: Record<string, unknown>;
    readonly output?: Record<string, unknown>;
    readonly durationMs?: number;
    readonly status?: string;
  }) => Promise<void>;
  readonly saveOutput?: (
    key: string,
    value: Record<string, unknown>,
    schema?: Record<string, unknown>
  ) => Promise<void>;

  readonly state?: {
    readonly get: (key: string) => Promise<unknown>;
    readonly set: (key: string, value: unknown) => Promise<void>;
    readonly delete: (key: string) => Promise<void>;
    readonly list: () => Promise<
      Array<{ key: string; value: unknown; updatedAt: string }>
    >;
  };

  readonly streamChunk?: (
    chunk: string,
    options?: {
      readonly streamId?: string;
      readonly done?: boolean;
    }
  ) => Promise<void>;

  readonly waitForEvent?: (
    eventKey: string,
    options?: {
      readonly timeoutSecs?: number;
      readonly notifyUrl?: string;
    }
  ) => Promise<{
    readonly status: string;
    readonly eventKey: string;
    readonly triggerId: string;
    readonly expiresAt: string;
  }>;

  readonly spawn?: (options: {
    readonly jobSlug: string;
    readonly projectId: string;
    readonly payload?: Record<string, unknown>;
    readonly priority?: number;
  }) => Promise<{ readonly id: string }>;
  readonly continue?: (
    payload?: Record<string, unknown>
  ) => Promise<{ readonly id: string }>;
  readonly annotate?: (annotations: Record<string, string>) => Promise<void>;
  readonly complete?: (result?: Record<string, unknown>) => Promise<void>;
  readonly fail?: (error: string) => Promise<void>;
};

/**
 * Options for defining a Strait job with full configuration.
 *
 * @example
 * ```ts
 * const syncJob = defineJob({
 *   name: "Sync Inventory",
 *   slug: "sync-inventory",
 *   endpointUrl: "https://worker.dev/jobs/sync",
 *   projectId: "proj_1",
 *   schema: zodSchema(z.object({ sku: z.string() })),
 *   cron: "* /5 * * * *",
 *   timezone: "America/New_York",
 *   maxConcurrency: 5,
 *   maxAttempts: 5,
 *   retryStrategy: "exponential",
 *   timeoutSecs: 300,
 *   tags: { team: "inventory" },
 *   run: async (payload, ctx) => {
 *     const result = await processSync(payload.sku);
 *     return { synced: true, ...result };
 *   },
 *   onSuccess: async ({ payload, output, ctx }) => { ... },
 *   onFailure: async ({ payload, error, ctx }) => { ... },
 * });
 * ```
 */
export type DefineJobOptions<TPayload, TOutput = unknown> = {
  /** Job display name. */
  readonly name: string;
  /** Unique slug identifier. */
  readonly slug: string;
  /** URL where the job executor is hosted. */
  readonly endpointUrl: string;
  /**
   * Schema for payload validation. Accepts either:
   * - A Standard Schema v1 object directly (Zod 3.24+, Valibot 1.0+, ArkType 2.0+)
   * - A `SchemaAdapter` from `zodSchema()`, `effectSchema()`, `customSchema()`, or `standardSchema()`
   */
  readonly schema: SchemaInput<TPayload>;
  /** Project ID — can also be provided at register() time. */
  readonly projectId?: string;

  /**
   * The code that executes when the job runs.
   *
   * **Important:** The SDK stores this handler but does not invoke it. Your
   * executor (Conductor worker, serverless function, or test harness) is
   * responsible for calling `job.run(payload, ctx)` and supplying a
   * {@link RunContext} with concrete implementations of `logger`,
   * `checkpoint`, `reportProgress`, and `heartbeat`.
   *
   * @example
   * ```ts
   * // In your worker/executor:
   * const result = await job.run(payload, {
   *   runId: "run_123",
   *   attempt: 1,
   *   signal: AbortSignal.timeout(30_000),
   *   logger: workerLogger,
   *   checkpoint: (state) => saveCheckpoint(runId, state),
   *   reportProgress: (pct) => updateProgress(runId, pct),
   *   heartbeat: () => sendHeartbeat(runId),
   * });
   * ```
   */
  readonly run?: (
    payload: TPayload,
    ctx: RunContext
  ) => Promise<TOutput> | TOutput;

  /**
   * Called after a successful run completes.
   *
   * **Note:** The SDK stores this hook but does not invoke it. Your executor
   * should call `job.onSuccess?.({ payload, output, ctx })` after `run`
   * returns successfully.
   */
  readonly onSuccess?: (context: {
    readonly payload: TPayload;
    readonly output: TOutput;
    readonly ctx: RunContext;
  }) => void | Promise<void>;
  /**
   * Called when a run fails.
   *
   * **Note:** The SDK stores this hook but does not invoke it. Your executor
   * should call `job.onFailure?.({ payload, error, ctx })` when `run` throws.
   */
  readonly onFailure?: (context: {
    readonly payload: TPayload;
    readonly error: unknown;
    readonly ctx: RunContext;
  }) => void | Promise<void>;
  /**
   * Called when a run starts executing.
   *
   * **Note:** The SDK stores this hook but does not invoke it. Your executor
   * should call `job.onStart?.({ payload, ctx })` before invoking `run`.
   */
  readonly onStart?: (context: {
    readonly payload: TPayload;
    readonly ctx: RunContext;
  }) => void | Promise<void>;

  /** Human-readable description. */
  readonly description?: string;
  /** Group ID for job grouping. */
  readonly groupId?: string;
  /** Key-value tags for filtering and organization. */
  readonly tags?: Readonly<Record<string, string>>;
  /** Environment ID for multi-environment setups. */
  readonly environmentId?: string;

  /** Cron expression for scheduled execution. */
  readonly cron?: string;
  /** IANA timezone for cron evaluation. */
  readonly timezone?: string;
  /** Cron expression defining execution windows. */
  readonly executionWindowCron?: string;

  /** Maximum concurrent runs for this job. */
  readonly maxConcurrency?: number;

  /** Maximum number of requests in the rate limit window. */
  readonly rateLimitMax?: number;
  /** Rate limit window duration in seconds. */
  readonly rateLimitWindowSecs?: number;

  /** Maximum number of attempts (including initial). */
  readonly maxAttempts?: number;
  /** Retry backoff strategy. */
  readonly retryStrategy?: "exponential" | "fixed";
  /** Explicit retry delay sequence in seconds. */
  readonly retryDelaysSecs?: readonly number[];

  /** Maximum execution time in seconds. */
  readonly timeoutSecs?: number;
  /** Time-to-live for run data in seconds. */
  readonly runTtlSecs?: number;
  /** Deduplication window in seconds. */
  readonly dedupWindowSecs?: number;

  /** Webhook URL for run status notifications. */
  readonly webhookUrl?: string;
  /** Webhook signing secret. */
  readonly webhookSecret?: string;
  /** Fallback endpoint URL if primary fails. */
  readonly fallbackEndpointUrl?: string;

  /** Extension points for future local execution integrations. */
  readonly hooks?: FutureLocalExecutorHooks;
};

/** Response from registering a job with the Strait API. */
export type JobResponse = {
  readonly id: string;
  readonly slug?: string;
  readonly name?: string;
  readonly [key: string]: unknown;
};

/** Response from triggering a job run. */
export type JobRunResponse = {
  readonly id: string;
  readonly status?: string;
  readonly [key: string]: unknown;
};

/** Response from triggering multiple job runs in bulk. */
export type BulkTriggerResponse = {
  readonly runs?: readonly {
    readonly id: string;
    readonly [key: string]: unknown;
  }[];
  readonly [key: string]: unknown;
};

/**
 * Typed trigger input for a job run.
 *
 * Maps to the API's `TriggerJobRequest` fields.
 */
export type TriggerJobInput<TPayload> = {
  readonly jobID?: string;
  readonly payload: TPayload;
  readonly idempotencyKey?: string;
  readonly priority?: number;
  readonly dryRun?: boolean;
  readonly metadata?: Readonly<Record<string, string>>;
  readonly scheduledAt?: string;
};

type JobRegistrationBody = {
  readonly project_id: string;
  readonly name: string;
  readonly slug: string;
  readonly endpoint_url: string;
  readonly payload_schema?: Readonly<Record<string, unknown>>;
  readonly description?: string;
  readonly group_id?: string;
  readonly tags?: Readonly<Record<string, string>>;
  readonly environment_id?: string;
  readonly cron?: string;
  readonly timezone?: string;
  readonly execution_window_cron?: string;
  readonly max_concurrency?: number;
  readonly rate_limit_max?: number;
  readonly rate_limit_window_secs?: number;
  readonly max_attempts?: number;
  readonly retry_strategy?: string;
  readonly retry_delays_secs?: readonly number[];
  readonly timeout_secs?: number;
  readonly run_ttl_secs?: number;
  readonly dedup_window_secs?: number;
  readonly webhook_url?: string;
  readonly webhook_secret?: string;
  readonly fallback_endpoint_url?: string;
  readonly [key: string]: unknown;
};

type JobTriggerBody = {
  readonly payload?: unknown;
  readonly idempotency_key?: string;
  readonly priority?: number;
  readonly dry_run?: boolean;
  readonly metadata?: Readonly<Record<string, string>>;
  readonly scheduled_at?: string;
};

type JobDslClient = {
  readonly createJob: (input: {
    readonly body: JobRegistrationBody;
  }) => Promise<unknown>;
  readonly triggerJob: (input: {
    readonly pathParams: { readonly jobID: string };
    readonly body?: JobTriggerBody;
  }) => Promise<unknown>;
  readonly triggerJobBulk?: (input: {
    readonly pathParams: { readonly jobID: string };
    readonly body?: { readonly items: readonly JobTriggerBody[] };
  }) => Promise<unknown>;
  readonly getRun?: (input: {
    readonly pathParams: { readonly runID: string };
  }) => Promise<{ readonly status?: string; readonly [key: string]: unknown }>;
};

/**
 * Defines a reusable job authoring unit with schema-backed payload validation.
 *
 * The returned definition can:
 * - build API registration bodies via `toRegistrationBody`
 * - register jobs via `register`
 * - trigger jobs via `trigger` / `triggerResult`
 * - trigger and wait via `triggerAndWait`
 * - batch trigger via `batchTrigger`
 * - expose the `run` handler for local execution or test harnesses
 *
 * After a successful `register`, trigger calls may omit `jobID` and reuse the
 * last registered identifier.
 *
 * @param options - Full job configuration including optional run handler and lifecycle hooks.
 * @returns A job definition object with registration, triggering, and execution methods.
 */
export const defineJob = <TPayload, TOutput = unknown>(
  options: DefineJobOptions<TPayload, TOutput>
) => {
  const schema = resolveSchema(options.schema);
  let lastRegisteredJobId: string | undefined;

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: field mapping is necessarily verbose
  const toRegistrationBody = (projectId?: string): JobRegistrationBody => {
    const resolvedProjectId = requireProjectId(
      options.projectId,
      projectId,
      `defineJob(${options.slug})`
    );

    const payloadSchema = schema.toJsonSchema?.();

    return {
      project_id: resolvedProjectId,
      name: options.name,
      slug: options.slug,
      endpoint_url: options.endpointUrl,
      ...(payloadSchema ? { payload_schema: payloadSchema } : {}),
      ...(options.description ? { description: options.description } : {}),
      ...(options.groupId ? { group_id: options.groupId } : {}),
      ...(options.tags ? { tags: options.tags } : {}),
      ...(options.environmentId
        ? { environment_id: options.environmentId }
        : {}),
      ...(options.cron ? { cron: options.cron } : {}),
      ...(options.timezone ? { timezone: options.timezone } : {}),
      ...(options.executionWindowCron
        ? { execution_window_cron: options.executionWindowCron }
        : {}),
      ...(options.maxConcurrency === undefined
        ? {}
        : { max_concurrency: options.maxConcurrency }),
      ...(options.rateLimitMax === undefined
        ? {}
        : { rate_limit_max: options.rateLimitMax }),
      ...(options.rateLimitWindowSecs === undefined
        ? {}
        : { rate_limit_window_secs: options.rateLimitWindowSecs }),
      ...(options.maxAttempts === undefined
        ? {}
        : { max_attempts: options.maxAttempts }),
      ...(options.retryStrategy
        ? { retry_strategy: options.retryStrategy }
        : {}),
      ...(options.retryDelaysSecs
        ? { retry_delays_secs: options.retryDelaysSecs }
        : {}),
      ...(options.timeoutSecs === undefined
        ? {}
        : { timeout_secs: options.timeoutSecs }),
      ...(options.runTtlSecs === undefined
        ? {}
        : { run_ttl_secs: options.runTtlSecs }),
      ...(options.dedupWindowSecs === undefined
        ? {}
        : { dedup_window_secs: options.dedupWindowSecs }),
      ...(options.webhookUrl ? { webhook_url: options.webhookUrl } : {}),
      ...(options.webhookSecret
        ? { webhook_secret: options.webhookSecret }
        : {}),
      ...(options.fallbackEndpointUrl
        ? { fallback_endpoint_url: options.fallbackEndpointUrl }
        : {}),
    };
  };

  const resolveJobID = (jobID?: string): string => {
    const resolved = jobID ?? lastRegisteredJobId;
    if (!resolved) {
      throw new Error(
        `defineJob(${options.slug}) trigger requires jobID or prior successful register()`
      );
    }

    return resolved;
  };

  const buildTriggerBody = (
    input: TriggerJobInput<unknown>
  ): JobTriggerBody => ({
    payload: input.payload,
    ...(input.idempotencyKey ? { idempotency_key: input.idempotencyKey } : {}),
    ...(input.priority === undefined ? {} : { priority: input.priority }),
    ...(input.dryRun === undefined ? {} : { dry_run: input.dryRun }),
    ...(input.metadata ? { metadata: input.metadata } : {}),
    ...(input.scheduledAt ? { scheduled_at: input.scheduledAt } : {}),
  });

  const trigger = async (
    client: JobDslClient,
    input: TriggerJobInput<TPayload>
  ): Promise<JobRunResponse> => {
    const payload = await schema.parse(input.payload);
    const body = buildTriggerBody({ ...input, payload });

    const result = await client.triggerJob({
      pathParams: { jobID: resolveJobID(input.jobID) },
      body,
    });

    return result as JobRunResponse;
  };

  return {
    kind: "job" as const,
    slug: options.slug,
    schema,
    hooks: options.hooks,

    /** The run handler, if provided. Exposed for local executors or test harnesses. */
    run: options.run,

    /** Lifecycle hooks, if provided. */
    onSuccess: options.onSuccess,
    onFailure: options.onFailure,
    onStart: options.onStart,

    toRegistrationBody,

    /**
     * Registers this job with the Strait API.
     *
     * @param client - A client with `createJob` method.
     * @param input - Optional project ID override.
     * @returns The API response with the created job's ID.
     */
    register: async (
      client: JobDslClient,
      input?: { readonly projectId?: string }
    ): Promise<JobResponse> => {
      const body = toRegistrationBody(input?.projectId);
      const created = await client.createJob({ body });

      lastRegisteredJobId = extractEntityId(created) ?? lastRegisteredJobId;
      await options.hooks?.onRegister?.({
        kind: "job",
        slug: options.slug,
        registration: body,
      });

      return created as JobResponse;
    },

    /**
     * Triggers a run of this job.
     *
     * @param client - A client with `triggerJob` method.
     * @param input - Trigger input with payload and optional fields.
     * @returns The API response with the run ID and status.
     */
    trigger,

    /**
     * Triggers a run and returns a {@link SdkResult} instead of throwing.
     */
    triggerResult: (
      client: JobDslClient,
      input: TriggerJobInput<TPayload>
    ): TriggerResult<JobRunResponse> =>
      fromPromise(() => trigger(client, input)),

    /**
     * Triggers multiple runs of this job in a single API call.
     *
     * @param client - A client with `triggerJobBulk` method.
     * @param input - Batch trigger input with items array.
     * @returns The API response with created runs.
     */
    batchTrigger: async (
      client: JobDslClient,
      input: {
        readonly jobID?: string;
        readonly items: readonly {
          readonly payload: TPayload;
          readonly scheduledAt?: string;
          readonly priority?: number;
          readonly idempotencyKey?: string;
          readonly tags?: Readonly<Record<string, string>>;
        }[];
      }
    ): Promise<BulkTriggerResponse> => {
      if (!client.triggerJobBulk) {
        throw new Error(
          "batchTrigger requires a client with triggerJobBulk method"
        );
      }

      const items = await Promise.all(
        input.items.map(async (item) => {
          const payload = await schema.parse(item.payload);
          return {
            payload,
            ...(item.scheduledAt ? { scheduled_at: item.scheduledAt } : {}),
            ...(item.priority === undefined ? {} : { priority: item.priority }),
            ...(item.idempotencyKey
              ? { idempotency_key: item.idempotencyKey }
              : {}),
            ...(item.tags ? { tags: item.tags } : {}),
          };
        })
      );

      const result = await client.triggerJobBulk({
        pathParams: { jobID: resolveJobID(input.jobID) },
        body: { items },
      });

      return result as BulkTriggerResponse;
    },

    /**
     * Triggers a run and polls until it reaches a terminal status.
     *
     * @param client - A client with `triggerJob` and `getRun` methods.
     * @param input - Trigger input with payload.
     * @param waitOptions - Polling configuration.
     * @returns The final run state after reaching a terminal status.
     */
    triggerAndWait: async (
      client: JobDslClient,
      input: TriggerJobInput<TPayload>,
      waitOptions?: WaitForRunOptions
    ): Promise<{
      readonly status?: string;
      readonly [key: string]: unknown;
    }> => {
      if (!client.getRun) {
        throw new Error("triggerAndWait requires a client with getRun method");
      }

      const run = await trigger(client, input);
      return waitForRun(client.getRun, run.id, waitOptions);
    },
  };
};
