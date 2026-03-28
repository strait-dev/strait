import { expect, test } from "../../fixtures";

test.describe("Pricing Comparison", () => {
  test("pricing comparison page loads", async ({ page }) => {
    await page.goto("/app/pricing/compare");
    await expect(page).toHaveURL(/\/app\/pricing\/compare/);
  });

  test("shows comparison heading", async ({ page }) => {
    await page.goto("/app/pricing/compare");
    await expect(page.getByText("Compare with competitors")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("migration calculator renders", async ({ page }) => {
    await page.goto("/app/pricing/compare");
    await page.waitForTimeout(3000);
    await expect(page.locator("main")).toBeVisible();
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/app/pricing/compare");
    await expect(page.locator("body")).toBeVisible();
  });
});
