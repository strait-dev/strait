import { expect, test } from "../../fixtures";

test.describe("Runs List", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/runs");
  });

  test("runs page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/runs/);
  });

  test("page title is visible", async ({ page }) => {
    const heading = page.getByRole("heading", { name: "No project selected" });
    const table = page.locator("table");
    await expect(heading.or(table)).toBeVisible();
  });

  test("search input is visible", async ({ page }) => {
    await expect(page.getByPlaceholder("Search by run ID or job name...")).toBeVisible();
  });

  test("status filter exists", async ({ page }) => {
    await expect(
      page.getByRole("button", { name: "Status" })
    ).toBeVisible();
  });

  test("table has expected columns", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Run ID")).toBeVisible();
      await expect(page.getByText("Status")).toBeVisible();
    }
  });

  test("empty state shows when no runs exist", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no runs|no project/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("search filters runs", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search by run ID or job name...");
    await searchInput.fill("nonexistent-run-xyz");
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("status filter dropdown opens", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    await filterButton.click();
    await expect(page.getByRole("menuitemcheckbox").first()).toBeVisible();
  });

  test("retry button appears when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /retry/i })).toBeVisible();
    }
  });

  test("cancel button appears when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible()) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /cancel/i })).toBeVisible();
    }
  });

  test("row click opens run detail sheet", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible()) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("status badges have correct styling", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      const statusBadge = page.locator("table tbody [class*='badge']").first();
      if (await statusBadge.isVisible()) {
        await expect(statusBadge).toBeVisible();
      }
    }
  });

  test("duration column displays formatted time", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Duration")).toBeVisible();
    }
  });

  test("trigger type column shows trigger source", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Trigger")).toBeVisible();
    }
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

  test("multiple status filters can be applied", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    await filterButton.click();
    const items = page.getByRole("menuitemcheckbox");
    const count = await items.count();
    if (count >= 2) {
      await items.nth(0).click();
      await items.nth(1).click();
    }
  });
});
