import { expect, type Page } from "@playwright/test";

const ROUTE_CRASH_PATTERNS = [
  /Failed to fetch dynamically imported module/i,
  /error loading dynamically imported module/i,
  /Importing a module script failed/i,
  /Error in route match/i,
  /\?tsr-split=/i,
];

export type RouteCrashWatcher = {
  readonly errors: string[];
  assertNoCrashes: () => void;
};

/** Capture TanStack route chunk failures that otherwise only surface in the browser. */
export function watchForRouteCrashes(page: Page): RouteCrashWatcher {
  const errors: string[] = [];

  const record = (source: string, value: string) => {
    if (ROUTE_CRASH_PATTERNS.some((pattern) => pattern.test(value))) {
      errors.push(`${source}: ${value}`);
    }
  };

  page.on("console", (message) => {
    if (message.type() === "error" || message.type() === "warning") {
      record(`console.${message.type()}`, message.text());
    }
  });

  page.on("pageerror", (error) => {
    record("pageerror", error.stack ?? error.message);
  });

  page.on("requestfailed", (request) => {
    const url = request.url();
    if (url.includes("?tsr-split=")) {
      record(
        "requestfailed",
        `${url} ${request.failure()?.errorText ?? "failed"}`
      );
    }
  });

  page.on("response", (response) => {
    const url = response.url();
    if (url.includes("?tsr-split=") && !response.ok()) {
      record("response", `${response.status()} ${url}`);
    }
  });

  return {
    errors,
    assertNoCrashes: () => {
      expect(errors).toEqual([]);
    },
  };
}
