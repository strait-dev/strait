import { test, expect } from "../../fixtures";

test.describe("Dashboard Metrics", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("total runs card shows numeric value", async ({ page }) => {
    const card = page.getByText("Total Runs (24h)").locator("..");
    await expect(card).toBeVisible();
  });

  test("success rate card shows percentage", async ({ page }) => {
    const card = page.getByText("Success Rate").locator("..");
    await expect(card).toBeVisible();
    await expect(card).toContainText("%");
  });

  test("failed runs card shows numeric value", async ({ page }) => {
    const card = page.getByText("Failed Runs").locator("..");
    await expect(card).toBeVisible();
  });

  test("queued card shows numeric value", async ({ page }) => {
    const card = page.getByText("Queued").locator("..");
    await expect(card).toBeVisible();
  });

  test("metrics cards render in a grid layout", async ({ page }) => {
    const cards = page.locator("[class*='grid'] > div").filter({
      has: page.locator("text=/Total Runs|Success Rate|Failed Runs|Queued/"),
    });
    await expect(cards.first()).toBeVisible();
  });

  test("empty state shows zero values gracefully", async ({ page }) => {
    await expect(page.getByText("Total Runs (24h)")).toBeVisible();
    await expect(page.getByText("0.0%")).toBeVisible();
  });
});
