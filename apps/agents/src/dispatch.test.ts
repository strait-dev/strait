import { describe, expect, it, vi } from "vitest";
import { handleDispatchFetch } from "./dispatch";
import type { DispatchEnvelope } from "./types";

const baseEnvelope: DispatchEnvelope = {
  version: "v1",
  run: {
    id: "run-1",
    project_id: "proj-1",
    attempt: 1,
    timeout_secs: 30,
  },
  agent: {
    id: "agent-1",
    slug: "support-agent",
    model: "gpt-5.4",
  },
  deployment: {
    id: "deployment-1",
    version: 1,
    provider: "cloudflare",
  },
  callback: {
    base_url: "https://api.strait.local",
    run_id: "run-1",
    run_token: "run-token-1",
  },
};

function buildRequest(body: unknown, token = "internal-secret"): Request {
  return new Request("https://dispatch.local/run", {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    body: JSON.stringify(body),
  });
}

describe("dispatch worker", () => {
  it("rejects unauthorized requests", async () => {
    const response = await handleDispatchFetch(
      buildRequest(
        {
          deployment_id: "dep-1",
          envelope: baseEnvelope,
          namespace: "ns-prod",
          provider: "cloudflare",
          run_id: "run-1",
          script_name: "agent-script",
        },
        "wrong-secret"
      ),
      {
        DISPATCHER: { get: () => null },
        INTERNAL_SECRET: "internal-secret",
      }
    );

    expect(response.status).toBe(401);
  });

  it("routes to the runtime worker and forwards callbacks", async () => {
    const callbackFetch = vi.fn(
      async () => new Response(null, { status: 201 })
    );
    const runtimeFetch = vi.fn(
      async () =>
        new Response(
          [
            JSON.stringify({
              type: "checkpoint",
              state: { phase: "planning" },
            }),
            JSON.stringify({
              completion_tokens: 8,
              cost_microusd: 200,
              model: "gpt-5.4",
              prompt_tokens: 12,
              provider: "local",
              type: "usage",
            }),
            JSON.stringify({
              input: { topic: "runtime failures" },
              output: { echoed: "runtime failures" },
              status: "completed",
              tool_name: "local.echo",
              type: "tool_call",
            }),
            JSON.stringify({
              chunk: "done",
              done: true,
              stream_id: "default",
              type: "stream",
            }),
            JSON.stringify({
              result: { ok: true },
              type: "complete",
            }),
          ].join("\n"),
          {
            headers: {
              "content-type": "application/x-ndjson",
            },
            status: 200,
          }
        )
    );

    const response = await handleDispatchFetch(
      buildRequest({
        deployment_id: "dep-1",
        envelope: baseEnvelope,
        namespace: "ns-prod",
        provider: "cloudflare",
        run_id: "run-1",
        sandbox_policy: {
          allow_hosts: ["api.openai.com"],
          default_action: "deny",
          mode: "outbound_worker",
        },
        script_name: "agent-script",
      }),
      {
        AGENT_RUNTIME_AUTH_TOKEN: "runtime-secret",
        DISPATCHER: {
          get: (scriptName, _, advanced) => {
            expect(advanced?.outbound?.run_id).toBe("run-1");
            expect(advanced?.outbound?.sandbox_policy?.mode).toBe(
              "outbound_worker"
            );
            return scriptName === "agent-script"
              ? { fetch: runtimeFetch }
              : null;
          },
        },
        INTERNAL_SECRET: "internal-secret",
      },
      { fetch: callbackFetch as typeof fetch }
    );

    expect(response.status).toBe(202);
    expect(runtimeFetch).toHaveBeenCalledTimes(1);
    expect(callbackFetch).toHaveBeenCalledTimes(5);
  });

  it("does not attach outbound routing hints for dynamic worker mode", async () => {
    const runtimeFetch = vi.fn(
      async () =>
        new Response(
          JSON.stringify({ type: "complete", result: { ok: true } }),
          {
            headers: {
              "content-type": "application/x-ndjson",
            },
            status: 200,
          }
        )
    );
    const callbackFetch = vi.fn(
      async () => new Response(null, { status: 201 })
    );

    const response = await handleDispatchFetch(
      buildRequest({
        deployment_id: "dep-1",
        envelope: baseEnvelope,
        namespace: "ns-prod",
        provider: "cloudflare",
        run_id: "run-1",
        sandbox_policy: {
          default_action: "deny",
          mode: "dynamic_worker",
        },
        script_name: "agent-script",
      }),
      {
        DISPATCHER: {
          get: (scriptName, _, advanced) => {
            expect(scriptName).toBe("agent-script");
            expect(advanced).toBeUndefined();
            return { fetch: runtimeFetch };
          },
        },
        INTERNAL_SECRET: "internal-secret",
      },
      { fetch: callbackFetch as typeof fetch }
    );

    expect(response.status).toBe(202);
    expect(runtimeFetch).toHaveBeenCalledTimes(1);
    expect(callbackFetch).toHaveBeenCalledTimes(1);
  });

  it("returns 404 for missing scripts", async () => {
    const response = await handleDispatchFetch(
      buildRequest({
        deployment_id: "dep-1",
        envelope: baseEnvelope,
        namespace: "ns-prod",
        provider: "cloudflare",
        run_id: "run-1",
        script_name: "missing-script",
      }),
      {
        DISPATCHER: { get: () => null },
        INTERNAL_SECRET: "internal-secret",
      }
    );

    expect(response.status).toBe(404);
  });

  it("maps runtime worker failures to a dispatch error", async () => {
    const response = await handleDispatchFetch(
      buildRequest({
        deployment_id: "dep-1",
        envelope: baseEnvelope,
        namespace: "ns-prod",
        provider: "cloudflare",
        run_id: "run-1",
        script_name: "agent-script",
      }),
      {
        DISPATCHER: {
          get: () => ({
            fetch: async () => new Response("boom", { status: 500 }),
          }),
        },
        INTERNAL_SECRET: "internal-secret",
      }
    );

    expect(response.status).toBe(502);
  });
});
