export class StraitSDKError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = new.target.name;
  }
}

export class StraitAPIError extends StraitSDKError {
  readonly status: number;
  readonly code?: string;
  readonly details?: readonly string[];
  readonly responseBody?: unknown;

  constructor(
    message: string,
    options: {
      status: number;
      code?: string;
      details?: readonly string[];
      responseBody?: unknown;
      cause?: unknown;
    }
  ) {
    super(message, { cause: options.cause });
    this.status = options.status;
    this.code = options.code;
    this.details = options.details;
    this.responseBody = options.responseBody;
  }
}

export class UnknownPricingError extends StraitSDKError {
  readonly provider: string;
  readonly model: string;

  constructor(provider: string, model: string) {
    super(`no pricing entry found for provider=${provider} model=${model}`);
    this.provider = provider;
    this.model = model;
  }
}

export type BudgetLimitKind = "cost" | "tokens" | "tool_calls";

export class BudgetExceededError extends StraitSDKError {
  readonly kind: BudgetLimitKind;
  readonly limit: number;
  readonly current: number;
  readonly requested: number;

  constructor(
    kind: BudgetLimitKind,
    limit: number,
    current: number,
    requested: number
  ) {
    super(
      `${kind} budget exceeded: current=${current} requested=${requested} limit=${limit}`
    );
    this.kind = kind;
    this.limit = limit;
    this.current = current;
    this.requested = requested;
  }
}
