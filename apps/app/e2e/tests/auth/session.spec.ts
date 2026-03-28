import { expect, test } from "@playwright/test";

test.describe("Session", () => {
  test("unauthenticated access redirects to login", async ({ browser }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/login/);
    await context.close();
  });

  test("authenticated user can access app pages", async ({ page }) => {
    // This test uses the storageState from global-setup
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/\/app/);
  });
});
