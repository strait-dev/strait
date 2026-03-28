import { BudgetLedger } from "./budget";
import { StraitSDKError } from "./errors";
import { StraitHTTPClient } from "./http";
import { normalizeBudgetInput } from "./internal";
import { defaultPricingCatalog, normalizeUsageReport } from "./pricing";
import type {
  BudgetSnapshot,
  CheckpointOptions,
  JsonValue,
  LogReport,
  PricingCatalog,
  ProgressReport,
  StraitContextEnv,
  StraitContextOptions,
  StreamChunkReport,
  ToolCallReport,
  UsageReport,
} from "./types";

type RunUsageResponse = {
  id: string;
  run_id: string;
  provider: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost_microusd: number;
};

type RunCheckpointResponse = {
  id: string;
  run_id: string;
  source: string;
  state: JsonValue;
};

type RunToolCallResponse = {
  id: string;
  run_id: string;
  tool_name: string;
  status: string;
};

type RunStateResponse = {
  run_id: string;
  state_key: string;
  value: JsonValue;
  updated_at: string;
};

type JobRunResponse = {
  id: string;
  status: string;
};

type StateScopeClient = {
  set: (
    key: string,
    value: JsonValue,
    signal?: AbortSignal
  ) => Promise<RunStateResponse>;
  get: (key: string, signal?: AbortSignal) => Promise<RunStateResponse>;
  list: (signal?: AbortSignal) => Promise<RunStateResponse[]>;
  delete: (key: string, signal?: AbortSignal) => Promise<void>;
};

function assertNonEmptyString(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function assertNonNegativeInt(
  value: number | undefined,
  field: string
): number | undefined {
  if (value == null) {
    return undefined;
  }
  if (!Number.isInteger(value) || value < 0) {
    throw new StraitSDKError(`${field} must be a non-negative integer`);
  }
  return value;
}

export class StraitContext {
  static fromEnv(
    env: StraitContextEnv | NodeJS.ProcessEnv = process.env,
    options: Omit<StraitContextOptions, "baseUrl" | "runId" | "runToken"> = {}
  ): StraitContext {
    const baseUrl = env.STRAIT_API_URL?.trim();
    const runId = env.STRAIT_RUN_ID?.trim();
    const runToken = env.STRAIT_RUN_TOKEN?.trim();

    if (!baseUrl) {
      throw new StraitSDKError("STRAIT_API_URL is required");
    }
    if (!runId) {
      throw new StraitSDKError("STRAIT_RUN_ID is required");
    }
    if (!runToken) {
      throw new StraitSDKError("STRAIT_RUN_TOKEN is required");
    }

    return new StraitContext({
      ...options,
      baseUrl,
      runId,
      runToken,
    });
  }

  readonly #client: StraitHTTPClient;
  readonly #pricingCatalog: PricingCatalog;
  readonly #budget: BudgetLedger;

  readonly runId: string;
  readonly run: { state: StateScopeClient };
  readonly workflow: { state: StateScopeClient };

  constructor(options: StraitContextOptions) {
    this.runId = assertNonEmptyString(options.runId, "runId");
    this.#client = new StraitHTTPClient(options);
    this.#pricingCatalog = options.pricingCatalog ?? defaultPricingCatalog;
    this.#budget = new BudgetLedger(normalizeBudgetInput(options.budget));
    this.run = {
      state: this.#createStateScope("/state"),
    };
    this.workflow = {
      state: this.#createStateScope("/workflow-state"),
    };
  }

  #createStateScope(basePath: string): StateScopeClient {
    return {
      set: (key, value, signal) => this.#setState(basePath, key, value, signal),
      get: (key, signal) => this.#getState(basePath, key, signal),
      list: (signal) =>
        this.#client.get<RunStateResponse[]>(basePath, {
          retryable: true,
          signal,
        }),
      delete: async (key, signal) => {
        await this.#deleteState(basePath, key, signal);
      },
    };
  }

  #setState(
    basePath: string,
    key: string,
    value: JsonValue,
    signal?: AbortSignal
  ): Promise<RunStateResponse> {
    return this.#client.post<RunStateResponse>(
      basePath,
      {
        key: assertNonEmptyString(key, "key"),
        value,
      },
      { retryable: true, signal }
    );
  }

  #getState(
    basePath: string,
    key: string,
    signal?: AbortSignal
  ): Promise<RunStateResponse> {
    return this.#client.get<RunStateResponse>(
      `${basePath}/${encodeURIComponent(assertNonEmptyString(key, "key"))}`,
      { retryable: true, signal }
    );
  }

  async #deleteState(
    basePath: string,
    key: string,
    signal?: AbortSignal
  ): Promise<void> {
    await this.#client.delete<void>(
      `${basePath}/${encodeURIComponent(assertNonEmptyString(key, "key"))}`,
      { retryable: true, signal }
    );
  }

  budgetSnapshot(): BudgetSnapshot {
    return this.#budget.snapshot();
  }

  budgetExceeded(): void {
    this.#budget.assertWithinLimits();
  }

  heartbeat(signal?: AbortSignal): Promise<{ status: string }> {
    return this.#client.post<{ status: string }>(
      "/heartbeat",
      {},
      { retryable: true, signal }
    );
  }

  log(report: LogReport, signal?: AbortSignal): Promise<{ id: string }> {
    return this.#client.post<{ id: string }>(
      "/log",
      {
        type: report.type,
        level: report.level,
        message: assertNonEmptyString(report.message, "message"),
        data: report.data,
      },
      { retryable: true, signal }
    );
  }

  progress(
    report: ProgressReport,
    signal?: AbortSignal
  ): Promise<{ id: string }> {
    if (report.percent < 0 || report.percent > 100) {
      throw new StraitSDKError("percent must be between 0 and 100");
    }
    return this.#client.post<{ id: string }>(
      "/progress",
      {
        percent: report.percent,
        message: assertNonEmptyString(report.message, "message"),
        step: report.step,
        eta_seconds: assertNonNegativeInt(report.etaSeconds, "etaSeconds"),
      },
      { retryable: true, signal }
    );
  }

  checkpoint(
    state: JsonValue,
    options: CheckpointOptions = {},
    signal?: AbortSignal
  ): Promise<RunCheckpointResponse> {
    return this.#client.post<RunCheckpointResponse>(
      "/checkpoint",
      {
        source: options.source,
        state,
      },
      { retryable: true, signal }
    );
  }

  reportUsage(
    usage: UsageReport,
    signal?: AbortSignal
  ): Promise<RunUsageResponse> {
    const normalized = normalizeUsageReport(usage, this.#pricingCatalog);
    this.#budget.recordUsage(normalized);

    return this.#client.post<RunUsageResponse>(
      "/usage",
      {
        provider: normalized.provider,
        model: normalized.model,
        prompt_tokens: normalized.promptTokens,
        completion_tokens: normalized.completionTokens,
        total_tokens: normalized.totalTokens,
        cost_microusd: normalized.costMicrousd,
      },
      { retryable: true, signal }
    );
  }

  reportToolCall(
    report: ToolCallReport,
    signal?: AbortSignal
  ): Promise<RunToolCallResponse> {
    this.#budget.recordToolCall();

    return this.#client.post<RunToolCallResponse>(
      "/tool-call",
      {
        tool_name: assertNonEmptyString(report.toolName, "toolName"),
        input: report.input,
        output: report.output,
        duration_ms: assertNonNegativeInt(report.durationMs, "durationMs"),
        status: report.status,
      },
      { retryable: true, signal }
    );
  }

  stream(
    report: StreamChunkReport,
    signal?: AbortSignal
  ): Promise<{ status: string }> {
    if (report.chunk.length === 0 && report.done !== true) {
      throw new StraitSDKError(
        "stream chunk requires a non-empty chunk unless done=true"
      );
    }
    return this.#client.post<{ status: string }>(
      "/stream",
      {
        chunk: report.chunk,
        stream_id: report.streamId,
        done: report.done,
      },
      { retryable: true, signal }
    );
  }

  complete(result?: JsonValue, signal?: AbortSignal): Promise<JobRunResponse> {
    return this.#client.post<JobRunResponse>(
      "/complete",
      {
        result,
      },
      { signal }
    );
  }

  fail(error: string, signal?: AbortSignal): Promise<JobRunResponse> {
    return this.#client.post<JobRunResponse>(
      "/fail",
      {
        error: assertNonEmptyString(error, "error"),
      },
      { signal }
    );
  }
}

export function createStraitContext(
  options: StraitContextOptions
): StraitContext {
  return new StraitContext(options);
}
