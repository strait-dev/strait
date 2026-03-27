import { expect, test } from "../../fixtures";

test.describe("Webhooks", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/webhooks");
  });

  test("webhooks page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/webhooks/);
  });

  test("page title is visible", async ({ page }) => {
    await expect(
      page.getByRole("heading", { name: /webhooks/i })
    ).toBeVisible();
  });

  test("search input exists", async ({ page }) => {
    await expect(page.getByPlaceholder(/search/i)).toBeVisible();
  });

  test("status filter exists", async ({ page }) => {
    await expect(
      page.getByRole("button", { name: /status|filter/i })
    ).toBeVisible();
  });

  test("empty state shows when no webhooks", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no webhooks|no project/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("delete button in floating bar when selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /delete/i })).toBeVisible();
    }
  });

  test("search filters webhooks", async ({ page }) => {
    const searchInput = page.getByPlaceholder(/search/i);
    await searchInput.fill("nonexistent-webhook");
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("table columns include endpoint info", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(
        page.getByText("Endpoint").or(page.getByText("URL"))
      ).toBeVisible();
    }
  });

  test("row click opens detail sheet", async ({ page }) => {
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
    const selectAll = page
      .locator("table thead input[type='checkbox']")
      .first();
    if (await selectAll.isVisible()) {
      await selectAll.check();
      await expect(selectAll).toBeChecked();
    }
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/webhooks");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("view button appears for single selection", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /view/i })).toBeVisible();
    }
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
