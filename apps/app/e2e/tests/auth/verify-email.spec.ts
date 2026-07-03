import { expect, test } from "@playwright/test";

test.describe("Verify email", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page has correct heading", async ({ page }) => {
    await page.goto("/verify-email?token=invalid-test-token");
    await expect(page.getByText("Email verification")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("invalid token shows error", async ({ page }) => {
    await page.goto("/verify-email?token=invalid-token-12345");
    await expect(page.getByText("Verification failed")).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByText("Invalid token")).toBeVisible();
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/verify-email?token=test");
    await expect(page.locator("body")).toBeVisible();
  });
});
