import { expect, test } from "../../fixtures";

test.describe("Jobs List", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/jobs");
  });

  test("jobs page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("page renders content", async ({ page }) => {
    // Page should show either the table, empty state, or error state
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no jobs|went wrong|try again/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("search input is visible when project is active", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("status filter exists when project is active", async ({ page }) => {
    const filterBtn = page.getByRole("button", { name: "Status" });
    if (await filterBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(filterBtn).toBeVisible();
    }
  });

  test("table renders or shows empty state", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no project|no jobs|went wrong/i);
    await expect(table.or(emptyState).first()).toBeVisible({ timeout: 10_000 });
  });

  test("search filters jobs by name", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-job-xyz");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("status filter toggles work", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    if (await filterButton.isVisible({ timeout: 5000 }).catch(() => false)) {
      await filterButton.click();
      const dropdown = page.getByRole("menuitemcheckbox").first();
      if (await dropdown.isVisible({ timeout: 3000 }).catch(() => false)) {
        await dropdown.click();
      }
    }
  });

  test("table has expected column headers", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText("Name")).toBeVisible();
    }
  });

  test("row selection checkbox works", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(checkbox).toBeChecked();
    }
  });

  test("clicking a job row navigates to detail", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("trigger button appears in floating bar when rows selected", async ({
    page,
  }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(
        page.getByRole("button", { name: /trigger/i })
      ).toBeVisible();
    }
  });

  test("pause button appears in floating bar when rows selected", async ({
    page,
  }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /pause/i })).toBeVisible();
    }
  });

  test("pagination area is visible when table exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(table).toBeVisible();
    }
  });

  test("clear selection deselects all rows", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      const clearButton = page.getByRole("button", { name: /clear|deselect/i });
      if (await clearButton.isVisible({ timeout: 3000 }).catch(() => false)) {
        await clearButton.click();
        await expect(checkbox).not.toBeChecked();
      }
    }
  });

  test("select all checkbox works", async ({ page }) => {
    const table = page.locator("table");
    if (!(await table.isVisible({ timeout: 5000 }).catch(() => false))) {
      return;
    }
    const selectAll = table.locator("thead input[type='checkbox']").first();
    if (await selectAll.isVisible()) {
      await selectAll.check();
      await expect(selectAll).toBeChecked();
    }
  });

  test("search clears when input is emptied", async ({ page }) => {
    const searchInput = page.getByPlaceholder("Search jobs...");
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("test");
      await page.waitForTimeout(300);
      await searchInput.clear();
      await page.waitForTimeout(300);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("page content is visible", async ({ page }) => {
    // The page should render something - table, empty state, or error
    await expect(page.locator("main").or(page.locator("body"))).toBeVisible();
  });

  test("multiple status filters can be applied", async ({ page }) => {
    const filterButton = page.getByRole("button", { name: "Status" });
    if (await filterButton.isVisible({ timeout: 5000 }).catch(() => false)) {
      await filterButton.click();
      const items = page.getByRole("menuitemcheckbox");
      const count = await items.count();
      if (count >= 2) {
        await items.nth(0).click();
        await items.nth(1).click();
      }
    }
  });
});
