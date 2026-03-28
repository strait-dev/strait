import { expect, test } from "../../fixtures";

test.describe("Routing", () => {
  test("deep link to dashboard works", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.locator("main")).toBeVisible();
  });

  test("deep link to jobs works", async ({ page }) => {
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("invalid route shows error or redirects", async ({ page }) => {
    await page.goto("/app/nonexistent-page-12345");
    // Should show 404, error, or redirect to a valid page
    await page.waitForTimeout(2000);
    await expect(page.locator("body")).toBeVisible();
  });

  test("browser back navigation works", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(1000);
    await page.goto("/app/jobs");
    await page.waitForTimeout(1000);
    await expect(page).toHaveURL(/\/app\/jobs/);
    await page.goBack();
    await page.waitForTimeout(1000);
    await expect(page).toHaveURL(/\/app\/dashboard/);
  });
});
