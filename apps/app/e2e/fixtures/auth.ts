import { test as base } from "@playwright/test";
import { ApiHelper } from "./api";

/**
 * Extended test fixture with authenticated page and API helper.
 *
 * Usage:
 * ```ts
 * import { test, expect } from "@/e2e/fixtures";
 * test("my test", async ({ page, api }) => { ... });
 * ```
 */
export const test = base.extend<{ api: ApiHelper }>({
  // biome-ignore lint/correctness/noEmptyPattern: Playwright fixture API requires destructured first param
  api: async ({}, use) => {
    const api = new ApiHelper();
    await use(api);
  },
});

export { expect } from "@playwright/test";
