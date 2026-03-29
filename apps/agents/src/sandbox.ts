import type { CloudflareSandboxPolicy } from "./types";

export type SandboxFetchOutcome = {
  bodyPreview: string;
  executor:
    | "dynamic_worker"
    | "dynamic_worker_fallback"
    | "outbound_worker"
    | "inline";
  host: string;
  outboundReason: string | null;
  policyTag: string;
  status: "blocked" | "completed";
  statusCode: number;
  url: string;
};

export type DynamicWorkerDefinition = {
  compatibilityDate: string;
  env?: Record<string, string>;
  mainModule: string;
};

export type DynamicWorkerHandle = {
  fetch(request: Request): Promise<Response>;
};

export type DynamicWorkerLoader = {
  get(
    workerID: string,
    factory: (workerID: string) => DynamicWorkerDefinition
  ): DynamicWorkerHandle | null;
};

type OutboundDecision =
  | { allow: true; host: string; policyTag: string }
  | { allow: false; host: string; policyTag: string; reason: string };

type SandboxFetchOptions = {
  compatibilityDate?: string;
  fetch: typeof fetch;
  loader?: DynamicWorkerLoader;
  policy: CloudflareSandboxPolicy;
  url: string;
};

const defaultDynamicWorkerCompatibilityDate = "2026-03-29";
const ipv4HostPattern = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

export const sandboxHeaders = Object.freeze({
  host: "x-strait-outbound-host",
  policyTag: "x-strait-outbound-policy-tag",
  reason: "x-strait-outbound-reason",
  status: "x-strait-outbound-status",
});

function buildPolicyTag(policy: CloudflareSandboxPolicy): string {
  return policy.policy_tag?.trim() || "default";
}

export function parseSandboxPolicy(value: unknown): CloudflareSandboxPolicy {
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
  if (typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as CloudflareSandboxPolicy;
}

export function isPrivateIPAddress(hostname: string): boolean {
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

export function evaluateSandboxRequest(
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
      policyTag: buildPolicyTag(policy),
      reason: "invalid_url",
    };
  }

  if (parsedURL.protocol !== "http:" && parsedURL.protocol !== "https:") {
    return {
      allow: false,
      host: parsedURL.hostname,
      policyTag: buildPolicyTag(policy),
      reason: "protocol_not_allowed",
    };
  }

  if (isPrivateIPAddress(parsedURL.hostname)) {
    return {
      allow: false,
      host: parsedURL.hostname,
      policyTag: buildPolicyTag(policy),
      reason: "private_network_blocked",
    };
  }

  const host = parsedURL.hostname.toLowerCase();
  const allowHosts = normalizeAllowHosts(policy);
  if (allowHosts.has(host)) {
    return { allow: true, host, policyTag: buildPolicyTag(policy) };
  }

  if ((policy.default_action ?? "deny") === "deny") {
    return {
      allow: false,
      host,
      policyTag: buildPolicyTag(policy),
      reason: "host_not_allowlisted",
    };
  }

  return { allow: true, host, policyTag: buildPolicyTag(policy) };
}

function buildBlockedOutcome(
  decision: Extract<OutboundDecision, { allow: false }>,
  url: string,
  executor: SandboxFetchOutcome["executor"]
): SandboxFetchOutcome {
  return {
    bodyPreview: '{"error":"sandbox_request_blocked"}',
    executor,
    host: decision.host,
    outboundReason: decision.reason,
    policyTag: decision.policyTag,
    status: "blocked",
    statusCode: 403,
    url,
  };
}

function buildAllowedOutcome(
  url: string,
  response: Response,
  decision: Extract<OutboundDecision, { allow: true }>,
  executor: SandboxFetchOutcome["executor"],
  bodyPreview: string
): SandboxFetchOutcome {
  return {
    bodyPreview,
    executor,
    host: decision.host,
    outboundReason: response.headers.get(sandboxHeaders.reason),
    policyTag:
      response.headers.get(sandboxHeaders.policyTag) ?? decision.policyTag,
    status:
      (response.headers.get(sandboxHeaders.status) ?? "allowed") === "blocked"
        ? "blocked"
        : "completed",
    statusCode: response.status,
    url,
  };
}

async function executeInlineSandboxFetch(
  url: string,
  policy: CloudflareSandboxPolicy,
  fetchFn: typeof fetch
): Promise<SandboxFetchOutcome> {
  const request = new Request(url, { method: "GET" });
  const decision = evaluateSandboxRequest(request, policy);
  if (!decision.allow) {
    return buildBlockedOutcome(decision, url, "dynamic_worker_fallback");
  }

  const response = await fetchFn(request);
  const bodyPreview = (await response.text()).slice(0, 256);
  return buildAllowedOutcome(
    url,
    response,
    decision,
    "dynamic_worker_fallback",
    bodyPreview
  );
}

function buildDynamicWorkerID(
  url: string,
  policy: CloudflareSandboxPolicy
): string {
  const base = [policy.policy_tag ?? "default", policy.network_class ?? "none"];
  const allowHosts = [...normalizeAllowHosts(policy)].sort().join("-");
  const mode = policy.mode ?? "dynamic_worker";
  const host = (() => {
    try {
      return new URL(url).hostname.toLowerCase();
    } catch {
      return "invalid";
    }
  })();
  return [mode, host, base.join("-"), allowHosts || "none"].join(":");
}

export function buildDynamicWorkerDefinition(
  policy: CloudflareSandboxPolicy,
  compatibilityDate = defaultDynamicWorkerCompatibilityDate
): DynamicWorkerDefinition {
  return {
    compatibilityDate,
    env: {
      STRAIT_SANDBOX_POLICY: JSON.stringify(policy),
    },
    mainModule: `function parsePolicy(raw) {
  if (!raw) return {};
  try { return JSON.parse(raw); } catch { return {}; }
}

function isPrivateIPAddress(hostname) {
  if (hostname === "localhost" || hostname.endsWith(".localhost")) return true;
  const ipv4 = hostname.match(/^(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})$/);
  if (ipv4) {
    const octets = ipv4.slice(1).map((part) => Number(part));
    const a = octets[0] ?? 0;
    const b = octets[1] ?? 0;
    return a === 10 || a === 127 || (a === 169 && b === 254) || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168);
  }
  const normalized = hostname.toLowerCase();
  return normalized === "::1" || normalized.startsWith("fc") || normalized.startsWith("fd") || normalized.startsWith("fe80:");
}

function normalizeAllowHosts(policy) {
  return new Set((policy.allow_hosts ?? []).map((host) => host.trim().toLowerCase()).filter((host) => host.length > 0));
}

function policyTag(policy) {
  return policy.policy_tag?.trim() || "default";
}

function evaluate(request, policy) {
  let parsedURL;
  try {
    parsedURL = new URL(request.url);
  } catch {
    return { allow: false, host: "invalid", policyTag: policyTag(policy), reason: "invalid_url" };
  }
  if (parsedURL.protocol !== "http:" && parsedURL.protocol !== "https:") {
    return { allow: false, host: parsedURL.hostname, policyTag: policyTag(policy), reason: "protocol_not_allowed" };
  }
  if (isPrivateIPAddress(parsedURL.hostname)) {
    return { allow: false, host: parsedURL.hostname, policyTag: policyTag(policy), reason: "private_network_blocked" };
  }
  const host = parsedURL.hostname.toLowerCase();
  const allowHosts = normalizeAllowHosts(policy);
  if (allowHosts.has(host)) {
    return { allow: true, host, policyTag: policyTag(policy) };
  }
  if ((policy.default_action ?? "deny") === "deny") {
    return { allow: false, host, policyTag: policyTag(policy), reason: "host_not_allowlisted" };
  }
  return { allow: true, host, policyTag: policyTag(policy) };
}

export default {
  async fetch(request, env) {
    const policy = parsePolicy(env.STRAIT_SANDBOX_POLICY);
    const decision = evaluate(request, policy);
    if (!decision.allow) {
      return new Response(
        JSON.stringify({
          body_preview: '{"error":"sandbox_request_blocked"}',
          outbound_reason: decision.reason,
          policy_tag: decision.policyTag,
          status_code: 403,
          url: request.url
        }),
        {
          status: 403,
          headers: { "content-type": "application/json; charset=utf-8" }
        }
      );
    }
    const upstream = await fetch(request);
    const bodyPreview = (await upstream.text()).slice(0, 256);
    return new Response(
      JSON.stringify({
        body_preview: bodyPreview,
        outbound_reason: null,
        policy_tag: decision.policyTag,
        status_code: upstream.status,
        url: request.url
      }),
      {
        status: upstream.status,
        headers: { "content-type": "application/json; charset=utf-8" }
      }
    );
  }
};`,
  };
}

async function executeDynamicWorkerFetch(
  url: string,
  policy: CloudflareSandboxPolicy,
  loader: DynamicWorkerLoader,
  compatibilityDate?: string
): Promise<SandboxFetchOutcome> {
  const workerID = buildDynamicWorkerID(url, policy);
  const worker = loader.get(workerID, () =>
    buildDynamicWorkerDefinition(
      policy,
      compatibilityDate ?? defaultDynamicWorkerCompatibilityDate
    )
  );
  if (!worker) {
    throw new Error("dynamic worker loader returned no worker");
  }

  const response = await worker.fetch(new Request(url, { method: "GET" }));
  const payload = (await response.json()) as {
    body_preview?: string;
    outbound_reason?: string | null;
    policy_tag?: string;
    status_code?: number;
    url?: string;
  };

  const parsedURL = new URL(url);
  return {
    bodyPreview: payload.body_preview ?? "",
    executor: "dynamic_worker",
    host: parsedURL.hostname.toLowerCase(),
    outboundReason: payload.outbound_reason ?? null,
    policyTag: payload.policy_tag ?? buildPolicyTag(policy),
    status: response.status === 403 ? "blocked" : "completed",
    statusCode: payload.status_code ?? response.status,
    url: payload.url ?? url,
  };
}

async function executeOutboundWorkerFetch(
  url: string,
  policy: CloudflareSandboxPolicy,
  fetchFn: typeof fetch
): Promise<SandboxFetchOutcome> {
  const response = await fetchFn(url, { method: "GET" });
  const bodyPreview = (await response.text()).slice(0, 256);
  const host = (() => {
    try {
      return new URL(url).hostname.toLowerCase();
    } catch {
      return "invalid";
    }
  })();
  return {
    bodyPreview,
    executor: "outbound_worker",
    host,
    outboundReason: response.headers.get(sandboxHeaders.reason),
    policyTag:
      response.headers.get(sandboxHeaders.policyTag) ?? buildPolicyTag(policy),
    status:
      (response.headers.get(sandboxHeaders.status) ?? "allowed") === "blocked"
        ? "blocked"
        : "completed",
    statusCode: response.status,
    url,
  };
}

async function executeInlineFetch(
  url: string,
  fetchFn: typeof fetch
): Promise<SandboxFetchOutcome> {
  const response = await fetchFn(url, { method: "GET" });
  const bodyPreview = (await response.text()).slice(0, 256);
  const host = (() => {
    try {
      return new URL(url).hostname.toLowerCase();
    } catch {
      return "invalid";
    }
  })();
  return {
    bodyPreview,
    executor: "inline",
    host,
    outboundReason: null,
    policyTag: buildPolicyTag({}),
    status: "completed",
    statusCode: response.status,
    url,
  };
}

export function executeSandboxFetch(
  options: SandboxFetchOptions
): Promise<SandboxFetchOutcome> {
  const policy = parseSandboxPolicy(options.policy);
  switch (policy.mode) {
    case "dynamic_worker":
      if (options.loader) {
        return executeDynamicWorkerFetch(
          options.url,
          policy,
          options.loader,
          options.compatibilityDate
        );
      }
      return executeInlineSandboxFetch(options.url, policy, options.fetch);
    case "outbound_worker":
      return executeOutboundWorkerFetch(options.url, policy, options.fetch);
    case "disabled":
    case undefined:
      return executeInlineFetch(options.url, options.fetch);
    default:
      return executeInlineFetch(options.url, options.fetch);
  }
}

export const sandboxTesting = {
  buildDynamicWorkerID,
};
