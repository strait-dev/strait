/**
 * Signals that a persisted CLI context could not be found.
 */
export class ContextNotFoundError extends Error {
  readonly contextName: string;

  constructor(contextName: string) {
    super(`Context '${contextName}' was not found.`);
    this.name = "ContextNotFoundError";
    this.contextName = contextName;
  }
}

/**
 * Signals that an API key is required for the attempted command.
 */
export class MissingCredentialError extends Error {
  constructor() {
    super("No API key was resolved. Configure one with 'strait auth login'.");
    this.name = "MissingCredentialError";
  }
}

/**
 * Signals that a command requires server URL resolution and none was provided.
 */
export class MissingServerURLError extends Error {
  constructor() {
    super(
      "No server URL was resolved. Configure one with 'strait context create <name> --server <url>'."
    );
    this.name = "MissingServerURLError";
  }
}

/**
 * Signals that a command requires project identifier resolution and none was provided.
 */
export class MissingProjectIDError extends Error {
  constructor() {
    super(
      "No project ID was resolved. Configure one with 'strait context create <name> --project <id>' or pass --project."
    );
    this.name = "MissingProjectIDError";
  }
}
