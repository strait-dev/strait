import { Effect } from "effect";

import { RendererServiceTag } from "../runtime";

const asRecord = (value: unknown): Record<string, unknown> | undefined =>
  typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : undefined;

const arrayRecord = (
  value: unknown
): readonly Record<string, unknown>[] | undefined => {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const allRecords = value.every(
    (entry) => typeof entry === "object" && entry !== null
  );

  return allRecords ? (value as readonly Record<string, unknown>[]) : undefined;
};

/**
 * Attempts to normalize varied list response shapes to array-of-records.
 */
export const normalizeCollection = (
  payload: unknown
): readonly Record<string, unknown>[] => {
  const directCollection = arrayRecord(payload);
  if (directCollection) {
    return directCollection;
  }

  const record = asRecord(payload);
  if (!record) {
    return [];
  }

  const wellKnownKeys = [
    "items",
    "data",
    "results",
    "jobs",
    "runs",
    "workflows",
    "events",
  ];
  for (const key of wellKnownKeys) {
    const value = arrayRecord(record[key]);
    if (value) {
      return value;
    }
  }

  for (const value of Object.values(record)) {
    const collection = arrayRecord(value);
    if (collection) {
      return collection;
    }
  }

  return [];
};

/**
 * Formats a single record into deterministic one-line text.
 */
export const summarizeRecord = (
  record: Record<string, unknown>,
  idCandidates: readonly string[]
): string => {
  const id =
    idCandidates
      .map((key) => record[key])
      .find((value): value is string => typeof value === "string") ??
    "<unknown>";

  let status = "";
  if (typeof record.status === "string") {
    status = ` status=${record.status}`;
  } else if (typeof record.state === "string") {
    status = ` status=${record.state}`;
  }

  return `${id}${status}`;
};

/**
 * Deterministic rendering helper shared by list/get command handlers.
 */
export const renderPayload = (
  payload: unknown,
  options: {
    readonly asJson?: boolean;
    readonly plainSummary?: readonly string[];
  }
): Effect.Effect<void, Error, RendererServiceTag> =>
  Effect.gen(function* () {
    const renderer = yield* RendererServiceTag;

    if (options.asJson) {
      yield* renderer.json(payload);
      return;
    }

    if (Array.isArray(payload)) {
      if (payload.length === 0) {
        yield* renderer.line("No records found.");
        return;
      }

      for (const line of options.plainSummary ??
        payload.map((entry) => String(entry))) {
        yield* renderer.line(line);
      }
      return;
    }

    yield* renderer.line(JSON.stringify(payload, null, 2));
  });
