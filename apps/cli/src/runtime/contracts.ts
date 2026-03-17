/**
 * Profile context persisted on disk for CLI usage.
 */
export type StraitCliContext = {
  /** Base API URL for this context. */
  readonly serverUrl: string;
  /** Optional default project identifier. */
  readonly projectId?: string;
  /** Optional API key bound to this context. */
  readonly apiKey?: string;
};

/**
 * Persisted CLI profile file shape.
 */
export type StraitCliProfile = {
  /** Active context name used when no explicit override is provided. */
  readonly activeContext?: string;
  /** Named contexts available to the user. */
  readonly contexts: Readonly<Record<string, StraitCliContext>>;
};

/**
 * Canonical empty profile value.
 */
export const EMPTY_PROFILE: StraitCliProfile = {
  activeContext: undefined,
  contexts: {},
};

/**
 * Connection settings resolved for API-facing commands.
 */
export type ResolvedConnection = {
  readonly serverUrl: string;
  readonly projectId?: string;
  readonly apiKey?: string;
  readonly contextName?: string;
};

/**
 * Runtime-scoped output mode.
 */
export type RenderMode = "interactive" | "deterministic";

/**
 * Output format used by deterministic rendering.
 */
export type RenderFormat = "plain" | "json";
