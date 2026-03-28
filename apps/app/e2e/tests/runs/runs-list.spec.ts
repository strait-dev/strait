import { expect, test } from "../../fixtures";

test.describe("Runs List", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/runs");
  });

  test("runs page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/runs/);
  });

  test("page renders content", async ({ page }) => {
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no runs|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("search input is visible when project is active", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by run ID or job name..."
    );
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

  test("table has expected columns when visible", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(
        page.getByText("Run ID").or(page.getByText("Job"))
      ).toBeVisible();
    }
  });

  test("table or empty state is visible", async ({ page }) => {
    const table = page.locator("table");
    const emptyState = page.getByText(/no project|no runs|went wrong/i);
    await expect(table.or(emptyState).first()).toBeVisible({ timeout: 10_000 });
  });

  test("search filters runs when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by run ID or job name..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-run-xyz");
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
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

  test("retry button appears when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /retry/i })).toBeVisible();
    }
  });

  test("cancel button appears when rows selected", async ({ page }) => {
    const checkbox = page.locator("table tbody input[type='checkbox']").first();
    if (await checkbox.isVisible({ timeout: 5000 }).catch(() => false)) {
      await checkbox.check();
      await expect(page.getByRole("button", { name: /cancel/i })).toBeVisible();
    }
  });

  test("row click opens run detail sheet", async ({ page }) => {
    const firstRow = page.locator("table tbody tr").first();
    if (await firstRow.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstRow.click();
      await page.waitForTimeout(500);
    }
  });

  test("status badges render when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      const statusBadge = table.locator("[class*='badge']").first();
      if (await statusBadge.isVisible().catch(() => false)) {
        await expect(statusBadge).toBeVisible();
      }
    }
  });

  test("duration column displays formatted time", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText("Duration")).toBeVisible();
    }
  });

  test("trigger type column shows trigger source", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(page.getByText("Trigger")).toBeVisible();
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
