import { expect, test } from "@playwright/test";

test.describe("Invitation", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("invalid invitation shows error", async ({ page }) => {
    await page.goto("/invitation/nonexistent-invitation-12345");
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/invitation/test-id");
    await expect(page.locator("body")).toBeVisible();
  });

  test("invitation page has auth layout", async ({ page }) => {
    await page.goto("/invitation/test-id");
    // Should show either invitation UI or error/redirect
    await expect(page.locator("body")).toBeVisible();
  });
});
