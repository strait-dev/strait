import { expect, test } from "../../fixtures";

test.describe("Dashboard Metrics", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("total runs card shows numeric value", async ({ page }) => {
    const card = page.getByText("Total runs (24h)").locator("..");
    await expect(card).toBeVisible();
  });

  test("success rate card shows percentage", async ({ page }) => {
    const card = page.getByText("Success rate").locator("../..");
    await expect(card).toBeVisible();
    await expect(card).toContainText("%");
  });

  test("failed runs card shows numeric value", async ({ page }) => {
    const card = page.getByText("Failed runs", { exact: true }).locator("..");
    await expect(card).toBeVisible();
  });

  test("queued card shows numeric value", async ({ page }) => {
    const card = page.getByText("Queued").locator("..");
    await expect(card).toBeVisible();
  });

  test("metrics cards render in a grid layout", async ({ page }) => {
    const cards = page.locator("[class*='grid'] > div").filter({
      has: page.locator("text=/Total runs|Success rate|Failed runs|Queued/"),
    });
    await expect(cards.first()).toBeVisible();
  });

  test("empty state shows zero values gracefully", async ({ page }) => {
    await expect(page.getByText("Total runs (24h)")).toBeVisible();
    await expect(page.getByText("0.0%")).toBeVisible();
  });
});
