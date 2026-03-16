import type { RunContext } from "./job";
import {
  type CreateRunContextOptions,
  createRunContext,
  type RunContextClient,
} from "./run-context";

type SdkDomain = {
  readonly checkpointRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly heartbeatRun: (input: {
    readonly pathParams: { readonly runID: string };
  }) => Promise<unknown>;
  readonly progressRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly logRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly usageRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly toolCallRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly outputRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly waitForEventRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly spawnRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly continueRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body?: unknown;
  }) => Promise<unknown>;
  readonly annotateRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
  readonly completeRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body?: unknown;
  }) => Promise<unknown>;
  readonly failRun: (input: {
    readonly pathParams: { readonly runID: string };
    readonly body: unknown;
  }) => Promise<unknown>;
};

type StraitClientLike = {
  readonly domainsPromise: {
    readonly sdk: SdkDomain;
  };
};

export const createRunContextFromClient = (
  client: StraitClientLike,
  runId: string,
  options?: CreateRunContextOptions
): RunContext => {
  const sdk = client.domainsPromise.sdk;

  const wrappedClient: RunContextClient = {
    checkpointRun: (input) => sdk.checkpointRun(input),
    heartbeatRun: (input) => sdk.heartbeatRun(input),
    progressRun: (input) => sdk.progressRun(input),
    logRun: (input) => sdk.logRun(input),
    usageRun: (input) => sdk.usageRun(input),
    toolCallRun: (input) => sdk.toolCallRun(input),
    outputRun: (input) => sdk.outputRun(input),
    waitForEventRun: (input) => sdk.waitForEventRun(input),
    // Remap: the generated client sends job_id (from OpenAPI spec)
    // but the Go backend expects job_slug. We pass the body directly
    // since our RunContextClient interface already uses job_slug.
    spawnRun: (input) => sdk.spawnRun(input),
    continueRun: (input) => sdk.continueRun(input),
    annotateRun: (input) => sdk.annotateRun(input),
    completeRun: (input) => sdk.completeRun(input),
    failRun: (input) => sdk.failRun(input),
  };

  return createRunContext(wrappedClient, runId, options);
};
