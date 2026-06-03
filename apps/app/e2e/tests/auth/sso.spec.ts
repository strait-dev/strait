import { expect, test } from "@playwright/test";

test.describe("SSO", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page loads with disabled SSO roadmap form", async ({ page }) => {
    await page.goto("/sso");
    await expect(page.getByText("SSO roadmap").first()).toBeVisible();
    await expect(
      page.getByText("SSO is not available in launch plans")
    ).toBeVisible();
  });

  test("back to sign in link works", async ({ page }) => {
    await page.goto("/sso");
    await page.getByRole("link", { name: /back to sign in/i }).click();
    await expect(page).toHaveURL(/login/);
  });
});
