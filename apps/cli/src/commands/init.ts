import { constants } from "node:fs";
import { access, readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { createInterface } from "node:readline/promises";
import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { RendererServiceTag } from "../runtime";

type InitFlags = {
  readonly yes?: boolean;
  readonly project?: string;
  readonly json?: boolean;
};

const fileExists = async (path: string): Promise<boolean> => {
  try {
    await access(path, constants.F_OK);
    return true;
  } catch {
    return false;
  }
};

const prompt = async (
  rl: ReturnType<typeof createInterface>,
  message: string,
  defaultValue?: string
): Promise<string> => {
  const suffix = defaultValue ? ` (${defaultValue})` : "";
  const answer = await rl.question(`${message}${suffix}: `);
  return answer.trim() || defaultValue || "";
};

const promptYesNo = async (
  rl: ReturnType<typeof createInterface>,
  message: string,
  defaultValue = false
): Promise<boolean> => {
  const hint = defaultValue ? "(Y/n)" : "(y/N)";
  const answer = await rl.question(`${message} ${hint}: `);
  const trimmed = answer.trim().toLowerCase();
  if (trimmed === "") {
    return defaultValue;
  }
  return trimmed === "y" || trimmed === "yes";
};

type StraitJsonConfig = {
  $schema: string;
  project: { id: string; name?: string };
  sdk?: { base_url?: string; auth_type?: string; timeout_ms?: number };
  src: string;
  runtime: string;
  build: { out_dir: string };
  deploy?: { default_environment?: string };
};

const runNonInteractive = (flags: InitFlags, configPath: string) =>
  Effect.gen(function* () {
    const renderer = yield* RendererServiceTag;
    const projectId = flags.project;
    if (!projectId) {
      return yield* Effect.fail(
        new Error(
          "Project ID is required in non-interactive mode. Pass --project <id>."
        )
      );
    }

    const config: StraitJsonConfig = {
      $schema: "https://strait.dev/schema.json",
      project: { id: projectId },
      src: "src",
      runtime: "node",
      build: { out_dir: ".strait" },
    };

    yield* Effect.tryPromise(() =>
      writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`)
    );

    if (flags.json) {
      yield* renderer.json(config);
    } else {
      yield* renderer.line("Created strait.json");
    }
  });

const appendToGitignore = (cwd: string) =>
  Effect.gen(function* () {
    const renderer = yield* RendererServiceTag;
    const gitignorePath = resolve(cwd, ".gitignore");
    const rl = createInterface({
      input: process.stdin,
      output: process.stdout,
    });

    try {
      const exists = yield* Effect.tryPromise(() => fileExists(gitignorePath));
      if (!exists) {
        return;
      }

      const content = yield* Effect.tryPromise(() =>
        readFile(gitignorePath, "utf-8")
      );
      if (content.includes(".strait")) {
        return;
      }

      const shouldAdd = yield* Effect.tryPromise(() =>
        promptYesNo(rl, "Add .strait to .gitignore?", true)
      );
      if (!shouldAdd) {
        return;
      }

      const newContent = content.endsWith("\n")
        ? `${content}.strait\n`
        : `${content}\n.strait\n`;
      yield* Effect.tryPromise(() => writeFile(gitignorePath, newContent));
      yield* renderer.line("Added .strait to .gitignore");
    } finally {
      rl.close();
    }
  });

const runInteractive = (flags: InitFlags, configPath: string, cwd: string) =>
  Effect.gen(function* () {
    const renderer = yield* RendererServiceTag;
    const rl = createInterface({
      input: process.stdin,
      output: process.stdout,
    });

    try {
      if (yield* Effect.tryPromise(() => fileExists(configPath))) {
        const overwrite = yield* Effect.tryPromise(() =>
          promptYesNo(rl, "A config file already exists. Overwrite?", false)
        );
        if (!overwrite) {
          yield* renderer.line("Aborted.");
          return;
        }
      }

      const projectId =
        flags.project ||
        (yield* Effect.tryPromise(() => prompt(rl, "Project ID")));
      if (!projectId) {
        return yield* Effect.fail(new Error("Project ID is required."));
      }

      const projectName = yield* Effect.tryPromise(() =>
        prompt(rl, "Project name (optional, Enter to skip)")
      );

      const src = yield* Effect.tryPromise(() =>
        prompt(rl, "Source directory", "src")
      );

      const runtimeAnswer = yield* Effect.tryPromise(() =>
        prompt(rl, "Runtime (node/bun)", "node")
      );
      const runtime = runtimeAnswer === "bun" ? "bun" : "node";

      const baseUrl = yield* Effect.tryPromise(() =>
        prompt(rl, "Base URL (optional, Enter to use env var)")
      );

      const config: StraitJsonConfig = {
        $schema: "https://strait.dev/schema.json",
        project: {
          id: projectId,
          ...(projectName ? { name: projectName } : {}),
        },
        ...(baseUrl ? { sdk: { base_url: baseUrl } } : {}),
        src,
        runtime,
        build: { out_dir: ".strait" },
      };

      yield* Effect.tryPromise(() =>
        writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`)
      );

      yield* renderer.line("Created strait.json");
    } finally {
      rl.close();
    }

    yield* appendToGitignore(cwd);
  });

/**
 * `strait init` command implementation.
 */
export const initCommand = buildCommand({
  async func(this: StraitCommandContext, flags: InitFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const cwd = process.cwd();
        const configPath = resolve(cwd, "strait.json");

        if (flags.yes) {
          yield* runNonInteractive(flags, configPath);
        } else {
          yield* runInteractive(flags, configPath, cwd);
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
      yes: {
        kind: "boolean",
        brief: "Non-interactive mode (use defaults)",
        optional: true,
      },
      project: {
        kind: "parsed",
        parse: String,
        brief: "Project ID",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
    aliases: {
      y: "yes",
    },
  },
  docs: {
    brief: "Initialize a new strait.json config file",
  },
});
