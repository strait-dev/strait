export type JsonPrimitive = null | boolean | number | string;

export type JsonValue =
  | JsonPrimitive
  | JsonValue[]
  | {
      [key: string]: JsonValue;
    };

export interface AgentBudget {
  maxCostMicrousd?: number;
  maxIterations?: number;
  maxTokens?: number;
  maxToolCalls?: number;
}

export type BudgetInput = AgentBudget | number | string;

export interface RetryPolicy {
  baseDelayMs: number;
  maxAttempts: number;
  maxDelayMs: number;
}

export interface ModelPricing {
  aliases?: readonly string[];
  cacheReadCostMicrousd?: number;
  cacheWriteCostMicrousd?: number;
  inputCostMicrousd: number;
  model: string;
  outputCostMicrousd: number;
  provider: string;
}

export type PricingCatalog = readonly ModelPricing[];

export interface UsageReport {
  completionTokenDetails?: {
    reasoningTokens?: number;
    textTokens?: number;
  };
  completionTokens: number;
  costMicrousd?: number;
  metadata?: JsonValue;
  model: string;
  promptTokenDetails?: {
    cacheReadTokens?: number;
    cacheWriteTokens?: number;
  };
  promptTokens: number;
  provider: string;
  totalTokens?: number;
}

export interface NormalizedUsageReport extends UsageReport {
  costMicrousd: number;
  totalTokens: number;
}

export interface ToolCallReport {
  durationMs?: number;
  input?: JsonValue;
  output?: JsonValue;
  status?: string;
  toolName: string;
}

export interface CheckpointOptions {
  source?: string;
}

export interface ProgressReport {
  etaSeconds?: number;
  message: string;
  percent: number;
  step?: string;
}

export interface LogReport {
  data?: JsonValue;
  level?: string;
  message: string;
  type?: string;
}

export interface StreamChunkReport {
  chunk: string;
  done?: boolean;
  streamId?: string;
}

export interface BudgetSnapshot {
  completionTokens: number;
  costMicrousd: number;
  iterations: number;
  limits: AgentBudget;
  promptTokens: number;
  toolCalls: number;
  totalTokens: number;
}

export interface StraitContextOptions {
  baseUrl: string;
  budget?: BudgetInput;
  fetch?: typeof fetch;
  pricingCatalog?: PricingCatalog;
  retry?: Partial<RetryPolicy>;
  runId: string;
  runToken: string;
  sdkVersion?: string;
}

export interface StraitContextEnv {
  STRAIT_API_URL?: string;
  STRAIT_RUN_ID?: string;
  STRAIT_RUN_TOKEN?: string;
}

export interface UsageTotals {
  completionTokens: number;
  costMicrousd: number;
  promptTokens: number;
  totalTokens: number;
}

export interface SandboxExecutionTarget {
  executionMode: "sandboxed";
  image?: string;
  mode: "dynamic-worker" | "outbound-worker";
  networkClass?: string;
  outboundPolicyTag?: string;
  runtime?: string;
  timeoutMs?: number;
}

export interface SandboxTool<TInput = JsonValue, TResult = JsonValue> {
  description?: string;
  execute: (input: TInput) => Promise<TResult>;
  name: string;
  sandbox: SandboxExecutionTarget;
}
