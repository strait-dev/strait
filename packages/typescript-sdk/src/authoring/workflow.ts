import { fromPromise } from "../composition/result";
import { type WaitForRunOptions, waitForRun } from "../composition/wait";
import { validateDag } from "./dag-validation";
import type { RunContext } from "./job";
import { stepToApi, type WorkflowStepDefinition } from "./steps";
import type {
  FutureLocalExecutorHooks,
  SchemaAdapter,
  TriggerResult,
} from "./types";
import { extractEntityId, requireProjectId } from "./utils";

/**
 * Options for defining a Strait workflow with full configuration.
 *
 * @example
 * ```ts
 * const pipeline = defineWorkflow({
 *   name: "Order Pipeline",
 *   slug: "order-pipeline",
 *   projectId: "proj_1",
 *   schema: zodSchema(z.object({ orderId: z.string() })),
 *   maxConcurrentRuns: 10,
 *   maxParallelSteps: 3,
 *   steps: [
 *     step.job("validate", "job_validate"),
 *     step.job("charge", "job_charge", { dependsOn: ["validate"] }),
 *   ],
 * });
 * ```
 */
export type DefineWorkflowOptions<TPayload> = {
  /** Workflow display name. */
  readonly name: string;
  /** Unique slug identifier. */
  readonly slug: string;
  /** Array of step definitions. Use the `step` builder for type safety. */
  readonly steps:
    | readonly WorkflowStepDefinition[]
    | readonly Readonly<Record<string, unknown>>[];
  /** Schema adapter for trigger payload validation. */
  readonly schema: SchemaAdapter<TPayload>;
  /** Project ID — can also be provided at register() time. */
  readonly projectId?: string;

  /** Human-readable description. */
  readonly description?: string;
  /** Key-value tags for filtering and organization. */
  readonly tags?: Readonly<Record<string, string>>;
  /** Environment ID for multi-environment setups. */
  readonly environmentId?: string;

  /** Maximum concurrent workflow runs. */
  readonly maxConcurrentRuns?: number;
  /** Maximum parallel steps executing simultaneously. */
  readonly maxParallelSteps?: number;

  /** Maximum execution time for the entire workflow in seconds. */
  readonly timeoutSecs?: number;

  /** Maximum number of retry attempts for failed steps. */
  readonly maxAttempts?: number;
  /** Default retry backoff strategy for steps. */
  readonly retryStrategy?: "exponential" | "fixed";

  /** Cron expression for scheduled execution. */
  readonly cron?: string;
  /** IANA timezone for cron evaluation. */
  readonly timezone?: string;

  /** Webhook URL for workflow status notifications. */
  readonly webhookUrl?: string;
  /** Webhook signing secret. */
  readonly webhookSecret?: string;

  /**
   * The code that executes when the workflow runs.
   *
   * Receives the validated payload and a {@link RunContext} for logging,
   * progress reporting, checkpointing, and cancellation.
   */
  readonly run?: (
    payload: TPayload,
    ctx: RunContext
  ) => Promise<unknown> | unknown;

  /** Called after a successful workflow run completes. */
  readonly onSuccess?: (context: {
    readonly payload: TPayload;
    readonly output: unknown;
    readonly ctx: RunContext;
  }) => void | Promise<void>;
  /** Called when a workflow run fails. */
  readonly onFailure?: (context: {
    readonly payload: TPayload;
    readonly error: unknown;
    readonly ctx: RunContext;
  }) => void | Promise<void>;

  /** Extension points for future local execution integrations. */
  readonly hooks?: FutureLocalExecutorHooks;
};

/** Response from registering a workflow with the Strait API. */
export type WorkflowResponse = {
  readonly id: string;
  readonly slug?: string;
  readonly name?: string;
  readonly [key: string]: unknown;
};

/** Response from triggering a workflow run. */
export type WorkflowRunResponse = {
  readonly id: string;
  readonly status?: string;
  readonly [key: string]: unknown;
};

/**
 * Typed trigger input for a workflow run.
 */
export type TriggerWorkflowInput<TPayload> = {
  readonly workflowID?: string;
  readonly payload: TPayload;
  readonly idempotencyKey?: string;
  readonly priority?: number;
  readonly dryRun?: boolean;
  readonly metadata?: Readonly<Record<string, string>>;
  readonly stepOverrides?: Readonly<Record<string, unknown>>;
};

type WorkflowRegistrationBody = {
  readonly project_id: string;
  readonly name: string;
  readonly slug: string;
  readonly steps: readonly Readonly<Record<string, unknown>>[];
  readonly description?: string;
  readonly tags?: Readonly<Record<string, string>>;
  readonly environment_id?: string;
  readonly max_concurrent_runs?: number;
  readonly max_parallel_steps?: number;
  readonly timeout_secs?: number;
  readonly max_attempts?: number;
  readonly retry_strategy?: string;
  readonly cron?: string;
  readonly timezone?: string;
  readonly webhook_url?: string;
  readonly webhook_secret?: string;
  readonly [key: string]: unknown;
};

type WorkflowTriggerBody = {
  readonly payload?: unknown;
  readonly idempotency_key?: string;
  readonly priority?: number;
  readonly dry_run?: boolean;
  readonly metadata?: Readonly<Record<string, string>>;
  readonly step_overrides?: Readonly<Record<string, unknown>>;
};

type WorkflowDslClient = {
  readonly createWorkflow: (input: {
    readonly body: WorkflowRegistrationBody;
  }) => Promise<unknown>;
  readonly triggerWorkflow: (input: {
    readonly pathParams: { readonly workflowID: string };
    readonly body?: WorkflowTriggerBody;
  }) => Promise<unknown>;
  readonly getRun?: (input: {
    readonly pathParams: { readonly runID: string };
  }) => Promise<{ readonly status?: string; readonly [key: string]: unknown }>;
};

/**
 * Defines a reusable workflow authoring unit with schema-backed payload validation.
 *
 * The returned definition can:
 * - build API registration bodies via `toRegistrationBody`
 * - register workflows via `register`
 * - trigger workflows via `trigger` / `triggerResult`
 * - trigger and wait via `triggerAndWait`
 *
 * After a successful `register`, trigger calls may omit `workflowID` and reuse
 * the last registered identifier.
 *
 * @param options - Full workflow configuration including steps, optional run handler, and lifecycle hooks.
 * @returns A workflow definition object with registration and triggering methods.
 */
export const defineWorkflow = <TPayload>(
  options: DefineWorkflowOptions<TPayload>
) => {
  let lastRegisteredWorkflowId: string | undefined;

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: field mapping is necessarily verbose
  const toRegistrationBody = (projectId?: string): WorkflowRegistrationBody => {
    const resolvedProjectId = requireProjectId(
      options.projectId,
      projectId,
      `defineWorkflow(${options.slug})`
    );

    // Convert typed step definitions to API format if they have a `type` field
    // (indicating they came from the step builder), otherwise pass through as-is
    const isTypedStep = (
      s: Readonly<Record<string, unknown>>
    ): s is WorkflowStepDefinition => "stepRef" in s && "type" in s;

    const convertedSteps = options.steps.map((s) =>
      isTypedStep(s as Readonly<Record<string, unknown>>)
        ? stepToApi(s as WorkflowStepDefinition)
        : s
    );

    // Validate DAG if steps are typed (have dependsOn fields)
    const typedSteps = options.steps.filter((s) =>
      isTypedStep(s as Readonly<Record<string, unknown>>)
    ) as WorkflowStepDefinition[];

    if (typedSteps.length > 0) {
      validateDag(typedSteps);
    }

    return {
      project_id: resolvedProjectId,
      name: options.name,
      slug: options.slug,
      steps: convertedSteps as readonly Readonly<Record<string, unknown>>[],
      ...(options.description ? { description: options.description } : {}),
      ...(options.tags ? { tags: options.tags } : {}),
      ...(options.environmentId
        ? { environment_id: options.environmentId }
        : {}),
      ...(options.maxConcurrentRuns === undefined
        ? {}
        : { max_concurrent_runs: options.maxConcurrentRuns }),
      ...(options.maxParallelSteps === undefined
        ? {}
        : { max_parallel_steps: options.maxParallelSteps }),
      ...(options.timeoutSecs === undefined
        ? {}
        : { timeout_secs: options.timeoutSecs }),
      ...(options.maxAttempts === undefined
        ? {}
        : { max_attempts: options.maxAttempts }),
      ...(options.retryStrategy
        ? { retry_strategy: options.retryStrategy }
        : {}),
      ...(options.cron ? { cron: options.cron } : {}),
      ...(options.timezone ? { timezone: options.timezone } : {}),
      ...(options.webhookUrl ? { webhook_url: options.webhookUrl } : {}),
      ...(options.webhookSecret
        ? { webhook_secret: options.webhookSecret }
        : {}),
    };
  };

  const resolveWorkflowID = (workflowID?: string): string => {
    const resolved = workflowID ?? lastRegisteredWorkflowId;
    if (!resolved) {
      throw new Error(
        `defineWorkflow(${options.slug}) trigger requires workflowID or prior successful register()`
      );
    }

    return resolved;
  };

  const trigger = async (
    client: WorkflowDslClient,
    input: TriggerWorkflowInput<TPayload>
  ): Promise<WorkflowRunResponse> => {
    const payload = await options.schema.parse(input.payload);

    const body: WorkflowTriggerBody = {
      payload,
      ...(input.idempotencyKey
        ? { idempotency_key: input.idempotencyKey }
        : {}),
      ...(input.priority === undefined ? {} : { priority: input.priority }),
      ...(input.dryRun === undefined ? {} : { dry_run: input.dryRun }),
      ...(input.metadata ? { metadata: input.metadata } : {}),
      ...(input.stepOverrides ? { step_overrides: input.stepOverrides } : {}),
    };

    const result = await client.triggerWorkflow({
      pathParams: {
        workflowID: resolveWorkflowID(input.workflowID),
      },
      body,
    });

    return result as WorkflowRunResponse;
  };

  return {
    kind: "workflow" as const,
    slug: options.slug,
    schema: options.schema,
    hooks: options.hooks,
    run: options.run,
    onSuccess: options.onSuccess,
    onFailure: options.onFailure,
    toRegistrationBody,

    /**
     * Registers this workflow with the Strait API.
     */
    register: async (
      client: WorkflowDslClient,
      input?: { readonly projectId?: string }
    ): Promise<WorkflowResponse> => {
      const body = toRegistrationBody(input?.projectId);
      const created = await client.createWorkflow({ body });

      lastRegisteredWorkflowId =
        extractEntityId(created) ?? lastRegisteredWorkflowId;
      await options.hooks?.onRegister?.({
        kind: "workflow",
        slug: options.slug,
        registration: body,
      });

      return created as WorkflowResponse;
    },

    /**
     * Triggers a run of this workflow.
     */
    trigger,

    /**
     * Triggers a run and returns a {@link SdkResult} instead of throwing.
     */
    triggerResult: (
      client: WorkflowDslClient,
      input: TriggerWorkflowInput<TPayload>
    ): TriggerResult<WorkflowRunResponse> =>
      fromPromise(() => trigger(client, input)),

    /**
     * Triggers a workflow run and polls until it reaches a terminal status.
     */
    triggerAndWait: async (
      client: WorkflowDslClient,
      input: TriggerWorkflowInput<TPayload>,
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
