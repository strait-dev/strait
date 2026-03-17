import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { buildProjectManifest, loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import { ApiServiceTag, FsProcessServiceTag } from "../runtime";
import {
  computeManifestChecksum,
  createDeploymentMutationBody,
  resolveDeploymentEnvironment,
  resolveManifestPath,
  toFileArtifactURI,
} from "./deployment-helpers";
import { renderPayload } from "./operational-helpers";

type DeployFlags = {
  readonly config?: string;
  readonly context?: string;
  readonly server?: string;
  readonly env?: string;
  readonly artifactUri?: string;
  readonly dryRun?: boolean;
  readonly json?: boolean;
};

const getDeploymentID = (response: unknown): string => {
  if (typeof response !== "object" || response === null) {
    throw new Error("deployment response does not include an object payload");
  }

  const deploymentID = (response as Record<string, unknown>).id;
  if (typeof deploymentID !== "string" || deploymentID.length === 0) {
    throw new Error("deployment response does not include id");
  }

  return deploymentID;
};

/**
 * `strait deploy` command creates and finalizes a deployment version.
 */
export const deployCommandRoute = buildCommand({
  async func(this: StraitCommandContext, flags: DeployFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const fsProcess = yield* FsProcessServiceTag;
        const apiService = yield* ApiServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for deploy", {
              cause: error,
            }),
        });

        const manifest = buildProjectManifest(loadedConfig.config);
        const environment = resolveDeploymentEnvironment(
          loadedConfig.config,
          flags.env
        );
        const manifestPath = resolveManifestPath(manifest);
        const artifactURI =
          flags.artifactUri && flags.artifactUri.trim().length > 0
            ? flags.artifactUri
            : toFileArtifactURI(manifestPath);

        const createBody = {
          project_id: manifest.project.id,
          environment,
          runtime: manifest.runtime,
          artifact_uri: artifactURI,
          manifest,
          checksum: computeManifestChecksum(manifest),
        };

        if (flags.dryRun) {
          yield* renderPayload(
            {
              action: "deploy",
              create: createBody,
              finalize: createDeploymentMutationBody(
                manifest.project.id,
                environment
              ),
            },
            {
              asJson: Boolean(flags.json),
            }
          );
          return;
        }

        yield* fsProcess.writeTextFile(
          manifestPath,
          `${JSON.stringify(manifest, null, 2)}\n`
        );

        const created = yield* apiService.requestJson<unknown>({
          method: "POST",
          path: "/v1/deployments",
          body: createBody,
          connection: {
            contextName: flags.context,
            serverUrl: flags.server,
            projectId: manifest.project.id,
          },
        });

        const deploymentID = getDeploymentID(created);
        const finalized = yield* apiService.requestJson<unknown>({
          method: "POST",
          path: `/v1/deployments/${encodeURIComponent(deploymentID)}/finalize`,
          body: createDeploymentMutationBody(manifest.project.id, environment),
          connection: {
            contextName: flags.context,
            serverUrl: flags.server,
            projectId: manifest.project.id,
          },
        });

        yield* renderPayload(
          {
            manifest_path: manifestPath,
            deployment: finalized,
          },
          {
            asJson: Boolean(flags.json),
          }
        );
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      config: {
        kind: "parsed",
        parse: String,
        brief: "Path to strait.config file",
        optional: true,
      },
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL override",
        optional: true,
      },
      env: {
        kind: "parsed",
        parse: String,
        brief: "Deployment environment",
        optional: true,
      },
      artifactUri: {
        kind: "parsed",
        parse: String,
        brief: "Artifact URI override",
        optional: true,
      },
      dryRun: {
        kind: "boolean",
        brief: "Print deploy payload without mutating server state",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Create and finalize a deployment version",
  },
});
