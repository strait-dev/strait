import type {
  HighLevelFunctionMap,
  HighLevelOperationInput,
} from "../high-level/generated";
import type {
  OperationRequestBodyById,
  OperationResponseBodyById,
} from "../internal/schema/_generated/schema";
import { fromPromise, type SdkResult } from "./result";

type DeploymentFunctions = Pick<
  HighLevelFunctionMap,
  | "createDeployment"
  | "finalizeDeployment"
  | "promoteDeployment"
  | "rollbackDeployment"
>;

type DeploymentVersion = OperationResponseBodyById["postV1Deployments"];

type RequiredOperationBody<
  TOperationId extends keyof OperationRequestBodyById,
> = HighLevelOperationInput<TOperationId> & {
  readonly body: NonNullable<OperationRequestBodyById[TOperationId]>;
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
   * Create call input. `body.project_id` and `body.environment` are reused for
   * finalize unless explicitly overridden by `finalizeBody`.
   */
  readonly create: RequiredOperationBody<"postV1Deployments">;
  /**
   * Optional finalize body override. Useful for explicit cross-checking.
   */
  readonly finalizeBody?: NonNullable<
    OperationRequestBodyById["postV1DeploymentsByDeploymentIDFinalize"]
  >;
  /**
   * Optional per-call overrides passed to finalize operation.
   */
  readonly finalize?: Omit<
    HighLevelOperationInput<"postV1DeploymentsByDeploymentIDFinalize">,
    "pathParams" | "body"
  >;
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
export type DeploymentMutationInput<
  TOperationId extends
    | "postV1DeploymentsByDeploymentIDFinalize"
    | "postV1DeploymentsByDeploymentIDPromote"
    | "postV1DeploymentsByDeploymentIDRollback",
> = {
  readonly deploymentID: string;
  readonly body: NonNullable<OperationRequestBodyById[TOperationId]>;
  readonly options?: Omit<
    HighLevelOperationInput<TOperationId>,
    "pathParams" | "body"
  >;
};

/**
 * Finalizes an existing deployment version.
 */
export const finalizeDeploymentVersion = async (
  client: DeploymentFunctions,
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDFinalize">
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
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDFinalize">
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
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDPromote">
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
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDPromote">
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
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDRollback">
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
  input: DeploymentMutationInput<"postV1DeploymentsByDeploymentIDRollback">
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
