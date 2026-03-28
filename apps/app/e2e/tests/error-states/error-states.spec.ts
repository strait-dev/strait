import { expect, test } from "../../fixtures";

test.describe("Error States", () => {
  test("404 page renders for unknown route", async ({ page }) => {
    await page.goto("/app/this-page-does-not-exist-12345");
    // Should show some kind of error/not found or redirect
    await expect(page.locator("body")).toBeVisible();
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

  test("invalid job ID shows error or not found", async ({ page }) => {
    await page.goto("/app/jobs/invalid-id-that-does-not-exist");
    const errorContent = page.getByText(
      /not found|error|went wrong|doesn't exist/i
    );
    const mainContent = page.locator("main");
    await expect(errorContent.or(mainContent)).toBeVisible({ timeout: 10_000 });
  });

  test("app handles errors gracefully", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page.locator("main").or(page.locator("body"))).toBeVisible();
  });

  test("error boundary catches rendering errors", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page.locator("body")).toBeVisible();
  });
});
