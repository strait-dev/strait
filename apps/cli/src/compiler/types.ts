/**
 * Supported hosted runtime kinds for authored projects.
 */
export type HostedRuntimeKind = "node" | "bun";

/**
 * Minimal job definition shape accepted by compiler.
 */
export type JobDefinitionInput = Readonly<Record<string, unknown>> & {
  readonly slug: string;
  readonly name: string;
  readonly endpointUrl?: string;
  readonly projectId?: string;
};

/**
 * Minimal workflow definition shape accepted by compiler.
 */
export type WorkflowDefinitionInput = Readonly<Record<string, unknown>> & {
  readonly slug: string;
  readonly name: string;
  readonly steps: readonly Readonly<Record<string, unknown>>[];
  readonly projectId?: string;
};

/**
 * Canonical CLI project config shape loaded from `strait.config.ts`.
 */
export type StraitProjectConfig = {
  readonly project: {
    readonly id: string;
    readonly name?: string;
  };
  readonly dirs?: {
    readonly jobs?: string;
    readonly workflows?: string;
  };
  readonly runtime?: {
    readonly kind: HostedRuntimeKind;
  };
  readonly build?: {
    readonly outDir?: string;
  };
  readonly deploy?: {
    readonly defaultEnvironment?: string;
  };
  readonly jobs?: readonly JobDefinitionInput[];
  readonly workflows?: readonly WorkflowDefinitionInput[];
};

/**
 * Manifest produced by code-first compilation.
 */
export type StraitProjectManifest = {
  readonly version: 1;
  readonly project: StraitProjectConfig["project"];
  readonly runtime: HostedRuntimeKind;
  readonly build: {
    readonly outDir: string;
    readonly generatedAt: string;
  };
  readonly jobs: readonly Readonly<Record<string, unknown>>[];
  readonly workflows: readonly Readonly<Record<string, unknown>>[];
};

/**
 * Type helper for `strait.config.ts` authoring.
 */
export const defineConfig = <TConfig extends StraitProjectConfig>(
  config: TConfig
): TConfig => config;
