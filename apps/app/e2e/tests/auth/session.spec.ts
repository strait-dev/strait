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

  test("logout clears session and redirects to login", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/\/app/);

    // Open the user menu in the header
    const userMenu = page.locator("[data-slot='avatar']").first();
    if (await userMenu.isVisible()) {
      await userMenu.click();
      await page.getByText("Sign out").click();
      await expect(page).toHaveURL(/login/, { timeout: 10_000 });
    }
  });
});
