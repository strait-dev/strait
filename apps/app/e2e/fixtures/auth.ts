import { test as base } from "@playwright/test";
import { ApiHelper } from "./api";

const BLOCKED_DOMAINS = [
  "posthog.com",
  "us.posthog.com",
  "sentry.io",
  "google-analytics.com",
  "googletagmanager.com",
  "accounts.google.com",
  "apis.google.com",
  "ngrok.com",
  "ngrok-free.dev",
  "stripe.com",
  "js.stripe.com",
];

/**
 * Extended test fixture with authenticated page, API helper, and
 * third-party resource blocking for faster test execution.
 */
export const test = base.extend<{ api: ApiHelper }>({
  page: async ({ page }, use) => {
    // Block analytics, tracking, and third-party scripts
    await page.route("**/*", (route) => {
      const url = route.request().url();
      if (BLOCKED_DOMAINS.some((d) => url.includes(d))) {
        return route.abort();
      }
      return route.continue();
    });
    await use(page);
  },
  // biome-ignore lint/correctness/noEmptyPattern: Playwright fixture API requires destructured first param
  api: async ({}, use) => {
    const api = new ApiHelper();
    await use(api);
  },
});

export { expect } from "@playwright/test";
