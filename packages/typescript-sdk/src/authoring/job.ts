import { fromPromise } from "../composition/result";
import type {
  FutureLocalExecutorHooks,
  SchemaAdapter,
  TriggerResult,
} from "./types";

type JobRegistrationBody = Readonly<Record<string, unknown>> & {
  readonly project_id: string;
  readonly name: string;
  readonly slug: string;
  readonly endpoint_url: string;
  readonly payload_schema?: Readonly<Record<string, unknown>>;
};

type JobTriggerBody = Readonly<Record<string, unknown>> & {
  readonly payload?: unknown;
};

type JobDslClient = {
  readonly createJob: (input: {
    readonly body: JobRegistrationBody;
  }) => Promise<unknown>;
  readonly triggerJob: (input: {
    readonly pathParams: { readonly jobID: string };
    readonly body?: JobTriggerBody;
  }) => Promise<unknown>;
};

type DefineJobOptions<TPayload> = {
  readonly name: string;
  readonly slug: string;
  readonly endpointUrl: string;
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
      `defineJob(${slug}) requires projectId in definition or register() call`
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

/**
 * Defines a reusable job authoring unit with schema-backed payload validation.
 *
 * The returned definition can:
 * - build API registration bodies via `toRegistrationBody`
 * - register jobs via `register`
 * - trigger jobs via `trigger` / `triggerResult`
 *
 * After a successful `register`, trigger calls may omit `jobID` and reuse the
 * last registered identifier.
 */
export const defineJob = <TPayload>(options: DefineJobOptions<TPayload>) => {
  let lastRegisteredJobId: string | undefined;

  const toRegistrationBody = (projectId?: string): JobRegistrationBody => {
    const resolvedProjectId = requireProjectId(
      options.projectId,
      projectId,
      options.slug
    );

    const payloadSchema = options.schema.toJsonSchema?.();

    return {
      ...options.defaults,
      ...(options.description ? { description: options.description } : {}),
      ...(payloadSchema ? { payload_schema: payloadSchema } : {}),
      project_id: resolvedProjectId,
      name: options.name,
      slug: options.slug,
      endpoint_url: options.endpointUrl,
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

  const trigger = async (
    client: JobDslClient,
    input: {
      readonly jobID?: string;
      readonly payload: TPayload;
      readonly body?: Readonly<Record<string, unknown>>;
    }
  ): Promise<unknown> => {
    const payload = await options.schema.parse(input.payload);

    return client.triggerJob({
      pathParams: { jobID: resolveJobID(input.jobID) },
      body: {
        ...input.body,
        payload,
      },
    });
  };

  return {
    kind: "job" as const,
    slug: options.slug,
    schema: options.schema,
    hooks: options.hooks,
    toRegistrationBody,
    register: async (
      client: JobDslClient,
      input?: { readonly projectId?: string }
    ): Promise<unknown> => {
      const body = toRegistrationBody(input?.projectId);
      const created = await client.createJob({ body });

      lastRegisteredJobId = extractEntityId(created) ?? lastRegisteredJobId;
      await options.hooks?.onRegister?.({
        kind: "job",
        slug: options.slug,
        registration: body,
      });

      return created;
    },
    trigger,
    triggerResult: (
      client: JobDslClient,
      input: {
        readonly jobID?: string;
        readonly payload: TPayload;
        readonly body?: Readonly<Record<string, unknown>>;
      }
    ): TriggerResult<unknown> => fromPromise(() => trigger(client, input)),
  };
};
