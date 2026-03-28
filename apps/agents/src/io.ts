import { readFileSync } from "node:fs";

import { Cause, Effect, Exit } from "effect";

import {
  buildRuntimeOutput,
  parseEnvelope,
  serializeOutputLine,
} from "./runtime";

function toError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}

export const readEnvelopeFromStdin = Effect.try({
  try: () => readFileSync(0, "utf8"),
  catch: toError,
}).pipe(Effect.flatMap(parseEnvelope));

export function writeOutput(
  lines: readonly string[]
): Effect.Effect<void, Error> {
  return Effect.try({
    try: () => {
      for (const line of lines) {
        process.stdout.write(`${line}\n`);
      }
    },
    catch: toError,
  });
}

export const program = readEnvelopeFromStdin.pipe(
  Effect.flatMap(buildRuntimeOutput),
  Effect.map((outputs) => outputs.map(serializeOutputLine)),
  Effect.flatMap(writeOutput)
);

export async function runCLI(): Promise<void> {
  const exit = await Effect.runPromiseExit(program);
  if (Exit.isSuccess(exit)) {
    return;
  }

  process.stderr.write(`${Cause.pretty(exit.cause)}\n`);
  process.exitCode = 1;
}
