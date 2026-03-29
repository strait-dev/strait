import { describe, expect, it, vi } from "vitest";

import {
  buildDynamicWorkerDefinition,
  type DynamicWorkerLoader,
  executeSandboxFetch,
  parseSandboxPolicy,
  sandboxTesting,
} from "./sandbox";

describe("sandbox execution", () => {
  it("parses invalid sandbox payloads as an empty policy", () => {
    expect(parseSandboxPolicy("{bad-json")).toEqual({});
    expect(parseSandboxPolicy(null)).toEqual({});
  });

  it("uses the dynamic worker loader by default", async () => {
    const loader: DynamicWorkerLoader = {
      get: (workerID, factory) => {
        expect(workerID).toContain("dynamic_worker");
        const definition = factory(workerID);
        expect(definition.compatibilityDate).toBe("2026-03-29");
        expect(definition.mainModule).toContain("STRAIT_SANDBOX_POLICY");

        return {
          fetch: async () =>
            new Response(
              JSON.stringify({
                body_preview: "ok",
                outbound_reason: null,
                policy_tag: "research",
                status_code: 200,
                url: "https://api.openai.com/v1/responses",
              }),
              {
                headers: {
                  "content-type": "application/json; charset=utf-8",
                },
                status: 200,
              }
            ),
        };
      },
    };

    const outcome = await executeSandboxFetch({
      compatibilityDate: "2026-03-29",
      fetch: vi.fn() as typeof fetch,
      loader,
      policy: {
        allow_hosts: ["api.openai.com"],
        default_action: "deny",
        mode: "dynamic_worker",
        network_class: "sandbox",
        policy_tag: "research",
      },
      url: "https://api.openai.com/v1/responses",
    });

    expect(outcome).toMatchObject({
      bodyPreview: "ok",
      executor: "dynamic_worker",
      policyTag: "research",
      status: "completed",
      statusCode: 200,
    });
  });

  it("falls back to inline evaluation when no dynamic worker loader is present", async () => {
    const fetchSpy = vi.fn(async () => new Response("ok", { status: 200 }));

    const outcome = await executeSandboxFetch({
      fetch: fetchSpy as typeof fetch,
      policy: {
        allow_hosts: ["api.openai.com"],
        default_action: "deny",
        mode: "dynamic_worker",
      },
      url: "https://api.openai.com/v1/responses",
    });

    expect(outcome.executor).toBe("dynamic_worker_fallback");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("preserves outbound worker mode as a compatibility path", async () => {
    const outcome = await executeSandboxFetch({
      fetch: vi.fn(
        async () =>
          new Response('{"error":"blocked"}', {
            headers: {
              "x-strait-outbound-policy-tag": "legacy",
              "x-strait-outbound-reason": "host_not_allowlisted",
              "x-strait-outbound-status": "blocked",
            },
            status: 403,
          })
      ) as typeof fetch,
      policy: {
        default_action: "deny",
        mode: "outbound_worker",
        policy_tag: "legacy",
      },
      url: "https://blocked.example.com",
    });

    expect(outcome).toMatchObject({
      executor: "outbound_worker",
      outboundReason: "host_not_allowlisted",
      policyTag: "legacy",
      status: "blocked",
      statusCode: 403,
    });
  });

  it("builds a stable dynamic worker definition and cache key", () => {
    const key = sandboxTesting.buildDynamicWorkerID(
      "https://api.openai.com/v1/responses",
      {
        allow_hosts: ["api.openai.com"],
        default_action: "deny",
        mode: "dynamic_worker",
        network_class: "sandbox",
        policy_tag: "research",
      }
    );
    expect(key).toContain("dynamic_worker");

    const definition = buildDynamicWorkerDefinition(
      {
        allow_hosts: ["api.openai.com"],
        default_action: "deny",
        mode: "dynamic_worker",
      },
      "2026-03-29"
    );

    expect(definition.env?.STRAIT_SANDBOX_POLICY).toContain(
      '"mode":"dynamic_worker"'
    );
    expect(definition.mainModule).toContain("host_not_allowlisted");
  });
});
