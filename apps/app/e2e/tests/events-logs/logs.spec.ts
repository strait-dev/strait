import { test, expect } from "../../fixtures";

test.describe("Logs", () => {
  test("logs page loads", async ({ page }) => {
    await page.goto("/app/logs");
    await expect(page.locator("main")).toBeVisible();
  });

  test("logs table or empty state is visible", async ({ page }) => {
    await page.goto("/app/logs");
    const table = page.locator("table");
    const emptyState = page.getByText(/no logs|no project/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/logs");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("logs page has correct URL", async ({ page }) => {
    await page.goto("/app/logs");
    await expect(page).toHaveURL(/\/app\/logs/);
  });

  test("table has expected columns when data exists", async ({ page }) => {
    await page.goto("/app/logs");
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Status").or(page.getByText("Event"))).toBeVisible();
    }
  });
});
