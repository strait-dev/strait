import { watch } from "node:fs";

import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { buildProjectManifest, loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import { RendererServiceTag } from "../runtime";

type DevFlags = {
  readonly config?: string;
  readonly watch?: boolean;
  readonly json?: boolean;
};

/**
 * `strait dev` command implementation.
 */
export const devCommandRoute = buildCommand({
  async func(this: StraitCommandContext, flags: DevFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const renderer = yield* RendererServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for dev", {
              cause: error,
            }),
        });

        const manifest = buildProjectManifest(loadedConfig.config);

        const summary = {
          configPath: loadedConfig.path,
          runtime: manifest.runtime,
          jobs: manifest.jobs.length,
          workflows: manifest.workflows.length,
          watch: Boolean(flags.watch),
        };

        if (flags.json) {
          yield* renderer.json(summary);
        } else {
          yield* renderer.line(
            `Dev ready (${summary.runtime}) jobs=${summary.jobs} workflows=${summary.workflows}`
          );
          yield* renderer.line(`Config: ${summary.configPath}`);
        }

        if (!flags.watch) {
          return;
        }

        yield* renderer.line("Watching config for one change cycle...");

        yield* Effect.tryPromise({
          try: () =>
            new Promise<void>((resolve, reject) => {
              const watcher = watch(loadedConfig.path, async () => {
                try {
                  const reloadedConfig = await loadProjectConfig({
                    configPath: loadedConfig.path,
                  });
                  const rebuiltManifest = buildProjectManifest(
                    reloadedConfig.config
                  );
                  if (flags.json) {
                    await renderer
                      .json({
                        event: "rebuild",
                        jobs: rebuiltManifest.jobs.length,
                        workflows: rebuiltManifest.workflows.length,
                      })
                      .pipe(Effect.runPromise);
                  } else {
                    await renderer
                      .line(
                        `Rebuilt manifest jobs=${rebuiltManifest.jobs.length} workflows=${rebuiltManifest.workflows.length}`
                      )
                      .pipe(Effect.runPromise);
                  }
                  watcher.close();
                  resolve();
                } catch (error) {
                  watcher.close();
                  reject(error);
                }
              });

              setTimeout(() => {
                watcher.close();
                resolve();
              }, 1000);
            }),
          catch: (error) =>
            new Error("dev watch cycle failed", {
              cause: error,
            }),
        });
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
      watch: {
        kind: "boolean",
        brief: "Watch project config for changes",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON status",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Start local code-first development flow",
  },
});
