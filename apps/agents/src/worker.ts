import { Cause, Effect, Exit } from "effect";

import {
  buildNDJSONResponseBody,
  buildRuntimeOutput,
  parseEnvelope,
} from "./core";

export type RuntimeWorkerEnv = {
  AGENT_RUNTIME_AUTH_TOKEN?: string;
  STRAIT_ENV?: string;
};

function jsonResponse(status: number, body: Record<string, unknown>): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json; charset=utf-8",
    },
  });
}

function readBearerToken(request: Request): string {
  const authorization = request.headers.get("authorization")?.trim() ?? "";
  if (!authorization.toLowerCase().startsWith("bearer ")) {
    return "";
  }
  return authorization.slice("Bearer ".length).trim();
}

export function verifyRuntimeWorkerAuth(
  request: Request,
  env: RuntimeWorkerEnv
): Effect.Effect<void, Error> {
  return Effect.sync(() => {
    const expected = env.AGENT_RUNTIME_AUTH_TOKEN?.trim();
    if (!expected) {
      throw new Error("AGENT_RUNTIME_AUTH_TOKEN is required");
    }

    const received = readBearerToken(request);
    if (!received) {
      throw new Error("authorization bearer token is required");
    }
    if (received !== expected) {
      throw new Error("authorization bearer token is invalid");
    }
  });
}

function statusForWorkerError(error: Error): number {
  if (error.message.includes("authorization")) {
    return 401;
  }
  if (error.message.includes("AGENT_RUNTIME_AUTH_TOKEN")) {
    return 500;
  }
  if (error.message.includes("dispatch envelope")) {
    return 400;
  }
  return 422;
}

function toError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}

export async function handleWorkerFetch(
  request: Request,
  env: RuntimeWorkerEnv
): Promise<Response> {
  if (request.method !== "POST") {
    return jsonResponse(405, {
      error: "method_not_allowed",
      message: "runtime worker only accepts POST requests",
    });
  }

  const program = verifyRuntimeWorkerAuth(request, env).pipe(
    Effect.flatMap(() =>
      Effect.tryPromise({
        try: () => request.text(),
        catch: (error) =>
          error instanceof Error ? error : new Error(String(error)),
      })
    ),
    Effect.flatMap(parseEnvelope),
    Effect.flatMap(buildRuntimeOutput),
    Effect.map((outputs) => buildNDJSONResponseBody(outputs))
  );

  const exit = await Effect.runPromiseExit(program);
  if (Exit.isFailure(exit)) {
    const error = toError(Cause.squash(exit.cause));
    return jsonResponse(statusForWorkerError(error), {
      error: "runtime_worker_error",
      message: error.message,
    });
  }

  return new Response(exit.value, {
    status: 200,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/x-ndjson; charset=utf-8",
    },
  });
}

const worker = {
  fetch(request: Request, env: RuntimeWorkerEnv): Promise<Response> {
    return handleWorkerFetch(request, env);
  },
};

export default worker;
