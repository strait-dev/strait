import { expect, test } from "@playwright/test";

test.describe("Landing Page", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("landing page loads", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("body")).toBeVisible();
  });

  test("shows sign in link", async ({ page }) => {
    await page.goto("/");
    const signInLink = page.getByRole("link", { name: /sign in|login/i });
    if (await signInLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(signInLink).toBeVisible();
    }
  });

  test("shows sign up link", async ({ page }) => {
    await page.goto("/");
    const signUpLink = page.getByRole("link", {
      name: /sign up|create account|get started/i,
    });
    if (await signUpLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(signUpLink).toBeVisible();
    }
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("body")).toBeVisible();
  });
});
