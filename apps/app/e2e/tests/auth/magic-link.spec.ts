import { expect, test } from "@playwright/test";

test.describe("Magic Link", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page loads with email input", async ({ page }) => {
    await page.goto("/magic-link");
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible();
  });

  test("back to sign in link works", async ({ page }) => {
    await page.goto("/magic-link");
    await page.getByRole("link", { name: /back to sign in/i }).click();
    await expect(page).toHaveURL(/login/);
  });

  test("page has correct heading", async ({ page }) => {
    await page.goto("/magic-link");
    await expect(page.getByText("Magic link sign in")).toBeVisible();
  });
});
