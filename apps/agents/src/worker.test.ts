import { Effect } from "effect";
import { describe, expect, it, vi } from "vitest";

import {
  buildNDJSONResponseBody,
  buildRuntimeOutput,
  parseEnvelope,
} from "./core";
import type { DynamicWorkerLoader } from "./sandbox";
import type { DispatchEnvelope } from "./types";
import { handleWorkerFetch, verifyRuntimeWorkerAuth } from "./worker";

const missingAuthorizationError = /authorization bearer token is required/i;

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
    run_token: "token-1",
  },
};

describe("worker runtime entrypoint", () => {
  it("requires a valid bearer token", async () => {
    await expect(
      Effect.runPromise(
        verifyRuntimeWorkerAuth(
          new Request("https://worker.local/dispatch", {
            method: "POST",
          }),
          { AGENT_RUNTIME_AUTH_TOKEN: "secret-1" }
        )
      )
    ).rejects.toThrow(missingAuthorizationError);
  });

  it("returns ndjson output matching the shared runtime core", async () => {
    const payload = JSON.stringify(baseEnvelope);
    const request = new Request("https://worker.local/dispatch", {
      method: "POST",
      headers: {
        authorization: "Bearer secret-1",
        "content-type": "application/json",
      },
      body: payload,
    });

    const response = await handleWorkerFetch(request, {
      AGENT_RUNTIME_AUTH_TOKEN: "secret-1",
    });

    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain(
      "application/x-ndjson"
    );

    const expected = buildNDJSONResponseBody(
      await Effect.runPromise(buildRuntimeOutput(baseEnvelope))
    );
    expect(await response.text()).toBe(expected);
  });

  it("rejects malformed dispatch payloads", async () => {
    const response = await handleWorkerFetch(
      new Request("https://worker.local/dispatch", {
        method: "POST",
        headers: {
          authorization: "Bearer secret-1",
          "content-type": "application/json",
        },
        body: `{"version":"v1"}`,
      }),
      { AGENT_RUNTIME_AUTH_TOKEN: "secret-1" },
      {
        fetch: vi.fn(
          async () =>
            new Response('{"error":"blocked"}', {
              headers: {
                "x-strait-outbound-reason": "host_not_allowlisted",
                "x-strait-outbound-status": "blocked",
              },
              status: 403,
            })
        ) as typeof fetch,
      }
    );

    expect(response.status).toBe(400);
    expect(await response.json()).toMatchObject({
      error: "runtime_worker_error",
    });
  });

  it("preserves scenario behavior through the worker handler", async () => {
    const response = await handleWorkerFetch(
      new Request("https://worker.local/dispatch", {
        method: "POST",
        headers: {
          authorization: "Bearer secret-1",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          ...baseEnvelope,
          payload: {
            _scenario: "invalid_json",
          },
        } satisfies DispatchEnvelope),
      }),
      { AGENT_RUNTIME_AUTH_TOKEN: "secret-1" }
    );

    expect(response.status).toBe(200);
    expect(await response.text()).toBe("{not-json}\n");
  });

  it("matches parse/build parity for worker requests", async () => {
    const parsed = await Effect.runPromise(
      parseEnvelope(JSON.stringify(baseEnvelope))
    );
    const output = await Effect.runPromise(buildRuntimeOutput(parsed));

    expect(buildNDJSONResponseBody(output)).toContain(`"type":"complete"`);
  });

  it("preserves dynamic worker sandbox tool-call telemetry", async () => {
    const loader: DynamicWorkerLoader = {
      get: () => ({
        fetch: async () =>
          new Response(
            JSON.stringify({
              body_preview: '{"error":"sandbox_request_blocked"}',
              outbound_reason: "host_not_allowlisted",
              policy_tag: "default",
              status_code: 403,
              url: "https://blocked.example.com",
            }),
            {
              headers: {
                "content-type": "application/json; charset=utf-8",
              },
              status: 403,
            }
          ),
      }),
    };

    const response = await handleWorkerFetch(
      new Request("https://worker.local/dispatch", {
        method: "POST",
        headers: {
          authorization: "Bearer secret-1",
          "content-type": "application/json",
        },
        body: JSON.stringify({
          ...baseEnvelope,
          deployment: {
            ...baseEnvelope.deployment,
            sandbox_policy: {
              default_action: "deny",
              mode: "dynamic_worker",
            },
          },
          payload: {
            _network_url: "https://blocked.example.com",
          },
        } satisfies DispatchEnvelope),
      }),
      {
        AGENT_RUNTIME_AUTH_TOKEN: "secret-1",
        SANDBOX_LOADER: loader,
      }
    );

    expect(response.status).toBe(200);
    const body = await response.text();
    expect(body).toContain(`"tool_name":"sandbox.fetch"`);
    expect(body).toContain(`"sandbox_executor":"dynamic_worker"`);
  });
});
