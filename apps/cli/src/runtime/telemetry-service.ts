import { Context, Effect, Layer } from "effect";

/**
 * Supported telemetry log levels.
 */
export type LogLevel = "debug" | "info" | "warn" | "error";

type TelemetryService = {
  /** Emits a structured log event at the provided level. */
  readonly log: (
    level: LogLevel,
    message: string,
    fields?: Readonly<Record<string, unknown>>
  ) => Effect.Effect<void>;
  /** Shortcut for debug logs. */
  readonly debug: (
    message: string,
    fields?: Readonly<Record<string, unknown>>
  ) => Effect.Effect<void>;
  /** Shortcut for info logs. */
  readonly info: (
    message: string,
    fields?: Readonly<Record<string, unknown>>
  ) => Effect.Effect<void>;
  /** Shortcut for warning logs. */
  readonly warn: (
    message: string,
    fields?: Readonly<Record<string, unknown>>
  ) => Effect.Effect<void>;
  /** Shortcut for error logs. */
  readonly error: (
    message: string,
    fields?: Readonly<Record<string, unknown>>
  ) => Effect.Effect<void>;
};

/**
 * Runtime service for CLI diagnostics and telemetry output.
 */
export class TelemetryServiceTag extends Context.Tag("TelemetryService")<
  TelemetryServiceTag,
  TelemetryService
>() {}

const levelWeight: Record<LogLevel, number> = {
  debug: 10,
  info: 20,
  warn: 30,
  error: 40,
};

const getLogLevel = (): LogLevel => {
  const raw = process.env.STRAIT_LOG_LEVEL?.toLowerCase();
  if (raw === "debug" || raw === "info" || raw === "warn" || raw === "error") {
    return raw;
  }
  return process.env.STRAIT_VERBOSE === "true" ? "debug" : "info";
};

const liveTelemetryService: TelemetryService = {
  log: (level, message, fields) =>
    Effect.sync(() => {
      const threshold = getLogLevel();
      if (levelWeight[level] < levelWeight[threshold]) {
        return;
      }

      const payload = {
        level,
        message,
        timestamp: new Date().toISOString(),
        ...(fields ?? {}),
      };

      process.stderr.write(`${JSON.stringify(payload)}\n`);
    }),
  debug: (message, fields) =>
    liveTelemetryService.log("debug", message, fields),
  info: (message, fields) => liveTelemetryService.log("info", message, fields),
  warn: (message, fields) => liveTelemetryService.log("warn", message, fields),
  error: (message, fields) =>
    liveTelemetryService.log("error", message, fields),
};

/**
 * Live telemetry service layer.
 */
export const TelemetryServiceLive = Layer.succeed(
  TelemetryServiceTag,
  liveTelemetryService
);
