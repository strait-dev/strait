export type JsonPrimitive = null | boolean | number | string;

export type JsonValue =
  | JsonPrimitive
  | JsonValue[]
  | {
      [key: string]: JsonValue;
    };

export interface AgentBudget {
  maxCostMicrousd?: number;
  maxTokens?: number;
  maxToolCalls?: number;
}

export interface RetryPolicy {
  maxAttempts: number;
  baseDelayMs: number;
  maxDelayMs: number;
}

export interface ModelPricing {
  provider: string;
  model: string;
  inputCostMicrousd: number;
  outputCostMicrousd: number;
  aliases?: readonly string[];
}

export type PricingCatalog = readonly ModelPricing[];

export interface UsageReport {
  provider: string;
  model: string;
  promptTokens: number;
  completionTokens: number;
  totalTokens?: number;
  costMicrousd?: number;
}

export interface NormalizedUsageReport extends UsageReport {
  totalTokens: number;
  costMicrousd: number;
}

export interface ToolCallReport {
  toolName: string;
  input?: JsonValue;
  output?: JsonValue;
  durationMs?: number;
  status?: string;
}

export interface CheckpointOptions {
  source?: string;
}

export interface ProgressReport {
  percent: number;
  message: string;
  step?: string;
  etaSeconds?: number;
}

export interface LogReport {
  message: string;
  type?: string;
  level?: string;
  data?: JsonValue;
}

export interface StreamChunkReport {
  chunk: string;
  streamId?: string;
  done?: boolean;
}

export interface BudgetSnapshot {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  costMicrousd: number;
  toolCalls: number;
  limits: AgentBudget;
}

export interface StraitContextOptions {
  baseUrl: string;
  runId: string;
  runToken: string;
  fetch?: typeof fetch;
  sdkVersion?: string;
  retry?: Partial<RetryPolicy>;
  pricingCatalog?: PricingCatalog;
  budget?: AgentBudget;
}

export interface StraitContextEnv {
  STRAIT_API_URL?: string;
  STRAIT_RUN_ID?: string;
  STRAIT_RUN_TOKEN?: string;
}
