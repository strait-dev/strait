export type JsonValue =
  | null
  | boolean
  | number
  | string
  | JsonValue[]
  | { [key: string]: JsonValue };

export const runtimeContractVersion = "v1";

export type DispatchEnvelope = {
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
    sandbox_policy?: JsonValue;
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

export type CloudflareSandboxPolicy = {
  mode?: "disabled" | "outbound_worker";
  outbound_worker_name?: string;
  default_action?: "allow" | "deny";
  allow_hosts?: string[];
  network_class?: string;
  policy_tag?: string;
};

export type RuntimeEvent =
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

export type RuntimeOutputLine =
  | { kind: "event"; event: RuntimeEvent }
  | { kind: "raw"; line: string };

export function asRecord(
  value: JsonValue | undefined
): Record<string, JsonValue> {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, JsonValue>;
  }
  return {};
}
