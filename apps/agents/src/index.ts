import { readFileSync } from "node:fs";
import { setTimeout as sleep } from "node:timers/promises";

type JsonValue =
  | null
  | boolean
  | number
  | string
  | JsonValue[]
  | { [key: string]: JsonValue };

type DispatchEnvelope = {
  version: string;
  run: {
    id: string;
    project_id: string;
    attempt: number;
    timeout_secs: number;
  };
  agent: {
    id: string;
    slug: string;
    model: string;
    config?: JsonValue;
  };
  deployment: {
    id: string;
    version: number;
    provider: string;
    config_snapshot?: JsonValue;
  };
  payload?: JsonValue;
  callback: {
    base_url: string;
    run_id: string;
    run_token: string;
  };
  retry?: {
    last_checkpoint?: JsonValue;
    checkpoint_at?: string;
    previous_error?: string;
  };
};

type RuntimeEvent =
  | { type: "checkpoint"; state: JsonValue }
  | {
      type: "usage";
      provider: string;
      model: string;
      prompt_tokens: number;
      completion_tokens: number;
      total_tokens?: number;
      cost_microusd?: number;
    }
  | {
      type: "tool_call";
      tool_name: string;
      input?: JsonValue;
      output?: JsonValue;
      duration_ms?: number;
      status?: string;
    }
  | { type: "stream"; chunk: string; stream_id?: string; done?: boolean }
  | { type: "complete"; result?: JsonValue }
  | { type: "fail"; error: string };

function parseEnvelope(): DispatchEnvelope {
  const raw = readFileSync(0, "utf8");
  return JSON.parse(raw) as DispatchEnvelope;
}

function asRecord(value: JsonValue | undefined): Record<string, JsonValue> {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, JsonValue>;
  }
  return {};
}

function emit(event: RuntimeEvent): void {
  process.stdout.write(`${JSON.stringify(event)}\n`);
}

async function main(): Promise<void> {
  const envelope = parseEnvelope();
  const payload = asRecord(envelope.payload);
  const config = asRecord(envelope.agent.config);

  const scenario = String(payload._scenario ?? config.scenario ?? "success");
  const delayMs = Number(payload._delay_ms ?? config.delay_ms ?? 0);

  if (delayMs > 0) {
    await sleep(delayMs);
  }

  if (scenario === "invalid_json") {
    process.stdout.write("{not-json}\n");
    return;
  }

  emit({
    type: "stream",
    stream_id: "default",
    chunk: `agent:${envelope.agent.slug}:thinking `,
  });
  emit({
    type: "checkpoint",
    state: {
      phase: "planning",
      agent_id: envelope.agent.id,
      deployment_id: envelope.deployment.id,
    },
  });

  if (scenario === "duplicate_checkpoint") {
    emit({
      type: "checkpoint",
      state: {
        phase: "planning",
        duplicate: true,
      },
    });
  }

  emit({
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
  });
  emit({
    type: "usage",
    provider: "local",
    model: envelope.agent.model || "local-agent",
    prompt_tokens: 12,
    completion_tokens: 8,
    total_tokens: 20,
    cost_microusd: 200,
  });
  emit({
    type: "stream",
    stream_id: "default",
    chunk: "done",
    done: true,
  });

  if (scenario === "disconnect") {
    return;
  }

  if (scenario === "fail") {
    emit({
      type: "fail",
      error: String(payload._error ?? "runtime requested failure"),
    });
    return;
  }

  emit({
    type: "complete",
    result: {
      ok: true,
      agent_id: envelope.agent.id,
      run_id: envelope.run.id,
      payload,
    },
  });
}

await main();
