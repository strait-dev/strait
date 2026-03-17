import type { OperationInput } from "../domains/index";
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

type DeploymentVersion = OperationResponseBodyById["postV1Deployments"];
type FinalizedDeploymentVersion =
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDFinalize"];
type PromotedDeploymentVersion =
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDPromote"];
type RolledBackDeploymentVersion =
  OperationResponseBodyById["postV1DeploymentsByDeploymentIDRollback"];

type CreateDeploymentOperationInput = OperationInput<
  CreateDeploymentVersionBody,
  DeploymentVersion
>;
type FinalizeDeploymentOperationInput = OperationInput<
  DeploymentVersionMutationBody,
  FinalizedDeploymentVersion
>;
type PromoteDeploymentOperationInput = OperationInput<
  DeploymentVersionMutationBody,
  PromotedDeploymentVersion
>;
type RollbackDeploymentOperationInput = OperationInput<
  DeploymentVersionMutationBody,
  RolledBackDeploymentVersion
>;

type DeploymentOperationClient = {
  readonly operationsPromise: {
    readonly postV1Deployments: (
      input?: CreateDeploymentOperationInput
    ) => Promise<DeploymentVersion>;
    readonly postV1DeploymentsByDeploymentIDFinalize: (
      input?: FinalizeDeploymentOperationInput
    ) => Promise<FinalizedDeploymentVersion>;
    readonly postV1DeploymentsByDeploymentIDPromote: (
      input?: PromoteDeploymentOperationInput
    ) => Promise<PromotedDeploymentVersion>;
    readonly postV1DeploymentsByDeploymentIDRollback: (
      input?: RollbackDeploymentOperationInput
    ) => Promise<RolledBackDeploymentVersion>;
  };
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
  } & Omit<CreateDeploymentOperationInput, "body">;
  /**
   * Optional finalize body override. Defaults to `{ project_id, environment }`
   * from create body.
   */
  readonly finalizeBody?: DeploymentVersionMutationBody;
  /**
   * Optional per-call overrides passed to finalize operation.
   */
  readonly finalize?: Omit<
    FinalizeDeploymentOperationInput,
    "pathParams" | "body"
  >;
};

/**
 * Output payload for create-and-finalize helper.
 */
export type CreateAndFinalizeDeploymentOutput = {
  readonly created: DeploymentVersion;
  readonly finalized: FinalizedDeploymentVersion;
};

/**
 * Input contract for create/finalize/promote helper.
 */
export type CreateFinalizePromoteDeploymentInput =
  CreateAndFinalizeDeploymentInput & {
    /**
     * Optional promote body override. Defaults to `finalizeBody`, then falls
     * back to `{ project_id, environment }` from create body.
     */
    readonly promoteBody?: DeploymentVersionMutationBody;
    /**
     * Optional per-call overrides passed to promote operation.
     */
    readonly promote?: Omit<
      PromoteDeploymentOperationInput,
      "pathParams" | "body"
    >;
  };

/**
 * Output payload for create/finalize/promote helper.
 */
export type CreateFinalizePromoteDeploymentOutput =
  CreateAndFinalizeDeploymentOutput & {
    readonly promoted: PromotedDeploymentVersion;
  };

const inferMutationBodyFromCreate = (
  create: CreateDeploymentVersionBody
): DeploymentVersionMutationBody => ({
  project_id: create.project_id,
  environment: create.environment,
});

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
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<FinalizeDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<FinalizedDeploymentVersion> =>
  client.operationsPromise.postV1DeploymentsByDeploymentIDFinalize({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link finalizeDeploymentVersion}.
 */
export const finalizeDeploymentVersionResult = (
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<FinalizeDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<SdkResult<FinalizedDeploymentVersion, unknown>> =>
  fromPromise(() => finalizeDeploymentVersion(client, input));

/**
 * Promotes a finalized deployment version into active state for an environment.
 */
export const promoteDeploymentVersion = async (
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<PromoteDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<PromotedDeploymentVersion> =>
  client.operationsPromise.postV1DeploymentsByDeploymentIDPromote({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link promoteDeploymentVersion}.
 */
export const promoteDeploymentVersionResult = (
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<PromoteDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<SdkResult<PromotedDeploymentVersion, unknown>> =>
  fromPromise(() => promoteDeploymentVersion(client, input));

/**
 * Rolls back an environment to a previous deployment version.
 */
export const rollbackDeploymentVersion = async (
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<RollbackDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<RolledBackDeploymentVersion> =>
  client.operationsPromise.postV1DeploymentsByDeploymentIDRollback({
    ...input.options,
    pathParams: { deploymentID: input.deploymentID },
    body: input.body,
  });

/**
 * Result variant of {@link rollbackDeploymentVersion}.
 */
export const rollbackDeploymentVersionResult = (
  client: DeploymentOperationClient,
  input: DeploymentMutationInput<
    Omit<RollbackDeploymentOperationInput, "pathParams" | "body">
  >
): Promise<SdkResult<RolledBackDeploymentVersion, unknown>> =>
  fromPromise(() => rollbackDeploymentVersion(client, input));

/**
 * Creates a deployment version and immediately finalizes it.
 */
export const createAndFinalizeDeployment = async (
  client: DeploymentOperationClient,
  input: CreateAndFinalizeDeploymentInput
): Promise<CreateAndFinalizeDeploymentOutput> => {
  const created = await client.operationsPromise.postV1Deployments(
    input.create
  );
  const deploymentID = resolveDeploymentID(created);

  const finalized = await finalizeDeploymentVersion(client, {
    deploymentID,
    body: input.finalizeBody ?? inferMutationBodyFromCreate(input.create.body),
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
  client: DeploymentOperationClient,
  input: CreateAndFinalizeDeploymentInput
): Promise<SdkResult<CreateAndFinalizeDeploymentOutput, unknown>> =>
  fromPromise(() => createAndFinalizeDeployment(client, input));

/**
 * Creates, finalizes, and promotes a deployment version using one contract.
 */
export const createFinalizePromoteDeployment = async (
  client: DeploymentOperationClient,
  input: CreateFinalizePromoteDeploymentInput
): Promise<CreateFinalizePromoteDeploymentOutput> => {
  const { created, finalized } = await createAndFinalizeDeployment(
    client,
    input
  );
  const deploymentID = resolveDeploymentID(finalized);

  const promoted = await promoteDeploymentVersion(client, {
    deploymentID,
    body:
      input.promoteBody ??
      input.finalizeBody ??
      inferMutationBodyFromCreate(input.create.body),
    options: input.promote,
  });

  return {
    created,
    finalized,
    promoted,
  };
};

/**
 * Result variant of {@link createFinalizePromoteDeployment}.
 */
export const createFinalizePromoteDeploymentResult = (
  client: DeploymentOperationClient,
  input: CreateFinalizePromoteDeploymentInput
): Promise<SdkResult<CreateFinalizePromoteDeploymentOutput, unknown>> =>
  fromPromise(() => createFinalizePromoteDeployment(client, input));
