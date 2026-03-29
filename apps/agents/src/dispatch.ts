import type { DispatchEnvelope, JsonValue, RuntimeEvent } from "./types";

export type CloudflareDispatchRequest = {
  deployment_id: string;
  provider: string;
  namespace: string;
  script_name: string;
  run_id: string;
  envelope: DispatchEnvelope;
};

export type DispatchWorkerEnv = {
  AGENT_RUNTIME_AUTH_TOKEN?: string;
  INTERNAL_SECRET?: string;
  DISPATCHER: {
    get(
      scriptName: string
    ): { fetch(request: Request): Promise<Response> } | null;
  };
};

type DispatchWorkerDeps = {
  fetch: typeof fetch;
};

const defaultDeps: DispatchWorkerDeps = {
  fetch: globalThis.fetch.bind(globalThis),
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

function validateDispatchAuth(
  request: Request,
  env: DispatchWorkerEnv
): Error | null {
  const expected = env.INTERNAL_SECRET?.trim();
  if (!expected) {
    return new Error("INTERNAL_SECRET is required");
  }
  const received = readBearerToken(request);
  if (!received) {
    return new Error("authorization bearer token is required");
  }
  if (received !== expected) {
    return new Error("authorization bearer token is invalid");
  }
  return null;
}

function parseDispatchRequest(value: unknown): CloudflareDispatchRequest {
  if (!value || typeof value !== "object") {
    throw new Error("dispatch request must be an object");
  }
  const request = value as Partial<CloudflareDispatchRequest>;
  if (
    typeof request.script_name !== "string" ||
    request.script_name.length === 0
  ) {
    throw new Error("dispatch request script_name is required");
  }
  if (typeof request.namespace !== "string" || request.namespace.length === 0) {
    throw new Error("dispatch request namespace is required");
  }
  if (typeof request.run_id !== "string" || request.run_id.length === 0) {
    throw new Error("dispatch request run_id is required");
  }
  if (!request.envelope || typeof request.envelope !== "object") {
    throw new Error("dispatch request envelope is required");
  }
  return request as CloudflareDispatchRequest;
}

function parseRuntimeEvent(line: string): RuntimeEvent {
  return JSON.parse(line) as RuntimeEvent;
}

function normalizeJSON(
  value: JsonValue | undefined,
  fallback: JsonValue
): JsonValue {
  return value ?? fallback;
}

function callbackRequestForEvent(event: RuntimeEvent): {
  path: string;
  body: Record<string, JsonValue | string | number | boolean>;
} {
  switch (event.type) {
    case "checkpoint":
      return {
        path: "checkpoint",
        body: {
          source: "agents_dispatch_worker",
          state: normalizeJSON(event.state, {}),
        },
      };
    case "usage":
      return {
        path: "usage",
        body: {
          provider: event.provider,
          model: event.model,
          prompt_tokens: event.prompt_tokens,
          completion_tokens: event.completion_tokens,
          total_tokens:
            event.total_tokens ?? event.prompt_tokens + event.completion_tokens,
          cost_microusd: event.cost_microusd ?? 0,
        },
      };
    case "tool_call":
      return {
        path: "tool-call",
        body: {
          tool_name: event.tool_name,
          input: normalizeJSON(event.input, {}),
          output: normalizeJSON(event.output, {}),
          duration_ms: event.duration_ms ?? 0,
          status: event.status ?? "completed",
        },
      };
    case "stream":
      return {
        path: "stream",
        body: {
          chunk: event.chunk,
          stream_id: event.stream_id ?? "default",
          done: event.done ?? false,
        },
      };
    case "complete":
      return {
        path: "complete",
        body: {
          result: normalizeJSON(event.result, null),
        },
      };
    case "fail":
      return {
        path: "fail",
        body: {
          error: event.error,
        },
      };
    default:
      throw new Error("unsupported runtime event");
  }
}

async function forwardRuntimeEvents(
  request: CloudflareDispatchRequest,
  runtimeResponse: Response,
  deps: DispatchWorkerDeps
): Promise<void> {
  const text = await runtimeResponse.text();
  const lines = text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  for (const line of lines) {
    const event = parseRuntimeEvent(line);
    const callback = callbackRequestForEvent(event);
    const endpoint = new URL(
      `/sdk/v1/runs/${request.envelope.callback.run_id}/${callback.path}`,
      request.envelope.callback.base_url
    );
    const response = await deps.fetch(endpoint.toString(), {
      method: "POST",
      headers: {
        authorization: `Bearer ${request.envelope.callback.run_token}`,
        "content-type": "application/json",
      },
      body: JSON.stringify(callback.body),
    });
    if (!response.ok) {
      const body = await response.text();
      throw new Error(
        `callback ${callback.path} failed with ${response.status}: ${body.trim()}`
      );
    }
  }
}

export async function handleDispatchFetch(
  request: Request,
  env: DispatchWorkerEnv,
  deps: DispatchWorkerDeps = defaultDeps
): Promise<Response> {
  if (request.method !== "POST") {
    return jsonResponse(405, {
      error: "method_not_allowed",
      message: "dispatch worker only accepts POST requests",
    });
  }

  const authError = validateDispatchAuth(request, env);
  if (authError) {
    return jsonResponse(authError.message.includes("required") ? 500 : 401, {
      error: "dispatch_auth_error",
      message: authError.message,
    });
  }

  let dispatchRequest: CloudflareDispatchRequest;
  try {
    dispatchRequest = parseDispatchRequest(await request.json());
  } catch (error) {
    return jsonResponse(400, {
      error: "invalid_dispatch_request",
      message: error instanceof Error ? error.message : String(error),
    });
  }

  const runtimeWorker = env.DISPATCHER.get(dispatchRequest.script_name);
  if (!runtimeWorker) {
    return jsonResponse(404, {
      error: "runtime_worker_not_found",
      message: `no worker is deployed for ${dispatchRequest.script_name}`,
    });
  }

  const runtimeAuthToken =
    env.AGENT_RUNTIME_AUTH_TOKEN?.trim() ?? env.INTERNAL_SECRET?.trim() ?? "";
  const runtimeResponse = await runtimeWorker.fetch(
    new Request(`https://${dispatchRequest.script_name}.dispatch/run`, {
      method: "POST",
      headers: {
        authorization: `Bearer ${runtimeAuthToken}`,
        "content-type": "application/json",
      },
      body: JSON.stringify(dispatchRequest.envelope),
    })
  );

  if (!runtimeResponse.ok) {
    return jsonResponse(502, {
      error: "runtime_worker_failed",
      message: `runtime worker returned ${runtimeResponse.status}`,
      runtime_status: runtimeResponse.status,
    });
  }

  try {
    await forwardRuntimeEvents(dispatchRequest, runtimeResponse, deps);
  } catch (error) {
    return jsonResponse(502, {
      error: "runtime_callback_failed",
      message: error instanceof Error ? error.message : String(error),
    });
  }

  return jsonResponse(202, {
    ok: true,
    run_id: dispatchRequest.run_id,
    script_name: dispatchRequest.script_name,
    status: "accepted",
  });
}

const dispatchWorker = {
  fetch(request: Request, env: DispatchWorkerEnv): Promise<Response> {
    return handleDispatchFetch(request, env);
  },
};

export default dispatchWorker;
