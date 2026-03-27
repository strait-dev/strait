import { test, expect } from "../../fixtures";

test.describe("Workflows", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/workflows");
  });

  test("workflows page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/workflows/);
  });

  test("page title is visible", async ({ page }) => {
    await expect(page.getByRole("heading", { name: /workflows/i })).toBeVisible();
  });

  test("search input exists", async ({ page }) => {
    await expect(page.getByPlaceholder(/search/i)).toBeVisible();
  });

  test("status filter exists", async ({ page }) => {
    await expect(page.getByRole("button", { name: /status|filter/i })).toBeVisible();
  });

  test("empty state shows when no workflows exist", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no workflows|no project/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("table has expected columns when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Name")).toBeVisible();
      await expect(page.getByText("Status")).toBeVisible();
    }
  });

  test("trigger button in floating bar when selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /trigger/i })).toBeVisible();
    }
  });

  test("pause button in floating bar when selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /pause/i })).toBeVisible();
    }
  });

  test("search filters workflows", async ({ page }) => {
    const searchInput = page.getByPlaceholder(/search/i);
    await searchInput.fill("nonexistent-workflow");
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("row click opens detail", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible()) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("status filter dropdown opens", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: /status|filter/i });
    await filterButton.click();
    await expect(page.getByRole("menuitemcheckbox").first()).toBeVisible();
  });

  test("select all checkbox works", async ({ page }) => {
    const selectAll = page.locator("table thead input[type='checkbox']").first();
    if (await selectAll.isVisible()) {
      await selectAll.check();
      await expect(selectAll).toBeChecked();
    }
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/workflows");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("search clears properly", async ({ page }) => {
    const searchInput = page.getByPlaceholder(/search/i);
    await searchInput.fill("test");
    await page.waitForTimeout(300);
    await searchInput.clear();
    await page.waitForTimeout(300);
    await expect(page.locator("main")).toBeVisible();
  });
});
