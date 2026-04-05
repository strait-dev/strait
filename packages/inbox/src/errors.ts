import { Data } from "effect";

export class InboxClientError extends Data.TaggedError("InboxClientError")<{
  readonly path: string;
  readonly method: string;
  readonly status?: number;
  readonly details: string;
  readonly cause?: unknown;
}> {}
