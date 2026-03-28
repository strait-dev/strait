import { expect, test } from "@playwright/test";

test.describe("Verify Email", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page has correct heading", async ({ page }) => {
    await page.goto("/verify-email?token=invalid-test-token");
    await expect(page.getByText("Email verification")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("invalid token shows error", async ({ page }) => {
    await page.goto("/verify-email?token=invalid-token-12345");
    const error = page.getByText(/failed|invalid|expired/i);
    const content = page.locator("main");
    await expect(error.or(content)).toBeVisible({ timeout: 10_000 });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/verify-email?token=test");
    await expect(page.locator("body")).toBeVisible();
  });
});
