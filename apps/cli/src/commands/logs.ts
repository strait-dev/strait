import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import {
  ApiServiceTag,
  ConfigServiceTag,
  type JsonApiRequest,
  RendererServiceTag,
} from "../runtime";
import { normalizeCollection } from "./operational-helpers";

type LogsFlags = {
  readonly follow?: boolean;
  readonly job?: string;
  readonly run?: string;
  readonly level?: string;
  readonly json?: boolean;
  readonly workflowRun?: string;
  readonly step?: string;
  readonly context?: string;
  readonly server?: string;
};

type LogEvent = {
  readonly timestamp?: string;
  readonly level?: string;
  readonly message?: string;
  readonly [key: string]: unknown;
};

const TRAILING_SLASH_RE = /\/+$/;

const LOG_LEVEL_PRIORITY: Record<string, number> = {
  error: 3,
  warn: 2,
  info: 1,
  debug: 0,
};

const levelColor = (level: string): string => {
  if (!process.stdout.isTTY) {
    return level.toUpperCase();
  }
  if (level === "error") {
    return `\x1b[31m${level.toUpperCase()}\x1b[0m`;
  }
  if (level === "warn") {
    return `\x1b[33m${level.toUpperCase()}\x1b[0m`;
  }
  return level.toUpperCase();
};

const passesLevelFilter = (
  eventLevel: string | undefined,
  filterLevel: string | undefined
): boolean => {
  if (!(filterLevel && eventLevel)) {
    return true;
  }
  const filterPriority = LOG_LEVEL_PRIORITY[filterLevel.toLowerCase()] ?? 0;
  const eventPriority = LOG_LEVEL_PRIORITY[eventLevel.toLowerCase()] ?? 0;
  return eventPriority >= filterPriority;
};

const formatLogLine = (event: LogEvent): string => {
  const timestamp = event.timestamp ?? new Date().toISOString();
  const level = event.level ?? "info";
  const message = event.message ?? "";
  return `[${timestamp}] [${levelColor(level)}] ${message}`;
};

const writeEventLine = (event: LogEvent, asJson: boolean): void => {
  const line = asJson ? JSON.stringify(event) : formatLogLine(event);
  process.stdout.write(`${line}\n`);
};

const resolveRunId = (
  apiService: { requestJson: ApiServiceTag["Type"]["requestJson"] },
  flags: LogsFlags,
  connectionInput: {
    contextName?: string;
    serverUrl?: string;
    projectId?: string;
  }
): Effect.Effect<string, Error> =>
  Effect.gen(function* () {
    if (flags.run) {
      return flags.run;
    }

    if (flags.job) {
      const jobsRequest: JsonApiRequest = {
        method: "GET",
        path: "/v1/jobs",
        connection: connectionInput,
      };
      const jobsRaw = yield* apiService.requestJson<unknown>(jobsRequest);
      const jobs = normalizeCollection(jobsRaw);
      const matchedJob = jobs.find(
        (j) => j.slug === flags.job || j.name === flags.job
      );
      if (!matchedJob) {
        return yield* Effect.fail(new Error(`Job "${flags.job}" not found`));
      }

      const jobId = matchedJob.id as string;
      const runsRequest: JsonApiRequest = {
        method: "GET",
        path: "/v1/runs",
        query: { limit: 1 },
        connection: connectionInput,
      };
      const runsRaw = yield* apiService.requestJson<unknown>(runsRequest);
      const runs = normalizeCollection(runsRaw);
      const matchedRun = runs.find((r) => r.job_id === jobId);
      if (!matchedRun) {
        return yield* Effect.fail(
          new Error(`No runs found for job "${flags.job}"`)
        );
      }
      return matchedRun.id as string;
    }

    return yield* Effect.fail(
      new Error("Either --run or --job flag is required")
    );
  });

const renderEvents = (
  events: readonly LogEvent[],
  flags: LogsFlags,
  renderer: { line: RendererServiceTag["Type"]["line"] }
): Effect.Effect<void> =>
  Effect.gen(function* () {
    for (const event of events) {
      if (!passesLevelFilter(event.level, flags.level)) {
        continue;
      }
      if (flags.json) {
        yield* renderer.line(JSON.stringify(event));
      } else {
        yield* renderer.line(formatLogLine(event));
      }
    }
  });

const parseSSEFrames = (
  buffer: string
): { events: string[]; remaining: string } => {
  const events: string[] = [];
  let remaining = buffer;

  let doubleNewline = remaining.indexOf("\n\n");
  while (doubleNewline !== -1) {
    const frame = remaining.slice(0, doubleNewline);
    remaining = remaining.slice(doubleNewline + 2);

    for (const line of frame.split("\n")) {
      if (line.startsWith("data: ")) {
        events.push(line.slice(6));
      }
    }
    doubleNewline = remaining.indexOf("\n\n");
  }

  return { events, remaining };
};

const processSSEChunk = (
  chunk: Uint8Array | undefined,
  decoder: TextDecoder,
  buffer: string,
  flags: LogsFlags
): string => {
  if (!chunk) {
    return buffer;
  }

  let updated = buffer + decoder.decode(chunk, { stream: true });
  const parsed = parseSSEFrames(updated);
  updated = parsed.remaining;

  for (const data of parsed.events) {
    try {
      const event = JSON.parse(data) as LogEvent;
      if (passesLevelFilter(event.level, flags.level)) {
        writeEventLine(event, Boolean(flags.json));
      }
    } catch {
      // skip malformed data
    }
  }

  return updated;
};

const readSSEStream = async (
  response: Response,
  flags: LogsFlags
): Promise<void> => {
  if (!response.body) {
    throw new Error("No response body for SSE stream");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let done = false;

  while (!done) {
    const chunk = await reader.read();
    done = chunk.done;
    buffer = processSSEChunk(chunk.value, decoder, buffer, flags);
  }
};

const streamSSE = (
  serverUrl: string,
  runId: string,
  apiKey: string | undefined,
  flags: LogsFlags
): Effect.Effect<void, Error> =>
  Effect.tryPromise({
    try: async () => {
      const url = `${serverUrl.replace(TRAILING_SLASH_RE, "")}/v1/runs/${encodeURIComponent(runId)}/stream`;
      const controller = new AbortController();
      const cleanup = () => controller.abort();
      process.on("SIGINT", cleanup);

      try {
        const headers: Record<string, string> = {
          Accept: "text/event-stream",
          "Cache-Control": "no-cache",
        };
        if (apiKey) {
          headers.Authorization = `Bearer ${apiKey}`;
        }

        const response = await fetch(url, {
          headers,
          signal: controller.signal,
        });

        if (!response.ok) {
          throw new Error(
            `SSE connection failed with status ${response.status}`
          );
        }

        await readSSEStream(response, flags);
      } finally {
        process.removeListener("SIGINT", cleanup);
      }
    },
    catch: (error) => {
      if (error instanceof Error && error.name === "AbortError") {
        return new Error("Stream ended");
      }
      return new Error("SSE stream error", { cause: error });
    },
  });

/**
 * `strait logs` fetches run output or streams live logs via SSE.
 */
export const logsCommand = buildCommand({
  async func(this: StraitCommandContext, flags: LogsFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;
        const configService = yield* ConfigServiceTag;
        const renderer = yield* RendererServiceTag;

        const connection = yield* configService.resolveConnection({
          contextName: flags.context,
          serverUrl: flags.server,
          requireServer: true,
        });

        const connectionInput = {
          contextName: connection.contextName,
          serverUrl: connection.serverUrl,
          projectId: connection.projectId,
        };

        const runId = yield* resolveRunId(apiService, flags, connectionInput);

        if (flags.follow) {
          yield* streamSSE(
            connection.serverUrl,
            runId,
            connection.apiKey,
            flags
          );
          return;
        }

        const runDetails = yield* apiService.requestJson<
          Record<string, unknown>
        >({
          method: "GET",
          path: `/v1/runs/${encodeURIComponent(runId)}`,
          connection: connectionInput,
        });

        const events: LogEvent[] = [];

        if (Array.isArray(runDetails.events)) {
          for (const event of runDetails.events) {
            if (typeof event === "object" && event !== null) {
              events.push(event as LogEvent);
            }
          }
        }

        if (
          typeof runDetails.output === "string" &&
          runDetails.output.length > 0 &&
          events.length === 0
        ) {
          events.push({
            timestamp:
              (runDetails.completed_at as string) ??
              (runDetails.updated_at as string) ??
              new Date().toISOString(),
            level: "info",
            message: runDetails.output as string,
          });
        }

        yield* renderEvents(events, flags, renderer);
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      follow: {
        kind: "boolean",
        brief: "Stream live logs via SSE",
        optional: true,
      },
      job: {
        kind: "parsed",
        parse: String,
        brief: "Job slug to find latest run",
        optional: true,
      },
      run: {
        kind: "parsed",
        parse: String,
        brief: "Run ID to fetch logs for",
        optional: true,
      },
      level: {
        kind: "parsed",
        parse: String,
        brief: "Minimum log level filter (debug, info, warn, error)",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output NDJSON format",
        optional: true,
      },
      workflowRun: {
        kind: "parsed",
        parse: String,
        brief: "Workflow run ID",
        optional: true,
      },
      step: {
        kind: "parsed",
        parse: String,
        brief: "Workflow step reference",
        optional: true,
      },
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL override",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Fetch run logs or stream live logs",
  },
});
