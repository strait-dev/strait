import { describe, expect, it } from "vitest";
import { applyPromptCaching, type PromptCacheOptions } from "./prompt-cache";

function getMessages(
  result: Record<string, unknown>
): Record<string, unknown>[] {
  return result.messages as Record<string, unknown>[];
}

function getProviderOpts(
  msg: Record<string, unknown>
): Record<string, unknown> {
  return msg.providerOptions as Record<string, unknown>;
}

describe("applyPromptCaching", () => {
  const disabled: PromptCacheOptions = { enabled: false };
  const enabled: PromptCacheOptions = { enabled: true };
  const enabledWithLastUser: PromptCacheOptions = {
    enabled: true,
    cacheLastUserMessage: true,
  };

  it("returns params unchanged when disabled", () => {
    const params = {
      messages: [
        { role: "system", content: "You are a helper." },
        { role: "user", content: "Hello" },
      ],
    };
    const result = applyPromptCaching(params, disabled);
    expect(result).toBe(params);
  });

  it("returns params unchanged when messages is missing", () => {
    const params = { temperature: 0.5 };
    const result = applyPromptCaching(params, enabled);
    expect(result).toBe(params);
  });

  it("returns params unchanged when messages is empty", () => {
    const params = { messages: [] };
    const result = applyPromptCaching(params, enabled);
    expect(result).toBe(params);
  });

  it("adds cache_control to system message", () => {
    const params = {
      messages: [
        { role: "system", content: "You are a helper." },
        { role: "user", content: "Hello" },
      ],
    };
    const result = applyPromptCaching(params, enabled);

    expect(result).not.toBe(params);
    const m = getMessages(result);
    expect(getProviderOpts(m[0] as Record<string, unknown>)).toEqual({
      anthropic: {
        cacheControl: { type: "ephemeral" },
      },
    });
    // User message should not be modified.
    expect((m[1] as Record<string, unknown>).providerOptions).toBeUndefined();
  });

  it("does not mutate the original params", () => {
    const original = {
      messages: [
        { role: "system", content: "System prompt." },
        { role: "user", content: "Query" },
      ],
    };
    const originalSystemMsg = original.messages[0];
    applyPromptCaching(original, enabled);
    expect(originalSystemMsg).not.toHaveProperty("providerOptions");
  });

  it("preserves existing providerOptions on system message", () => {
    const params = {
      messages: [
        {
          role: "system",
          content: "System prompt.",
          providerOptions: { openai: { logprobs: true } },
        },
      ],
    };
    const result = applyPromptCaching(params, enabled);
    const m = getMessages(result);
    const opts = getProviderOpts(m[0] as Record<string, unknown>);
    expect(opts.openai).toEqual({ logprobs: true });
    expect(opts.anthropic).toEqual({
      cacheControl: { type: "ephemeral" },
    });
  });

  it("returns params unchanged when no system message exists", () => {
    const params = {
      messages: [
        { role: "user", content: "Hello" },
        { role: "assistant", content: "Hi" },
      ],
    };
    const result = applyPromptCaching(params, enabled);
    expect(result).toBe(params);
  });

  it("caches last user message when cacheLastUserMessage is true", () => {
    const params = {
      messages: [
        { role: "system", content: "System" },
        { role: "user", content: "First" },
        { role: "assistant", content: "Reply" },
        { role: "user", content: "Second" },
      ],
    };
    const result = applyPromptCaching(params, enabledWithLastUser);
    const m = getMessages(result);

    // System message should have cache_control.
    expect(getProviderOpts(m[0] as Record<string, unknown>).anthropic).toEqual({
      cacheControl: { type: "ephemeral" },
    });

    // Only the last user message should have cache_control.
    expect((m[1] as Record<string, unknown>).providerOptions).toBeUndefined();
    expect(getProviderOpts(m[3] as Record<string, unknown>).anthropic).toEqual({
      cacheControl: { type: "ephemeral" },
    });
  });

  it("respects custom systemPromptCacheType", () => {
    const params = {
      messages: [{ role: "system", content: "Prompt" }],
    };
    const result = applyPromptCaching(params, {
      enabled: true,
      systemPromptCacheType: "ephemeral",
    });
    const m = getMessages(result);
    expect(getProviderOpts(m[0] as Record<string, unknown>).anthropic).toEqual({
      cacheControl: { type: "ephemeral" },
    });
  });

  it("handles non-array messages gracefully", () => {
    const params = { messages: "not-an-array" };
    const result = applyPromptCaching(params, enabled);
    expect(result).toBe(params);
  });
});
