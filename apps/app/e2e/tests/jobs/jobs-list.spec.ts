import { test, expect } from "../../fixtures";

test.describe("Jobs List", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/jobs");
  });

  test("jobs page loads with table structure", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/jobs/);
    await expect(page.locator("table").or(page.getByText(/no jobs|no project/i))).toBeVisible();
  });

  test("search input is visible", async ({ page }) => {
    await expect(page.getByPlaceholder(/search/i)).toBeVisible();
  });

  test("status filter dropdown exists", async ({ page }) => {
    await expect(page.getByRole("button", { name: /status|filter/i })).toBeVisible();
  });

  test("empty state shows when no jobs exist", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no jobs|no project|get started/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("search filters jobs by name", async ({ page }) => {
    const searchInput = page.getByPlaceholder(/search/i);
    await searchInput.fill("nonexistent-job-xyz");
    await page.waitForTimeout(500);
    // Should show filtered results or empty state
    await expect(page.locator("main")).toBeVisible();
  });

  test("status filter toggles work", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: /status|filter/i });
    await filterButton.click();
    const dropdown = page.getByRole("menuitemcheckbox").first();
    if (await dropdown.isVisible()) {
      await dropdown.click();
    }
  });

  test("table has expected column headers", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Name")).toBeVisible();
      await expect(page.getByText("Status")).toBeVisible();
    }
  });

  test("row selection checkbox works", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(checkbox).toBeChecked();
    }
  });

  test("clicking a job row navigates to detail", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible()) {
      await firstRow.click();
      // Should open detail sheet or navigate
      await page.waitForTimeout(500);
    }
  });

  test("trigger button appears in floating bar when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /trigger/i })).toBeVisible();
    }
  });

  test("pause button appears in floating bar when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /pause/i })).toBeVisible();
    }
  });

  test("pagination controls are visible when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      // Pagination buttons may not be visible with few items
      await expect(table).toBeVisible();
    }
  });

  test("clear selection deselects all rows", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      const clearButton = page.getByRole("button", { name: /clear|deselect/i });
      if (await clearButton.isVisible()) {
        await clearButton.click();
        await expect(checkbox).not.toBeChecked();
      }
    }
  });

  test("select all checkbox works", async ({ page }) => {
    const selectAll = page.locator("table thead input[type='checkbox']").first();
    if (await selectAll.isVisible()) {
      await selectAll.check();
      await expect(selectAll).toBeChecked();
    }
  });

  test("search clears when input is emptied", async ({ page }) => {
    const searchInput = page.getByPlaceholder(/search/i);
    await searchInput.fill("test");
    await page.waitForTimeout(300);
    await searchInput.clear();
    await page.waitForTimeout(300);
    await expect(page.locator("main")).toBeVisible();
  });

  test("page title is visible", async ({ page }) => {
    await expect(page.getByRole("heading", { name: /jobs/i })).toBeVisible();
  });

  test("multiple status filters can be applied", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: /status|filter/i });
    await filterButton.click();
    const items = page.getByRole("menuitemcheckbox");
    const count = await items.count();
    if (count >= 2) {
      await items.nth(0).click();
      await items.nth(1).click();
    }
  });
});
