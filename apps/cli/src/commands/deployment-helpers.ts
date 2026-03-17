import { createHash } from "node:crypto";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

import type { StraitProjectConfig, StraitProjectManifest } from "../compiler";

/**
 * Mutation payload shape for finalize/promote/rollback deployment endpoints.
 */
export type DeploymentMutationBody = {
  readonly project_id: string;
  readonly environment: string;
};

/**
 * Resolves deploy target environment from CLI flag or project config defaults.
 */
export const resolveDeploymentEnvironment = (
  config: StraitProjectConfig,
  envOverride?: string
): string => {
  const explicitEnv = envOverride?.trim();
  if (explicitEnv && explicitEnv.length > 0) {
    return explicitEnv;
  }

  const defaultEnv = config.deploy?.defaultEnvironment?.trim();
  return defaultEnv && defaultEnv.length > 0 ? defaultEnv : "production";
};

/**
 * Creates stable SHA-256 checksum for deployment payload provenance.
 */
export const computeManifestChecksum = (
  manifest: StraitProjectManifest
): string =>
  createHash("sha256").update(JSON.stringify(manifest)).digest("hex");

/**
 * Resolves local manifest path used as deployment artifact source.
 */
export const resolveManifestPath = (
  manifest: StraitProjectManifest,
  cwd?: string
): string =>
  resolve(cwd ?? process.cwd(), join(manifest.build.outDir, "manifest.json"));

/**
 * Converts absolute file path into artifact URI.
 */
export const toFileArtifactURI = (absolutePath: string): string =>
  pathToFileURL(absolutePath).href;

/**
 * Creates mutation body shared by finalize/promote/rollback endpoints.
 */
export const createDeploymentMutationBody = (
  projectID: string,
  environment: string
): DeploymentMutationBody => ({
  project_id: projectID,
  environment,
});
