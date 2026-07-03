import { expect, test } from "../../fixtures";

test.describe("Device authorization", () => {
  test("page loads with code param", async ({ page }) => {
    await page.goto("/device?code=TEST-CODE");
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/device");
    await expect(page.locator("body")).toBeVisible();
  });

  test("unauthenticated access redirects to login", async ({ browser }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto("/device?code=TEST");
    await expect(page).toHaveURL(/login/);
    await context.close();
  });
});
