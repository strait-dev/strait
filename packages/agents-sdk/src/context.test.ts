import { afterEach, describe, expect, it, vi } from "vitest";
import { StraitContext } from "./context";
import { StraitAPIError } from "./errors";
import { createPricingCatalog } from "./pricing";

const baseUrl = "https://api.strait.test";
const runId = "run-123";
const runToken = "token-123";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("StraitContext", () => {
  it("reports usage with computed cost and SDK headers", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "usage-1",
          run_id: runId,
          provider: "openai",
          model: "gpt-4.1",
          prompt_tokens: 10,
          completion_tokens: 4,
          total_tokens: 14,
          cost_microusd: 52,
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        }
      )
    );

    const context = new StraitContext({
      baseUrl,
      runId,
      runToken,
      fetch: fetchMock,
      pricingCatalog: createPricingCatalog([
        {
          provider: "openai",
          model: "gpt-4.1",
          inputCostMicrousd: 2,
          outputCostMicrousd: 8,
        },
      ]),
    });

    await context.reportUsage({
      provider: "OpenAI",
      model: "gpt-4.1",
      promptTokens: 10,
      completionTokens: 4,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("https://api.strait.test/sdk/v1/runs/run-123/usage");
    expect(init?.method).toBe("POST");

    const headers = new Headers(init?.headers);
    expect(headers.get("authorization")).toBe(`Bearer ${runToken}`);
    expect(headers.get("x-sdk-version")).toBe("2.0.0");

    expect(JSON.parse(String(init?.body))).toEqual({
      provider: "openai",
      model: "gpt-4.1",
      prompt_tokens: 10,
      completion_tokens: 4,
      total_tokens: 14,
      cost_microusd: 52,
    });
  });

  it("retries retryable telemetry calls when configured", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ title: "temporary outage" }), {
          status: 503,
          headers: {
            "content-type": "application/json",
          },
        })
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "checkpoint-1",
            run_id: runId,
            source: "sdk",
            state: { phase: "planning" },
          }),
          {
            status: 200,
            headers: {
              "content-type": "application/json",
            },
          }
        )
      );

    const context = new StraitContext({
      baseUrl,
      runId,
      runToken,
      fetch: fetchMock,
      retry: {
        maxAttempts: 2,
        baseDelayMs: 0,
        maxDelayMs: 0,
      },
    });

    await context.checkpoint({ phase: "planning" });

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry terminal completion calls", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ detail: "conflict" }), {
        status: 409,
        headers: {
          "content-type": "application/json",
        },
      })
    );

    const context = new StraitContext({
      baseUrl,
      runId,
      runToken,
      fetch: fetchMock,
      retry: {
        maxAttempts: 3,
        baseDelayMs: 0,
        maxDelayMs: 0,
      },
    });

    await expect(context.complete({ ok: true })).rejects.toBeInstanceOf(
      StraitAPIError
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("uses workflow-scoped state endpoints for shared orchestration state", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          run_id: "wf-run-1",
          state_key: "shared-plan",
          value: { phase: "research" },
          updated_at: "2026-03-28T09:00:00Z",
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        }
      )
    );

    const context = new StraitContext({
      baseUrl,
      runId,
      runToken,
      fetch: fetchMock,
    });

    await context.workflow.state.set("shared-plan", { phase: "research" });

    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe(
      "https://api.strait.test/sdk/v1/runs/run-123/workflow-state"
    );
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      key: "shared-plan",
      value: {
        phase: "research",
      },
    });
  });
});
