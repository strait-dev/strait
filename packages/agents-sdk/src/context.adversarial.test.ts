import { describe, expect, it, vi } from "vitest";
import { StraitContext } from "./context";
import { BudgetExceededError, StraitSDKError } from "./errors";

const missingAPIURLMessage = "STRAIT_API_URL is required";

describe("StraitContext adversarial cases", () => {
  it("fails fast when required environment variables are missing", () => {
    expect(() => StraitContext.fromEnv({})).toThrow(missingAPIURLMessage);
  });

  it("rejects malformed usage payloads before any network call", () => {
    const fetchMock = vi.fn<typeof fetch>();
    const context = new StraitContext({
      baseUrl: "https://api.strait.test",
      runId: "run-1",
      runToken: "token-1",
      fetch: fetchMock,
    });

    expect(() =>
      context.reportUsage({
        provider: "openai",
        model: "gpt-4.1",
        promptTokens: -1,
        completionTokens: 1,
      })
    ).toThrow(StraitSDKError);

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("enforces tool call budgets locally before posting", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ id: "tool-1" }), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      })
    );

    const context = new StraitContext({
      baseUrl: "https://api.strait.test",
      runId: "run-1",
      runToken: "token-1",
      fetch: fetchMock,
      budget: {
        maxToolCalls: 1,
      },
    });

    await context.reportToolCall({
      toolName: "search",
      status: "completed",
    });

    expect(() =>
      context.reportToolCall({
        toolName: "search",
        status: "completed",
      })
    ).toThrow(BudgetExceededError);

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
