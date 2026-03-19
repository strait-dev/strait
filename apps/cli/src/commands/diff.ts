import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { buildProjectManifest, loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import { ApiServiceTag, RendererServiceTag } from "../runtime";
import type { DiffResult } from "./diff-helpers";
import { computeDiff } from "./diff-helpers";
import { normalizeCollection } from "./operational-helpers";

type DiffFlags = {
  readonly config?: string;
  readonly context?: string;
  readonly server?: string;
  readonly json?: boolean;
  readonly env?: string;
};

const colorize = (text: string, code: string): string =>
  process.stdout.isTTY ? `\x1b[${code}m${text}\x1b[0m` : text;

const renderDiffPlain = (
  result: DiffResult,
  renderer: {
    line: (msg: string) => Effect.Effect<void>;
  }
): Effect.Effect<void> =>
  Effect.gen(function* () {
    for (const entry of result.additions) {
      yield* renderer.line(
        colorize(`+ NEW ${entry.kind}: ${entry.slug}`, "32")
      );
    }

    for (const entry of result.modifications) {
      yield* renderer.line(
        colorize(`~ MODIFY ${entry.kind}: ${entry.slug}`, "33")
      );
      if (entry.fields) {
        for (const field of entry.fields) {
          yield* renderer.line(
            `    ${field.field}: ${JSON.stringify(field.remote)} -> ${JSON.stringify(field.local)}`
          );
        }
      }
    }

    for (const entry of result.removals) {
      yield* renderer.line(
        colorize(`- REMOVE ${entry.kind}: ${entry.slug}`, "31")
      );
    }

    for (const warning of result.warnings) {
      yield* renderer.line(colorize(`  Warning: ${warning}`, "33"));
    }

    const total =
      result.additions.length +
      result.modifications.length +
      result.removals.length;
    if (total === 0) {
      yield* renderer.line("No changes detected.");
    } else {
      yield* renderer.line(
        `\n${result.additions.length} new, ${result.modifications.length} modified, ${result.removals.length} removed`
      );
    }
  });

/**
 * `strait diff` compares local config against deployed remote state.
 */
export const diffCommandRoute = buildCommand({
  async func(this: StraitCommandContext, flags: DiffFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;
        const renderer = yield* RendererServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for diff", {
              cause: error,
            }),
        });

        const manifest = buildProjectManifest(loadedConfig.config);
        const projectId = manifest.project.id;

        const connectionInput = {
          contextName: flags.context,
          serverUrl: flags.server,
          projectId,
        };

        const remoteJobsRaw = yield* apiService.requestJson<unknown>({
          method: "GET",
          path: "/v1/jobs",
          requireProject: true,
          connection: connectionInput,
        });

        const remoteWorkflowsRaw = yield* apiService.requestJson<unknown>({
          method: "GET",
          path: "/v1/workflows",
          requireProject: true,
          connection: connectionInput,
        });

        const remoteJobs = normalizeCollection(remoteJobsRaw);
        const remoteWorkflows = normalizeCollection(remoteWorkflowsRaw);

        const localJobs = manifest.jobs as Record<string, unknown>[];
        const localWorkflows = manifest.workflows as Record<string, unknown>[];

        const result = computeDiff(
          localJobs,
          localWorkflows,
          remoteJobs,
          remoteWorkflows
        );

        if (flags.json) {
          yield* renderer.json(result);
        } else {
          yield* renderDiffPlain(result, renderer);
        }
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
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
      env: {
        kind: "parsed",
        parse: String,
        brief: "Deployment environment",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Compare local config against deployed remote state",
  },
});
