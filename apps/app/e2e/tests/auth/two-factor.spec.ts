import { expect, test } from "@playwright/test";

test.describe("Two-Factor", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page loads with verification form", async ({ page }) => {
    await page.goto("/two-factor");
    await expect(page.getByText("Two-factor verification")).toBeVisible();
  });

  test("back to sign in link works", async ({ page }) => {
    await page.goto("/two-factor");
    await page.getByRole("link", { name: /back to sign in/i }).click();
    await expect(page).toHaveURL(/login/);
  });
});
