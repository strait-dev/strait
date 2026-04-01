import { Effect } from "effect";
import { withStrait as withAnthropic } from "./anthropic";
import type { StraitContext } from "./context";
import { runPromise } from "./effects";
import { StraitSDKError } from "./errors";
import { withStrait as withOpenAI } from "./openai";
import type { JsonValue } from "./types";

type ParseableSchema<TResult> = {
  parse: (value: unknown) => TResult;
};

type OpenAIInferOptions<TClient, TResult> = {
  client: TClient;
  model: string;
  provider: "openai";
  request: Record<string, unknown>;
  selectResult?: (result: unknown) => TResult;
  structuredOutput?: ParseableSchema<TResult>;
  streamId?: string;
};

type AnthropicInferOptions<TClient, TResult> = {
  client: TClient;
  model: string;
  provider: "anthropic";
  request: Record<string, unknown>;
  selectResult?: (result: unknown) => TResult;
  structuredOutput?: ParseableSchema<TResult>;
  streamId?: string;
};

type InferOptions<TOpenAIClient, TAnthropicClient, TResult> =
  | OpenAIInferOptions<TOpenAIClient, TResult>
  | AnthropicInferOptions<TAnthropicClient, TResult>;

type OpenAIAgentLoopOptions<TClient> = {
  client: TClient;
  request: Record<string, unknown>;
  provider: "openai";
  streamId?: string;
};

type AnthropicAgentLoopOptions<TClient> = {
  client: TClient;
  request: Record<string, unknown>;
  provider: "anthropic";
  streamId?: string;
};

type AgentLoopOptions<TOpenAIClient, TAnthropicClient> =
  | OpenAIAgentLoopOptions<TOpenAIClient>
  | AnthropicAgentLoopOptions<TAnthropicClient>;

type WrapOperation<TResult> =
  | PromiseLike<TResult>
  | (() => PromiseLike<TResult>);

/** Validates a non-empty name. Throws before entering Effect context. */
function requireName(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function resolveWrapOperation<TResult>(
  operation: WrapOperation<TResult>
): PromiseLike<TResult> {
  return typeof operation === "function"
    ? (operation as () => PromiseLike<TResult>)()
    : operation;
}

function applyStructuredOutput<TResult>(
  result: unknown,
  options: {
    selectResult?: (result: unknown) => TResult;
    structuredOutput?: ParseableSchema<TResult>;
  }
): TResult {
  if (options.selectResult) {
    return options.selectResult(result);
  }

  if (options.structuredOutput) {
    return options.structuredOutput.parse(result);
  }

  return result as TResult;
}

export function createAIStep<
  TOpenAIClient = unknown,
  TAnthropicClient = unknown,
>(context: StraitContext) {
  return {
    wrap<TResult>(
      name: string,
      operation: WrapOperation<TResult>,
      options: {
        checkpointSource?: string;
        metadata?: JsonValue;
      } = {}
    ): Promise<TResult> {
      const stepName = requireName(name, "name");
      return runPromise(
        Effect.gen(function* () {
          const startedAt = Date.now();
          yield* Effect.tryPromise(() =>
            context.checkpoint(
              {
                step: stepName,
                phase: "started",
                metadata: options.metadata ?? null,
              },
              { source: options.checkpointSource ?? "ai-step" }
            )
          );

          const result = yield* Effect.tryPromise(() =>
            Promise.resolve(resolveWrapOperation(operation))
          );

          yield* Effect.tryPromise(() =>
            context.checkpoint(
              {
                step: stepName,
                phase: "completed",
                durationMs: Date.now() - startedAt,
              },
              { source: options.checkpointSource ?? "ai-step" }
            )
          );

          return result;
        })
      );
    },

    infer<TResult = unknown>(
      name: string,
      options: InferOptions<TOpenAIClient, TAnthropicClient, TResult>
    ): Promise<TResult> {
      if (options.provider === "openai") {
        const client = withOpenAI(options.client as any, {
          context,
          streamId: options.streamId,
        });

        return this.wrap(name, () =>
          client.chat.completions
            .create({
              model: options.model,
              ...options.request,
            })
            .then((result: unknown) => applyStructuredOutput(result, options))
        );
      }

      const client = withAnthropic(options.client as any, {
        context,
        streamId: options.streamId,
      });

      return this.wrap(name, () =>
        client.messages
          .create({
            model: options.model,
            ...options.request,
          })
          .then((result: unknown) => applyStructuredOutput(result, options))
      );
    },

    agentLoop(
      name: string,
      options: AgentLoopOptions<TOpenAIClient, TAnthropicClient>
    ): Promise<unknown> {
      if (options.provider === "openai") {
        const client = withOpenAI(options.client as any, {
          context,
          streamId: options.streamId,
        });

        return this.wrap(name, () => {
          const runner = client.chat.completions.runTools(options.request);

          if (
            runner != null &&
            typeof runner === "object" &&
            "finalContent" in runner &&
            typeof runner.finalContent === "function"
          ) {
            return runner.finalContent();
          }

          return runner;
        });
      }

      const client = withAnthropic(options.client as any, {
        context,
        streamId: options.streamId,
      });

      return this.wrap(name, () => {
        const runner = client.beta?.messages?.toolRunner?.(options.request);
        if (runner == null) {
          throw new StraitSDKError(
            "anthropic client does not support beta.messages.toolRunner"
          );
        }

        if (
          typeof runner === "object" &&
          "finalMessage" in runner &&
          typeof runner.finalMessage === "function"
        ) {
          return runner.finalMessage();
        }

        return runner;
      });
    },
  };
}
