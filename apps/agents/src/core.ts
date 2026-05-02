import { Duration, Effect } from "effect";
import {
  type DynamicWorkerLoader,
  executeSandboxFetch,
  parseSandboxPolicy,
} from "./sandbox";
import {
  asRecord,
  type DispatchEnvelope,
  type JsonValue,
  type RuntimeOutputLine,
  runtimeContractVersion,
} from "./types";

type RuntimeMode = "dynamic_planner" | "generic" | "synthesizer" | "worker";

type PlannerTask = {
  agentId: string;
  payload?: Record<string, JsonValue>;
  stepRef: string;
};

type JsonObject = { [key: string]: JsonValue };
type RuntimeDependencies = {
  dynamicWorkerCompatibilityDate?: string;
  dynamicWorkerLoader?: DynamicWorkerLoader;
  fetch: typeof fetch;
};

const defaultDependencies: RuntimeDependencies = {
  fetch: globalThis.fetch.bind(globalThis),
};

function resolveParentStepRef(
  payload: Record<string, JsonValue>,
  slug: string
): string {
  if (
    typeof payload.parentStepRef === "string" &&
    payload.parentStepRef.trim().length > 0
  ) {
    return payload.parentStepRef.trim();
  }

  return slug === "planner" ? "planner" : "plan";
}

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

function assertStringArray(
  value: JsonValue | undefined,
  field: string
): string[] | undefined {
  if (value == null) {
    return;
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be an array`);
  }

  return value.map((item, index) => {
    if (typeof item !== "string" || item.trim().length === 0) {
      throw new Error(`${field}[${index}] must be a non-empty string`);
    }
    return item.trim();
  });
}

function resolveRuntimeMode(payload: Record<string, JsonValue>): RuntimeMode {
  const rawMode = payload._mode ?? payload.mode;
  if (rawMode === "dynamic_planner") {
    return "dynamic_planner";
  }
  if (rawMode === "synthesizer") {
    return "synthesizer";
  }
  if (rawMode === "worker") {
    return "worker";
  }
  return "generic";
}

function createWorkerPayload(
  payload: Record<string, JsonValue>,
  index: number
): Record<string, JsonValue> {
  const baseLens = payload.workerLenses;
  const lens =
    Array.isArray(baseLens) && typeof baseLens[index] === "string"
      ? baseLens[index]
      : `track-${index + 1}`;

  return {
    lens,
    topic: payload.topic ?? "unknown-topic",
  };
}

function createPlannerTasks(
  payload: Record<string, JsonValue>
): readonly PlannerTask[] {
  const workerAgentIds = assertStringArray(
    payload.workerAgentIds,
    "workerAgentIds"
  );
  if (workerAgentIds == null || workerAgentIds.length === 0) {
    throw new Error("dynamic_planner mode requires workerAgentIds");
  }

  return workerAgentIds.map((agentId, index) => ({
    agentId,
    payload: createWorkerPayload(payload, index),
    stepRef: `worker-${index + 1}`,
  }));
}

function buildResult(
  envelope: DispatchEnvelope,
  payload: Record<string, JsonValue>
): JsonValue {
  const mode = resolveRuntimeMode(payload);

  if (mode === "dynamic_planner") {
    const tasks = createPlannerTasks(payload);
    const parentStepRef = resolveParentStepRef(payload, envelope.agent.slug);
    const synthesisAgentId =
      typeof payload.synthesizerAgentId === "string" &&
      payload.synthesizerAgentId.trim().length > 0
        ? payload.synthesizerAgentId.trim()
        : "agent-synthesizer";

    return {
      contract_version: runtimeContractVersion,
      dynamic_steps: [
        ...tasks.map((task) => {
          const step: JsonObject = {
            agent_id: task.agentId,
            depends_on: [parentStepRef],
            step_ref: task.stepRef,
          };
          if (task.payload != null) {
            step.payload = task.payload;
          }
          return step;
        }),
        {
          agent_id: synthesisAgentId,
          depends_on: tasks.map((task) => task.stepRef),
          payload: {
            summary_style: payload.summaryStyle ?? "brief",
            topic: payload.topic ?? "unknown-topic",
          },
          step_ref: "synthesis",
        },
      ],
      plan_summary: `planned ${tasks.length} dynamic worker steps`,
      run_id: envelope.run.id,
    };
  }

  if (mode === "worker") {
    return {
      contract_version: runtimeContractVersion,
      finding: `Investigated ${String(payload.topic ?? "unknown-topic")} via ${String(payload.lens ?? "general")}.`,
      run_id: envelope.run.id,
    };
  }

  if (mode === "synthesizer") {
    return {
      contract_version: runtimeContractVersion,
      run_id: envelope.run.id,
      summary: `Prepared a ${String(payload.summaryStyle ?? "brief")} summary for ${String(payload.topic ?? "unknown-topic")}.`,
    };
  }

  return {
    ok: true,
    contract_version: runtimeContractVersion,
    agent_id: envelope.agent.id,
    run_id: envelope.run.id,
    payload,
  };
}

function buildCheckpointState(
  envelope: DispatchEnvelope,
  payload: Record<string, JsonValue>
): Record<string, JsonValue> {
  const mode = resolveRuntimeMode(payload);
  if (mode === "dynamic_planner") {
    return {
      agent_id: envelope.agent.id,
      phase: "planning",
      planned_workers: (assertStringArray(
        payload.workerAgentIds,
        "workerAgentIds"
      )?.length ?? 0) as JsonValue,
    };
  }

  if (mode === "worker") {
    return {
      agent_id: envelope.agent.id,
      lens: payload.lens ?? "general",
      phase: "executing",
    };
  }

  if (mode === "synthesizer") {
    return {
      agent_id: envelope.agent.id,
      phase: "synthesizing",
      topic: payload.topic ?? "unknown-topic",
    };
  }

  return {
    agent_id: envelope.agent.id,
    deployment_id: envelope.deployment.id,
    phase: "planning",
  };
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

/**
 * Returns a cached tool response if one matches by tool_name + input.
 * Used during what-if replay to serve deterministic tool outputs.
 */
export function findCachedToolResponse(
  cached: DispatchEnvelope["cached_tool_calls"],
  toolName: string,
  input: JsonValue
): JsonValue | undefined {
  if (!cached?.length) {
    return;
  }
  const inputStr = JSON.stringify(input);
  for (const entry of cached) {
    if (
      entry.tool_name === toolName &&
      JSON.stringify(entry.input) === inputStr
    ) {
      return entry.output;
    }
  }
  return;
}

/**
 * Builds runtime output entries for cached tool calls (what-if replay).
 */
function buildCachedToolCallOutputs(
  envelope: DispatchEnvelope
): RuntimeOutputLine[] {
  if (!envelope.cached_tool_calls?.length) {
    return [];
  }
  return envelope.cached_tool_calls.map((entry) => ({
    kind: "event" as const,
    event: {
      type: "tool_call" as const,
      tool_name: entry.tool_name,
      input: entry.input ?? {},
      output: entry.output ?? {},
      duration_ms: 0,
      status: "cached",
    },
  }));
}

export function buildRuntimeOutput(
  envelope: DispatchEnvelope,
  deps: RuntimeDependencies = defaultDependencies
): Effect.Effect<readonly RuntimeOutputLine[], Error> {
  const payload = asRecord(envelope.payload);
  const config = asRecord(envelope.agent.config);
  const mergedPayload = {
    ...config,
    ...payload,
  };

  const scenario = String(mergedPayload._scenario ?? "success");
  const delayMs = Number(mergedPayload._delay_ms ?? 0);

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
          state: buildCheckpointState(envelope, mergedPayload),
        },
      },
    ];

    const networkToolCall = yield* buildNetworkToolCall(
      envelope,
      mergedPayload,
      deps
    );
    if (networkToolCall) {
      outputs.push(networkToolCall);
    }

    // Emit discovery events for available peer agents.
    outputs.push(...buildAgentToolRegistrations(envelope));

    if (scenario === "duplicate_checkpoint") {
      outputs.push({
        kind: "event",
        event: {
          type: "checkpoint",
          state: {
            duplicate: true,
            phase: "planning",
          },
        },
      });
    }

    // Serve cached tool responses for what-if replay runs.
    outputs.push(...buildCachedToolCallOutputs(envelope));

    outputs.push(
      {
        kind: "event",
        event: {
          type: "tool_call",
          tool_name: "local.echo",
          input: {
            payload: mergedPayload,
          },
          output: {
            echoed:
              mergedPayload.prompt ??
              mergedPayload.input ??
              mergedPayload.topic ??
              "ok",
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
          error: String(mergedPayload._error ?? "runtime requested failure"),
        },
      });
      return outputs;
    }

    outputs.push({
      kind: "event",
      event: {
        type: "complete",
        result: buildResult(envelope, mergedPayload),
      },
    });

    return outputs;
  }).pipe(Effect.mapError(toError));
}

/**
 * Registers available peer agents as tool definitions in the output stream.
 * Agents are invoked via the SDK invoke-agent callback endpoint;
 * this emits discovery events so the caller knows which agents are available.
 */
function buildAgentToolRegistrations(
  envelope: DispatchEnvelope
): RuntimeOutputLine[] {
  if (!envelope.available_agents?.length) {
    return [];
  }
  return envelope.available_agents.map((agent) => ({
    kind: "event" as const,
    event: {
      type: "tool_call" as const,
      tool_name: `strait.agent.${agent.slug}`,
      input: {
        agent_id: agent.id,
        agent_slug: agent.slug,
        name: agent.name ?? agent.slug,
        description: agent.description ?? "",
      },
      output: { registered: true },
      duration_ms: 0,
      status: "registered",
    },
  }));
}

function buildNetworkToolCall(
  envelope: DispatchEnvelope,
  payload: Record<string, JsonValue>,
  deps: RuntimeDependencies
): Effect.Effect<RuntimeOutputLine | null, Error> {
  const targetURL = payload._network_url;
  if (typeof targetURL !== "string" || targetURL.trim().length === 0) {
    return Effect.succeed(null);
  }

  return Effect.tryPromise({
    try: async () => {
      const policy = parseSandboxPolicy(envelope.deployment.sandbox_policy);
      const outcome = await executeSandboxFetch({
        compatibilityDate: deps.dynamicWorkerCompatibilityDate,
        fetch: deps.fetch,
        loader: deps.dynamicWorkerLoader,
        policy,
        url: targetURL,
      });

      return {
        kind: "event" as const,
        event: {
          type: "tool_call" as const,
          tool_name: "sandbox.fetch",
          input: {
            network_class: policy.network_class ?? null,
            policy_tag: policy.policy_tag ?? null,
            sandbox_mode: policy.mode ?? "disabled",
            url: targetURL,
          },
          output: {
            body_preview: outcome.bodyPreview,
            outbound_reason: outcome.outboundReason,
            sandbox_executor: outcome.executor,
            status_code: outcome.statusCode,
            url: targetURL,
          },
          duration_ms: 5,
          status: outcome.status,
        },
      };
    },
    catch: toError,
  }).pipe(
    Effect.catchAll((error) =>
      Effect.succeed({
        kind: "event" as const,
        event: {
          type: "tool_call" as const,
          tool_name: "sandbox.fetch",
          input: {
            url: targetURL,
          },
          output: {
            error: error.message,
            url: targetURL,
          },
          duration_ms: 5,
          status: "failed",
        },
      })
    )
  );
}

export function buildNDJSONResponseBody(
  outputs: readonly RuntimeOutputLine[]
): string {
  if (outputs.length === 0) {
    return "";
  }
  return `${outputs.map(serializeOutputLine).join("\n")}\n`;
}
