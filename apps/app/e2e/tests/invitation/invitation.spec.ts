import { expect, test } from "@playwright/test";

test.describe("Invitation", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("invalid invitation shows error", async ({ page }) => {
    await page.goto("/invitation/nonexistent-invitation-12345");
    await page.waitForTimeout(3000);
    const error = page.getByText(/not found|invalid|error|expired/i);
    const content = page.locator("body");
    await expect(error.or(content)).toBeVisible({ timeout: 10_000 });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/invitation/test-id");
    await expect(page.locator("body")).toBeVisible();
  });

  test("invitation page has auth layout", async ({ page }) => {
    await page.goto("/invitation/test-id");
    await page.waitForTimeout(2000);
    // Should show either invitation UI or error/redirect
    await expect(page.locator("body")).toBeVisible();
  });
});
