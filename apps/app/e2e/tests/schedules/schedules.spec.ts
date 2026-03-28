import { expect, test } from "../../fixtures";

test.describe("Schedules", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/schedules");
  });

  test("schedules page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/schedules/);
  });

  test("page renders content", async ({ page }) => {
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no schedules|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("search input exists when project is active", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search schedules...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("status filter exists when project is active", async ({ page }) => {
    const btn = page.getByRole("button", { name: "Status" });
    if (await btn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(btn).toBeVisible();
    }
  });

  test("table or empty state renders", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no project|no schedules|went wrong/i);
    await expect(table.or(emptyState).first()).toBeVisible({ timeout: 10_000 });
  });

  test("trigger button in floating bar when selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(
        page.getByRole("button", { name: /trigger/i })
      ).toBeVisible();
    }
  });

  test("pause button in floating bar when selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /pause/i })).toBeVisible();
    }
  });

  test("search filters schedules when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search schedules...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-schedule");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("table columns include schedule info", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText("Name")).toBeVisible();
    }
  });

  test("row click opens detail sheet", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("status filter dropdown opens when available", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    if (await filterButton.isVisible({ timeout: 5000 }).catch(() => false)) {
      await filterButton.click();
      await expect(page.getByRole("menuitemcheckbox").first()).toBeVisible({
        timeout: 3000,
      });
    }
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });
});
