/**
 * Base configuration shared by all workflow step types.
 *
 * @example
 * ```ts
 * step.job("process", "job_process", {
 *   dependsOn: ["validate"],
 *   onFailure: "fail_workflow",
 *   retryMaxAttempts: 3,
 *   retryBackoff: "exponential",
 *   resourceClass: "large",
 * })
 * ```
 */
export type BaseStepOptions = {
  readonly dependsOn?: readonly string[];
  readonly condition?: Readonly<Record<string, unknown>>;
  readonly onFailure?: "fail_workflow" | "skip_dependents" | "continue";
  readonly payload?: Readonly<Record<string, unknown>>;
  readonly retryMaxAttempts?: number;
  readonly retryBackoff?: "exponential" | "fixed";
  readonly retryInitialDelaySecs?: number;
  readonly retryMaxDelaySecs?: number;
  readonly timeoutSecsOverride?: number;
  readonly outputTransform?: string;
  readonly concurrencyKey?: string;
  readonly resourceClass?: "small" | "medium" | "large";
};

/** A step that executes a registered job. */
export type JobStep = BaseStepOptions & {
  readonly type: "job";
  readonly stepRef: string;
  readonly jobId: string;
};

/** A step that pauses until manually approved. */
export type ApprovalStep = BaseStepOptions & {
  readonly type: "approval";
  readonly stepRef: string;
  readonly approvalTimeoutSecs?: number;
  readonly approvers?: readonly string[];
};

/** A step that triggers a nested workflow. */
export type SubWorkflowStep = BaseStepOptions & {
  readonly type: "sub_workflow";
  readonly stepRef: string;
  readonly subWorkflowId: string;
  readonly maxNestingDepth?: number;
};

/** A step that pauses until an external event is received. */
export type WaitForEventStep = BaseStepOptions & {
  readonly type: "wait_for_event";
  readonly stepRef: string;
  readonly eventKey: string;
  readonly eventTimeoutSecs?: number;
  readonly eventNotifyUrl?: string;
};

/** A step that sleeps for a fixed duration. */
export type SleepStep = BaseStepOptions & {
  readonly type: "sleep";
  readonly stepRef: string;
  readonly sleepDurationSecs: number;
};

/** Discriminated union of all workflow step types. */
export type WorkflowStepDefinition =
  | JobStep
  | ApprovalStep
  | SubWorkflowStep
  | WaitForEventStep
  | SleepStep;

/**
 * Converts a camelCase step definition to the snake_case API format.
 *
 * @param def - A typed step definition from the step builder.
 * @returns A plain object ready for API registration.
 */
export const stepToApi = (
  def: WorkflowStepDefinition
): Readonly<Record<string, unknown>> => {
  const base: Record<string, unknown> = {
    step_ref: def.stepRef,
    type: def.type,
  };

  if (def.dependsOn?.length) base.depends_on = def.dependsOn;
  if (def.condition) base.condition = def.condition;
  if (def.onFailure) base.on_failure = def.onFailure;
  if (def.payload) base.payload = def.payload;
  if (def.retryMaxAttempts !== undefined)
    base.retry_max_attempts = def.retryMaxAttempts;
  if (def.retryBackoff) base.retry_backoff = def.retryBackoff;
  if (def.retryInitialDelaySecs !== undefined)
    base.retry_initial_delay_secs = def.retryInitialDelaySecs;
  if (def.retryMaxDelaySecs !== undefined)
    base.retry_max_delay_secs = def.retryMaxDelaySecs;
  if (def.timeoutSecsOverride !== undefined)
    base.timeout_secs_override = def.timeoutSecsOverride;
  if (def.outputTransform) base.output_transform = def.outputTransform;
  if (def.concurrencyKey) base.concurrency_key = def.concurrencyKey;
  if (def.resourceClass) base.resource_class = def.resourceClass;

  switch (def.type) {
    case "job":
      base.job_id = def.jobId;
      break;
    case "approval":
      if (def.approvalTimeoutSecs !== undefined)
        base.approval_timeout_secs = def.approvalTimeoutSecs;
      if (def.approvers?.length) base.approvers = def.approvers;
      break;
    case "sub_workflow":
      base.sub_workflow_id = def.subWorkflowId;
      if (def.maxNestingDepth !== undefined)
        base.max_nesting_depth = def.maxNestingDepth;
      break;
    case "wait_for_event":
      base.event_key = def.eventKey;
      if (def.eventTimeoutSecs !== undefined)
        base.event_timeout_secs = def.eventTimeoutSecs;
      if (def.eventNotifyUrl) base.event_notify_url = def.eventNotifyUrl;
      break;
    case "sleep":
      base.sleep_duration_secs = def.sleepDurationSecs;
      break;
  }

  return base;
};

/**
 * Fluent builder for creating type-safe workflow step definitions.
 *
 * @example
 * ```ts
 * const steps = [
 *   step.job("validate", "job_validate"),
 *   step.job("charge", "job_charge", { dependsOn: ["validate"], onFailure: "fail_workflow" }),
 *   step.approval("review", { dependsOn: ["charge"], approvalTimeoutSecs: 3600 }),
 *   step.waitForEvent("confirm", "shipping.confirmed", { dependsOn: ["review"], eventTimeoutSecs: 86400 }),
 *   step.sleep("cooldown", 60, { dependsOn: ["confirm"] }),
 *   step.subWorkflow("notify", "wf_notifications", { dependsOn: ["cooldown"] }),
 * ];
 * ```
 */
export const step = {
  /**
   * Creates a job step that executes a registered job.
   *
   * @param ref - Unique step reference within the workflow.
   * @param jobId - ID of the registered job to execute.
   * @param opts - Optional base step configuration.
   */
  job: (ref: string, jobId: string, opts?: BaseStepOptions): JobStep => ({
    type: "job",
    stepRef: ref,
    jobId,
    ...opts,
  }),

  /**
   * Creates an approval step that pauses until manually approved.
   *
   * @param ref - Unique step reference within the workflow.
   * @param opts - Optional approval-specific and base step configuration.
   */
  approval: (
    ref: string,
    opts?: BaseStepOptions & {
      readonly approvalTimeoutSecs?: number;
      readonly approvers?: readonly string[];
    }
  ): ApprovalStep => ({
    type: "approval",
    stepRef: ref,
    ...opts,
  }),

  /**
   * Creates a sub-workflow step that triggers a nested workflow.
   *
   * @param ref - Unique step reference within the workflow.
   * @param workflowId - ID of the nested workflow to trigger.
   * @param opts - Optional sub-workflow-specific and base step configuration.
   */
  subWorkflow: (
    ref: string,
    workflowId: string,
    opts?: BaseStepOptions & { readonly maxNestingDepth?: number }
  ): SubWorkflowStep => ({
    type: "sub_workflow",
    stepRef: ref,
    subWorkflowId: workflowId,
    ...opts,
  }),

  /**
   * Creates a wait-for-event step that pauses until an external event is received.
   *
   * @param ref - Unique step reference within the workflow.
   * @param eventKey - Event key to wait for.
   * @param opts - Optional event-specific and base step configuration.
   */
  waitForEvent: (
    ref: string,
    eventKey: string,
    opts?: BaseStepOptions & {
      readonly eventTimeoutSecs?: number;
      readonly eventNotifyUrl?: string;
    }
  ): WaitForEventStep => ({
    type: "wait_for_event",
    stepRef: ref,
    eventKey,
    ...opts,
  }),

  /**
   * Creates a sleep step that pauses for a fixed duration.
   *
   * @param ref - Unique step reference within the workflow.
   * @param durationSecs - Sleep duration in seconds.
   * @param opts - Optional base step configuration.
   */
  sleep: (
    ref: string,
    durationSecs: number,
    opts?: BaseStepOptions
  ): SleepStep => ({
    type: "sleep",
    stepRef: ref,
    sleepDurationSecs: durationSecs,
    ...opts,
  }),
};
