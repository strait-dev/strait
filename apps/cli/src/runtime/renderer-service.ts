import { Context, Effect, Layer } from "effect";

import type { RenderFormat, RenderMode } from "./contracts";

/**
 * Renderer mode selection input.
 */
export type RenderSelectionInput = {
  readonly ci?: boolean;
  readonly requestedFormat?: RenderFormat;
};

/**
 * Resolved renderer mode contract.
 */
export type RenderSelection = {
  readonly mode: RenderMode;
  readonly format: RenderFormat;
  readonly animate: boolean;
};

type RendererService = {
  /** Resolves renderer mode from environment and invocation options. */
  readonly resolveSelection: (
    input?: RenderSelectionInput
  ) => Effect.Effect<RenderSelection>;
  /** Writes a plain line to stdout. */
  readonly line: (message: string) => Effect.Effect<void>;
  /** Writes JSON payload to stdout. */
  readonly json: (payload: unknown) => Effect.Effect<void, Error>;
  /** Writes an error message to stderr. */
  readonly error: (message: string) => Effect.Effect<void>;
};

/**
 * Runtime service for command rendering.
 */
export class RendererServiceTag extends Context.Tag("RendererService")<
  RendererServiceTag,
  RendererService
>() {}

const isInteractiveTerminal = (): boolean =>
  Boolean(process.stdout.isTTY) && process.env.CI !== "true";

const hasReducedMotionPreference = (): boolean => {
  const reducedMotion =
    process.env.NO_COLOR === "1" || process.env.CLICOLOR === "0";
  const cliMotionOverride = process.env.STRAIT_REDUCED_MOTION === "true";
  return reducedMotion || cliMotionOverride;
};

const liveRendererService: RendererService = {
  resolveSelection: (input) =>
    Effect.sync(() => {
      const deterministic =
        input?.ci === true ||
        !isInteractiveTerminal() ||
        input?.requestedFormat === "json";

      return {
        mode: deterministic ? "deterministic" : "interactive",
        format: input?.requestedFormat ?? "plain",
        animate: !(deterministic || hasReducedMotionPreference()),
      } satisfies RenderSelection;
    }),
  line: (message) =>
    Effect.sync(() => {
      process.stdout.write(`${message}\n`);
    }),
  json: (payload) =>
    Effect.try({
      try: () => {
        process.stdout.write(`${JSON.stringify(payload, null, 2)}\n`);
      },
      catch: (error) =>
        new Error("failed to render JSON output", { cause: error }),
    }),
  error: (message) =>
    Effect.sync(() => {
      process.stderr.write(`${message}\n`);
    }),
};

/**
 * Live renderer service layer.
 */
export const RendererServiceLive = Layer.succeed(
  RendererServiceTag,
  liveRendererService
);
