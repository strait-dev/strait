import { describe, expect, it, vi } from "vitest";

import { handleOutboundFetch } from "./outbound";

describe("outbound worker", () => {
  it("blocks private-network targets", async () => {
    const response = await handleOutboundFetch(
      new Request("http://127.0.0.1:8080/health"),
      {
        sandbox_policy: {
          default_action: "allow",
          mode: "outbound_worker",
        },
      },
      { fetch: vi.fn() as typeof fetch }
    );

    expect(response.status).toBe(403);
    expect(response.headers.get("x-strait-outbound-reason")).toBe(
      "private_network_blocked"
    );
  });

  it("blocks hosts outside the allowlist", async () => {
    const response = await handleOutboundFetch(
      new Request("https://blocked.example.com"),
      {
        sandbox_policy: {
          allow_hosts: ["api.openai.com"],
          default_action: "deny",
          mode: "outbound_worker",
          policy_tag: "llm-egress",
        },
      },
      { fetch: vi.fn() as typeof fetch }
    );

    expect(response.status).toBe(403);
    expect(response.headers.get("x-strait-outbound-policy-tag")).toBe(
      "llm-egress"
    );
  });

  it("proxies allowlisted requests", async () => {
    const upstreamFetch = vi.fn(
      async () => new Response("ok", { status: 200 })
    );

    const response = await handleOutboundFetch(
      new Request("https://api.openai.com/v1/responses"),
      {
        run_id: "run-1",
        sandbox_policy: {
          allow_hosts: ["api.openai.com"],
          default_action: "deny",
          mode: "outbound_worker",
        },
      },
      { fetch: upstreamFetch as typeof fetch }
    );

    expect(response.status).toBe(200);
    expect(await response.text()).toBe("ok");
    expect(response.headers.get("x-strait-outbound-status")).toBe("allowed");
    expect(upstreamFetch).toHaveBeenCalledTimes(1);
  });

  it("blocks non-http protocols", async () => {
    const response = await handleOutboundFetch(
      new Request("ftp://files.example.com"),
      {
        sandbox_policy: {
          default_action: "allow",
          mode: "outbound_worker",
        },
      },
      { fetch: vi.fn() as typeof fetch }
    );

    expect(response.status).toBe(403);
    expect(response.headers.get("x-strait-outbound-reason")).toBe(
      "protocol_not_allowed"
    );
  });

  it("keeps allowlisted hosts stable under casing and whitespace variants", async () => {
    for (const hostname of [
      "api.openai.com",
      "example.org",
      "subdomain.service.dev",
    ]) {
      const fetchSpy = vi.fn(async () => new Response("ok", { status: 200 }));
      const response = await handleOutboundFetch(
        new Request(`https://${hostname}/`),
        {
          sandbox_policy: {
            allow_hosts: [` ${hostname.toUpperCase()} `],
            default_action: "deny",
            mode: "outbound_worker",
          },
        },
        { fetch: fetchSpy as typeof fetch }
      );
      expect(response.status).toBe(200);
      expect(await response.text()).toBe("ok");
    }
  });
});
