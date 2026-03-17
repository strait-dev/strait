/** Strait SDK — HTTP client for managed execution containers. */

export interface CompleteRequest {
  result: unknown;
}

export interface FailRequest {
  error: string;
  error_class?: string;
}

export interface LogRequest {
  level: string;
  message: string;
}

export interface PayloadResponse {
  payload: unknown;
}

export class StraitClient {
  private readonly baseUrl: string;
  private readonly token: string;

  constructor(baseUrl: string, token: string) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.token = token;
  }

  private headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.token}`,
      "Content-Type": "application/json",
    };
  }

  /** Fetch with retry and exponential backoff for critical calls. */
  private async fetchWithRetry(
    url: string,
    init: RequestInit,
    maxAttempts = 3
  ): Promise<Response> {
    let lastError: Error | undefined;
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        const resp = await fetch(url, {
          ...init,
          signal: AbortSignal.timeout(30_000),
        });
        if (resp.ok || resp.status < 500) return resp;
        lastError = new Error(`server error (${resp.status})`);
      } catch (err: unknown) {
        lastError = err instanceof Error ? err : new Error(String(err));
      }
      if (attempt < maxAttempts - 1) {
        const backoffMs = (1 << attempt) * 1000 + Math.random() * 500;
        await new Promise((r) => setTimeout(r, backoffMs));
      }
    }
    throw lastError ?? new Error("fetchWithRetry: all attempts failed");
  }

  /** Mark a run as completed with a result. */
  async complete(runId: string, result: unknown): Promise<void> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/complete`;
    const body: CompleteRequest = { result };

    const resp = await this.fetchWithRetry(url, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(
        `complete failed (${resp.status}): ${err.error || resp.statusText}`
      );
    }
  }

  /** Mark a run as failed with an error. */
  async fail(
    runId: string,
    error: string,
    errorClass?: string
  ): Promise<void> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/fail`;
    const body: FailRequest = { error, error_class: errorClass };

    const resp = await this.fetchWithRetry(url, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(
        `fail failed (${resp.status}): ${err.error || resp.statusText}`
      );
    }
  }

  /** Send a heartbeat for a run. */
  async heartbeat(runId: string): Promise<void> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/heartbeat`;

    const resp = await fetch(url, {
      method: "POST",
      headers: this.headers(),
      signal: AbortSignal.timeout(30_000),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(
        `heartbeat failed (${resp.status}): ${err.error || resp.statusText}`
      );
    }
  }

  /** Fetch the payload for a run. */
  async fetchPayload(runId: string): Promise<unknown> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/payload`;

    const resp = await fetch(url, {
      method: "GET",
      headers: this.headers(),
      signal: AbortSignal.timeout(30_000),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(
        `fetchPayload failed (${resp.status}): ${err.error || resp.statusText}`
      );
    }

    const data = (await resp.json()) as PayloadResponse;
    return data.payload;
  }

  /** Send a log entry for a run. */
  async log(runId: string, level: string, message: string): Promise<void> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/log`;
    const body: LogRequest = { level, message };

    const resp = await fetch(url, {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(30_000),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(
        `log failed (${resp.status}): ${err.error || resp.statusText}`
      );
    }
  }

  /** Report in-container resource usage (best-effort, non-fatal). */
  async reportResources(
    runId: string,
    memoryMb: number,
    memoryPercent?: number,
    cpuPercent?: number
  ): Promise<void> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/resources`;
    const body: Record<string, number> = { memory_mb: memoryMb };
    if (memoryPercent !== undefined) body.memory_percent = memoryPercent;
    if (cpuPercent !== undefined) body.cpu_percent = cpuPercent;

    try {
      await fetch(url, {
        method: "POST",
        headers: this.headers(),
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(10_000),
      });
    } catch {
      // Resource reporting is best-effort.
    }
  }
}
