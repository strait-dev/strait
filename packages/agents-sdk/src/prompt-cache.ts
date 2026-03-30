/**
 * Prompt caching utilities for reducing token costs on repeated system prompts.
 *
 * Supports Anthropic's cache_control mechanism and OpenAI's equivalent when
 * available. The middleware is provider-agnostic and only annotates messages
 * that the provider will understand.
 */

export type PromptCacheType = "ephemeral";

export interface PromptCacheOptions {
  /** Also cache the last user message (useful for multi-turn). */
  cacheLastUserMessage?: boolean;
  /** Enable or disable prompt caching. */
  enabled: boolean;
  /** Cache type applied to the system prompt. Defaults to "ephemeral". */
  systemPromptCacheType?: PromptCacheType;
}

type MessageLike = Record<string, unknown>;

/**
 * Annotates provider options on messages so that compatible providers
 * (Anthropic, OpenAI) can cache the system prompt across requests.
 *
 * This function is pure -- it returns a shallow copy of params with
 * modified messages rather than mutating the input.
 */
export function applyPromptCaching(
  params: Record<string, unknown>,
  options: PromptCacheOptions
): Record<string, unknown> {
  if (!options.enabled) {
    return params;
  }

  const messages = params.messages as MessageLike[] | undefined;
  if (!Array.isArray(messages) || messages.length === 0) {
    return params;
  }

  const cacheType = options.systemPromptCacheType ?? "ephemeral";
  const updated = messages.map((msg) => ({ ...msg }));
  let changed = false;

  // Annotate system message with cache_control for Anthropic.
  const systemIdx = updated.findIndex((m) => m.role === "system");
  const systemMsg = systemIdx === -1 ? undefined : updated[systemIdx];
  if (systemMsg) {
    updated[systemIdx] = {
      ...systemMsg,
      providerOptions: {
        ...(systemMsg.providerOptions as Record<string, unknown>),
        anthropic: {
          cacheControl: { type: cacheType },
        },
      },
    };
    changed = true;
  }

  // Optionally cache the last user message for multi-turn conversations.
  if (options.cacheLastUserMessage) {
    for (let i = updated.length - 1; i >= 0; i--) {
      const msg = updated[i];
      if (msg?.role === "user") {
        updated[i] = {
          ...msg,
          providerOptions: {
            ...(msg.providerOptions as Record<string, unknown>),
            anthropic: {
              cacheControl: { type: cacheType },
            },
          },
        };
        changed = true;
        break;
      }
    }
  }

  if (!changed) {
    return params;
  }

  return { ...params, messages: updated };
}
