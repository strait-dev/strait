import { expect, test } from "@playwright/test";

test.describe("SSO", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page loads with SSO form", async ({ page }) => {
    await page.goto("/sso");
    await expect(page.getByText("Enterprise SSO")).toBeVisible();
  });

  test("back to sign in link works", async ({ page }) => {
    await page.goto("/sso");
    await page.getByRole("link", { name: /back to sign in/i }).click();
    await expect(page).toHaveURL(/login/);
  });
});
