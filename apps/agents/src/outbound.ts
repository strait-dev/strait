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

type OutboundDecision =
  | { allow: true; host: string; policyTag: string }
  | { allow: false; host: string; policyTag: string; reason: string };

const ipv4HostPattern = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

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

function parseSandboxPolicy(
  value: OutboundWorkerEnv["sandbox_policy"]
): CloudflareSandboxPolicy {
  if (!value) {
    return {};
  }
  if (typeof value === "string") {
    try {
      return JSON.parse(value) as CloudflareSandboxPolicy;
    } catch {
      return {};
    }
  }
  return value;
}

function isPrivateIPAddress(hostname: string): boolean {
  if (hostname === "localhost" || hostname.endsWith(".localhost")) {
    return true;
  }

  const ipv4 = hostname.match(ipv4HostPattern);
  if (ipv4) {
    const octets = ipv4.slice(1).map((part) => Number(part));
    const a = octets[0] ?? 0;
    const b = octets[1] ?? 0;
    return (
      a === 10 ||
      a === 127 ||
      (a === 169 && b === 254) ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && b === 168)
    );
  }

  const normalized = hostname.toLowerCase();
  return (
    normalized === "::1" ||
    normalized.startsWith("fc") ||
    normalized.startsWith("fd") ||
    normalized.startsWith("fe80:")
  );
}

function normalizeAllowHosts(policy: CloudflareSandboxPolicy): Set<string> {
  return new Set(
    (policy.allow_hosts ?? [])
      .map((host) => host.trim().toLowerCase())
      .filter((host) => host.length > 0)
  );
}

function evaluateOutboundRequest(
  request: Request,
  policy: CloudflareSandboxPolicy
): OutboundDecision {
  let parsedURL: URL;
  try {
    parsedURL = new URL(request.url);
  } catch {
    return {
      allow: false,
      host: "invalid",
      policyTag: policy.policy_tag ?? "default",
      reason: "invalid_url",
    };
  }

  if (parsedURL.protocol !== "http:" && parsedURL.protocol !== "https:") {
    return {
      allow: false,
      host: parsedURL.hostname,
      policyTag: policy.policy_tag ?? "default",
      reason: "protocol_not_allowed",
    };
  }

  if (isPrivateIPAddress(parsedURL.hostname)) {
    return {
      allow: false,
      host: parsedURL.hostname,
      policyTag: policy.policy_tag ?? "default",
      reason: "private_network_blocked",
    };
  }

  const host = parsedURL.hostname.toLowerCase();
  const allowHosts = normalizeAllowHosts(policy);
  if (allowHosts.has(host)) {
    return { allow: true, host, policyTag: policy.policy_tag ?? "default" };
  }

  if ((policy.default_action ?? "deny") === "deny") {
    return {
      allow: false,
      host,
      policyTag: policy.policy_tag ?? "default",
      reason: "host_not_allowlisted",
    };
  }

  return { allow: true, host, policyTag: policy.policy_tag ?? "default" };
}

export async function handleOutboundFetch(
  request: Request,
  env: OutboundWorkerEnv,
  deps: OutboundWorkerDeps = defaultDeps
): Promise<Response> {
  const policy = parseSandboxPolicy(env.sandbox_policy);
  const decision = evaluateOutboundRequest(request, policy);

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
        "x-strait-outbound-host": decision.host,
        "x-strait-outbound-policy-tag": decision.policyTag,
        "x-strait-outbound-reason": decision.reason,
        "x-strait-outbound-status": "blocked",
      }
    );
  }

  const response = await deps.fetch(request);
  const headers = new Headers(response.headers);
  headers.set("x-strait-outbound-host", decision.host);
  headers.set("x-strait-outbound-policy-tag", decision.policyTag);
  headers.set("x-strait-outbound-status", "allowed");

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
