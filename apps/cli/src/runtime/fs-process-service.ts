import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import { Context, Effect, Layer } from "effect";

/**
 * Result returned by subprocess execution.
 */
export type ProcessResult = {
  readonly exitCode: number;
  readonly stdout: string;
  readonly stderr: string;
};

/**
 * Process execution options.
 */
export type ProcessRunOptions = {
  readonly cwd?: string;
  readonly env?: Readonly<Record<string, string>>;
  readonly timeoutMs?: number;
};

type FsProcessService = {
  /** Reads UTF-8 text file content. */
  readonly readTextFile: (path: string) => Effect.Effect<string, Error>;
  /** Writes UTF-8 text file content, creating parent directories when required. */
  readonly writeTextFile: (
    path: string,
    content: string
  ) => Effect.Effect<void, Error>;
  /** Returns whether a file path exists. */
  readonly exists: (path: string) => Effect.Effect<boolean>;
  /** Runs a subprocess and captures output streams. */
  readonly run: (
    command: string,
    args: readonly string[],
    options?: ProcessRunOptions
  ) => Effect.Effect<ProcessResult, Error>;
};

/**
 * Runtime service for local filesystem and subprocess operations.
 */
export class FsProcessServiceTag extends Context.Tag("FsProcessService")<
  FsProcessServiceTag,
  FsProcessService
>() {}

const liveFsProcessService: FsProcessService = {
  readTextFile: (path) =>
    Effect.tryPromise({
      try: () => readFile(path, "utf8"),
      catch: (error) =>
        new Error(`failed to read file '${path}'`, { cause: error }),
    }),
  writeTextFile: (path, content) =>
    Effect.gen(function* () {
      yield* Effect.tryPromise({
        try: () => mkdir(dirname(path), { recursive: true }),
        catch: (error) =>
          new Error(`failed to prepare parent directory for '${path}'`, {
            cause: error,
          }),
      });

      yield* Effect.tryPromise({
        try: () => writeFile(path, content, "utf8"),
        catch: (error) =>
          new Error(`failed to write file '${path}'`, { cause: error }),
      });
    }),
  exists: (path) => Effect.sync(() => existsSync(path)),
  run: (command, args, options) =>
    Effect.tryPromise({
      try: () =>
        new Promise<ProcessResult>((resolve, reject) => {
          const child = spawn(command, [...args], {
            cwd: options?.cwd,
            env: options?.env
              ? { ...process.env, ...options.env }
              : process.env,
          });

          let stdout = "";
          let stderr = "";

          const timeout =
            options?.timeoutMs && options.timeoutMs > 0
              ? setTimeout(() => {
                  child.kill("SIGTERM");
                }, options.timeoutMs)
              : undefined;

          child.stdout.on("data", (chunk: Buffer) => {
            stdout += chunk.toString("utf8");
          });

          child.stderr.on("data", (chunk: Buffer) => {
            stderr += chunk.toString("utf8");
          });

          child.on("error", (error) => {
            if (timeout) {
              clearTimeout(timeout);
            }
            reject(error);
          });

          child.on("close", (exitCode) => {
            if (timeout) {
              clearTimeout(timeout);
            }
            resolve({
              exitCode: exitCode ?? 1,
              stdout,
              stderr,
            });
          });
        }),
      catch: (error) =>
        new Error(`failed to run command '${command}'`, {
          cause: error,
        }),
    }),
};

/**
 * Live fs/process service layer.
 */
export const FsProcessServiceLive = Layer.succeed(
  FsProcessServiceTag,
  liveFsProcessService
);
