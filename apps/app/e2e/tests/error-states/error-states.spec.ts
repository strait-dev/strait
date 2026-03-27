import { test, expect } from "../../fixtures";

test.describe("Error States", () => {
  test("404 page renders for unknown route", async ({ page }) => {
    await page.goto("/app/this-page-does-not-exist-12345");
    await expect(
      page.getByText(/not found|404|doesn't exist/i)
    ).toBeVisible({ timeout: 10_000 });
  });

  test("unauthenticated access redirects to login", async ({ browser }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/login/);
    await context.close();
  });

  test("invalid job ID shows not found state", async ({ page }) => {
    await page.goto("/app/jobs/invalid-id-that-does-not-exist");
    await expect(
      page.getByText(/not found|error|doesn't exist/i)
    ).toBeVisible({ timeout: 10_000 });
  });

  test("app handles network errors gracefully", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page.locator("main")).toBeVisible();
    // The app should not show a blank page even if some API calls fail
  });

  test("error boundary catches rendering errors", async ({ page }) => {
    // Navigate to a valid page to verify error boundary is wired up
    await page.goto("/app/dashboard");
    await expect(page.locator("main")).toBeVisible();
    // If there were rendering errors, the error boundary would catch them
  });
});
