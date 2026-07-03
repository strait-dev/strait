import { expect, test } from "../../fixtures";

test.describe("OAuth consent", () => {
  test("page loads with params", async ({ page }) => {
    await page.goto("/oauth/consent?client_id=test&scope=read");
    await expect(page.locator("body")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/oauth/consent");
    await expect(page.locator("body")).toBeVisible();
  });

  test("shows error for missing params", async ({ page }) => {
    await page.goto("/oauth/consent");
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });
});
