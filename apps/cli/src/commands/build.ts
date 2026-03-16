import { join } from "node:path";
import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { buildProjectManifest, loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import { FsProcessServiceTag, RendererServiceTag } from "../runtime";

type BuildFlags = {
  readonly config?: string;
  readonly outDir?: string;
  readonly dryRun?: boolean;
  readonly json?: boolean;
};

/**
 * `strait build` command implementation.
 */
export const buildCommandRoute = buildCommand({
  async func(this: StraitCommandContext, flags: BuildFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const fsProcess = yield* FsProcessServiceTag;
        const renderer = yield* RendererServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for build", {
              cause: error,
            }),
        });

        const manifest = buildProjectManifest(loadedConfig.config, {
          outDirOverride: flags.outDir,
        });

        if (flags.dryRun || flags.json) {
          yield* renderer.json(manifest);
          return;
        }

        const outputPath = join(manifest.build.outDir, "manifest.json");
        yield* fsProcess.writeTextFile(
          outputPath,
          `${JSON.stringify(manifest, null, 2)}\n`
        );

        yield* renderer.line(`Manifest written to ${outputPath}`);
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
      outDir: {
        kind: "parsed",
        parse: String,
        brief: "Output directory override",
        optional: true,
      },
      dryRun: {
        kind: "boolean",
        brief: "Print manifest without writing files",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON manifest",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Compile code-first definitions into a deterministic manifest",
  },
});
