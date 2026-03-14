import { fromPromise, type SdkResult } from "../composition/result";
import type { FutureLocalExecutorHooks, SchemaAdapter } from "./types";

type WorkflowRegistrationBody = Readonly<Record<string, unknown>> & {
  readonly project_id: string;
  readonly name: string;
  readonly slug: string;
  readonly steps: readonly Readonly<Record<string, unknown>>[];
};

type WorkflowTriggerBody = Readonly<Record<string, unknown>> & {
  readonly payload?: unknown;
};

type WorkflowDslClient = {
  readonly createWorkflow: (input: {
    readonly body: WorkflowRegistrationBody;
  }) => Promise<unknown>;
  readonly triggerWorkflow: (input: {
    readonly pathParams: { readonly workflowID: string };
    readonly body?: WorkflowTriggerBody;
  }) => Promise<unknown>;
};

type DefineWorkflowOptions<TPayload> = {
  readonly name: string;
  readonly slug: string;
  readonly steps: readonly Readonly<Record<string, unknown>>[];
  readonly schema: SchemaAdapter<TPayload>;
  readonly projectId?: string;
  readonly description?: string;
  readonly defaults?: Readonly<Record<string, unknown>>;
  readonly hooks?: FutureLocalExecutorHooks;
};

const requireProjectId = (
  definitionProjectId: string | undefined,
  registrationProjectId: string | undefined,
  slug: string
): string => {
  const resolved = registrationProjectId ?? definitionProjectId;
  if (!resolved) {
    throw new Error(
      `defineWorkflow(${slug}) requires projectId in definition or register() call`
    );
  }

  return resolved;
};

const extractEntityId = (value: unknown): string | undefined => {
  if (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof (value as { readonly id: unknown }).id === "string"
  ) {
    return (value as { readonly id: string }).id;
  }

  return undefined;
};

export const defineWorkflow = <TPayload>(
  options: DefineWorkflowOptions<TPayload>
) => {
  let lastRegisteredWorkflowId: string | undefined;

  const toRegistrationBody = (projectId?: string): WorkflowRegistrationBody => {
    const resolvedProjectId = requireProjectId(
      options.projectId,
      projectId,
      options.slug
    );

    return {
      ...options.defaults,
      ...(options.description ? { description: options.description } : {}),
      project_id: resolvedProjectId,
      name: options.name,
      slug: options.slug,
      steps: options.steps,
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
    input: {
      readonly workflowID?: string;
      readonly payload: TPayload;
      readonly body?: Readonly<Record<string, unknown>>;
    }
  ): Promise<unknown> => {
    const payload = await options.schema.parse(input.payload);

    return client.triggerWorkflow({
      pathParams: { workflowID: resolveWorkflowID(input.workflowID) },
      body: {
        ...input.body,
        payload,
      },
    });
  };

  return {
    kind: "workflow" as const,
    slug: options.slug,
    schema: options.schema,
    hooks: options.hooks,
    toRegistrationBody,
    register: async (
      client: WorkflowDslClient,
      input?: { readonly projectId?: string }
    ): Promise<unknown> => {
      const body = toRegistrationBody(input?.projectId);
      const created = await client.createWorkflow({ body });

      lastRegisteredWorkflowId =
        extractEntityId(created) ?? lastRegisteredWorkflowId;
      await options.hooks?.onRegister?.({
        kind: "workflow",
        slug: options.slug,
        registration: body,
      });

      return created;
    },
    trigger,
    triggerResult: (
      client: WorkflowDslClient,
      input: {
        readonly workflowID?: string;
        readonly payload: TPayload;
        readonly body?: Readonly<Record<string, unknown>>;
      }
    ): Promise<SdkResult<unknown, unknown>> =>
      fromPromise(() => trigger(client, input)),
  };
};
