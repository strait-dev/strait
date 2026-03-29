import {
  evaluateSandboxRequest,
  parseSandboxPolicy,
  sandboxHeaders,
} from "./sandbox";
import type { CloudflareSandboxPolicy } from "./types";

export type OutboundWorkerEnv = {
  agent_id?: string;
  deployment_id?: string;
  run_id?: string;
  sandbox_policy?: CloudflareSandboxPolicy | string;
  script_name?: string;
};

type OutboundWorkerDeps = {
  fetch: typeof fetch;
};

const defaultDeps: OutboundWorkerDeps = {
  fetch: globalThis.fetch.bind(globalThis),
};

function jsonResponse(
  status: number,
  body: Record<string, unknown>,
  headers: HeadersInit = {}
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json; charset=utf-8",
      ...headers,
    },
  });
}

export async function handleOutboundFetch(
  request: Request,
  env: OutboundWorkerEnv,
  deps: OutboundWorkerDeps = defaultDeps
): Promise<Response> {
  const policy = parseSandboxPolicy(env.sandbox_policy);
  const decision = evaluateSandboxRequest(request, policy);

  if (!decision.allow) {
    return jsonResponse(
      403,
      {
        error: "outbound_request_blocked",
        host: decision.host,
        reason: decision.reason,
        run_id: env.run_id ?? null,
      },
      {
        [sandboxHeaders.host]: decision.host,
        [sandboxHeaders.policyTag]: decision.policyTag,
        [sandboxHeaders.reason]: decision.reason,
        [sandboxHeaders.status]: "blocked",
      }
    );
  }

  const response = await deps.fetch(request);
  const headers = new Headers(response.headers);
  headers.set(sandboxHeaders.host, decision.host);
  headers.set(sandboxHeaders.policyTag, decision.policyTag);
  headers.set(sandboxHeaders.status, "allowed");

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

const outboundWorker = {
  fetch(request: Request, env: OutboundWorkerEnv): Promise<Response> {
    return handleOutboundFetch(request, env);
  },
};

export default outboundWorker;
