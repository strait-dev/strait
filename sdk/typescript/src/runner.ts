/** Strait SDK — Runner for managed execution containers. */

import { StraitClient } from "./client";

const HEARTBEAT_INTERVAL_MS = 10_000;
const SIGTERM_GRACE_MS = 5_000;
const RESOURCE_MONITOR_INTERVAL_MS = 5_000;
const MEMORY_WARN_PERCENT = 80;
const MEMORY_ERROR_PERCENT = 90;

/** Context passed to the user's handler function. */
export interface RunContext {
  /** The unique run identifier. */
  runId: string;
  /** The job slug for this run. */
  jobSlug: string;
  /** Current attempt number. */
  attempt: number;
  /** The run payload (inline or fetched). */
  payload: unknown;
  /** Abort signal — set when SIGTERM is received. */
  signal: AbortSignal;
  /** Secrets injected via STRAIT_SECRET_* env vars. */
  secrets: Record<string, string>;
  /** Send a log entry for this run. */
  log(level: string, msg: string): Promise<void>;
}

/** Handler function type. */
export type RunHandler<T> = (ctx: RunContext) => Promise<T>;

/** Read STRAIT_SECRET_* env vars into a plain object. */
function readSecrets(): Record<string, string> {
  const secrets: Record<string, string> = {};
  const prefix = "STRAIT_SECRET_";
  for (const key of Object.keys(process.env)) {
    if (key.startsWith(prefix)) {
      const name = key.slice(prefix.length);
      const value = process.env[key];
      if (value !== undefined) {
        secrets[name] = value;
      }
    }
  }
  return secrets;
}

export class StraitRunner {
  private readonly client: StraitClient;
  private readonly runId: string;
  private readonly jobSlug: string;
  private readonly attempt: number;
  private readonly payloadMode: string;
  private readonly inlinePayload: unknown;

  constructor(opts: {
    client: StraitClient;
    runId: string;
    jobSlug: string;
    attempt: number;
    payloadMode: string;
    inlinePayload: unknown;
  }) {
    this.client = opts.client;
    this.runId = opts.runId;
    this.jobSlug = opts.jobSlug;
    this.attempt = opts.attempt;
    this.payloadMode = opts.payloadMode;
    this.inlinePayload = opts.inlinePayload;
  }

  /** Create a StraitRunner from environment variables. */
  static fromEnv(): StraitRunner {
    const runId = process.env.STRAIT_RUN_ID;
    if (!runId) {
      throw new Error("STRAIT_RUN_ID environment variable is required");
    }

    const token = process.env.STRAIT_SDK_TOKEN;
    if (!token) {
      throw new Error("STRAIT_SDK_TOKEN environment variable is required");
    }

    const baseUrl =
      process.env.STRAIT_API_URL ?? "https://api.runstrait.com";
    const jobSlug = process.env.STRAIT_JOB_SLUG ?? "";
    const attempt = Number.parseInt(process.env.STRAIT_ATTEMPT ?? "1", 10);
    const payloadMode = process.env.STRAIT_PAYLOAD_MODE ?? "inline";

    let inlinePayload: unknown = undefined;
    if (payloadMode === "inline" && process.env.STRAIT_PAYLOAD) {
      try {
        inlinePayload = JSON.parse(process.env.STRAIT_PAYLOAD);
      } catch {
        inlinePayload = process.env.STRAIT_PAYLOAD;
      }
    }

    const client = new StraitClient(baseUrl, token);

    return new StraitRunner({
      client,
      runId,
      jobSlug,
      attempt,
      payloadMode,
      inlinePayload,
    });
  }

  /** Execute the handler with full lifecycle management. */
  async run<T>(handler: RunHandler<T>): Promise<void> {
    const abortController = new AbortController();
    let heartbeatTimer: ReturnType<typeof setInterval> | undefined;
    let graceTimer: ReturnType<typeof setTimeout> | undefined;

    // SIGTERM handler: abort and give grace period.
    const onSigterm = () => {
      abortController.abort();
      graceTimer = setTimeout(() => {
        process.exit(1);
      }, SIGTERM_GRACE_MS);
    };
    process.on("SIGTERM", onSigterm);

    let resourceTimer: ReturnType<typeof setInterval> | undefined;

    let exitCode = 0;
    try {
      // Start heartbeat.
      heartbeatTimer = setInterval(() => {
        this.client.heartbeat(this.runId).catch(() => {
          // Heartbeat failures are non-fatal; the server will time out the run.
        });
      }, HEARTBEAT_INTERVAL_MS);

      // Start resource monitor.
      const memoryLimitMb = process.env.STRAIT_MEMORY_LIMIT_MB
        ? Number.parseFloat(process.env.STRAIT_MEMORY_LIMIT_MB)
        : undefined;

      resourceTimer = setInterval(() => {
        try {
          const mem = process.memoryUsage();
          const rssMb = mem.rss / (1024 * 1024);
          let memPct: number | undefined;

          if (memoryLimitMb && memoryLimitMb > 0) {
            memPct = (rssMb / memoryLimitMb) * 100;
            if (memPct >= MEMORY_ERROR_PERCENT) {
              console.error(`[strait] memory pressure critical: ${rssMb.toFixed(1)}MB (${memPct.toFixed(1)}%)`);
            } else if (memPct >= MEMORY_WARN_PERCENT) {
              console.warn(`[strait] memory pressure warning: ${rssMb.toFixed(1)}MB (${memPct.toFixed(1)}%)`);
            }
          }

          this.client.reportResources(this.runId, rssMb, memPct).catch(() => {});
        } catch {
          // Resource monitoring is non-fatal.
        }
      }, RESOURCE_MONITOR_INTERVAL_MS);

      // Resolve payload.
      let payload: unknown = this.inlinePayload;
      if (this.payloadMode === "fetch") {
        payload = await this.client.fetchPayload(this.runId);
      }

      // Build context.
      const ctx: RunContext = {
        runId: this.runId,
        jobSlug: this.jobSlug,
        attempt: this.attempt,
        payload,
        signal: abortController.signal,
        secrets: readSecrets(),
        log: (level: string, msg: string) =>
          this.client.log(this.runId, level, msg),
      };

      // Run the handler.
      const result = await handler(ctx);

      // Report success.
      await this.client.complete(this.runId, result);
    } catch (err: unknown) {
      exitCode = 1;
      // Report failure.
      const errorMessage =
        err instanceof Error ? err.message : String(err);
      const errorClass =
        err instanceof Error ? err.constructor.name : undefined;

      try {
        await this.client.fail(this.runId, errorMessage, errorClass);
      } catch {
        // If fail reporting itself fails, we still exit.
      }
    } finally {
      if (heartbeatTimer !== undefined) {
        clearInterval(heartbeatTimer);
      }
      if (resourceTimer !== undefined) {
        clearInterval(resourceTimer);
      }
      if (graceTimer !== undefined) {
        clearTimeout(graceTimer);
      }
      process.removeListener("SIGTERM", onSigterm);
      process.exit(exitCode);
    }
  }
}
