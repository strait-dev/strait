import type { HighLevelOperationInput } from "../high-level/generated";
import type { OperationResponseBodyById } from "../internal/schema/_generated/schema";
import { fromPromise, type SdkResult } from "./result";

/**
 * Request payload for creating a deployment version.
 */
export type CreateDeploymentVersionBody = {
  readonly project_id: string;
  readonly environment: string;
  readonly runtime: "node" | "bun";
  readonly artifact_uri: string;
  readonly manifest?: Readonly<Record<string, unknown>>;
  readonly checksum?: string;
};

/**
 * Request payload for finalize/promote/rollback deployment mutations.
 */
export type DeploymentVersionMutationBody = {
  readonly project_id: string;
  readonly environment: string;
};

type DeploymentCreateOptions = Omit<
  HighLevelOperationInput<"postV1Deployments">,
  "body"
>;

type DeploymentFinalizeOptions = Omit<
  HighLevelOperationInput<"postV1DeploymentsByDeploymentIDFinalize">,
  "pathParams" | "body"
>;

type DeploymentPromoteOptions = Omit<
  HighLevelOperationInput<"postV1DeploymentsByDeploymentIDPromote">,
  "pathParams" | "body"
>;

type DeploymentRollbackOptions = Omit<
  HighLevelOperationInput<"postV1DeploymentsByDeploymentIDRollback">,
  "pathParams" | "body"
>;

type DeploymentVersion = OperationResponseBodyById["postV1Deployments"];

type DeploymentFunctions = {
  readonly createDeployment: (
    input: {
      readonly body: CreateDeploymentVersionBody;
    } & DeploymentCreateOptions
  ) => Promise<DeploymentVersion>;
  readonly finalizeDeployment: (
    input: {
      readonly pathParams: { readonly deploymentID: string };
      readonly body: DeploymentVersionMutationBody;
    } & DeploymentFinalizeOptions
  ) => Promise<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDFinalize"]
  >;
  readonly promoteDeployment: (
    input: {
      readonly pathParams: { readonly deploymentID: string };
      readonly body: DeploymentVersionMutationBody;
    } & DeploymentPromoteOptions
  ) => Promise<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDPromote"]
  >;
  readonly rollbackDeployment: (
    input: {
      readonly pathParams: { readonly deploymentID: string };
      readonly body: DeploymentVersionMutationBody;
    } & DeploymentRollbackOptions
  ) => Promise<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDRollback"]
  >;
};

const resolveDeploymentID = (deployment: DeploymentVersion): string => {
  if (
    typeof deployment !== "object" ||
    deployment === null ||
    typeof deployment.id !== "string" ||
    deployment.id.length === 0
  ) {
    throw new Error("deployment response is missing a usable id");
  }

  return deployment.id;
};

/**
 * Input contract for creating and immediately finalizing a deployment version.
 */
export type CreateAndFinalizeDeploymentInput = {
  /**
   * Create payload and optional call-level overrides.
   */
  readonly create: {
    readonly body: CreateDeploymentVersionBody;
  } & DeploymentCreateOptions;
  /**
   * Optional finalize body override. Defaults to `{ project_id, environment }`
   * from create body.
   */
  readonly finalizeBody?: DeploymentVersionMutationBody;
  /**
   * Optional per-call overrides passed to finalize operation.
   */
  readonly finalize?: DeploymentFinalizeOptions;
};

/**
 * Output payload for create-and-finalize helper.
 */
export type CreateAndFinalizeDeploymentOutput = {
  readonly created: DeploymentVersion;
  readonly finalized: OperationResponseBodyById["postV1DeploymentsByDeploymentIDFinalize"];
};

/**
 * Input contract for mutation operations that target one deployment version.
 */
export type DeploymentMutationInput<TOptions> = {
  readonly deploymentID: string;
  readonly body: DeploymentVersionMutationBody;
  readonly options?: TOptions;
};

/**
 * Finalizes an existing deployment version.
 */
export const finalizeDeploymentVersion = async (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentFinalizeOptions>
): Promise<
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDFinalize"]
> =>
  client.finalizeDeployment({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link finalizeDeploymentVersion}.
 */
export const finalizeDeploymentVersionResult = (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentFinalizeOptions>
): Promise<
  SdkResult<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDFinalize"],
    unknown
  >
> => fromPromise(() => finalizeDeploymentVersion(client, input));

/**
 * Promotes a finalized deployment version into active state for an environment.
 */
export const promoteDeploymentVersion = async (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentPromoteOptions>
): Promise<
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDPromote"]
> =>
  client.promoteDeployment({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link promoteDeploymentVersion}.
 */
export const promoteDeploymentVersionResult = (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentPromoteOptions>
): Promise<
  SdkResult<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDPromote"],
    unknown
  >
> => fromPromise(() => promoteDeploymentVersion(client, input));

/**
 * Rolls back an environment to a previous deployment version.
 */
export const rollbackDeploymentVersion = async (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentRollbackOptions>
): Promise<
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDRollback"]
> =>
  client.rollbackDeployment({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link rollbackDeploymentVersion}.
 */
export const rollbackDeploymentVersionResult = (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<DeploymentRollbackOptions>
): Promise<
  SdkResult<
    OperationResponseBodyById["postV1DeploymentsByDeploymentIDRollback"],
    unknown
  >
> => fromPromise(() => rollbackDeploymentVersion(client, input));

/**
 * Creates a deployment version and immediately finalizes it.
 */
export const createAndFinalizeDeployment = async (
  client: DeploymentFunctions,
  input: CreateAndFinalizeDeploymentInput
): Promise<CreateAndFinalizeDeploymentOutput> => {
  const created = await client.createDeployment(input.create);
  const deploymentID = resolveDeploymentID(created);

  const finalized = await finalizeDeploymentVersion(client, {
    deploymentID,
    body: input.finalizeBody ?? {
      project_id: input.create.body.project_id,
      environment: input.create.body.environment,
    },
    options: input.finalize,
  });

  return {
    created,
    finalized,
  };
};

/**
 * Result variant of {@link createAndFinalizeDeployment}.
 */
export const createAndFinalizeDeploymentResult = (
  client: DeploymentFunctions,
  input: CreateAndFinalizeDeploymentInput
): Promise<SdkResult<CreateAndFinalizeDeploymentOutput, unknown>> =>
  fromPromise(() => createAndFinalizeDeployment(client, input));
