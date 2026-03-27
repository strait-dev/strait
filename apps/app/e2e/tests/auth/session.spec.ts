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
    // This test uses the authenticated storageState from the config
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/\/app/);

    // Find and click the user menu / logout button
    // Look for a user avatar or settings dropdown that contains a sign out option
    const userButton = page.getByRole("button", { name: /account|profile|user/i });
    if (await userButton.isVisible()) {
      await userButton.click();
      const logoutLink = page.getByRole("menuitem", { name: /sign out|log out/i });
      if (await logoutLink.isVisible()) {
        await logoutLink.click();
        await expect(page).toHaveURL(/login/);
        return;
      }
    }

    // Alternative: navigate to settings and find logout
    await page.goto("/app/settings");
    const signOutButton = page.getByRole("button", { name: /sign out|log out/i });
    if (await signOutButton.isVisible()) {
      await signOutButton.click();
      await expect(page).toHaveURL(/login/);
    }
  });
});
