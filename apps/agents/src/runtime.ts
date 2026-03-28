import { Duration, Effect } from "effect";

import {
  asRecord,
  type DispatchEnvelope,
  type RuntimeOutputLine,
  runtimeContractVersion,
} from "./types";

function toError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}

function assertEnvelope(value: unknown): DispatchEnvelope {
  if (!value || typeof value !== "object") {
    throw new Error("dispatch envelope must be an object");
  }

  const envelope = value as Partial<DispatchEnvelope>;
  if (typeof envelope.version !== "string" || envelope.version.length === 0) {
    throw new Error("dispatch envelope version is required");
  }
  if (
    !envelope.run ||
    typeof envelope.run.id !== "string" ||
    typeof envelope.run.project_id !== "string"
  ) {
    throw new Error("dispatch envelope run is invalid");
  }
  if (
    !envelope.agent ||
    typeof envelope.agent.id !== "string" ||
    typeof envelope.agent.slug !== "string" ||
    typeof envelope.agent.model !== "string"
  ) {
    throw new Error("dispatch envelope agent is invalid");
  }
  if (
    !envelope.deployment ||
    typeof envelope.deployment.id !== "string" ||
    typeof envelope.deployment.provider !== "string"
  ) {
    throw new Error("dispatch envelope deployment is invalid");
  }
  if (
    !envelope.callback ||
    typeof envelope.callback.base_url !== "string" ||
    typeof envelope.callback.run_id !== "string" ||
    typeof envelope.callback.run_token !== "string"
  ) {
    throw new Error("dispatch envelope callback is invalid");
  }

  return envelope as DispatchEnvelope;
}

export function parseEnvelope(
  raw: string
): Effect.Effect<DispatchEnvelope, Error> {
  return Effect.try({
    try: () => assertEnvelope(JSON.parse(raw)),
    catch: toError,
  });
}

export function serializeOutputLine(output: RuntimeOutputLine): string {
  return output.kind === "raw" ? output.line : JSON.stringify(output.event);
}

export function buildRuntimeOutput(
  envelope: DispatchEnvelope
): Effect.Effect<readonly RuntimeOutputLine[]> {
  const payload = asRecord(envelope.payload);
  const config = asRecord(envelope.agent.config);

  const scenario = String(payload._scenario ?? config.scenario ?? "success");
  const delayMs = Number(payload._delay_ms ?? config.delay_ms ?? 0);

  return Effect.gen(function* () {
    if (delayMs > 0) {
      yield* Effect.sleep(Duration.millis(delayMs));
    }

    if (scenario === "invalid_json") {
      return [
        {
          kind: "raw",
          line: "{not-json}",
        },
      ] as const;
    }

    const outputs: RuntimeOutputLine[] = [
      {
        kind: "event",
        event: {
          type: "stream",
          stream_id: "default",
          chunk: `agent:${envelope.agent.slug}:thinking `,
        },
      },
      {
        kind: "event",
        event: {
          type: "checkpoint",
          state: {
            phase: "planning",
            agent_id: envelope.agent.id,
            deployment_id: envelope.deployment.id,
          },
        },
      },
    ];

    if (scenario === "duplicate_checkpoint") {
      outputs.push({
        kind: "event",
        event: {
          type: "checkpoint",
          state: {
            phase: "planning",
            duplicate: true,
          },
        },
      });
    }

    outputs.push(
      {
        kind: "event",
        event: {
          type: "tool_call",
          tool_name: "local.echo",
          input: {
            payload,
          },
          output: {
            echoed: payload.prompt ?? payload.input ?? "ok",
          },
          duration_ms: 5,
          status: "completed",
        },
      },
      {
        kind: "event",
        event: {
          type: "usage",
          provider: "local",
          model: envelope.agent.model || "local-agent",
          prompt_tokens: 12,
          completion_tokens: 8,
          total_tokens: 20,
          cost_microusd: 200,
        },
      },
      {
        kind: "event",
        event: {
          type: "stream",
          stream_id: "default",
          chunk: "done",
          done: true,
        },
      }
    );

    if (scenario === "disconnect") {
      return outputs;
    }

    if (scenario === "fail") {
      outputs.push({
        kind: "event",
        event: {
          type: "fail",
          error: String(payload._error ?? "runtime requested failure"),
        },
      });
      return outputs;
    }

    outputs.push({
      kind: "event",
      event: {
        type: "complete",
        result: {
          ok: true,
          contract_version: runtimeContractVersion,
          agent_id: envelope.agent.id,
          run_id: envelope.run.id,
          payload,
        },
      },
    });

    return outputs;
  });
}
